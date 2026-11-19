// Package swarm lists Claude agent-team tmux servers (claude-swarm-* sockets). It never connects to them.
package swarm

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// List returns the names of Unix sockets in dir that start with Prefix, sorted.
// A missing dir means no tmux server has run: empty list, no error.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir) // sorted by name
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("swarm list %s: %w", dir, err)
	}
	names := []string{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), Prefix) && e.Type()&fs.ModeSocket != 0 {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
