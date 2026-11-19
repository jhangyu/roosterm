package bridge_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"roosterm/host/internal/bridge"
	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

// noPing keeps the ticker out of the tests.
const noPing = time.Hour

type env struct {
	t    *testing.T
	fake *hctest.Fake
	sup  *hc.Supervisor
	srv  *httptest.Server
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
	mux := http.NewServeMux()
	mux.Handle("GET /ws/tabs/{id}", bridge.New(sup, noPing))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &env{t: t, fake: f, sup: sup, srv: srv}
}

func (e *env) tab() string {
	e.t.Helper()
	c, err := e.sup.Client()
	if err != nil {
		e.t.Fatal(err)
	}
	term, err := c.Run(e.t.Context(), hc.RunParams{Argv: []string{"/bin/sh"}, Size: hc.Size{Cols: 80, Rows: 24}, NoParent: true,
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: "sh"}})
	if err != nil {
		e.t.Fatal(err)
	}
	return term.Address
}

func (e *env) dial(id string) *websocket.Conn {
	e.t.Helper()
	u := "ws" + strings.TrimPrefix(e.srv.URL, "http") + "/ws/tabs/" + id + "?cols=80&rows=24"
	conn, _, err := websocket.Dial(e.t.Context(), u, nil)
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func read(t *testing.T, conn *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	typ, b, err := conn.Read(t.Context())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return typ, b
}

// closeOf reads until the server closes and returns the close frame.
func closeOf(t *testing.T, conn *websocket.Conn) (websocket.CloseError, [][]byte) {
	t.Helper()
	var texts [][]byte
	for {
		typ, b, err := conn.Read(t.Context())
		if err == nil {
			if typ == websocket.MessageText {
				texts = append(texts, b)
			}
			continue
		}
		var ce websocket.CloseError
		if !errors.As(err, &ce) {
			t.Fatalf("read: expected a close frame, got %v", err)
		}
		return ce, texts
	}
}

func write(t *testing.T, conn *websocket.Conn, typ websocket.MessageType, b string) {
	t.Helper()
	if err := conn.Write(t.Context(), typ, []byte(b)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// wsPaint is what the browser gets for an attach: clear scrollback (ED3), then hc's first paint,
// so the scrollback the paint replays never stacks on what a reconnecting terminal already holds.
func wsPaint(id string) []byte { return append([]byte("\x1b[3J"), hctest.Paint(id)...) }

func TestBridgeFirstPaintThenEcho(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	if typ, b := read(t, conn); typ != websocket.MessageBinary || !bytes.Equal(b, wsPaint(id)) {
		t.Fatalf("first message: expected binary paint, got %v %q", typ, b)
	}
	write(t, conn, websocket.MessageBinary, "echo $$\r")
	if got := <-e.fake.Input(id); string(got) != "echo $$\r" {
		t.Fatalf("hc input: got %q", got)
	}
	if typ, b := read(t, conn); typ != websocket.MessageBinary || string(b) != "echo $$\r" {
		t.Fatalf("echo: got %v %q", typ, b)
	}
}

func TestBridgeResizeReachesHC(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	read(t, conn)
	write(t, conn, websocket.MessageText, `{"type":"resize","cols":100,"rows":30}`)
	if got := <-e.fake.Resized(id); got != (hc.Size{Cols: 100, Rows: 30}) {
		t.Fatalf("hc resize: got %+v", got)
	}
}

func TestBridgeExitClosesWith4000(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	read(t, conn)
	e.fake.Exit(id, 7)
	ce, _ := closeOf(t, conn)
	if ce.Code != bridge.CloseExited || ce.Reason != `{"reason":"exited","exitCode":7,"signal":null}` {
		t.Fatalf("close: expected 4000 with exit JSON, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeFinishedTabCloses4000(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	e.fake.Exit(id, 3)
	ce, _ := closeOf(t, e.dial(id))
	if ce.Code != bridge.CloseExited || ce.Reason != `{"reason":"exited","exitCode":3,"signal":null}` {
		t.Fatalf("close: expected 4000 exitCode 3, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeUnknownTabCloses4004(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ce, _ := closeOf(t, e.dial("t999"))
	if ce.Code != bridge.CloseNotFound || ce.Reason != "tab t999 not found" {
		t.Fatalf("close: expected 4004, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeForeignTerminalCloses4004(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	e.fake.Seed(hc.Terminal{Address: "x1", State: hc.StateLive, Labels: map[string]string{}})
	ce, _ := closeOf(t, e.dial("x1"))
	if ce.Code != bridge.CloseNotFound {
		t.Fatalf("close: expected 4004, got %d %q", ce.Code, ce.Reason)
	}
}

func TestBridgeBadTextCloses4400(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	for _, msg := range []string{`{"type":"nope"}`, `{"type":"resize","cols":0,"rows":5}`, `not json`} {
		conn := e.dial(id)
		read(t, conn)
		write(t, conn, websocket.MessageText, msg)
		ce, texts := closeOf(t, conn)
		if ce.Code != bridge.CloseBadMessage || !strings.HasPrefix(ce.Reason, "bad message: ") {
			t.Fatalf("%s: expected 4400, got %d %q", msg, ce.Code, ce.Reason)
		}
		if len(texts) != 1 || !strings.Contains(string(texts[0]), `"kind":"bad_request"`) {
			t.Fatalf("%s: expected one error text before close, got %q", msg, texts)
		}
	}
}

func TestBridgeBadSizeIs400(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	for _, q := range []string{"cols=0&rows=24", "cols=80", "cols=80&rows=1001", "cols=x&rows=24"} {
		req, err := http.NewRequestWithContext(t.Context(), "GET", e.srv.URL+"/ws/tabs/t1?"+q, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Error struct{ Kind string } `json:"error"`
		}
		decErr := json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest || decErr != nil || body.Error.Kind != "bad_request" {
			t.Errorf("%s: expected 400 bad_request, got %d (%v)", q, resp.StatusCode, decErr)
		}
	}
}

func TestBridgeHCAttachErrorCloses4500(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	e.fake.FailNext("terminal.attach", hc.RPCError{Message: "spawn pending", Kind: "unavailable"})
	ce, texts := closeOf(t, e.dial(id))
	if ce.Code != bridge.CloseHCError || !strings.Contains(ce.Reason, "spawn pending") {
		t.Fatalf("close: expected 4500 spawn pending, got %d %q", ce.Code, ce.Reason)
	}
	if len(texts) != 1 || !strings.Contains(string(texts[0]), `"kind":"unavailable"`) {
		t.Fatalf("expected one error text with kind unavailable, got %q", texts)
	}
}

func TestBridgeCoreOfflineThenOnline(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	id := e.tab()
	conn := e.dial(id)
	read(t, conn)
	e.fake.Stop()
	if typ, b := read(t, conn); typ != websocket.MessageText || string(b) != `{"type":"core","state":"offline"}` {
		t.Fatalf("after core stop: expected offline text, got %v %q", typ, b)
	}
	write(t, conn, websocket.MessageBinary, "x")
	if _, b := read(t, conn); !strings.Contains(string(b), `"kind":"core_offline"`) || !strings.Contains(string(b), `"op":"tab.input"`) {
		t.Fatalf("input while offline: expected core_offline error, got %q", b)
	}
	write(t, conn, websocket.MessageText, `{"type":"resize","cols":90,"rows":20}`)
	write(t, conn, websocket.MessageBinary, "y") // its error reply proves the resize before it was handled
	if _, b := read(t, conn); !strings.Contains(string(b), `"kind":"core_offline"`) {
		t.Fatalf("second input while offline: expected core_offline error, got %q", b)
	}
	e.fake.Restart(t)
	if typ, b := read(t, conn); typ != websocket.MessageText || string(b) != `{"type":"core","state":"online"}` {
		t.Fatalf("after restart: expected online text, got %v %q", typ, b)
	}
	if typ, b := read(t, conn); typ != websocket.MessageBinary || !bytes.Equal(b, wsPaint(id)) {
		t.Fatalf("after online: expected first paint, got %v %q", typ, b)
	}
	if term, _ := e.fake.Get(id); term.Cols != 90 || term.Rows != 20 {
		t.Fatalf("re-attach size: expected 90x20 from the offline resize, got %dx%d", term.Cols, term.Rows)
	}
}
