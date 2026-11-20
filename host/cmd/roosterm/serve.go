package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"roosterm/host/internal/api"
	"roosterm/host/internal/auth"
	"roosterm/host/internal/bridge"
	"roosterm/host/internal/config"
	"roosterm/host/internal/hc"
	"roosterm/host/internal/swarm"
	"roosterm/host/internal/web"
)

const (
	shutdownGrace     = 5 * time.Second  // fail-fast cap on graceful HTTP shutdown
	readHeaderTimeout = 10 * time.Second // slow-header (slowloris) cap
)

func serve(args []string, stdout, stderr io.Writer) int {
	path, code, ok := configPath("serve", args, stderr)
	if !ok {
		return code
	}
	if err := serveConfig(path, stdout); err != nil {
		fmt.Fprintf(stderr, "roosterm serve: %v\n", err)
		return exitError
	}
	return exitOK
}

// serveConfig is the composition root: the only place that reads the real clock.
func serveConfig(path string, stdout io.Writer) error {
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if _, err := auth.VerifyPassword(cfg.PasswordHash, ""); err != nil {
		return fmt.Errorf("config %s: password_hash: %w", path, err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home directory: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	socket := cfg.SocketPath(home)
	sup, err := hc.Start(ctx, socket)
	if err != nil {
		return err
	}
	sessions, err := auth.OpenSessions(config.SessionsPath(path), cfg.SessionTTL, time.Now)
	if err != nil {
		return errors.Join(err, sup.Close())
	}
	h := web.New(web.Options{
		Username: cfg.Username, PasswordHash: cfg.PasswordHash, SessionTTL: cfg.SessionTTL,
		Sessions: sessions, Limiter: auth.NewLimiter(time.Now), DeviceLimiter: auth.NewLimiter(time.Now),
		API:       api.New(api.Options{Core: sup, Shell: cfg.Shell, Home: home, SwarmDir: swarm.Dir(os.Getenv, os.Getuid())}),
		TabSocket: bridge.New(sup, cfg.PingInterval),
	})
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return errors.Join(fmt.Errorf("listen %s: %w", cfg.Listen, err), sup.Close())
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: readHeaderTimeout,
		BaseContext: func(net.Listener) context.Context { return ctx }}
	fmt.Fprintf(stdout, "listening on %s, hc %s\n", ln.Addr(), socket)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return errors.Join(fmt.Errorf("http serve: %w", err), sup.Close())
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	return errors.Join(srv.Shutdown(sctx), sup.Close())
}
