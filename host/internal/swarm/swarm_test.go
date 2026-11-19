package swarm_test

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"roosterm/host/internal/swarm"
)

func TestListOnlyClaudeSwarmSockets(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	for _, name := range []string{"claude-swarm-9", "default", "claude-swarm-10"} {
		ln, err := net.Listen("unix", filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
	}
	if err := os.WriteFile(filepath.Join(dir, "claude-swarm-file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := swarm.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude-swarm-10", "claude-swarm-9"}
	if !slices.Equal(got, want) {
		t.Fatalf("List: expected %v, got %v", want, got)
	}
}

func TestListRemovesDeadSockets(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	live, err := net.Listen("unix", filepath.Join(dir, "claude-swarm-1"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { live.Close() })
	dead, err := net.Listen("unix", filepath.Join(dir, "claude-swarm-2"))
	if err != nil {
		t.Fatal(err)
	}
	dead.(*net.UnixListener).SetUnlinkOnClose(false)
	dead.Close()
	plain := filepath.Join(dir, "claude-swarm-file")
	if err := os.WriteFile(plain, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := swarm.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"claude-swarm-1"}; !slices.Equal(got, want) {
		t.Fatalf("List: expected %v, got %v", want, got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "claude-swarm-2")); !os.IsNotExist(err) {
		t.Fatalf("dead socket file: expected removed, Lstat err %v", err)
	}
	if _, err := os.Stat(plain); err != nil {
		t.Fatalf("non-socket file: expected kept, got %v", err)
	}
}

func TestListMissingDir(t *testing.T) {
	t.Parallel()
	got, err := swarm.List(filepath.Join(t.TempDir(), "absent"))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("List missing dir: expected [], nil; got %#v, %v", got, err)
	}
}

func TestDir(t *testing.T) {
	t.Parallel()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if got := swarm.Dir(getenv, 501); got != "/tmp/tmux-501" {
		t.Fatalf("Dir without TMUX_TMPDIR: got %s", got)
	}
	env["TMUX_TMPDIR"] = "/var/tmux"
	if got := swarm.Dir(getenv, 501); got != "/var/tmux/tmux-501" {
		t.Fatalf("Dir with TMUX_TMPDIR: got %s", got)
	}
}
