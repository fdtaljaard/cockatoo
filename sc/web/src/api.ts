// Typed client for the Go backend. Hand-written for now; generate from OpenAPI when drift hurts.

export interface Health {
  ok: boolean
  time: string
  version: string // the build stamp of the running exe
}

// A saved mail account. The password is never sent back.
export interface Account {
  id: number
  provider: "gmail"
  name: string
  email: string
  auth: "password" | "google" // an app password, or Google's own sign-in
  createdAt: string
}

// Google's own sign-in setup: the Google Cloud client cockatoo signs in with. The secret never comes back.
export interface GoogleSettings {
  configured: boolean
  clientId?: string
}

// A started Google sign-in.
export interface SignInStart {
  state: string
  url: string // Google's sign-in page, for a link when the browser did not open
  opened: boolean // cockatoo opened it in the default browser
}

export interface SignInStatus {
  state: "waiting" | "done" | "failed"
  account?: Account
  error?: string
}

// What the setup form sends.
export interface AccountInput {
  name: string // shown on the account; the address when left empty
  email: string
  password: string // a Google app password
}

// What the mail server says about the inbox.
export interface Inbox {
  messages: number
  unseen: number
}

// One inbox message's header, as kept from the last refresh. No body.
export interface MailMessage {
  accountId: number
  uidValidity: number
  uid: number
  messageId: string
  fromName: string
  fromAddr: string
  subject: string
  receivedAt: string // UTC
  seen: boolean
  handled?: Handled // what you have done about it; absent when nothing
  routine?: RoutineMatch // the routine for it, while it is not dealt with
  sort?: SortInfo // the list it is in, where the lists are shown
  suggestion?: Suggestion // the AI's idea of its list, when it has been asked
}

// The lists mail is sorted into, by what it asks of you (SCOPE.md, section 8a).
export type ListName = "reply" | "attention" | "save" | "unsorted" | "ignore"

// The list a message is in and how it got there.
export interface SortInfo {
  list: ListName
  by?: "you" | "ai" | "rule" // absent while it is unsorted
  ruleId?: number
}

// The AI's suggestion for a message: a list, or "done" for mail that needs nothing.
export interface Suggestion {
  list: Exclude<ListName, "unsorted"> | "done"
  reason: string
  skill?: string // the skill of yours it followed, by name
  at: string
}

// What the email agent knows about how you work, in your own words (SCOPE.md, section 7a).
export interface Skill {
  id: number
  name: string
  applies: string // when it applies
  instructions: string // what to do
  madeBy: "you" | "agent" | "cockatoo" // who made this version: you, the agent (approved by you), or a starter
  note?: string
  updatedAt: string
}

export interface SkillInput {
  id: number // 0 for a new skill
  name: string
  applies: string
  instructions: string
  note: string
  fromAgent: boolean // the agent proposed it and you approved it
}

// What the agent proposes to remember from a correction. Nothing is saved until you approve it.
export interface Proposal {
  action: "edit" | "new" | "none"
  skillId?: number
  name?: string
  applies?: string
  instructions?: string // the whole skill after the change
  before?: string // with an edit: the instructions before it
  summary: string
}

// One list, with the counts for the navigation and the new mail no rule or choice has sorted yet.
export interface ListsView {
  list: ListName
  all: boolean // all mail, of any age; otherwise the last 30 days
  since?: string
  counts: Record<ListName, number> // open mail in each list
  done: number // mail dealt with in this list, shown only on request
  messages: MailMessage[]
  new: MailMessage[]
}

export interface SuggestResult {
  suggestions: { key: MessageKey; suggestion: Suggestion }[]
  sent: number // sent to the AI now; 0 when every message was asked about before
  leftOut: number // over the limit of one request
  textError?: string
}

// What you have done about a message, read from the audit and the Sent folder.
export interface Handled {
  savedTo?: string[] // the folders its attachments were saved in
  savedFiles?: number
  savedAt?: string
  doneAt?: string // marked done, and not taken back since
  replied?: boolean // the Sent folder holds a reply to it
}

