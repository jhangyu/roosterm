// Package bridge turns one WebSocket into one hc attach stream (docs/api.md, "WebSocket").
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/hc"
)

const (
	CloseExited     websocket.StatusCode = 4000
	CloseNotFound   websocket.StatusCode = 4004
	CloseBadMessage websocket.StatusCode = 4400
	CloseHCError    websocket.StatusCode = 4500
)

const (
	maxCells       = 1000
	maxInput       = 1 << 20
	maxCloseReason = 123 // coder/websocket: 125-byte close payload minus the 2-byte code
	opAttach       = "tab.attach"
	opInput        = "tab.input"
)

type handler struct {
	core *hc.Supervisor
	ping time.Duration
}

// New serves GET /ws/tabs/{id}?cols=N&rows=N. The caller enforces the session cookie before it.
func New(core *hc.Supervisor, pingInterval time.Duration) http.Handler {
	return &handler{core: core, ping: pingInterval}
}

func dim(r *http.Request, id, name string) (uint16, *apierr.Error) {
	v := r.URL.Query().Get(name)
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > maxCells {
		return 0, apierr.BadRequest(opAttach, id, fmt.Sprintf("query %s: expected an integer 1..%d, got %q", name, maxCells, v))
	}
	return uint16(n), nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cols, e := dim(r, id, "cols")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	rows, e := dim(r, id, "rows")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	conn, err := websocket.Accept(w, r, nil) // default Origin check, compression off
	if err != nil {
		log.Printf("ws accept tab %s: %v", id, err) // Accept already wrote the HTTP error
		return
	}
	defer func() {
		// ponytail: after a completed Close handshake CloseNow is a no-op; its error only says "already closed".
		_ = conn.CloseNow()
	}()
	conn.SetReadLimit(maxInput)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	s := &session{conn: conn, core: h.core, id: id, size: hc.Size{Cols: cols, Rows: rows}, in: make(chan clientMsg)}
	go s.read(ctx)
	go s.pinger(ctx, cancel, h.ping)
	s.run(ctx)
}

type clientMsg struct {
	typ  websocket.MessageType
	data []byte
}

type session struct {
	conn        *websocket.Conn
	core        *hc.Supervisor
	id          string
	size        hc.Size
	in          chan clientMsg
	offlineSent bool
}

// read forwards client messages; a read error means the client is gone or the conn closed: in is closed.
func (s *session) read(ctx context.Context) {
	defer close(s.in)
	for {
		typ, data, err := s.conn.Read(ctx)
		if err != nil {
			return
		}
		select {
		case s.in <- clientMsg{typ, data}:
		case <-ctx.Done():
			return
		}
	}
}

func (s *session) pinger(ctx context.Context, cancel context.CancelFunc, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, pcancel := context.WithTimeout(ctx, every) // fail-fast cap: a peer silent for a whole interval is gone
			err := s.conn.Ping(pctx)
			pcancel()
			if err != nil && ctx.Err() == nil {
				log.Printf("ws ping tab %s: %v", s.id, err)
				cancel()
				return
			}
		}
	}
}

type step int

const (
	stepStop  step = iota // session over, the socket was closed or the client left
	stepRetry             // hc connection ended: back to waiting for a client
	stepPump              // attached: pump the stream
)

func (s *session) run(ctx context.Context) {
	for {
		c, ok := s.online(ctx)
		if !ok {
			return
		}
		st, next := s.open(ctx, c)
		switch next {
		case stepStop:
			return
		case stepRetry:
			continue
		}
		if s.pump(ctx, st) == stepStop {
			return
		}
	}
}

