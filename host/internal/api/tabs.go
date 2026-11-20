package api

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"unicode"
	"unicode/utf8"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/hc"
	"roosterm/host/internal/swarm"
)

const maxLabelLen = 64

var initialSize = hc.Size{Cols: 80, Rows: 24}

func badRequest(op, tab, msg string) *apierr.Error {
	return &apierr.Error{Status: http.StatusBadRequest, Op: op, Tab: tab, Kind: apierr.KindBadRequest, Message: msg}
}

func checkLabel(op, tab, label string) *apierr.Error {
	n := utf8.RuneCountInString(label)
	ok := n >= 1 && n <= maxLabelLen
	for _, r := range label {
		if unicode.IsControl(r) {
			ok = false
		}
	}
	if !ok {
		return badRequest(op, tab, fmt.Sprintf("label: expected 1..%d characters without control characters, got %q", maxLabelLen, label))
	}
	return nil
}

func (s *server) client(op, tab string) (*hc.Client, *apierr.Error) {
	c, err := s.o.Core.Client()
	if err != nil {
		return nil, hcError(op, tab, err)
	}
	return c, nil
}

// owned returns the terminal if it exists and carries the roosterm tab label; anything else is 404.
func owned(ctx context.Context, c *hc.Client, op, id string) (hc.Terminal, *apierr.Error) {
	t, err := c.Get(ctx, id)
	if err != nil {
		return hc.Terminal{}, hcError(op, id, err)
	}
	if !hc.IsTab(t) {
		return hc.Terminal{}, &apierr.Error{Status: http.StatusNotFound, Op: op, Tab: id, Kind: apierr.KindNotFound,
			Message: fmt.Sprintf("tab %s not found", id)}
	}
	return t, nil
}

func (s *server) listTabs(w http.ResponseWriter, r *http.Request) {
	const op = "tab.list"
	c, e := s.client(op, "")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	ts, err := c.List(r.Context(), map[string]string{hc.LabelOwner: hc.LabelOwnerTab})
	if err != nil {
		apierr.Write(w, hcError(op, "", err))
		return
	}
	slices.SortStableFunc(ts, func(a, b hc.Terminal) int { return cmp.Compare(a.CreatedMS, b.CreatedMS) })
	home, e := s.homeDir(op)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	out := make([]Tab, 0, len(ts))
	for _, t := range ts {
		out = append(out, toTab(home, t))
	}
	apierr.WriteJSON(w, http.StatusOK, out)
}

type createRequest struct {
	Command []string `json:"command"`
	Label   *string  `json:"label"`
	Cwd     string   `json:"cwd"` // relative to home
}

func (s *server) createTab(w http.ResponseWriter, r *http.Request) {
	const op = "tab.create"
	var req createRequest
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		apierr.Write(w, e)
		return
	}
	argv := []string{s.o.Shell, "-l"}
	if req.Command != nil {
		argv = req.Command
		if len(argv) == 0 {
			apierr.Write(w, badRequest(op, "", "command: expected a non-empty array of non-empty strings, got []"))
			return
		}
		for i, a := range argv {
			if a == "" {
				apierr.Write(w, badRequest(op, "", fmt.Sprintf("command[%d]: expected a non-empty string, got \"\"", i)))
				return
			}
		}
	}
	if name, ok := swarm.IsAttachArgv(argv); ok {
		var err error
		if argv, err = swarm.AttachArgv(s.o.SwarmDir, name); err != nil {
			apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: op, Kind: apierr.KindInternal, Message: err.Error()})
			return
		}
	}
	cwd, e := s.resolveDir(op, req.Cwd)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	home, e := s.homeDir(op)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	label := "~"
	if cwd != home {
		label = filepath.Base(cwd)
	}
	if req.Label != nil {
		label = *req.Label
	}
	if e := checkLabel(op, "", label); e != nil {
		apierr.Write(w, e)
		return
	}
	c, e := s.client(op, "")
	if e != nil {
		apierr.Write(w, e)
		return
	}
	t, err := c.Run(r.Context(), hc.RunParams{Argv: ScrubArgv(argv), Cwd: cwd, Size: initialSize, NoParent: true,
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: label}})
	if err != nil {
		apierr.Write(w, hcError(op, "", err))
		return
	}
	apierr.WriteJSON(w, http.StatusCreated, toTab(home, t))
}

// updateTab renames and/or hides a tab; hidden only drops it from clients' tab strips, the process keeps running.
func (s *server) updateTab(w http.ResponseWriter, r *http.Request) {
	const op = "tab.update"
	id := r.PathValue("id")
	var req struct {
		Label  *string `json:"label"`
		Hidden *bool   `json:"hidden"`
	}
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		e.Tab = id
		apierr.Write(w, e)
		return
	}
	labels := map[string]string{}
	if req.Label != nil {
		if e := checkLabel(op, id, *req.Label); e != nil {
			apierr.Write(w, e)
			return
		}
		labels[hc.LabelName] = *req.Label
		labels[hc.LabelRenamed] = hc.LabelRenamedOn
	}
	if req.Hidden != nil {
		labels[hc.LabelHidden] = "" // hc removes a label set to ""
		if *req.Hidden {
			labels[hc.LabelHidden] = hc.LabelHiddenOn
		}
	}
	if len(labels) == 0 {
		apierr.Write(w, badRequest(op, id, `body: expected at least one of "label" (string) or "hidden" (boolean), got neither`))
		return
	}
	c, e := s.client(op, id)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	if _, e := owned(r.Context(), c, op, id); e != nil {
		apierr.Write(w, e)
		return
	}
	t, err := c.Label(r.Context(), id, labels)
	if err != nil {
		apierr.Write(w, hcError(op, id, err))
		return
	}
	home, e := s.homeDir(op)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	apierr.WriteJSON(w, http.StatusOK, toTab(home, t))
}

func (s *server) closeTab(w http.ResponseWriter, r *http.Request) {
	const op = "tab.close"
	id := r.PathValue("id")
	c, e := s.client(op, id)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	t, e := owned(r.Context(), c, op, id)
	if e != nil {
		apierr.Write(w, e)
		return
	}
	if t.State != hc.StateFinished {
		if _, err := c.Kill(r.Context(), id); err != nil {
			apierr.Write(w, hcError(op, id, err))
			return
		}
	}
	if err := c.Remove(r.Context(), id); err != nil {
		apierr.Write(w, hcError(op, id, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
