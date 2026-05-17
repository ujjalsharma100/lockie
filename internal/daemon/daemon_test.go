package daemon_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ujjalsharma100/lockie/internal/audit"
	"github.com/ujjalsharma100/lockie/internal/daemon"
	"github.com/ujjalsharma100/lockie/internal/store"
	"github.com/ujjalsharma100/lockie/internal/store/disk"
	"github.com/ujjalsharma100/lockie/internal/store/memory"
	"github.com/ujjalsharma100/lockie/internal/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.RunMain(m))
}

// startTestDaemon boots a server on a per-test socket and returns a
// teardown func. macOS caps `struct sockaddr_un.sun_path` at 104
// bytes, and `t.TempDir()` on darwin lands under
// /var/folders/.../T/<TestName>/<NNN>/ which routinely blows past
// that. We mint a short directory under the resolved tmp root and
// register a t.Cleanup, sidestepping the ceiling while staying
// parallel-safe.
func startTestDaemon(t *testing.T) (socketPath string, stop func()) {
	t.Helper()
	return startTestDaemonWithStore(t, memory.New(), audit.Noop{})
}

func startTestDaemonWithStore(t *testing.T, st store.Store, auditLog audit.Appender) (socketPath string, stop func()) {
	t.Helper()
	dir, err := os.MkdirTemp(shortTmp(t), "lk-")
	if err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath = filepath.Join(dir, "d.sock")
	h, err := daemon.NewHandlerWith(st, auditLog)
	if err != nil {
		t.Fatalf("NewHandlerWith: %v", err)
	}
	srv := daemon.NewServer(socketPath, h)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	stop = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
		_ = st.Close()
	}
	return socketPath, stop
}

func TestDaemon_AliasAddListForget(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "aliases.json")
	st, err := disk.Open(path)
	if err != nil {
		t.Fatalf("disk.Open: %v", err)
	}
	sock, stop := startTestDaemonWithStore(t, st, audit.Noop{})
	defer stop()

	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := c.AliasAdd(ctx, daemon.AliasAddParams{Name: "MYKEY", Value: "xyz"}); err != nil {
		t.Fatalf("AliasAdd: %v", err)
	}
	list, err := c.AliasList(ctx, daemon.AliasListParams{})
	if err != nil {
		t.Fatalf("AliasList: %v", err)
	}
	if len(list.Aliases) != 1 || list.Aliases[0].Name != "MYKEY" {
		t.Fatalf("AliasList = %#v; want one MYKEY", list.Aliases)
	}
	info, err := c.AliasGet(ctx, daemon.AliasGetParams{Name: "MYKEY"})
	if err != nil {
		t.Fatalf("AliasGet: %v", err)
	}
	if info.Hash == "" {
		t.Errorf("AliasGet hash empty")
	}
	if err := c.AliasForget(ctx, daemon.AliasForgetParams{Name: "MYKEY"}); err != nil {
		t.Fatalf("AliasForget: %v", err)
	}
	list2, err := c.AliasList(ctx, daemon.AliasListParams{})
	if err != nil {
		t.Fatalf("AliasList after forget: %v", err)
	}
	if len(list2.Aliases) != 0 {
		t.Fatalf("aliases still present: %#v", list2.Aliases)
	}
}

func TestDaemon_AuditLogOnPostTool(t *testing.T) {
	t.Parallel()
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	auditLog, err := audit.Open(auditPath)
	if err != nil {
		t.Fatalf("audit.Open: %v", err)
	}
	sock, stop := startTestDaemonWithStore(t, memory.New(), auditLog)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sid := openSession(ctx, t, c)
	_, err = c.HookPostTool(ctx, daemon.HookPostToolParams{
		SessionID: sid,
		Tool:      "Read",
		Output:    daemon.HookPostToolOutput{Content: "key=" + testutil.StripeSecretKey + "\n"},
	})
	if err != nil {
		t.Fatalf("HookPostTool: %v", err)
	}
	events, err := audit.Read(auditPath, audit.Filter{})
	if err != nil {
		t.Fatalf("audit.Read: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected audit events after post-tool redaction")
	}
	if events[0].SessionID != sid || events[0].Tool != "Read" {
		t.Errorf("event context = session %q tool %q", events[0].SessionID, events[0].Tool)
	}
	if !strings.HasPrefix(events[0].Placeholder, "STRIPE_KEY_") {
		t.Errorf("placeholder = %q", events[0].Placeholder)
	}
	raw, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(raw), testutil.StripeSecretKey) {
		t.Fatal("audit log contains literal secret")
	}
}

