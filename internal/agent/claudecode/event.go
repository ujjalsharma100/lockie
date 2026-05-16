package claudecode

import (
	"errors"

	"github.com/ujjalsharma100/lockie/internal/agent"
)

// errNotImplemented marks the wire-format translation work that step 8.7
// fills in. The interface is satisfied so the registry compiles; calling
// these in step 8.2 is a programmer error.
var errNotImplemented = errors.New("claudecode: event translation not implemented (step 8.7)")

// decodeEvent will translate a Claude Code hook event JSON into the
// canonical agent.Event. Stub for step 8.2.
func decodeEvent(_ []byte, _ agent.HookType) (*agent.Event, error) {
	return nil, errNotImplemented
}

// encodeResponse will translate the canonical agent.Response into the
// Claude Code wire format. Stub for step 8.2.
func encodeResponse(_ *agent.Response, _ agent.HookType) ([]byte, error) {
	return nil, errNotImplemented
}
