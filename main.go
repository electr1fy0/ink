package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/electr1fy0/ink/internal/ring"
	"gopkg.in/yaml.v3"
)

var cfg Config

type Store interface {
	Add(string, Entry)
	Get(string) (Entry, bool)
	Delete(string) bool
	GetAll() map[string]Entry
}

type Op string

const (
	Put    Op = "put"
	Delete Op = "delete"
	Get    Op = "get"
)

type LogEntry struct {
	Op        Op
	Key       string
	Value     string
	Timestamp time.Time
}

type MapStore struct {
	mu   sync.RWMutex
	data map[string]Entry
	wal  Wal
}

func (m *MapStore) Add(key string, entry Entry) {
	m.mu.Lock()
	exists, ok := m.data[key]

	// no op if already newer entry
	if exists.Timestamp.Before(entry.Timestamp) || !ok {
		m.data[key] = entry
	}

	m.mu.Unlock()
}

func (m *MapStore) Get(key string) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, ok := m.data[key]
	return value, ok
}

func (m *MapStore) Delete(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.data[key]; !ok {
		return false
	}
	delete(m.data, key)
	return true
}

func (m *MapStore) GetAll() map[string]Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]Entry, len(m.data))
	for key, value := range m.data {
		out[key] = value
	}

	return out
}

type Wal struct {
	filename string
}

func (m *Wal) Add(entry *LogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}
	f, err := os.OpenFile(m.filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log-file: %w", err)
	}
	defer f.Close()
	data = []byte(string(data) + "\n")
	_, err = f.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to log-file: %w", err)
	}
	err = f.Sync()
	if err != nil {
		return fmt.Errorf("failed to fsync the log-file %w", err)
	}

	return nil
}

type Handler struct {
	Store Store
	Wal   *Wal
}

type HttpError struct {
	Status  int
	Message string
	Err     error
}

func (h *HttpError) Error() string {
	return h.Message
}

func httpError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	var httpErr *HttpError
	if ok := errors.As(err, &httpErr); ok {
		http.Error(w, httpErr.Message, httpErr.Status)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpError(w, fn(w, r))
	}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	value, ok := h.Store.Get(key)
	if !ok {
		return &HttpError{
			Status:  http.StatusNotFound,
			Message: "not found",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"key":   key,
		"value": value.Value,
	}); err != nil {
		return &HttpError{
			Status:  http.StatusInternalServerError,
			Message: "failed to encode response",
			Err:     err,
		}
	}

	return nil
}

type Entry struct {
	Value     string    `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

func (h *Handler) InternalPut(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	var toWrite Entry
	if err := json.NewDecoder(r.Body).Decode(&toWrite); err != nil {
		return &HttpError{
			Status:  http.StatusBadRequest,
			Message: "failed to decode internal put body",
			Err:     err,
		}
	}

	logEntry := LogEntry{
		Op:        "write",
		Key:       key,
		Value:     toWrite.Value,
		Timestamp: toWrite.Timestamp,
	}
	if err := h.Wal.Add(&logEntry); err != nil {
		return &HttpError{
			Status:  http.StatusInternalServerError,
			Message: "wal write failed",
			Err:     err,
		}
	}
	h.Store.Add(key, toWrite)

	w.WriteHeader(201)
	w.Write([]byte("written"))

	return nil
}

func (h *Handler) Put(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	var v Entry
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return &HttpError{
			Status:  http.StatusBadRequest,
			Message: err.Error(),
			Err:     err,
		}
	}

	fmt.Println("before replicate")
	if ReplicateWrite(&v, key, 3) < 2 {
		return &HttpError{
			Status:  http.StatusServiceUnavailable,
			Message: "failed to reach quorum of 2 acks",
		}
	} else {
		json.NewEncoder(w).Encode(map[string]string{
			"response": "quorum achieved",
		})
	}

	fmt.Println("loop")
	return nil
}

func ReplicateWrite(v *Entry, key string, n int) int {
	fmt.Println("loop")
	nodes := hashRing.GetNodes(key, n)
	body, err := json.Marshal(v)
	var result = make(chan bool, n)
	acks := 0
	for _, nodeID := range nodes {
		for _, p := range cfg.Peers {
			if nodeID == p.ID {
				go func(p Peer) {
					if err != nil {
						result <- false
						return
					}
					// forming http://localhost:port/{key}
					// address is : + port already
					url := fmt.Sprintf("http://localhost%s/internal/%s", p.Address, key)
					req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
					if err != nil {
						result <- false
						return
					}
					req.Header.Set("Content-Type", "application/json")
					client := &http.Client{}
					resp, err := client.Do(req)
					if err != nil {
						result <- false
						return
					}

					defer resp.Body.Close()
					if err != nil {
						result <- false
					}

					result <- true
				}(p)
			}
		}
	}
channelLoop:
	for {
		select {
		case res := <-result:
			if res {
				acks++
			}
		case <-time.After(1 * time.Second):
			break channelLoop
		}
	}
	return acks
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	if !h.Store.Delete(key) {
		return &HttpError{
			Status:  http.StatusNotFound,
			Message: "not found",
		}
	}

	if h.Wal != nil {
		if err := h.Wal.Add(&LogEntry{
			Op:        Delete,
			Key:       key,
			Timestamp: time.Now(),
		}); err != nil {
			return &HttpError{
				Status:  http.StatusInternalServerError,
				Message: "wal write failed",
				Err:     err,
			}
		}
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) GetAll(w http.ResponseWriter, r *http.Request) error {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(h.Store.GetAll()); err != nil {
		return &HttpError{
			Status:  http.StatusInternalServerError,
			Message: "failed to encode response",
			Err:     err,
		}
	}
	return nil
}
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("don't worry about me mate"))
}

type Config struct {
	Address string `yaml:"address"`
	NodeID  string `yaml:"node_id"`
	Peers   []Peer
}

type Peer struct {
	Address string `yaml:"address"`
	ID      string `yaml:"id"`
}

var hashRing = ring.NewRing(10)

func main() {
	configID := os.Args[1]
	config, err := os.Open("config" + configID + ".yaml")
	if err != nil {
		panic(err)
	}

	yaml.NewDecoder(config).Decode(&cfg)
	hashRing.AddNode(cfg.NodeID)
	for _, p := range cfg.Peers {
		hashRing.AddNode(p.ID)
	}

	fmt.Printf("%+v", cfg)

	store := &MapStore{data: make(map[string]Entry)}
	wal := &Wal{"ink-wal"}

	h := Handler{
		store, wal,
	}

	data, err := os.ReadFile(h.Wal.filename)
	if err != nil && !os.IsNotExist(err) {
		log.Fatal("failed to read log file: ", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry LogEntry
		err := json.Unmarshal([]byte(line), &entry)
		if err != nil {
			log.Fatal("failed to unmarshal log entry: ", err)
		}
		switch entry.Op {
		case Delete:
			h.Store.Delete(entry.Key)
		default:
			h.Store.Add(entry.Key, Entry{Value: entry.Value, Timestamp: entry.Timestamp})
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /{key}", handle(h.Put))

	mux.HandleFunc("PUT /internal/{key}", handle(h.InternalPut))
	mux.HandleFunc("GET /{key}", handle(h.Get))
	mux.HandleFunc("DELETE /{key}", handle(h.Delete))
	mux.HandleFunc("GET /", handle(h.GetAll))

	log.Fatal(http.ListenAndServe(cfg.Address, mux))
}
