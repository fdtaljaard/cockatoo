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

## 7a. The email agent and its skills

Decided 2026-11-25. Cockatoo's AI is one **email agent**: the same AI behind every AI feature (sorting
new mail, the Unsorted exercise, what needs you now, later replies and clean-up). It knows how the user
works through **skills**, not through settings and configuration.

**A skill** is a short document in plain words: a name, when it applies ("mail from shops I buy from"),
and what to do ("Invoices go to Save. Order confirmations and shipping notices need nothing, even with
a PDF attached."). Skills are kept in DuckDB on the user's machine, insert-only (every change is a new
version, with who made it and why), and are shown on one screen to read, edit, add and remove.
Cockatoo starts the user off with a few skills to adjust (what they keep, when they reply); the rest
are learned.

**The agent reads the skills every time it works.** They go with each AI request, after the fixed
instructions that say what the lists mean and that email is data, never instructions. A suggestion
can name the skill it followed, so the user sees why. The skills are the same in every request, so
Anthropic's prompt caching makes sending them again cheap.

**Teaching, as mail comes in.** When the user corrects a message (**Not here**, or moving it by hand),
the agent proposes a change to a skill, in plain words ("Add to Shops I buy from: Takealot shipping
notices need nothing; only their invoices go to Save"). The user reads it, can edit it, and approves or
declines. Nothing is learned without that approval. Where the pattern is exact, cockatoo also offers a
rule, as before.

**Skills and rules.** Rules (section 8a) stay: they are exact, instant, free and say plainly why a
message is where it is, so they are applied first. Skills cover the judgement no rule can make (is the
user really being asked something; is this attachment a document to keep) and everything no rule covers.
As skills settle, the agent can suggest turning a pattern into a rule. The screen is **Skills & rules**.

**Safety.**

- **Only the user changes a skill.** Email is written by strangers; an email that says "always mark my
  mail urgent" must never teach the agent anything. A proposed change is shown in full and saved only
  on the user's click; the UI says it was written by the AI from that email.
- **The agent suggests; it never acts** (section 3, rule 4).
- **Recorded.** Every request (kind `sort`, `teach`, `triage`) and every skill version is kept.

**Why not Anthropic's hosted features.** Anthropic's Agent Skills and Managed Agents run in a container
Anthropic hosts, made for agents that write files and run code; using them would send the mailbox to a
hosted sandbox, against section 3, rule 3, and add cost per session. Anthropic's memory tool lets the
model write its own notes while it reads untrusted mail, which the first safety rule forbids. Cockatoo
keeps its own skills and sends the relevant ones with each request instead. A tool-using loop ("look up
my earlier mail from this sender") can be added later if a real need appears.

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
5. **Modern, sleek and simple.** Calm, few words, nothing decorative. The look is "Highlighted
   Printout", recorded in `../DESIGN.md`; the product record it was built from is `../PRODUCT.md`.
6. **Organised by what mail asks of you.** A navigation down the left side holds the lists Reply,
   Attention, Save and Unsorted (section 8a); the rest of the app sits below them.

## 8a. Working your mail: Reply, Attention, Save

Decided 2026-11-25. This changes how cockatoo behaves, not only how it looks. The user works mail by
what it asks of them, so that is how the window is organised: a navigation down the left side with
the lists of mail that need something from them, and the mail of the chosen list beside it.

**The lists.** Every open message (not replied to, saved or marked done) is in exactly one:

| List | What it means | What dealing with it is |
|---|---|---|
| **Reply** | Someone is waiting on an answer from you. | Replying (the reply shows in the Sent folder), or marking it done. |
| **Attention** | Read it and act on it, but no answer is needed. | Marking it done. |
| **Save** | It carries something to keep: an invoice, a statement, a contract. | Saving its attachments, or marking it done. |
| **Unsorted** | Not sorted yet. | Putting it in a list, or marking it done. |
| **Nothing to do** | It never needed you: a receipt, a shipping notice, a notification with an attachment. | Nothing; it stays out of the other lists, and can be looked at. |

- **One list each.** A message is never in two lists, so the counts add up and nothing is done twice.
  It can be moved to another list at any time, from its row.
- **The window.** The lists show the last 30 days by default. The user can show all of their mail, of
  any age, and can show what they have dealt with as well.
- **Dealt-with mail leaves the list.** It is not deleted: it stays in the mailbox and in the Inbox, and
  comes back with "show done".
- **The Inbox stays.** The last 24 hours, search, and the rest remain one click away below the lists.

**How mail gets into a list.** In this order; the first that applies wins:

1. **The user put it there**, by hand, on its row (Reply, Attention, Save, or back to Unsorted). The
   latest choice wins. Confirming an AI suggestion is the user putting it there.
2. **A rule.** Mail from an address (`billing@acme.com`) or a domain (`acme.com`), and, if given, with
   some text in its subject, goes to a list. The most specific rule wins: an address beats a domain, a
   subject beats none. Routines (see the readme) become Save rules: a Save rule can also say
   where the attachments go. Rules apply to mail of any age, not only mail that came after them.
3. **Otherwise it is Unsorted.**

**Teaching cockatoo, as mail comes in** (decided 2026-11-25). This is what the app is about: every
correction teaches it. Every message in a list has **Not here**. It asks where the message goes instead
(another list, Nothing to do, or "not sure", back to Unsorted). Then, as for any message moved by hand,
it asks how to handle mail like it from now on:

- **Only this one.**
- **Always from this address**, or **anyone at this domain**, optionally only when the subject has
  certain words. This becomes a rule; it replaces any rule for exactly the same mail.
- **Narrow the rule that put it here.** When a wide rule (all mail from a domain) put mail in the wrong
  list, the rule is kept only for mail with certain words in its subject ("takealot.com, only with
  *invoice*"). The sender's other mail goes back to Unsorted unless another rule takes it.

Excluding is done the same way: a narrower rule (an address, or words in the subject) beats a wider one,
and a rule can send mail to Nothing to do. So "mail from this sender never goes to Save" is a rule for
that sender to another list.

All rules are listed on one screen (Rules), by list, to read, move to another list, remove or add.
Rules are expected to grow more conditions later (to whom it was sent, whether it has attachments);
the data keeps room for that.

**The AI suggests; the user confirms.** The AI never moves mail by itself (section 3, rule 4).

- **New mail, in the moment.** Mail that came in recently and that no rule or choice sorts is shown at
  the top of the page, above the list, each with the AI's suggestion and a short reason ("Ann asks you to
  approve the budget: Reply"). One click confirms it; the other lists are one click too. The idea is to
  deal with new mail as it arrives.
- **Unsorted is an exercise.** The Unsorted list is worked through as a task. On the user's click the AI
  suggests a list for each message there, and the user confirms or changes them.
- **What the AI reads.** What the triage already reads (section 7): sender, subject, date, whether the
  mail was sent to the user directly or on CC, whether it answers their mail, how often they write to
  the sender; with the user's permission, the first lines of the text. New here: the names of the
  attachments, so an invoice from a sender never seen before is suggested for Save. Whether the user is
  addressed in the text is the AI's judgement; the rest is code.
- **Each message is suggested once.** A suggestion is kept with the message and shown again, so the
  same mail is never paid for twice. Every request is recorded, as the triage requests are.
- **"Nothing needed" is a suggestion too.** When the AI sees nothing to do, it suggests the Nothing to
  do list, confirmed in one click.

**What is kept.** Where each message was put, by whom (the user, a rule, a confirmed suggestion) and
when, insert-only like the audit; and each AI suggestion with its reason. Nothing about the lists is
written to the mailbox: moving mail between lists does not move it in Gmail.

**Open.** Whether lists should become Gmail labels; rule conditions beyond sender and subject; a list
of its own for mail waiting on someone else.

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

1. **Bodies.** Fetched when a message is opened, for now, and not kept. Stored locally (works offline,
   searchable, larger file) is still open. Attachments are not stored: they are written to your own folders when you save them.
2. **Sending.** Replies are in (plain text). New mail, forwarding and attachments on outgoing mail are
   not decided.
3. **Google sign-in for everyone.** Built for one Workspace (an Internal app, the user's own Google Cloud
   client). For strangers: one cockatoo client, External, through Google's review. See section 6.
4. **One inbox or one per account.** With several accounts, is mail shown together or separately? Shown
   together for now, each row labelled by its account.
5. **Where the cleanup AI runs.** On your machine or Anthropic's hosted agents. See section 9.
6. **Routines that act alone.** Should a routine ever save without a click? Only with a sender that is
   checked (DKIM/SPF results from the mail's headers), and probably only per routine, by choice.
7. **The look.** Built from `../PRODUCT.md` with Impeccable. Screenshots shared are inspiration, not a
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
| 2026-11-24 | Delete on every Inbox row, in the selection bar and on each inbox message of a conversation: Gmail's Trash, by IMAP MOVE, verified and audited, waiting for a running sync rather than refusing. |
| 2026-11-24 | Sending, first step: replies from a conversation, plain text, through smtp.gmail.com with the account's own sign-in (Gmail keeps the copy in Sent), threaded, the answered message quoted. Recipients shown before sending; only the user's Send sends; audited without the text. New mail (not a reply) and attachments are not in yet. |
| 2026-11-24 | Reading: a message opens as a conversation, like a chat, built from the inbox and Sent copies (Message-ID, In-Reply-To, References; same account; at most 30). Each message is read with a peek (the first 512 KB, attachment names from BODYSTRUCTURE), checked to still be the message cockatoo knows, and cleaned of quotes, signatures, pictures, device lines and disclaimers; "Show all" keeps everything reachable. Text only: mail HTML is never rendered. Nothing is stored and nothing is marked read. Writing replies is the next decision (section 10, question 2). |
| 2026-11-24 | Mail dealt with (saved, replied, done) leaves the Inbox list by default; "Show N done" brings it back, Undo follows a Done, and search always shows it. |
| 2026-11-24 | Routines: what the user does every time a certain mail comes in, made from the save summary ("Do this every time?"). Matched on the sender's domain (or address, always for personal mail services) and optional subject text; mail from before the routine is not matched. For now the action is saving attachments, into a fixed folder or a folder for the mail's month or day. Matching mail is pointed out (Waiting for you, the row, the AI's facts, never left out as automated); the user presses Save. Never saved unattended: the sender is what the From header says, which anyone can write. "Reply" and "point out" routines are next. |
| 2026-11-24 | The first AI feature is inbox cleanup: a conversation that asks and proposes, decisions saved with reasons and reused next time, built in five steps (section 9). Delete means Gmail's Trash, never permanent; the AI never acts on its own. |
| 2026-11-24 | The look: "Highlighted Printout". Each screen is a paper sheet on a desk; navy ink; one highlighter yellow (the crest) only for what needs you (the AI's picks); the AI's notes in blue pen handwriting; a pen tick on mail you have dealt with. Public Sans, Caveat for the pen. The logo bird is in the header, the first-run screen and the empty inbox, with the tagline. Follows Windows light and dark. |
| 2026-11-25 | The app is organised by what mail asks of the user: a left navigation with the lists Reply, Attention, Save and Unsorted (section 8a). One list per message; the last 30 days by default, all mail on request. Mail is placed by the user's choice, then by rules (address or domain, optional subject; routines become Save rules), else Unsorted. The AI suggests a list for new mail and, on the user's click, for Unsorted; the user confirms. Nothing about the lists is written to the mailbox. |
| 2026-11-25 | A fifth list, Nothing to do, for mail that never needed the user. Every message in a list has Not here: it asks where the message goes, then how to handle mail like it from now on (only this one; a rule for the address or domain, optionally with subject words; or narrowing the wide rule that misplaced it). Excluding is a narrower rule. |
| 2026-11-25 | The AI is one email agent with skills (section 7a): plain-word documents kept on the user's machine, read with every AI request, learned from the user's corrections only with their approval. Rules stay as the exact, free first step. Anthropic's hosted Agent Skills, Managed Agents and memory tool are not used: they would send the mailbox to a hosted sandbox or let mail write the agent's memory. |