func TestDaemon_Health(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	h, err := c.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.PID <= 0 {
		t.Errorf("Health.PID = %d; want > 0", h.PID)
	}
	if h.Version == "" {
		t.Errorf("Health.Version is empty")
	}
}

func TestDaemon_SessionStartMintsID(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r, err := c.SessionStart(ctx, daemon.SessionStartParams{})
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if r.SessionID == "" {
		t.Fatalf("session id is empty")
	}
}

func TestDaemon_SessionStartHonoursProvidedID(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	want := "cc-session-abc123"
	r, err := c.SessionStart(ctx, daemon.SessionStartParams{SessionID: want})
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	if r.SessionID != want {
		t.Errorf("session id = %q; want %q", r.SessionID, want)
	}
}

func TestDaemon_HookPromptRedacts(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sid := openSession(ctx, t, c)
	prompt := "please use api key " + testutil.StripeSecretKey + " for the call"
	r, err := c.HookPrompt(ctx, daemon.HookPromptParams{
		SessionID: sid,
		Prompt:    prompt,
	})
	if err != nil {
		t.Fatalf("HookPrompt: %v", err)
	}
	if !r.Modified {
		t.Fatalf("expected Modified=true; got false (out=%q)", r.Prompt)
	}
	if strings.Contains(r.Prompt, testutil.StripeSecretKey) {
		t.Fatalf("redacted prompt still contains literal: %q", r.Prompt)
	}
	if !strings.Contains(r.Prompt, "STRIPE_KEY_") {
		t.Fatalf("redacted prompt missing placeholder: %q", r.Prompt)
	}
}

func TestDaemon_PostToolRedact_ThenPreToolRehydrate(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sid := openSession(ctx, t, c)

	// 1) PostToolUse: tool emitted the literal — redact it.
	post, err := c.HookPostTool(ctx, daemon.HookPostToolParams{
		SessionID: sid,
		Tool:      "Read",
		Output:    daemon.HookPostToolOutput{Content: "API_KEY=" + testutil.StripeSecretKey + "\n"},
	})
	if err != nil {
		t.Fatalf("HookPostTool: %v", err)
	}
	if !post.Modified {
		t.Fatalf("post-tool not modified; out=%q", post.Output.Content)
	}
	placeholder := extractPlaceholder(t, post.Output.Content, "STRIPE_KEY_")

	// 2) PreToolUse: model echoed the placeholder back; rehydrate.
	pre, err := c.HookPreTool(ctx, daemon.HookPreToolParams{
		SessionID: sid,
		Tool:      "Bash",
		Input:     map[string]any{"command": "curl -H 'Authorization: Bearer " + placeholder + "' x"},
	})
	if err != nil {
		t.Fatalf("HookPreTool: %v", err)
	}
	if !pre.Modified {
		t.Fatalf("pre-tool not modified; in=%v", pre.Input)
	}
	got, _ := pre.Input["command"].(string)
	if !strings.Contains(got, testutil.StripeSecretKey) {
		t.Fatalf("rehydrated command missing literal: %q", got)
	}
	if strings.Contains(got, placeholder) {
		t.Fatalf("rehydrated command still contains placeholder: %q", got)
	}
}

