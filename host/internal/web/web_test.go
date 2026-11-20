package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"roosterm/host/internal/auth"
	"roosterm/host/internal/web"
)

const sessionTTL = time.Hour

type env struct {
	t        *testing.T
	h        http.Handler
	now      time.Time
	apiCalls int
	wsCalls  int
}

// newEnv wires web.New with a fixed clock and counting API/TabSocket handlers.
// Requests go through ServeHTTP directly (one goroutine), so plain fields are race-free.
func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, now: time.Unix(1_800_000_000, 0)}
	clock := func() time.Time { return e.now }
	sessions, err := auth.OpenSessions(filepath.Join(t.TempDir(), "sessions.json"), sessionTTL, clock)
	if err != nil {
		t.Fatal(err)
	}
	hash := auth.HashPassword("pw", []byte("0123456789abcdef"), auth.Cost{Time: 1, MemoryKiB: 64, Threads: 1})
	e.h = web.New(web.Options{Username: "me", PasswordHash: hash, SessionTTL: sessionTTL, Sessions: sessions,
		Limiter: auth.NewLimiter(clock),
		API:     http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { e.apiCalls++ }),
		TabSocket: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			e.wsCalls++
			if r.PathValue("id") != "t1" {
				t.Errorf("TabSocket path value id: expected t1, got %q", r.PathValue("id"))
			}
		}),
	})
	return e
}

func (e *env) do(method, target, body string, cookie *http.Cookie, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) login(user, pass string) *httptest.ResponseRecorder {
	b, err := json.Marshal(map[string]string{"username": user, "password": pass})
	if err != nil {
		e.t.Fatal(err)
	}
	return e.do("POST", "/login", string(b), nil, nil)
}

func (e *env) loggedIn() *http.Cookie {
	e.t.Helper()
	rec := e.login("me", "pw")
	if rec.Code != http.StatusNoContent {
		e.t.Fatalf("login: expected 204, got %d %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return &http.Cookie{Name: c.Name, Value: c.Value}
		}
	}
	e.t.Fatal("login: no session cookie")
	return nil
}

