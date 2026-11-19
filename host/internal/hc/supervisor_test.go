package hc_test

import (
	"bytes"
	"errors"
	"testing"

	"roosterm/host/internal/hc"
	"roosterm/host/internal/hc/hctest"
)

func TestSupervisorOfflineThenReconnect(t *testing.T) {
	t.Parallel()
	f := hctest.Start(t)
	ctx := t.Context()
	sup, err := hc.Start(ctx, f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sup.Close(); err != nil {
			t.Errorf("supervisor close: %v", err)
		}
	})
	c1, err := sup.Client()
	if err != nil {
		t.Fatal(err)
	}
	term, err := c1.Run(ctx, tabParams())
	if err != nil {
		t.Fatal(err)
	}
	f.Stop()
	<-c1.Done()
	if _, err := sup.Client(); !errors.Is(err, hc.ErrCoreOffline) {
		t.Fatalf("Client while core down: expected ErrCoreOffline, got %v", err)
	}
	f.Restart(t)
	c2, err := sup.Await(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if c2 == c1 {
		t.Fatal("Await: expected a new client after restart")
	}
	st, err := c2.Attach(ctx, term.Address, hc.Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if p, err := st.Recv(ctx); err != nil || !bytes.Equal(p, hctest.Paint(term.Address)) {
		t.Fatalf("re-attach first paint: got %q, %v", p, err)
	}
}
