// Dev-only stand-in for the backend. Loaded by api.ts only when import.meta.env.DEV and
// VITE_MOCK_API=1, so the screens can be worked on with no exe and no mail account.
// Accounts and AI settings are kept in sessionStorage: they survive a reload and go when the tab
// closes. The password "wrong" and the API key "wrong" are refused, to show the error states.

import {
  ApiError,
  type Account,
  type AccountInput,
  type AISettings,
  type Inbox,
  type InboxList,
  type MailMessage,
  type Overview,
  type SenderCount,
  type SyncStatus,
} from "./api"

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

// A day of made-up mail for each saved account. Refresh marks the oldest unread one as read, so
// a refresh visibly changes something.
const SAMPLE: [minutesAgo: number, fromName: string, fromAddr: string, subject: string, seen: boolean][] = [
  [4, "Renée Dubois", "renee@example.com", "Précis for Thursday – draft attached", false],
  [38, "GitHub", "noreply@github.com", "[cockatoo] Build passed on main", false],
  [95, "Sam Okafor", "sam@example.com", "Lunch on Friday?", true],
  [180, "", "billing@example.com", "Your invoice for November", false],
  [420, "José Gómez", "jose@example.com", "Re: site visit notes", true],
  [790, "Priya Nair", "priya@example.com", "A very long subject line that goes on to show how the list handles text that does not fit on one line", true],
  [1300, "Newsletter", "news@example.com", "", true],
]
const SEEN_KEY = "cockatoo.mock.refreshed"

function mockInbox(accounts: Account[], refreshed: boolean): InboxList {
  const now = Date.now()
  const readAfterRefresh = refreshed ? 1 : 0
  const messages: MailMessage[] = accounts.flatMap((a, ai) =>
    SAMPLE.map(([mins, fromName, fromAddr, subject, seen], i) => ({
      accountId: a.id,
      uidValidity: 1,
      uid: 100 - i,
      messageId: `${a.id}-${i}@example.com`,
      fromName,
      fromAddr,
      subject: ai === 0 ? subject : `${subject} (${a.name})`,
      receivedAt: new Date(now - (mins + ai * 7) * 60_000).toISOString(),
      seen: seen || (i === 3 && readAfterRefresh === 1),
    })),
  )
  messages.sort((x, y) => y.receivedAt.localeCompare(x.receivedAt))
  return { since: new Date(now - 24 * 3_600_000).toISOString(), messages }
}

