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
}

// The last 24 hours of every account's inbox, newest first, from cockatoo's copy.
export interface InboxList {
  since: string
  messages: MailMessage[]
}

// What the sync is doing. It copies every account's inbox headers into cockatoo, in the background.
export interface SyncStatus {
  running: boolean
  accounts: AccountSync[]
}

export interface AccountSync {
  accountId: number
  email: string
  state: "waiting" | "listing" | "reading" | "done" | "failed"
  read: number // headers read so far in this sync
  toRead: number // headers this sync has to read
  skipped: number // messages the server could not give
  error?: string
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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
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
    request<Inbox>("/api/accounts/test", { method: "POST", body: JSON.stringify(input) }),
  // Tests first; saves only when the sign-in works.
  addAccount: (input: AccountInput) =>
    request<Account>("/api/accounts", { method: "POST", body: JSON.stringify(input) }),
  inbox: (id: number) => request<Inbox>(`/api/accounts/${id}/inbox`),
  removeAccount: (id: number) => request<void>(`/api/accounts/${id}`, { method: "DELETE" }),

  googleSettings: () => request<GoogleSettings>("/api/google"),
  saveGoogleClient: (clientId: string, clientSecret: string) =>
    request<GoogleSettings>("/api/google", { method: "PUT", body: JSON.stringify({ clientId, clientSecret }) }),
  // Opens Google's sign-in page in the default browser. hint: the address, when signing in again.
  startGoogleSignIn: (hint = "") =>
    request<SignInStart>("/api/google/signin", { method: "POST", body: JSON.stringify({ hint }) }),
  googleSignInStatus: (state: string) => request<SignInStatus>(`/api/google/signin/${encodeURIComponent(state)}`),

  // The messages kept from the last refresh, at once.
  storedInbox: () => request<InboxList>("/api/inbox"),
  // Starts bringing every account's copy up to date in the background. Nothing is marked as read.
  startSync: () => request<SyncStatus>("/api/sync", { method: "POST" }),
  syncStatus: () => request<SyncStatus>("/api/sync"),
  overview: (id: number) => request<Overview>(`/api/accounts/${id}/overview`),

  // Nothing is changed by a preview.
  previewAction: (id: number, criteria: Criteria) =>
    request<ActionPreview>(`/api/accounts/${id}/actions/preview`, { method: "POST", body: JSON.stringify(criteria) }),
  // Moves the confirmed messages to Trash or Spam; audited. 409 when the count no longer matches.
  applyAction: (id: number, kind: ActionKind, criteria: Criteria, expected: number) =>
    request<ActionOutcome>(`/api/accounts/${id}/actions`, {
      method: "POST",
      body: JSON.stringify({ kind, criteria, expected }),
    }),
  // Records in the audit that the user opened an unsubscribe address. Cockatoo fetches nothing.
  logUnsubscribe: (id: number, address: string, target: string) =>
    request<void>(`/api/accounts/${id}/unsubscribe-opened`, { method: "POST", body: JSON.stringify({ address, target }) }),

  aiSettings: () => request<AISettings>("/api/ai"),
  // Turning AI off keeps the key.
  setAIEnabled: (enabled: boolean) =>
    request<AISettings>("/api/ai/enabled", { method: "PUT", body: JSON.stringify({ enabled }) }),
  // Checks the key with Anthropic first (free); saves only when it is accepted, and turns AI on.
  saveAIKey: (key: string) => request<AISettings>("/api/ai/key", { method: "PUT", body: JSON.stringify({ key }) }),
  removeAIKey: () => request<AISettings>("/api/ai/key", { method: "DELETE" }),
  // Sends one short message with the saved key; costs a little on the user's key.
  aiTestMessage: () => request<AIReply>("/api/ai/test-message", { method: "POST" }),
}
