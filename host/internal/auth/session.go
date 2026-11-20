package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"roosterm/host/internal/atomicfile"
)

const (
	tokenLen                    = 32
	sessionFileMode fs.FileMode = 0o600
)

// Sessions stores sha256(token) → expiry in a JSON file. Safe for concurrent use.
type Sessions struct {
	mu      sync.Mutex
	path    string
	ttl     time.Duration
	now     Clock
	entries map[string]int64 // sha256 hex → expires unix ms
}

type sessionFile struct {
	Sessions []sessionEntry `json:"sessions"`
}

type sessionEntry struct {
	Hash      string `json:"hash"`
	ExpiresMS int64  `json:"expires_ms"`
}

// OpenSessions loads path (missing file = empty store). A file readable by others or with broken JSON is an error.
func OpenSessions(path string, ttl time.Duration, now Clock) (*Sessions, error) {
	s := &Sessions{path: path, ttl: ttl, now: now, entries: map[string]int64{}}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sessions %s: %w", path, err)
	}
	if perm := info.Mode().Perm(); perm&^sessionFileMode != 0 {
		return nil, fmt.Errorf("sessions %s: expected mode 0600, got %#o", path, perm)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("sessions %s: %w", path, err)
	}
	var f sessionFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("sessions %s: expected {\"sessions\":[...]} JSON, got %w", path, err)
	}
	nowMS := now().UnixMilli()
	for _, e := range f.Sessions {
		if e.ExpiresMS > nowMS {
			s.entries[e.Hash] = e.ExpiresMS
		}
	}
	return s, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create returns a new token (43 chars base64url) and persists its hash.
func (s *Sessions) Create() (string, error) {
	raw := make([]byte, tokenLen)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	h := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	nowMS := now.UnixMilli()
	for k, exp := range s.entries {
		if exp <= nowMS {
			delete(s.entries, k)
		}
	}
	s.entries[h] = now.Add(s.ttl).UnixMilli()
	if err := s.saveLocked(); err != nil {
		delete(s.entries, h)
		return "", err
	}
	return token, nil
}

func (s *Sessions) Valid(token string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.entries[hashToken(token)]
	return ok && s.now().UnixMilli() < exp
}

// Delete removes token; an unknown token is not an error.
func (s *Sessions) Delete(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := hashToken(token)
	if _, ok := s.entries[h]; !ok {
		return nil
	}
	delete(s.entries, h)
	return s.saveLocked()
}

func (s *Sessions) saveLocked() error {
	f := sessionFile{Sessions: []sessionEntry{}}
	for h, exp := range s.entries {
		f.Sessions = append(f.Sessions, sessionEntry{Hash: h, ExpiresMS: exp})
	}
	slices.SortFunc(f.Sessions, func(a, b sessionEntry) int { return strings.Compare(a.Hash, b.Hash) })
	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("sessions %s: encode: %w", s.path, err)
	}
	if err := atomicfile.Write(s.path, b, sessionFileMode); err != nil {
		return fmt.Errorf("sessions %s: %w", s.path, err)
	}
	return nil
}
