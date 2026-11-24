import { useEffect, useState } from "react"
import { ExternalLink, Mail } from "lucide-react"

import {
  api,
  type Account,
  type ActionKind,
  type ActionOutcome,
  type ActionPreview,
  type Criteria,
  type Overview,
  type SenderCount,
} from "@/api"
import { Button, buttonVariants } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { SyncLine } from "@/components/sync-line"
import { useSync } from "@/hooks/use-sync"
import { cn, tabClass } from "@/lib/utils"
import { Working } from "@/components/working"

type Load = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; overview: Overview }

// An action the user has started and not yet confirmed: which row, what to do.
type Pending = { key: string; kind: ActionKind; criteria: Criteria; label: string }

const n = (v: number) => v.toLocaleString()
const year = (iso: string) => new Date(iso).getFullYear()
const where = (kind: ActionKind) => (kind === "spam" ? "Spam" : "Trash")

// Clean up: the shape of one account's whole inbox, with the actions on the rows themselves.
// Steps one and two of SCOPE.md section 9. Every action is confirmed in its row and audited.
export function CleanupScreen({ accounts }: { accounts: Account[] }) {
  const [accountId, setAccountId] = useState(accounts[0].id)
  const [load, setLoad] = useState<Load>({ kind: "loading" })
  const [reload, setReload] = useState(0)
  const [pending, setPending] = useState<Pending | null>(null)
  const [result, setResult] = useState<ActionOutcome | null>(null)
  const sync = useSync()

  useEffect(() => {
    api
      .overview(accountId)
      .then((overview) => setLoad({ kind: "ready", overview }))
      .catch((e: Error) => setLoad({ kind: "error", message: e.message }))
  }, [accountId, sync.tick, reload])

  const o = load.kind === "ready" && load.overview.accountId === accountId ? load.overview : null
  const running = sync.status?.running ?? true
  const canAct = Boolean(o?.sync.complete) && !running

  function done(outcome: ActionOutcome) {
    setPending(null)
    setResult(outcome)
    setReload((r) => r + 1)
  }

  const actions: Actions = { accountId, pending, canAct, start: setPending, cancel: () => setPending(null), done }

  return (
    <div>
      <h1 className="text-[2rem] leading-tight font-bold tracking-[-0.025em]">Clean up</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        {o && o.messages > 0
          ? `${n(o.messages)} messages in your inbox · ${n(o.unread)} unread · since ${o.oldest ? year(o.oldest) : "—"}`
          : "The shape of your inbox, from every message's sender and date."}
      </p>

      {accounts.length > 1 && (
        <div className="mt-5 flex flex-wrap gap-x-5 gap-y-1" role="group" aria-label="Account">
          {accounts.map((a) => (
            <Button
              key={a.id}
              variant="ghost"
              className={tabClass(a.id === accountId)}
              aria-pressed={a.id === accountId}
              onClick={() => {
                setAccountId(a.id)
                setPending(null)
                setResult(null)
              }}
            >
              {a.name}
            </Button>
          ))}
        </div>
      )}

      <div className="mt-3 space-y-1">
        <SyncLine status={sync.status} error={sync.error} />
        {o && !o.sync.complete && o.messages > 0 && (
          <p className="text-sm text-muted-foreground">
            Cockatoo has read {n(o.messages)} of {n(o.sync.serverTotal)} messages so far. The figures below grow as it
            reads the rest, and Trash and Spam turn on when it has read them all.
          </p>
        )}
      </div>

      <div aria-live="polite">{result && <ResultLine result={result} onClose={() => setResult(null)} />}</div>

      {load.kind === "loading" && <Working className="mt-8">Opening…</Working>}
      {load.kind === "error" && (
        <p role="alert" className="mt-8 text-sm text-destructive">
          Cockatoo could not read its copy of your inbox: {load.message}
        </p>
      )}

      {o && o.messages === 0 && (
        <p className="mt-12 text-center text-sm text-muted-foreground">
          {running ? "Reading your mailbox…" : "Your inbox is empty."}
        </p>
      )}

      {o && o.messages > 0 && (
        <div className="mt-8 grid gap-x-12 gap-y-12 xl:grid-cols-[minmax(0,4fr)_minmax(0,7fr)]">
          <Years overview={o} actions={actions} />
          <Senders overview={o} actions={actions} />
        </div>
      )}
    </div>
  )
}

