package web

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"roosterm/host/internal/apierr"
)

//go:embed all:dist
var dist embed.FS

const indexFile = "index.html"

// staticHandler serves dist/; unknown paths get index.html (SPA routes).
func staticHandler() http.Handler {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(fmt.Sprintf("web: embedded dist: %v", err)) // constant directory name: cannot fail at run time
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			apierr.Write(w, &apierr.Error{Status: http.StatusMethodNotAllowed, Op: "static", Kind: apierr.KindBadRequest,
				Message: fmt.Sprintf("method: expected GET or HEAD for %s, got %s", r.URL.Path, r.Method)})
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = indexFile
		}
		info, err := fs.Stat(files, name)
		switch {
		case errors.Is(err, fs.ErrNotExist) || (err == nil && info.IsDir()):
			name = indexFile
		case err != nil:
			apierr.Write(w, &apierr.Error{Status: http.StatusInternalServerError, Op: "static", Kind: apierr.KindInternal, Message: err.Error()})
			return
		}
		http.ServeFileFS(w, r, files, name)
	})
}
