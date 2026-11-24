package api

import (
	"fmt"
	"net/http"
	"strconv"

	"cockatoo/internal/account"
	"cockatoo/internal/store"
)

// folderRoutes are the save folders, browsing inside them, and saving attachments into them. The
// page only ever names a save folder by id and a relative path inside it; a full path comes in
// only when a save folder is added.
func folderRoutes(mux *http.ServeMux, accounts *account.Service) {
	mux.HandleFunc("GET /api/folders", func(w http.ResponseWriter, r *http.Request) {
		list, err := accounts.ListSaveFolders(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})

	mux.HandleFunc("POST /api/folders", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		f, err := accounts.AddSaveFolder(r.Context(), in.Name, in.Path)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusCreated, f)
	})

	mux.HandleFunc("DELETE /api/folders/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := folderID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := accounts.RemoveSaveFolder(r.Context(), id); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// The folders inside a place in a save folder. path is relative, "/"-separated.
	mux.HandleFunc("GET /api/folders/{id}/browse", func(w http.ResponseWriter, r *http.Request) {
		id, err := folderID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		b, err := accounts.BrowseSaveFolder(r.Context(), id, r.URL.Query().Get("path"))
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, b)
	})

	// Makes a folder inside a place in a save folder and answers with it, browsed.
	mux.HandleFunc("POST /api/folders/{id}/subfolders", func(w http.ResponseWriter, r *http.Request) {
		id, err := folderID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var in struct {
			Path string `json:"path"`
			Name string `json:"name"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		b, err := accounts.MakeSaveSubfolder(r.Context(), id, in.Path, in.Name)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusCreated, b)
	})

	// Saves the attachments of the chosen messages; audited. Per-message problems are in the
	// answer, which is 200 even when some messages had nothing to save.
	mux.HandleFunc("POST /api/attachments/save", func(w http.ResponseWriter, r *http.Request) {
		var req account.SaveRequest
		if err := readJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		res, err := accounts.SaveAttachments(r.Context(), req)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Routines: what the user does every time a certain mail comes in.
	mux.HandleFunc("GET /api/routines", func(w http.ResponseWriter, r *http.Request) {
		list, err := accounts.ListRoutines(r.Context())
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})
	mux.HandleFunc("POST /api/routines", func(w http.ResponseWriter, r *http.Request) {
		var in account.RoutineInput
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		routine, err := accounts.AddRoutine(r.Context(), in)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusCreated, routine)
	})
	mux.HandleFunc("DELETE /api/routines/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("routine id must be a positive number, not %q", r.PathValue("id")))
			return
		}
		if err := accounts.RemoveRoutine(r.Context(), id); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Moves chosen messages to Gmail's Trash; audited.
	mux.HandleFunc("POST /api/messages/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []store.MessageKey `json:"messages"`
		}
		if err := readJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		res, err := accounts.DeleteMessages(r.Context(), req.Messages)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})

	// Marks messages done, or not done; audited. Changes nothing in the mailbox.
	mux.HandleFunc("POST /api/messages/done", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []store.MessageKey `json:"messages"`
			Done     bool               `json:"done"`
		}
		if err := readJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		n, err := accounts.MarkDone(r.Context(), req.Messages, req.Done)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"marked": n})
	})
}

func folderID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("save folder id must be a positive number, not %q", r.PathValue("id"))
	}
	return id, nil
}
