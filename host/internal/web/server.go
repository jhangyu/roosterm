// Package web is roosterm's HTTP shell: login/logout, the session gate, and the embedded static page.
package web

import (
	"net/http"
	"time"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/auth"
)

type Options struct {
	Username     string
	PasswordHash string
	SessionTTL   time.Duration
	Sessions     *auth.Sessions
	Limiter      *auth.Limiter
	API          http.Handler // mounted at /api/
	TabSocket    http.Handler // mounted at GET /ws/tabs/{id}
}

type server struct{ o Options }

func New(o Options) http.Handler {
	s := &server{o: o}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", s.login)
	mux.Handle("POST /logout", s.requireSession(http.HandlerFunc(s.logout)))
	mux.Handle("/api/", s.requireSession(o.API))
	mux.Handle("GET /ws/tabs/{id}", s.requireSession(o.TabSocket))
	// "/" (all methods) rather than "GET /": "GET /" conflicts with "/api/" in Go 1.22+ routing.
	mux.Handle("/", staticHandler())
	return mux
}

// requireSession runs before any WebSocket upgrade, so a handshake without a cookie gets HTTP 401.
func (s *server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(auth.CookieName) // only error: http.ErrNoCookie
		if err != nil || !s.o.Sessions.Valid(c.Value) {
			apierr.Write(w, &apierr.Error{Status: http.StatusUnauthorized, Op: "session",
				Kind: apierr.KindUnauthorized, Message: "missing or expired session cookie"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
