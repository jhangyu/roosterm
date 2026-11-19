package api

import (
	"net/http"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/swarm"
)

func (s *server) listSwarms(w http.ResponseWriter, r *http.Request) {
	names, err := swarm.List(s.o.SwarmDir)
	if err != nil {
		apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: "swarm.list", Kind: apierr.KindInternal, Message: err.Error()})
		return
	}
	apierr.WriteJSON(w, http.StatusOK, names)
}
