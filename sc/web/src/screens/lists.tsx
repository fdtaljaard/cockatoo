import { useCallback, useEffect, useRef, useState } from "react"
import { Trash2, X } from "lucide-react"

import {
  api,
  type Account,
  type ListName,
  type ListsView,
  type MailMessage,
  type MessageKey,
  type Proposal,
  type Routine,
  type RoutineMatch,
} from "@/api"
import { ConversationView } from "@/components/conversation"
import { PenTick } from "@/components/marks"
import { SaveDialog } from "@/components/save-dialog"
import { SyncLine } from "@/components/sync-line"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"
import { Working } from "@/components/working"
import { useSync } from "@/hooks/use-sync"
import { when } from "@/lib/format"
import { LISTS, listLabel } from "@/lib/lists"
import { domainAllowed, domainOf } from "@/lib/routine"
import { cn, tabClass } from "@/lib/utils"
import { OpenButton } from "@/screens/inbox"

type Load = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; view: ListsView }

// Kept between visits on this computer: which account, how far back, and whether mail you have
// dealt with shows. The first lines setting is the AI panel's.
const SHOWN_KEY = "cockatoo.inbox.account"
const ALL_KEY = "cockatoo.lists.all"
const DONE_KEY = "cockatoo.lists.showDone"
const TEXT_KEY = "cockatoo.triage.withText"

const keyOf = (m: MessageKey) => `${m.accountId}-${m.uidValidity}-${m.uid}`
const keyOnly = (m: MessageKey): MessageKey => ({ accountId: m.accountId, uidValidity: m.uidValidity, uid: m.uid })

