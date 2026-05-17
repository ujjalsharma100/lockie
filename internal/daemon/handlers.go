package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ujjalsharma100/lockie/internal/detect"
	"github.com/ujjalsharma100/lockie/internal/placeholder"
	"github.com/ujjalsharma100/lockie/internal/store"
	"github.com/ujjalsharma100/lockie/internal/substitute"
	"github.com/ujjalsharma100/lockie/internal/version"
)

// Handler is the daemon's request dispatcher. It owns the long-lived
// state the server otherwise can't keep: the secret Store, the
// in-memory session registry, and the detector chain used for
// redaction. One Handler instance is shared across every accepted
// connection — its methods must be safe for concurrent use.
//
// Construction order in NewHandler matters: Detector is built first so
// each session can share the same engine without re-compiling the
// rule set on every session.start.
type Handler struct {
	store    store.Store
	detector detect.Detector
	sessions *sessionRegistry
	started  time.Time

	// newSessionID is overridable for tests; defaults to
	// store.NewValueID, which is timestamp-ordered and unique enough
	// for the lifetime of a single daemon process.
	newSessionID func() (string, error)
}

// NewHandler returns a Handler wired to the supplied store. A nil
// store is rejected — Phase 1 still wants the alias plane reachable
// from `hook.pre_tool` once step 8.8 lands, and tests pass a memory
// store anyway.
func NewHandler(st store.Store) (*Handler, error) {
	if st == nil {
		return nil, fmt.Errorf("daemon: NewHandler requires a non-nil Store")
	}
	det, err := detect.NewDefaultEngine()
	if err != nil {
		return nil, fmt.Errorf("daemon: build detector: %w", err)
	}
	return &Handler{
		store:    st,
		detector: det,
		sessions: newSessionRegistry(),
		started:  time.Now(),
		newSessionID: func() (string, error) {
			id, err := store.NewValueID()
			return string(id), err
		},
	}, nil
}

// Dispatch routes a Request to the per-method handler and returns the
// matching Response. It never returns a Go error — protocol errors are
// signalled via Response.Error so the server's per-connection loop
// keeps running after a malformed request.
func (h *Handler) Dispatch(req *Request) *Response {
	resp := &Response{ID: req.ID}
	switch req.Method {
	case MethodSessionStart:
		h.handleSessionStart(req, resp)
	case MethodSessionStop:
		h.handleSessionStop(req, resp)
	case MethodHookPrompt:
		h.handleHookPrompt(req, resp)
	case MethodHookPreTool:
		h.handleHookPreTool(req, resp)
	case MethodHookPostTool:
		h.handleHookPostTool(req, resp)
	case MethodAliasAdd:
		h.handleAliasAdd(req, resp)
	case MethodAliasList:
		h.handleAliasList(req, resp)
	case MethodAliasGet:
		h.handleAliasGet(req, resp)
	case MethodAliasForget:
		h.handleAliasForget(req, resp)
	case MethodHealth:
		h.handleHealth(req, resp)
	default:
		setError(resp, ErrCodeUnknownMethod, fmt.Sprintf("unknown method %q", req.Method))
	}
	return resp
}

func (h *Handler) handleSessionStart(req *Request, resp *Response) {
	var p SessionStartParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	id := p.SessionID
	if id == "" {
		newID, err := h.newSessionID()
		if err != nil {
			setError(resp, ErrCodeInternal, fmt.Sprintf("mint session id: %v", err))
			return
		}
		id = newID
	}
	h.sessions.getOrCreate(id)
	setResult(resp, SessionStartResult{SessionID: id})
}

func (h *Handler) handleSessionStop(req *Request, resp *Response) {
	var p SessionStopParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	if p.SessionID == "" {
		setError(resp, ErrCodeInvalidParams, "session_id is required")
		return
	}
	h.sessions.drop(p.SessionID)
	setResult(resp, SessionStopResult{})
}

func (h *Handler) handleHookPrompt(req *Request, resp *Response) {
	var p HookPromptParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	if p.SessionID == "" {
		setError(resp, ErrCodeInvalidParams, "session_id is required")
		return
	}
	sub := h.substituterFor(p.SessionID)
	out, events, err := sub.Redact([]byte(p.Prompt))
	if err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("redact prompt: %v", err))
		return
	}
	setResult(resp, HookPromptResult{Modified: len(events) > 0, Prompt: string(out)})
}

func (h *Handler) handleHookPreTool(req *Request, resp *Response) {
	var p HookPreToolParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	if p.SessionID == "" {
		setError(resp, ErrCodeInvalidParams, "session_id is required")
		return
	}
	sub := h.substituterFor(p.SessionID)
	rewritten, changed, err := rehydrateMap(sub, p.Input)
	if err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("rehydrate input: %v", err))
		return
	}
	setResult(resp, HookPreToolResult{Modified: changed, Input: rewritten})
}

