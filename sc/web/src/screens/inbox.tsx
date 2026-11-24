import { useEffect, useState } from "react"
import { CircleCheck } from "lucide-react"

import { api, type Account, type Handled, type InboxList, type MailMessage, type MessageKey } from "@/api"
import { Button } from "@/components/ui/button"
import { SaveDialog } from "@/components/save-dialog"
import { SyncLine } from "@/components/sync-line"
import { Checkbox } from "@/components/ui/checkbox"
import { TriagePanel } from "@/components/triage-panel"
import { useSync } from "@/hooks/use-sync"
import { cn } from "@/lib/utils"

type Load = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; list: InboxList }

// Which account the Inbox shows, kept between visits on this computer.
const SHOWN_KEY = "cockatoo.inbox.account"

// The last 24 hours of every account's inbox, newest first, read from cockatoo's copy. A sync
// starts when the screen opens; the list follows it as new mail arrives in the copy. With several
// accounts, buttons above the list show all of them together or one at a time.
export function InboxScreen({ accounts, onGoToAI }: { accounts: Account[]; onGoToAI: () => void }) {
  const [load, setLoad] = useState<Load>({ kind: "loading" })
  const [shown, setShown] = useState<number | "all">(() => {
    const saved = Number(localStorage.getItem(SHOWN_KEY))
    return accounts.some((a) => a.id === saved) ? saved : "all"
  })
  const sync = useSync()
  // Chosen messages, by key, kept while the list reloads and the account filter changes.
  const [chosen, setChosen] = useState<Set<string>>(() => new Set())
  const [saving, setSaving] = useState(false)
  // Bumped after you mark or save something, so the list and the AI's picks read it again.
  const [changed, setChanged] = useState(0)
  const [actionError, setActionError] = useState<string | null>(null)

  function show(which: number | "all") {
    setShown(which)
    localStorage.setItem(SHOWN_KEY, String(which))
  }

  useEffect(() => {
    api
      .storedInbox()
      .then((list) => setLoad({ kind: "ready", list }))
      .catch((e: Error) => setLoad((now) => (now.kind === "ready" ? now : { kind: "error", message: e.message })))
  }, [sync.tick, changed])

  const names = new Map(accounts.map((a) => [a.id, a.name]))
  const severalAccounts = accounts.length > 1
  // An account removed since it was chosen falls back to all of them.
  const current = shown !== "all" && accounts.some((a) => a.id === shown) ? shown : "all"
  const list = load.kind === "ready" ? load.list : null
  const messages = list ? list.messages.filter((m) => current === "all" || m.accountId === current) : []
  const unread = messages.filter((m) => !m.seen).length
  const dealtWith = messages.filter((m) => m.handled).length
  const unreadOf = (id: number) => list?.messages.filter((m) => m.accountId === id && !m.seen).length ?? 0
  const running = sync.status?.running ?? true
  const chosenHere = messages.filter((m) => chosen.has(keyOf(m)))
  const allChosen = messages.length > 0 && chosenHere.length === messages.length

  function toggle(m: MailMessage, on: boolean) {
    setChosen((prev) => {
      const next = new Set(prev)
      if (on) next.add(keyOf(m))
      else next.delete(keyOf(m))
      return next
    })
  }

  async function markDone(keys: MessageKey[], done: boolean) {
    setActionError(null)
    try {
      await api.markDone(keys.map(keyOnly), done)
      setChanged((c) => c + 1)
      return true
    } catch (e) {
      setActionError((e as Error).message)
      return false
    }
  }

  function toggleAll(on: boolean) {
    setChosen((prev) => {
      const next = new Set(prev)
      for (const m of messages) {
        if (on) next.add(keyOf(m))
        else next.delete(keyOf(m))
      }
      return next
    })
  }

  return (
    <div>
      <div className="flex items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Inbox</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Last 24 hours
            {messages.length > 0 && ` · ${unread} unread`}
            {dealtWith > 0 && ` · ${dealtWith} done`}
          </p>
        </div>
        <Button variant="outline" onClick={sync.start} disabled={running}>
          {running ? "Checking…" : "Refresh"}
        </Button>
      </div>

      {severalAccounts && (
        <div className="mt-4 flex flex-wrap gap-1" role="group" aria-label="Which account">
          <Button
            variant={current === "all" ? "secondary" : "ghost"}
            aria-pressed={current === "all"}
            onClick={() => show("all")}
          >
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

      <TriagePanel
        scope={current}
        accountNames={names}
        showAccount={severalAccounts && current === "all"}
        syncRunning={running}
        onGoToAI={onGoToAI}
        refresh={changed}
        onDone={(m) => markDone([m], true)}
      />

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
        <div className="sticky top-0 z-10 mt-4 flex min-h-11 flex-wrap items-center gap-3 border-b bg-background py-2">
          <Checkbox
            aria-label={allChosen ? "Clear the selection" : "Choose all messages shown"}
            checked={allChosen}
            onCheckedChange={(c) => toggleAll(c === true)}
          />
          {chosenHere.length === 0 ? (
            <span className="text-sm text-muted-foreground">
              Choose messages to save their attachments or mark them done
            </span>
          ) : (
            <>
              <span className="text-sm tabular-nums">{chosenHere.length} chosen</span>
              <Button size="sm" onClick={() => setSaving(true)}>
                Save attachments
              </Button>
              <Button
                size="sm"
                variant="outline"
                onClick={async () => {
                  if (await markDone(chosenHere, true)) toggleAll(false)
                }}
              >
                Mark done
              </Button>
              <Button size="sm" variant="ghost" onClick={() => toggleAll(false)}>
                Clear
              </Button>
            </>
          )}
        </div>
      )}

      {actionError && (
        <p role="alert" className="mt-2 text-sm text-destructive">
          {actionError}
        </p>
      )}

      {messages.length > 0 && (
        <ul className="divide-y border-b">
          {messages.map((m) => (
            <Row
              key={keyOf(m)}
              message={m}
              account={severalAccounts && current === "all" ? names.get(m.accountId) : undefined}
              chosen={chosen.has(keyOf(m))}
              onChoose={(on) => toggle(m, on)}
              onDone={(done) => markDone([m], done)}
            />
          ))}
        </ul>
      )}

      {saving && (
        <SaveDialog
          messages={chosenHere.map(keyOnly)}
          onClose={(saved) => {
            setSaving(false)
            if (saved) {
              toggleAll(false)
              setChanged((c) => c + 1)
            }
          }}
        />
      )}
    </div>
  )
}

