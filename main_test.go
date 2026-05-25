package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/electr1fy0/ink/internal/app"
	"github.com/electr1fy0/ink/internal/handler"
	"github.com/electr1fy0/ink/internal/ring"
	"github.com/electr1fy0/ink/internal/store"
	"github.com/electr1fy0/ink/internal/wal"
	"github.com/google/go-cmp/cmp"
)

func TestAddToLog(t *testing.T) {
	curTime := time.Now()
	entry := &wal.LogEntry{
		Op:        "get",
		Key:       "meow",
		Value:     "wow",
		Timestamp: curTime,
	}
	fmt.Println("testing")
	w := wal.Wal{Filename: "log-test"}

	err := w.Add(entry)

	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile("log-test")
	if err != nil {
		t.Fatal(err)
	}

	readData := strings.TrimSpace(string(data))
	os.Remove("log-test")

	var got = &wal.LogEntry{}
	lines := strings.Split(readData, "\n")

	last := lines[len(lines)-1]

	if err := json.Unmarshal([]byte(last), got); err != nil {
		t.Fatal("failed to unmarshal", err)
	}
	if diff := cmp.Diff(got, entry); diff != "" {
		t.Fatalf("expected: %q, got %q", entry, got)
	}

}

func startTestNode(t *testing.T, id string, port string, peerConfigs map[string]app.Peer) (*app.App, *http.Server, func()) {
	t.Helper()
	s := &store.MapStore{Data: make(map[string]store.Entry)}
	walFilename := "ink-wal-test-" + id
	_ = os.Remove(walFilename)
	w := &wal.Wal{Filename: walFilename}

	hashRing := ring.NewRing(10)
	hashRing.AddNode(id)
	for peerID := range peerConfigs {
		hashRing.AddNode(peerID)
	}

	cfg := &app.Config{
		NodeID:  id,
		Address: port,
		Peers:   peerConfigs,
	}

	inkApp := app.NewApp(s, w, hashRing, cfg)
	h := &handler.Handler{App: inkApp}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/{key}", handler.Handle(h.InternalGet))
	mux.HandleFunc("PUT /internal/{key}", handler.Handle(h.InternalPut))
	mux.HandleFunc("PUT /{key}", handler.Handle(h.Put))
	mux.HandleFunc("GET /{key}", handler.Handle(h.Get))
	mux.HandleFunc("DELETE /{key}", handler.Handle(h.Delete))
	mux.HandleFunc("GET /", handler.Handle(h.GetAll))

	server := &http.Server{
		Addr:    port,
		Handler: mux,
	}

	listener, err := net.Listen("tcp", "127.0.0.1"+port)
	if err != nil {
		t.Fatalf("failed to listen on port %s: %v", port, err)
	}

	go func() {
		_ = server.Serve(listener)
	}()

	cleanup := func() {
		_ = server.Shutdown(context.Background())
		_ = os.Remove(walFilename)
	}

	return inkApp, server, cleanup
}

func TestDistributedDeleteTombstones(t *testing.T) {
	peersForNode1 := map[string]app.Peer{
		"node2": {Address: ":9902", ID: "node2"},
		"node3": {Address: ":9903", ID: "node3"},
	}
	peersForNode2 := map[string]app.Peer{
		"node1": {Address: ":9901", ID: "node1"},
		"node3": {Address: ":9903", ID: "node3"},
	}
	peersForNode3 := map[string]app.Peer{
		"node1": {Address: ":9901", ID: "node1"},
		"node2": {Address: ":9902", ID: "node2"},
	}

	app1, _, cleanup1 := startTestNode(t, "node1", ":9901", peersForNode1)
	defer cleanup1()

	app2, _, cleanup2 := startTestNode(t, "node2", ":9902", peersForNode2)
	defer cleanup2()

	_, _, cleanup3 := startTestNode(t, "node3", ":9903", peersForNode3)
	defer cleanup3()

	key := "my-key"
	val := "my-value"

	if err := app1.Put(key, val); err != nil {
		t.Fatalf("failed to Put key: %v", err)
	}

	entry, ok := app1.Get(key)
	if !ok {
		t.Fatalf("expected key to exist")
	}
	if entry.Value != val {
		t.Fatalf("expected value %s, got %s", val, entry.Value)
	}

	if err := app1.Delete(key); err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	_, ok = app1.Get(key)
	if ok {
		t.Fatalf("expected key to be deleted, but it was found")
	}

	_, ok = app2.Get(key)
	if ok {
		t.Fatalf("expected key to be deleted on replica node2, but it was found")
	}

	all := app1.GetAll()
	if _, exists := all[key]; exists {
		t.Fatalf("expected key to be excluded from GetAll")
	}

	sNew := &store.MapStore{Data: make(map[string]store.Entry)}
	hashRingNew := ring.NewRing(10)
	hashRingNew.AddNode("node1")
	hashRingNew.AddNode("node2")
	hashRingNew.AddNode("node3")

	app1Restarted := app.NewApp(sNew, &wal.Wal{Filename: "ink-wal-test-node1"}, hashRingNew, &app.Config{
		NodeID:  "node1",
		Address: ":9901",
		Peers:   peersForNode1,
	})

	if err := app1Restarted.Recover(); err != nil {
		t.Fatalf("failed to recover: %v", err)
	}

	recoveredEntry, ok := app1Restarted.InternalGet(key)
	if !ok {
		t.Fatalf("expected tombstone entry to exist in recovered store")
	}
	if !recoveredEntry.Deleted {
		t.Fatalf("expected recovered entry to have Deleted=true (tombstone)")
	}
}
