// Package hctest is an in-process fake herdr-core speaking the real wire format over a Unix socket.
package hctest

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"roosterm/host/internal/hc"
)

const (
	chanBuffer = 64
	errCode    = -32000
	sigHUP     = 1
)

// Paint is the first-paint payload the fake sends on every attach.
func Paint(id string) []byte { return []byte("\x18\x1b[!p" + "screen:" + id) }

type Fake struct {
	Path string

	mu      sync.Mutex
	ln      net.Listener
	conns   map[*fakeConn]struct{}
	terms   map[string]hc.Terminal
	runs    int
	fail    map[string]hc.RPCError
	inputs  map[string]chan []byte
	resizes map[string]chan hc.Size
	wg      sync.WaitGroup
}

type fakeConn struct {
	conn     net.Conn
	wmu      sync.Mutex
	streams  map[uint32]string // channel → terminal id, guarded by Fake.mu
	nextChan uint32
}

// Start listens at <os.MkdirTemp("", "rt")>/hc.sock (short path: macOS limits socket paths to 104 bytes).
func Start(t testing.TB) *Fake {
	t.Helper()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	f := &Fake{Path: filepath.Join(dir, "hc.sock"), conns: map[*fakeConn]struct{}{}, terms: map[string]hc.Terminal{},
		fail: map[string]hc.RPCError{}, inputs: map[string]chan []byte{}, resizes: map[string]chan hc.Size{}}
	t.Cleanup(func() {
		f.Stop()
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("hctest cleanup %s: %v", dir, err)
		}
	})
	f.listen(t)
	return f
}

// listen binds a temp name and renames it to Path, so the socket file appears only once it accepts
// (a watcher's dial on the CREATE event never meets a bound-but-not-listening socket).
func (f *Fake) listen(t testing.TB) {
	t.Helper()
	tmp := f.Path + ".tmp"
	ln, err := net.Listen("unix", tmp)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := os.Rename(tmp, f.Path); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.ln = ln
	f.mu.Unlock()
	f.wg.Add(1)
	go f.accept(ln)
}

// Restart listens again on Path after Stop (terminals survive, like a real core restart).
func (f *Fake) Restart(t testing.TB) {
	t.Helper()
	f.listen(t)
}

// Stop closes the listener and every connection and removes the socket file (core crash/stop).
func (f *Fake) Stop() {
	f.mu.Lock()
	ln := f.ln
	f.ln = nil
	conns := f.conns
	f.conns = map[*fakeConn]struct{}{}
	f.mu.Unlock()
	if ln == nil {
		return
	}
	if err := ln.Close(); err != nil {
		panic(fmt.Sprintf("hctest stop: close listener: %v", err))
	}
	for fc := range conns {
		// ponytail: the peer may have closed first; either way the connection is gone.
		_ = fc.conn.Close()
	}
	if err := os.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		panic(fmt.Sprintf("hctest stop: remove %s: %v", f.Path, err))
	}
	f.wg.Wait()
}

func (f *Fake) Seed(term hc.Terminal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terms[term.Address] = term
}

func (f *Fake) Get(id string) (hc.Terminal, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.terms[id]
	return t, ok
}

// FailNext makes the next call of method return e.
func (f *Fake) FailNext(method string, e hc.RPCError) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[method] = e
}

// Input yields DATA payloads viewers sent to id.
func (f *Fake) Input(id string) <-chan []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inputLocked(id)
}

func (f *Fake) inputLocked(id string) chan []byte {
	ch, ok := f.inputs[id]
	if !ok {
		ch = make(chan []byte, chanBuffer)
		f.inputs[id] = ch
	}
	return ch
}

// Resized yields RESIZE frames viewers sent to id.
func (f *Fake) Resized(id string) <-chan hc.Size {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.resizedLocked(id)
}

func (f *Fake) resizedLocked(id string) chan hc.Size {
	ch, ok := f.resizes[id]
	if !ok {
		ch = make(chan hc.Size, chanBuffer)
		f.resizes[id] = ch
	}
	return ch
}

// Exit finishes id with reason "exited" and sends the CLOSE reason to every attach stream of id.
func (f *Fake) Exit(id string, code int) {
	reason := "exited"
	f.finish(id, hc.Terminal{Reason: &reason, ExitCode: &code})
}

