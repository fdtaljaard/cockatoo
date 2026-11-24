import { useEffect, useState } from "react"

import { api, type Account, type InboxList, type MailMessage } from "@/api"
import { Button } from "@/components/ui/button"
import { SyncLine } from "@/components/sync-line"
import { useSync } from "@/hooks/use-sync"
import { cn } from "@/lib/utils"

type Load = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; list: InboxList }

// Which account the Inbox shows, kept between visits on this computer.
const SHOWN_KEY = "cockatoo.inbox.account"

// The last 24 hours of every account's inbox, newest first, read from cockatoo's copy. A sync
// starts when the screen opens; the list follows it as new mail arrives in the copy. With several
// accounts, buttons above the list show all of them together or one at a time.
export function InboxScreen({ accounts }: { accounts: Account[] }) {
  const [load, setLoad] = useState<Load>({ kind: "loading" })
  const [shown, setShown] = useState<number | "all">(() => {
    const saved = Number(localStorage.getItem(SHOWN_KEY))
    return accounts.some((a) => a.id === saved) ? saved : "all"
  })
  const sync = useSync()

  function show(which: number | "all") {
    setShown(which)
    localStorage.setItem(SHOWN_KEY, String(which))
  }

  useEffect(() => {
    api
      .storedInbox()
      .then((list) => setLoad({ kind: "ready", list }))
      .catch((e: Error) => setLoad((now) => (now.kind === "ready" ? now : { kind: "error", message: e.message })))
  }, [sync.tick])

  const names = new Map(accounts.map((a) => [a.id, a.name]))
  const severalAccounts = accounts.length > 1
  // An account removed since it was chosen falls back to all of them.
  const current = shown !== "all" && accounts.some((a) => a.id === shown) ? shown : "all"
  const list = load.kind === "ready" ? load.list : null
  const messages = list ? list.messages.filter((m) => current === "all" || m.accountId === current) : []
  const unread = messages.filter((m) => !m.seen).length
  const unreadOf = (id: number) => list?.messages.filter((m) => m.accountId === id && !m.seen).length ?? 0
  const running = sync.status?.running ?? true

  return (
    <div>
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Inbox</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Last 24 hours{messages.length > 0 ? ` · ${unread} unread` : ""}
          </p>
        </div>
        <Button variant="outline" onClick={sync.start} disabled={running}>
          {running ? "Checking…" : "Refresh"}
        </Button>
      </div>

      {severalAccounts && (
        <div className="mt-4 flex flex-wrap gap-1" role="group" aria-label="Which account">
          <Button variant={current === "all" ? "secondary" : "ghost"} aria-pressed={current === "all"} onClick={() => show("all")}>
            All
            {list && <Count n={list.messages.filter((m) => !m.seen).length} />}
          </Button>
          {accounts.map((a) => (
            <Button
              key={a.id}
              variant={current === a.id ? "secondary" : "ghost"}
              aria-pressed={current === a.id}
              onClick={() => show(a.id)}
            >
              {a.name}
              {list && <Count n={unreadOf(a.id)} />}
            </Button>
          ))}
        </div>
      )}

      <div className="mt-3">
        <SyncLine status={sync.status} error={sync.error} />
      </div>

      {load.kind === "loading" && <p className="mt-6 text-sm text-muted-foreground">Opening…</p>}
      {load.kind === "error" && (
        <p role="alert" className="mt-6 text-sm text-destructive">
          Cockatoo could not read its stored mail: {load.message}
        </p>
      )}

      {list && messages.length === 0 && (
        <p className="mt-10 text-center text-sm text-muted-foreground">
          {running
            ? "Checking for mail…"
            : current === "all"
              ? "No mail in the last 24 hours."
              : `No mail in the last 24 hours for ${names.get(current)}.`}
        </p>
      )}

      {messages.length > 0 && (
        <ul className="mt-4 divide-y border-y">
          {messages.map((m) => (
            <Row
              key={`${m.accountId}-${m.uidValidity}-${m.uid}`}
              message={m}
              account={severalAccounts && current === "all" ? names.get(m.accountId) : undefined}
            />
          ))}
        </ul>
      )}
    </div>
  )
}

function Row({ message: m, account }: { message: MailMessage; account?: string }) {
  const sender = m.fromName || m.fromAddr || "(unknown sender)"
  return (
    <li className="flex items-baseline gap-3 py-3">
      <span
        aria-hidden
        className={cn("size-2 shrink-0 translate-y-[-1px] rounded-full", m.seen ? "bg-transparent" : "bg-primary")}
      />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline justify-between gap-3">
          <p className={cn("truncate text-sm", m.seen ? "text-muted-foreground" : "font-semibold")} title={m.fromAddr}>
            {!m.seen && <span className="sr-only">Unread. </span>}
            {sender}
          </p>
          <time dateTime={m.receivedAt} className="shrink-0 text-xs tabular-nums text-muted-foreground">
            {when(m.receivedAt)}
          </time>
        </div>
        <p className={cn("truncate text-sm", m.seen ? "text-muted-foreground" : "text-foreground")}>
          {m.subject || <span className="italic">(no subject)</span>}
        </p>
        {account && <p className="mt-0.5 text-xs text-muted-foreground">{account}</p>}
      </div>
    </li>
  )
}

// "14:05" for today, "Yesterday 14:05" before that.
function when(iso: string): string {
  const d = new Date(iso)
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  const today = new Date()
  return d.toDateString() === today.toDateString() ? time : `Yesterday ${time}`
}

// The unread count on an account button; nothing when there is none.
function Count({ n }: { n: number }) {
  if (n === 0) return null
  return (
    <span className="tabular-nums text-muted-foreground">
      {n.toLocaleString()}
      <span className="sr-only"> unread</span>
    </span>
  )
}
