// Package api exposes the app over HTTP JSON and serves the embedded UI.
package api

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cockatoo/internal/account"
	"cockatoo/internal/ai"
	"cockatoo/internal/store"
)

// Handler builds the HTTP routes: /api/* for JSON, everything else is the single-page UI.
// version is the build stamp, shown in /api/health so "which build am I looking at" is never a guess.
func Handler(st *store.Store, accounts *account.Service, assistant *ai.Service, ui fs.FS, version string) http.Handler {
	mux := http.NewServeMux()
	accountRoutes(mux, accounts)
	aiRoutes(mux, assistant)

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

// LocalOnly refuses requests that did not come from the app's own page. The server listens on
// 127.0.0.1 only, but a web page on any site, open in a browser on this computer, can still
// send requests to that address. Two checks stop it: the Host header must name this computer
// (a site that points its own name at 127.0.0.1 fails here), and a request that says where it
// came from (Origin) must have come from this same address.
func LocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			writeError(w, http.StatusForbidden, fmt.Errorf("cockatoo only answers on this computer, not on %q", r.Host))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if u, err := url.Parse(origin); err != nil || u.Host != r.Host {
				writeError(w, http.StatusForbidden, fmt.Errorf("cockatoo does not answer requests from %s", origin))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
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
