// Package hc speaks herdr-core's framed JSON-RPC over its Unix socket (docs/hc-bridge.md).
package hc

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Frame kinds (docs/hc-bridge.md §3).
const (
	KindData   byte = 0
	KindCredit byte = 1
	KindClose  byte = 2
	KindResize byte = 3
)

const (
	frameHeaderLen         = 9
	maxFramePayload        = 16 << 20
	rpcChannel      uint32 = 0
)

// Frame is [channel u32 LE][kind u8][len u32 LE][payload].
type Frame struct {
	Channel uint32
	Kind    byte
	Payload []byte
}

func WriteFrame(w io.Writer, f Frame) error {
	buf := make([]byte, frameHeaderLen+len(f.Payload))
	binary.LittleEndian.PutUint32(buf[0:4], f.Channel)
	buf[4] = f.Kind
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(f.Payload)))
	copy(buf[frameHeaderLen:], f.Payload)
	_, err := w.Write(buf)
	return err
}

func ReadFrame(r io.Reader) (Frame, error) {
	var h [frameHeaderLen]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return Frame{}, err
	}
	f := Frame{Channel: binary.LittleEndian.Uint32(h[0:4]), Kind: h[4]}
	n := binary.LittleEndian.Uint32(h[5:9])
	if n > maxFramePayload {
		return Frame{}, fmt.Errorf("hc frame on channel %d: expected payload <= %d bytes, got %d", f.Channel, maxFramePayload, n)
	}
	f.Payload = make([]byte, n)
	if _, err := io.ReadFull(r, f.Payload); err != nil {
		return Frame{}, fmt.Errorf("hc frame on channel %d: payload: %w", f.Channel, err)
	}
	return f, nil
}

// resizePayload is u16 LE cols, u16 LE rows.
func resizePayload(s Size) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint16(b[0:2], s.Cols)
	binary.LittleEndian.PutUint16(b[2:4], s.Rows)
	return b
}

// ParseResize decodes a RESIZE payload (used by the fake core).
func ParseResize(p []byte) (Size, error) {
	if len(p) != 4 {
		return Size{}, fmt.Errorf("hc resize payload: expected 4 bytes, got %d", len(p))
	}
	return Size{Cols: binary.LittleEndian.Uint16(p[0:2]), Rows: binary.LittleEndian.Uint16(p[2:4])}, nil
}
