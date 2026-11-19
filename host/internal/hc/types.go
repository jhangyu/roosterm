package hc

import (
	"errors"
	"fmt"
)

type Size struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// Terminal is the hc terminal record (fields roosterm reads).
type Terminal struct {
	Address   string            `json:"address"`
	Argv      []string          `json:"argv"`
	Labels    map[string]string `json:"labels"`
	State     string            `json:"state"`
	Reason    *string           `json:"reason"`
	ExitCode  *int              `json:"exit_code"`
	Signal    *int              `json:"signal"`
	PID       *int              `json:"pid"`
	Cwd       string            `json:"cwd"`
	CreatedMS int64             `json:"created_ms"`
	Cols      uint16            `json:"cols"`
	Rows      uint16            `json:"rows"`
}

const (
	StateLive     = "live"
	StateFinished = "finished"
)

// Tab ownership labels.
const (
	LabelOwner    = "roosterm"
	LabelOwnerTab = "tab"
	LabelName     = "roosterm.name"
)

// hc error kinds roosterm branches on.
const (
	KindNotFound = "not_found"
	KindFinished = "finished"
)

func IsTab(t Terminal) bool { return t.Labels[LabelOwner] == LabelOwnerTab }

type RunParams struct {
	Argv     []string          `json:"argv"`
	Cwd      string            `json:"cwd"`
	Size     Size              `json:"size"`
	Labels   map[string]string `json:"labels"`
	NoParent bool              `json:"no_parent"`
}

// RPCError is a JSON-RPC error returned by hc.
type RPCError struct {
	Method  string
	Code    int
	Message string
	Kind    string
	Hint    string
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("hc %s: %s (kind %s)", e.Method, e.Message, e.Kind)
}

// Exit is the CLOSE reason hc sends when a program ends.
type Exit struct {
	Reason   string `json:"reason"`
	ExitCode *int   `json:"exit_code"`
	Signal   *int   `json:"signal"`
}

func (e *Exit) Error() string {
	return fmt.Sprintf("program ended: reason %s, exit code %s, signal %s", e.Reason, optInt(e.ExitCode), optInt(e.Signal))
}

func optInt(p *int) string {
	if p == nil {
		return "none"
	}
	return fmt.Sprint(*p)
}

var (
	ErrCoreGone    = errors.New("hc core connection closed")
	ErrCoreOffline = errors.New("hc core offline")
	ErrDetached    = errors.New("hc stream detached")
)