type textMsg struct {
	Type    string `json:"type"`
	State   string `json:"state,omitempty"`
	Op      string `json:"op,omitempty"`
	Tab     string `json:"tab,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
}

func (s *session) text(ctx context.Context, m textMsg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("ws text tab %s: %w", s.id, err)
	}
	return s.conn.Write(ctx, websocket.MessageText, b)
}

func truncate(s string) string {
	if len(s) <= maxCloseReason {
		return s
	}
	n := maxCloseReason
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func (s *session) close(code websocket.StatusCode, reason string) {
	if err := s.conn.Close(code, truncate(reason)); err != nil {
		log.Printf("ws close tab %s (%d): %v", s.id, code, err)
	}
}

func (s *session) shutdown() { s.close(websocket.StatusGoingAway, "server shutdown") }

// online returns a client; while hc is offline it tells the client once and keeps draining input.
func (s *session) online(ctx context.Context) (*hc.Client, bool) {
	if c, err := s.core.Client(); err == nil {
		return c, true
	}
	if !s.offlineSent {
		if err := s.text(ctx, textMsg{Type: "core", State: "offline"}); err != nil {
			return nil, false
		}
		s.offlineSent = true
	}
	actx, acancel := context.WithCancel(ctx)
	defer acancel()
	type awaited struct {
		c   *hc.Client
		err error
	}
	ready := make(chan awaited, 1)
	go func() {
		c, err := s.core.Await(actx)
		ready <- awaited{c, err}
	}()
	for {
		select {
		case a := <-ready:
			if a.err != nil {
				s.shutdown()
				return nil, false
			}
			return a.c, true
		case m, ok := <-s.in:
			if !ok {
				return nil, false
			}
			if !s.offlineInput(ctx, m) {
				return nil, false
			}
		case <-ctx.Done():
			s.shutdown()
			return nil, false
		}
	}
}

func (s *session) offlineInput(ctx context.Context, m clientMsg) bool {
	if m.typ == websocket.MessageBinary {
		err := s.text(ctx, textMsg{Type: "error", Op: opInput, Tab: s.id, Kind: apierr.KindCoreOffline,
			Message: "input dropped: hc core offline"})
		return err == nil
	}
	size, err := parseResize(m.data)
	if err != nil {
		s.badMessage(ctx, err)
		return false
	}
	s.size = size
	return true
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

func parseResize(b []byte) (hc.Size, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var m resizeMsg
	if err := dec.Decode(&m); err != nil {
		return hc.Size{}, fmt.Errorf("expected {\"type\":\"resize\",\"cols\":N,\"rows\":N}, got %v", err)
	}
	if m.Type != "resize" {
		return hc.Size{}, fmt.Errorf("type: expected \"resize\", got %q", m.Type)
	}
	if m.Cols < 1 || m.Cols > maxCells || m.Rows < 1 || m.Rows > maxCells {
		return hc.Size{}, fmt.Errorf("size: expected cols and rows 1..%d, got %dx%d", maxCells, m.Cols, m.Rows)
	}
	return hc.Size{Cols: uint16(m.Cols), Rows: uint16(m.Rows)}, nil
}

func (s *session) badMessage(ctx context.Context, err error) {
	if terr := s.text(ctx, textMsg{Type: "error", Op: opInput, Tab: s.id, Kind: apierr.KindBadRequest, Message: err.Error()}); terr != nil {
		return // client already gone
	}
	s.close(CloseBadMessage, "bad message: "+err.Error())
}

type exitReason struct {
	Reason   string `json:"reason"`
	ExitCode *int   `json:"exitCode"`
	Signal   *int   `json:"signal"`
}

func (s *session) closeExit(e *hc.Exit) {
	b, err := json.Marshal(exitReason{Reason: e.Reason, ExitCode: e.ExitCode, Signal: e.Signal})
	if err != nil {
		log.Printf("ws exit reason tab %s: %v", s.id, err)
	}
	s.close(CloseExited, string(b))
}

func exitOf(t hc.Terminal) *hc.Exit {
	e := &hc.Exit{ExitCode: t.ExitCode, Signal: t.Signal}
	if t.Reason != nil {
		e.Reason = *t.Reason
	}
	return e
}

// hcFail handles an error from Get/Attach.
func (s *session) hcFail(ctx context.Context, err error) step {
	var re *hc.RPCError
	switch {
	case errors.Is(err, hc.ErrCoreGone):
		return stepRetry
	case ctx.Err() != nil:
		s.shutdown()
	case errors.As(err, &re) && re.Kind == hc.KindNotFound:
		s.close(CloseNotFound, fmt.Sprintf("tab %s not found", s.id))
	default:
		kind := apierr.KindInternal
		if re != nil {
			kind = re.Kind
		}
		if terr := s.text(ctx, textMsg{Type: "error", Op: opAttach, Tab: s.id, Kind: kind, Message: err.Error()}); terr == nil {
			s.close(CloseHCError, err.Error())
		}
	}
	return stepStop
}

// open checks the tab and attaches; the stream is non-nil only with stepPump.
func (s *session) open(ctx context.Context, c *hc.Client) (*hc.Stream, step) {
	t, err := c.Get(ctx, s.id)
	if err != nil {
		return nil, s.hcFail(ctx, err)
	}
	if !hc.IsTab(t) {
		s.close(CloseNotFound, fmt.Sprintf("tab %s not found", s.id))
		return nil, stepStop
	}
	if t.State == hc.StateFinished {
		s.closeExit(exitOf(t))
		return nil, stepStop
	}
	st, err := c.Attach(ctx, s.id, s.size)
	var re *hc.RPCError
	if errors.As(err, &re) && re.Kind == hc.KindFinished {
		t, err := c.Get(ctx, s.id)
		if err != nil {
			return nil, s.hcFail(ctx, err)
		}
		s.closeExit(exitOf(t))
		return nil, stepStop
	}
	if err != nil {
		return nil, s.hcFail(ctx, err)
	}
	return st, stepPump
}

func (s *session) detach(st *hc.Stream) {
	if err := st.Close(); err != nil && !errors.Is(err, hc.ErrCoreGone) {
		log.Printf("hc detach tab %s: %v", s.id, err)
	}
}

// clearScrollback (ED3) goes before each attach's first paint: the paint replays scrollback, and the
// browser keeps its terminal across reconnects, so without it every reattach would duplicate history.
var clearScrollback = []byte("\x1b[3J")

// forward copies hc output to the WebSocket and returns credit after each message.
func (s *session) forward(ctx context.Context, st *hc.Stream) error {
	for first := true; ; first = false {
		p, err := st.Recv(ctx)
		if err != nil {
			return err
		}
		msg := p
		if first {
			msg = append(append([]byte{}, clearScrollback...), p...)
		}
		if err := s.conn.Write(ctx, websocket.MessageBinary, msg); err != nil {
			return fmt.Errorf("ws write tab %s: %w", s.id, err)
		}
		if err := st.Credit(len(p)); err != nil {
			return err
		}
	}
}

func (s *session) pump(ctx context.Context, st *hc.Stream) step {
	if s.offlineSent {
		if err := s.text(ctx, textMsg{Type: "core", State: "online"}); err != nil {
			s.detach(st)
			return stepStop
		}
		s.offlineSent = false
	}
	pctx, pcancel := context.WithCancel(ctx)
	defer pcancel()
	out := make(chan error, 1)
	go func() { out <- s.forward(pctx, st) }()
	for {
		select {
		case err := <-out:
			var ex *hc.Exit
			switch {
			case errors.As(err, &ex):
				s.closeExit(ex)
				return stepStop
			case errors.Is(err, hc.ErrCoreGone):
				return stepRetry
			case ctx.Err() != nil:
				s.detach(st)
				s.shutdown()
				return stepStop
			default: // WebSocket write failed (client gone) or unexpected stream end
				log.Printf("ws tab %s: %v", s.id, err)
				s.detach(st)
				return stepStop
			}
		case m, ok := <-s.in:
			if !ok {
				s.detach(st)
				return stepStop
			}
			var err error
			if m.typ == websocket.MessageBinary {
				err = st.Write(m.data)
			} else {
				size, perr := parseResize(m.data)
				if perr != nil {
					s.detach(st)
					s.badMessage(ctx, perr)
					return stepStop
				}
				s.size = size
				err = st.Resize(size)
			}
			if err != nil { // every hc write error wraps ErrCoreGone
				return stepRetry
			}
		case <-ctx.Done():
			s.detach(st)
			s.shutdown()
			return stepStop
		}
	}
}