func errKind(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Op   string `json:"op"`
			Kind string `json:"kind"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body %q: %v", rec.Body, err)
	}
	if body.Error.Op == "" {
		t.Fatalf("error body %q: missing op", rec.Body)
	}
	return body.Error.Kind
}

func TestLoginSetsCookieAttributes(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.login("me", "pw")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("login: expected 204, got %d %s", rec.Code, rec.Body)
	}
	sc := rec.Header().Get("Set-Cookie")
	for _, want := range []string{"roosterm_session=", "Path=/", "Max-Age=3600", "HttpOnly", "Secure", "SameSite=Strict"} {
		if !strings.Contains(sc, want) {
			t.Errorf("Set-Cookie %q: missing %q", sc, want)
		}
	}
	if c := rec.Result().Cookies(); len(c) != 1 || len(c[0].Value) != 43 {
		t.Fatalf("session cookie: expected one 43-char token, got %v", c)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, c := range [][2]string{{"me", "nope"}, {"you", "pw"}} {
		rec := e.login(c[0], c[1])
		if rec.Code != http.StatusUnauthorized || errKind(t, rec) != "unauthorized" {
			t.Errorf("login %v: expected 401 unauthorized, got %d %s", c, rec.Code, rec.Body)
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("login %v: expected no cookie", c)
		}
	}
	if rec := e.do("POST", "/login", `{"username":"me"}`, nil, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("login without password: expected 400, got %d", rec.Code)
	}
	if rec := e.do("POST", "/login", `{"username":"me","password":"pw","x":1}`, nil, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("login with unknown field: expected 400, got %d", rec.Code)
	}
}

func TestLoginRateLimited(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for i := 0; i < auth.MaxFailures; i++ {
		if rec := e.login("me", "nope"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: expected 401, got %d", i+1, rec.Code)
		}
	}
	rec := e.login("me", "pw")
	if rec.Code != http.StatusTooManyRequests || errKind(t, rec) != "too_many_attempts" {
		t.Fatalf("locked login: expected 429 too_many_attempts, got %d %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("Retry-After: expected 60, got %q", got)
	}
	e.now = e.now.Add(auth.Lockout)
	if rec := e.login("me", "pw"); rec.Code != http.StatusNoContent {
		t.Fatalf("after lockout: expected 204, got %d", rec.Code)
	}
}

func TestAPIRequiresSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, c := range []*http.Cookie{nil, {Name: auth.CookieName, Value: "bogus"}} {
		rec := e.do("GET", "/api/tabs", "", c, nil)
		if rec.Code != http.StatusUnauthorized || errKind(t, rec) != "unauthorized" {
			t.Fatalf("cookie %v: expected 401, got %d", c, rec.Code)
		}
	}
	if e.apiCalls != 0 {
		t.Fatalf("API handler: expected 0 calls without session, got %d", e.apiCalls)
	}
	if rec := e.do("GET", "/api/tabs", "", e.loggedIn(), nil); rec.Code != http.StatusOK || e.apiCalls != 1 {
		t.Fatalf("with session: expected 200 and 1 API call, got %d and %d", rec.Code, e.apiCalls)
	}
}

func TestWebSocketUpgradeRejectedWithoutCookie(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	h := http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}, "Sec-Websocket-Version": {"13"},
		"Sec-Websocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="}}
	rec := e.do("GET", "/ws/tabs/t1?cols=80&rows=24", "", nil, h)
	if rec.Code != http.StatusUnauthorized || e.wsCalls != 0 {
		t.Fatalf("upgrade without cookie: expected 401 and 0 handler calls, got %d and %d", rec.Code, e.wsCalls)
	}
	if rec := e.do("GET", "/ws/tabs/t1?cols=80&rows=24", "", e.loggedIn(), h); e.wsCalls != 1 {
		t.Fatalf("upgrade with cookie: expected 1 handler call, got %d (status %d)", e.wsCalls, rec.Code)
	}
}

func TestExpiredSessionRejected(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.loggedIn()
	e.now = e.now.Add(sessionTTL)
	if rec := e.do("GET", "/api/tabs", "", c, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired session: expected 401, got %d", rec.Code)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.loggedIn()
	rec := e.do("POST", "/logout", "", c, nil)
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout: expected 204 with Max-Age=0 cookie, got %d %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	if rec := e.do("GET", "/api/tabs", "", c, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("after logout: expected 401, got %d", rec.Code)
	}
}

func TestCrossOriginStateChangeRejected(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c := e.loggedIn()
	const cmd = `{"command":["/usr/bin/touch","/tmp/pwn"]}`
	attacks := map[string]http.Header{
		"same-site":   {"Sec-Fetch-Site": {"same-site"}, "Origin": {"https://blog.example.com"}, "Content-Type": {"text/plain"}},
		"origin only": {"Origin": {"http://localhost:3000"}, "Content-Type": {"text/plain"}},
		"null origin": {"Origin": {"null"}, "Content-Type": {"text/plain"}},
	}
	for name, h := range attacks {
		rec := e.do("POST", "/api/tabs", cmd, c, h)
		if rec.Code != http.StatusForbidden || errKind(t, rec) != "cross_origin" || e.apiCalls != 0 {
			t.Fatalf("%s: expected 403 cross_origin and 0 API calls, got %d %s and %d calls", name, rec.Code, rec.Body, e.apiCalls)
		}
	}
	site := http.Header{"Sec-Fetch-Site": {"same-site"}}
	if rec := e.do("POST", "/logout", "", c, site); rec.Code != http.StatusForbidden || errKind(t, rec) != "cross_origin" {
		t.Fatalf("cross-site logout: expected 403 cross_origin, got %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("GET", "/api/tabs", "", c, nil); rec.Code != http.StatusOK {
		t.Fatalf("after refused logout: expected session alive (200), got %d", rec.Code)
	}
	cross := func(pass string) *httptest.ResponseRecorder {
		return e.do("POST", "/login", `{"username":"me","password":"`+pass+`"}`, nil, http.Header{"Sec-Fetch-Site": {"cross-site"}})
	}
	if rec := cross("pw"); rec.Code != http.StatusForbidden || rec.Header().Get("Set-Cookie") != "" {
		t.Fatalf("cross-site login: expected 403 without cookie, got %d %q", rec.Code, rec.Header().Get("Set-Cookie"))
	}
	for range 5 {
		cross("wrong")
	}
	if rec := e.login("me", "pw"); rec.Code != http.StatusNoContent {
		t.Fatalf("login after cross-site failures: expected 204 (limiter untouched), got %d", rec.Code)
	}

	same := e.apiCalls
	for name, h := range map[string]http.Header{
		"same-origin": {"Sec-Fetch-Site": {"same-origin"}},
		"origin=host": {"Origin": {"http://example.com"}},
	} {
		if rec := e.do("POST", "/api/tabs", cmd, c, h); rec.Code != http.StatusOK {
			t.Fatalf("%s control: expected 200, got %d %s", name, rec.Code, rec.Body)
		}
		same++
		if e.apiCalls != same {
			t.Fatalf("%s control: expected API reached (%d calls), got %d", name, same, e.apiCalls)
		}
	}
	if rec := e.do("GET", "/api/tabs", "", c, http.Header{"Sec-Fetch-Site": {"cross-site"}}); rec.Code != http.StatusOK {
		t.Fatalf("cross-site GET: expected 200, got %d", rec.Code)
	}
}
