// Dev-only stand-in for the backend. Loaded by api.ts only when import.meta.env.DEV and
// VITE_MOCK_API=1, so the screens can be worked on with no exe and no mail account.
// Accounts and AI settings are kept in sessionStorage: they survive a reload and go when the tab
// closes. The password "wrong" and the API key "wrong" are refused, to show the error states.
// Open it with ?demo for two accounts, the AI turned on and an answer from it, as for screenshots.

import {
  ApiError,
  type Account,
  type AccountInput,
  type AISettings,
  type Handled,
  type Inbox,
  type InboxList,
  type MailMessage,
  type MessageKey,
  type ConversationMessage,
  type Routine,
  type RoutineInput,
  type RoutineMatch,
  type SaveFolder,
  type Overview,
  type SenderCount,
  type SyncStatus,
} from "./api"

const AI_KEY = "cockatoo.mock.ai"

const demo = new URLSearchParams(location.search).has("demo")
if (demo && !sessionStorage.getItem("cockatoo.mock.accounts")) {
  const at = new Date().toISOString()
  sessionStorage.setItem(
    "cockatoo.mock.accounts",
    JSON.stringify([
      { id: 1, provider: "gmail", name: "Personal", email: "sam@example.com", auth: "password", createdAt: at },
      { id: 2, provider: "gmail", name: "Work", email: "sam@work.example.com", auth: "google", createdAt: at },
    ]),
  )
  sessionStorage.setItem("cockatoo.mock.refreshed", "1")
  sessionStorage.setItem("cockatoo.mock.sync", JSON.stringify({ startedAt: Date.now() - 60_000, ms: 1 })) // read already
  sessionStorage.setItem(
    AI_KEY,
    JSON.stringify({ provider: "anthropic", enabled: true, model: "claude-opus-5-5", hasKey: true, updatedAt: at }),
  )
  // The AI's answer, in place before the page asks for it.
  queueMicrotask(
    () => void mockRequest("/api/triage", { method: "POST", body: JSON.stringify({ scope: "all", withText: true }) }),
  )
}

function loadAI(): AISettings {
  const saved = sessionStorage.getItem(AI_KEY)
  if (saved) return JSON.parse(saved) as AISettings
  return {
    provider: "anthropic",
    enabled: false,
    model: "claude-opus-5-5",
    hasKey: false,
    updatedAt: new Date().toISOString(),
  }
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
      throw new ApiError(
        "Anthropic did not accept this API key. Check that it was copied in full and has not been deleted.",
        502,
      )
    }
    return saveAI({
      ...ai,
      enabled: true,
      hasKey: true,
      keyHint: `${key.slice(0, 7)}…${key.slice(-4)}`,
    })
  }
  if (method === "DELETE" && path === "/api/ai/key") return saveAI({ ...ai, hasKey: false, keyHint: undefined })
  if (method === "POST" && path === "/api/ai/test-message") {
    if (!ai.enabled) throw new ApiError("AI is turned off. Turn it on under AI.", 409)
    if (!ai.hasKey) throw new ApiError("No Anthropic API key is saved. Add one under AI.", 409)
    return {
      text: "Hello! Cockatoo can talk to me just fine.",
      model: ai.model,
    }
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
  [
    790,
    "Priya Nair",
    "priya@example.com",
    "A very long subject line that goes on to show how the list handles text that does not fit on one line",
    true,
  ],
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
  return { since: new Date(now - 24 * 3_600_000).toISOString(), messages: notDeleted(messages), waiting: [] }
}

// Mock messages moved to Trash, by key.
const DELETED_KEY = "cockatoo.mock.deleted"
const deleted = () => new Set(JSON.parse(sessionStorage.getItem(DELETED_KEY) ?? "[]") as string[])
const notDeleted = (list: MailMessage[]) => {
  const gone = deleted()
  return list.filter((m) => !gone.has(`${m.accountId}-${m.uidValidity}-${m.uid}`))
}