// One list of mail (Reply, Attention, Save or Unsorted), with the new mail no rule has sorted at
// the top, to deal with as it comes in (SCOPE.md, section 8a). Every row can move to another
// list or say it does not belong where it is; after either, cockatoo asks how to handle mail like it
// from now on. That is how cockatoo learns how you work, as mail comes in.
export function ListScreen({
  list,
  accounts,
  onCounts,
  onGo,
  onGoToAI,
}: {
  list: ListName
  accounts: Account[]
  onCounts: (counts: ListsView["counts"]) => void
  onGo: (list: ListName) => void
  onGoToAI: () => void
}) {
  const info = LISTS.find((l) => l.id === list)!
  const sync = useSync()
  const [load, setLoad] = useState<Load>({ kind: "loading" })
  const [changed, setChanged] = useState(0)
  const [shown, setShown] = useState<number | "all">(() => {
    const saved = Number(localStorage.getItem(SHOWN_KEY))
    return accounts.some((a) => a.id === saved) ? saved : "all"
  })
  const current = shown !== "all" && accounts.some((a) => a.id === shown) ? shown : "all"
  const [all, setAll] = useState(() => localStorage.getItem(ALL_KEY) === "true")
  const [showDone, setShowDone] = useState(() => localStorage.getItem(DONE_KEY) === "true")
  const [rules, setRules] = useState<Map<number, Routine>>(() => new Map())
  const [aiReady, setAiReady] = useState<boolean | null>(null)
  const [chosen, setChosen] = useState<Set<string>>(() => new Set())
  const [busy, setBusy] = useState<Set<string>>(() => new Set())
  const [error, setError] = useState<string | null>(null)
  const [notice, setNotice] = useState<string | null>(null)
  const [undo, setUndo] = useState<MessageKey[] | null>(null)
  // After a move by hand: what moved, from where, by which rule, so the next step can ask about mail like it.
  const [offer, setOffer] = useState<{
    message: MessageKey
    subject: string
    addr: string
    from: ListName
    to: ListName
    rule?: Routine
  } | null>(null)
  const [saving, setSaving] = useState<{ messages: MessageKey[]; start?: RoutineMatch } | null>(null)
  const [suggesting, setSuggesting] = useState(false)
  const asked = useRef<Set<string>>(new Set()) // new mail already sent for a suggestion this visit

  useEffect(() => {
    let live = true
    api
      .lists(list, { all, account: current, done: showDone })
      .then((view) => {
        if (!live) return
        setLoad({ kind: "ready", view })
        onCounts(view.counts)
      })
      .catch(
        (e: Error) => live && setLoad((now) => (now.kind === "ready" ? now : { kind: "error", message: e.message })),
      )
    return () => {
      live = false
    }
  }, [list, all, current, showDone, changed, sync.tick, onCounts])

  useEffect(() => {
    api.routines().then(
      (rs) => setRules(new Map(rs.map((r) => [r.id, r]))),
      () => {},
    )
  }, [changed])

  useEffect(() => {
    api.aiSettings().then(
      (s) => setAiReady(s.enabled && s.hasKey),
      () => setAiReady(false),
    )
  }, [])

  const view = load.kind === "ready" ? load.view : null
  const names = new Map(accounts.map((a) => [a.id, a.name]))
  const several = accounts.length > 1
  const withText = localStorage.getItem(TEXT_KEY) !== "false"

  // New mail is suggested as it arrives: each message once, on your key.
  useEffect(() => {
    if (!view || !aiReady || suggesting) return
    const todo = view.new.filter((m) => !m.suggestion && !asked.current.has(keyOf(m))).slice(0, 20)
    if (todo.length === 0) return
    for (const m of todo) asked.current.add(keyOf(m))
    setSuggesting(true)
    api
      .suggest(todo.map(keyOnly), withText)
      .then(() => setChanged((c) => c + 1))
      .catch((e: Error) => setError(`The AI could not suggest lists for your new mail: ${e.message}`))
      .finally(() => setSuggesting(false))
  }, [view, aiReady, suggesting, withText])

  // The opened message; the list stays, hidden, with its scroll position.
  const [opened, setOpened] = useState<MessageKey | null>(null)
  const listScroll = useRef(0)
  const open = (m: MessageKey) => {
    listScroll.current = window.scrollY
    setOpened((now) => (now && keyOf(now) === keyOf(m) ? now : keyOnly(m)))
    window.scrollTo(0, 0)
  }
  const back = useCallback(() => {
    setOpened(null)
    requestAnimationFrame(() => window.scrollTo(0, listScroll.current))
  }, [])

  const mark = (keys: MessageKey[], on: boolean) =>
    setBusy((prev) => {
      const next = new Set(prev)
      for (const k of keys) {
        if (on) next.add(keyOf(k))
        else next.delete(keyOf(k))
      }
      return next
    })

  async function run(keys: MessageKey[], what: () => Promise<unknown>) {
    setError(null)
    setNotice(null)
    mark(keys, true)
    try {
      await what()
      setChanged((c) => c + 1)
      setChosen((prev) => {
        const next = new Set(prev)
        for (const k of keys) next.delete(keyOf(k))
        return next
      })
      return true
    } catch (e) {
      setError((e as Error).message)
      return false
    } finally {
      mark(keys, false)
    }
  }

  async function move(ms: MailMessage[], to: ListName, confirmed = false) {
    const keys = ms.map(keyOnly)
    const ok = await run(keys, () => api.sortMessages(keys, to, confirmed))
    setUndo(null)
    // One message moved by hand: ask how to handle mail like it from now on.
    const m = ms[0]
    const rule = m?.sort?.ruleId ? rules.get(m.sort.ruleId) : undefined
    if (ok && !confirmed && ms.length === 1 && m.fromAddr && (to !== "unsorted" || rule)) {
      setOffer({
        message: keyOnly(m),
        subject: m.subject,
        addr: m.fromAddr.toLowerCase(),
        from: m.sort?.list ?? "unsorted",
        to,
        rule,
      })
    } else setOffer(null)
  }

  async function markDone(keys: MessageKey[], done: boolean) {
    const ok = await run(keys, () => api.markDone(keys.map(keyOnly), done))
    if (ok) setUndo(done ? keys.map(keyOnly) : null)
    return ok
  }

  async function remove(keys: MessageKey[]) {
    const ok = await run(keys, async () => {
      const r = await api.deleteMessages(keys.map(keyOnly))
      setNotice(
        r.outcome === "unknown"
          ? "Cockatoo could not confirm what moved to Trash. The next check of your mailbox will show it."
          : `Moved ${r.moved === 1 ? "1 message" : `${r.moved} messages`} to Trash${r.notMoved ? `; ${r.notMoved} could not be moved` : ""}. Gmail keeps Trash for 30 days.`,
      )
    })
    if (ok && opened && keys.some((k) => keyOf(k) === keyOf(opened))) back()
  }

  async function makeRule(from: string, subject: string, to: ListName) {
    setError(null)
    try {
      await api.addRoutine({
        from,
        subject,
        list: to as Exclude<ListName, "unsorted">,
        folderId: 0,
        path: "",
        dated: "",
      })
      setNotice(
        `From now on, mail from ${from}${subject.trim() ? ` with “${subject.trim()}” in the subject` : ""} goes to ${listLabel(to)}. Rules are under Skills & rules.`,
      )
      setChanged((c) => c + 1)
      return true
    } catch (e) {
      setError((e as Error).message)
      return false
    }
  }

  // The rule that put mail in the wrong place was too wide: keep it only for mail with these words in
  // the subject. The sender's other mail goes back to Unsorted, unless another rule takes it.
  async function narrowRule(rule: Routine, subject: string) {
    setError(null)
    try {
      await api.addRoutine({
        from: rule.from,
        subject,
        list: rule.action,
        folderId: rule.folderId,
        path: rule.path,
        dated: rule.dated ?? "",
      })
      await api.removeRoutine(rule.id)
      setNotice(
        `Your rule now puts mail from ${rule.from} in ${listLabel(rule.action)} only with “${subject.trim()}” in the subject.`,
      )
      setChanged((c) => c + 1)
      return true
    } catch (e) {
      setError((e as Error).message)
      return false
    }
  }

  // Unsorted is worked through: the AI suggests a list for each message, you confirm.
  async function suggestAll() {
    if (!view) return
    const todo = view.messages.filter((m) => !m.suggestion && !m.handled)
    if (todo.length === 0) return
    setError(null)
    setNotice(null)
    setSuggesting(true)
    try {
      const r = await api.suggest(todo.map(keyOnly), withText)
      setNotice(
        r.leftOut > 0
          ? `The AI suggested lists for ${r.sent} messages. ${r.leftOut} more are left; ask again for the next ones.`
          : `The AI suggested lists for ${r.sent} ${r.sent === 1 ? "message" : "messages"}. Confirm or change each one.`,
      )
      setChanged((c) => c + 1)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSuggesting(false)
    }
  }

  // The AI's "nothing to do" goes to the Nothing to do list.
  const target = (m: MailMessage): ListName => (m.suggestion!.list === "done" ? "ignore" : m.suggestion!.list)

  async function confirmAll(ms: MailMessage[]) {
    for (const to of ["reply", "attention", "save", "ignore"] as const) {
      const group = ms.filter((m) => target(m) === to)
      if (group.length) await move(group, to, true)
    }
    setOffer(null)
  }

  function confirm(m: MailMessage) {
    if (m.suggestion) move([m], target(m), true)
  }

  const messages = view?.messages ?? []
  const chosenHere = messages.filter((m) => chosen.has(keyOf(m)))
  const allChosen = messages.length > 0 && chosenHere.length === messages.length
  const suggestedHere = messages.filter((m) => m.suggestion && !m.handled)
  const unsuggested = messages.filter((m) => !m.suggestion && !m.handled).length
  const newMail = list === "unsorted" ? [] : (view?.new ?? [])
  const running = sync.status?.running ?? true

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

  const rowProps = (m: MailMessage) => ({
    m,
    account: several && current === "all" ? names.get(m.accountId) : undefined,
    busy: busy.has(keyOf(m)),
    rule: m.sort?.ruleId ? rules.get(m.sort.ruleId) : undefined,
    onOpen: () => open(m),
    onMove: (to: ListName) => move([m], to),
    onConfirm: () => confirm(m),
    onDone: (done: boolean) => markDone([keyOnly(m)], done),
    onDelete: () => remove([keyOnly(m)]),
    onSave: () => setSaving({ messages: [keyOnly(m)], start: m.routine }),
  })

  return (
    <div>
      {opened && (
        <ConversationView
          messageKey={opened}
          refresh={changed}
          onBack={back}
          onSave={(key, routine) => setSaving({ messages: [key], start: routine })}
          onDone={(key, done) => markDone([key], done)}
          onDelete={(key) => remove([key])}
          deleting={busy}
        />
      )}
      <div hidden={opened !== null}>
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="min-w-0">
            <h1 className="text-[2rem] leading-tight font-bold tracking-[-0.025em]">{info.label}</h1>
            <p className="mt-1 text-sm text-muted-foreground">{info.what}</p>
          </div>
          <Button variant="outline" onClick={sync.start} disabled={running}>
            Refresh
          </Button>
        </div>

        <div className="mt-5 flex flex-wrap items-center gap-x-5 gap-y-2">
          <div className="flex gap-x-5" role="group" aria-label="How far back">
            <Button variant="ghost" aria-pressed={!all} className={tabClass(!all)} onClick={() => chooseAll(false)}>
              Last 30 days
            </Button>
            <Button variant="ghost" aria-pressed={all} className={tabClass(all)} onClick={() => chooseAll(true)}>
              All mail
            </Button>
          </div>
          {several && (
            <div className="flex flex-wrap gap-x-5 sm:border-l sm:pl-5" role="group" aria-label="Which account">
              <Button
                variant="ghost"
                aria-pressed={current === "all"}
                className={tabClass(current === "all")}
                onClick={() => chooseAccount("all")}
              >
                All accounts
              </Button>
              {accounts.map((a) => (
                <Button
                  key={a.id}
                  variant="ghost"
                  aria-pressed={current === a.id}
                  className={tabClass(current === a.id)}
                  onClick={() => chooseAccount(a.id)}
                >
                  {a.name}
                </Button>
              ))}
            </div>
          )}
        </div>

        <div className="mt-3">
          <SyncLine status={sync.status} error={sync.error} />
        </div>

        {newMail.length > 0 && (
          <NewMail
            messages={newMail}
            suggesting={suggesting}
            aiReady={aiReady}
            onGoToAI={onGoToAI}
            onAll={() => onGo("unsorted")}
            row={rowProps}
          />
        )}

        {list === "unsorted" && view && messages.length > 0 && (
          <div className="mt-6 flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3">
            <p className="min-w-0 flex-1 text-sm">
              {suggesting ? (
                <Working>Asking Claude where each message belongs…</Working>
              ) : unsuggested > 0 ? (
                aiReady === false ? (
                  "Turn on the AI to have it suggest a list for each message."
                ) : (
                  `The AI can suggest a list for ${unsuggested === 1 ? "this message" : `these ${unsuggested} messages`}: sender, subject, attachment names${withText ? " and first lines" : ""} go to Anthropic with your key.`
                )
              ) : (
                "Every message here has a suggestion. Confirm each one, or all at once."
              )}
            </p>
            {aiReady === false ? (
              <Button variant="outline" onClick={onGoToAI}>
                Turn on AI
              </Button>
            ) : (
              unsuggested > 0 && (
                <Button onClick={suggestAll} disabled={suggesting || aiReady === null}>
                  {suggesting && <Spinner />}
                  Suggest lists
                </Button>
              )
            )}
            {suggestedHere.length > 1 && !suggesting && (
              <Button variant="outline" onClick={() => confirmAll(suggestedHere)}>
                Confirm all {suggestedHere.length}
              </Button>
            )}
          </div>
        )}

        {load.kind === "loading" && <Working className="mt-6">Sorting your mail…</Working>}
        {load.kind === "error" && (
          <p role="alert" className="mt-6 text-sm text-destructive">
            Cockatoo could not read its stored mail: {load.message}
          </p>
        )}

        {view && (
          <div className="mt-8 flex flex-wrap items-baseline justify-between gap-3">
            <h2 className="section-head">
              {info.label}
              <span className="tabular-nums">{view.counts[list]}</span>
              <span className="font-medium normal-case tracking-normal">
                · {all ? "all your mail" : "the last 30 days"}
              </span>
            </h2>
            {view.done > 0 && (
              <button
                type="button"
                aria-pressed={showDone}
                onClick={() => chooseDone(!showDone)}
                className="text-sm text-muted-foreground underline underline-offset-4 hover:text-foreground"
              >
                {showDone ? `Hide ${view.done} done` : `Show ${view.done} done`}
              </button>
            )}
          </div>
        )}

        {error && (
          <p role="alert" className="mt-2 text-sm text-destructive">
            {error}
          </p>
        )}
        {notice && (
          <p role="status" className="mt-2 text-sm">
            {notice}
          </p>
        )}
        {offer && (
          <FuturePanel
            key={keyOf(offer.message) + offer.to}
            {...offer}
            aiReady={aiReady}
            withText={withText}
            onRule={makeRule}
            onNarrow={narrowRule}
            onClose={() => setOffer(null)}
          />
        )}
        {undo && (
          <p role="status" className="mt-2 flex items-center gap-2 text-sm">
            {undo.length === 1 ? "Marked done." : `Marked ${undo.length} done.`}
            <Button size="sm" variant="ghost" onClick={() => markDone(undo, false)}>
              Undo
            </Button>
          </p>
        )}

        {messages.length > 0 && (
          <div className="sticky top-0 z-10 mt-2 flex min-h-12 flex-wrap items-center gap-3 border-b-2 border-foreground/80 bg-card py-2">
            <Checkbox
              aria-label={allChosen ? "Clear the selection" : "Choose all messages shown"}
              checked={allChosen}
              onCheckedChange={(c) => toggleAll(c === true)}
            />
            {chosenHere.length === 0 ? (
              <span className="min-w-0 flex-1 text-sm text-muted-foreground">
                Choose messages to move them together or mark them done
              </span>
            ) : (
              <>
                <span className="text-sm tabular-nums">{chosenHere.length} chosen</span>
                {LISTS.filter((l) => l.id !== list).map((l) => (
                  <Button key={l.id} size="sm" variant="outline" onClick={() => move(chosenHere, l.id)}>
                    <l.icon aria-hidden />
                    {l.label}
                  </Button>
                ))}
                <Button size="sm" variant="outline" onClick={() => markDone(chosenHere.map(keyOnly), true)}>
                  Mark done
                </Button>
                <Button size="sm" variant="outline" onClick={() => remove(chosenHere.map(keyOnly))}>
                  <Trash2 aria-hidden />
                  Delete
                </Button>
                <Button size="sm" variant="ghost" onClick={() => toggleAll(false)}>
                  Clear
                </Button>
              </>
            )}
          </div>
        )}

        {view && messages.length === 0 && load.kind === "ready" && (
          <p className="mt-10 text-center text-sm text-muted-foreground">
            {list === "unsorted"
              ? "Nothing left to sort. Every message is in a list."
              : `Nothing in ${info.label} ${all ? "" : "from the last 30 days "}that you have not dealt with.`}
          </p>
        )}

        {messages.length > 0 && (
          <ul className="divide-y">
            {messages.map((m) => (
              <li key={keyOf(m)}>
                <ListRow
                  {...rowProps(m)}
                  list={list}
                  chosen={chosen.has(keyOf(m))}
                  onChoose={(on) =>
                    setChosen((prev) => {
                      const next = new Set(prev)
                      if (on) next.add(keyOf(m))
                      else next.delete(keyOf(m))
                      return next
                    })
                  }
                />
              </li>
            ))}
          </ul>
        )}
      </div>

      {saving && (
        <SaveDialog
          messages={saving.messages}
          start={saving.start}
          onClose={(saved) => {
            setSaving(null)
            if (saved) setChanged((c) => c + 1)
          }}
        />
      )}
    </div>
  )

  function chooseAll(on: boolean) {
    setAll(on)
    localStorage.setItem(ALL_KEY, String(on))
  }
  function chooseDone(on: boolean) {
    setShowDone(on)
    localStorage.setItem(DONE_KEY, String(on))
  }
  function chooseAccount(which: number | "all") {
    setShown(which)
    localStorage.setItem(SHOWN_KEY, String(which))
  }
}

