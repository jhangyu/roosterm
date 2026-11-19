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
