# Ink

A minimal, Dynamo-style leaderless sharded and replicated key-value store written in Go.
<img width="5172" height="2932" alt="Untitled-2026-02-10-1723" src="https://github.com/user-attachments/assets/f7ead91b-9b8c-4e93-86df-b188ad788473" />
<img width="3846" height="2592" alt="Untitled-2026-03-26-1616" src="https://github.com/user-attachments/assets/c116e72e-5d3d-4648-9ca4-107681ac544e" />

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

## Deploying a standalone node on Harbor

Import this repository with its included `paas.yaml`. Harbor builds the server,
starts it without arguments, and supplies `PORT`. Ink then runs as one standalone
node on `127.0.0.1:$PORT`; writes require one local acknowledgement. `/health`
is the readiness endpoint. No cluster config files or peers are needed.

`go run . 1`, `2`, and `3` continue to use the original cluster configs and
two-acknowledgement writes. Supplying `PORT` with a config ID overrides that
node's listener, so its peers must also be configured with the correct address.

Set `INK_DATA_DIR` to a writable, persistent directory to retain the write-ahead
log. Local cluster mode defaults to the current directory. Platform mode defaults
to `/tmp/ink-standalone`; Harbor's Linux sandbox gives it temporary storage that
is lost when the sandbox stops. A healthy deployment with that default is a demo,
not durable storage. Harbor currently mounts persistent directories only through
its managed SQLite bindings; adding `INK_DATA_DIR` alone does not create a mount.
Do not bind a SQLite service to Ink: Ink uses its own WAL, not `DATABASE_URL`.
Persistent standalone operation requires a writable volume mounted by the host.
Stop the old standalone process before starting another against the same WAL;
live overlapping releases are not supported for this in-memory store.

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