func (f *Fake) finish(id string, end hc.Terminal) hc.Terminal {
	f.mu.Lock()
	t := f.terms[id]
	t.State = hc.StateFinished
	t.Reason, t.ExitCode, t.Signal = end.Reason, end.ExitCode, end.Signal
	f.terms[id] = t
	type target struct {
		fc *fakeConn
		ch uint32
	}
	var targets []target
	for fc := range f.conns {
		for ch, tid := range fc.streams {
			if tid == id {
				targets = append(targets, target{fc, ch})
				delete(fc.streams, ch)
			}
		}
	}
	f.mu.Unlock()
	payload, err := json.Marshal(map[string]any{"exit_code": t.ExitCode, "finished_ms": 0, "reason": t.Reason, "signal": t.Signal})
	if err != nil {
		panic(err)
	}
	for _, tg := range targets {
		tg.fc.write(hc.Frame{Channel: tg.ch, Kind: hc.KindClose, Payload: payload})
	}
	return t
}

func (f *Fake) accept(ln net.Listener) {
	defer f.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed by Stop
		}
		fc := &fakeConn{conn: conn, streams: map[uint32]string{}}
		f.mu.Lock()
		f.conns[fc] = struct{}{}
		f.mu.Unlock()
		f.wg.Add(1)
		go f.serve(fc)
	}
}

func (fc *fakeConn) write(fr hc.Frame) {
	fc.wmu.Lock()
	defer fc.wmu.Unlock()
	// ponytail: a write to a connection the viewer already closed is expected in teardown.
	_ = hc.WriteFrame(fc.conn, fr)
}

func (f *Fake) serve(fc *fakeConn) {
	defer f.wg.Done()
	defer func() {
		_ = fc.conn.Close() // ponytail: connection is finished either way
		f.mu.Lock()
		delete(f.conns, fc)
		f.mu.Unlock()
	}()
	r := bufio.NewReader(fc.conn)
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	var hello struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
	}
	if err := json.Unmarshal(line, &hello); err != nil || hello.Method != "hello" {
		return
	}
	reply := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":{"generation":1,"framed":true,"features":["framed","attach","events"],"version":"0.1.21","core":{"id":"fake","name":"fake"},"methods":[]}}`+"\n", hello.ID)
	fc.wmu.Lock()
	_, err = fc.conn.Write([]byte(reply))
	fc.wmu.Unlock()
	if err != nil {
		return
	}
	for {
		fr, err := hc.ReadFrame(r)
		if err != nil {
			return
		}
		f.handle(fc, fr)
	}
}

func (f *Fake) handle(fc *fakeConn, fr hc.Frame) {
	if fr.Channel == 0 {
		if fr.Kind == hc.KindData {
			f.rpc(fc, fr.Payload)
		}
		return
	}
	f.mu.Lock()
	id, ok := fc.streams[fr.Channel]
	if ok && fr.Kind == hc.KindClose {
		delete(fc.streams, fr.Channel)
	}
	var input chan []byte
	var resized chan hc.Size
	if ok {
		input, resized = f.inputLocked(id), f.resizedLocked(id)
	}
	f.mu.Unlock()
	if !ok {
		return
	}
	switch fr.Kind {
	case hc.KindData:
		input <- fr.Payload
		fc.write(hc.Frame{Channel: fr.Channel, Kind: hc.KindData, Payload: fr.Payload}) // echo
	case hc.KindResize:
		size, err := hc.ParseResize(fr.Payload)
		if err != nil {
			panic(err)
		}
		f.mu.Lock()
		t := f.terms[id]
		t.Cols, t.Rows = size.Cols, size.Rows
		f.terms[id] = t
		f.mu.Unlock()
		resized <- size
	case hc.KindClose:
		fc.write(hc.Frame{Channel: fr.Channel, Kind: hc.KindClose})
	}
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Kind string `json:"kind"`
		Hint string `json:"hint,omitempty"`
	} `json:"data"`
}

func newErr(kind, msg, hint string) *rpcErr {
	e := &rpcErr{Code: errCode, Message: msg}
	e.Data.Kind, e.Data.Hint = kind, hint
	return e
}

func notFound(id string) *rpcErr {
	return newErr(hc.KindNotFound, fmt.Sprintf("no terminal %q", id), "see `hc ls --all`")
}

