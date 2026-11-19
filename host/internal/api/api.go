// Package api serves the REST tab and swarm endpoints (plan section "HTTP/WS API").
package api

import (
	"errors"
	"fmt"
	"net/http"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/hc"
)

type Options struct {
	Core     *hc.Supervisor
	Shell    string // argv[0] of a default tab, run with "-l"
	Home     string // cwd of every tab
	SwarmDir string // tmux socket directory
}

type Tab struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Exited   bool    `json:"exited"`
	ExitCode *int    `json:"exitCode"`
	Signal   *int    `json:"signal"`
	Reason   *string `json:"reason"`
}

type server struct{ o Options }

func New(o Options) http.Handler {
	s := &server{o: o}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tabs", s.listTabs)
	mux.HandleFunc("POST /api/tabs", s.createTab)
	mux.HandleFunc("PATCH /api/tabs/{id}", s.renameTab)
	mux.HandleFunc("DELETE /api/tabs/{id}", s.closeTab)
	mux.HandleFunc("GET /api/swarms", s.listSwarms)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		apierr.Write(w, &apierr.Error{Status: http.StatusNotFound, Op: "api", Kind: apierr.KindNotFound,
			Message: fmt.Sprintf("no endpoint %s %s", r.Method, r.URL.Path)})
	})
	return mux
}

func toTab(t hc.Terminal) Tab {
	return Tab{ID: t.Address, Label: t.Labels[hc.LabelName], Exited: t.State == hc.StateFinished,
		ExitCode: t.ExitCode, Signal: t.Signal, Reason: t.Reason}
}

// hcError maps an hc failure to the API error table.
func hcError(op, tab string, err error) *apierr.Error {
	if errors.Is(err, hc.ErrCoreOffline) || errors.Is(err, hc.ErrCoreGone) {
		return &apierr.Error{Status: http.StatusServiceUnavailable, Op: op, Tab: tab, Kind: apierr.KindCoreOffline, Message: err.Error()}
	}
	var re *hc.RPCError
	if errors.As(err, &re) {
		msg := re.Method
		if tab != "" {
			msg += " " + tab
		}
		msg += ": " + re.Message
		status := http.StatusBadGateway
		if re.Kind == hc.KindNotFound {
			status = http.StatusNotFound
		}
		return &apierr.Error{Status: status, Op: op, Tab: tab, Kind: re.Kind, Hint: re.Hint, Message: msg}
	}
	return &apierr.Error{Status: http.StatusInternalServerError, Op: op, Tab: tab, Kind: apierr.KindInternal, Message: err.Error()}
}
