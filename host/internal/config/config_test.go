package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roosterm/host/internal/config"
)

// valid has every required key; each case below breaks one thing.
var valid = map[string]string{
	"listen": "127.0.0.1:8787", "ping_interval": "25s", "session_ttl": "720h",
	"shell": "/bin/sh", "username": "u", "password_hash": "h",
}

func writeConfig(t *testing.T, kv map[string]string) string {
	t.Helper()
	var b strings.Builder
	for k, v := range kv {
		b.WriteString(k + " = \"" + v + "\"\n")
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func with(key, val string) map[string]string {
	kv := map[string]string{}
	for k, v := range valid {
		kv[k] = v
	}
	kv[key] = val
	return kv
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, wantErr string
		kv            map[string]string
	}{
		{"unknown key", "unknown keys [colour]", with("colour", "red")},
		{"zero ping_interval", "ping_interval: expected a positive duration", with("ping_interval", "0s")},
		{"bad session_ttl", "session_ttl: expected a positive duration", with("session_ttl", "soon")},
		{"relative shell", "shell: expected an absolute path", with("shell", "bin/sh")},
	}
	for _, key := range []string{"listen", "ping_interval", "session_ttl", "shell", "username", "password_hash"} {
		cases = append(cases, struct {
			name, wantErr string
			kv            map[string]string
		}{"empty " + key, key + ": expected a non-empty value", with(key, "")})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load(writeConfig(t, tc.kv))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Load: expected an error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	want := config.Config{Listen: "127.0.0.1:9000", HCSocket: "/tmp/hc.sock", PingInterval: 10 * time.Second,
		SessionTTL: 2 * time.Hour, Shell: "/bin/sh", Username: "u", PasswordHash: "h"}
	path := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := config.Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip: expected %+v, got %+v", want, got)
	}
}
