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

func checkLabel(op, tab, label string) *apierr.Error {
	n := utf8.RuneCountInString(label)
	ok := n >= 1 && n <= maxLabelLen
	for _, r := range label {
		if unicode.IsControl(r) {
			ok = false
		}
	}
	if !ok {
		return apierr.BadRequest(op, tab, fmt.Sprintf("label: expected 1..%d characters without control characters, got %q", maxLabelLen, label))
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
		return hc.Terminal{}, apierr.NotFound(op, id, fmt.Sprintf("tab %s not found", id))
	}
	return t, nil
}

func (s *server) listTabs(w http.ResponseWriter, r *http.Request) *apierr.Error {
	const op = "tab.list"
	c, e := s.client(op, "")
	if e != nil {
		return e
	}
	ts, err := c.List(r.Context(), map[string]string{hc.LabelOwner: hc.LabelOwnerTab})
	if err != nil {
		return hcError(op, "", err)
	}
	slices.SortStableFunc(ts, func(a, b hc.Terminal) int { return cmp.Compare(a.CreatedMS, b.CreatedMS) })
	home, e := s.homeDir(op)
	if e != nil {
		return e
	}
	out := make([]Tab, 0, len(ts))
	for _, t := range ts {
		out = append(out, toTab(home, t))
	}
	apierr.WriteJSON(w, http.StatusOK, out)
	return nil
}

type createRequest struct {
	Command []string `json:"command"`
	Label   *string  `json:"label"`
	Cwd     string   `json:"cwd"`   // relative to home
	Swarm   *string  `json:"swarm"` // claude-swarm-* socket name; excludes command and cwd
	Pane    *paneRef `json:"pane"`  // existing Claude Code tmux pane; excludes command, cwd and swarm
}

type paneRef struct {
	Socket string `json:"socket"`
	ID     string `json:"id"`
}

func (s *server) createTab(w http.ResponseWriter, r *http.Request) *apierr.Error {
	const op = "tab.create"
	var req createRequest
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		return e
	}
	argv := []string{s.o.Shell, "-l"}
	if req.Pane != nil {
		if req.Command != nil || req.Cwd != "" || req.Swarm != nil {
			return apierr.BadRequest(op, "", `pane: expected no "command", "cwd" or "swarm" together with "pane", got more than one`)
		}
		if !swarm.IsSocketName(req.Pane.Socket) || !swarm.IsPaneID(req.Pane.ID) {
			return apierr.BadRequest(op, "", fmt.Sprintf(`pane: expected a non-swarm socket name without "/" and a pane id like "%%3", got socket %q id %q`, req.Pane.Socket, req.Pane.ID))
		}
		argv = swarm.PaneAttachArgv(s.o.SwarmDir, req.Pane.Socket, req.Pane.ID)
	} else if req.Swarm != nil {
		if req.Command != nil || req.Cwd != "" {
			return apierr.BadRequest(op, "", `swarm: expected no "command" or "cwd" together with "swarm", got both`)
		}
		if !swarm.IsName(*req.Swarm) {
			return apierr.BadRequest(op, "", fmt.Sprintf(`swarm: expected a socket name starting with %q and without "/", got %q`, swarm.Prefix, *req.Swarm))
		}
		var err error
		if argv, err = swarm.AttachArgv(r.Context(), s.o.SwarmDir, *req.Swarm); err != nil {
			return apierr.Internal(op, "", err.Error())
		}
	} else if req.Command != nil {
		argv = req.Command
		if len(argv) == 0 {
			return apierr.BadRequest(op, "", "command: expected a non-empty array of non-empty strings, got []")
		}
		for i, a := range argv {
			if a == "" {
				return apierr.BadRequest(op, "", fmt.Sprintf("command[%d]: expected a non-empty string, got \"\"", i))
			}
		}
	}
	cwd, home, e := s.resolveDir(op, req.Cwd)
	if e != nil {
		return e
	}
	label := "~"
	if cwd != home {
		label = filepath.Base(cwd)
	}
	if req.Swarm != nil {
		label = *req.Swarm
	}
	if req.Pane != nil {
		label = req.Pane.Socket + ":" + req.Pane.ID
	}
	if req.Label != nil {
		label = *req.Label
	}
	if e := checkLabel(op, "", label); e != nil {
		return e
	}
	c, e := s.client(op, "")
	if e != nil {
		return e
	}
	t, err := c.Run(r.Context(), hc.RunParams{Argv: ScrubArgv(argv), Cwd: cwd, Size: initialSize, NoParent: true,
		Labels: map[string]string{hc.LabelOwner: hc.LabelOwnerTab, hc.LabelName: label}})
	if err != nil {
		return hcError(op, "", err)
	}
	apierr.WriteJSON(w, http.StatusCreated, toTab(home, t))
	return nil
}

// updateTab renames and/or hides a tab; hidden only drops it from clients' tab strips, the process keeps running.
func (s *server) updateTab(w http.ResponseWriter, r *http.Request) *apierr.Error {
	const op = "tab.update"
	id := r.PathValue("id")
	var req struct {
		Label  *string `json:"label"`
		Hidden *bool   `json:"hidden"`
	}
	if e := apierr.DecodeJSON(w, r, op, &req); e != nil {
		e.Tab = id
		return e
	}
	labels := map[string]string{}
	if req.Label != nil {
		if e := checkLabel(op, id, *req.Label); e != nil {
			return e
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
		return apierr.BadRequest(op, id, `body: expected at least one of "label" (string) or "hidden" (boolean), got neither`)
	}
	c, e := s.client(op, id)
	if e != nil {
		return e
	}
	if _, e := owned(r.Context(), c, op, id); e != nil {
		return e
	}
	t, err := c.Label(r.Context(), id, labels)
	if err != nil {
		return hcError(op, id, err)
	}
	home, e := s.homeDir(op)
	if e != nil {
		return e
	}
	apierr.WriteJSON(w, http.StatusOK, toTab(home, t))
	return nil
}

func (s *server) closeTab(w http.ResponseWriter, r *http.Request) *apierr.Error {
	const op = "tab.close"
	id := r.PathValue("id")
	c, e := s.client(op, id)
	if e != nil {
		return e
	}
	t, e := owned(r.Context(), c, op, id)
	if e != nil {
		return e
	}
	if t.State != hc.StateFinished {
		if _, err := c.Kill(r.Context(), id); err != nil {
			return hcError(op, id, err)
		}
	}
	if err := c.Remove(r.Context(), id); err != nil {
		return hcError(op, id, err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