// A mailbox of about 11,000 messages over fifteen years, read by a sync that takes a few seconds
// the first time and a moment after that.
const YEARS: [year: number, messages: number, unread: number][] = [
  [2012, 214, 31], [2013, 356, 48], [2014, 522, 77], [2015, 611, 92], [2016, 704, 66], [2017, 823, 58],
  [2018, 765, 41], [2019, 688, 37], [2020, 981, 52], [2021, 1104, 44], [2022, 942, 39], [2023, 873, 33],
  [2024, 1022, 61], [2025, 1236, 84], [2026, 1184, 112],
]
const SENDERS: [name: string, address: string, messages: number, unread: number, from: number, to: number][] = [
  ["Northwind Deals", "deals@mail.northwind.example", 1412, 402, 2014, 2026],
  ["LinkedIn", "messages-noreply@linkedin.example", 986, 51, 2013, 2026],
  ["Daily Digest", "digest@news.example", 734, 12, 2016, 2026],
  ["GitHub", "notifications@github.example", 702, 3, 2015, 2026],
  ["Contoso Travel", "offers@contoso-travel.example", 655, 188, 2012, 2024],
  ["Fabrikam Bank", "alerts@fabrikam.example", 441, 0, 2017, 2026],
  ["", "no-reply@accounts.example", 388, 14, 2013, 2026],
  ["Adventure Works", "hello@adventure-works.example", 352, 121, 2018, 2023],
  ["Sam Okafor", "sam@example.com", 311, 0, 2012, 2026],
  ["Wide World Importers", "news@wwi.example", 290, 97, 2014, 2021],
  ["Priya Nair", "priya@example.com", 244, 2, 2019, 2026],
  ["Tailspin Toys", "promo@tailspin.example", 231, 76, 2015, 2025],
  ["Coho Winery", "club@coho.example", 198, 64, 2016, 2026],
  ["Calendar", "calendar-notification@example.com", 176, 0, 2013, 2026],
  ["Litware", "updates@litware.example", 152, 9, 2020, 2026],
  ["José Gómez", "jose@example.com", 147, 0, 2014, 2026],
  ["Margie's Travel", "deals@margiestravel.example", 139, 51, 2012, 2019],
  ["Proseware", "team@proseware.example", 121, 4, 2021, 2026],
  ["Renée Dubois", "renee@example.com", 118, 1, 2017, 2026],
  ["Lucerne Publishing", "newsletter@lucerne.example", 104, 38, 2013, 2022],
  ["Fourth Coffee", "rewards@fourthcoffee.example", 97, 33, 2018, 2026],
  ["Trey Research", "reports@treyresearch.example", 88, 5, 2019, 2026],
  ["City of Example", "rates@city.example", 74, 0, 2015, 2026],
  ["Blue Yonder Airlines", "flights@blueyonder.example", 69, 2, 2012, 2025],
  ["A very long sender name that goes on to show how the table handles it", "someone.with.a.long.address@subdomain.example.com", 61, 20, 2016, 2024],
]
const YEAR_TOTAL = YEARS.reduce((n, [, m]) => n + m, 0)
const SYNC_KEY = "cockatoo.mock.sync"
const FIRST_SYNC_MS = 8000

type MockSync = { startedAt: number; ms: number }

function syncState(): MockSync | null {
  const saved = sessionStorage.getItem(SYNC_KEY)
  return saved ? (JSON.parse(saved) as MockSync) : null
}

// How far the mock sync has read, 0 to 1.
function syncFraction(): number {
  const s = syncState()
  if (!s) return 0
  return Math.min(1, (Date.now() - s.startedAt) / s.ms)
}

function mockSyncStatus(accounts: Account[]): SyncStatus {
  const s = syncState()
  const f = syncFraction()
  const running = s !== null && f < 1
  const firstRun = s !== null && s.ms === FIRST_SYNC_MS
  return {
    running,
    accounts: accounts.map((a) => ({
      accountId: a.id,
      email: a.email,
      state: running ? "reading" : "done",
      read: firstRun ? Math.round(YEAR_TOTAL * f) : 0,
      toRead: firstRun ? YEAR_TOTAL : 0,
      skipped: 0,
    })),
  }
}

// Senders whose mail carries an unsubscribe address in the mock, and what it is.
const UNSUB: Record<string, { url?: string; mailto?: string; oneClick: boolean }> = {
  "deals@mail.northwind.example": { url: "https://northwind.example/unsubscribe?u=1", oneClick: true },
  "messages-noreply@linkedin.example": { url: "https://linkedin.example/settings/email", oneClick: false },
  "digest@news.example": { url: "https://news.example/u/1", mailto: "mailto:leave@news.example", oneClick: true },
  "offers@contoso-travel.example": { url: "https://contoso-travel.example/unsub", oneClick: true },
  "hello@adventure-works.example": { mailto: "mailto:unsubscribe@adventure-works.example", oneClick: false },
  "news@wwi.example": { url: "https://wwi.example/prefs", oneClick: false },
  "promo@tailspin.example": { url: "https://tailspin.example/u", oneClick: true },
  "club@coho.example": { url: "https://coho.example/leave", oneClick: false },
  "deals@margiestravel.example": { url: "https://margiestravel.example/u", oneClick: true },
  "newsletter@lucerne.example": { url: "https://lucerne.example/unsubscribe", oneClick: false },
  "rewards@fourthcoffee.example": { url: "https://fourthcoffee.example/u", oneClick: true },
}

// What the mock's actions have moved out of the inbox: whole senders and whole years.
const MOVED_KEY = "cockatoo.mock.moved"
type Moved = { senders: string[]; years: number[] }