// The last 24 hours of every account's inbox, newest first, from cockatoo's copy.
export interface InboxList {
  since: string
  messages: MailMessage[]
  waiting: MailMessage[] // mail of the last 60 days a routine is waiting on, newest first
}

// One message of an opened conversation.
export interface ConversationMessage {
  key?: MessageKey // inbox mail only: what Save and Done work on
  folder: "inbox" | "sent"
  mine: boolean // you sent it
  opened: boolean // the message you opened
  fromName: string
  fromAddr: string
  at: string
  seen: boolean
  text: string // what the person wrote: no quotes, signature or pictures
  full: string // everything it said, as text
  cut: boolean // longer than what was read
  attachments: { name: string; size: number }[]
  handled?: Handled
  routine?: RoutineMatch
  problem?: string // why it could not be shown
}

// An opened message and the messages around it, oldest first.
export interface Conversation {
  subject: string
  messageId: string // the opened message's, to find it in Gmail
  account: string // the account's address
  messages: ConversationMessage[]
  older: number // older messages left out
  sentPartial: boolean // the Sent folder is not fully read: replies may be missing
  reply?: ReplyPreview // who a reply goes to: the newest message from someone else
}

// Who a reply to a message goes to; cc is added with "Reply all".
export interface ReplyPreview {
  key: MessageKey
  to: string[]
  cc: string[]
}

// A reply that was sent.
export interface ReplyResult {
  messageId: string
  to: string[]
  cc: string[]
  at: string
}

// What deleting chosen messages did: moved to Gmail's Trash.
export interface DeleteResult {
  moved: number
  notMoved: number
  outcome: "done" | "partial" | "failed" | "unknown"
  error?: string
}

// What a search of the inbox found: newest first, at most 200.
export interface SearchResult {
  query: string
  messages: MailMessage[]
  more: boolean // there were more; the newest are shown
}

// Something you do every time a certain mail comes in. For now: save its attachments.
export interface Routine {
  id: number
  from: string // an address ("billing@acme.com") or a domain ("acme.com")
  subject?: string // text the subject contains; absent for any
  action: Exclude<ListName, "unsorted"> // the list it puts mail in
  folderId: number // Save rules only, and optional: 0 for none
  path: string // inside the save folder; with dated, where the dated folder goes
  dated?: "month" | "day" // a subfolder named for the mail's date
  createdAt: string
  folderName?: string
  missing: boolean // its save folder is gone
}

// What a routine wants done with one message: where its attachments go.
export interface RoutineMatch {
  routineId: number
  action: "save"
  folderId: number
  folderName?: string
  parent: string // where the dated folder goes, or the target itself
  name?: string // the dated folder for this message, "2026-11"
  missing?: boolean
}

// A routine as you set it up.
export interface RoutineInput {
  from: string
  subject: string
  list?: Exclude<ListName, "unsorted"> // absent is "save"
  folderId: number // 0 for none
  path: string
  dated: "" | "month" | "day"
}

// What the sync is doing. It copies every account's inbox headers into cockatoo, in the background.
export interface SyncStatus {
  running: boolean
  accounts: AccountSync[]
}

export interface AccountSync {
  accountId: number
  email: string
  state: "waiting" | "listing" | "reading" | "sent" | "done" | "failed"
  read: number // headers read so far in this sync
  toRead: number // headers this sync has to read
  skipped: number // messages the server could not give
  error?: string
  sentError?: string // the Sent folder could not be read; the inbox is fine
}

// The shape of one account's whole inbox, from cockatoo's copy.
export interface Overview {
  accountId: number
  sync: { serverTotal: number; complete: boolean; syncedAt: string | null }
  messages: number
  unread: number
  oldest: string | null
  years: { year: number; messages: number; unread: number }[] // oldest first
  senders: SenderCount[] // the 25 with the most mail
  unsubscribable: SenderCount[] // the 25 with the most mail that say how to unsubscribe
}

