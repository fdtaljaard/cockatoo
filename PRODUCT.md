# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

The UI is a web page served by a local Windows program (`cockatoo.exe`) and shown in an Edge or Chrome
app window. Its design language is web, not native Windows.

## Users

People with busy mailboxes who want them under control without learning a complicated mail program: often
several accounts, thousands of messages and hundreds unread, built up over years. They open cockatoo to see
what came in, deal with it quickly, and clean up what has piled up. Cockatoo is a product for other people,
not only for its makers: a stranger must be able to install it, add an account and get value without help.

## Product Purpose

An extremely simple email client: your mail accounts in one place, with AI working in the background. It
exists because mail clients are deep and busy, and because cleaning up a large mailbox by hand is tedious.
Success is a user who can see their mail and act on it at a glance, and who can bring a messy inbox under
control in one guided session, with the next session shorter than the first.

## Positioning

Flat instead of deep: every action is visible on the thing it acts on, with no menus or sub-menus. AI that
suggests and asks instead of a table of rules: it looks at the mailbox, asks questions about what it sees,
proposes, and remembers the user's decisions with their reasons so they never answer the same question
twice. The user always decides; the AI never acts on its own.

## Operating Context

- One Windows program; data stays on the user's machine in a DuckDB file beside the exe.
- Mail accounts are added with the provider's own credentials (Gmail first, over IMAP with an app
  password; Google's own sign-in is a near-term need).
- AI uses the user's own Anthropic API key. Only what an AI feature needs leaves the machine, sent straight
  to Anthropic.
- Typical session: open, scan the inbox, act on rows directly, occasionally run a cleanup session.

## Capabilities and Constraints

- Built: Gmail accounts (app password or Google sign-in), the AI connection with the user's key, the
  inbox of the last 24 hours across accounts with search, the AI's "what needs your attention" picks,
  conversations read as a chat with signatures and quotes removed, plain-text replies, delete (to Trash),
  done marks, saving attachments to the user's folders, routines for recurring mail, and cleanup steps
  (whole-inbox overview, Trash, Spam, unsubscribe). Every mailbox action is audited.
- Planned first AI feature: inbox cleanup as a conversation (see `sc/SCOPE.md`, section 9).
- "Delete" means the provider's Trash, never permanent deletion. Anything that changes the mailbox runs
  only on the user's click.
- Multi-select actions are allowed; menus, sub-menus and hidden actions are not.
- Undecided: whether message bodies are stored locally, sending mail, one inbox versus one per account.

## Brand Commitments

- Name: cockatoo (lowercase in the UI).
- The look must be modern, sleek and simple, a little fun but productive (2026-11-24). It must not turn
  cartoony or childish, busy or animated, or look like Gmail or Outlook.
- The logo and its tagline are in: a white sulphur-crested cockatoo with a yellow crest holding a blue
  envelope, the navy wordmark, and "Only the emails that matter." (`sc/Cockatoo_Logo.webp`).
- Light and dark: cockatoo is open all day and follows the Windows light or dark setting.
- Voice: plain words, short sentences, no jargon.

## Evidence on Hand

- The logo file: `sc/Cockatoo_Logo.webp` (1536x1024, transparent; not yet used in the app, and not
  committed to the repository yet).
- No users, testimonials, metrics or screenshots of real use exist yet; none may be invented.

## Product Principles

1. **Simple wins.** A feature that makes a screen harder to read or a task take more steps loses.
2. **Flat, not deep.** Every action is visible where the user is looking; nothing hides behind a selection,
   a hover or a menu.
3. **The AI suggests, the user decides.** The AI prepares, asks and proposes; actions run on the user's
   click.
4. **Remember, don't re-ask.** Decisions are kept with the user's reasons and reused, and stay readable
   and changeable.
5. **The user's data stays theirs.** It lives on their machine; only what an AI feature needs goes to the
   AI provider, with their key.
