package api

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"roosterm/host/internal/apierr"
)

// Directory error kinds; escape and permission denial are both 403 but stay distinguishable.
const (
	kindOutsideHome      = "outside_home"
	kindPermissionDenied = "permission_denied"
)

type DirEntry struct {
	Name   string `json:"name"`
	Hidden bool   `json:"hidden"`
}

type dirListing struct {
	Path    string     `json:"path"`
	Entries []DirEntry `json:"entries"`
}

// fsError maps a filesystem failure on path to the API error table.
func fsError(op, rel string, err error) *apierr.Error {
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
		return apierr.NotFound(op, "", fmt.Sprintf("path %q: expected an existing directory under home, got %v", rel, err))
	case errors.Is(err, fs.ErrPermission):
		return &apierr.Error{Status: http.StatusForbidden, Op: op, Kind: kindPermissionDenied,
			Message: fmt.Sprintf("path %q: expected a readable directory, got %v", rel, err)}
	}
	return apierr.Internal(op, "", fmt.Sprintf("path %q: %v", rel, err))
}

// homeDir is the symlink-resolved home every tab cwd and listing is confined to.
func (s *server) homeDir(op string) (string, *apierr.Error) {
	h, err := filepath.EvalSymlinks(s.o.Home)
	if err != nil {
		return "", apierr.Internal(op, "", fmt.Sprintf("home %q: expected a resolvable directory, got %v", s.o.Home, err))
	}
	return h, nil
}

func inside(home, p string) bool {
	return p == home || strings.HasPrefix(p, home+string(filepath.Separator))
}

// resolveDir is the single trust-boundary check for a client path relative to home:
// it returns the symlink-resolved absolute directory and home, or the API error to send.
func (s *server) resolveDir(op, rel string) (abs, home string, e *apierr.Error) {
	if strings.ContainsRune(rel, 0) {
		return "", "", apierr.BadRequest(op, "", fmt.Sprintf("path: expected no NUL byte, got %q", rel))
	}
	if filepath.IsAbs(rel) {
		return "", "", apierr.BadRequest(op, "", fmt.Sprintf("path: expected a path relative to home, got absolute %q", rel))
	}
	if slices.Contains(strings.Split(rel, "/"), "..") {
		return "", "", apierr.BadRequest(op, "", fmt.Sprintf("path: expected no \"..\" segment, got %q", rel))
	}
	home, e = s.homeDir(op)
	if e != nil {
		return "", "", e
	}
	abs, err := filepath.EvalSymlinks(filepath.Join(home, rel))
	if err != nil {
		return "", "", fsError(op, rel, err)
	}
	if !inside(home, abs) {
		return "", "", &apierr.Error{Status: http.StatusForbidden, Op: op, Kind: kindOutsideHome,
			Message: fmt.Sprintf("path %q: expected to resolve inside home %s, got %s", rel, home, abs)}
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return "", "", fsError(op, rel, err)
	}
	if !fi.IsDir() {
		return "", "", apierr.NotFound(op, "", fmt.Sprintf("path %q: expected a directory, got %s", rel, fi.Mode().Type()))
	}
	return abs, home, nil
}

// relCwd reports an hc cwd relative to home; a cwd outside home (not created by roosterm) stays absolute.
func relCwd(home, cwd string) string {
	if !inside(home, cwd) {
		return cwd
	}
	if r, _ := filepath.Rel(home, cwd); r != "." {
		return r
	}
	return ""
}

func (s *server) listDirs(w http.ResponseWriter, r *http.Request) *apierr.Error {
	const op = "dirs.list"
	rel := r.URL.Query().Get("path")
	abs, home, e := s.resolveDir(op, rel)
	if e != nil {
		return e
	}
	des, err := os.ReadDir(abs)
	if err != nil {
		return fsError(op, rel, err)
	}
	out := dirListing{Path: rel, Entries: []DirEntry{}}
	for _, de := range des { // os.ReadDir returns entries sorted by name
		isDir := de.IsDir()
		if de.Type()&fs.ModeSymlink != 0 {
			t, err := filepath.EvalSymlinks(filepath.Join(abs, de.Name()))
			if err == nil && inside(home, t) { // dangling or escaping links are not offered
				fi, err := os.Stat(t)
				isDir = err == nil && fi.IsDir()
			}
		}
		if isDir {
			out.Entries = append(out.Entries, DirEntry{Name: de.Name(), Hidden: strings.HasPrefix(de.Name(), ".")})
		}
	}
	apierr.WriteJSON(w, http.StatusOK, out)
	return nil
}
