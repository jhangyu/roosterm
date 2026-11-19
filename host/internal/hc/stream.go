package hc

import (
	"context"
	"encoding/binary"
	"sync"
)

// Stream is one attach channel.
type Stream struct {
	c        *Client
	channel  uint32
	terminal string

	mu     sync.Mutex
	queue  [][]byte // ponytail: unbounded, safe because hc stops sending at its 256 KiB credit window
	endErr error
	notify chan struct{} // buffered 1
}

func newStream(c *Client, channel uint32, terminal string) *Stream {
	return &Stream{c: c, channel: channel, terminal: terminal, notify: make(chan struct{}, 1)}
}

func (s *Stream) push(p []byte) {
	s.mu.Lock()
	s.queue = append(s.queue, p)
	s.mu.Unlock()
	s.signal()
}

func (s *Stream) end(err error) {
	s.mu.Lock()
	if s.endErr == nil {
		s.endErr = err
	}
	s.mu.Unlock()
	s.signal()
}

func (s *Stream) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

// Recv returns the next DATA payload. Queued data is delivered before the end error:
// *Exit (program ended), ErrDetached (after Close) or an error wrapping ErrCoreGone.
func (s *Stream) Recv(ctx context.Context) ([]byte, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			p := s.queue[0]
			s.queue[0] = nil
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return p, nil
		}
		if s.endErr != nil {
			err := s.endErr
			s.mu.Unlock()
			return nil, err
		}
		s.mu.Unlock()
		select {
		case <-s.notify:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// Credit tells hc that n bytes from this channel were consumed.
func (s *Stream) Credit(n int) error {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(n))
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindCredit, Payload: b})
}

// Write sends keystroke bytes as DATA.
func (s *Stream) Write(p []byte) error {
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindData, Payload: p})
}

func (s *Stream) Resize(size Size) error {
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindResize, Payload: resizePayload(size)})
}

// Close detaches (empty CLOSE). A stream that already ended needs no detach.
func (s *Stream) Close() error {
	s.mu.Lock()
	ended := s.endErr != nil
	s.mu.Unlock()
	if ended {
		return nil
	}
	return s.c.writeFrame(Frame{Channel: s.channel, Kind: KindClose})
}
