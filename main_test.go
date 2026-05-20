package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/electr1fy0/ink/internal/wal"
	"github.com/google/go-cmp/cmp"
)

func TestAddToLog(t *testing.T) {
	curTime := time.Now()
	entry := &wal.LogEntry{
		"get", "meow", "wow", curTime,
	}
	fmt.Println("testing")
	w := wal.Wal{"log-test"}

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