type RowProps = {
  m: MailMessage
  account?: string
  busy: boolean
  rule?: Routine
  onOpen: () => void
  onMove: (to: ListName) => void
  onConfirm: () => void
  onDone: (done: boolean) => void
  onDelete: () => void
  onSave: () => void
}

// New mail no rule or choice has sorted, at the top of every list: each with the AI's suggestion,
// one click to confirm it, one click for any other list.
function NewMail({
  messages,
  suggesting,
  aiReady,
  onGoToAI,
  onAll,
  row,
}: {
  messages: MailMessage[]
  suggesting: boolean
  aiReady: boolean | null
  onGoToAI: () => void
  onAll: () => void
  row: (m: MailMessage) => RowProps
}) {
  const shown = messages.slice(0, 3)
  return (
    <section aria-labelledby="new-heading" className="mt-8">
      <div className="flex flex-wrap items-baseline justify-between gap-3">
        <h2 id="new-heading" className="text-lg font-bold tracking-[-0.01em]">
          Just in, not sorted <span className="font-medium text-muted-foreground tabular-nums">{messages.length}</span>
        </h2>
        {messages.length > shown.length && (
          <Button variant="ghost" size="sm" onClick={onAll}>
            All {messages.length} in Unsorted
          </Button>
        )}
      </div>
      {suggesting && <Working className="mt-1">Asking Claude where the new mail belongs…</Working>}
      {aiReady === false && (
        <p className="mt-1 text-sm text-muted-foreground">
          Put each one in a list.{" "}
          <button type="button" onClick={onGoToAI} className="underline underline-offset-4 hover:text-foreground">
            Turn on the AI
          </button>{" "}
          to have it suggest one.
        </p>
      )}
      <ul className="mt-2 divide-y border-y">
        {shown.map((m) => (
          <li key={keyOf(m)}>
            <ListRow {...row(m)} list="unsorted" compact />
          </li>
        ))}
      </ul>
    </section>
  )
}

