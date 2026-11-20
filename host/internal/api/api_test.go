package api_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	home     string // symlink-resolved; t.TempDir() sits under a symlink on macOS
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
	home := t.TempDir()
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	return &env{t: t, fake: f, sup: sup, swarmDir: dir, home: resolved,
		h: api.New(api.Options{Core: sup, Shell: "/bin/zsh", Home: home, SwarmDir: dir})}
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
	if tab.ID != "t1" || tab.Label != "~" || tab.Cwd != "" || tab.Renamed || tab.Exited || tab.ExitCode != nil || tab.Reason != nil {
		t.Fatalf("tab: got %+v", tab)
	}
	term, ok := e.fake.Get("t1")
	if !ok || !slices.Equal(term.Argv, api.ScrubArgv([]string{"/bin/zsh", "-l"})) || term.Cwd != e.home ||
		term.Labels[hc.LabelOwner] != hc.LabelOwnerTab || term.Labels[hc.LabelName] != "~" || term.Cols != 80 || term.Rows != 24 {
		t.Fatalf("hc terminal: got %+v", term)
	}
}

func TestCreateTabCommandAndLabel(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	rec := e.do("POST", "/api/tabs", `{"command":["tmux","-L","other-4242","a"],"label":"swarm 4242"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d %s", rec.Code, rec.Body)
	}
	if tab := decode[api.Tab](t, rec); tab.Label != "swarm 4242" {
		t.Fatalf("label: got %q", tab.Label)
	}
	term, _ := e.fake.Get("t1")
	if !slices.Equal(term.Argv, api.ScrubArgv([]string{"tmux", "-L", "other-4242", "a"})) {
		t.Fatalf("argv: got %v", term.Argv)
	}
}

func TestCreateTabInFolder(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if err := os.MkdirAll(filepath.Join(e.home, "project", "roosterm"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := e.do("POST", "/api/tabs", `{"command":["claude"],"cwd":"project/roosterm"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d %s", rec.Code, rec.Body)
	}
	if tab := decode[api.Tab](t, rec); tab.Label != "roosterm" || tab.Cwd != "project/roosterm" {
		t.Fatalf("tab: expected label roosterm cwd project/roosterm, got %+v", tab)
	}
	if term, _ := e.fake.Get("t1"); term.Cwd != filepath.Join(e.home, "project", "roosterm") {
		t.Fatalf("hc cwd: expected %s, got %q", filepath.Join(e.home, "project", "roosterm"), term.Cwd)
	}
}

func TestCreateTabBadCwdNeverReachesHC(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	if err := os.WriteFile(filepath.Join(e.home, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for body, want := range map[string]int{
		`{"cwd":"../x"}`: http.StatusBadRequest, `{"cwd":"/etc"}`: http.StatusBadRequest,
		`{"cwd":"nope"}`: http.StatusNotFound, `{"cwd":"file"}`: http.StatusNotFound,
	} {
		if rec := e.do("POST", "/api/tabs", body); rec.Code != want {
			t.Errorf("%s: expected %d, got %d %s", body, want, rec.Code, rec.Body)
		}
	}
	if _, ok := e.fake.Get("t1"); ok {
		t.Fatal("bad cwd: expected no terminal created")
	}
}

func TestDirs(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	outside := t.TempDir()
	for _, d := range []string{"b", "a", ".hidden", "a/inner"} {
		if err := os.Mkdir(filepath.Join(e.home, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range map[string]string{"link-in": filepath.Join(e.home, "b"), "link-out": outside, "dangling": filepath.Join(e.home, "gone")} {
		if err := os.Symlink(target, filepath.Join(e.home, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(e.home, "c.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	rec := e.do("GET", "/api/dirs", "")
	type listing struct {
		Path    string         `json:"path"`
		Entries []api.DirEntry `json:"entries"`
	}
	got := decode[listing](t, rec)
	want := []api.DirEntry{{Name: ".hidden", Hidden: true}, {Name: "a"}, {Name: "b"}, {Name: "link-in"}}
	if rec.Code != http.StatusOK || got.Path != "" || !slices.Equal(got.Entries, want) {
		t.Fatalf("home listing: expected 200 %v, got %d %+v", want, rec.Code, got)
	}
	if got := decode[listing](t, e.do("GET", "/api/dirs?path=a", "")); got.Path != "a" || !slices.Equal(got.Entries, []api.DirEntry{{Name: "inner"}}) {
		t.Fatalf("a listing: got %+v", got)
	}
	if got := decode[listing](t, e.do("GET", "/api/dirs?path=a/inner", "")); got.Entries == nil || len(got.Entries) != 0 {
		t.Fatalf("empty dir: expected [], got %+v", got)
	}
	for q, want := range map[string][2]any{
		"..": {400, "bad_request"}, "a/../b": {400, "bad_request"}, "%2Fetc": {400, "bad_request"}, "a%00": {400, "bad_request"},
		"link-out": {403, "outside_home"}, "missing": {404, "not_found"}, "c.txt": {404, "not_found"}, "dangling": {404, "not_found"},
	} {
		rec := e.do("GET", "/api/dirs?path="+q, "")
		if b := decode[errBody](t, rec).Error; rec.Code != want[0] || b.Kind != want[1] || b.Op != "dirs.list" {
			t.Errorf("path %s: expected %v, got %d %s", q, want, rec.Code, rec.Body)
		}
	}
}

func TestDirsPermissionDenied(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	locked := filepath.Join(e.home, "locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })
	rec := e.do("GET", "/api/dirs?path=locked", "")
	if rec.Code != http.StatusForbidden || decode[errBody](t, rec).Error.Kind != "permission_denied" {
		t.Fatalf("locked: expected 403 permission_denied, got %d %s", rec.Code, rec.Body)
	}
	if rec := e.do("POST", "/api/tabs", `{"cwd":"locked"}`); rec.Code != http.StatusCreated {
		t.Fatalf("unlistable folder stays selectable: expected 201, got %d %s", rec.Code, rec.Body)
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
	titled := tabTerm("x2", 20, "b")
	titled.Title, titled.Cwd = "Fix the login bug", filepath.Join(e.home, "proj")
	titled.Argv = api.ScrubArgv([]string{"/usr/local/bin/claude", "--resume"})
	e.fake.Seed(titled)
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
	if tabs[0].Title != "Fix the login bug" || tabs[0].Cwd != "proj" || tabs[0].Program != "claude" || tabs[1].Program != "" {
		t.Fatalf("titled tab: expected title and cwd proj, got %+v", tabs[0])
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
	if tabs := decode[[]api.Tab](t, e.do("GET", "/api/tabs", "")); tabs[0].Renamed {
		t.Fatalf("new tab: expected not renamed, got %+v", tabs[0])
	}
	rec := e.do("PATCH", "/api/tabs/t1", `{"label":"hello world"}`)
	if tab := decode[api.Tab](t, rec); rec.Code != http.StatusOK || tab.Label != "hello world" || !tab.Renamed {
		t.Fatalf("rename: expected 200 hello world, got %d %s", rec.Code, rec.Body)
	}
	if term, _ := e.fake.Get("t1"); term.Labels[hc.LabelName] != "hello world" || term.Labels[hc.LabelOwner] != hc.LabelOwnerTab {
		t.Fatalf("hc labels: got %v", term.Labels)
	}
}

func TestHideAndUnhideTab(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.do("POST", "/api/tabs", `{}`)
	e.do("POST", "/api/tabs", `{}`)
	rec := e.do("PATCH", "/api/tabs/t1", `{"hidden":true}`)
	if rec.Code != http.StatusOK || !decode[api.Tab](t, rec).Hidden {
		t.Fatalf("hide: expected 200 hidden, got %d %s", rec.Code, rec.Body)
	}
	if term, _ := e.fake.Get("t1"); term.Labels[hc.LabelHidden] != hc.LabelHiddenOn || term.State != hc.StateLive {
		t.Fatalf("hide: expected live terminal with %s=%s, got %s %v", hc.LabelHidden, hc.LabelHiddenOn, term.State, term.Labels)
	}
	tabs := decode[[]api.Tab](t, e.do("GET", "/api/tabs", ""))
	if len(tabs) != 2 || tabs[0].ID != "t1" || !tabs[0].Hidden || tabs[1].Hidden {
		t.Fatalf("list after hide: expected [t1 hidden, t2 shown], got %+v", tabs)
	}
	rec = e.do("PATCH", "/api/tabs/t1", `{"hidden":false}`)
	if rec.Code != http.StatusOK || decode[api.Tab](t, rec).Hidden {
		t.Fatalf("unhide: expected 200 not hidden, got %d %s", rec.Code, rec.Body)
	}
	if term, _ := e.fake.Get("t1"); term.Labels[hc.LabelHidden] != "" || len(term.Labels) != 2 {
		t.Fatalf("unhide: expected %s removed, got %v", hc.LabelHidden, term.Labels)
	}
	if rec := e.do("PATCH", "/api/tabs/t1", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty patch: expected 400, got %d %s", rec.Code, rec.Body)
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
	for _, req := range [][3]string{{"PATCH", "/api/tabs/x1", `{"label":"a"}`}, {"PATCH", "/api/tabs/x1", `{"hidden":true}`}, {"DELETE", "/api/tabs/x1", ""}, {"DELETE", "/api/tabs/t999", ""}} {
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

func TestScrubArgvDropsClaudeAndTmuxEnvKeepsRest(t *testing.T) {
	t.Parallel()
	argv := api.ScrubArgv([]string{"/usr/bin/env"})
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = []string{"TMUX=/x,1,0", "TMUX_PANE=%1", "CLAUDECODE=1", "CLAUDE_CODE_X=1", "CLAUDE_PID=9",
		"HC_TERMINAL_ID=t9", "TMUX_TMPDIR=/keep", "PATH=" + os.Getenv("PATH")}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run scrubbed env: expected exit 0, got %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range got {
		if strings.HasPrefix(l, "TMUX=") || strings.HasPrefix(l, "TMUX_PANE=") || strings.HasPrefix(l, "CLAUDE") {
			t.Fatalf("env line %q survived the scrub", l)
		}
	}
	for _, want := range []string{"HC_TERMINAL_ID=t9", "TMUX_TMPDIR=/keep"} {
		if !slices.Contains(got, want) {
			t.Fatalf("expected %q kept, got %v", want, got)
		}
	}
}
