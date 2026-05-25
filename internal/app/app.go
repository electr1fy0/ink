package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/electr1fy0/ink/internal/ring"
	"github.com/electr1fy0/ink/internal/store"
	"github.com/electr1fy0/ink/internal/wal"
)

type Config struct {
	Address string          `yaml:"address"`
	NodeID  string          `yaml:"node_id"`
	Peers   map[string]Peer `yaml:"peers"` // NodeID -> Peer
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

func (a *App) InternalGet(key string) (store.Entry, bool) {
	return a.Store.Get(key)
}

func (a *App) Get(key string) (store.Entry, bool) {
	nodes := a.Ring.GetNodes(key, 3)

	type nodeRes struct {
		nodeID string
		entry  *store.Entry
		err    error
	}
	resChan := make(chan nodeRes, len(nodes))

	for _, nodeID := range nodes {
		if nodeID == a.Cfg.NodeID {
			e, ok := a.InternalGet(key)
			if ok {
				resChan <- nodeRes{nodeID: nodeID, entry: &e}
			} else {
				resChan <- nodeRes{nodeID: nodeID, entry: nil}
			}
			continue
		}

		targetPeer, ok := a.Cfg.Peers[nodeID]
		if !ok {
			resChan <- nodeRes{nodeID: nodeID, err: fmt.Errorf("peer not found")}
			continue
		}

		go func(p Peer) {
			url := fmt.Sprintf("http://localhost%s/internal/%s", p.Address, key)
			client := &http.Client{Timeout: 1 * time.Second}
			resp, err := client.Get(url)
			if err != nil {
				resChan <- nodeRes{nodeID: p.ID, err: err}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotFound {
				resChan <- nodeRes{nodeID: p.ID, entry: nil}
				return
			}

			if resp.StatusCode != http.StatusOK {
				resChan <- nodeRes{nodeID: p.ID, err: fmt.Errorf("status %d", resp.StatusCode)}
				return
			}

			var e store.Entry
			if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
				resChan <- nodeRes{nodeID: p.ID, err: err}
				return
			}
			resChan <- nodeRes{nodeID: p.ID, entry: &e}
		}(targetPeer)
	}

	var latest *store.Entry

	// nodeID -> Entry in node
	responses := make(map[string]*store.Entry)
	successCount := 0

	timeout := time.After(2 * time.Second)
	for range len(nodes) {
		select {
		case res := <-resChan:
			if res.err == nil {
				successCount++
				responses[res.nodeID] = res.entry
				if res.entry != nil {
					if latest == nil || res.entry.Timestamp.After(latest.Timestamp) {
						latest = res.entry
					}
				}
			}
		case <-timeout:
			goto next
		}
	}
next:
	// quorum failed
	// we lose consistency for availability
	if successCount < 2 {
		entry, ok := a.Store.Get(key)
		if !ok || entry.Deleted {
			return store.Entry{}, false
		}
		return entry, true
	}

	if latest == nil {
		return store.Entry{}, false
	}

	go a.readRepair(key, *latest, responses)

	if latest.Deleted {
		return store.Entry{}, false
	}

	return *latest, true
}

func (a *App) readRepair(key string, latest store.Entry, responses map[string]*store.Entry) {
	for nodeID, entry := range responses {
		if entry == nil || entry.Timestamp.Before(latest.Timestamp) {
			if nodeID == a.Cfg.NodeID {
				a.Store.Add(key, latest)
				continue
			}

			if p, ok := a.Cfg.Peers[nodeID]; ok {
				body, _ := json.Marshal(latest)
				url := fmt.Sprintf("http://localhost%s/internal/%s", p.Address, key)
				req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				client := &http.Client{Timeout: 1 * time.Second}
				resp, err := client.Do(req)
				if err == nil {
					resp.Body.Close()
				}
			}
		}
	}
}

func (a *App) GetAll() map[string]store.Entry {
	all := a.Store.GetAll()
	active := make(map[string]store.Entry)
	for k, v := range all {
		if !v.Deleted {
			active[k] = v
		}
	}
	return active
}

func (a *App) InternalPut(key string, entry store.Entry) error {
	op := wal.Put
	if entry.Deleted {
		op = wal.Delete
	}
	logEntry := wal.LogEntry{
		Op:        op,
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
	_, ok := a.Get(key)
	if !ok {
		return fmt.Errorf("key not found")
	}

	entry := store.Entry{
		Timestamp: time.Now(),
		Deleted:   true,
	}

	if a.ReplicateWrite(&entry, key, 3) < 2 {
		return fmt.Errorf("failed to reach quorum of 2 acks")
	}

	logEntry := wal.LogEntry{
		Op:        wal.Delete,
		Key:       key,
		Timestamp: entry.Timestamp,
	}

	if err := a.Wal.Add(&logEntry); err != nil {
		return fmt.Errorf("wal write failed: %w", err)
	}
	a.Store.Add(key, entry)
	return nil
}

func (a *App) ReplicateWrite(v *store.Entry, key string, n int) int {
	nodes := a.Ring.GetNodes(key, n)
	body, err := json.Marshal(v)
	if err != nil {
		return 0
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	acks := 0

	for _, nodeID := range nodes {
		if nodeID == a.Cfg.NodeID {
			mu.Lock()
			acks++
			mu.Unlock()
			continue
		}

		if p, ok := a.Cfg.Peers[nodeID]; ok {
			wg.Add(1)
			go func(p Peer) {
				defer wg.Done()
				url := fmt.Sprintf("http://localhost%s/internal/%s", p.Address, key)
				req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
				if err != nil {
					return
				}
				req.Header.Set("Content-Type", "application/json")
				client := &http.Client{Timeout: 1 * time.Second}
				resp, err := client.Do(req)
				if err != nil {
					return
				}
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
					mu.Lock()
					acks++
					mu.Unlock()
				}
			}(p)
		}
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
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
			continue
		}
		switch entry.Op {
		case wal.Delete:
			a.Store.Add(entry.Key, store.Entry{Timestamp: entry.Timestamp, Deleted: true})
		default:
			a.Store.Add(entry.Key, store.Entry{Value: entry.Value, Timestamp: entry.Timestamp})
		}
	}
	return scanner.Err()
}