type Actions = {
  accountId: number
  pending: Pending | null
  canAct: boolean
  start: (p: Pending) => void
  cancel: () => void
  done: (o: ActionOutcome) => void
}

// How much mail arrived each year, as a bar per year, with Trash on the row.
function Years({ overview: o, actions }: { overview: Overview; actions: Actions }) {
  const most = Math.max(...o.years.map((y) => y.messages))
  return (
    <section aria-labelledby="years-heading">
      <h2 id="years-heading" className="section-head">
        By year
      </h2>
      <table className="mt-3 w-full text-sm">
        <thead className="sr-only">
          <tr>
            <th>Year</th>
            <th>Messages</th>
            <th>Unread</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {[...o.years].reverse().map((y) => {
            const key = `year-${y.year}`
            const open = actions.pending?.key === key
            return <YearRow key={key} rowKey={key} y={y} most={most} open={open} actions={actions} />
          })}
        </tbody>
      </table>
    </section>
  )
}

function YearRow({
  rowKey,
  y,
  most,
  open,
  actions,
}: {
  rowKey: string
  y: Overview["years"][number]
  most: number
  open: boolean
  actions: Actions
}) {
  return (
    <>
      <tr
        title={`${y.year}: ${n(y.messages)} messages, ${n(y.unread)} unread`}
        className={cn("group hover:bg-muted/50", open && "bg-muted/50")}
      >
        <td className="w-12 py-1 pr-3 tabular-nums text-muted-foreground group-hover:text-foreground">{y.year}</td>
        <td className="py-1">
          <div className="flex items-center gap-2">
            <div
              className="h-2 rounded-r-[4px] bg-foreground/70 group-hover:bg-foreground"
              style={{ width: `${Math.max(1, (y.messages / most) * 100)}%` }}
            />
            <span className="shrink-0 tabular-nums">{n(y.messages)}</span>
          </div>
        </td>
        <td className="w-24 py-1 pl-3 text-right tabular-nums text-muted-foreground">
          {y.unread > 0 ? `${n(y.unread)} unread` : ""}
        </td>
        <td className="w-16 py-1 pl-2 text-right">
          <Button
            size="xs"
            variant="ghost"
            disabled={!actions.canAct || open}
            onClick={() =>
              actions.start({
                key: rowKey,
                kind: "trash",
                criteria: { kind: "year", year: y.year },
                label: `received in ${y.year}`,
              })
            }
          >
            Trash
          </Button>
        </td>
      </tr>
      {open && actions.pending && (
        <tr>
          <td colSpan={4} className="pb-3">
            <Confirm pending={actions.pending} actions={actions} />
          </td>
        </tr>
      )}
    </>
  )
}

// The senders with the most mail, or the ones whose mail says how to unsubscribe, with Trash,
// Spam and Unsubscribe on each row.
function Senders({ overview: o, actions }: { overview: Overview; actions: Actions }) {
  const [only, setOnly] = useState<"all" | "unsubscribe">("all")
  const rows = only === "all" ? o.senders : o.unsubscribable
  return (
    <section aria-labelledby="senders-heading">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 id="senders-heading" className="section-head">
          Senders
        </h2>
        <div className="flex gap-1" role="group" aria-label="Which senders">
          <Button
            size="sm"
            variant={only === "all" ? "secondary" : "ghost"}
            aria-pressed={only === "all"}
            onClick={() => setOnly("all")}
          >
            Most mail
          </Button>
          <Button
            size="sm"
            variant={only === "unsubscribe" ? "secondary" : "ghost"}
            aria-pressed={only === "unsubscribe"}
            onClick={() => setOnly("unsubscribe")}
          >
            Can unsubscribe · {o.unsubscribable.length}
          </Button>
        </div>
      </div>

      {rows.length === 0 ? (
        <p className="mt-6 text-sm text-muted-foreground">
          {only === "unsubscribe" ? "None of the mail cockatoo has read says how to unsubscribe." : "No senders yet."}
        </p>
      ) : (
        <table className="mt-3 w-full table-fixed text-sm">
          <thead>
            <tr className="text-left text-xs text-muted-foreground">
              <th className="pb-2 font-normal">Sender</th>
              <th className="w-16 pb-2 pl-3 text-right font-normal sm:w-20">Messages</th>
              <th className="w-16 pb-2 pl-3 text-right font-normal">Unread</th>
              <th className="hidden w-24 pb-2 pl-3 text-right font-normal md:table-cell">Years</th>
              <th className="w-[14rem] pb-2 pl-3 font-normal">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody className="divide-y border-y">
            {rows.map((s) => (
              <SenderRow key={s.address || "(none)"} sender={s} actions={actions} />
            ))}
          </tbody>
        </table>
      )}
    </section>
  )
}

