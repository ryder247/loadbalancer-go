# Go Load Balancer

A simple Layer 4 TCP load balancer implemented in Go with round-robin distribution and health checking.

## Features

- **Round-robin load balancing**: Distributes incoming connections evenly across backend servers
- **Health checking**: Automatically detects and excludes unhealthy backends
- **Concurrent handling**: Each connection is handled in a separate goroutine
- **Thread-safe**: Uses atomic operations and mutexes for concurrent access
- **Bidirectional proxy**: Full TCP proxy with data copying in both directions

## Usage

### Running the Load Balancer

```bash
go run main.go
```

The load balancer will start on port 8080 and distribute traffic to the following backend servers:
- 127.0.0.1:8081
- 127.0.0.1:8082  
- 127.0.0.1:8083

### Testing

Run unit tests:
```bash
go test -v
```

Run benchmark tests:
```bash
go test -bench=.
```

## Architecture

### Components

- **LoadBalancer**: Main struct that manages backends and handles connections
- **Backend**: Represents a backend server with address and health status
- **Round-robin selection**: Uses atomic counters for thread-safe backend selection
- **Health checker**: Runs every 30 seconds to verify backend availability

### Load Balancing Algorithm

The load balancer uses a simple round-robin algorithm with the following characteristics:
- Atomic counter for thread-safe selection
- Skips inactive backends
- Returns nil if no backends are available

### Health Checking

- Runs every 30 seconds in a separate goroutine
- Attempts TCP connection with 5-second timeout
- Automatically marks backends as active/inactive
- Logs status changes

## Configuration

To modify backend servers, edit the `backends` slice in the `main()` function:

```go
backends := []string{
    "127.0.0.1:8081",
    "127.0.0.1:8082",
    "127.0.0.1:8083",
}
```

## Performance

Benchmark results on Apple M2 Pro:
- GetNextBackend: ~90 ns/op
- Concurrent GetNextBackend: ~102 ns/op
- LoadBalancer creation: ~285 ns/op
- Health check cycle: ~2.2 ms/op

## Example Backend Servers

To test the load balancer, you can create simple backend servers:

```bash
# Terminal 1
python3 -m http.server 8081

# Terminal 2  
python3 -m http.server 8082

# Terminal 3
python3 -m http.server 8083
```

Then connect to the load balancer:
```bash
curl http://localhost:8080
```