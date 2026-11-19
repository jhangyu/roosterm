package web

import (
	"crypto/subtle"
	"fmt"
	"math"
	"net/http"
	"strconv"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/auth"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	const op = "login"
	if retry, ok := s.o.Limiter.Allow(); !ok {
		secs := max(1, int(math.Ceil(retry.Seconds())))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		apierr.Write(w, &apierr.Error{Status: http.StatusTooManyRequests, Op: op, Kind: apierr.KindTooManyAttempts,
			Message: fmt.Sprintf("too many failed logins: retry in %d s", secs)})
		return
	}
	var req loginRequest
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		apierr.Write(w, e)
		return
	}
	if req.Username == "" || req.Password == "" {
		apierr.Write(w, &apierr.Error{Status: http.StatusBadRequest, Op: op, Kind: apierr.KindBadRequest,
			Message: "username, password: expected non-empty strings, got an empty value"})
		return
	}
	// Both checks always run (no early exit on a wrong username).
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.o.Username)) == 1
	passOK, err := auth.VerifyPassword(s.o.PasswordHash, req.Password)
	if err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: op, Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	if !userOK || !passOK {
		s.o.Limiter.Fail()
		apierr.Write(w, &apierr.Error{Status: http.StatusUnauthorized, Op: op, Kind: apierr.KindUnauthorized,
			Message: "wrong username or password"})
		return
	}
	s.o.Limiter.Succeed()
	token, err := s.o.Sessions.Create()
	if err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: op, Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	http.SetCookie(w, auth.SessionCookie(token, s.o.SessionTTL))
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(auth.CookieName)
	if err != nil { // unreachable behind requireSession; kept as a loud failure, not a silent pass
		apierr.Write(w, &apierr.Error{Status: http.StatusUnauthorized, Op: "logout", Kind: apierr.KindUnauthorized, Message: err.Error()})
		return
	}
	if err := s.o.Sessions.Delete(c.Value); err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: "logout", Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	http.SetCookie(w, auth.ClearCookie())
	w.WriteHeader(http.StatusNoContent)
}