// One message: who, what, where it sits and why, and every action on the row.
function ListRow({
  m,
  list,
  account,
  busy,
  rule,
  chosen,
  onChoose,
  compact = false,
  onOpen,
  onMove,
  onConfirm,
  onDone,
  onDelete,
  onSave,
}: RowProps & { list: ListName; chosen?: boolean; onChoose?: (on: boolean) => void; compact?: boolean }) {
  const sender = m.fromName || m.fromAddr || "(unknown sender)"
  const h = m.handled
  const g = !h && list === "unsorted" ? m.suggestion : undefined
  const [asking, setAsking] = useState(false) // "Not here": where does it go?
  return (
    <div
      aria-busy={busy}
      className={cn(
        "flex flex-col gap-x-4 gap-y-2 px-2 py-3.5 transition-opacity duration-150 md:flex-row md:flex-wrap md:items-start",
        chosen && "bg-muted",
        busy && "opacity-50",
      )}
    >
      <div className="flex min-w-0 flex-1 items-baseline gap-3">
        {onChoose && (
          <Checkbox
            aria-label={`Choose: ${m.subject || "(no subject)"} from ${sender}`}
            checked={chosen}
            onCheckedChange={(c) => onChoose(c === true)}
            className="translate-y-[3px]"
          />
        )}
        {h ? (
          <PenTick className="-mx-1 translate-y-[3px]" />
        ) : (
          <span
            aria-hidden
            className={cn("size-2 shrink-0 translate-y-[-1px] rounded-full", m.seen ? "bg-transparent" : "bg-primary")}
          />
        )}
        <div className="min-w-0 flex-1 cursor-pointer" onClick={onOpen}>
          <div className="flex items-baseline justify-between gap-3">
            <p
              className={cn("truncate text-[0.9375rem]", m.seen || h ? "text-muted-foreground" : "font-semibold")}
              title={m.fromAddr}
            >
              {!m.seen && <span className="sr-only">Unread. </span>}
              {sender}
            </p>
            <time dateTime={m.receivedAt} className="shrink-0 text-xs text-muted-foreground tabular-nums">
              {when(m.receivedAt)}
            </time>
          </div>
          <OpenButton
            subject={m.subject}
            mark={h ? "struck" : g && g.list !== "done" ? "now" : undefined}
            className={m.seen || h ? "text-muted-foreground" : "text-foreground"}
          />
          {g && (
            <p className="pen mt-1 text-lg">
              {listLabel(g.list)}: {g.reason}
              {g.skill && (
                <span className="ml-1.5 font-sans text-xs text-muted-foreground">· your skill “{g.skill}”</span>
              )}
            </p>
          )}
          {!compact && <PlacedLine m={m} rule={rule} />}
          {account && <p className="mt-0.5 text-xs text-muted-foreground">{account}</p>}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-1.5 pl-7 md:shrink-0 md:justify-end md:pl-0">
        {g && (
          <Button size="sm" onClick={onConfirm} disabled={busy}>
            {listLabel(g.list)}
          </Button>
        )}
        {list !== "unsorted" && !h && (
          <Button size="sm" variant="ghost" onClick={() => setAsking(!asking)} aria-expanded={asking} disabled={busy}>
            <X aria-hidden />
            Not here
          </Button>
        )}
        <div className={cn("flex gap-1", list !== "unsorted" && "hidden")} role="group" aria-label="Put it in a list">
          {LISTS.filter((l) => l.id !== "unsorted").map((l) => {
            const here = m.sort?.list === l.id
            if (g && listLabel(g.list) === l.label) return null
            return (
              <Button
                key={l.id}
                size="sm"
                variant="ghost"
                aria-pressed={here}
                title={here ? `In ${l.label}` : `Move to ${l.label}`}
                aria-label={here ? `In ${l.label}` : `Move to ${l.label}`}
                disabled={busy}
                onClick={() => (here ? undefined : onMove(l.id))}
                className={cn(here ? "bg-secondary text-foreground" : "font-medium text-muted-foreground")}
              >
                <l.icon aria-hidden />
                <span className="hidden lg:inline">{l.label}</span>
              </Button>
            )
          })}
        </div>
        {!h && list === "reply" && (
          <Button size="sm" variant="outline" onClick={onOpen}>
            Reply
          </Button>
        )}
        {!h && list === "save" && (
          <Button size="sm" variant={m.routine && !m.routine.missing ? "default" : "outline"} onClick={onSave}>
            Save
          </Button>
        )}
        {h?.doneAt ? (
          <Button size="sm" variant="ghost" onClick={() => onDone(false)}>
            Not done
          </Button>
        ) : (
          !h && (
            <Button size="sm" variant="outline" onClick={() => onDone(true)} disabled={busy}>
              Done
            </Button>
          )
        )}
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label={`Delete: ${m.subject || "(no subject)"}`}
          title="Move to Trash"
          disabled={busy}
          onClick={onDelete}
        >
          {busy ? <Spinner /> : <Trash2 aria-hidden />}
        </Button>
      </div>
      {asking && (
        <div
          className="flex basis-full flex-wrap items-center gap-1.5 pl-7 text-sm md:pl-9"
          role="group"
          aria-label="Where it goes"
        >
          <span className="mr-1 font-medium">Not {listLabel(list)}. Where does it go?</span>
          {LISTS.filter((l) => l.id !== list).map((l) => (
            <Button
              key={l.id}
              size="sm"
              variant="outline"
              onClick={() => {
                setAsking(false)
                onMove(l.id)
              }}
            >
              <l.icon aria-hidden />
              {l.id === "unsorted" ? "Not sure" : l.label}
            </Button>
          ))}
          <Button size="sm" variant="ghost" onClick={() => setAsking(false)}>
            Cancel
          </Button>
        </div>
      )}
    </div>
  )
}