func (h *Handler) handleHookPostTool(req *Request, resp *Response) {
	var p HookPostToolParams
	if err := decodeParams(req.Params, &p); err != nil {
		setError(resp, ErrCodeInvalidParams, err.Error())
		return
	}
	if p.SessionID == "" {
		setError(resp, ErrCodeInvalidParams, "session_id is required")
		return
	}
	sub := h.substituterFor(p.SessionID)
	out, changed, err := redactOutput(sub, p.Output)
	if err != nil {
		setError(resp, ErrCodeInternal, fmt.Sprintf("redact output: %v", err))
		return
	}
	setResult(resp, HookPostToolResult{Modified: changed, Output: out})
}

func (h *Handler) handleHealth(_ *Request, resp *Response) {
	setResult(resp, HealthResult{
		Version:       version.Version,
		UptimeSeconds: int64(time.Since(h.started).Seconds()),
		PID:           os.Getpid(),
		Sessions:      h.sessions.count(),
	})
}

// substituterFor returns a Substituter bound to the SessionMap for the
// given id; it lazily registers the session if the id is unknown so
// hosts that skip an explicit session.start (e.g. agents that fire
// PostToolUse before any SessionStart event) still get redaction
// without protocol-level failure.
func (h *Handler) substituterFor(sessionID string) *substitute.Substituter {
	sess := h.sessions.getOrCreate(sessionID)
	return &substitute.Substituter{Detector: h.detector, Session: sess}
}

// ---- session registry ------------------------------------------------------

type sessionRegistry struct {
	mu sync.RWMutex
	m  map[string]*placeholder.Session
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{m: make(map[string]*placeholder.Session)}
}

func (r *sessionRegistry) getOrCreate(id string) *placeholder.Session {
	r.mu.RLock()
	if s, ok := r.m[id]; ok {
		r.mu.RUnlock()
		return s
	}
	r.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	// Re-check under the write lock — another goroutine may have raced
	// us. Without this, two concurrent first-touches on the same id
	// would each install a fresh Session and the second would lose its
	// mintings on the next call.
	if s, ok := r.m[id]; ok {
		return s
	}
	s := placeholder.NewSession()
	r.m[id] = s
	return s
}

func (r *sessionRegistry) drop(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, id)
}

func (r *sessionRegistry) count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.m)
}

// ---- helpers ---------------------------------------------------------------

func decodeParams(raw json.RawMessage, into any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("decode params: %w", err)
	}
	return nil
}

func setResult(resp *Response, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		// json.Marshal failing on a struct we own is a programmer
		// error — degrade to a protocol-level error so the client
		// at least sees something coherent.
		setError(resp, ErrCodeInternal, fmt.Sprintf("encode result: %v", err))
		return
	}
	resp.Result = b
	resp.Error = nil
}

func setError(resp *Response, code int, msg string) {
	resp.Result = nil
	resp.Error = &ErrorObject{Code: code, Message: msg}
}

// redactOutput runs Redact over the string-valued fields of a
// HookPostToolOutput. It returns whether any field was rewritten.
func redactOutput(sub *substitute.Substituter, in HookPostToolOutput) (HookPostToolOutput, bool, error) {
	changed := false
	out := in
	for _, slot := range []struct {
		src *string
		dst *string
	}{
		{&in.Stdout, &out.Stdout},
		{&in.Stderr, &out.Stderr},
		{&in.Content, &out.Content},
		{&in.Diff, &out.Diff},
	} {
		if *slot.src == "" {
			continue
		}
		rewritten, events, err := sub.Redact([]byte(*slot.src))
		if err != nil {
			return HookPostToolOutput{}, false, err
		}
		*slot.dst = string(rewritten)
		if len(events) > 0 {
			changed = true
		}
	}
	return out, changed, nil
}

// rehydrateMap walks a tool-input map[string]any and rehydrates every
// string-valued leaf. Nested maps and arrays are walked recursively.
// The input map is not mutated — a fresh map is returned.
func rehydrateMap(sub *substitute.Substituter, in map[string]any) (map[string]any, bool, error) {
	if in == nil {
		return nil, false, nil
	}
	changed := false
	out := make(map[string]any, len(in))
	for k, v := range in {
		nv, c, err := rehydrateValue(sub, v)
		if err != nil {
			return nil, false, err
		}
		out[k] = nv
		if c {
			changed = true
		}
	}
	return out, changed, nil
}

func rehydrateValue(sub *substitute.Substituter, v any) (any, bool, error) {
	switch t := v.(type) {
	case string:
		rewritten, err := sub.Rehydrate([]byte(t))
		if err != nil {
			return nil, false, err
		}
		out := string(rewritten)
		return out, out != t, nil
	case map[string]any:
		return rehydrateMap(sub, t)
	case []any:
		changed := false
		arr := make([]any, len(t))
		for i, elem := range t {
			ne, c, err := rehydrateValue(sub, elem)
			if err != nil {
				return nil, false, err
			}
			arr[i] = ne
			if c {
				changed = true
			}
		}
		return arr, changed, nil
	default:
		return v, false, nil
	}
}
