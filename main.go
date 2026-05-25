package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/electr1fy0/ink/internal/app"
	"github.com/electr1fy0/ink/internal/handler"
	"github.com/electr1fy0/ink/internal/ring"
	"github.com/electr1fy0/ink/internal/store"
	"github.com/electr1fy0/ink/internal/wal"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: ink <config_id>")
	}

	configID := os.Args[1]

	config, err := os.Open("config" + configID + ".yaml")
	if err != nil {
		log.Fatal("failed to open config: ", err)
	}

	var cfg app.Config
	if err = yaml.NewDecoder(config).Decode(&cfg); err != nil {
		log.Fatal("failed to parse config: ", err)
	}

	hashRing := ring.NewRing(10)
	hashRing.AddNode(cfg.NodeID)
	for _, p := range cfg.Peers {
		hashRing.AddNode(p.ID)
	}

	fmt.Printf("Starting node %s on %s\n", cfg.NodeID, cfg.Address)

	s := &store.MapStore{Data: make(map[string]store.Entry)}
	w := &wal.Wal{Filename: "ink-wal-" + cfg.NodeID} // unique wal per node

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
	mux.HandleFunc("PUT /{key}", handler.Handle(h.Put))
	mux.HandleFunc("GET /{key}", handler.Handle(h.Get))
	mux.HandleFunc("DELETE /{key}", handler.Handle(h.Delete))
	mux.HandleFunc("GET /", handler.Handle(h.GetAll))
	mux.HandleFunc("GET /health", h.Health)

	return mux
}
