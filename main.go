package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Backend represents a backend server
type Backend struct {
	Address string
	Active  bool
}

// LoadBalancer represents the L4 load balancer
type LoadBalancer struct {
	backends []Backend
	current  uint64 // for round-robin selection
	mu       sync.RWMutex
}

// NewLoadBalancer creates a new load balancer with given backends
func NewLoadBalancer(backends []string) *LoadBalancer {
	lb := &LoadBalancer{
		backends: make([]Backend, len(backends)),
	}

	for i, addr := range backends {
		lb.backends[i] = Backend{
			Address: addr,
			Active:  true,
		}
	}

	return lb
}

// GetNextBackend selects the next available backend using round-robin
func (lb *LoadBalancer) GetNextBackend() *Backend {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	if len(lb.backends) == 0 {
		return nil
	}

	// Simple round-robin selection
	for i := 0; i < len(lb.backends); i++ {
		idx := atomic.AddUint64(&lb.current, 1) % uint64(len(lb.backends))
		backend := &lb.backends[idx]

		if backend.Active {
			return backend
		}
	}

	return nil
}

// HealthCheck performs basic health checks on backends
func (lb *LoadBalancer) HealthCheck() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			lb.checkBackends()
		}
	}
}

func (lb *LoadBalancer) checkBackends() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	for i := range lb.backends {
		backend := &lb.backends[i]

		// Try to establish a connection to check if backend is alive
		conn, err := net.DialTimeout("tcp", backend.Address, 5*time.Second)
		if err != nil {
			if backend.Active {
				log.Printf("Backend %s is down: %v", backend.Address, err)
			}
			backend.Active = false
		} else {
			if !backend.Active {
				log.Printf("Backend %s is back online", backend.Address)
			}
			backend.Active = true
			conn.Close()
		}
	}
}

// HandleConnection handles incoming client connections
func (lb *LoadBalancer) HandleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	// Get next available backend
	backend := lb.GetNextBackend()
	if backend == nil {
		log.Println("No available backends")
		return
	}

	// Connect to backend server
	backendConn, err := net.DialTimeout("tcp", backend.Address, 10*time.Second)
	if err != nil {
		log.Printf("Failed to connect to backend %s: %v", backend.Address, err)
		return
	}
	defer backendConn.Close()

	log.Printf("Proxying connection from %s to %s",
		clientConn.RemoteAddr(), backend.Address)

	// Start bidirectional data copying
	var wg sync.WaitGroup
	wg.Add(2)

	// Copy data from client to backend
	go func() {
		defer wg.Done()
		defer backendConn.Close()
		_, err := io.Copy(backendConn, clientConn)
		if err != nil && err != io.EOF {
			log.Printf("Error copying client->backend: %v", err)
		}
	}()

	// Copy data from backend to client
	go func() {
		defer wg.Done()
		defer clientConn.Close()
		_, err := io.Copy(clientConn, backendConn)
		if err != nil && err != io.EOF {
			log.Printf("Error copying backend->client: %v", err)
		}
	}()

	// Wait for both goroutines to finish
	wg.Wait()
}

// Start starts the load balancer on the specified address
func (lb *LoadBalancer) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %v", address, err)
	}
	defer listener.Close()

	log.Printf("Load balancer started on %s", address)
	log.Printf("Backends: %v", lb.getBackendAddresses())

	// Start health checking in background
	go lb.HealthCheck()

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		// Handle each connection in a separate goroutine
		go lb.HandleConnection(conn)
	}
}

func (lb *LoadBalancer) getBackendAddresses() []string {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	addresses := make([]string, len(lb.backends))
	for i, backend := range lb.backends {
		addresses[i] = backend.Address
	}
	return addresses
}

func main() {
	// Define backend servers
	backends := []string{
		"127.0.0.1:8081",
		"127.0.0.1:8082",
		"127.0.0.1:8083",
	}

	// Create and start load balancer
	lb := NewLoadBalancer(backends)

	// Start load balancer on port 8080
	if err := lb.Start(":8080"); err != nil {
		log.Fatalf("Failed to start load balancer: %v", err)
	}
}
