# Ink

A minimal, Dynamo-style leaderless sharded and replicated key-value store written in Go.

## Features

* **Consistent Hashing**: Partitions key-space across nodes via a consistent hashing ring.
* **Quorum Reads & Writes**: Coordinates client operations with configurable quorum logic (e.g., 2-of-3 replicas) to maintain consistency.
* **Read Repair**: Identifies and repairs lagging or inconsistent replicas during read operations.
* **Tombstone Deletions**: Coordinates deletions via tombstones and write quorum replication to prevent key resurrection.
* **Crash Recovery**: Persists data to a Write-Ahead Log (WAL) to restore state upon node restart.

## Running the Cluster

To start a local 3-node cluster, run each command in a separate terminal:

```bash
# Start Node A on :8001
go run main.go 1

# Start Node B on :8002
go run main.go 2

# Start Node C on :8003
go run main.go 3
```

## API Usage

Clients can interact with any node in the cluster.

### 1. Write a Key
```bash
curl -X PUT -H "Content-Type: application/json" -d '{"value": "my-data"}' http://localhost:8001/my-key
```

### 2. Read a Key
```bash
curl http://localhost:8001/my-key
```

### 3. Delete a Key
```bash
curl -X DELETE http://localhost:8001/my-key
```

### 4. List All Active Keys
```bash
curl http://localhost:8001/
```

## Testing

Run unit and integration tests using:
```bash
go test ./...
```
