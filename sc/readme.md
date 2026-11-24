# cockatoo

An email client in one Windows program: one `cockatoo.exe`, one `cockatoo.duckdb` beside it. The same
shape as kakapo: a Go backend that serves a React frontend embedded in the exe. Why it is being built and
what its core parts are is in [SCOPE.md](SCOPE.md).

## Run

Build it (below), then double-click `cockatoo.exe`. It serves the UI on `http://127.0.0.1:7332` and opens
it as an app window. Closing the window stops the app. Flags:

```
cockatoo.exe -port 7332 -data <folder> -no-browser
```

`cockatoo.log` beside the exe shows when the app started and why it stopped. A second double-click while
it is running opens its window again.

## Accounts

When no account is saved, the app opens on a form: Gmail address, app password, and an optional name.
**Test connection** signs in to Gmail and counts the inbox. **Save account** signs in again and saves only
when that works. With an account saved, the app opens on the account list, where each account signs in
and shows its inbox count.

- **App password.** A 16-letter password Google makes for one app, at `myaccount.google.com/apppasswords`.
  It needs 2-Step Verification on the Google account. Your normal Google password does not work.
- **Where it is kept.** The account is a row in `cockatoo.duckdb`. The app password is encrypted with
  Windows DPAPI before it is stored: it opens only for the same Windows user on the same computer. The
  address and name are not encrypted, and no password is ever sent back to the page.
- **Moving the file.** A `cockatoo.duckdb` copied to another computer or another Windows user keeps its
  accounts but their passwords cannot be read. Remove each account and add it again.
- **The connection.** IMAP over TLS to `imap.gmail.com:993`. Nothing in the mailbox is changed.

## AI

Under **AI** in the header: tick **Use AI in cockatoo**, paste an Anthropic API key and **Save key**.

- **Checking the key.** Save asks Anthropic about the model with the key, which costs nothing, and saves
  only when the key is accepted. A key that is refused changes nothing; a key saved before stays.
- **Where it is kept.** In `cockatoo.duckdb`, encrypted with DPAPI like the mail passwords. The page only
  ever sees whether a key is saved and its start and last four characters.
- **Off and remove.** Turning AI off keeps the key. **Remove key** deletes it from cockatoo; the key still
  works in your Anthropic account.
- **Send a test message.** Sends one short message to Claude with the saved key and shows the reply and
  the model that answered. It costs a fraction of a cent.
- **The client.** `internal/ai` wraps Anthropic's official Go SDK (`anthropic-sdk-go`). It reads nothing
  from the environment: an `ANTHROPIC_API_KEY` set on the machine is not used. The model is Claude Opus
  5.5. When Anthropic's safety checks decline a request, the server retries it on the model Anthropic
  recommends (`fallbacks: "default"`). Every feature gets its client from `ai.Service.Client`, which
  refuses when AI is off or no key is saved.

## The window

The UI is a web page, not a browser extension and not a WebView: it is not built for Edge or for Chrome,
and any current browser shows it. The exe only chooses where to show it:

1. Microsoft Edge in app mode (`msedge.exe --app=<url>`): a window with no tabs or address bar. Edge is
   first because every Windows 10, 11 and Server 2022 machine has it.
2. Google Chrome in app mode, with the same flags, when Edge is not there.
3. A tab in the default browser when neither is.

The window has its own browser profile in `%LOCALAPPDATA%\cockatoo\`, so it shares no cookies, extensions
or history with the user's own browser. The exe does not watch the browser's process. The page holds
`GET /api/events` open, and the exe stops 10 seconds after the last page lets go of it.

## Build

Needs Go 1.25+, Node 22+, and a MinGW-w64 gcc on `PATH` (the DuckDB driver is CGO; the gcc is a build-time
dependency only, the exe is static).

```powershell
.\build.ps1
```

Frontend only, with live reload against a running backend (`cockatoo.exe -no-browser`):

```powershell
cd web; npm run dev
```

Frontend with no backend and no mail account, answering from `web/src/api.mock.ts` (the password
`wrong` is refused, anything else connects):

```powershell
cd web; $env:VITE_MOCK_API = '1'; npm run dev
```

Tests:

```powershell
go test ./...
```

The tests start an in-memory IMAP server in place of Gmail and a small HTTP server in place of
Anthropic, so they need no account, no key and no network.

## Layout

```
cmd/cockatoo/     main: flags, DuckDB path, HTTP listener, the app window
internal/api/     HTTP JSON routes and the embedded single-page UI
internal/account/ use cases for mail accounts: test, save, read the inbox, remove
internal/ai/      the Anthropic client, and the AI settings that hold the key and turn it on
internal/mail/    IMAP: sign in and count the inbox
internal/secret/  encrypts passwords with Windows DPAPI before they are stored
internal/store/   DuckDB: schema migrations, single writer, queries
web/              React + TypeScript + Tailwind + shadcn/ui; web/dist is embedded by web/embed.go
```

A schema change is a new entry at the end of `migrations` in `internal/store/store.go`; each runs once.
