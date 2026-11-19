package api_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"roosterm/host/internal/api"
	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

type env struct {
	t        *testing.T
	fake     *hctest.Fake
	sup      *hc.Supervisor
	h        http.Handler
	swarmDir string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	f := hctest.Start(t)
	sup, err := hc.Start(t.Context(), f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sup.Close(); err != nil {
			t.Errorf("supervisor close: %v", err)
		}
	})
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return &env{t: t, fake: f, sup: sup, swarmDir: dir,
		h: api.New(api.Options{Core: sup, Shell: "/bin/zsh", Home: "/home/me", SwarmDir: dir})}
}

func (e *env) do(method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(e.t.Context(), method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body, err)
	}
	return v
}

type errBody struct {
	Error struct {
		Op, Tab, Message, Kind, Hint string
	} `json:"error"`
}

func tabTerm(id string, created int64, label string) hc.Terminal {
	return hc.Terminal{Address: id, State: hc.StateLive, CreatedMS: created,
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: label}}
}

func TestCreateTabDefaultShell(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.do("POST", "/api/tabs", `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d %s", rec.Code, rec.Body)
	}
	tab := decode[api.Tab](t, rec)
	if tab.ID != "t1" || tab.Label != "zsh" || tab.Exited || tab.ExitCode != nil || tab.Reason != nil {
		t.Fatalf("tab: got %+v", tab)
	}
	term, ok := e.fake.Get("t1")
	if !ok || !slices.Equal(term.Argv, []string{"/bin/zsh", "-l"}) || term.Cwd != "/home/me" ||
		term.Labels[hc.LabelOwner] != hc.LabelOwnerTab || term.Labels[hc.LabelName] != "zsh" || term.Cols != 80 || term.Rows != 24 {
		t.Fatalf("hc terminal: got %+v", term)
	}
}

func TestCreateTabCommandAndLabel(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.do("POST", "/api/tabs", `{"command":["tmux","-L","claude-swarm-4242","a"],"label":"swarm 4242"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d %s", rec.Code, rec.Body)
	}
	if tab := decode[api.Tab](t, rec); tab.Label != "swarm 4242" {
		t.Fatalf("label: got %q", tab.Label)
	}
	term, _ := e.fake.Get("t1")
	if !slices.Equal(term.Argv, []string{"tmux", "-L", "claude-swarm-4242", "a"}) {
		t.Fatalf("argv: got %v", term.Argv)
	}
	rec = e.do("POST", "/api/tabs", `{"command":["/usr/bin/top"]}`)
	if tab := decode[api.Tab](t, rec); tab.Label != "top" {
		t.Fatalf("default label from argv[0]: expected top, got %q", tab.Label)
	}
}

func TestCreateTabRejectsBadInput(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	cases := map[string]int{
		`{"command":[]}`:                              http.StatusBadRequest,
		`{"command":["sh",""]}`:                       http.StatusBadRequest,
		`{"label":""}`:                                http.StatusBadRequest,
		`{"label":"a\u0007"}`:                         http.StatusBadRequest,
		`{"label":"` + strings.Repeat("x", 65) + `"}`: http.StatusBadRequest,
		`{"bogus":1}`:                                 http.StatusBadRequest,
		`not json`:                                    http.StatusBadRequest,
		`{} {}`:                                       http.StatusBadRequest,
		`{"label":"` + strings.Repeat("x", 70000) + `"}`: http.StatusRequestEntityTooLarge,
	}
	for body, want := range cases {
		rec := e.do("POST", "/api/tabs", body)
		if rec.Code != want || decode[errBody](t, rec).Error.Kind != "bad_request" {
			t.Errorf("body %.40q: expected %d bad_request, got %d %s", body, want, rec.Code, rec.Body)
		}
	}
	if _, ok := e.fake.Get("t1"); ok {
		t.Fatal("bad input: expected no terminal created")
	}
}

func TestListTabsOnlyRoostermInOrder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if rec := e.do("GET", "/api/tabs", ""); rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("empty list: expected 200 [], got %d %q", rec.Code, rec.Body)
	}
	e.fake.Seed(hc.Terminal{Address: "x1", State: hc.StateLive, CreatedMS: 1, Labels: map[string]string{}})
	e.fake.Seed(tabTerm("x3", 30, "c"))
	e.fake.Seed(tabTerm("x2", 20, "b"))
	done := tabTerm("x4", 40, "d")
	reason, code := "exited", 0
	done.State, done.Reason, done.ExitCode = hc.StateFinished, &reason, &code
	e.fake.Seed(done)
	tabs := decode[[]api.Tab](t, e.do("GET", "/api/tabs", ""))
	var ids []string
	for _, tab := range tabs {
		ids = append(ids, tab.ID)
	}
	if !slices.Equal(ids, []string{"x2", "x3", "x4"}) {
		t.Fatalf("ids: expected [x2 x3 x4], got %v", ids)
	}
	last := tabs[2]
	if !last.Exited || last.ExitCode == nil || *last.ExitCode != 0 || last.Reason == nil || *last.Reason != "exited" {
		t.Fatalf("finished tab: got %+v", last)
	}
}

