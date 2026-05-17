// Integration tests for lockie add / list / show / forget (§8.8).
package integration_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ujjalsharma100/lockie/internal/audit"
	"github.com/ujjalsharma100/lockie/internal/cli"
	"github.com/ujjalsharma100/lockie/internal/daemon"
	"github.com/ujjalsharma100/lockie/internal/store/disk"
)

func TestAliasCommands_AddListForget(t *testing.T) {
	aliasesPath := filepath.Join(t.TempDir(), "aliases.json")

	sock, stop := startTestDaemonWithDisk(t, aliasesPath)
	defer stop()

	runCLI(t, sock, []string{"add", "MYKEY", "xyz"})
	out := runCLI(t, sock, []string{"list"})
	if !strings.Contains(out, "MYKEY") {
		t.Fatalf("list missing MYKEY:\n%s", out)
	}
	show := runCLI(t, sock, []string{"show", "MYKEY"})
	if !strings.Contains(show, "value_id:") || strings.Contains(show, "xyz") {
		t.Fatalf("show output unexpected (must not contain literal):\n%s", show)
	}
	runCLI(t, sock, []string{"forget", "MYKEY"})
	out2 := runCLI(t, sock, []string{"list"})
	if strings.Contains(out2, "MYKEY") {
		t.Fatalf("list still has MYKEY after forget:\n%s", out2)
	}

	st, err := disk.Open(aliasesPath)
	if err != nil {
		t.Fatalf("reopen disk: %v", err)
	}
	defer st.Close()
	list, err := st.ListAliases("")
	if err != nil {
		t.Fatalf("ListAliases: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("aliases.json still has %#v", list)
	}
}

func startTestDaemonWithDisk(t *testing.T, aliasesPath string) (socketPath string, stop func()) {
	t.Helper()
	dir := filepath.Dir(aliasesPath)
	sockDir, err := os.MkdirTemp("/tmp", "lk-alias-")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	socketPath = filepath.Join(sockDir, "d.sock")

	st, err := disk.Open(aliasesPath)
	if err != nil {
		t.Fatalf("disk.Open: %v", err)
	}
	h, err := daemon.NewHandlerWith(st, audit.Noop{})
	if err != nil {
		t.Fatalf("NewHandlerWith: %v", err)
	}
	srv := daemon.NewServer(socketPath, h)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = dir // aliases parent already created by disk.Open
	stop = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
		_ = st.Close()
	}
	return socketPath, stop
}

func runCLI(t *testing.T, socket string, args []string) string {
	t.Helper()
	full := append(append([]string{}, args...), "--socket", socket)
	root := cli.NewRoot()
	root.SetArgs(full)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("lockie %v: %v\noutput:\n%s", full, err, out.String())
	}
	return out.String()
}
