package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/electr1fy0/ink/internal/app"
	"github.com/electr1fy0/ink/internal/handler"
	"github.com/electr1fy0/ink/internal/ring"
	"github.com/electr1fy0/ink/internal/store"
	"github.com/electr1fy0/ink/internal/wal"
)

func TestRuntimeConfig(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		env        map[string]string
		wantError  string
		address    string
		standalone bool
	}{
		{name: "platform standalone", env: map[string]string{"PORT": "45555"}, address: "127.0.0.1:45555", standalone: true},
		{name: "local cluster unchanged", args: []string{"1"}, address: ":8001"},
		{name: "platform port overrides cluster", args: []string{"1"}, env: map[string]string{"PORT": "37029"}, address: "127.0.0.1:37029"},
		{name: "no arguments or port", wantError: "usage:"},
		{name: "extra arguments", args: []string{"1", "2"}, wantError: "usage:"},
		{name: "missing config", args: []string{"missing"}, wantError: "failed to open config"},
		{name: "invalid port", env: map[string]string{"PORT": "bad"}, wantError: "PORT must"},
		{name: "zero port", env: map[string]string{"PORT": "0"}, wantError: "PORT must"},
		{name: "large port", env: map[string]string{"PORT": "65536"}, wantError: "PORT must"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, dataDir, err := runtimeConfig(tc.args, func(k string) string { return tc.env[k] })
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v", err)
				}
				return
			}
			if err != nil || cfg.Address != tc.address || cfg.Standalone != tc.standalone {
				t.Fatalf("config = %+v, error = %v", cfg, err)
			}
			if tc.standalone && (len(cfg.Peers) != 0 || dataDir != filepath.Join(os.TempDir(), "ink-standalone")) {
				t.Fatalf("standalone config = %+v, data = %q", cfg, dataDir)
			}
			if len(tc.args) == 1 && len(cfg.Peers) != 2 {
				t.Fatalf("lost cluster peers: %+v", cfg)
			}
		})
	}
}

func TestStandaloneCRUDAndRecovery(t *testing.T) {
	dataDir := t.TempDir()
	cfg, gotDir, err := runtimeConfig(nil, func(k string) string {
		return map[string]string{"PORT": "45555", "INK_DATA_DIR": dataDir}[k]
	})
	if err != nil || gotDir != dataDir {
		t.Fatalf("config: %q, %v", gotDir, err)
	}
	start := func() *httptest.Server {
		r := ring.NewRing(10)
		r.AddNode(cfg.NodeID)
		a := app.NewApp(&store.MapStore{Data: make(map[string]store.Entry)}, &wal.Wal{Filename: filepath.Join(gotDir, "ink-wal-"+cfg.NodeID)}, r, &cfg)
		if err := a.Recover(); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(SetupRoutes(&handler.Handler{App: a}))
		t.Cleanup(server.Close)
		return server
	}
	request := func(s *httptest.Server, method, path, body string, status int) map[string]string {
		t.Helper()
		req, err := http.NewRequest(method, s.URL+path, bytes.NewBufferString(body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := s.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != status {
			t.Fatalf("%s %s = %d, want %d", method, path, resp.StatusCode, status)
		}
		var result map[string]string
		if resp.Header.Get("Content-Type") == "application/json" && path != "/" {
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	s := start()
	request(s, "GET", "/health", "", 200)
	request(s, "PUT", "/keep", `{"value":"persisted"}`, 200)
	request(s, "PUT", "/remove", `{"value":"deleted"}`, 200)
	request(s, "DELETE", "/remove", "", 204)
	request(s, "GET", "/remove", "", 404)
	s.Close()
	s = start()
	if got := request(s, "GET", "/keep", "", 200); got["value"] != "persisted" {
		t.Fatalf("recovered = %+v", got)
	}
	request(s, "GET", "/remove", "", 404)
	request(s, "GET", "/", "", 200)
	request(s, "PUT", "/keep", `{"value":"updated"}`, 200)
	if got := request(s, "GET", "/keep", "", 200); got["value"] != "updated" {
		t.Fatalf("updated = %+v", got)
	}
}

func TestClusterStillRequiresTwoAcknowledgements(t *testing.T) {
	cfg := &app.Config{NodeID: "alone"}
	r := ring.NewRing(10)
	r.AddNode(cfg.NodeID)
	a := app.NewApp(&store.MapStore{Data: make(map[string]store.Entry)}, &wal.Wal{Filename: filepath.Join(t.TempDir(), "wal")}, r, cfg)
	if err := a.Put("key", "value"); err == nil || !strings.Contains(err.Error(), "quorum") {
		t.Fatalf("cluster accepted one ack: %v", err)
	}
}
