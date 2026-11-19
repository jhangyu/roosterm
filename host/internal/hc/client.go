package hc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"
	"time"
)

const (
	protocolGeneration = 1
	jsonrpcVersion     = "2.0"
)

var requiredFeatures = []string{"attach", "events"}

// Client is one Unix connection to hc: calls on channel 0, attach streams on other channels.
type Client struct {
	conn net.Conn
	r    *bufio.Reader
	path string

	wmu sync.Mutex // frames must not interleave

	mu      sync.Mutex
	closed  bool
	nextID  uint64
	pending map[uint64]*pendingCall
	streams map[uint32]*Stream

	done      chan struct{}
	closeOnce sync.Once
}

type pendingCall struct {
	method   string
	terminal string // set for terminal.attach: the reader registers the stream on the reply
	attach   bool
	reply    chan callReply // buffered 1
}

type callReply struct {
	result json.RawMessage
	stream *Stream
	err    error
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint64 `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type rpcResponse struct {
	ID     *uint64         `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Kind string `json:"kind"`
			Hint string `json:"hint"`
		} `json:"data"`
	} `json:"error"`
}

// Dial connects to the hc socket and performs the plain-mode hello.
func Dial(ctx context.Context, socketPath string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("hc not running: %w; expected a listening herdr-core socket at %s (start it with \"hc server start\")", err, socketPath)
	}
	r := bufio.NewReader(conn)
	if err := hello(ctx, conn, r, socketPath); err != nil {
		// ponytail: hello already failed; the close error adds nothing.
		_ = conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, r: r, path: socketPath, pending: map[uint64]*pendingCall{},
		streams: map[uint32]*Stream{}, done: make(chan struct{})}
	go c.readLoop()
	return c, nil
}