function moved(): Moved {
  return JSON.parse(sessionStorage.getItem(MOVED_KEY) ?? '{"senders":[],"years":[]}') as Moved
}

function senderRows(share: number, complete: boolean, firstYear: number, gone: Moved): SenderCount[] {
  return SENDERS.filter(([, address]) => !gone.senders.includes(address)).map(
    ([name, address, messages, unread, from, to]) => ({
      name,
      address,
      messages: Math.max(1, Math.round(messages * share)),
      unread: Math.round(unread * share),
      oldest: new Date(Date.UTC(complete ? Math.max(from, firstYear) : Math.max(from, firstYear), 5, 1)).toISOString(),
      latest: new Date(Date.UTC(to, 5, 1)).toISOString(),
      unsubscribe: UNSUB[address] ?? null,
    }),
  )
}

// The overview as far as the sync has read: newest years fill first, as the real sync reads.
// Senders and years moved by an action are gone from it.
function mockOverview(accountId: number): Overview {
  const gone = moved()
  const goneBySender = SENDERS.filter(([, address]) => gone.senders.includes(address)).reduce(
    (n, [, , messages, unread]) => ({ messages: n.messages + messages, unread: n.unread + unread }),
    { messages: 0, unread: 0 },
  )
  const keep = 1 - goneBySender.messages / YEAR_TOTAL
  let left = Math.round(YEAR_TOTAL * syncFraction())
  const complete = left === YEAR_TOTAL
  const years = [...YEARS]
    .reverse()
    .map(([year, messages, unread]) => {
      const take = Math.min(messages, left)
      left -= take
      return { year, messages: Math.round(take * keep), unread: Math.round(((unread * take) / messages) * keep) }
    })
    .filter((y) => y.messages > 0 && !gone.years.includes(y.year))
    .reverse()
  const share = years.reduce((n, y) => n + y.messages, 0) / (YEAR_TOTAL * keep || 1)
  const firstYear = years[0]?.year ?? 2026
  const senders = share > 0 ? senderRows(share, complete, firstYear, gone) : []
  return {
    accountId,
    sync: { serverTotal: YEAR_TOTAL, complete, syncedAt: complete ? new Date().toISOString() : null },
    messages: years.reduce((n, y) => n + y.messages, 0),
    unread: years.reduce((n, y) => n + y.unread, 0),
    oldest: years.length ? new Date(Date.UTC(years[0].year, 0, 3)).toISOString() : null,
    years,
    senders,
    unsubscribable: senders.filter((x) => x.unsubscribe),
  }
}

// Google sign-in in the mock: the client ID must end in .apps.googleusercontent.com; a sign-in
// finishes three seconds after it starts, as work@example.com (or the hint when signing in again).
const GOOGLE_KEY = "cockatoo.mock.google"