export interface SenderCount {
  address: string // lower case; empty for mail without a sender address
  name: string
  messages: number
  unread: number
  oldest: string
  latest: string
  // From the sender's own mail: an https page or a mailto address. Opened only by the user's click.
  unsubscribe: { url?: string; mailto?: string; oneClick: boolean } | null
}

// Which messages an action is for; the same rule the overview's rows count by.
export type Criteria = { kind: "sender"; address: string } | { kind: "year"; year: number }

export type ActionKind = "trash" | "spam"

// What an action would touch, for the user to confirm.
export interface ActionPreview {
  messages: number
  unread: number
  oldest: string
  latest: string
  subjects: string[] // the three most recent
}

// What an action did.
export interface ActionOutcome {
  actionId: number
  description: string
  outcome: "done" | "partial" | "failed" | "unknown"
  moved: number
  notMoved: number
  error?: string
}

// A place on disk to save attachments to.
export interface SaveFolder {
  id: number
  name: string
  path: string
  createdAt: string
  missing: boolean // the path is no longer there
}

// A place inside a save folder, and the folders in it.
export interface FolderBrowse {
  folderId: number
  name: string
  root: string
  path: string // relative, "/"-separated; "" is the save folder itself
  full: string // the full path, to show
  folders: string[]
}

// Names one message in cockatoo's copy.
export interface MessageKey {
  accountId: number
  uidValidity: number
  uid: number
}

// What saving attachments did.
export interface SaveResult {
  folder: string // the full path saved to
  files: number
  messages: {
    key: MessageKey
    subject: string
    from: string // the sender's address, lower case
    receivedAt: string
    files: { path: string; bytes: number }[]
    skipped?: string[]
    problem?: string
  }[]
}

// One message the AI says needs the user, with why.
export interface TriagePick {
  message: MailMessage
  priority: "now" | "today"
  reason: string
  overdue: boolean // unread for longer than a day
  // What cockatoo knows without the AI.
  signals?: {
    toYou: boolean
    ccYou: boolean
    repliesToYou: boolean
    wrote: number
    automated: boolean
    routine?: string // what one of your routines says to do with it
  }
}

// "What needs you now": the AI's last answer for a scope.
export interface TriageResult {
  id: number // 0 when nothing was sent (nothing unread)
  askedAt: string
  model?: string
  summary: string
  picks: TriagePick[]
  sent: number
  leftOut: number // over the limit, the oldest
  leftAutomated: number // left out as automated or bulk, before the AI was asked
  leftAnswered: number // left out: already answered
  textSent: boolean // the first lines of each message went with it
  textError?: string // the text could not be read; headers only
  signalsPartial: boolean // the Sent folder is not read yet
  partial: boolean // a mailbox was not completely read
  dropped: number // picks read or gone since
  inputTokens: number
  outputTokens: number
}

// The AI settings. The API key is never sent back, only whether one is saved and a hint of it.
export interface AISettings {
  provider: "anthropic"
  enabled: boolean
  model: string
  hasKey: boolean
  keyHint?: string // "sk-ant-…a1B2"
  updatedAt: string
}

// Claude's answer to the test message.
export interface AIReply {
  text: string
  model: string // the model that answered
}

export class ApiError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.status = status
  }
}

// Dev only: `VITE_MOCK_API=1 npm run dev` answers from api.mock.ts, with no backend and no mail
// server. In a production build import.meta.env.DEV is false, so this branch and the mock module
// are dropped.
export const useMock = import.meta.env.DEV && import.meta.env.VITE_MOCK_API === "1"

