# Distributed Key-Value Storage System

A fault-tolerant, linearizable distributed key-value storage system built with the Raft consensus algorithm in Go. This implementation provides strong consistency guarantees and automatic fault recovery through leader election and log replication.

## Features

- **Strong Consistency**: Linearizable operations verified with the Porcupine linearizability checker
- **Fault Tolerance**: Automatic leader election and recovery from server failures
- **Distributed Consensus**: Full implementation of the Raft consensus algorithm
- **Request Deduplication**: Handles duplicate client requests to ensure exactly-once semantics
- **Persistent Storage**: State persistence for crash recovery
- **Concurrent Operations**: Support for multiple concurrent clients
- **Network Partition Handling**: Continues operation as long as a majority of servers are available

## Architecture

The system is built on a layered architecture:

```
┌─────────────────────────────────────┐
│         Client (Clerk)              │
│  - Get, Put, Append operations      │
│  - Leader discovery                 │
│  - Request retry logic              │
└──────────────┬──────────────────────┘
               │ RPC
┌──────────────▼──────────────────────┐
│      KVRaft Server Layer            │
│  - Request deduplication            │
│  - Operation application            │
│  - Result notification              │
└──────────────┬──────────────────────┘
               │
┌──────────────▼──────────────────────┐
│        Raft Consensus Layer         │
│  - Leader election                  │
│  - Log replication                  │
│  - Commitment decisions             │
│  - Persistence                      │
└─────────────────────────────────────┘
```

## Components

### Core Modules

- **`raft/`**: Complete Raft consensus algorithm implementation
  - Leader election with randomized timeouts
  - Log replication and consistency
  - Persistent state management
  - Heartbeat mechanism

- **`kvraft/`**: Key-value service layer
  - `server.go`: KV server with operation handling
  - `client.go`: Client (Clerk) implementation
  - `common.go`: RPC request/reply definitions
  - `KVStateMachine.go`: In-memory key-value store interface

- **`labrpc/`**: Custom RPC framework
  - Network simulation capabilities
  - Support for unreliable networks
  - Configurable delays and packet loss

- **`labgob/`**: Encoding/decoding library
  - Efficient serialization for RPC messages
  - Support for Go's encoding/gob

- **`porcupine/`**: Linearizability checker
  - Verifies linearizability of operations
  - Operation history analysis
  - Visualization support

- **`logger/`**: Structured logging system
  - Color-coded output for different servers
  - Timestamp tracking
  - Configurable log levels

- **`models/`**: Data models for testing
  - KV operation definitions
  - Linearizability model

## Getting Started

### Prerequisites

- Go 1.20 or higher
- Unix-like environment (Linux, macOS)

### Installation

1. Clone the repository:
```bash
git clone https://github.com/Abda11ah9/Distributed-K-V-Storage.git
cd Distributed-K-V-Storage
```

2. Navigate to the source directory:
```bash
cd src
```

3. Build the project:
```bash
go build ./...
```

## Usage

### Client API

The client (Clerk) provides three operations:

```go
import "lab5/kvraft"

// Create a new client
clerk := kvraft.MakeClerk(servers)

// Get a value
value := clerk.Get("mykey")

// Put a value (replaces existing value)
clerk.Put("mykey", "myvalue")

// Append to a value
clerk.Append("mykey", "moredata")
```

### Server Setup

```go
import (
    "lab5/kvraft"
    "lab5/labrpc"
    "lab5/raft"
)

// Create RPC endpoints for all servers
servers := make([]*labrpc.ClientEnd, 5)

// Start a KV server
persister := raft.MakePersister()
kv := kvraft.StartKVServer(servers, serverID, persister, -1)
```

## Implementation Details

### Raft Consensus

The Raft implementation includes:

- **Leader Election**
  - Randomized election timeouts (300-600ms)
  - Majority voting for leader selection
  - Term-based leadership

- **Log Replication**
  - AppendEntries RPC for log distribution
  - Consistency checks with previous log entries
  - Automatic log reconciliation

- **Commitment**
  - Commits entries replicated on majority
  - Sequential application of committed entries

### Request Deduplication

The system prevents duplicate execution of client requests:

- Each client has a unique `ClientId`
- Each request has a monotonically increasing `RequestId`
- Server tracks latest executed `RequestId` per client
- Duplicate requests return cached results without re-execution

### Fault Tolerance

- **Server Failures**: System continues with majority (⌈n/2⌉ + 1) of servers
- **Network Partitions**: Majority partition continues; minority blocks
- **Leader Crashes**: Automatic leader re-election
- **Message Loss**: Client retries with exponential backoff

### Linearizability

Operations are linearizable:
- All operations appear to execute atomically
- Operations respect real-time ordering
- Verified using the Porcupine checker in tests

## Performance Characteristics

- **Latency**: ~80ms timeout for leader operations
- **Throughput**: Optimized for concurrent client operations
- **Election Time**: 300-600ms randomized timeout
- **Heartbeat Interval**: 120ms

## Testing Strategy

The test suite validates:

1. **Basic Functionality**: Simple Get/Put/Append operations
2. **Concurrency**: Multiple clients operating simultaneously
3. **Unreliable Networks**: Random packet loss and delays
4. **Partitions**: Network splits and majority/minority scenarios
5. **Crash Recovery**: Server failures and restarts
6. **Linearizability**: All operations maintain linearizable consistency
7. **Performance**: Operations complete within acceptable time bounds

## Contributing

This is a proprietary project. See `Copyright.txt` for licensing information.

## Contact

For licensing inquiries: <firstname>.nbl<at>gmail.com

## Acknowledgments

- The Raft consensus algorithm: [In Search of an Understandable Consensus Algorithm](https://raft.github.io/raft.pdf)
- Porcupine linearizability checker: [github.com/anishathalye/porcupine](https://github.com/anishathalye/porcupine)

## References

- [Raft Paper](https://raft.github.io/raft.pdf) - Original Raft consensus algorithm
- [Raft Visualization](https://raft.github.io/) - Interactive Raft visualization
- [Linearizability](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf) - Herlihy & Wing paper on linearizability
