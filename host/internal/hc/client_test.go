package hc_test

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

func tabParams() hc.RunParams {
	return hc.RunParams{Argv: []string{"/bin/sh"}, Cwd: "/", Size: hc.Size{Cols: 80, Rows: 24},
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: "sh"}, NoParent: true}
}

func dialFake(t *testing.T) (*hctest.Fake, *hc.Client) {
	t.Helper()
	f := hctest.Start(t)
	c, err := hc.Dial(t.Context(), f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Errorf("client close: %v", err)
		}
	})
	return f, c
}

func TestFrameRoundTrip(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	in := hc.Frame{Channel: 7, Kind: hc.KindResize, Payload: []byte{1, 2, 3, 4}}
	if err := hc.WriteFrame(&buf, in); err != nil {
		t.Fatal(err)
	}
	want := []byte{7, 0, 0, 0, 3, 4, 0, 0, 0, 1, 2, 3, 4}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("wire bytes: expected %v, got %v", want, buf.Bytes())
	}
	out, err := hc.ReadFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if out.Channel != 7 || out.Kind != hc.KindResize || !bytes.Equal(out.Payload, in.Payload) {
		t.Fatalf("round trip: expected %+v, got %+v", in, out)
	}
}

func TestDialMissingSocketNamesPath(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "none.sock")
	_, err = hc.Dial(t.Context(), path)
	if err == nil {
		t.Fatal("Dial missing socket: expected error, got nil")
	}
	for _, want := range []string{path, "hc server start"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Dial error %q: missing %q", err, want)
		}
	}
	if _, err := hc.Start(t.Context(), path); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("Start missing socket: expected error naming %s, got %v", path, err)
	}
}

func TestDialRejectsWrongGeneration(t *testing.T) {
	t.Parallel()
	dir, err := os.MkdirTemp("", "rt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "g.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if _, err := bufio.NewReader(conn).ReadBytes('\n'); err != nil {
			return
		}
		conn.Write([]byte(`{"jsonrpc":"2.0","id":0,"result":{"generation":2,"framed":true,"features":["attach","events"]}}` + "\n"))
	}()
	_, err = hc.Dial(t.Context(), path)
	if err == nil || !strings.Contains(err.Error(), "expected generation 1, got 2") {
		t.Fatalf("Dial: expected generation error, got %v", err)
	}
}

func TestCallReturnsRPCError(t *testing.T) {
	t.Parallel()
	_, c := dialFake(t)
	_, err := c.Get(t.Context(), "t999")
	var re *hc.RPCError
	if !errors.As(err, &re) {
		t.Fatalf("Get unknown: expected *hc.RPCError, got %v", err)
	}
	if re.Method != "terminal.get" || re.Code != -32000 || re.Kind != hc.KindNotFound || re.Message != `no terminal "t999"` || re.Hint == "" {
		t.Fatalf("RPCError fields: got %+v", re)
	}
}

func TestAttachFirstPaintThenEcho(t *testing.T) {
	t.Parallel()
	f, c := dialFake(t)
	ctx := t.Context()
	term, err := c.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, term.Address, hc.Size{Cols: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.Recv(ctx)
	if err != nil || !bytes.Equal(p, hctest.Paint(term.Address)) {
		t.Fatalf("first Recv: expected paint, got %q, %v", p, err)
	}
	if err := st.Credit(len(p)); err != nil {
		t.Fatal(err)
	}
	if err := st.Write([]byte("ls\r")); err != nil {
		t.Fatal(err)
	}
	if got := <-f.Input(term.Address); string(got) != "ls\r" {
		t.Fatalf("fake input: expected ls\\r, got %q", got)
	}
	if p, err := st.Recv(ctx); err != nil || string(p) != "ls\r" {
		t.Fatalf("echo: expected ls\\r, got %q, %v", p, err)
	}
	if err := st.Resize(hc.Size{Cols: 132, Rows: 40}); err != nil {
		t.Fatal(err)
	}
	if got := <-f.Resized(term.Address); got != (hc.Size{Cols: 132, Rows: 40}) {
		t.Fatalf("fake resize: got %+v", got)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Recv(ctx); !errors.Is(err, hc.ErrDetached) {
		t.Fatalf("after Close: expected ErrDetached, got %v", err)
	}
}

func TestStreamExitDeliversCode(t *testing.T) {
	t.Parallel()
	f, c := dialFake(t)
	ctx := t.Context()
	term, err := c.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Recv(ctx); err != nil {
		t.Fatal(err)
	}
	f.Exit(term.Address, 7)
	_, err = st.Recv(ctx)
	var ex *hc.Exit
	if !errors.As(err, &ex) || ex.Reason != "exited" || ex.ExitCode == nil || *ex.ExitCode != 7 || ex.Signal != nil {
		t.Fatalf("Recv after exit: expected *hc.Exit{exited, 7, nil}, got %v", err)
	}
	_, err = c.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	var re *hc.RPCError
	if !errors.As(err, &re) || re.Kind != hc.KindFinished {
		t.Fatalf("Attach finished: expected kind finished, got %v", err)
	}
}

func TestStreamEndsWhenCoreGone(t *testing.T) {
	t.Parallel()
	f, c := dialFake(t)
	ctx := t.Context()
	term, err := c.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	st, err := c.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Recv(ctx); err != nil {
		t.Fatal(err)
	}
	f.Stop()
	if _, err := st.Recv(ctx); !errors.Is(err, hc.ErrCoreGone) {
		t.Fatalf("Recv after core stop: expected ErrCoreGone, got %v", err)
	}
	<-c.Done()
	if _, err := c.Get(ctx, term.Address); !errors.Is(err, hc.ErrCoreGone) {
		t.Fatalf("Get after core stop: expected ErrCoreGone, got %v", err)
	}
}
