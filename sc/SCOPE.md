# cockatoo: high-level scope

Date: 2026-11-24. Status: first draft, written from the brief before any screenshots were shared. Built so far:
the scaffold and adding a Gmail account (`readme.md`).

This document says why cockatoo is being built and what its core parts are. It is not a list of work. We
read it before a design or build decision and ask: does this still fit?

Lines marked **(proposed)** are suggestions that have not been agreed. Everything else comes from the brief.

## 1. One line

An extremely simple email client for Windows: one program, your mail accounts in one place, and AI built in.

## 2. What it is for

- **Simple.** How mail is shown and how it is managed must be extremely simple. This is the main reason the
  app exists, and the test every screen has to pass.
- **Several accounts.** You can add more than one email account. Gmail is first.
- **AI built in.** The app has its own AI client. You supply your own API key. Anthropic is first.
- **Its own memory.** DuckDB keeps what the app knows about your mail.

## 3. Grounding rules

1. **Simple wins.** When a feature makes a screen harder to read or a task take more steps, the feature
   loses. Fewer things on screen, fewer settings, plain words.
2. **It runs on your machine.** **(proposed)** There is no cockatoo server. The exe talks to your mail
   provider and to the AI provider directly, with your own sign-in and your own key.
3. **The AI suggests, you decide.** **(proposed)** The AI can read, summarise, sort and draft. It does not
   send, delete or move mail on its own. Anything that changes your mailbox waits for you.
4. **One seam per provider.** **(proposed)** Gmail is the first mail provider and Anthropic the first AI
   provider, each behind a small interface, so a second one is an addition and not a rewrite.

## 4. The moving parts

| Part | What it does | Where it lives |
|---|---|---|
| The window | Shows mail and takes your actions. A React page in an Edge or Chrome app window. | `web/` |
| The backend | One Go program. Serves the page, owns everything below. | `cmd/cockatoo`, `internal/api` |
| Accounts | Adds, lists and removes mail accounts. Tests the sign-in before saving. | `internal/account` |
| Mail provider | Talks to one kind of mailbox: list, read, change, send. Gmail first. | `internal/mail` |
| Sync | Copies what changed in each mailbox into the store, and sends your changes back. | new: `internal/sync` |
| The store | The DuckDB file: accounts, mail, AI results, settings. One writer. | `internal/store` |
| Secrets | Encrypts passwords and keys before they go into the store. | `internal/secret` |
| AI client | Sends a request to the AI provider with your key and returns the answer. Anthropic first. | `internal/ai` |

How they connect:

```
window (React)  <-->  backend (Go, 127.0.0.1)
                         |-- accounts --> secrets (encrypt) --> store
                         |-- sync <--> mail provider <--> Gmail
                         |-- AI client --> Anthropic (your key)
                         '-- store --> cockatoo.duckdb
```

The app runs only while its window is open, so mail syncs while you are using it and not in the background.

## 5. What is kept where

| What | Where | Why |
|---|---|---|
| Your mail itself | The mail provider (Gmail) | It stays the source of truth. Cockatoo can be deleted and nothing is lost. |
| What cockatoo knows about your mail | `cockatoo.duckdb` beside the exe | Fast to show and search, and it is where AI results are kept so they are not paid for twice. |
| Mail passwords, the AI API key | `cockatoo.duckdb`, encrypted | Encrypted with the Windows sign-in (DPAPI) before they are stored. They open only for the same Windows user on the same computer, so a copy of the file carries no usable password. |

How much of each message DuckDB holds (headers only, or the full text and attachments) is not decided. See
section 8.

## 6. Mail accounts: Gmail first

Phase one connects to Gmail over IMAP with a Google **app password**: a 16-letter password Google makes
for one app. The setup form asks for the address and that password, signs in, and saves the account only
when the sign-in works.

- It needs 2-Step Verification on the Google account. It needs no Google Cloud project and no approval
  from Google, which is why it comes first.
- A personal Gmail and a Google Workspace address use the same server, so the same form serves both.
  A Workspace administrator can switch app passwords off; that case needs Google's own sign-in (OAuth).
- Google's own sign-in (OAuth with the Gmail API) is the later alternative. It gives threads and labels
  directly, but needs a Google Cloud project, and Google's verification before anyone outside a test list
  can use it.

## 7. AI: Anthropic first

- You turn AI on and enter your own Anthropic API key in the app. It is checked with Anthropic (a free
  call), then encrypted with DPAPI and kept in the DuckDB file. Turning AI off keeps the key.
- The backend calls the Claude API with Anthropic's official Go SDK. The model is stored with the setting,
  with Claude Opus 5.5 as the default; there is no model picker yet.
- When Anthropic's safety checks decline a request, the server retries it on the model Anthropic
  recommends for that case (`fallbacks: "default"`).
- **(proposed)** Start with single requests (summarise this thread, sort this message, draft this reply).
  No agent until a feature needs one.
- Mail text goes to Anthropic only when you use an AI feature, straight from your machine with your key.
- Each AI request costs money on your key. Results are stored, so the same question about the same message
  is asked once.

What the AI actually does for the user is not decided. See section 8.

## 8. Not decided yet

1. **What the AI does.** Summaries, sorting, drafting replies, search in plain words, something else? This
   shapes the screens more than anything.
2. **How much mail is stored locally.** Headers and a preview only (small file, needs the network to open a
   message), or the full text (works offline, searchable, larger file)? And attachments?
3. **Sending.** The brief says display and manage. Is writing and sending mail in the first version?
4. **Google's own sign-in.** When do we add OAuth beside the app password? See section 6.
5. **One inbox or one per account.** With several accounts, is mail shown together or separately?
6. **The look.** Waiting for the screenshots, which are inspiration and not a design to copy.

## 9. Not in scope for now

- Mail providers other than Gmail, and AI providers other than Anthropic. The seams are there; the second
  ones come later.
- Anything on a server: no hosted sync, no accounts with us, no telemetry.
- Calendar, contacts and chat.
- Mac and Linux. The window and the credential store are Windows-first, as in kakapo.

## 10. Decisions log

| Date | Decision |
|---|---|
| 2026-11-24 | Stack as kakapo: Go backend, React frontend embedded in one exe, DuckDB beside it. |
| 2026-11-24 | Gmail is the first mail provider. Anthropic is the first AI provider, with the user's own key. |
| 2026-11-24 | DuckDB keeps what the app knows about the user's mail. |
| 2026-11-24 | Phase one: the app opens on a setup form when there is no account. Personal Gmail first, over IMAP with an app password. |
| 2026-11-24 | Account details are saved in DuckDB, the password encrypted with Windows DPAPI. This replaces the earlier idea of Windows Credential Manager. |
| 2026-11-24 | AI: an on/off switch and the user's Anthropic key, encrypted the same way. Official Anthropic Go SDK, Claude Opus 5.5, refusal fallbacks on. |