func TestRenameTab(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.do("POST", "/api/tabs", `{}`)
	rec := e.do("PATCH", "/api/tabs/t1", `{"label":"hello world"}`)
	if rec.Code != http.StatusOK || decode[api.Tab](t, rec).Label != "hello world" {
		t.Fatalf("rename: expected 200 hello world, got %d %s", rec.Code, rec.Body)
	}
	if term, _ := e.fake.Get("t1"); term.Labels[hc.LabelName] != "hello world" || term.Labels[hc.LabelOwner] != hc.LabelOwnerTab {
		t.Fatalf("hc labels: got %v", term.Labels)
	}
}

func TestDeleteLiveTabKillsThenRemoves(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.do("POST", "/api/tabs", `{}`)
	if rec := e.do("DELETE", "/api/tabs/t1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete live: expected 204, got %d %s", rec.Code, rec.Body)
	}
	if _, ok := e.fake.Get("t1"); ok {
		t.Fatal("delete live: terminal still in hc")
	}
}

func TestDeleteFinishedTab(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	done := tabTerm("x1", 1, "a")
	done.State = hc.StateFinished
	e.fake.Seed(done)
	if rec := e.do("DELETE", "/api/tabs/x1", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete finished: expected 204, got %d %s", rec.Code, rec.Body)
	}
	if _, ok := e.fake.Get("x1"); ok {
		t.Fatal("delete finished: terminal still in hc")
	}
}

func TestForeignTerminalIsNotFound(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.fake.Seed(hc.Terminal{Address: "x1", State: hc.StateLive, Labels: map[string]string{}})
	for _, req := range [][3]string{{"PATCH", "/api/tabs/x1", `{"label":"a"}`}, {"DELETE", "/api/tabs/x1", ""}, {"DELETE", "/api/tabs/t999", ""}} {
		rec := e.do(req[0], req[1], req[2])
		if rec.Code != http.StatusNotFound || decode[errBody](t, rec).Error.Kind != "not_found" {
			t.Errorf("%s %s: expected 404 not_found, got %d %s", req[0], req[1], rec.Code, rec.Body)
		}
	}
	if _, ok := e.fake.Get("x1"); !ok {
		t.Fatal("foreign terminal was removed")
	}
}

func TestHCErrorForwarded(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.fake.FailNext("terminal.run", hc.RPCError{Message: "spawn pending", Kind: "unavailable", Hint: "retry later"})
	rec := e.do("POST", "/api/tabs", `{}`)
	b := decode[errBody](t, rec).Error
	if rec.Code != http.StatusBadGateway || b.Kind != "unavailable" || b.Hint != "retry later" || !strings.Contains(b.Message, "spawn pending") || b.Op != "tab.create" {
		t.Fatalf("hc error: expected 502 unavailable, got %d %s", rec.Code, rec.Body)
	}
}

func TestCoreOfflineIs503(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	c, err := e.sup.Client()
	if err != nil {
		t.Fatal(err)
	}
	e.fake.Stop()
	<-c.Done()
	rec := e.do("GET", "/api/tabs", "")
	if rec.Code != http.StatusServiceUnavailable || decode[errBody](t, rec).Error.Kind != "core_offline" {
		t.Fatalf("offline: expected 503 core_offline, got %d %s", rec.Code, rec.Body)
	}
}

func TestSwarmsEndpoint(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, name := range []string{"claude-swarm-7", "default"} {
		ln, err := net.Listen("unix", filepath.Join(e.swarmDir, name))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ln.Close() })
	}
	rec := e.do("GET", "/api/swarms", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "[\"claude-swarm-7\"]\n" {
		t.Fatalf("swarms: expected 200 [\"claude-swarm-7\"], got %d %q", rec.Code, rec.Body)
	}
}
