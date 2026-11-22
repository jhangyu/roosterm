package swarm

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Pane is a tmux pane running Claude Code on a socket that is not a swarm socket.
type Pane struct {
	Socket  string `json:"socket"`
	Session string `json:"session"`
	Window  int    `json:"window"`
	ID      string `json:"id"` // tmux pane id, "%N"
	Cwd     string `json:"cwd"`
}

// SocketError is a socket whose `tmux list-panes` failed; it is reported, not dropped.
type SocketError struct {
	Socket  string `json:"socket"`
	Message string `json:"message"`
}

// panesFormat makes `tmux list-panes -a` print one "window pane command session:path" line per pane.
// No tabs: without a UTF-8 locale (e.g. under launchd) tmux prints control characters as "_".
// The session goes before ":" because tmux session names cannot contain ":"; the path (spaces, ":") goes last.
const panesFormat = "#{window_index} #{pane_id} #{pane_current_command} #{session_name}:#{pane_current_path}"

const paneFields = 4

var paneIDPattern = regexp.MustCompile(`^%[0-9]+$`)

// IsPaneID reports whether id is a tmux pane id ("%" and digits).
func IsPaneID(id string) bool { return paneIDPattern.MatchString(id) }

// IsSocketName reports whether name is a bare, non-swarm socket name usable inside the socket dir.
func IsSocketName(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.HasPrefix(name, Prefix)
}

// parsePanes keeps the claudeProgram panes of `tmux list-panes -a -F panesFormat` output for socket.
func parsePanes(socket, listPanes string) ([]Pane, error) {
	panes := []Pane{}
	for _, line := range strings.Split(listPanes, "\n") {
		if line == "" {
			continue
		}
		f := strings.SplitN(line, " ", paneFields)
		var session, cwd string
		found := len(f) == paneFields
		if found {
			session, cwd, found = strings.Cut(f[3], ":")
		}
		if !found {
			return nil, fmt.Errorf("claude panes %s: expected \"window pane command session:path\", got %q", socket, line)
		}
		if f[2] != claudeProgram {
			continue
		}
		window, err := strconv.Atoi(f[0])
		if err != nil || !IsPaneID(f[1]) {
			return nil, fmt.Errorf("claude panes %s: expected numeric window and %%N pane id, got %q", socket, line)
		}
		panes = append(panes, Pane{Socket: socket, Session: session, Window: window, ID: f[1], Cwd: cwd})
	}
	return panes, nil
}

// ClaudePanes lists the Claude Code panes on every live non-swarm socket in dir, sockets sorted by name.
// A socket whose list-panes fails (or prints unparseable lines) goes to the SocketError list; the others still list.
func ClaudePanes(ctx context.Context, dir string) ([]Pane, []SocketError, error) {
	socks, err := liveSockets(ctx, dir, IsSocketName)
	if err != nil {
		return nil, nil, err
	}
	panes, failed := []Pane{}, []SocketError{}
	for _, name := range socks {
		out, err := exec.CommandContext(ctx, "tmux", "-S", filepath.Join(dir, name), "list-panes", "-a", "-F", panesFormat).Output()
		if err != nil {
			failed = append(failed, SocketError{name, fmt.Sprintf("claude panes %s: expected tmux list-panes exit 0, got %v", name, err)})
			continue
		}
		ps, err := parsePanes(name, string(out))
		if err != nil {
			failed = append(failed, SocketError{name, err.Error()})
			continue
		}
		panes = append(panes, ps...)
	}
	return panes, failed, nil
}

// PaneAttachArgv is the command that shows pane id on socket name (targetAttachArgv with the pane as target).
func PaneAttachArgv(dir, name, id string) []string {
	return targetAttachArgv(filepath.Join(dir, name), id)
}
