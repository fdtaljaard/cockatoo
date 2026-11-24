package api

import (
	"html/template"
	"net/http"

	"cockatoo/internal/account"
)

// googleRoutes are Google's own sign-in: its setup (the Google Cloud client), starting a
// sign-in, following it, and the page Google sends the browser back to. The client secret comes
// in on PUT and never goes out; the sign-in code arrives on the callback and is never shown.
func googleRoutes(mux *http.ServeMux, accounts *account.Service) {
	mux.HandleFunc("GET /api/google", func(w http.ResponseWriter, r *http.Request) {
		settings, err := accounts.GoogleSettings(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	mux.HandleFunc("PUT /api/google", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		settings, err := accounts.SaveGoogleClient(r.Context(), in.ClientID, in.ClientSecret)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, settings)
	})

	// Opens Google's sign-in page in the default browser and answers with its state. hint is the
	// address when signing in again.
	mux.HandleFunc("POST /api/google/signin", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Hint string `json:"hint"`
		}
		if err := readJSON(w, r, &in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		start, err := accounts.StartGoogleSignIn(r.Context(), in.Hint)
		if err != nil {
			writeError(w, statusFor(err), err)
			return
		}
		writeJSON(w, http.StatusOK, start)
	})

	mux.HandleFunc("GET /api/google/signin/{state}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, accounts.GoogleSignInStatus(r.PathValue("state")))
	})

	// Where Google sends the browser back. A plain page, not the app: the browser may not be the
	// app's window. Only cockatoo's own words are shown; nothing from the address is echoed.
	mux.HandleFunc("GET "+account.CallbackPath, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		st := accounts.FinishGoogleSignIn(r.Context(), q.Get("state"), q.Get("code"), q.Get("error"))
		page := callbackPage{OK: st.State == "done", Message: st.Error}
		if st.Account != nil {
			page.Email = st.Account.Email
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !page.OK {
			w.WriteHeader(http.StatusBadRequest)
		}
		_ = callbackTemplate.Execute(w, page)
	})
}

type callbackPage struct {
	OK      bool
	Email   string
	Message string
}

var callbackTemplate = template.Must(template.New("callback").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>cockatoo</title>
<style>
  :root { color-scheme: light dark; font-family: system-ui, sans-serif; }
  body { margin: 0; min-height: 100svh; display: grid; place-items: center; background: Canvas; color: CanvasText; }
  main { max-width: 32rem; padding: 2rem; }
  h1 { font-size: 1.25rem; margin: 0 0 .75rem; }
  p { margin: 0 0 .5rem; line-height: 1.5; opacity: .85; }
</style>
</head>
<body>
<main>
{{if .OK}}
  <h1>Signed in to Google</h1>
  <p>Cockatoo can now read {{.Email}}. Close this tab and go back to cockatoo.</p>
{{else}}
  <h1>Not signed in</h1>
  <p>{{.Message}}</p>
  <p>Close this tab and try again from cockatoo.</p>
{{end}}
</main>
</body>
</html>`))
