package hc

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Supervisor keeps one Client and re-dials when the socket file changes while offline.
type Supervisor struct {
	path    string
	watcher *fsnotify.Watcher
	cancel  context.CancelFunc
	stopped chan struct{}

	mu     sync.Mutex
	cur    *Client       // nil while offline
	online chan struct{} // closed while online; replaced on each transition to offline
}

// Start dials once (failure is returned: hc not running) and then watches the socket directory.
// ponytail: dial before watch; a core that dies and re-binds inside that microsecond window is
// noticed only on the next event for the socket name.
func Start(ctx context.Context, socketPath string) (*Supervisor, error) {
	c, err := Dial(ctx, socketPath)
	if err != nil {
		return nil, err
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("hc supervisor %s: new watcher: %w", socketPath, err), c.Close())
	}
	dir := filepath.Dir(socketPath)
	if err := w.Add(dir); err != nil {
		return nil, errors.Join(fmt.Errorf("hc supervisor %s: watch %s: %w", socketPath, dir, err), w.Close(), c.Close())
	}
	online := make(chan struct{})
	close(online)
	lctx, cancel := context.WithCancel(ctx)
	s := &Supervisor{path: socketPath, watcher: w, cancel: cancel, stopped: make(chan struct{}), cur: c, online: online}
	go s.loop(lctx)
	return s, nil
}

// currentLocked returns the live client or nil, flipping to offline when the client's connection ended.
func (s *Supervisor) currentLocked() *Client {
	if s.cur != nil && isClosed(s.cur.Done()) {
		s.cur = nil
		s.online = make(chan struct{})
	}
	return s.cur
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// Client returns the current client, or ErrCoreOffline.
func (s *Supervisor) Client() (*Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.currentLocked(); c != nil {
		return c, nil
	}
	return nil, fmt.Errorf("hc %s: %w", s.path, ErrCoreOffline)
}

// Await blocks until a client is online or ctx ends.
func (s *Supervisor) Await(ctx context.Context) (*Client, error) {
	for {
		s.mu.Lock()
		c := s.currentLocked()
		ch := s.online
		s.mu.Unlock()
		if c != nil {
			return c, nil
		}
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func matchesSocket(ev fsnotify.Event, socketPath string) bool {
	return filepath.Clean(ev.Name) == filepath.Clean(socketPath)
}

func (s *Supervisor) loop(ctx context.Context) {
	defer close(s.stopped)
	for {
		s.mu.Lock()
		var done <-chan struct{}
		if c := s.currentLocked(); c != nil {
			done = c.Done()
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-done:
			// next iteration's currentLocked flips to offline
		case ev, ok := <-s.watcher.Events:
			if !ok {
				return
			}
			if matchesSocket(ev, s.path) {
				s.redial(ctx)
			}
		case err, ok := <-s.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("hc watch %s: %v", filepath.Dir(s.path), err)
		}
	}
}

// redial dials once if offline. Events while online are ignored (macOS repeats CREATE).
func (s *Supervisor) redial(ctx context.Context) {
	s.mu.Lock()
	c := s.currentLocked()
	s.mu.Unlock()
	if c != nil {
		return
	}
	nc, err := Dial(ctx, s.path)
	if err != nil {
		log.Printf("hc reconnect: %v", err)
		return
	}
	s.mu.Lock()
	s.cur = nc
	close(s.online)
	s.mu.Unlock()
}

// Close stops watching and closes the current client.
func (s *Supervisor) Close() error {
	s.cancel()
	<-s.stopped
	err := s.watcher.Close()
	s.mu.Lock()
	c := s.cur
	s.cur = nil
	s.online = make(chan struct{}) // Await callers wait for their ctx from now on
	s.mu.Unlock()
	if c != nil {
		err = errors.Join(err, c.Close())
	}
	return err
}
