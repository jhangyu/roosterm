package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"
)

const CookieName = "roosterm_session"

const (
	DeviceCookieName = "roosterm_device"
	// DeviceCookieMaxAge is how long a browser keeps the known-device marker (400 days, the browser cap).
	DeviceCookieMaxAge = 400 * 24 * time.Hour
)

// DeviceMarker is the value of the known-device cookie. It is derived from the password hash, so
// only a past successful login (or read access to config.toml) yields it and changing the password invalidates it.
func DeviceMarker(passwordHash string) string {
	m := hmac.New(sha256.New, []byte(passwordHash))
	m.Write([]byte("roosterm login device marker"))
	return hex.EncodeToString(m.Sum(nil))
}

// DeviceCookie marks a browser that logged in before; it only selects the login rate bucket and grants no access.
func DeviceCookie(marker string) *http.Cookie {
	return &http.Cookie{Name: DeviceCookieName, Value: marker, Path: "/login", MaxAge: int(DeviceCookieMaxAge.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}

func SessionCookie(token string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: token, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}

// ClearCookie expires the session cookie (MaxAge -1 is written as Max-Age=0).
func ClearCookie() *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}
