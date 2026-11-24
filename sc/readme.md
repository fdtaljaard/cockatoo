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
- **Headers only.** Sender, subject, arrival time and whether it is read. Message bodies are not copied:
  one is read only when you ask for something that needs it (the first lines, for the AI; attachments,
  when you save them) and is not kept.
- **Read-only.** The inbox is opened read-only over IMAP, so nothing is marked as read.
- **Inbox only.** Counts cover mail in the inbox, one per message across all Gmail tabs, so they can
  differ from Gmail's own numbers.
- **Kept in DuckDB.** In the `message` and `inbox_sync` tables, a cache of what Gmail holds. Senders and
  subjects are stored as they are, not encrypted. Removing an account removes its copy.
- **Odd messages.** A subject in an unknown charset is kept as it was sent. A message the server cannot
  give is skipped and counted, and the sync carries on.
- **Search.** The box above the list searches cockatoo's copy of the whole inbox, not only the last
  24 hours: every word must be in the sender's name or address or in the subject (any case). The
  newest 200 are shown. Message text is not searched, since only headers are kept. The account buttons
  apply; Escape or the cross clears it.
- **Several accounts.** One Inbox list, each row naming its account; Clean up shows one account at a
  time, chosen with the buttons above it.

## Reading mail: the conversation

Click a message (in the list, in search results, under Waiting for you or among the AI's picks) to
open it as a conversation, like a chat: what each person wrote, oldest first, your own replies on
the right. **Inbox** (or Escape) goes back to the list where you were.

- **Which messages.** The ones linked to it by Message-ID and In-Reply-To (and the References it
  names), in your inbox and your Sent folder, in the same account. Mail you archived in Gmail is not
  in either, so it is not shown; **Open in Gmail** shows the whole thread there. At most 30, the
  newest.
- **Uncluttered.** Quoted earlier messages, signatures, "Sent from my iPhone" lines, legal
  disclaimers and the contact details under a sign-off are left out, and pictures are never shown.
  Quote and signature blocks are found by the marks Gmail, Outlook, Apple Mail, Thunderbird, Yahoo
  and Proton put on them; text rules cover plain mail. Answers written between quoted lines, and
  forwards, are kept as they are. **Show all** on any message shows everything it said.
- **Text only.** Mail HTML is turned into text and never shown as HTML, so nothing loads from the
  internet (no tracking pixels). Web addresses in the text are links; other links show as their text.
- **Read with a peek.** Opening a message does not mark it as read, in Gmail or in cockatoo, and
  nothing read is kept. The first 512 KB of each message is read; attachments are named from the
  server's description of the message, not downloaded.
- **Checked.** Before a message is shown, cockatoo checks it is still the message it knows by that
  number; if the mailbox changed, it says so instead.
- **Actions.** Each inbox message in it has **Save attachments** (opened at its routine's folder when
  it has one), **Done** and **Delete**, as on its row.

### Replying

The box at the foot of a conversation replies to the newest message from someone else. It shows who
the reply goes to before you send: the sender (or the address they ask replies to go to), and with
**Reply all** everyone else it went to, never yourself.

- **Send** (or Ctrl+Enter) is the only thing that sends. Nothing is ever sent by the AI.
- Plain text, through Gmail's own server (`smtp.gmail.com`, with the same app password or Google
  sign-in as reading), so it comes from your address and Gmail keeps it in Sent. It is in the same
  thread (In-Reply-To and References), with the message answered quoted below your text.
- It shows in the conversation at once; from the Sent folder after the next check.
- Before sending, cockatoo reads the message answered again and checks it is still the one you saw;
  if the mailbox changed, nothing is sent.
- **Audited.** Each reply is recorded (`reply`): who to, about what, and whether Gmail took it. The
  text is not kept by cockatoo; Gmail's Sent folder has it.

## What needs you now (AI)

