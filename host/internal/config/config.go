// Package config reads and writes ~/.config/roosterm/config.toml.
package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"

	"roosterm/host/internal/atomicfile"
)

const (
	DefaultListen       = "127.0.0.1:8787"
	DefaultPingInterval = 25 * time.Second
	DefaultSessionTTL   = 720 * time.Hour
)

const (
	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
	execBits os.FileMode = 0o111
)

type Config struct {
	Listen       string
	HCSocket     string // raw value, may be ""
	PingInterval time.Duration
	SessionTTL   time.Duration
	Shell        string
	Username     string
	PasswordHash string
}

type fileConfig struct {
	Listen       string `toml:"listen"`
	HCSocket     string `toml:"hc_socket"`
	PingInterval string `toml:"ping_interval"`
	SessionTTL   string `toml:"session_ttl"`
	Shell        string `toml:"shell"`
	Username     string `toml:"username"`
	PasswordHash string `toml:"password_hash"`
}

// DefaultPath is $XDG_CONFIG_HOME/roosterm/config.toml, else $HOME/.config/roosterm/config.toml.
func DefaultPath() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "roosterm", "config.toml"), nil
	}
	home := os.Getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("config path: expected HOME or XDG_CONFIG_HOME to be set, got both empty")
	}
	return filepath.Join(home, ".config", "roosterm", "config.toml"), nil
}

// SessionsPath is sessions.json next to the config file.
func SessionsPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "sessions.json")
}

func Load(path string) (Config, error) {
	var fc fileConfig
	md, err := toml.DecodeFile(path, &fc)
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	if keys := md.Undecoded(); len(keys) > 0 {
		return Config{}, fmt.Errorf("config %s: unknown keys %v: expected only listen, hc_socket, ping_interval, session_ttl, shell, username, password_hash", path, keys)
	}
	for _, f := range []struct{ key, val string }{
		{"listen", fc.Listen}, {"ping_interval", fc.PingInterval}, {"session_ttl", fc.SessionTTL},
		{"shell", fc.Shell}, {"username", fc.Username}, {"password_hash", fc.PasswordHash},
	} {
		if f.val == "" {
			return Config{}, fmt.Errorf("config %s: %s: expected a non-empty value, got \"\"", path, f.key)
		}
	}
	ping, err := positiveDuration(path, "ping_interval", fc.PingInterval)
	if err != nil {
		return Config{}, err
	}
	ttl, err := positiveDuration(path, "session_ttl", fc.SessionTTL)
	if err != nil {
		return Config{}, err
	}
	if err := checkShell(path, fc.Shell); err != nil {
		return Config{}, err
	}
	return Config{Listen: fc.Listen, HCSocket: fc.HCSocket, PingInterval: ping, SessionTTL: ttl,
		Shell: fc.Shell, Username: fc.Username, PasswordHash: fc.PasswordHash}, nil
}

func positiveDuration(path, key, v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("config %s: %s: expected a positive duration like \"25s\", got %q", path, key, v)
	}
	return d, nil
}

func checkShell(path, shell string) error {
	if !filepath.IsAbs(shell) {
		return fmt.Errorf("config %s: shell: expected an absolute path, got %q", path, shell)
	}
	info, err := os.Stat(shell)
	if err != nil {
		return fmt.Errorf("config %s: shell: expected an executable file, got %v", path, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&execBits == 0 {
		return fmt.Errorf("config %s: shell: expected an executable regular file, got mode %v", path, info.Mode())
	}
	return nil
}

// Save writes c atomically (tmp file 0600 + rename), creating the directory 0700.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("config save %s: %w", path, err)
	}
	var buf bytes.Buffer
	err := toml.NewEncoder(&buf).Encode(fileConfig{
		Listen: c.Listen, HCSocket: c.HCSocket, PingInterval: c.PingInterval.String(),
		SessionTTL: c.SessionTTL.String(), Shell: c.Shell, Username: c.Username, PasswordHash: c.PasswordHash,
	})
	if err != nil {
		return fmt.Errorf("config save %s: encode: %w", path, err)
	}
	if err := atomicfile.Write(path, buf.Bytes(), fileMode); err != nil {
		return fmt.Errorf("config save %s: %w", path, err)
	}
	return nil
}

// SocketPath is hc_socket if set, else hc's own default rule (docs/hc-bridge.md §1) under home.
func (c Config) SocketPath(home string) string {
	if c.HCSocket != "" {
		return c.HCSocket
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "herdr-core", "default.sock")
	}
	run := fmt.Sprintf("/run/user/%d", os.Getuid())
	if info, err := os.Stat(run); err == nil && info.IsDir() {
		return filepath.Join(run, "herdr-core", "default.sock")
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "herdr-core", "default", "default.sock")
}
