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

func (s *server) login(w http.ResponseWriter, r *http.Request) *apierr.Error {
	const op = "login"
	if retry, ok := s.o.Limiter.Allow(); !ok {
		secs := max(1, int(math.Ceil(retry.Seconds())))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		return &apierr.Error{Status: http.StatusTooManyRequests, Op: op, Kind: apierr.KindTooManyAttempts,
			Message: fmt.Sprintf("too many failed logins: retry in %d s", secs)}
	}
	var req loginRequest
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		return e
	}
	if req.Username == "" || req.Password == "" {
		return apierr.BadRequest(op, "", "username, password: expected non-empty strings, got an empty value")
	}
	// Both checks always run (no early exit on a wrong username).
	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(s.o.Username)) == 1
	passOK, err := auth.VerifyPassword(s.o.PasswordHash, req.Password)
	if err != nil {
		return apierr.Internal(op, "", err.Error())
	}
	if !userOK || !passOK {
		s.o.Limiter.Fail()
		return apierr.Unauthorized(op, "wrong username or password")
	}
	s.o.Limiter.Succeed()
	token, err := s.o.Sessions.Create()
	if err != nil {
		return apierr.Internal(op, "", err.Error())
	}
	http.SetCookie(w, auth.SessionCookie(token, s.o.SessionTTL))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) *apierr.Error {
	c, err := r.Cookie(auth.CookieName)
	if err != nil { // unreachable behind requireSession; kept as a loud failure, not a silent pass
		return apierr.Unauthorized("logout", err.Error())
	}
	if err := s.o.Sessions.Delete(c.Value); err != nil {
		return apierr.Internal("logout", "", err.Error())
	}
	http.SetCookie(w, auth.ClearCookie())
	w.WriteHeader(http.StatusNoContent)
	return nil
}
