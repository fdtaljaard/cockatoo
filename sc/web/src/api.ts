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
  createdAt: string
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
