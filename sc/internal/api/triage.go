package api

import (
	"net/http"

	"cockatoo/internal/store"
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

// suggestRoutes: the AI's suggestion of a list for each message (billed to the user's key; each
// message is asked about once).
func suggestRoutes(mux *http.ServeMux, t *triage.Service) {
	mux.HandleFunc("POST /api/suggest", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Messages []store.MessageKey `json:"messages"`
			WithText bool               `json:"withText"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		res, err := t.Suggest(r.Context(), in.Messages, in.WithText)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

// teachRoutes: from a correction, the agent proposes a change to the user's skills. Nothing is saved
// here: the user approves the proposal with POST /api/skills.
func teachRoutes(mux *http.ServeMux, t *triage.Service) {
	mux.HandleFunc("POST /api/teach", func(w http.ResponseWriter, r *http.Request) {
		var in triage.TeachInput
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		p, err := t.Teach(r.Context(), in)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})
}

func scopeOf(s string) string {
	if s == "" {
		return "all"
	}
	return s
}
