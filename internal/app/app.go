package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/electr1fy0/ink/internal/ring"
	"github.com/electr1fy0/ink/internal/store"
	"github.com/electr1fy0/ink/internal/wal"
)

type Config struct {
	Address string `yaml:"address"`
	NodeID  string `yaml:"node_id"`
	Peers   []Peer `yaml:"peers"`
}

type Peer struct {
	Address string `yaml:"address"`
	ID      string `yaml:"id"`
}

type App struct {
	Store store.Store
	Wal   *wal.Wal
	Ring  *ring.Ring
	Cfg   *Config
}

func NewApp(s store.Store, w *wal.Wal, r *ring.Ring, cfg *Config) *App {
	return &App{
		Store: s,
		Wal:   w,
		Ring:  r,
		Cfg:   cfg,
	}
}

func (a *App) Get(key string) (store.Entry, bool) {
	return a.Store.Get(key)
}

func (a *App) GetAll() map[string]store.Entry {
	return a.Store.GetAll()
}

func (a *App) InternalPut(key string, entry store.Entry) error {
	logEntry := wal.LogEntry{
		Op:        wal.Put,
		Key:       key,
		Value:     entry.Value,
		Timestamp: entry.Timestamp,
	}
	if err := a.Wal.Add(&logEntry); err != nil {
		return fmt.Errorf("wal write failed: %w", err)
	}
	a.Store.Add(key, entry)
	return nil
}

func (a *App) Put(key string, value string) error {
	entry := store.Entry{
		Value:     value,
		Timestamp: time.Now(),
	}

	if a.ReplicateWrite(&entry, key, 3) < 2 {
		return fmt.Errorf("failed to reach quorum of 2 acks")
	}

	logEntry := wal.LogEntry{
		Op:        wal.Put,
		Key:       key,
		Value:     entry.Value,
		Timestamp: entry.Timestamp,
	}
	if err := a.Wal.Add(&logEntry); err != nil {
		return fmt.Errorf("wal write failed: %w", err)
	}
	a.Store.Add(key, entry)

	return nil
}

func (a *App) Delete(key string) error {
	if !a.Store.Delete(key) {
		return fmt.Errorf("key not found")
	}

	if a.Wal != nil {
		logEntry := wal.LogEntry{
			Op:        wal.Delete,
			Key:       key,
			Timestamp: time.Now(),
		}
		if err := a.Wal.Add(&logEntry); err != nil {
			return fmt.Errorf("wal write failed: %w", err)
		}
	}
	return nil
}

func (a *App) ReplicateWrite(v *store.Entry, key string, n int) int {
	nodes := a.Ring.GetNodes(key, n)
	body, err := json.Marshal(v)
	if err != nil {
		fmt.Printf("failed to marshal entry: %v\n", err)
		return 0
	}
	var result = make(chan bool, n)
	acks := 0
	for _, nodeID := range nodes {
		// Self-check
		if nodeID == a.Cfg.NodeID {
			acks++
			continue
		}

		for _, p := range a.Cfg.Peers {
			if nodeID == p.ID {
				go func(p Peer) {
					url := fmt.Sprintf("http://localhost%s/internal/%s", p.Address, key)
					req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
					if err != nil {
						result <- false
						return
					}
					req.Header.Set("Content-Type", "application/json")
					client := &http.Client{Timeout: 1 * time.Second}
					resp, err := client.Do(req)
					if err != nil {
						result <- false
						return
					}
					defer resp.Body.Close()
					result <- (resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK)
				}(p)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Wait for results from peers only
	peerCount := 0
	for _, nodeID := range nodes {
		if nodeID != a.Cfg.NodeID {
			peerCount++
		}
	}

	for range peerCount {
		select {
		case res := <-result:
			if res {
				acks++
			}
		case <-ctx.Done():
			return acks
		}
	}
	return acks
}

func (a *App) Recover() error {
	f, err := os.Open(a.Wal.Filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var entry wal.LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			fmt.Printf("failed to unmarshal log entry: %v\n", err)
			continue
		}
		switch entry.Op {
		case wal.Delete:
			a.Store.Delete(entry.Key)
		default:
			a.Store.Add(entry.Key, store.Entry{Value: entry.Value, Timestamp: entry.Timestamp})
		}
	}
	return scanner.Err()
}
