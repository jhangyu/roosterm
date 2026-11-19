package auth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roosterm/host/internal/auth"
)

// cheap keeps argon2 fast under -race; VerifyPassword reads params from the hash.
var cheap = auth.Cost{Time: 1, MemoryKiB: 64, Threads: 1}

var salt = []byte("0123456789abcdef")

func TestVerifyPassword(t *testing.T) {
	t.Parallel()
	h := auth.HashPasswordCost("pw", salt, cheap)
	if !strings.HasPrefix(h, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("hash prefix: got %s", h)
	}
	if ok, err := auth.VerifyPassword(h, "pw"); err != nil || !ok {
		t.Fatalf("right password: expected true, nil; got %v, %v", ok, err)
	}
	if ok, err := auth.VerifyPassword(h, "nope"); err != nil || ok {
		t.Fatalf("wrong password: expected false, nil; got %v, %v", ok, err)
	}
	if auth.DefaultCost != (auth.Cost{Time: 3, MemoryKiB: 65536, Threads: 4}) {
		t.Fatalf("DefaultCost: got %+v", auth.DefaultCost)
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"":                                      "password hash:",
		"$bcrypt$v=19$m=64,t=1,p=1$AAAA$AAAA":   "password hash:",
		"$argon2id$v=18$m=64,t=1,p=1$AAAA$AAAA": "version",
		"$argon2id$v=19$m=0,t=1,p=1$AAAA$AAAA":  "params",
		"$argon2id$v=19$m=64,t=1,p=1$!!!!$AAAA": "salt",
		"$argon2id$v=19$m=64,t=1,p=1$AAAA$":     "key",
	}
	for phc, want := range cases {
		_, err := auth.VerifyPassword(phc, "pw")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("VerifyPassword(%q): expected error containing %q, got %v", phc, want, err)
		}
	}
}

func TestSessionCookieAttributes(t *testing.T) {
	t.Parallel()
	c := auth.SessionCookie("tok", 2*time.Hour).String()
	for _, want := range []string{"roosterm_session=tok", "Path=/", "Max-Age=7200", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(c, want) {
			t.Errorf("session cookie %q: missing %q", c, want)
		}
	}
	clear := auth.ClearCookie().String()
	for _, want := range []string{"roosterm_session=", "Max-Age=0", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(clear, want) {
			t.Errorf("clear cookie %q: missing %q", clear, want)
		}
	}
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newClock() *fakeClock { return &fakeClock{t: time.Unix(1_800_000_000, 0)} }

func TestSessionExpiry(t *testing.T) {
	t.Parallel()
	clk := newClock()
	s, err := auth.OpenSessions(filepath.Join(t.TempDir(), "sessions.json"), time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 43 {
		t.Fatalf("token length: expected 43, got %d", len(tok))
	}
	if !s.Valid(tok) {
		t.Fatal("fresh token: expected valid")
	}
	if s.Valid("garbage") {
		t.Fatal("unknown token: expected invalid")
	}
	clk.t = clk.t.Add(time.Hour)
	if s.Valid(tok) {
		t.Fatal("token at expiry: expected invalid")
	}
}

func TestSessionsSurviveReopen(t *testing.T) {
	t.Parallel()
	clk := newClock()
	path := filepath.Join(t.TempDir(), "sessions.json")
	s, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := s.Create()
	if err != nil {
		t.Fatal(err)
	}
	s2, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.Valid(tok) {
		t.Fatal("after reopen: expected valid")
	}
	if err := s2.Delete(tok); err != nil {
		t.Fatal(err)
	}
	if err := s2.Delete("unknown"); err != nil {
		t.Fatalf("delete unknown: expected nil, got %v", err)
	}
	s3, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	if s3.Valid(tok) {
		t.Fatal("after delete and reopen: expected invalid")
	}
}

func TestSessionsFileMode(t *testing.T) {
	t.Parallel()
	clk := newClock()
	path := filepath.Join(t.TempDir(), "sessions.json")
	s, err := auth.OpenSessions(path, time.Hour, clk.now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("sessions mode: expected 0600, got %#o", info.Mode().Perm())
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.OpenSessions(path, time.Hour, clk.now); err == nil || !strings.Contains(err.Error(), "expected mode 0600") {
		t.Fatalf("0644 file: expected mode error, got %v", err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.OpenSessions(path, time.Hour, clk.now); err == nil {
		t.Fatal("broken JSON: expected error, got nil")
	}
}

func TestLimiterLockout(t *testing.T) {
	t.Parallel()
	clk := newClock()
	l := auth.NewLimiter(clk.now)
	for i := 0; i < auth.MaxFailures-1; i++ {
		l.Fail()
	}
	if _, ok := l.Allow(); !ok {
		t.Fatal("after 4 failures: expected allowed")
	}
	l.Fail()
	if d, ok := l.Allow(); ok || d != auth.Lockout {
		t.Fatalf("after 5 failures: expected (60s, false), got (%v, %v)", d, ok)
	}
	clk.t = clk.t.Add(30 * time.Second)
	if d, ok := l.Allow(); ok || d != 30*time.Second {
		t.Fatalf("30s into lockout: expected (30s, false), got (%v, %v)", d, ok)
	}
	clk.t = clk.t.Add(30 * time.Second)
	if _, ok := l.Allow(); !ok {
		t.Fatal("after lockout: expected allowed")
	}
	l.Fail()
	if _, ok := l.Allow(); !ok {
		t.Fatal("one failure after lockout: expected allowed (count reset)")
	}
	for i := 0; i < auth.MaxFailures-2; i++ {
		l.Fail()
	}
	l.Succeed()
	l.Fail()
	if _, ok := l.Allow(); !ok {
		t.Fatal("after success: expected count reset")
	}
}
