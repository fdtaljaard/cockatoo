package api

import (
	"net/http"

	"cockatoo/internal/ai"
)

// aiRoutes are the AI settings. The API key comes in on PUT /api/ai/key and never goes out:
// the settings carry only whether a key is saved and a short hint of it.
func aiRoutes(mux *http.ServeMux, svc *ai.Service) {
	mux.HandleFunc("GET /api/ai", func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.Settings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	mux.HandleFunc("PUT /api/ai/enabled", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Enabled bool `json:"enabled"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		settings, err := svc.SetEnabled(r.Context(), in.Enabled)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	// Checks the key with Anthropic first (free); saves only when it is accepted.
	mux.HandleFunc("PUT /api/ai/key", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Key string `json:"key"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		settings, err := svc.SaveKey(r.Context(), in.Key)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	mux.HandleFunc("DELETE /api/ai/key", func(w http.ResponseWriter, r *http.Request) {
		settings, err := svc.RemoveKey(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	// Sends one short message with the saved key. It costs a little on the user's key.
	mux.HandleFunc("POST /api/ai/test-message", func(w http.ResponseWriter, r *http.Request) {
		reply, err := svc.TestMessage(r.Context())
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, reply)
	})
}