// How the message got into its list, or what you did about it.
function PlacedLine({ m, rule }: { m: MailMessage; rule?: Routine }) {
  const h = m.handled
  if (h) {
    const parts: string[] = []
    if (h.savedFiles)
      parts.push(`Saved ${h.savedFiles} ${h.savedFiles === 1 ? "file" : "files"} to ${(h.savedTo ?? []).join(", ")}`)
    if (h.replied) parts.push("Replied")
    if (h.doneAt) parts.push("Done")
    return <p className="mt-1 text-xs font-medium text-done">{parts.join(" · ")}</p>
  }
  const by = m.sort?.by
  if (!by) return null
  const text =
    by === "rule"
      ? `Your rule: mail from ${rule?.from ?? "this sender"}${rule?.subject ? ` about “${rule.subject}”` : ""}`
      : by === "ai"
        ? "You took the AI's suggestion"
        : "You put it here"
  return <p className="mt-1 text-xs text-muted-foreground">{text}</p>
}

// After a move by hand: how to handle mail like it from now on, in two parts. A rule: for the sender
// (or anyone at the domain), with words in the subject if you like; or, when a wide rule of yours put
// it in the wrong list, that rule made narrower. And, with the AI on, what the agent proposes to
// remember in your skills (SCOPE.md, section 7a): shown in full, editable, saved only on your click.
function FuturePanel({
  message,
  subject,
  addr,
  from,
  to,
  rule,
  aiReady,
  withText,
  onRule,
  onNarrow,
  onClose,
}: {
  message: MessageKey
  subject: string
  addr: string
  from: ListName
  to: ListName
  rule?: Routine
  aiReady: boolean | null
  withText: boolean
  onRule: (from: string, subject: string, to: ListName) => Promise<boolean>
  onNarrow: (rule: Routine, subject: string) => Promise<boolean>
  onClose: () => void
}) {
  const [words, setWords] = useState("")
  const [narrow, setNarrow] = useState("")
  const [ruleDone, setRuleDone] = useState(false)
  const domain = domainOf(addr)
  const wide = rule && !rule.subject && rule.action === from
  const offersRule = to !== "unsorted" || wide

  type Teach =
    | { kind: "off" }
    | { kind: "asking" }
    | { kind: "error"; message: string }
    | { kind: "proposed"; p: Proposal; text: string }
    | { kind: "saving"; p: Proposal; text: string }
    | { kind: "saved"; name: string }
    | { kind: "declined" }
  const [teach, setTeach] = useState<Teach>({ kind: aiReady ? "asking" : "off" })
  useEffect(() => {
    if (!aiReady) return
    let live = true
    api
      .teach({ message, from, to, ruleId: rule?.id ?? 0, withText })
      .then((p) => live && setTeach({ kind: "proposed", p, text: p.instructions ?? "" }))
      .catch((e: Error) => live && setTeach({ kind: "error", message: e.message }))
    return () => {
      live = false
    }
  }, [aiReady, message, from, to, rule, withText])

  async function remember(p: Proposal, text: string) {
    setTeach({ kind: "saving", p, text })
    try {
      const k = await api.saveSkill({
        id: p.action === "edit" ? (p.skillId ?? 0) : 0,
        name: p.name ?? "",
        applies: p.applies ?? "",
        instructions: text,
        note: p.summary,
        fromAgent: true,
      })
      setTeach({ kind: "saved", name: k.name })
    } catch (e) {
      setTeach({ kind: "error", message: (e as Error).message })
    }
  }

  const teachOpen = teach.kind === "asking" || teach.kind === "proposed" || teach.kind === "saving"
  return (
    <div role="status" className="mt-2 space-y-3 rounded-lg border px-4 py-3 text-sm">
      <div className="flex items-start justify-between gap-3">
        <p>
          <span className="font-semibold">
            Moved “{subject || "(no subject)"}” to {listLabel(to)}.
          </span>{" "}
          {to !== "unsorted" ? "What about mail like it from now on?" : "Cockatoo will leave it for you to sort."}
        </p>
        <Button size="sm" variant="ghost" onClick={onClose}>
          Close
        </Button>
      </div>
      {offersRule && !ruleDone && to !== "unsorted" && (
        <div className="flex flex-wrap items-center gap-2">
          <span className="section-head mr-1">A rule</span>
          <Input
            aria-label="Only when the subject has these words (optional)"
            placeholder="only with these words in the subject"
            value={words}
            onChange={(e) => setWords(e.target.value)}
            className="h-8 w-64"
          />
          <Button size="sm" variant="outline" onClick={async () => setRuleDone(await onRule(addr, words, to))}>
            Always from {addr}
          </Button>
          {domainAllowed(addr) && (
            <Button size="sm" variant="outline" onClick={async () => setRuleDone(await onRule(domain, words, to))}>
              Anyone at {domain}
            </Button>
          )}
          <Button size="sm" variant="ghost" onClick={() => setRuleDone(true)}>
            No rule
          </Button>
        </div>
      )}
      {wide && rule && !ruleDone && (
        <div className="flex flex-wrap items-center gap-2 border-t pt-3">
          <span>
            It was in {listLabel(from)} because of your rule for all mail from {rule.from}. Keep that rule only for mail
            with
          </span>
          <Input
            aria-label="Words the subject must have"
            placeholder="invoice"
            value={narrow}
            onChange={(e) => setNarrow(e.target.value)}
            className="h-8 w-40"
          />
          <span>in the subject.</span>
          <Button
            size="sm"
            variant="outline"
            disabled={!narrow.trim()}
            onClick={async () => setRuleDone(await onNarrow(rule, narrow))}
          >
            Narrow the rule
          </Button>
        </div>
      )}

      {teach.kind !== "off" && teach.kind !== "declined" && (
        <div className="space-y-2 border-t pt-3">
          <p className="section-head">Teach your agent</p>
          {teach.kind === "asking" && <Working>Your agent is working out what to remember…</Working>}
          {teach.kind === "error" && (
            <p role="alert" className="text-destructive">
              {teach.message}
            </p>
          )}
          {teach.kind === "saved" && (
            <p>
              <PenTick className="mr-1 inline size-3.5 align-[-2px]" />
              Remembered in your skill “{teach.name}”. It is under Skills &amp; rules.
            </p>
          )}
          {(teach.kind === "proposed" || teach.kind === "saving") &&
            (teach.p.action === "none" ? (
              <p className="text-muted-foreground">{teach.p.summary}</p>
            ) : (
              <>
                <p className="pen text-xl">{teach.p.summary}</p>
                <p className="text-muted-foreground">
                  {teach.p.action === "edit"
                    ? `It changes your skill “${teach.p.name}” to read:`
                    : `A new skill, “${teach.p.name}”${teach.p.applies ? ` (for ${teach.p.applies.replace(/\.$/, "").toLowerCase()})` : ""}:`}
                </p>
                <textarea
                  aria-label="What the skill will say"
                  value={teach.text}
                  rows={3}
                  disabled={teach.kind === "saving"}
                  onChange={(e) => setTeach({ ...teach, text: e.target.value })}
                  className="block w-full resize-y rounded-md border border-input bg-card px-3 py-2 text-[0.9375rem] outline-none focus-visible:border-pen focus-visible:ring-3 focus-visible:ring-ring/40 dark:bg-transparent"
                />
                <div className="flex flex-wrap items-center gap-2">
                  <Button
                    size="sm"
                    onClick={() => remember(teach.p, teach.text)}
                    disabled={teach.kind === "saving" || !teach.text.trim()}
                  >
                    {teach.kind === "saving" && <Spinner />}
                    Remember
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setTeach({ kind: "declined" })}>
                    Don't
                  </Button>
                  <span className="text-xs text-muted-foreground">
                    Written by the AI from this email. Read it before you approve; change it as you like.
                  </span>
                </div>
              </>
            ))}
        </div>
      )}
      {!teachOpen && (ruleDone || !offersRule) && teach.kind !== "error" && (
        <div className="flex justify-end">
          <Button size="sm" variant="outline" onClick={onClose}>
            Done
          </Button>
        </div>
      )}
    </div>
  )
}