function SenderRow({ sender: s, actions }: { sender: SenderCount; actions: Actions }) {
  const key = `sender-${s.address}`
  const open = actions.pending?.key === key
  const from = year(s.oldest)
  const to = year(s.latest)
  const who = s.name ? `${s.name} (${s.address})` : s.address
  const start = (kind: ActionKind) =>
    actions.start({ key, kind, criteria: { kind: "sender", address: s.address }, label: `from ${who}` })

  return (
    <>
      <tr className={cn("hover:bg-muted/50", open && "bg-muted/50")}>
        <td className="py-2">
          <p className="truncate">{s.name || s.address || "(unknown sender)"}</p>
          {s.name && <p className="truncate text-xs text-muted-foreground">{s.address}</p>}
          {s.unsubscribe && <p className="truncate text-xs text-muted-foreground">{unsubscribeWhere(s.unsubscribe)}</p>}
        </td>
        <td className="py-2 pl-3 text-right tabular-nums">{n(s.messages)}</td>
        <td className={cn("py-2 pl-3 text-right tabular-nums", s.unread === 0 && "text-muted-foreground")}>
          {n(s.unread)}
        </td>
        <td className="hidden whitespace-nowrap py-2 pl-3 text-right tabular-nums text-muted-foreground md:table-cell">
          {from === to ? from : `${from}–${to}`}
        </td>
        <td className="py-2 pl-3">
          {s.address ? (
            <div className="flex items-center justify-end gap-0.5">
              <Button size="xs" variant="ghost" disabled={!actions.canAct || open} onClick={() => start("trash")}>
                Trash
              </Button>
              <Button size="xs" variant="ghost" disabled={!actions.canAct || open} onClick={() => start("spam")}>
                Spam
              </Button>
              <Unsubscribe sender={s} accountId={actions.accountId} />
            </div>
          ) : (
            <p className="text-right text-xs text-muted-foreground">No sender address</p>
          )}
        </td>
      </tr>
      {open && actions.pending && (
        <tr>
          <td colSpan={5} className="pb-3">
            <Confirm pending={actions.pending} actions={actions} />
          </td>
        </tr>
      )}
    </>
  )
}

// The sender's own unsubscribe address, as a real link. Cockatoo records that it was opened and
// fetches nothing itself; the page, or the email it writes, does the unsubscribing.
function Unsubscribe({ sender: s, accountId }: { sender: SenderCount; accountId: number }) {
  const u = s.unsubscribe
  const target = u?.url ?? u?.mailto
  if (!u || !target) {
    return <span className="inline-block w-[7rem]" aria-hidden />
  }
  const byEmail = !u.url
  const host = byEmail ? target.replace(/^mailto:/i, "").split("?")[0] : safeHost(target)
  return (
    <a
      href={target}
      target={byEmail ? undefined : "_blank"}
      rel="noopener noreferrer"
      title={byEmail ? `Writes an unsubscribe email to ${host}` : `Opens ${host}`}
      onClick={() => {
        api.logUnsubscribe(accountId, s.address, target).catch(() => {})
      }}
      className={cn(buttonVariants({ size: "xs", variant: "ghost" }), "w-[7rem] justify-start")}
    >
      {byEmail ? <Mail aria-hidden /> : <ExternalLink aria-hidden />}
      Unsubscribe
    </a>
  )
}

// "Unsubscribe opens news.example" or "Unsubscribe emails leave@news.example".
function unsubscribeWhere(u: NonNullable<SenderCount["unsubscribe"]>): string {
  if (u.url) return `Unsubscribe opens ${safeHost(u.url)}`
  return `Unsubscribe emails ${(u.mailto ?? "").replace(/^mailto:/i, "").split("?")[0]}`
}

