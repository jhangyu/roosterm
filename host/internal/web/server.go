// Package web is roosterm's HTTP shell: login/logout, the session gate, and the embedded static page.
package web

import (
	"fmt"
	"net/http"
	"time"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/auth"
)

type Options struct {
	Username      string
	PasswordHash  string
	SessionTTL    time.Duration
	Sessions      *auth.Sessions
	Limiter       *auth.Limiter // anonymous clients
	DeviceLimiter *auth.Limiter // browsers carrying the known-device cookie
	API           http.Handler  // mounted at /api/
	TabSocket     http.Handler  // mounted at GET /ws/tabs/{id}
}

type server struct {
	o      Options
	marker string
}

func New(o Options) http.Handler {
	s := &server{o: o, marker: auth.DeviceMarker(o.PasswordHash)}
	mux := http.NewServeMux()
	mux.Handle("POST /login", apierr.HandlerFunc(s.login))
	mux.Handle("POST /logout", s.requireSession(apierr.HandlerFunc(s.logout)))
	mux.Handle("/api/", s.requireSession(o.API))
	mux.Handle("GET /ws/tabs/{id}", s.requireSession(o.TabSocket))
	// "/" (all methods) rather than "GET /": "GET /" conflicts with "/api/" in Go 1.22+ routing.
	mux.Handle("/", staticHandler())
	return crossOriginGuard(mux)
}

// crossOriginGuard refuses state-changing requests a browser marks as not same-origin.
// SameSite=Strict is per site, so a sibling subdomain or another localhost port still carries the cookie.
// It uses Check, not Handler, to keep the JSON error body.
func crossOriginGuard(next http.Handler) http.Handler {
	cop := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := cop.Check(r); err != nil {
			apierr.Write(w, apierr.Forbidden("cross_origin", fmt.Sprintf(
				"%s %s refused: expected Sec-Fetch-Site same-origin or none, or Origin host equal to Host %q; got Sec-Fetch-Site %q, Origin %q: %v",
				r.Method, r.URL.Path, r.Host, r.Header.Get("Sec-Fetch-Site"), r.Header.Get("Origin"), err)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireSession runs before any WebSocket upgrade, so a handshake without a cookie gets HTTP 401.
func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.CookieName) // only error: http.ErrNoCookie
		if err != nil || !s.o.Sessions.Valid(c.Value) {
			apierr.Write(w, apierr.Unauthorized("session", "missing or expired session cookie"))
			return
		}
		next.ServeHTTP(w, r)
	})
}
