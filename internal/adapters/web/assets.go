package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed assets/*
var assets embed.FS

func (s *Server) handleAssets() http.Handler {
	assetFiles, err := fs.Sub(assets, "assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	fileServer := http.StripPrefix("/assets/", http.FileServer(http.FS(assetFiles)))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=31536000")
		if strings.HasSuffix(r.URL.Path, ".css") {
			w.Header().Set("Content-Type", "text/css")
		} else if strings.HasSuffix(r.URL.Path, ".js") {
			w.Header().Set("Content-Type", "application/javascript")
		}
		fileServer.ServeHTTP(w, r)
	})
}
