package cursor

import (
	"errors"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

var errNotImplemented = errors.New("cursor: event translation not implemented (step 8.7)")

// decodeEvent will translate a Cursor hook event JSON into the
// canonical agent.Event. Stub for step 8.2.
func decodeEvent(_ []byte, _ agent.HookType) (*agent.Event, error) {
	return nil, errNotImplemented
}

// encodeResponse will translate the canonical agent.Response into the
// Cursor wire format. Stub for step 8.2.
func encodeResponse(_ *agent.Response, _ agent.HookType) ([]byte, error) {
	return nil, errNotImplemented
}
