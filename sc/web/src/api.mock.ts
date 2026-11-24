// Dev-only stand-in for the backend. Loaded by api.ts only when import.meta.env.DEV and
// VITE_MOCK_API=1, so the screens can be worked on with no exe and no mail account.
// Accounts and AI settings are kept in sessionStorage: they survive a reload and go when the tab
// closes. The password "wrong" and the API key "wrong" are refused, to show the error states.

import { ApiError, type Account, type AccountInput, type AISettings, type Inbox } from "./api"

const AI_KEY = "cockatoo.mock.ai"

function loadAI(): AISettings {
  const saved = sessionStorage.getItem(AI_KEY)
  if (saved) return JSON.parse(saved) as AISettings
  return { provider: "anthropic", enabled: false, model: "claude-opus-5-5", hasKey: false, updatedAt: new Date().toISOString() }
}

function saveAI(settings: AISettings): AISettings {
  const next = { ...settings, updatedAt: new Date().toISOString() }
  sessionStorage.setItem(AI_KEY, JSON.stringify(next))
  return next
}

function mockAI(method: string, path: string, init?: RequestInit): unknown {
  const ai = loadAI()
  const body = init?.body ? (JSON.parse(init.body as string) as { enabled?: boolean; key?: string }) : {}
  if (method === "GET" && path === "/api/ai") return ai
  if (method === "PUT" && path === "/api/ai/enabled") return saveAI({ ...ai, enabled: Boolean(body.enabled) })
  if (method === "PUT" && path === "/api/ai/key") {
    const key = (body.key ?? "").replace(/\s+/g, "")
    if (!key) throw new ApiError("Paste your Anthropic API key.", 400)
    if (key === "wrong") {
      throw new ApiError("Anthropic did not accept this API key. Check that it was copied in full and has not been deleted.", 502)
    }
    return saveAI({ ...ai, enabled: true, hasKey: true, keyHint: `${key.slice(0, 7)}…${key.slice(-4)}` })
  }
  if (method === "DELETE" && path === "/api/ai/key") return saveAI({ ...ai, hasKey: false, keyHint: undefined })
  if (method === "POST" && path === "/api/ai/test-message") {
    if (!ai.enabled) throw new ApiError("AI is turned off. Turn it on under AI.", 409)
    if (!ai.hasKey) throw new ApiError("No Anthropic API key is saved. Add one under AI.", 409)
    return { text: "Hello! Cockatoo can talk to me just fine.", model: ai.model }
  }
  return undefined
}

const KEY = "cockatoo.mock.accounts"

function load(): Account[] {
  return JSON.parse(sessionStorage.getItem(KEY) ?? "[]") as Account[]
}

function save(accounts: Account[]) {
  sessionStorage.setItem(KEY, JSON.stringify(accounts))
}

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

function check(input: AccountInput): Inbox {
  if (!input.email.trim()) throw new ApiError("Enter your Gmail address.", 400)
  if (!input.password.trim()) throw new ApiError("Enter the app password.", 400)
  if (input.password === "wrong") {
    throw new ApiError(
      "Gmail did not accept this address and app password. Check the address, and that this is an app password and not your Google password. Gmail said: Invalid credentials (Failure)",
      502,
    )
  }
  return { messages: 1284, unseen: 12 }
}

export async function mockRequest<T>(path: string, init?: RequestInit): Promise<T> {
  await wait(500)
  const method = init?.method ?? "GET"
  const body = init?.body ? (JSON.parse(init.body as string) as AccountInput) : undefined
  const accounts = load()
  const id = Number(path.match(/^\/api\/accounts\/(\d+)/)?.[1])

  if (path.startsWith("/api/ai")) {
    const answer = mockAI(method, path, init)
    if (answer !== undefined) return answer as T
  }
  if (method === "GET" && path === "/api/health") {
    return { ok: true, time: new Date().toISOString(), version: "mock" } as T
  }
  if (method === "GET" && path === "/api/accounts") return accounts as T
  if (method === "POST" && path === "/api/accounts/test" && body) return check(body) as T
  if (method === "POST" && path === "/api/accounts" && body) {
    check(body)
    const email = body.email.trim().toLowerCase()
    if (accounts.some((a) => a.email === email)) throw new ApiError("this address is already added", 409)
    const account: Account = {
      id: Math.max(0, ...accounts.map((a) => a.id)) + 1,
      provider: "gmail",
      name: body.name.trim() || email,
      email,
      createdAt: new Date().toISOString(),
    }
    save([...accounts, account])
    return account as T
  }
  if (method === "GET" && path.endsWith("/inbox") && accounts.some((a) => a.id === id)) {
    return { messages: 1284, unseen: 12 } as T
  }
  if (method === "DELETE" && accounts.some((a) => a.id === id)) {
    save(accounts.filter((a) => a.id !== id))
    return undefined as T
  }
  throw new ApiError(`no such API route: ${method} ${path}`, 404)
}