func hello(ctx context.Context, conn net.Conn, r *bufio.Reader, path string) error {
	stop := context.AfterFunc(ctx, func() {
		// Unblocks the hello read when ctx ends; the deadline error is reported below.
		_ = conn.SetDeadline(time.Unix(1, 0))
	})
	defer stop()
	req, err := json.Marshal(rpcRequest{JSONRPC: jsonrpcVersion, ID: 0, Method: "hello",
		Params: map[string]any{"framed": true, "generation": protocolGeneration}})
	if err != nil {
		return fmt.Errorf("hc hello at %s: encode: %w", path, err)
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("hc hello at %s: write: %w", path, err)
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("hc hello at %s: read: %w", path, err)
	}
	var resp struct {
		Result *struct {
			Generation int      `json:"generation"`
			Framed     bool     `json:"framed"`
			Features   []string `json:"features"`
		} `json:"result"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("hc hello at %s: expected a JSON-RPC line, got %q: %w", path, line, err)
	}
	if resp.Result == nil {
		return fmt.Errorf("hc hello at %s: expected a result, got error %s", path, resp.Error)
	}
	if resp.Result.Generation != protocolGeneration {
		return fmt.Errorf("hc hello at %s: expected generation %d, got %d", path, protocolGeneration, resp.Result.Generation)
	}
	if !resp.Result.Framed {
		return fmt.Errorf("hc hello at %s: expected framed true, got false", path)
	}
	for _, f := range requiredFeatures {
		if !slices.Contains(resp.Result.Features, f) {
			return fmt.Errorf("hc hello at %s: expected feature %q, got features %v", path, f, resp.Result.Features)
		}
	}
	return nil
}

// Done is closed when the Unix connection has ended.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close ends the connection and waits for the reader to stop.
func (c *Client) Close() error {
	err := c.conn.Close()
	<-c.done
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

func (c *Client) readLoop() {
	c.shutdown(c.readFrames())
}

func (c *Client) readFrames() error {
	for {
		f, err := ReadFrame(c.r)
		if err != nil {
			return err
		}
		if f.Channel == rpcChannel {
			if f.Kind == KindData {
				c.routeReply(f.Payload)
			}
			continue
		}
		c.mu.Lock()
		s := c.streams[f.Channel]
		if s != nil && f.Kind == KindClose {
			delete(c.streams, f.Channel)
		}
		c.mu.Unlock()
		if s == nil {
			continue // channel already detached by us
		}
		switch f.Kind {
		case KindData:
			s.push(f.Payload)
		case KindClose:
			if len(f.Payload) == 0 {
				s.end(ErrDetached)
				continue
			}
			var e Exit
			if err := json.Unmarshal(f.Payload, &e); err != nil {
				s.end(fmt.Errorf("hc stream %s: close reason %q: %w", s.terminal, f.Payload, err))
				continue
			}
			s.end(&e)
		}
	}
}

func (c *Client) routeReply(payload []byte) {
	var resp rpcResponse
	if err := json.Unmarshal(payload, &resp); err != nil || resp.ID == nil {
		return // not a reply (notification) or not JSON: nothing waits for it
	}
	c.mu.Lock()
	pc := c.pending[*resp.ID]
	delete(c.pending, *resp.ID)
	if pc == nil {
		c.mu.Unlock()
		return // caller gave up (ctx done)
	}
	rep := callReply{result: resp.Result}
	switch {
	case resp.Error != nil:
		rep.err = &RPCError{Method: pc.method, Code: resp.Error.Code, Message: resp.Error.Message,
			Kind: resp.Error.Data.Kind, Hint: resp.Error.Data.Hint}
	case pc.attach:
		var a struct {
			Channel uint32 `json:"channel"`
		}
		if err := json.Unmarshal(resp.Result, &a); err != nil || a.Channel == rpcChannel {
			rep.err = fmt.Errorf("hc %s %s: expected result with a non-zero channel, got %s", pc.method, pc.terminal, resp.Result)
			break
		}
		// Registered before the reader reads the next frame, so the first paint is not lost.
		rep.stream = newStream(c, a.Channel, pc.terminal)
		c.streams[a.Channel] = rep.stream
	}
	c.mu.Unlock()
	pc.reply <- rep
}

func (c *Client) shutdown(cause error) {
	c.closeOnce.Do(func() {
		gone := fmt.Errorf("%w (%s: %v)", ErrCoreGone, c.path, cause)
		c.mu.Lock()
		c.closed = true
		pending, streams := c.pending, c.streams
		c.pending, c.streams = map[uint64]*pendingCall{}, map[uint32]*Stream{}
		c.mu.Unlock()
		// ponytail: the reader already failed; closing is cleanup only.
		_ = c.conn.Close()
		close(c.done) // before waking callers, so Supervisor.Client sees offline when they retry
		for _, pc := range pending {
			pc.reply <- callReply{err: fmt.Errorf("hc %s: %w", pc.method, gone)}
		}
		for _, s := range streams {
			s.end(gone)
		}
	})
}

func (c *Client) writeFrame(f Frame) error {
	c.wmu.Lock()
	err := WriteFrame(c.conn, f)
	c.wmu.Unlock()
	if err != nil {
		// A failed write means the connection is unusable; closing it lets the reader end every stream.
		_ = c.conn.Close()
		return fmt.Errorf("hc write channel %d: %w (%v)", f.Channel, ErrCoreGone, err)
	}
	return nil
}

// Call sends one JSON-RPC request and decodes the result into result (nil: ignore result).
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	_, err := c.call(ctx, method, "", false, params, result)
	return err
}

func (c *Client) call(ctx context.Context, method, terminal string, attach bool, params, result any) (*Stream, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("hc %s: %w", method, ErrCoreGone)
	}
	c.nextID++
	id := c.nextID
	pc := &pendingCall{method: method, terminal: terminal, attach: attach, reply: make(chan callReply, 1)}
	c.pending[id] = pc
	c.mu.Unlock()

	msg, err := json.Marshal(rpcRequest{JSONRPC: jsonrpcVersion, ID: id, Method: method, Params: params})
	if err != nil {
		c.forget(id)
		return nil, fmt.Errorf("hc %s: encode params: %w", method, err)
	}
	if err := c.writeFrame(Frame{Channel: rpcChannel, Kind: KindData, Payload: msg}); err != nil {
		c.forget(id)
		return nil, fmt.Errorf("hc %s: %w", method, err)
	}
	select {
	case rep := <-pc.reply:
		if rep.err != nil {
			return nil, rep.err
		}
		if result != nil {
			if err := json.Unmarshal(rep.result, result); err != nil {
				return nil, fmt.Errorf("hc %s: decode result %s: %w", method, rep.result, err)
			}
		}
		return rep.stream, nil
	case <-ctx.Done():
		// ponytail: an attach reply arriving after this is dropped and its channel stays open on the
		// hc side until the connection ends; only happens when the caller's ctx ends mid-attach.
		c.forget(id)
		return nil, fmt.Errorf("hc %s: %w", method, ctx.Err())
	}
}

func (c *Client) forget(id uint64) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

type terminalRef struct {
	Terminal string `json:"terminal"`
}

func (c *Client) Run(ctx context.Context, p RunParams) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.run", p, &t)
	return t, err
}

// List returns live and finished terminals carrying all given labels.
func (c *Client) List(ctx context.Context, labels map[string]string) ([]Terminal, error) {
	var res struct {
		Terminals []Terminal `json:"terminals"`
	}
	err := c.Call(ctx, "terminal.list", map[string]any{"all": true, "labels": labels}, &res)
	if err != nil {
		return nil, err
	}
	if res.Terminals == nil {
		res.Terminals = []Terminal{}
	}
	return res.Terminals, nil
}

func (c *Client) Get(ctx context.Context, id string) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.get", terminalRef{id}, &t)
	return t, err
}

// Label merges labels; a value "" removes that key.
func (c *Client) Label(ctx context.Context, id string, labels map[string]string) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.label", map[string]any{"terminal": id, "labels": labels}, &t)
	return t, err
}

func (c *Client) Kill(ctx context.Context, id string) (Terminal, error) {
	var t Terminal
	err := c.Call(ctx, "terminal.kill", terminalRef{id}, &t)
	return t, err
}

func (c *Client) Remove(ctx context.Context, id string) error {
	return c.Call(ctx, "terminal.rm", terminalRef{id}, nil)
}

// attachHistoryLines is how much main-screen scrollback the first paint replays; it equals the
// web terminal's scrollback (SCROLLBACK_LINES in web/src/terminal/TerminalPane.tsx), which trims the rest.
const attachHistoryLines = 5000

// Attach opens a viewer stream; the first Recv returns the first paint: up to attachHistoryLines
// of scrollback, then the full screen.
func (c *Client) Attach(ctx context.Context, id string, size Size) (*Stream, error) {
	return c.call(ctx, "terminal.attach", id, true,
		map[string]any{"terminal": id, "cols": size.Cols, "rows": size.Rows, "history": attachHistoryLines}, nil)
}
