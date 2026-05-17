package audit

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ujjalsharma100/lockie/internal/config"
)

// Filter constrains events returned by Read.
type Filter struct {
	// Since drops events strictly before this instant (UTC).
	Since time.Time
	// Name matches Placeholder exactly when non-empty.
	Name string
}

// Log is a thread-safe append-only JSON-lines audit file.
type Log struct {
	path string
	mu   sync.Mutex
}

// OpenDefault opens (or creates) ~/.lockie/audit.log.
func OpenDefault() (*Log, error) {
	path, err := config.AuditPath()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

// Open opens (or creates) an audit log at path.
func Open(path string) (*Log, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("audit: mkdir %s: %w", dir, err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	_ = f.Close()
	return &Log{path: path}, nil
}

// Path returns the backing file path.
func (l *Log) Path() string { return l.path }

// Append writes one JSON line per event. Partial writes are avoided by
// buffering each line before a single Write call under the mutex.
func (l *Log) Append(events ...Event) error {
	if l == nil || len(events) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("audit: open %s: %w", l.path, err)
	}
	defer f.Close()
	for _, ev := range events {
		if ev.Timestamp.IsZero() {
			ev.Timestamp = time.Now().UTC()
		}
		line, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("audit: encode event: %w", err)
		}
		line = append(line, '\n')
		if _, err := f.Write(line); err != nil {
			return fmt.Errorf("audit: write %s: %w", l.path, err)
		}
	}
	return nil
}

// Read returns every event in the log matching f. An absent file yields
// a nil slice without error.
func Read(path string, f Filter) ([]Event, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("audit: read %s: %w", path, err)
	}
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			return nil, fmt.Errorf("audit: decode line: %w", err)
		}
		if !f.Since.IsZero() && ev.Timestamp.Before(f.Since) {
			continue
		}
		if f.Name != "" && ev.Placeholder != f.Name {
			continue
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("audit: scan %s: %w", path, err)
	}
	return out, nil
}

// Noop is an Appender that discards all events (for tests).
type Noop struct{}

func (Noop) Append(...Event) error { return nil }

// Appender persists audit events. *Log and Noop satisfy it.
type Appender interface {
	Append(events ...Event) error
}
