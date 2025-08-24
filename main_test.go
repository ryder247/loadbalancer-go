package main

import (
	"net"
	"sync"
	"testing"
)

func TestNewLoadBalancer(t *testing.T) {
	backends := []string{"127.0.0.1:8081", "127.0.0.1:8082"}
	lb := NewLoadBalancer(backends)

	if len(lb.backends) != 2 {
		t.Errorf("Expected 2 backends, got %d", len(lb.backends))
	}

	for i, backend := range lb.backends {
		if backend.Address != backends[i] {
			t.Errorf("Expected backend %d address %s, got %s", i, backends[i], backend.Address)
		}
		if !backend.Active {
			t.Errorf("Expected backend %d to be active", i)
		}
	}
}

func TestGetNextBackend(t *testing.T) {
	backends := []string{"127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083"}
	lb := NewLoadBalancer(backends)

	// Test round-robin selection
	selectedBackends := make(map[string]int)
	for i := 0; i < 6; i++ {
		backend := lb.GetNextBackend()
		if backend == nil {
			t.Fatal("GetNextBackend returned nil")
		}
		selectedBackends[backend.Address]++
	}

	// Each backend should be selected twice in 6 iterations
	for _, addr := range backends {
		if count := selectedBackends[addr]; count != 2 {
			t.Errorf("Expected backend %s to be selected 2 times, got %d", addr, count)
		}
	}
}

func TestGetNextBackendNoBackends(t *testing.T) {
	lb := NewLoadBalancer([]string{})
	backend := lb.GetNextBackend()
	if backend != nil {
		t.Error("Expected nil backend when no backends available")
	}
}

func TestGetNextBackendAllInactive(t *testing.T) {
	backends := []string{"127.0.0.1:8081", "127.0.0.1:8082"}
	lb := NewLoadBalancer(backends)

	// Mark all backends as inactive
	lb.mu.Lock()
	for i := range lb.backends {
		lb.backends[i].Active = false
	}
	lb.mu.Unlock()

	backend := lb.GetNextBackend()
	if backend != nil {
		t.Error("Expected nil backend when all backends are inactive")
	}
}

func TestGetBackendAddresses(t *testing.T) {
	backends := []string{"127.0.0.1:8081", "127.0.0.1:8082"}
	lb := NewLoadBalancer(backends)

	addresses := lb.getBackendAddresses()
	if len(addresses) != len(backends) {
		t.Errorf("Expected %d addresses, got %d", len(backends), len(addresses))
	}

	for i, addr := range addresses {
		if addr != backends[i] {
			t.Errorf("Expected address %s, got %s", backends[i], addr)
		}
	}
}

func TestConcurrentGetNextBackend(t *testing.T) {
	backends := []string{"127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083"}
	lb := NewLoadBalancer(backends)

	var wg sync.WaitGroup
	numGoroutines := 10
	selectionsPerGoroutine := 100
	
	results := make([][]string, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			results[goroutineID] = make([]string, selectionsPerGoroutine)
			
			for j := 0; j < selectionsPerGoroutine; j++ {
				backend := lb.GetNextBackend()
				if backend != nil {
					results[goroutineID][j] = backend.Address
				}
			}
		}(i)
	}
	
	wg.Wait()
	
	// Count total selections
	totalSelections := make(map[string]int)
	for _, result := range results {
		for _, addr := range result {
			if addr != "" {
				totalSelections[addr]++
			}
		}
	}
	
	// Each backend should be selected roughly equally
	expectedPerBackend := (numGoroutines * selectionsPerGoroutine) / len(backends)
	tolerance := expectedPerBackend / 10 // 10% tolerance
	
	for _, addr := range backends {
		count := totalSelections[addr]
		if count < expectedPerBackend-tolerance || count > expectedPerBackend+tolerance {
			t.Errorf("Backend %s selection count %d not within tolerance of expected %d", 
				addr, count, expectedPerBackend)
		}
	}
}

// Mock server for testing health checks
func startMockServer(address string, shouldFail bool) (net.Listener, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if !shouldFail {
				conn.Close()
			} else {
				conn.Close()
			}
		}
	}()
	
	return listener, nil
}

func TestHealthCheck(t *testing.T) {
	// Start a mock server that will be healthy
	healthyServer, err := startMockServer("127.0.0.1:0", false)
	if err != nil {
		t.Fatalf("Failed to start healthy server: %v", err)
	}
	defer healthyServer.Close()
	healthyAddr := healthyServer.Addr().String()
	
	// Use an address that will fail to connect
	unhealthyAddr := "127.0.0.1:99999"
	
	backends := []string{healthyAddr, unhealthyAddr}
	lb := NewLoadBalancer(backends)
	
	// Manually trigger health check
	lb.checkBackends()
	
	// Check that healthy backend is active and unhealthy is inactive
	lb.mu.RLock()
	var healthyBackend, unhealthyBackend *Backend
	for i := range lb.backends {
		if lb.backends[i].Address == healthyAddr {
			healthyBackend = &lb.backends[i]
		} else if lb.backends[i].Address == unhealthyAddr {
			unhealthyBackend = &lb.backends[i]
		}
	}
	lb.mu.RUnlock()
	
	if healthyBackend == nil || !healthyBackend.Active {
		t.Error("Healthy backend should be active")
	}
	
	if unhealthyBackend == nil || unhealthyBackend.Active {
		t.Error("Unhealthy backend should be inactive")
	}
}

// Benchmark tests
func BenchmarkGetNextBackend(b *testing.B) {
	backends := []string{
		"127.0.0.1:8081",
		"127.0.0.1:8082",
		"127.0.0.1:8083",
		"127.0.0.1:8084",
		"127.0.0.1:8085",
	}
	lb := NewLoadBalancer(backends)
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.GetNextBackend()
		}
	})
}

func BenchmarkGetNextBackendConcurrent(b *testing.B) {
	backends := []string{
		"127.0.0.1:8081",
		"127.0.0.1:8082",
		"127.0.0.1:8083",
		"127.0.0.1:8084",
		"127.0.0.1:8085",
	}
	lb := NewLoadBalancer(backends)
	
	b.ResetTimer()
	b.SetParallelism(10)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			backend := lb.GetNextBackend()
			if backend == nil {
				b.Error("GetNextBackend returned nil")
			}
		}
	})
}

func BenchmarkNewLoadBalancer(b *testing.B) {
	backends := []string{
		"127.0.0.1:8081",
		"127.0.0.1:8082",
		"127.0.0.1:8083",
		"127.0.0.1:8084",
		"127.0.0.1:8085",
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewLoadBalancer(backends)
	}
}

func BenchmarkHealthCheck(b *testing.B) {
	// Start mock servers
	var listeners []net.Listener
	var backends []string
	
	for i := 0; i < 5; i++ {
		listener, err := startMockServer("127.0.0.1:0", false)
		if err != nil {
			b.Fatalf("Failed to start mock server: %v", err)
		}
		listeners = append(listeners, listener)
		backends = append(backends, listener.Addr().String())
	}
	
	defer func() {
		for _, listener := range listeners {
			listener.Close()
		}
	}()
	
	lb := NewLoadBalancer(backends)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lb.checkBackends()
	}
}