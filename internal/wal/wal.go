package wal

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Wal struct {
	Filename string
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

func (m *Wal) Add(entry *LogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}
	f, err := os.OpenFile(m.Filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