On **Inbox**, **Ask AI** asks Claude which unread mail needs a reply or an action from you, and which
you can skim, for the account buttons' choice (All or one account). It answers with a sentence and a
short list, **Now** and **Today**, each with what you have to do and the facts behind it ("Answers your
message · To you · You've written to them 14 times").

**First, without the AI.** Cockatoo sorts your unread mail of the last 30 days from facts it reads from
the headers and your **Sent** folder (synced with the inbox, headers only):

- sent to you directly, or only on CC, or to a list;
- whether it answers a message you sent (its In-Reply-To is one of your sent Message-IDs);
- how often you have written to the sender (your aliases count as you: they are the From of your sent mail);
- whether you already replied to it;
- list or automated mail: `List-Id`, `List-Unsubscribe`, `Precedence: bulk/list/junk`,
  `Auto-Submitted`, or a no-reply / notifications sender.

Mail you already answered, saved the attachments of or marked done is left out, and so is automated mail from someone you never write to that
does not answer you. Only on that evidence: a reply to your own mail, or mail from someone you write to,
is never left out, even when it comes through a list. How much was left out is shown under the answer.

**Then the AI**, for what is left (newest first, at most 80): the sender, subject, date, the facts above,
and, if **Include the first lines of each message** is ticked, the first 600 characters of what the
sender wrote (read with a peek, so nothing is marked read; quoted replies cut off). The text is sent to
Anthropic with your key and not kept by cockatoo; the record keeps only that text was sent and how much.

- **What comes back.** A fixed answer shape (structured outputs). Code keeps only picks that name a
  message that was sent, once each; an answer that cannot be read is an error, never "nothing needs
  you". Senders, subjects and text are written by strangers: they go in a marked data block, and
  nothing in an answer acts.
- **Kept.** The answer shows again when you come back, with mail read since taken out; **Ask again**
  asks anew. Every request is recorded, insert-only, in `ai_request` (when, which accounts, model,
  tokens, how much was left out, whether text was sent, the answer or the error) and `ai_request_item`
  (each message sent).
- **Cost.** Billed to your Anthropic key; the panel shows the tokens each answer took. It waits while
  cockatoo is checking your mail, because read states and the Sent folder are brought up to date at the
  end of that check.
- **What it cannot know.** Whether you replied from another app before cockatoo read your Sent folder,
  and anything in a message beyond its first lines.

## Delete

Every row in the Inbox (and every inbox message in a conversation) has a **Delete** button, and
**Delete** in the bar deletes the ticked messages. Delete moves to Gmail's Trash, which Gmail keeps
for 30 days; it is never permanent. It moves only those messages, by IMAP MOVE, checks with the server
what actually left the inbox, and is audited like the other moves (`trash`, criteria
`{"kind":"messages"}`). If cockatoo is in the middle of checking the mailbox, the delete waits for it.

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

## Save attachments

**Folders** holds the places on disk attachments go to: a name and the path of a folder that already
exists, for example *Expenses* at `C:\Users\you\Documents\Expenses` (in File Explorer, right-click the
folder and choose **Copy as path**). Removing one only removes it from cockatoo; the folder and its files
stay.

In the **Inbox**, tick one or more messages (or the box above the list for all of them) and press **Save
attachments**:

1. Choose a folder. The ones whose path is no longer there are shown but cannot be chosen.
2. Its subfolders are listed; open one, or make one. This month (`2026-11`) and today (`2026-11-24`) are
   one click each; any other name can be typed. The bar at the top goes back up.
3. **Save to ...** saves every attachment of the chosen messages there, and says what was saved and what
   was not, per message.

- **What is saved.** Files sent as attachments, and named files shown in the message (a PDF, say).
  Pictures that are part of the message itself (a logo, a signature) are left out and listed as such. A
  forwarded email is saved as an `.eml` file.
- **Never overwritten.** A file with the same name gets ` (2)`, ` (3)` and so on. Names are made safe for
  Windows (no `\ / : * ? " < > |`, no `CON` or `NUL`).
- **Marked as from the internet.** Each file carries the same mark a browser download does, so Windows and
  Office treat it with the same care (Protected View, SmartScreen).
- **Inside the folder only.** Subfolder names cannot climb out (`..`), and cockatoo refuses to save through
  a shortcut folder (a symbolic link or junction) inside it.
- **Read-only.** Messages are read with a peek, so nothing is marked as read. A message over 50 MB is
  skipped and said so. A message that has left the inbox since the last sync is skipped.
- **Audited.** Each save is recorded like the other actions (below), with every file written.

## Routines: mail you deal with the same way every time

A routine is something you do every time a certain mail comes in. For now that is saving its
attachments: "every time mail comes from acme.com with *invoice* in the subject, save its attachments
to Expenses, in a folder for the month".

- **Making one.** After a save, the summary asks **Do this every time?** for each sender whose files
  were saved. Choose the whole domain (`acme.com`, the default) or the one address
  (`billing@acme.com`; the only choice for gmail.com, outlook.com and the like), and if you like, a word
  the subject always has (left empty, it matches any subject). **Remember** saves it.
- **The month or day.** If you saved into a folder named like a month (`2026-11`) or a day
  (`2026-11-24`), the routine saves each mail into the folder for *that mail's* month or day (the date
  it arrived, in this computer's time zone), next to it, and makes the folder when it is not there.
  Saved anywhere else, it saves to that same folder every time.
- **What it does.** Mail a routine is for, arriving after the routine was made, shows in **Waiting for
  you** at the top of the Inbox (the last 60 days, read or not) and on its row in the list, with
  **Save**: the save dialog opens at the right folder, and you press **Save to ...**. Once saved (or
  marked done) it leaves the list. **What needs you now** always includes it, even though invoices look
  like automated mail, and tells the AI what the routine is, so it can say "save its attachment".
- **Never on its own.** A routine matches on the sender's address as the mail states it, which anyone
  can write; so cockatoo points the mail out and you press Save. It does not save by itself.
- **Routines** lists them, and removes one. A routine whose save folder was removed says so.
- Kept in DuckDB, in the `routine` table. A one-click save is audited like any other.

## Done: mail you have dealt with

Once you have dealt with a message, its row in the Inbox turns green and says how:

- **Saved 2 files to C:\...\Expenses\2026-11**, after you saved its attachments (the folders they went
  to are listed);
- **Replied**, when the Sent folder holds your reply to it (from Gmail or anywhere else; it shows after
  the next sync);
- **Done**, when you pressed **Done** on the row, or ticked several and pressed **Mark done**. **Not done**
  takes it back.

Mail you have dealt with leaves the Inbox list. The header counts it, and **Show 3 done** brings it back
(green; **Hide 3 done** hides it again, and the choice is kept). After **Done** or **Mark done**, **Undo**
takes it back. Search always shows it. It also drops out of **What needs you now**, and its Done button
is on each of the AI's picks too.

Done is cockatoo's own mark: nothing changes in Gmail, and the message stays unread and in the inbox.
Nothing is stored beside the audit: saved, done and not done are read from it (the latest mark wins),
and replied from the copy of the Sent folder. A message is known by its UID, so if Gmail ever renumbers
the inbox (rare), the marks on older mail no longer show; the audit still holds them.

## Audit

Every action is recorded in DuckDB before anything changes and again after, insert-only:

- `action`: when, which account (by address, so the record outlives the account), what (`trash`, `spam`,
  `unsubscribe`, `save_attachments`, `done`, `not_done`, `reply`), the criteria (`{"kind":"sender","address":...}` or `{"kind":"year","year":...}`), the
  description you confirmed, who asked (`you`; later `ai`), the folder used, how many messages matched.
- `action_item`: each message the action was for: UID, Message-ID, sender, subject, date.
- `action_result`: when it finished, `done`, `partial`, `failed` or `opened`, how many moved, which did
  not, and the error.
- `action_file`: for a save, each file written: the message's UID, the full path and the size.

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

## Progress

Anything that waits on Gmail or Claude shows that it is working:

- A thin bar across the top of the window while cockatoo waits on something you asked for (it shows
  only after a moment, so quick answers do not flicker it). Polls of the sync's state are left out.
- Where you asked: a spinner on the button (Send, Save, Delete, Mark done, Ask AI, Move to Trash, Test
  connection, Save key), and a line saying what is happening, with the seconds counted after the
  first few (opening a conversation, asking Claude, saving attachments).
- The sync: a spinner while it checks, and a bar while it reads a known number of messages.
- A message on its way to Trash is dimmed on its row, its button turning, until Gmail confirms.

Spinners and the bar stand still for people who ask their system for less motion.

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
internal/triage/  what needs you now: the unread mail sent to the AI, its answer checked and recorded
internal/ai/      the Anthropic client, and the AI settings that hold the key and turn it on
internal/mail/    IMAP: sign in (password or Google token), count, list and read headers; move to Trash or Spam;
                  read a message's text or its attachments; clean a message for reading
internal/files/   save folders on disk: safe names, subfolders, writing a file without overwriting
internal/secret/  encrypts passwords with Windows DPAPI before they are stored
internal/store/   DuckDB: schema migrations, single writer, queries
web/              React + TypeScript + Tailwind + shadcn/ui; web/dist is embedded by web/embed.go
```

A schema change is a new entry at the end of `migrations` in `internal/store/store.go`; each runs once.