func (f *Fake) rpc(fc *fakeConn, payload []byte) {
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		panic(fmt.Sprintf("hctest: bad request %q: %v", payload, err))
	}
	var p rpcParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		panic(fmt.Sprintf("hctest: bad params %q: %v", req.Params, err))
	}
	result, rerr, after := f.dispatch(fc, req.Method, p)
	msg := map[string]any{"jsonrpc": "2.0", "id": req.ID}
	if rerr != nil {
		msg["error"] = rerr
	} else {
		msg["result"] = result
	}
	b, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	fc.write(hc.Frame{Channel: 0, Kind: hc.KindData, Payload: b})
	if after != nil {
		after()
	}
}

// rpcParams is the union of every method's params; RunParams.Labels is shadowed by Labels.
type rpcParams struct {
	hc.RunParams
	Terminal string            `json:"terminal"`
	All      bool              `json:"all"`
	Cols     uint16            `json:"cols"`
	Rows     uint16            `json:"rows"`
	Labels   map[string]string `json:"labels"`
}

// dispatch returns the result or error, and an action to run after the reply is written.
func (f *Fake) dispatch(fc *fakeConn, method string, p rpcParams) (any, *rpcErr, func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.fail[method]; ok {
		delete(f.fail, method)
		code := e.Code
		if code == 0 {
			code = errCode
		}
		re := newErr(e.Kind, e.Message, e.Hint)
		re.Code = code
		return nil, re, nil
	}
	switch method {
	case "terminal.run":
		f.runs++
		// p.RunParams.Labels is shadowed by the outer "labels" field; p.Labels carries it.
		t := hc.Terminal{Address: fmt.Sprintf("t%d", f.runs), Argv: p.RunParams.Argv, Labels: map[string]string{}, State: hc.StateLive,
			Cwd: p.RunParams.Cwd, CreatedMS: int64(f.runs), Cols: p.RunParams.Size.Cols, Rows: p.RunParams.Size.Rows}
		for k, v := range p.Labels {
			t.Labels[k] = v
		}
		f.terms[t.Address] = t
		return t, nil, nil
	case "terminal.list":
		out := []hc.Terminal{}
		for _, t := range f.terms {
			if t.State == hc.StateFinished && !p.All {
				continue
			}
			match := true
			for k, v := range p.Labels {
				if t.Labels[k] != v {
					match = false
				}
			}
			if match {
				out = append(out, t)
			}
		}
		return map[string]any{"terminals": out, "hidden_finished": 0, "machines": []any{}}, nil, nil
	}
	t, ok := f.terms[p.Terminal]
	if !ok {
		return nil, notFound(p.Terminal), nil
	}
	switch method {
	case "terminal.get":
		return t, nil, nil
	case "terminal.label":
		merged := map[string]string{}
		for k, v := range t.Labels {
			merged[k] = v
		}
		for k, v := range p.Labels {
			if v == "" {
				delete(merged, k)
			} else {
				merged[k] = v
			}
		}
		t.Labels = merged
		f.terms[p.Terminal] = t
		return t, nil, nil
	case "terminal.kill":
		reason, sig := "killed", sigHUP
		// finish takes f.mu itself, so it runs after the reply, outside this lock
		t.State, t.Reason, t.ExitCode, t.Signal = hc.StateFinished, &reason, nil, &sig
		return t, nil, func() { f.finish(p.Terminal, hc.Terminal{Reason: &reason, Signal: &sig}) }
	case "terminal.rm":
		if t.State != hc.StateFinished {
			return nil, newErr("invalid_params", fmt.Sprintf("%s is live; kill it first", p.Terminal), ""), nil
		}
		delete(f.terms, p.Terminal)
		return map[string]string{"removed": p.Terminal}, nil, nil
	case "terminal.attach":
		if t.State == hc.StateFinished {
			return nil, newErr(hc.KindFinished, p.Terminal+" has finished", "read what it left: hc read "+p.Terminal), nil
		}
		fc.nextChan++
		ch := fc.nextChan
		fc.streams[ch] = p.Terminal
		t.Cols, t.Rows = p.Cols, p.Rows
		f.terms[p.Terminal] = t
		return map[string]any{"channel": ch, "automatic_input": true, "resize_notifications": false, "terminal": t}, nil,
			func() { fc.write(hc.Frame{Channel: ch, Kind: hc.KindData, Payload: Paint(p.Terminal)}) }
	}
	return nil, newErr("invalid_params", "hctest: method not implemented: "+method, ""), nil
}