const keyOf = (m: MessageKey) => `${m.accountId}-${m.uidValidity}-${m.uid}`
const keyOnly = (m: MessageKey): MessageKey => ({
  accountId: m.accountId,
  uidValidity: m.uidValidity,
  uid: m.uid,
})

function Row({
  message: m,
  account,
  chosen,
  onChoose,
  onDone,
}: {
  message: MailMessage
  account?: string
  chosen: boolean
  onChoose: (on: boolean) => void
  onDone: (done: boolean) => void
}) {
  const sender = m.fromName || m.fromAddr || "(unknown sender)"
  const h = m.handled
  return (
    <li className={cn("flex items-baseline gap-3 px-2 py-3", chosen ? "bg-muted/60" : h && "bg-done/15")}>
      <Checkbox
        aria-label={`Choose: ${m.subject || "(no subject)"} from ${sender}`}
        checked={chosen}
        onCheckedChange={(c) => onChoose(c === true)}
        className="translate-y-[3px]"
      />
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
        {h && <HandledLine h={h} />}
        {account && <p className="mt-0.5 text-xs text-muted-foreground">{account}</p>}
      </div>
      {/* One width for every row, so the times line up whether or not a row has a button. */}
      <div className="flex w-20 shrink-0 justify-end self-center">
        {!h && (
          <Button size="sm" variant="outline" onClick={() => onDone(true)}>
            Done
          </Button>
        )}
        {h?.doneAt && (
          <Button size="sm" variant="ghost" onClick={() => onDone(false)}>
            Not done
          </Button>
        )}
      </div>
    </li>
  )
}

// What you did about a message, in one line: "Saved 2 files to C:\Expenses\2026-11 · Replied".
function HandledLine({ h }: { h: Handled }) {
  const parts: string[] = []
  if (h.savedFiles) {
    const files = `${h.savedFiles} ${h.savedFiles === 1 ? "file" : "files"}`
    parts.push(`Saved ${files} to ${(h.savedTo ?? []).join(", ")}`)
  }
  if (h.replied) parts.push("Replied")
  if (h.doneAt) parts.push("Done")
  return (
    <p className="mt-1 flex items-start gap-1.5 text-xs text-foreground">
      <CircleCheck aria-hidden className="size-3.5 shrink-0 translate-y-px text-done" />
      <span className="min-w-0 break-all">{parts.join(" · ")}</span>
    </p>
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