function safeHost(url: string): string {
  try {
    return new URL(url).host
  } catch {
    return url
  }
}

type ConfirmState =
  | { kind: "loading" }
  | { kind: "ready"; preview: ActionPreview }
  | { kind: "busy"; preview: ActionPreview }
  | { kind: "error"; message: string; preview?: ActionPreview }

// The confirm, in the row: exactly what will move, then the user's click.
function Confirm({ pending, actions }: { pending: Pending; actions: Actions }) {
  const [state, setState] = useState<ConfirmState>({ kind: "loading" })
  const { accountId } = actions

  useEffect(() => {
    api
      .previewAction(accountId, pending.criteria)
      .then((preview) => setState({ kind: "ready", preview }))
      .catch((e: Error) => setState({ kind: "error", message: e.message }))
  }, [accountId, pending.criteria])

  async function apply(preview: ActionPreview) {
    setState({ kind: "busy", preview })
    try {
      actions.done(await api.applyAction(accountId, pending.kind, pending.criteria, preview.messages))
    } catch (e) {
      setState({ kind: "error", message: (e as Error).message, preview })
    }
  }

  const preview = "preview" in state ? state.preview : undefined
  const dest = where(pending.kind)

  return (
    <div className="rounded-lg border bg-card px-4 py-3 text-sm">
      {state.kind === "loading" && <p className="text-muted-foreground">Counting…</p>}
      {preview && (
        <>
          <p>
            Move {n(preview.messages)} {preview.messages === 1 ? "message" : "messages"} {pending.label} to {dest}?
            {preview.unread > 0 && ` ${n(preview.unread)} of them ${preview.unread === 1 ? "is" : "are"} unread.`}
          </p>
          {preview.subjects.length > 0 && (
            <ul className="mt-2 space-y-0.5 text-muted-foreground">
              {preview.subjects.map((subject, i) => (
                <li key={i} className="truncate">
                  {subject || "(no subject)"}
                </li>
              ))}
            </ul>
          )}
          <p className="mt-2 text-muted-foreground">
            {pending.kind === "spam"
              ? "They go to Gmail's Spam folder, where Gmail keeps them for 30 days."
              : "You can restore them from Gmail's Trash for 30 days."}
          </p>
        </>
      )}
      {state.kind === "error" && (
        <p role="alert" className="mt-2 text-destructive">
          {state.message}
        </p>
      )}
      <div className="mt-3 flex gap-2">
        {preview && (
          <Button
            variant={pending.kind === "spam" ? "destructive" : "default"}
            onClick={() => apply(preview)}
            disabled={state.kind === "busy" || preview.messages === 0}
          >
            {state.kind === "busy" && <Spinner />}
            {state.kind === "busy" ? `Moving ${n(preview.messages)}…` : `Move to ${dest}`}
          </Button>
        )}
        <Button variant="ghost" onClick={actions.cancel} disabled={state.kind === "busy"}>
          Cancel
        </Button>
      </div>
    </div>
  )
}

function ResultLine({ result: r, onClose }: { result: ActionOutcome; onClose: () => void }) {
  const failed = r.outcome !== "done"
  return (
    <div
      role={failed ? "alert" : "status"}
      className={cn(
        "mt-4 flex items-start justify-between gap-4 rounded-lg border px-4 py-3 text-sm",
        failed && "border-destructive/40 text-destructive",
      )}
    >
      <p>
        {r.outcome === "done" &&
          `Done: ${r.description.replace(/^Move/, "moved").replace(/ \([^)]*\)$/, "")}. You can restore them from Gmail for 30 days.`}
        {r.outcome === "partial" &&
          `Moved ${n(r.moved)} of ${n(r.moved + r.notMoved)}. ${n(r.notMoved)} are still in your inbox. ${r.error ?? ""}`}
        {r.outcome === "failed" && `Nothing was moved. ${r.error ?? ""}`}
        {r.outcome === "unknown" &&
          `Cockatoo lost the connection and could not confirm what moved. Look in Gmail's Trash or Spam; the next check of your mailbox sets this list right. ${r.error ?? ""}`}
      </p>
      <Button size="xs" variant="ghost" onClick={onClose}>
        Close
      </Button>
    </div>
  )
}
