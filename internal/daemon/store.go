package daemon

import (
	"fmt"

	"github.com/ujjalsharma100/lockie/internal/config"
	"github.com/ujjalsharma100/lockie/internal/store"
	"github.com/ujjalsharma100/lockie/internal/store/disk"
)

// OpenStore returns the durable store for the daemon. Tests pass a
// custom path via LOCKIE_DATA_DIR + aliases.json under that dir, or
// inject memory directly through NewHandler.
func OpenStore() (store.Store, error) {
	if _, err := config.EnsureUserDataDir(); err != nil {
		return nil, fmt.Errorf("daemon: %w", err)
	}
	st, err := disk.OpenDefault()
	if err != nil {
		return nil, fmt.Errorf("daemon: open store: %w", err)
	}
	return st, nil
}