// Older mail, outside the 24 hours: last month's invoices, for the Waiting list.
function mockOlder(accounts: Account[]): MailMessage[] {
  const now = Date.now()
  return notDeleted(
    accounts.flatMap((a) =>
      [
        [5, "Your invoice for October (late)"],
        [33, "Your invoice for October"],
      ].map(([days, subject], i) => ({
        accountId: a.id,
        uidValidity: 1,
        uid: 50 - i,
        messageId: `${a.id}-old${i}@example.com`,
        fromName: "",
        fromAddr: "billing@example.com",
        subject: String(subject),
        receivedAt: new Date(now - Number(days) * 86_400_000).toISOString(),
        seen: true,
      })),
    ),
  )
}

// Mock routines. Unlike cockatoo, the mock also matches mail from before a routine was made, so
// the Waiting list shows something at once.
const ROUTINES_KEY = "cockatoo.mock.routines"
const loadRoutines = () => JSON.parse(sessionStorage.getItem(ROUTINES_KEY) ?? "[]") as Routine[]
function mockMatch(m: MailMessage): RoutineMatch | undefined {
  const from = m.fromAddr.toLowerCase()
  const r = loadRoutines()
    .filter(
      (r) =>
        (r.from.includes("@") ? from === r.from : from.endsWith("@" + r.from)) &&
        (!r.subject || m.subject.toLowerCase().includes(r.subject)),
    )
    .pop()
  if (!r) return undefined
  const d = new Date(m.receivedAt)
  const month = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`
  const name = r.dated === "month" ? month : r.dated === "day" ? `${month}-${String(d.getDate()).padStart(2, "0")}` : ""
  return { routineId: r.id, action: "save", folderId: r.folderId, folderName: r.folderName, parent: r.path, name }
}

// A mailbox of about 11,000 messages over fifteen years, read by a sync that takes a few seconds
// the first time and a moment after that.
const YEARS: [year: number, messages: number, unread: number][] = [
  [2012, 214, 31],
  [2013, 356, 48],
  [2014, 522, 77],
  [2015, 611, 92],
  [2016, 704, 66],
  [2017, 823, 58],
  [2018, 765, 41],
  [2019, 688, 37],
  [2020, 981, 52],
  [2021, 1104, 44],
  [2022, 942, 39],
  [2023, 873, 33],
  [2024, 1022, 61],
  [2025, 1236, 84],
  [2026, 1184, 112],
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
  [
    "A very long sender name that goes on to show how the table handles it",
    "someone.with.a.long.address@subdomain.example.com",
    61,
    20,
    2016,
    2024,
  ],
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
  "deals@mail.northwind.example": {
    url: "https://northwind.example/unsubscribe?u=1",
    oneClick: true,
  },
  "messages-noreply@linkedin.example": {
    url: "https://linkedin.example/settings/email",
    oneClick: false,
  },
  "digest@news.example": {
    url: "https://news.example/u/1",
    mailto: "mailto:leave@news.example",
    oneClick: true,
  },
  "offers@contoso-travel.example": {
    url: "https://contoso-travel.example/unsub",
    oneClick: true,
  },
  "hello@adventure-works.example": {
    mailto: "mailto:unsubscribe@adventure-works.example",
    oneClick: false,
  },
  "news@wwi.example": { url: "https://wwi.example/prefs", oneClick: false },
  "promo@tailspin.example": {
    url: "https://tailspin.example/u",
    oneClick: true,
  },
  "club@coho.example": { url: "https://coho.example/leave", oneClick: false },
  "deals@margiestravel.example": {
    url: "https://margiestravel.example/u",
    oneClick: true,
  },
  "newsletter@lucerne.example": {
    url: "https://lucerne.example/unsubscribe",
    oneClick: false,
  },
  "rewards@fourthcoffee.example": {
    url: "https://fourthcoffee.example/u",
    oneClick: true,
  },
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
    (n, [, , messages, unread]) => ({
      messages: n.messages + messages,
      unread: n.unread + unread,
    }),
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
      return {
        year,
        messages: Math.round(take * keep),
        unread: Math.round(((unread * take) / messages) * keep),
      }
    })
    .filter((y) => y.messages > 0 && !gone.years.includes(y.year))
    .reverse()
  const share = years.reduce((n, y) => n + y.messages, 0) / (YEAR_TOTAL * keep || 1)
  const firstYear = years[0]?.year ?? 2026
  const senders = share > 0 ? senderRows(share, complete, firstYear, gone) : []
  return {
    accountId,
    sync: {
      serverTotal: YEAR_TOTAL,
      complete,
      syncedAt: complete ? new Date().toISOString() : null,
    },
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
  const saved = JSON.parse(sessionStorage.getItem(GOOGLE_KEY) ?? "{}") as {
    clientId?: string
    started?: Record<string, { at: number; hint: string }>
  }
  const store = () => sessionStorage.setItem(GOOGLE_KEY, JSON.stringify(saved))
  if (method === "GET" && path === "/api/google")
    return { configured: Boolean(saved.clientId), clientId: saved.clientId }
  if (method === "PUT" && path === "/api/google") {
    if (!(body.clientId ?? "").trim().endsWith(".apps.googleusercontent.com")) {
      throw new ApiError(
        "The client ID ends in .apps.googleusercontent.com. Copy it from the Google Cloud console.",
        400,
      )
    }
    if (!(body.clientSecret ?? "").trim())
      throw new ApiError("Paste the client secret from the same client in the Google Cloud console.", 400)
    saved.clientId = body.clientId.trim()
    store()
    return { configured: true, clientId: saved.clientId }
  }
  if (method === "POST" && path === "/api/google/signin") {
    if (!saved.clientId)
      throw new ApiError("Google sign-in is not set up yet. Add the client ID and secret first.", 409)
    const state = Math.random().toString(36).slice(2)
    saved.started = {
      ...(saved.started ?? {}),
      [state]: { at: Date.now(), hint: body.hint ?? "" },
    }
    store()
    return {
      state,
      url: "https://accounts.google.com/o/oauth2/v2/auth?mock=1",
      opened: true,
    }
  }
  const m = path.match(/^\/api\/google\/signin\/(.+)$/)
  if (method === "GET" && m) {
    const s = saved.started?.[decodeURIComponent(m[1])]
    if (!s)
      return {
        state: "failed",
        error: "This sign-in has expired. Start again.",
      }
    if (Date.now() - s.at < 3000) return { state: "waiting" }
    const email = s.hint || "work@example.com"
    let account = accounts.find((a) => a.email === email)
    if (account) {
      account.auth = "google"
    } else {
      account = {
        id: Math.max(0, ...accounts.map((a) => a.id)) + 1,
        provider: "gmail",
        name: email,
        email,
        auth: "google",
        createdAt: new Date().toISOString(),
      }
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

const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, demo ? 0 : ms)) // no waiting in a demo

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

// What the user did about mock messages, by key.
const HANDLED_KEY = "cockatoo.mock.handled"
const mockKey = (k: MessageKey) => `${k.accountId}-${k.uidValidity}-${k.uid}`
function loadHandled(): Record<string, Handled> {
  return JSON.parse(sessionStorage.getItem(HANDLED_KEY) ?? "{}") as Record<string, Handled>
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
    const list = mockInbox(accounts, sessionStorage.getItem(SEEN_KEY) !== null)
    const handled = loadHandled()
    const withMarks = (m: MailMessage, replied = false): MailMessage => {
      const h = { ...handled[mockKey(m)] }
      if (replied) h.replied = true
      if (Object.keys(h).length) return { ...m, handled: h }
      const routine = mockMatch(m)
      return routine ? { ...m, routine } : m
    }
    // One message answered from Gmail, as the Sent folder would show.
    list.messages = list.messages.map((m, i) => withMarks(m, i === 2))
    list.waiting = [...list.messages, ...mockOlder(accounts).map((m) => withMarks(m))].filter(
      (m) => m.routine && !m.handled,
    )
    return list as T
  }
  if (method === "POST" && path === "/api/messages/delete") {
    await wait(600)
    const b = JSON.parse(init!.body as string) as { messages: MessageKey[] }
    const gone = deleted()
    for (const k of b.messages) gone.add(mockKey(k))
    sessionStorage.setItem(DELETED_KEY, JSON.stringify([...gone]))
    return { moved: b.messages.length, notMoved: 0, outcome: "done" } as T
  }
  if (method === "POST" && path === "/api/reply") {
    await wait(900)
    const b = JSON.parse(init!.body as string) as { message: MessageKey; text: string; all: boolean }
    if (!b.text.trim()) throw new ApiError("Write something to send.", 400)
    if (b.text.includes("fail")) throw new ApiError("Gmail did not take the message: 550 mock refusal", 502)
    return {
      messageId: `mock-${Date.now()}@example.com`,
      to: ["the sender"],
      cc: b.all ? ["Bob Jones <bob@example.com>"] : [],
      at: new Date().toISOString(),
    } as T
  }
  if (method === "GET" && path.startsWith("/api/conversation")) {
    await wait(400)
    const params = new URLSearchParams(path.split("?")[1])
    const all = [...mockInbox(accounts, true).messages, ...mockOlder(accounts)]
    const m = all.find((x) => String(x.accountId) === params.get("account") && String(x.uid) === params.get("uid"))
    if (!m) throw new ApiError("This message is no longer in your inbox.", 404)
    const at = (minutesBefore: number) =>
      new Date(new Date(m.receivedAt).getTime() - minutesBefore * 60_000).toISOString()
    const name = m.fromName || m.fromAddr
    const invoice = /invoice/i.test(m.subject)
    const handled = loadHandled()[mockKey(m)]
    const messages: ConversationMessage[] = [
      {
        folder: "inbox",
        mine: false,
        opened: false,
        fromName: name,
        fromAddr: m.fromAddr,
        at: at(26 * 60),
        seen: true,
        text: invoice
          ? "Hi, could you confirm the billing address for your account?\n\nKind regards\nAccounts team"
          : "Hi! Are you free on Friday? There's a new place on Bree Street I've wanted to try.\n\nCheers\n" + name,
        full: invoice
          ? "Hi, could you confirm the billing address for your account?\n\nKind regards\nAccounts team\nAcme (Pty) Ltd | Reg. 2001/000000/07\n+27 21 000 0000 | www.example.com\n\nThis email and any attachments are confidential and intended solely for the addressee."
          : "Hi! Are you free on Friday? There's a new place on Bree Street I've wanted to try.\n\nCheers\n" +
            name +
            "\nSent from my iPhone",
        cut: false,
        attachments: [],
      },
      {
        folder: "sent",
        mine: true,
        opened: false,
        fromName: "",
        fromAddr: "me@example.com",
        at: at(25 * 60),
        seen: true,
        text: invoice ? "It's 12 Long Street, Cape Town 8001." : "Friday works. 12:30? I'll book.",
        full: invoice
          ? "It's 12 Long Street, Cape Town 8001.\n\nOn Sun, Accounts team wrote:\n> Hi, could you confirm the billing address for your account?"
          : "Friday works. 12:30? I'll book.\n\nOn Sun, " + name + " wrote:\n> Hi! Are you free on Friday?",
        cut: false,
        attachments: [],
      },
      {
        key: { accountId: m.accountId, uidValidity: m.uidValidity, uid: m.uid },
        folder: "inbox",
        mine: false,
        opened: true,
        fromName: name,
        fromAddr: m.fromAddr,
        at: m.receivedAt,
        seen: m.seen,
        text: invoice
          ? "Thank you. Your invoice for this month is attached. You can see it online too: https://billing.example.com/invoices\n\nKind regards\nAccounts team"
          : m.subject || "(no text)",
        full: invoice
          ? "Thank you. Your invoice for this month is attached. You can see it online too: https://billing.example.com/invoices\n\nKind regards\nAccounts team\nAcme (Pty) Ltd | Reg. 2001/000000/07\n+27 21 000 0000\n\nOn Mon, you wrote:\n> It's 12 Long Street, Cape Town 8001."
          : m.subject,
        cut: false,
        attachments: invoice ? [{ name: "invoice-2026-11.pdf", size: 48213 }] : [],
        handled,
        routine: handled ? undefined : mockMatch(m),
      },
    ]
    return {
      subject: m.subject,
      messageId: m.messageId,
      account: "me@example.com",
      messages,
      older: 0,
      sentPartial: false,
      reply: {
        key: { accountId: m.accountId, uidValidity: m.uidValidity, uid: m.uid },
        to: [m.fromName ? `${m.fromName} <${m.fromAddr}>` : m.fromAddr],
        cc: ["Bob Jones <bob@example.com>"],
      },
    } as T
  }
  if (method === "GET" && path.startsWith("/api/inbox/search")) {
    const params = new URLSearchParams(path.split("?")[1])
    const words = (params.get("q") ?? "").toLowerCase().split(/\s+/).filter(Boolean)
    const account = params.get("account")
    const all = [...mockInbox(accounts, true).messages, ...mockOlder(accounts)]
    const messages = all
      .filter((m) => account === "all" || String(m.accountId) === account)
      .filter((m) => words.every((w) => `${m.fromName} ${m.fromAddr} ${m.subject}`.toLowerCase().includes(w)))
      .sort((x, y) => y.receivedAt.localeCompare(x.receivedAt))
    return { query: params.get("q") ?? "", messages, more: false } as T
  }
  if (path.startsWith("/api/routines")) {
    const routines = loadRoutines()
    if (method === "GET") return routines as T
    if (method === "POST") {
      const b = JSON.parse(init!.body as string) as RoutineInput
      const from = b.from.trim().toLowerCase().replace(/^@/, "")
      if (!from.includes("."))
        throw new ApiError(`${from || "That"} does not look like a domain, such as acme.com.`, 400)
      const subject = b.subject.trim().toLowerCase()
      if (routines.some((r) => r.from === from && (r.subject ?? "") === subject))
        throw new ApiError("there is already a routine for this mail", 409)
      const folders = JSON.parse(sessionStorage.getItem("cockatoo.mock.folders") ?? "[]") as SaveFolder[]
      const r: Routine = {
        id: Math.max(0, ...routines.map((x) => x.id)) + 1,
        from,
        subject: subject || undefined,
        action: "save",
        folderId: b.folderId,
        path: b.path,
        dated: b.dated || undefined,
        createdAt: new Date().toISOString(),
        folderName: folders.find((f) => f.id === b.folderId)?.name,
        missing: false,
      }
      sessionStorage.setItem(ROUTINES_KEY, JSON.stringify([...routines, r]))
      return r as T
    }
    if (method === "DELETE") {
      const id = Number(path.split("/").pop())
      sessionStorage.setItem(ROUTINES_KEY, JSON.stringify(routines.filter((r) => r.id !== id)))
      return undefined as T
    }
  }
  if (method === "POST" && path === "/api/messages/done") {
    const b = JSON.parse(init!.body as string) as {
      messages: MessageKey[]
      done: boolean
    }
    if (!b.messages.length) throw new ApiError("Choose at least one message.", 400)
    const handled = loadHandled()
    for (const k of b.messages) {
      const h = { ...handled[mockKey(k)] }
      if (b.done) h.doneAt = new Date().toISOString()
      else delete h.doneAt
      handled[mockKey(k)] = h
    }
    sessionStorage.setItem(HANDLED_KEY, JSON.stringify(handled))
    return { marked: b.messages.length } as T
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
    const c = JSON.parse(init!.body as string) as {
      kind: string
      address?: string
      year?: number
    }
    const o = mockOverview(id)
    const row =
      c.kind === "sender" ? o.senders.find((x) => x.address === c.address) : o.years.find((y) => y.year === c.year)
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
    const req = JSON.parse(init!.body as string) as {
      kind: string
      criteria: { kind: string; address?: string; year?: number }
      expected: number
    }
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
  if (path.startsWith("/api/folders") || path === "/api/attachments/save") {
    const FOLDERS_KEY = "cockatoo.mock.folders"
    type MockFolder = {
      id: number
      name: string
      path: string
      createdAt: string
      missing: boolean
      tree: string[]
    }
    const folders = JSON.parse(sessionStorage.getItem(FOLDERS_KEY) ?? "[]") as MockFolder[]
    const store = () => sessionStorage.setItem(FOLDERS_KEY, JSON.stringify(folders))
    const body = init?.body ? (JSON.parse(init.body as string) as Record<string, unknown>) : {}
    const fid = Number(path.match(/^\/api\/folders\/(\d+)/)?.[1])
    const folder = folders.find((f) => f.id === fid)
    const browse = (f: MockFolder, rel: string) => ({
      folderId: f.id,
      name: f.name,
      root: f.path,
      path: rel,
      full: rel ? `${f.path}\\${rel.replaceAll("/", "\\")}` : f.path,
      folders: f.tree
        .filter(
          (p) =>
            (rel ? p.startsWith(rel + "/") : true) && p.split("/").length === (rel ? rel.split("/").length + 1 : 1),
        )
        .map((p) => p.split("/").pop()!)
        .sort(),
    })
    if (method === "GET" && path === "/api/folders") return folders.map(({ tree: _t, ...f }) => f) as T
    if (method === "POST" && path === "/api/folders") {
      const p = String(body.path ?? "")
        .trim()
        .replace(/^"|"$/g, "")
      if (!String(body.name ?? "").trim()) throw new ApiError("Give the folder a name, for example Expenses.", 400)
      if (!/^[A-Za-z]:\\/.test(p))
        throw new ApiError("Enter the full path, starting with a drive letter such as C:\\.", 400)
      const f = {
        id: Math.max(0, ...folders.map((x) => x.id)) + 1,
        name: String(body.name).trim(),
        path: p,
        createdAt: new Date().toISOString(),
        missing: false,
        tree: ["2026-09", "2026-10", "Receipts"],
      }
      folders.push(f)
      store()
      return f as T
    }
    if (method === "DELETE" && folder) {
      folders.splice(folders.indexOf(folder), 1)
      store()
      return undefined as T
    }
    if (method === "GET" && folder && path.includes("/browse")) {
      return browse(folder, new URLSearchParams(path.split("?")[1]).get("path") ?? "") as T
    }
    if (method === "POST" && folder && path.endsWith("/subfolders")) {
      const name = String(body.name ?? "").trim()
      if (!name || /[\\/:*?"<>|]/.test(name) || name === "..")
        throw new ApiError(`"${name}" cannot be a folder name.`, 400)
      const rel = body.path ? `${body.path}/${name}` : name
      if (!folder.tree.includes(rel)) folder.tree.push(rel)
      store()
      return browse(folder, rel) as T
    }
    if (method === "POST" && path === "/api/attachments/save") {
      await wait(1200)
      const f = folders.find((x) => x.id === Number(body.folderId))!
      const dir = browse(f, String(body.path ?? "")).full
      const keys = body.messages as {
        accountId: number
        uidValidity: number
        uid: number
      }[]
      const all = [...mockInbox(load(), true).messages, ...mockOlder(load())]
      const messages = keys.map((key, i) => {
        const m = all.find((x) => mockKey(x) === mockKey(key))
        const about = { key, subject: m?.subject ?? "", from: m?.fromAddr ?? "", receivedAt: m?.receivedAt ?? "" }
        return /invoice|attached/i.test(about.subject)
          ? {
              ...about,
              files: [{ path: `${dir}\\${i === 0 ? "invoice.pdf" : `invoice (${i + 1}).pdf`}`, bytes: 48213 }],
              skipped: i === 0 ? ["a picture shown in the message itself (logo.png)"] : undefined,
            }
          : { ...about, files: [], problem: "The message has no attachments." }
      })
      const handled = loadHandled()
      for (const m of messages) {
        if (!m.files.length) continue
        const h = { ...handled[mockKey(m.key)] }
        h.savedTo = [dir]
        h.savedFiles = (h.savedFiles ?? 0) + m.files.length
        h.savedAt = new Date().toISOString()
        handled[mockKey(m.key)] = h
      }
      sessionStorage.setItem(HANDLED_KEY, JSON.stringify(handled))
      return {
        folder: dir,
        files: messages.reduce((n, m) => n + m.files.length, 0),
        messages,
      } as T
    }
  }
  if (path.startsWith("/api/triage")) {
    const TRIAGE_KEY = "cockatoo.mock.triage"
    const saved = JSON.parse(sessionStorage.getItem(TRIAGE_KEY) ?? "{}") as Record<string, unknown>
    if (method === "GET") {
      const scope = new URLSearchParams(path.split("?")[1] ?? "").get("scope") ?? "all"
      return (saved[scope] ?? null) as T
    }
    await wait(2000)
    const scope = (JSON.parse(init!.body as string) as { scope: string }).scope
    const inbox = mockInbox(accounts, true).messages.filter((m) => scope === "all" || String(m.accountId) === scope)
    const unread = inbox.filter((m) => !m.seen)
    const older: MailMessage = {
      ...unread[0],
      uid: 1,
      fromName: "Thandi Mokoena",
      fromAddr: "thandi@example.com",
      subject: "Contract changes – need your sign-off",
      receivedAt: new Date(Date.now() - 3 * 86_400_000).toISOString(),
    }
    const picks = [
      {
        message: older,
        priority: "now",
        reason: "Thandi asks you to sign off the contract changes; she has waited three days.",
        overdue: true,
        signals: {
          toYou: true,
          ccYou: false,
          repliesToYou: true,
          wrote: 14,
          automated: false,
        },
      },
      ...(unread[0]
        ? [
            {
              message: unread[0],
              priority: "now",
              reason: "Renée needs your comments on Thursday's draft.",
              overdue: false,
              signals: {
                toYou: true,
                ccYou: false,
                repliesToYou: false,
                wrote: 3,
                automated: false,
              },
            },
          ]
        : []),
      ...(unread[0]
        ? [
            {
              message: {
                ...unread[0],
                uid: 2,
                fromName: "Lindiwe Khumalo",
                fromAddr: "lindiwe@example.com",
                subject: "Quote for the garden wall",
                receivedAt: new Date(Date.now() - 26 * 3_600_000).toISOString(),
              },
              priority: "today",
              reason: "Lindiwe sent the quote you asked for and needs a yes by Friday.",
              overdue: false,
              signals: {
                toYou: true,
                ccYou: false,
                repliesToYou: false,
                wrote: 2,
                automated: false,
              },
            },
          ]
        : []),
    ]
    const result = {
      id: Date.now(),
      askedAt: new Date().toISOString(),
      model: "claude-opus-5-5",
      summary:
        "Thandi is waiting on your sign-off of the contract changes, and Renée needs comments on Thursday's draft.",
      picks,
      sent: 41,
      leftOut: 0,
      leftAutomated: 312,
      leftAnswered: 4,
      textSent: (JSON.parse(init!.body as string) as { withText: boolean }).withText,
      signalsPartial: false,
      partial: false,
      dropped: 0,
      inputTokens: 3840,
      outputTokens: 212,
    }
    saved[scope] = result
    sessionStorage.setItem(TRIAGE_KEY, JSON.stringify(saved))
    return result as T
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