// Requests in flight that you wait on, for the activity bar. Polls of the sync's state and the
// health check are left out: they run all the time.
let active = 0
const listeners = new Set<(n: number) => void>()
export function onActivity(fn: (n: number) => void): () => void {
  listeners.add(fn)
  fn(active)
  return () => {
    listeners.delete(fn)
  }
}
function track(delta: number) {
  active += delta
  for (const fn of listeners) fn(active)
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const quiet = (init?.method ?? "GET") === "GET" && (path.startsWith("/api/sync") || path === "/api/health")
  if (quiet) return send<T>(path, init)
  track(1)
  try {
    return await send<T>(path, init)
  } finally {
    track(-1)
  }
}

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  if (useMock) {
    const { mockRequest } = await import("./api.mock")
    return mockRequest<T>(path, init)
  }
  const res = await fetch(path, {
    ...init,
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
  })
  if (!res.ok) {
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      /* no JSON body */
    }
    throw new ApiError(message, res.status)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  health: () => request<Health>("/api/health"),
  listAccounts: () => request<Account[]>("/api/accounts"),
  // Signs in with the details as typed and counts the inbox. Saves nothing.
  testAccount: (input: AccountInput) =>
    request<Inbox>("/api/accounts/test", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  // Tests first; saves only when the sign-in works.
  addAccount: (input: AccountInput) =>
    request<Account>("/api/accounts", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  inbox: (id: number) => request<Inbox>(`/api/accounts/${id}/inbox`),
  removeAccount: (id: number) => request<void>(`/api/accounts/${id}`, { method: "DELETE" }),

  googleSettings: () => request<GoogleSettings>("/api/google"),
  saveGoogleClient: (clientId: string, clientSecret: string) =>
    request<GoogleSettings>("/api/google", {
      method: "PUT",
      body: JSON.stringify({ clientId, clientSecret }),
    }),
  // Opens Google's sign-in page in the default browser. hint: the address, when signing in again.
  startGoogleSignIn: (hint = "") =>
    request<SignInStart>("/api/google/signin", {
      method: "POST",
      body: JSON.stringify({ hint }),
    }),
  googleSignInStatus: (state: string) => request<SignInStatus>(`/api/google/signin/${encodeURIComponent(state)}`),

  saveFolders: () => request<SaveFolder[]>("/api/folders"),
  addSaveFolder: (name: string, path: string) =>
    request<SaveFolder>("/api/folders", {
      method: "POST",
      body: JSON.stringify({ name, path }),
    }),
  removeSaveFolder: (id: number) => request<void>(`/api/folders/${id}`, { method: "DELETE" }),
  browseFolder: (id: number, path: string) =>
    request<FolderBrowse>(`/api/folders/${id}/browse?path=${encodeURIComponent(path)}`),
  makeSubfolder: (id: number, path: string, name: string) =>
    request<FolderBrowse>(`/api/folders/${id}/subfolders`, {
      method: "POST",
      body: JSON.stringify({ path, name }),
    }),
  // Marks messages done, or not done; audited. Changes nothing in the mailbox.
  // Opens a message: its conversation, read from the mail server (nothing is marked as read).
  conversation: (k: MessageKey) =>
    request<Conversation>(`/api/conversation?account=${k.accountId}&uidValidity=${k.uidValidity}&uid=${k.uid}`),
  // Searches the whole copy of the inbox by sender and subject.
  searchInbox: (q: string, account: number | "all") =>
    request<SearchResult>(`/api/inbox/search?q=${encodeURIComponent(q)}&account=${account}`),
  lists: (list: ListName, o: { all: boolean; account: number | "all"; done: boolean }) =>
    request<ListsView>(
      `/api/lists?list=${list}&account=${o.account}${o.all ? "&all=1" : ""}${o.done ? "&done=1" : ""}`,
    ),
  sortMessages: (messages: MessageKey[], list: ListName, confirmed = false) =>
    request<void>("/api/messages/sort", { method: "POST", body: JSON.stringify({ messages, list, confirmed }) }),
  suggest: (messages: MessageKey[], withText: boolean) =>
    request<SuggestResult>("/api/suggest", { method: "POST", body: JSON.stringify({ messages, withText }) }),
  skills: () => request<Skill[]>("/api/skills"),
  saveSkill: (k: SkillInput) => request<Skill>("/api/skills", { method: "POST", body: JSON.stringify(k) }),
  removeSkill: (id: number) => request<void>(`/api/skills/${id}`, { method: "DELETE" }),
  teach: (t: { message: MessageKey; from: ListName; to: ListName; ruleId: number; withText: boolean }) =>
    request<Proposal>("/api/teach", { method: "POST", body: JSON.stringify(t) }),
  routines: () => request<Routine[]>("/api/routines"),
  addRoutine: (r: RoutineInput) => request<Routine>("/api/routines", { method: "POST", body: JSON.stringify(r) }),
  removeRoutine: (id: number) => request<void>(`/api/routines/${id}`, { method: "DELETE" }),
  // Sends a reply through Gmail; audited.
  reply: (r: { message: MessageKey; text: string; all: boolean }) =>
    request<ReplyResult>("/api/reply", { method: "POST", body: JSON.stringify(r) }),
  // Moves messages to Gmail's Trash; audited.
  deleteMessages: (messages: MessageKey[]) =>
    request<DeleteResult>("/api/messages/delete", { method: "POST", body: JSON.stringify({ messages }) }),
  markDone: (messages: MessageKey[], done: boolean) =>
    request<{ marked: number }>("/api/messages/done", {
      method: "POST",
      body: JSON.stringify({ messages, done }),
    }),
  // Saves the attachments of these messages into a place in a save folder; audited.
  saveAttachments: (folderId: number, path: string, messages: MessageKey[]) =>
    request<SaveResult>("/api/attachments/save", {
      method: "POST",
      body: JSON.stringify({ folderId, path, messages }),
    }),

  // The last answer for a scope ("all" or an account id), without asking the AI; null when none.
  latestTriage: (scope: string) => request<TriageResult | null>(`/api/triage?scope=${encodeURIComponent(scope)}`),
  // Asks the AI; billed to the user's key.
  runTriage: (scope: string, withText: boolean) =>
    request<TriageResult>("/api/triage", {
      method: "POST",
      body: JSON.stringify({ scope, withText }),
    }),

  // The messages kept from the last refresh, at once.
  storedInbox: () => request<InboxList>("/api/inbox"),
  // Starts bringing every account's copy up to date in the background. Nothing is marked as read.
  startSync: () => request<SyncStatus>("/api/sync", { method: "POST" }),
  syncStatus: () => request<SyncStatus>("/api/sync"),
  overview: (id: number) => request<Overview>(`/api/accounts/${id}/overview`),

  // Nothing is changed by a preview.
  previewAction: (id: number, criteria: Criteria) =>
    request<ActionPreview>(`/api/accounts/${id}/actions/preview`, {
      method: "POST",
      body: JSON.stringify(criteria),
    }),
  // Moves the confirmed messages to Trash or Spam; audited. 409 when the count no longer matches.
  applyAction: (id: number, kind: ActionKind, criteria: Criteria, expected: number) =>
    request<ActionOutcome>(`/api/accounts/${id}/actions`, {
      method: "POST",
      body: JSON.stringify({ kind, criteria, expected }),
    }),
  // Records in the audit that the user opened an unsubscribe address. Cockatoo fetches nothing.
  logUnsubscribe: (id: number, address: string, target: string) =>
    request<void>(`/api/accounts/${id}/unsubscribe-opened`, {
      method: "POST",
      body: JSON.stringify({ address, target }),
    }),

  aiSettings: () => request<AISettings>("/api/ai"),
  // Turning AI off keeps the key.
  setAIEnabled: (enabled: boolean) =>
    request<AISettings>("/api/ai/enabled", {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  // Checks the key with Anthropic first (free); saves only when it is accepted, and turns AI on.
  saveAIKey: (key: string) =>
    request<AISettings>("/api/ai/key", {
      method: "PUT",
      body: JSON.stringify({ key }),
    }),
  removeAIKey: () => request<AISettings>("/api/ai/key", { method: "DELETE" }),
  // Sends one short message with the saved key; costs a little on the user's key.
  aiTestMessage: () => request<AIReply>("/api/ai/test-message", { method: "POST" }),
}
