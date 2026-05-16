package agent

import (
	"errors"
	"reflect"
	"testing"
)

type fakeAgent struct {
	name string
}

func (f *fakeAgent) Name() string                                   { return f.name }
func (f *fakeAgent) DisplayName() string                            { return f.name }
func (f *fakeAgent) SupportedHooks() []HookType                     { return AllHooks() }
func (f *fakeAgent) Detect() (DetectResult, error)                  { return DetectResult{}, nil }
func (f *fakeAgent) Install(InstallOptions) error                   { return nil }
func (f *fakeAgent) Uninstall(Scope) error                          { return nil }
func (f *fakeAgent) Status(Scope) (Status, error)                   { return Status{}, nil }
func (f *fakeAgent) DecodeEvent([]byte, HookType) (*Event, error)   { return nil, nil }
func (f *fakeAgent) EncodeResponse(*Response, HookType) ([]byte, error) {
	return nil, nil
}

func TestRegisterAndGet(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	a := &fakeAgent{name: "alpha"}
	b := &fakeAgent{name: "beta"}
	Register(a)
	Register(b)

	got, err := Get("alpha")
	if err != nil {
		t.Fatalf("Get(alpha) error: %v", err)
	}
	if got != a {
		t.Fatalf("Get(alpha) returned wrong agent")
	}

	if _, err := Get("missing"); !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("Get(missing) error = %v, want ErrUnknownAgent", err)
	}
}

func TestNamesSorted(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	Register(&fakeAgent{name: "cursor"})
	Register(&fakeAgent{name: "claude-code"})

	want := []string{"claude-code", "cursor"}
	if got := Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	Register(&fakeAgent{name: "dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on duplicate Register")
		}
	}()
	Register(&fakeAgent{name: "dup"})
}

func TestRegisterNilPanics(t *testing.T) {
	t.Cleanup(resetForTest)
	resetForTest()

	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on nil Register")
		}
	}()
	Register(nil)
}

func TestScopeString(t *testing.T) {
	cases := map[Scope]string{
		ScopeUser:         "user",
		ScopeProject:      "project",
		ScopeProjectLocal: "project-local",
		Scope(99):         "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Scope(%d).String() = %q, want %q", s, got, want)
		}
	}
}
