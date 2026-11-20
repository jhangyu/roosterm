package main

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"roosterm/host/internal/auth"
	"roosterm/host/internal/config"
)

// cheapCost keeps the argon2 hash far under the 1 s test budget; the real command uses auth.DefaultCost.
var cheapCost = auth.Cost{Time: 1, MemoryKiB: 64, Threads: 1}

// runPasswd feeds input as piped (non-terminal) stdin to setPassword for a new config file.
func runPasswd(t *testing.T, input string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	return path, setPassword(path, r, io.Discard, cheapCost)
}

func TestSetPasswordWritesVerifiableHash(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	path, err := runPasswd(t, "u\npw\npw\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Username != "u" {
		t.Fatalf("username: expected %q, got %q", "u", cfg.Username)
	}
	ok, err := auth.VerifyPassword(cfg.PasswordHash, "pw")
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(written hash, %q): expected true, got %v, %v", "pw", ok, err)
	}
}

func TestSetPasswordRejectsMismatchedRepeat(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	path, err := runPasswd(t, "u\npw\nother\n")
	if err == nil || !strings.Contains(err.Error(), "repeat password") {
		t.Fatalf("expected a repeat password error, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("config %s: expected no file written, got stat error %v", path, err)
	}
}

func TestSetPasswordSamePasswordGivesDifferentHash(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	var hashes [2]string
	for i := range hashes {
		path, err := runPasswd(t, "u\npw\npw\n")
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		hashes[i] = cfg.PasswordHash
	}
	if hashes[0] == hashes[1] {
		t.Fatalf("two passwd runs with the same password: expected different hashes, got %q twice", hashes[0])
	}
}
