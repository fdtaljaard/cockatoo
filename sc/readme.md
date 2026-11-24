# cockatoo

An email client in one Windows program: one `cockatoo.exe`, one `cockatoo.duckdb` beside it. The same
shape as kakapo: a Go backend that serves a React frontend embedded in the exe.

## Run

Build it (below), then double-click `cockatoo.exe`. It serves the UI on `http://127.0.0.1:7332` and opens
it as an app window. Closing the window stops the app. Flags:

```
cockatoo.exe -port 7332 -data <folder> -no-browser
```

`cockatoo.log` beside the exe shows when the app started and why it stopped. A second double-click while
it is running opens its window again.

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

Tests:

```powershell
go test ./...
```

## Layout

```
cmd/cockatoo/     main: flags, DuckDB path, HTTP listener, the app window
internal/api/     HTTP JSON routes and the embedded single-page UI
internal/store/   DuckDB: schema migrations, single writer, queries
web/              React + TypeScript + Tailwind + shadcn/ui; web/dist is embedded by web/embed.go
```

A schema change is a new entry at the end of `migrations` in `internal/store/store.go`; each runs once.
