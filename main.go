package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/electr1fy0/ink/internal/app"
	"github.com/electr1fy0/ink/internal/handler"
	"github.com/electr1fy0/ink/internal/ring"
	"github.com/electr1fy0/ink/internal/store"
	"github.com/electr1fy0/ink/internal/wal"
	"gopkg.in/yaml.v3"
)

func main() {
	cfg, dataDir, err := runtimeConfig(os.Args[1:], os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatal("failed to create data directory: ", err)
	}

	hashRing := ring.NewRing(10)
	hashRing.AddNode(cfg.NodeID)
	for _, p := range cfg.Peers {
		hashRing.AddNode(p.ID)
	}

	fmt.Printf("Starting node %s on %s\n", cfg.NodeID, cfg.Address)

	s := &store.MapStore{Data: make(map[string]store.Entry)}
	w := &wal.Wal{Filename: filepath.Join(dataDir, "ink-wal-"+cfg.NodeID)}
	// Check write access before becoming healthy, including on read-only releases.
	f, err := os.OpenFile(w.Filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal("failed to open writable log: ", err)
	}
	if err := f.Close(); err != nil {
		log.Fatal("failed to close log: ", err)
	}

	inkApp := app.NewApp(s, w, hashRing, &cfg)

	if err := inkApp.Recover(); err != nil {
		log.Fatal("failed to recover from log: ", err)
	}

	h := &handler.Handler{
		App: inkApp,
	}

	mux := SetupRoutes(h)
	log.Fatal(http.ListenAndServe(cfg.Address, mux))
}

func SetupRoutes(h *handler.Handler) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /internal/{key}", handler.Handle(h.InternalGet))
	mux.HandleFunc("PUT /internal/{key}", handler.Handle(h.InternalPut))
	mux.HandleFunc("DELETE /internal/{key}", handler.Handle(h.InternalDelete))
	mux.HandleFunc("PUT /{key}", handler.Handle(h.Put))
	mux.HandleFunc("GET /{key}", handler.Handle(h.Get))
	mux.HandleFunc("DELETE /{key}", handler.Handle(h.Delete))
	mux.HandleFunc("GET /", handler.Handle(h.GetAll))
	mux.HandleFunc("GET /health", h.Health)

	return mux
}

// A platform-provided PORT selects a standalone node when no cluster config
// is supplied. Explicit config IDs retain the original two-ack cluster mode.
func runtimeConfig(args []string, getenv func(string) string) (app.Config, string, error) {
	var cfg app.Config
	port := getenv("PORT")
	if len(args) > 1 || len(args) == 0 && port == "" {
		return cfg, "", fmt.Errorf("usage: ink <config_id>, or set PORT for standalone mode")
	}
	if len(args) == 1 {
		config, err := os.Open("config" + args[0] + ".yaml")
		if err != nil {
			return cfg, "", fmt.Errorf("failed to open config: %w", err)
		}
		defer config.Close()
		if err := yaml.NewDecoder(config).Decode(&cfg); err != nil {
			return cfg, "", fmt.Errorf("failed to parse config: %w", err)
		}
	} else {
		cfg.NodeID = "standalone"
		cfg.Standalone = true
	}
	if cfg.NodeID == "" || cfg.NodeID == "." || cfg.NodeID == ".." || strings.ContainsAny(cfg.NodeID, `/\`+"\x00") {
		return cfg, "", fmt.Errorf("node_id must be a nonempty file name")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return cfg, "", fmt.Errorf("PORT must be an integer between 1 and 65535")
		}
		cfg.Address = net.JoinHostPort("127.0.0.1", strconv.Itoa(n))
	}
	dataDir := getenv("INK_DATA_DIR")
	if dataDir == "" {
		dataDir = "."
		if port != "" {
			dataDir = filepath.Join(os.TempDir(), "ink-"+cfg.NodeID)
		}
	}
	return cfg, dataDir, nil
}
