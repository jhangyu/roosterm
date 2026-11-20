package api

import (
	"net/http"

	"roosterm/host/internal/apierr"
	"roosterm/host/internal/swarm"
)

func (s *server) listSwarms(w http.ResponseWriter, r *http.Request) *apierr.Error {
	names, err := swarm.List(r.Context(), s.o.SwarmDir)
	if err != nil {
		return apierr.Internal("swarm.list", "", err.Error())
	}
	apierr.WriteJSON(w, http.StatusOK, names)
	return nil
}
