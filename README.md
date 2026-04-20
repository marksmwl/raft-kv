# Raft KV Store

A distributed key-value store built in Go, utilizing the Raft consensus algorithm for fault tolerance and high availability. It runs as a cluster of nodes where each node contains an HTTP API server, a Raft consensus module, and an in-memory key-value store.

## Features

- **Raft Consensus**: Implements leader election and distributed log replication.
- **In-Memory Storage**: Fast, thread-safe, in-memory key-value map (`map[string]string`).
- **HTTP API**: Provides a simple RESTful interface for clients.
- **Local Cluster**: Out-of-the-box configuration to run a 5-node cluster locally for testing and development.
- **gRPC**: The project uses gRPC for inter-node communication and client-server interactions.

## Architecture

1. **Client Requests**: Clients send `PUT`, `GET`, or `DELETE` requests to any node's HTTP API.
2. **Write Routing**: Write operations are forwarded to the current Raft leader.
3. **Log Replication**: The leader appends the command to its write-ahead log (WAL) and broadcasts it to follower nodes.
4. **Commit & Apply**: Once a majority of nodes acknowledge the log entry, it is considered committed and gets applied to the local key-value state machine.

## Getting Started

### Prerequisites

- [Go](https://golang.org/dl/) 1.20 or later

### Running the Cluster

To start a local 5-node cluster, run:

```bash
go run main.go
```

This command will initialize:
- **5 Raft nodes** (internal RPC communication on `localhost:8080` to `localhost:8084`)
- **5 HTTP API servers** (client requests on `localhost:9080` to `localhost:9084`)

### Usage

You can interact with the KV store using standard HTTP tools like `curl`:

**Set a key-value pair**
```bash
# Make sure to send this to the current leader's port (or it will fail/redirect based on implementation)
curl -X PUT http://localhost:9080/key/mykey -d "myvalue"
```

**Get a value**
```bash
curl http://localhost:9080/key/mykey
```

## Project Structure

- `main.go`: Application entry point; orchestrates and initializes the cluster.
- `api/`: HTTP API server and the KV state machine.
- `kvstore/`: The core in-memory storage engine.
- `raft/`: The Raft consensus implementation (election, log replication, RPCs).
- `proto/`: Protocol Buffer definitions for the ongoing gRPC migration.
- `stress_tests/`: Automated tests including network partition chaos scenarios and linearizability verification.

## Future Plans (Production-Grade Specs)

As outlined in the [Implementation Plan](implementation_plan.md), upcoming features include:
- Complete migration to gRPC.
- Persistent Write-Ahead Log (WAL) and state persistence across restarts.
- Snapshotting and log compaction.
- Linearizable reads and a smart gRPC client with automatic failover.
