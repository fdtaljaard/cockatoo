# cockatoo: high-level scope

Date: 2026-11-24. Status: first draft. Built so far: the scaffold, Gmail accounts, the AI connection, the
last-24-hours inbox, and cleanup steps 1 and 2: the whole-inbox overview, and Trash, Spam and the unsubscribe
link on its rows, all audited; Google sign-in for work accounts (`readme.md`). The product record for design work is `../PRODUCT.md`.

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
- **A product for other people.** Built to be used by anyone with a mailbox, not only by us. Wording, setup
  and sign-in are designed for a stranger installing it.

## 3. Grounding rules

1. **Simple wins.** When a feature makes a screen harder to read or a task take more steps, the feature
   loses. Fewer things on screen, fewer settings, plain words.
2. **Flat, not deep.** Every action is visible where you are looking. See section 8.
3. **It runs on your machine.** **(proposed)** There is no cockatoo server. The exe talks to your mail
   provider and to the AI provider directly, with your own sign-in and your own key.
4. **The AI suggests, you decide.** The AI can read, summarise, sort, ask and propose. It does not send,
   delete or move mail on its own. Anything that changes your mailbox waits for your click.
5. **Everything is audited.** Every action cockatoo takes on a mailbox is recorded before it changes
   anything (what, which messages, by what criteria, who asked) and again after (what happened). The record
   is insert-only and outlives the account. A page to read it comes later; the record starts now.
6. **One seam per provider.** **(proposed)** Gmail is the first mail provider and Anthropic the first AI
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
| Files | Save folders on disk: lists and makes subfolders, writes attachments without overwriting. | `internal/files` |

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
| Attachments you save | Your own folders, set up on the Folders screen | You choose where; cockatoo keeps only the list of folders and, in the audit, which files it wrote. |
| Mail passwords, the AI API key | `cockatoo.duckdb`, encrypted | Encrypted with the Windows sign-in (DPAPI) before they are stored. They open only for the same Windows user on the same computer, so a copy of the file carries no usable password. |

Headers (sender, recipients, subject, time, read or not) are stored as they are, not encrypted; so are the
headers of the Sent folder. Message text is not stored: it is read when needed (the first lines, for the
AI; the attachments, when you save them) and let go. Whether bodies are ever stored is not decided. See section 10.

## 6. Mail accounts: Gmail first

Phase one connects to Gmail over IMAP with a Google **app password**: a 16-letter password Google makes
for one app. The setup form asks for the address and that password, signs in, and saves the account only
when the sign-in works.

- It needs 2-Step Verification on the Google account. It needs no Google Cloud project and no approval
  from Google, which is why it comes first.
- A personal Gmail and a Google Workspace address use the same server, so the same form serves both.
  A Workspace administrator can switch app passwords off; that case needs Google's own sign-in (OAuth).
- Google Workspace accounts no longer take passwords from mail apps (Google, May 2025), so they sign in
  with Google's own sign-in: OAuth for a desktop app, then IMAP with the token (XOAUTH2). Built
  2026-11-24. It needs a Google Cloud OAuth client; inside one Workspace the app can be Internal, with no
  review by Google. Offering it to anyone needs an External app and Google's review of full mail access,
  which is where a product for other people ends up.

## 7. AI: Anthropic first

- You turn AI on and enter your own Anthropic API key in the app. It is checked with Anthropic (a free
  call), then encrypted with DPAPI and kept in the DuckDB file. Turning AI off keeps the key.
- The backend calls the Claude API with Anthropic's official Go SDK. The model is stored with the setting,
  with Claude Opus 5.5 as the default; there is no model picker yet.
- When Anthropic's safety checks decline a request, the server retries it on the model Anthropic
  recommends for that case (`fallbacks: "default"`).
- Mail text goes to Anthropic only when you use an AI feature, straight from your machine with your key.
- Each AI request costs money on your key. Results are stored, so the same question about the same message
  is asked once.

The AI's first jobs: **what needs you now** on the Inbox (built 2026-11-24: cockatoo first leaves out
automated and already-answered mail using the headers and the Sent folder, then the AI reads the rest,
with the first lines of each message when the user allows it, and says what needs a reply), and cleaning
up the inbox (section 9).
Every request to the AI is recorded, insert-only, with the messages it sent and the tokens it took.

## 8. The UI: flat, quiet, at a glance

Email clients are deep: you select a message to find its menu, and the menu has sub-menus. Cockatoo is
flat. The rules every screen is checked against:

1. **Every action is on the thing it acts on.** A row in the inbox carries its own actions, visible on the
   row: read, delete, spam (and later unsubscribe). No right-click menu, no "more" button, no sub-menu.
2. **One glance shows what you can do.** If an action exists for what you are looking at, it is on screen.
   Nothing is hidden behind a selection, a hover-only state or a settings page.
3. **Many at once is allowed.** Selecting several rows to act on them together is fine; the same actions
   then apply to the selection, shown in the same place.
4. **The AI works in the background.** It prepares, groups and suggests. It shows up as an answer or a
   proposal beside your mail, not as a separate tool you have to learn.
5. **Modern, sleek and simple.** Calm, few words, nothing decorative. The visual design itself is not
   decided yet; `../PRODUCT.md` holds the product record it will be built from (with Impeccable).

## 9. Cleaning up the inbox with AI

The first AI feature. A mailbox with thousands of messages and hundreds unread, spread over years, is the
common case. Cleaning it up should feel like a short conversation, not a settings page.

**The experience.** You open a cleanup session. The AI has already looked at your inbox and starts the
conversation: what it sees (how old, who sends the most, what you never open) and what your goal is. It
asks questions about what it found ("these 14 senders sent 312 unread messages and you have opened none in
two years: are they important?") and proposes batches. Each proposal is a row with its actions on it:
delete, spam, keep, and later unsubscribe. You decide; cockatoo does it.

**It remembers.** Every decision is kept with its reason, in your words: "delete: newsletters from
shop.example.com, I never read them". The next session starts from those decisions: it applies them to the
mail that arrived since (shown for your OK, not done silently) and only asks about what is new. You never
answer the same question twice, and the AI still sees everything fresh. The saved decisions are a list you
can read and change, not a hidden profile.

**How it works, in short.**

- The AI does not read thousands of messages one by one. Cockatoo works out the facts from the headers in
  DuckDB (per sender: how many, how many unread, how old, ever opened) and gives the AI those summaries and a
  few sample subjects. That is fast, cheap, and the only mail data that leaves the machine: senders,
  subjects and counts, to Anthropic, with your key. No message bodies.
- The AI answers with structured proposals ("these senders, this action"). Cockatoo checks each one
  against the index, shows it, and runs it only on your click. Subjects and sender names are written by
  strangers, so nothing the AI says ever triggers an action by itself.
- "Delete" moves mail to Gmail's Trash, where it can be restored for 30 days. Cockatoo never deletes
  permanently. "Spam" moves it to Gmail's Spam.

**The steps, each one something to try before the next.**

1. **The whole inbox, at a glance.** Read the headers of every inbox message (not just the last 24 hours)
   and show the shape of it: how much per year, the biggest senders, what is unread. Read-only, no AI.
   *Built 2026-11-24: the Clean up screen.*
2. **Delete and spam, for real.** The row actions and the batch: "everything before 2020", "everything
   from this sender", previewed with a count and confirmed. This covers "delete what goes back way back"
   without any AI, and it is the plumbing the AI's proposals will use. With it, finding the senders that say
   how to unsubscribe, and opening that page (or email) with one click. *Built 2026-11-24.*
3. **The cleanup conversation.** The AI session over the summaries: it asks, proposes batches, you decide.
4. **Memory.** Decisions saved with their reasons; the next session applies them to new mail first and asks
   only about the rest.
5. **Unsubscribe for you.** Cockatoo sending the unsubscribe itself (the standard one-click request, or
   the email) instead of opening the sender's page. Last, because it sends something out on your behalf.

**(proposed)** The AI session runs in the backend on your machine: a loop over the Claude API with a few
tools that read the summaries in DuckDB. The alternative is Anthropic's hosted agents, which run the loop
for us but keep the session's history on Anthropic's side; with mail data and a product for others,
running it locally fits rule 3. To decide before step 3.

## 10. Not decided yet

1. **Bodies.** Are message bodies stored locally (works offline, searchable, larger file) or fetched when a
   message is opened? Attachments are not stored: they are written to your own folders when you save them.
2. **Sending.** The brief says display and manage. Is writing and sending mail in the first version?
3. **Google sign-in for everyone.** Built for one Workspace (an Internal app, the user's own Google Cloud
   client). For strangers: one cockatoo client, External, through Google's review. See section 6.
4. **One inbox or one per account.** With several accounts, is mail shown together or separately? Shown
   together for now, each row labelled by its account.
5. **Where the cleanup AI runs.** On your machine or Anthropic's hosted agents. See section 9.
6. **The look.** Built from `../PRODUCT.md` with Impeccable. Screenshots shared are inspiration, not a
   design to copy.

## 11. Not in scope for now

- Mail providers other than Gmail, and AI providers other than Anthropic. The seams are there; the second
  ones come later.
- Anything on a server: no hosted sync, no accounts with us, no telemetry.
- Calendar, contacts and chat.
- Mac and Linux. The window and the credential store are Windows-first, as in kakapo.

## 12. Decisions log

| Date | Decision |
|---|---|
| 2026-11-24 | Stack as kakapo: Go backend, React frontend embedded in one exe, DuckDB beside it. |
| 2026-11-24 | Gmail is the first mail provider. Anthropic is the first AI provider, with the user's own key. |
| 2026-11-24 | DuckDB keeps what the app knows about the user's mail. |
| 2026-11-24 | Phase one: the app opens on a setup form when there is no account. Personal Gmail first, over IMAP with an app password. |
| 2026-11-24 | Account details are saved in DuckDB, the password encrypted with Windows DPAPI. This replaces the earlier idea of Windows Credential Manager. |
| 2026-11-24 | AI: an on/off switch and the user's Anthropic key, encrypted the same way. Official Anthropic Go SDK, Claude Opus 5.5, refusal fallbacks on. |
| 2026-11-24 | Inbox, first step: the last 24 hours, headers only (sender, subject, arrival time, read or not), read-only over IMAP and kept in DuckDB as a cache. One list for all accounts, each row labelled by account when there are several (provisional; see section 10, question 4). |
| 2026-11-24 | Cockatoo is a product for other people, not only for us. |
| 2026-11-24 | The UI is flat: every action visible on the thing it acts on, no menus or sub-menus, multi-select allowed, AI in the background. |
| 2026-11-24 | Cleanup step 1: cockatoo keeps a copy of every inbox message's headers in DuckDB, filled by a background sync (newest first, in saved batches; removals only after a complete pass). The Inbox list and the Clean up overview both read from it. |
| 2026-11-24 | Every action on a mailbox is audited, insert-only: a record before (what, which messages, criteria, source) and a result after. A page to read it comes later. |
| 2026-11-24 | Cleanup step 2: Trash (a sender's mail, or a year's) and Spam (a sender's mail) on the rows, confirmed in the row with the exact count; moves only over IMAP MOVE into the folder Gmail marks `\Trash` or `\Junk`, never a created one. Unsubscribe: senders whose mail carries List-Unsubscribe are shown, and the link opens on the user's click; cockatoo itself fetches nothing. |
| 2026-11-24 | Google sign-in for work accounts: OAuth desktop flow (loopback, PKCE) in the default browser, refresh token encrypted with DPAPI, IMAP over XOAUTH2 with a fresh access token per connection. The Google Cloud client is the user's own for now, entered in the app. |
| 2026-11-24 | AI on the Inbox, "what needs you now": one request with a fixed answer shape over the unread mail of the last 30 days (up to 300, headers and sender facts only); code checks every pick against what was sent; every AI request recorded insert-only. |
| 2026-11-24 | "What needs you now", revised after real use: headers and subjects alone were not enough. Cockatoo now reads recipients, In-Reply-To and list / automated markers, and the Sent folder's headers; it leaves out mail already answered and automated mail from people the user never writes to (never a reply to their own mail or mail from someone they write to); the first 600 characters of each remaining message go to the AI when the user allows it. The text is not stored. |
| 2026-11-24 | Save attachments: save folders are a name and an existing path. In the Inbox, tick messages and Save attachments; choose a folder, open or make a subfolder (this month and today are one click), save. Files are never overwritten, carry the internet mark, and stay inside the folder. Audited, with each file written. An exception to rule 8.2, as asked: Save attachments shows only once messages are ticked (rule 8.3); while none are, the bar above the list says what ticking does. |
| 2026-11-24 | Mail you have dealt with shows green on its Inbox row, saying how: attachments saved (with the folder), replied (from the Sent folder) or marked done (a Done button on every row and AI pick, Mark done for several, Not done to take it back). Done is local, never a change in Gmail. It is not a new state: it is read from the audit (the latest mark wins) and the Sent copy. Mail dealt with is left out of "what needs you now". |
| 2026-11-24 | The first AI feature is inbox cleanup: a conversation that asks and proposes, decisions saved with reasons and reused next time, built in five steps (section 9). Delete means Gmail's Trash, never permanent; the AI never acts on its own. |