func TestDaemon_SessionIsolation(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sidA := openSession(ctx, t, c)
	sidB := openSession(ctx, t, c)

	// Mint a placeholder in A.
	post, err := c.HookPostTool(ctx, daemon.HookPostToolParams{
		SessionID: sidA,
		Tool:      "Read",
		Output:    daemon.HookPostToolOutput{Content: testutil.StripeSecretKey},
	})
	if err != nil {
		t.Fatalf("HookPostTool A: %v", err)
	}
	placeholder := extractPlaceholder(t, post.Output.Content, "STRIPE_KEY_")

	// Rehydrating it from session B must NOT resolve — B has its own map.
	pre, err := c.HookPreTool(ctx, daemon.HookPreToolParams{
		SessionID: sidB,
		Tool:      "Bash",
		Input:     map[string]any{"command": "echo " + placeholder},
	})
	if err != nil {
		t.Fatalf("HookPreTool B: %v", err)
	}
	cmd, _ := pre.Input["command"].(string)
	if strings.Contains(cmd, testutil.StripeSecretKey) {
		t.Fatalf("cross-session leak: session B rehydrated A's placeholder (%q)", cmd)
	}
}

func TestDaemon_UnknownMethod(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	c := daemon.NewClient(sock)
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.Call(ctx, "no.such.method", map[string]any{}, nil)
	if err == nil {
		t.Fatalf("expected error for unknown method")
	}
	eo, ok := err.(*daemon.ErrorObject)
	if !ok {
		t.Fatalf("expected *daemon.ErrorObject; got %T (%v)", err, err)
	}
	if eo.Code != daemon.ErrCodeUnknownMethod {
		t.Errorf("error code = %d; want %d", eo.Code, daemon.ErrCodeUnknownMethod)
	}
}

// TestDaemon_100ConcurrentPostTool drives 100 parallel goroutines, each
// running one PostToolUse round-trip. The exit criterion for §8.6
// names this exact shape: "100 concurrent hook.post_tool requests,
// correct responses, no panics".
func TestDaemon_100ConcurrentPostTool(t *testing.T) {
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const N = 100
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := daemon.NewClient(sock)
			defer c.Close()
			// Each goroutine owns its session so we exercise both the
			// session registry's getOrCreate path under contention and
			// independent redaction state.
			sid := fmt.Sprintf("sess-%03d", i)
			if _, err := c.SessionStart(ctx, daemon.SessionStartParams{SessionID: sid}); err != nil {
				errs <- fmt.Errorf("session start %d: %w", i, err)
				return
			}
			r, err := c.HookPostTool(ctx, daemon.HookPostToolParams{
				SessionID: sid,
				Tool:      "Read",
				Output:    daemon.HookPostToolOutput{Content: testutil.StripeSecretKey},
			})
			if err != nil {
				errs <- fmt.Errorf("post-tool %d: %w", i, err)
				return
			}
			if !r.Modified {
				errs <- fmt.Errorf("post-tool %d: not modified", i)
				return
			}
			if strings.Contains(r.Output.Content, testutil.StripeSecretKey) {
				errs <- fmt.Errorf("post-tool %d: literal leaked", i)
				return
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// TestDaemon_Stress runs 10k requests over a small pool of long-lived
// connections and checks p99 latency. The IMPLEMENTATION.md §8.6
// budget is "< 10 ms p99"; we record-and-report rather than fail to
// keep CI portable (CI runners vary in throughput), and surface a
// soft-fail when the budget is missed by > 5× so regressions still
// page.
func TestDaemon_Stress(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in -short")
	}
	t.Parallel()
	sock, stop := startTestDaemon(t)
	defer stop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const (
		workers = 8
		total   = 10_000
	)
	perWorker := total / workers
	samples := make([]time.Duration, total)
	var idx int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			c := daemon.NewClient(sock)
			defer c.Close()
			sid := fmt.Sprintf("stress-%d", w)
			if _, err := c.SessionStart(ctx, daemon.SessionStartParams{SessionID: sid}); err != nil {
				t.Errorf("session start: %v", err)
				return
			}
			for i := 0; i < perWorker; i++ {
				start := time.Now()
				_, err := c.HookPostTool(ctx, daemon.HookPostToolParams{
					SessionID: sid,
					Tool:      "Read",
					Output:    daemon.HookPostToolOutput{Content: testutil.StripeSecretKey},
				})
				dur := time.Since(start)
				if err != nil {
					t.Errorf("worker %d iter %d: %v", w, i, err)
					return
				}
				// Lock-free slot reservation via atomic counter
				// avoids a per-call mutex on the latency record.
				slot := atomic.AddInt64(&idx, 1) - 1
				if slot < int64(len(samples)) {
					samples[slot] = dur
				}
			}
		}(w)
	}
	wg.Wait()

	n := int(atomic.LoadInt64(&idx))
	if n > len(samples) {
		n = len(samples)
	}
	samples = samples[:n]
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p50 := samples[n*50/100]
	p99 := samples[n*99/100]
	t.Logf("stress %d reqs: p50=%s p99=%s max=%s", n, p50, p99, samples[n-1])
	// Soft budget: p99 < 50 ms (5× the spec budget) to keep CI green
	// on slow runners while still catching catastrophic regressions.
	if p99 > 50*time.Millisecond {
		t.Errorf("p99 latency %s exceeds soft budget 50ms", p99)
	}
}

