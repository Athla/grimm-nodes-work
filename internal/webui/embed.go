// Package webui embeds the built React/Vite SPA bundle and exposes it as an
// http.Handler suitable for mounting at "/" alongside the API routes.
//
// The bundle is produced by `make build-frontend`, which builds webui/dist/
// and copies it into internal/webui/dist/ prior to `go build`. When the bundle
// is missing (for example, during Go-only iteration), Handler() returns a
// short instructional response instead of a confusing 404.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded SPA. Requests for
// known asset paths are served from the bundle; any other path falls back to
// index.html so client-side routes resolve correctly.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return bundleMissingHandler("internal/webui/dist subtree unavailable: " + err.Error())
	}

	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return bundleMissingHandler("frontend bundle not built — run `make build-frontend` to produce webui/dist/")
	}

	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			fileServer.ServeHTTP(w, r)
			return
		}

		if _, err := fs.Stat(sub, path); err != nil {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}

		fileServer.ServeHTTP(w, r)
	})
}

func bundleMissingHandler(msg string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(msg + "\n"))
	})
}