function mockGoogle(method: string, path: string, init: RequestInit | undefined, accounts: Account[]): unknown {
  const body = init?.body ? (JSON.parse(init.body as string) as Record<string, string>) : {}
  const saved = JSON.parse(sessionStorage.getItem(GOOGLE_KEY) ?? "{}") as { clientId?: string; started?: Record<string, { at: number; hint: string }> }
  const store = () => sessionStorage.setItem(GOOGLE_KEY, JSON.stringify(saved))
  if (method === "GET" && path === "/api/google") return { configured: Boolean(saved.clientId), clientId: saved.clientId }
  if (method === "PUT" && path === "/api/google") {
    if (!(body.clientId ?? "").trim().endsWith(".apps.googleusercontent.com")) {
      throw new ApiError("The client ID ends in .apps.googleusercontent.com. Copy it from the Google Cloud console.", 400)
    }
    if (!(body.clientSecret ?? "").trim()) throw new ApiError("Paste the client secret from the same client in the Google Cloud console.", 400)
    saved.clientId = body.clientId.trim()
    store()
    return { configured: true, clientId: saved.clientId }
  }
  if (method === "POST" && path === "/api/google/signin") {
    if (!saved.clientId) throw new ApiError("Google sign-in is not set up yet. Add the client ID and secret first.", 409)
    const state = Math.random().toString(36).slice(2)
    saved.started = { ...(saved.started ?? {}), [state]: { at: Date.now(), hint: body.hint ?? "" } }
    store()
    return { state, url: "https://accounts.google.com/o/oauth2/v2/auth?mock=1", opened: true }
  }
  const m = path.match(/^\/api\/google\/signin\/(.+)$/)
  if (method === "GET" && m) {
    const s = saved.started?.[decodeURIComponent(m[1])]
    if (!s) return { state: "failed", error: "This sign-in has expired. Start again." }
    if (Date.now() - s.at < 3000) return { state: "waiting" }
    const email = s.hint || "work@example.com"
    let account = accounts.find((a) => a.email === email)
    if (account) {
      account.auth = "google"
    } else {
      account = { id: Math.max(0, ...accounts.map((a) => a.id)) + 1, provider: "gmail", name: email, email, auth: "google", createdAt: new Date().toISOString() }
      accounts.push(account)
    }
    save(accounts)
    return { state: "done", account }
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
  if (method === "GET" && path === "/api/inbox") {
    if (syncFraction() === 0) return { since: new Date().toISOString(), messages: [] } as T
    return mockInbox(accounts, sessionStorage.getItem(SEEN_KEY) !== null) as T
  }
  if (method === "POST" && path === "/api/sync") {
    const s = syncState()
    if (!s || syncFraction() >= 1) {
      if (s) sessionStorage.setItem(SEEN_KEY, "1")
      sessionStorage.setItem(SYNC_KEY, JSON.stringify({ startedAt: Date.now(), ms: s ? 1200 : FIRST_SYNC_MS }))
    }
    return mockSyncStatus(accounts) as T
  }
  if (method === "GET" && path === "/api/sync") return mockSyncStatus(accounts) as T
  if (method === "POST" && path.endsWith("/actions/preview")) {
    const c = JSON.parse(init!.body as string) as { kind: string; address?: string; year?: number }
    const o = mockOverview(id)
    const row =
      c.kind === "sender"
        ? o.senders.find((x) => x.address === c.address)
        : o.years.find((y) => y.year === c.year)
    if (!row) throw new ApiError("There is no mail left that matches.", 409)
    return {
      messages: row.messages,
      unread: row.unread,
      oldest: "oldest" in row ? row.oldest : new Date(Date.UTC(c.year!, 0, 2)).toISOString(),
      latest: "latest" in row ? row.latest : new Date(Date.UTC(c.year!, 11, 30)).toISOString(),
      subjects: ["Your weekly picks are here", "Last chance: 40% off", "We miss you"],
    } as T
  }
  if (method === "POST" && /\/actions$/.test(path)) {
    await wait(900)
    const req = JSON.parse(init!.body as string) as { kind: string; criteria: { kind: string; address?: string; year?: number }; expected: number }
    const gone = moved()
    if (req.criteria.kind === "sender") gone.senders.push(req.criteria.address!)
    else gone.years.push(req.criteria.year!)
    sessionStorage.setItem(MOVED_KEY, JSON.stringify(gone))
    const where = req.kind === "spam" ? "Spam" : "Trash"
    const what = req.criteria.kind === "sender" ? `from ${req.criteria.address}` : `received in ${req.criteria.year}`
    return {
      actionId: Date.now(),
      description: `Move ${req.expected} messages ${what} to ${where}`,
      outcome: "done",
      moved: req.expected,
      notMoved: 0,
    } as T
  }
  if (method === "POST" && path.endsWith("/unsubscribe-opened")) return undefined as T
  if (method === "GET" && /^\/api\/accounts\/\d+\/overview$/.test(path) && accounts.some((a) => a.id === id)) {
    return mockOverview(id) as T
  }
  if (path.startsWith("/api/google")) {
    const answer = mockGoogle(method, path, init, accounts)
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
      auth: "password",
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
