// Package auth holds the single-user login: argon2id hash, session file, rate limit, cookie.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Clock is injected wherever auth needs the time.
type Clock func() time.Time

// Cost is the argon2id parameter set written into a PHC string.
type Cost struct {
	Time      uint32
	MemoryKiB uint32
	Threads   uint8
}

// DefaultCost is RFC 9106's second recommended option (64 MiB, t=3, p=4).
var DefaultCost = Cost{Time: 3, MemoryKiB: 64 * 1024, Threads: 4}

const (
	argonKeyLen = 32
	saltLen     = 16
	phcParts    = 6 // "", "argon2id", "v=19", "m=..,t=..,p=..", salt, key
)

var phcB64 = base64.RawStdEncoding

func NewSalt() ([]byte, error) {
	s := make([]byte, saltLen)
	if _, err := rand.Read(s); err != nil {
		return nil, fmt.Errorf("new salt: %w", err)
	}
	return s, nil
}

// HashPassword returns $argon2id$v=19$m=<KiB>,t=<n>,p=<n>$<salt>$<key> (raw std base64).
func HashPassword(password string, salt []byte, c Cost) string {
	key := argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Threads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, c.MemoryKiB, c.Time, c.Threads,
		phcB64.EncodeToString(salt), phcB64.EncodeToString(key))
}

// VerifyPassword checks password against a PHC string. The error is only for a malformed hash.
func VerifyPassword(phc, password string) (bool, error) {
	parts := strings.Split(phc, "$")
	if len(parts) != phcParts || parts[0] != "" || parts[1] != "argon2id" {
		return false, fmt.Errorf("password hash: expected $argon2id$v=19$m=<n>,t=<n>,p=<n>$<salt>$<key>, got %d $-separated parts", len(parts))
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return false, fmt.Errorf("password hash version: expected v=%d, got %q", argon2.Version, parts[2])
	}
	var c Cost
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &c.MemoryKiB, &c.Time, &c.Threads); err != nil ||
		c.MemoryKiB == 0 || c.Time == 0 || c.Threads == 0 {
		return false, fmt.Errorf("password hash params: expected m=<n>,t=<n>,p=<n> all > 0, got %q", parts[3])
	}
	salt, err := phcB64.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return false, fmt.Errorf("password hash salt: expected non-empty raw base64, got error %v", err)
	}
	key, err := phcB64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return false, fmt.Errorf("password hash key: expected non-empty raw base64, got error %v", err)
	}
	got := argon2.IDKey([]byte(password), salt, c.Time, c.MemoryKiB, c.Threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}
