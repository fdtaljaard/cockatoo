package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"cockatoo/internal/account"
	"cockatoo/internal/ai"
	"cockatoo/internal/store"
)

// accountRoutes are the mail accounts. A password comes in on test and on add, and never goes out.
func accountRoutes(mux *http.ServeMux, accounts *account.Service) {
	mux.HandleFunc("GET /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		list, err := accounts.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})

	// Signs in with the details as typed and counts the inbox. Saves nothing.
	mux.HandleFunc("POST /api/accounts/test", func(w http.ResponseWriter, r *http.Request) {
		var in account.Input
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		inbox, err := accounts.Test(r.Context(), in)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, inbox)
	})

	// Tests first; saves only when the sign-in works.
	mux.HandleFunc("POST /api/accounts", func(w http.ResponseWriter, r *http.Request) {
		var in account.Input
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		a, err := accounts.Add(r.Context(), in)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusCreated, a)
	})

	// The messages kept from the last refresh, at once, without asking the mail server.
	mux.HandleFunc("GET /api/inbox", func(w http.ResponseWriter, r *http.Request) {
		list, err := accounts.StoredInbox(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	})

	// Starts bringing every account's copy of its inbox up to date, in the background, and
	// answers at once with the progress. Nothing new starts while a sync runs.
	mux.HandleFunc("POST /api/sync", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusAccepted, accounts.StartSync())
	})

	mux.HandleFunc("GET /api/sync", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, accounts.SyncStatus())
	})

	// The shape of one account's whole inbox: totals, per year, the biggest senders.
	mux.HandleFunc("GET /api/accounts/{id}/overview", func(w http.ResponseWriter, r *http.Request) {
		id, err := accountID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		o, err := accounts.Overview(r.Context(), id)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, o)
	})

	// What an action would touch, for the user to confirm. Nothing is changed.
	mux.HandleFunc("POST /api/accounts/{id}/actions/preview", func(w http.ResponseWriter, r *http.Request) {
		id, err := accountID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var c store.Criteria
		if err := readJSON(w, r, &c); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		p, err := accounts.PreviewAction(r.Context(), id, c)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	})

	// Moves the confirmed messages to Trash or Spam, audited before and after. 409 when the
	// match changed, the mailbox changed, or a sync is running: nothing is changed then.
	mux.HandleFunc("POST /api/accounts/{id}/actions", func(w http.ResponseWriter, r *http.Request) {
		id, err := accountID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var req account.ActionRequest
		if err := readJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		out, err := accounts.ApplyAction(r.Context(), id, req)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	})

	// Records that the user opened a sender's unsubscribe address. Cockatoo fetches nothing.
	mux.HandleFunc("POST /api/accounts/{id}/unsubscribe-opened", func(w http.ResponseWriter, r *http.Request) {
		id, err := accountID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		var in struct {
			Address string `json:"address"`
			Target  string `json:"target"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := accounts.LogUnsubscribe(r.Context(), id, in.Address, in.Target); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Every action cockatoo took, newest first. The page that shows it comes later.
	mux.HandleFunc("GET /api/audit", func(w http.ResponseWriter, r *http.Request) {
		entries, err := accounts.Audit(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, entries)
	})

	mux.HandleFunc("GET /api/accounts/{id}/inbox", func(w http.ResponseWriter, r *http.Request) {
		id, err := accountID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		inbox, err := accounts.Inbox(r.Context(), id)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, inbox)
	})

	mux.HandleFunc("DELETE /api/accounts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := accountID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := accounts.Remove(r.Context(), id); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func accountID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("account id must be a positive number, not %q", r.PathValue("id"))
	}
	return id, nil
}

// statusFor maps an error to its HTTP status: 400 for a form that is not filled in, 404 and 409
// for the store's answers and for AI that is off or has no key, 502 for everything the mail
// server or Anthropic said or failed to say.
func statusFor(err error) int {
	var invalid *account.InvalidError
	var aiInvalid *ai.InvalidError
	switch {
	case errors.As(err, &invalid), errors.As(err, &aiInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ai.ErrOff), errors.Is(err, ai.ErrNoKey), errors.Is(err, account.ErrBusy):
		return http.StatusConflict
	case errors.As(err, new(*account.ConflictError)):
		return http.StatusConflict
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrDuplicate):
		return http.StatusConflict
	default:
		return http.StatusBadGateway
	}
}

// readJSON reads a small JSON body into v.
func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("the request body is not valid JSON: %w", err)
	}
	return nil
}
