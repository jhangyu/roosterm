package swarm

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestAttachArgv(t *testing.T) {
	t.Parallel()
	const sock = "/d/tmux-501/claude-swarm-7"
	for _, tc := range []struct {
		panes string
		want  []string
	}{
		{"s:0 zsh\ns:1 claude.exe\ns:15 zsh\ns:15 claude.exe\ns:15 claude.exe\n", []string{"tmux", "-S", sock, "set-option", "-t", "s:15", "mouse", "on", ";",
			"select-window", "-t", "s:15", ";", "select-pane", "-t", "s:15", ";", "attach-session", "-t", "s:15"}},
		{"s:0 zsh\n", []string{"tmux", "-S", sock, "set-option", "mouse", "on", ";", "select-window", ";", "select-pane", ";", "attach-session"}},
	} {
		if got := attachArgv(sock, tc.panes); !slices.Equal(got, tc.want) {
			t.Errorf("attachArgv(%q): expected %v, got %v", tc.panes, tc.want, got)
		}
	}
}

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
	got, err := List(t.Context(), dir)
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
	got, err := List(t.Context(), dir)
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
	got, err := List(t.Context(), filepath.Join(t.TempDir(), "absent"))
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("List missing dir: expected [], nil; got %#v, %v", got, err)
	}
}

func TestDir(t *testing.T) {
	t.Parallel()
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if got := Dir(getenv, 501); got != "/tmp/tmux-501" {
		t.Fatalf("Dir without TMUX_TMPDIR: got %s", got)
	}
	env["TMUX_TMPDIR"] = "/var/tmux"
	if got := Dir(getenv, 501); got != "/var/tmux/tmux-501" {
		t.Fatalf("Dir with TMUX_TMPDIR: got %s", got)
	}
}

func TestParsePanes(t *testing.T) {
	t.Parallel()
	got, err := parsePanes("claude-relay", "0 %1 zsh s:/a\n2 %7 claude.exe s:/Users/me/my proj:x\n")
	want := []Pane{{Socket: "claude-relay", Session: "s", Window: 2, ID: "%7", Cwd: "/Users/me/my proj:x"}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("parsePanes: expected %v, got %v, %v", want, got, err)
	}
	if _, err := parsePanes("x", "s_0_%1_claude.exe_/a\n"); err == nil {
		t.Fatal("parsePanes malformed line: expected error")
	}
}

func TestSocketNameAndPaneID(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]bool{"claude-relay": true, "default": true, "": false, "..": false, "a/b": false, "claude-swarm-1": false} {
		if got := IsSocketName(name); got != want {
			t.Errorf("IsSocketName(%q): expected %v, got %v", name, want, got)
		}
	}
	for id, want := range map[string]bool{"%0": true, "%12": true, "%": false, "12": false, "%1;x": false, "%a": false} {
		if got := IsPaneID(id); got != want {
			t.Errorf("IsPaneID(%q): expected %v, got %v", id, want, got)
		}
	}
}

func TestPaneAttachArgv(t *testing.T) {
	t.Parallel()
	got := PaneAttachArgv("/d", "claude-relay", "%3")
	want := []string{"tmux", "-S", "/d/claude-relay", "set-option", "-t", "%3", "mouse", "on", ";", "select-window", "-t", "%3", ";", "select-pane", "-t", "%3", ";", "attach-session", "-t", "%3"}
	if !slices.Equal(got, want) {
		t.Fatalf("PaneAttachArgv: expected %v, got %v", want, got)
	}
}