// TestDaemon_AutoLaunch verifies the launcher path: with no socket
// present, EnsureRunning forks the lockie binary built for this
// test, waits for the socket, and returns a working client.
func TestDaemon_AutoLaunch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping auto-launch test in -short (forks subprocess)")
	}
	t.Parallel()
	bin := buildLockieBinary(t)
	dir, err := os.MkdirTemp(shortTmp(t), "lk-")
	if err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "d.sock")

	opts := daemon.DefaultLaunchOptions(socketPath)
	opts.Binary = bin
	opts.Args = []string{"daemon", "start", "--foreground", "--socket", socketPath}
	opts.WaitTimeout = 3 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := daemon.EnsureRunning(ctx, opts)
	if err != nil {
		t.Fatalf("EnsureRunning: %v", err)
	}
	defer client.Close()
	health, err := client.Health(ctx)
	if err != nil {
		t.Fatalf("Health after auto-launch: %v", err)
	}
	if health.PID <= 0 {
		t.Errorf("auto-launched daemon reported pid %d", health.PID)
	}
	// Best-effort shutdown so a leaked daemon doesn't hold the socket
	// past the test's lifetime.
	t.Cleanup(func() {
		p, perr := os.FindProcess(health.PID)
		if perr == nil {
			_ = p.Kill()
		}
	})
}

// ---- helpers ---------------------------------------------------------------

// shortTmp returns a tmp directory short enough to host a Unix-domain
// socket path on macOS. On darwin `os.TempDir()` resolves to a long
// /var/folders path; we re-root under /tmp instead, which is the
// canonical fallback every macOS install ships with.
func shortTmp(t *testing.T) string {
	t.Helper()
	const fallback = "/tmp"
	if info, err := os.Stat(fallback); err == nil && info.IsDir() {
		return fallback
	}
	return os.TempDir()
}

func openSession(ctx context.Context, t *testing.T, c *daemon.Client) string {
	t.Helper()
	r, err := c.SessionStart(ctx, daemon.SessionStartParams{})
	if err != nil {
		t.Fatalf("SessionStart: %v", err)
	}
	return r.SessionID
}

// extractPlaceholder finds the first <prefix>NNN in s and returns it.
func extractPlaceholder(t *testing.T, s, prefix string) string {
	t.Helper()
	i := strings.Index(s, prefix)
	if i < 0 {
		t.Fatalf("placeholder %q not found in %q", prefix, s)
	}
	end := i + len(prefix)
	for end < len(s) && (s[end] >= '0' && s[end] <= '9') {
		end++
	}
	if end == i+len(prefix) {
		t.Fatalf("placeholder %q has no counter in %q", prefix, s)
	}
	return s[i:end]
}

// buildLockieBinary compiles the lockie binary into the test's tmp dir
// and returns the absolute path. Reused by every test that needs to
// fork a real daemon process.
func buildLockieBinary(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(shortTmp(t), "lk-bin-")
	if err != nil {
		t.Fatalf("mkdir tmp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	out := filepath.Join(dir, "lockie")
	build := exec.Command("go", "build", "-o", out, "github.com/ujjalsharma100/lockie/cmd/lockie")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("go build lockie: %v", err)
	}
	return out
}
