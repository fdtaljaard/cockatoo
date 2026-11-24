// Package api exposes the app over HTTP JSON and serves the embedded UI.
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"cockatoo/internal/store"
)

// Handler builds the HTTP routes: /api/* for JSON, everything else is the single-page UI.
// version is the build stamp, shown in /api/health so "which build am I looking at" is never a guess.
func Handler(st *store.Store, ui fs.FS, version string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now(), "version": version})
	})

	// What the DuckDB file says about itself.
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := st.Status(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	})

	// Any other /api/ path is an unknown route, not a page of the UI.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, fmt.Errorf("no such API route: %s %s", r.Method, r.URL.Path))
	})
	mux.Handle("/", spa(ui))
	return logRequests(mux)
}

// spa serves the built frontend and falls back to index.html for client-side routes.
func spa(ui fs.FS) http.Handler {
	files := http.FileServer(http.FS(ui))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(ui, p); err != nil {
			r.URL.Path = "/"
			p = "index.html"
		}
		if p == "index.html" {
			// Never let a browser profile keep an old shell; the hashed assets under /assets/ cache freely.
			w.Header().Set("Cache-Control", "no-store")
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}
