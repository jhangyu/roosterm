// Package swarm lists live Claude agent-team tmux servers (claude-swarm-* sockets).
// It probes each socket with a bare connect (no tmux protocol) and deletes the files of dead servers.
package swarm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const Prefix = "claude-swarm-"

// Dir is tmux's socket directory: ${TMUX_TMPDIR:-/tmp}/tmux-<uid> (man tmux).
func Dir(getenv func(string) string, uid int) string {
	base := getenv("TMUX_TMPDIR")
	if base == "" {
		base = "/tmp"
	}
	return filepath.Join(base, fmt.Sprintf("tmux-%d", uid))
}

// List returns the names of live Unix sockets in dir that start with Prefix, sorted.
func List(ctx context.Context, dir string) ([]string, error) {
	return liveSockets(ctx, dir, func(name string) bool { return strings.HasPrefix(name, Prefix) })
}

// liveSockets returns the names of live Unix sockets in dir that satisfy keep, sorted.
// A socket whose connect is refused belongs to a dead tmux server: its file is removed and not listed.
// A missing dir means no tmux server has run: empty list, no error.
func liveSockets(ctx context.Context, dir string, keep func(name string) bool) ([]string, error) {
	entries, err := os.ReadDir(dir) // sorted by name
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("swarm list %s: %w", dir, err)
	}
	names := []string{}
	for _, e := range entries {
		if keep(e.Name()) && e.Type()&fs.ModeSocket != 0 {
			live, err := probe(ctx, filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			if live {
				names = append(names, e.Name())
			}
		}
	}
	return names, nil
}

// probe reports whether a server accepts connections on the socket at path.
// ECONNREFUSED means dead: the stale file is removed. Anything else unexpected is an error.
func probe(ctx context.Context, path string) (bool, error) {
	c, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err == nil {
		c.Close()
		return true, nil
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return false, fmt.Errorf("swarm probe %s: expected connect ok or ECONNREFUSED, got %w", path, err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("swarm remove dead socket %s: expected removed or already gone, got %w", path, err)
	}
	return false, nil
}

// claudeProgram is the pane_current_command of a Claude member pane.
const claudeProgram = "claude.exe"

// listPanesFormat makes `tmux list-panes -a` print one "session:window command" line per pane.
const listPanesFormat = "#{session_name}:#{window_index} #{pane_current_command}"

// memberTarget picks the "session:window" holding the most Claude member panes from
// `tmux list-panes -a -F listPanesFormat` output.
// Ties go to the first window listed. ok is false when no pane runs claudeProgram.
func memberTarget(listPanes string) (target string, ok bool) {
	counts := map[string]int{}
	order := []string{}
	for _, line := range strings.Split(listPanes, "\n") {
		window, command, found := strings.Cut(line, " ")
		if !found || command != claudeProgram {
			continue
		}
		if counts[window] == 0 {
			order = append(order, window)
		}
		counts[window]++
	}
	for _, w := range order {
		if !ok || counts[w] > counts[target] {
			target, ok = w, true
		}
	}
	return target, ok
}

// AttachArgv is the command that attaches to the swarm socket name, targeting the window with the member panes
// (a bare attach shows the session's active window, which may be an idle shell). No member pane: the bare attach.
func AttachArgv(ctx context.Context, dir, name string) ([]string, error) {
	sock := filepath.Join(dir, name)
	out, err := exec.CommandContext(ctx, "tmux", "-S", sock, "list-panes", "-a", "-F", listPanesFormat).Output()
	if err != nil {
		return nil, fmt.Errorf("swarm list-panes %s: expected tmux exit 0, got %w", name, err)
	}
	return attachArgv(sock, string(out)), nil
}

// attachArgv builds the attach command for socket path sock from `tmux list-panes` output.
func attachArgv(sock, listPanes string) []string {
	argv := []string{"tmux", "-S", sock}
	if target, ok := memberTarget(listPanes); ok {
		return append(append(argv, mouseOn(target)...), "a", "-t", target)
	}
	return append(argv, "a")
}

// mouseOn turns on mouse mode for target's session, followed by a command separator.
// Claude Code's transcript lives in tmux's scrollback, which only tmux can scroll: with mouse on, the wheel
// (and touch drags, sent as wheel events) enters copy mode and scrolls instead of reaching Claude as arrow keys.
// It is a session option, so it stays on for other clients of that session too.
func mouseOn(target string) []string {
	return []string{"set-option", "-t", target, "mouse", "on", ";"}
}

// IsName reports whether name is a bare swarm socket name (Prefix, no path separators).
func IsName(name string) bool {
	return strings.HasPrefix(name, Prefix) && filepath.Base(name) == name
}
