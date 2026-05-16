package agent

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrUnknownAgent is returned by Get when the requested name is not
// registered.
var ErrUnknownAgent = errors.New("agent: unknown agent")

var (
	registryMu sync.RWMutex
	registry   = map[string]Agent{}
)

// Register adds a in the global agent registry. Each adapter package
// calls Register from its init() so the binary's CLI can find it by
// name. Registering the same name twice panics — agent names must be
// unique and known at compile time.
func Register(a Agent) {
	if a == nil {
		panic("agent.Register: nil Agent")
	}
	name := a.Name()
	if name == "" {
		panic("agent.Register: Agent.Name() is empty")
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("agent.Register: duplicate agent %q", name))
	}
	registry[name] = a
}

// Get returns the registered Agent for the given canonical name (e.g.
// "claude-code", "cursor"). Returns ErrUnknownAgent if the name has
// not been registered.
func Get(name string) (Agent, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAgent, name)
	}
	return a, nil
}

// All returns the registered agents in deterministic (name-sorted)
// order. Useful for `lockie status` and tests.
func All() []Agent {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]Agent, 0, len(registry))
	for _, a := range registry {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Names returns the canonical names of every registered agent, sorted.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// resetForTest clears the registry. Tests in this package use it to
// keep test cases hermetic; it is intentionally not exported.
func resetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = map[string]Agent{}
}
