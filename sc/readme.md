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

## Google sign-in (work and school accounts)

Google Workspace accounts no longer take a password from mail apps; they sign in with Google itself
(OAuth). The setup form shows both ways: **Work or school account** (Google sign-in) above, **Personal
Gmail, with an app password** below.

**How it works.** **Sign in with Google** opens Google's page in your default browser (where you are
probably already signed in). You choose the account and allow cockatoo to read your mail. Google sends
the browser back to `http://127.0.0.1:<port>/oauth/google/callback` with a one-time code; cockatoo
exchanges it, with a PKCE verifier only it holds, for a refresh token, checks the mailbox opens with it,
and saves the account. The browser tab then says it is done; the app notices on its own.

- **What is kept.** The refresh token, encrypted with Windows DPAPI like a password. A fresh access token
  is asked for at every connection and never stored.
- **Address.** Taken from Google's answer, not typed. Signing in again with the same address updates the
  same account: its copy of the inbox and its audit stay. An address already added with an app password is
  left alone unless you sign it in with Google on purpose.
- **Removing an account** deletes the token from cockatoo; it does not withdraw cockatoo's access at Google
  (do that under Third-party apps in your Google Account).
- **When it stops working** (revoked, expired, password changed): the account row says so with Google's
  own error and shows **Sign in with Google again**.
- **Scopes.** `https://mail.google.com/` (the only one Gmail's IMAP takes), `openid` and `email`.

### Setting up the Google Cloud client (once)

Cockatoo signs in through an OAuth client in a Google Cloud project. For a Workspace account, make the
project inside that Workspace so the app can be **Internal**: anyone in the organisation can sign in, and
Google does not review it. Google's console is reorganised often; these follow its "Google Auth Platform"
pages as of November 2026.

1. In the Google Cloud console, create a project (or pick one) under your Workspace organisation.
2. Open **Google Auth Platform**. Under **Branding**, give the app a name (cockatoo) and a support email.
3. Under **Audience**, choose **Internal**.
4. Under **Data Access**, nothing needs adding for an Internal app; Google lists scopes only for apps used
   outside the organisation.
5. Under **Clients**, create a client of type **Desktop app** (check the name against the console; this
   step was not confirmed against Google's current pages). Copy its **Client ID** and **Client secret**.
6. In cockatoo, **Mail accounts → Add account → Work or school account**: paste both and **Save Google
   client**, then **Sign in with Google**.

If your Workspace restricts third-party apps, an admin may have to allow the client in the Admin console
(Security → API controls). IMAP must also be on for users (Apps → Google Workspace → Gmail → End User
Access).

An **External** app (anyone with a Google account) works the same way but, while in Testing, only for up
to 100 listed test users, and their sign-in reportedly lapses after 7 days. Publishing it for everyone
needs Google's review, which for full mail access includes a security assessment.

## Inbox

The app opens on **Inbox** once an account is saved: the last 24 hours of every account's inbox, newest
first, unread ones marked. **Clean up** shows the shape of a whole inbox: how much mail arrived each year,
how much is unread, and the 25 senders with the most mail.

Both read from cockatoo's copy of the inbox in DuckDB, which the **sync** keeps up to date. A sync starts
when either screen opens (and on **Refresh**) and runs in the background:

1. It lists every message in the inbox with whether it is read (quick, even for thousands).
2. It reads the headers of the messages not yet in the copy, newest first, 500 at a time. Each batch is
   saved at once, so the Inbox list fills first, and a sync that is stopped carries on next time.
3. After a complete pass it updates read states and removes what has left the inbox (deleted, archived,
   moved in Gmail). A failed or partial pass never removes anything.

- **First sync.** It reads every message's headers, which for a large inbox can take a minute or more.
  Until it finishes, Clean up says how far it has got; its figures cover what has been read so far.
- **Headers only.** Sender, subject, arrival time and whether it is read. No message bodies.
- **Read-only.** The inbox is opened read-only over IMAP, so nothing is marked as read.
- **Inbox only.** Counts cover mail in the inbox, one per message across all Gmail tabs, so they can
  differ from Gmail's own numbers.
- **Kept in DuckDB.** In the `message` and `inbox_sync` tables, a cache of what Gmail holds. Senders and
  subjects are stored as they are, not encrypted. Removing an account removes its copy.
- **Odd messages.** A subject in an unknown charset is kept as it was sent. A message the server cannot
  give is skipped and counted, and the sync carries on.
- **Several accounts.** One Inbox list, each row naming its account; Clean up shows one account at a
  time, chosen with the buttons above it.

## Clean up: Trash, Spam and Unsubscribe

The actions are on the rows themselves. A year's row has **Trash**; a sender's row has **Trash**, **Spam**
and, when that sender's mail says how, **Unsubscribe**. **Can unsubscribe** lists only those senders.

- **Confirm in the row.** Trash or Spam opens a confirm under the row: how many messages, how many unread,
  the three latest subjects. **Move to Trash** moves exactly that many; if the mailbox changed in between,
  nothing moves and it says so.
- **Where it goes.** Trash and Spam are the folders Gmail marks as such (`\Trash`, `\Junk`), found by
  that mark or by Gmail's own names. Cockatoo never creates a folder and never deletes permanently: Gmail
  keeps Trash and Spam for 30 days.
- **How it moves.** Over IMAP MOVE, 500 at a time. A server without MOVE is refused (the older way, copy,
  flag and expunge, can remove other messages). After moving, cockatoo asks the server what actually left
  the inbox and only takes that out of its copy.
- **When.** Only once the whole mailbox has been read, and never while a sync is running. An action that
  has started runs to its end even if the window is closed; the app waits for it before stopping.
- **Unsubscribe.** Read from each message's `List-Unsubscribe` header, https and mailto only. The button is
  a link to the sender's own page (or an email to them), opened by your click. Cockatoo does not fetch or
  send anything itself yet.

## Audit

Every action is recorded in DuckDB before anything changes and again after, insert-only:

- `action`: when, which account (by address, so the record outlives the account), what (`trash`, `spam`,
  `unsubscribe`), the criteria (`{"kind":"sender","address":...}` or `{"kind":"year","year":...}`), the
  description you confirmed, who asked (`you`; later `ai`), the folder used, how many messages matched.
- `action_item`: each message the action was for: UID, Message-ID, sender, subject, date.
- `action_result`: when it finished, `done`, `partial`, `failed` or `opened`, how many moved, which did
  not, and the error.

`GET /api/audit` returns the latest 200. A page to read it comes later. The senders and subjects of moved
mail stay in the audit, unencrypted, after the mail itself is gone.

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
internal/account/ use cases for mail accounts: test, save, remove; Google sign-in; the sync, overview and actions
internal/ai/      the Anthropic client, and the AI settings that hold the key and turn it on
internal/mail/    IMAP: sign in (password or Google token), count, list and read headers; move to Trash or Spam
internal/secret/  encrypts passwords with Windows DPAPI before they are stored
internal/store/   DuckDB: schema migrations, single writer, queries
web/              React + TypeScript + Tailwind + shadcn/ui; web/dist is embedded by web/embed.go
```

A schema change is a new entry at the end of `migrations` in `internal/store/store.go`; each runs once.
