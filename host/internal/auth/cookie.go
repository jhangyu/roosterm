package auth

import (
	"net/http"
	"time"
)

const CookieName = "roosterm_session"

func SessionCookie(token string, ttl time.Duration) *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: token, Path: "/", MaxAge: int(ttl.Seconds()),
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}

// ClearCookie expires the session cookie (MaxAge -1 is written as Max-Age=0).
func ClearCookie() *http.Cookie {
	return &http.Cookie{Name: CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
}
