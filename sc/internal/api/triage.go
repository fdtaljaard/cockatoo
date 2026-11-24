package api

import (
	"net/http"

	"cockatoo/internal/triage"
)

// triageRoutes: what needs the user now. GET reads the last answer for a scope ("all" or an
// account id) without asking the AI; POST asks it (billed to the user's key).
func triageRoutes(mux *http.ServeMux, t *triage.Service) {
	mux.HandleFunc("GET /api/triage", func(w http.ResponseWriter, r *http.Request) {
		result, found, err := t.Latest(r.Context(), scopeOf(r.URL.Query().Get("scope")))
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		if !found {
			writeJSON(w, http.StatusOK, nil)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("POST /api/triage", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Scope    string `json:"scope"`
			WithText bool   `json:"withText"` // send the first lines of each message that may need the user
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := t.Run(r.Context(), scopeOf(in.Scope), in.WithText)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func scopeOf(s string) string {
	if s == "" {
		return "all"
	}
	return s
}
