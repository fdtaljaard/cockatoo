import { useCallback, useEffect, useRef, useState } from "react"
import { Repeat, Search, Trash2, X } from "lucide-react"

import {
  api,
  type Account,
  type Handled,
  type InboxList,
  type MailMessage,
  type MessageKey,
  type RoutineMatch,
  type SearchResult,
} from "@/api"
import { Button } from "@/components/ui/button"
import { ConversationView } from "@/components/conversation"
import { SaveDialog } from "@/components/save-dialog"
import { SyncLine } from "@/components/sync-line"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { AllClear, PenTick } from "@/components/marks"
import { Spinner } from "@/components/ui/spinner"
import { Working } from "@/components/working"
import { TriagePanel } from "@/components/triage-panel"
import { useSync } from "@/hooks/use-sync"
import { when } from "@/lib/format"
import { matchWhere } from "@/lib/routine"
import { cn, tabClass as tab } from "@/lib/utils"

type Load = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; list: InboxList }
type Found = { q: string; result: SearchResult } | { q: string; error: string }

// Which account the Inbox shows, and whether mail you have dealt with is shown, kept between
// visits on this computer.
const SHOWN_KEY = "cockatoo.inbox.account"
const SHOW_DONE_KEY = "cockatoo.inbox.showDone"

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
  // The save dialog, for the chosen messages or for one message a routine is waiting on.
  const [saving, setSaving] = useState<{ messages: MessageKey[]; start?: RoutineMatch; chosen: boolean } | null>(null)
  // Bumped after you mark or save something, so the list and the AI's picks read it again.
  const [changed, setChanged] = useState(0)
  const [actionError, setActionError] = useState<string | null>(null)
  // Search: while there is text in the box, the list is what it found in the whole inbox.
  const [query, setQuery] = useState("")
  const [found, setFound] = useState<Found | null>(null)
  // Mail you have dealt with leaves the list; this brings it back. Search always shows it.
  const [showDone, setShowDone] = useState(() => localStorage.getItem(SHOW_DONE_KEY) === "true")
  // The messages just marked done, to take it back.
  const [undo, setUndo] = useState<MessageKey[] | null>(null)

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
  const q = query.trim()
  const searching = q !== ""
  const result = found && found.q === q && "result" in found ? found.result : null
  const searchError = found && found.q === q && "error" in found ? found.error : null
  const fetched = searching
    ? (result?.messages ?? [])
    : list
      ? list.messages.filter((m) => current === "all" || m.accountId === current)
      : []
  const hideDone = !searching && !showDone
  const messages = hideDone ? fetched.filter((m) => !m.handled) : fetched
  const unread = fetched.filter((m) => !m.seen).length
  const dealtWith = fetched.filter((m) => m.handled).length
  const waiting = (list?.waiting ?? []).filter((m) => current === "all" || m.accountId === current)
  const saveFor = (m: MailMessage) => setSaving({ messages: [keyOnly(m)], start: m.routine, chosen: false })
  // The opened message. The list stays, hidden, and its scroll position comes back with it.
  const [opened, setOpened] = useState<MessageKey | null>(null)
  const listScroll = useRef(0)
  const open = (m: MessageKey) => {
    listScroll.current = window.scrollY
    // A click on the subject also reaches the row: open once, not twice.
    setOpened((now) => (now && keyOf(now) === keyOf(m) ? now : keyOnly(m)))
    window.scrollTo(0, 0)
  }
  const back = useCallback(() => {
    setOpened(null)
    requestAnimationFrame(() => window.scrollTo(0, listScroll.current))
  }, [])
  const unreadOf = (id: number) => list?.messages.filter((m) => m.accountId === id && !m.seen).length ?? 0
  const running = sync.status?.running ?? true
  const chosenHere = messages.filter((m) => chosen.has(keyOf(m)))
  const allChosen = messages.length > 0 && chosenHere.length === messages.length

  // Searched a moment after typing stops, and again when marks change or new mail is read.
  useEffect(() => {
    if (!q) return
    let live = true
    const timer = setTimeout(() => {
      api.searchInbox(q, current).then(
        (r) => live && setFound({ q, result: r }),
        (e: Error) => live && setFound({ q, error: e.message }),
      )
    }, 250)
    return () => {
      live = false
      clearTimeout(timer)
    }
  }, [q, current, changed, sync.tick])

  function toggle(m: MailMessage, on: boolean) {
    setChosen((prev) => {
      const next = new Set(prev)
      if (on) next.add(keyOf(m))
      else next.delete(keyOf(m))
      return next
    })
  }

  function chooseShowDone(on: boolean) {
    setShowDone(on)
    localStorage.setItem(SHOW_DONE_KEY, String(on))
  }

  // Moves messages to Gmail's Trash. An opened message that goes leaves the conversation.
  const [notice, setNotice] = useState<string | null>(null)
  // The messages the AI picked above, so their rows in the list wear the same highlighter.
  const [picked, setPicked] = useState<Map<string, "now" | "today">>(() => new Map())

  // Messages on their way to Trash, and what the bar above the list is doing, for the spinners.
  const [deleting, setDeleting] = useState<Set<string>>(() => new Set())
  const [barBusy, setBarBusy] = useState<"done" | "delete" | null>(null)
  const markDeleting = (keys: MessageKey[], on: boolean) =>
    setDeleting((prev) => {
      const next = new Set(prev)
      for (const k of keys) {
        if (on) next.add(keyOf(k))
        else next.delete(keyOf(k))
      }
      return next
    })

  async function deleteMessages(keys: MessageKey[]) {
    setActionError(null)
    setNotice(null)
    setUndo(null)
    markDeleting(keys, true)
    try {
      const r = await api.deleteMessages(keys.map(keyOnly))
      setChanged((c) => c + 1)
      const n = r.moved
      setNotice(
        r.outcome === "unknown"
          ? "Cockatoo could not confirm what moved to Trash. The next check of your mailbox will show it."
          : `Moved ${n === 1 ? "1 message" : `${n} messages`} to Trash${r.notMoved ? `; ${r.notMoved} could not be moved` : ""}. Gmail keeps Trash for 30 days.`,
      )
      if (opened && keys.some((k) => keyOf(k) === keyOf(opened))) back()
      setChosen((prev) => {
        const next = new Set(prev)
        for (const k of keys) next.delete(keyOf(k))
        return next
      })
      return true
    } catch (e) {
      setActionError((e as Error).message)
      return false
    } finally {
      markDeleting(keys, false)
    }
  }

  async function markDone(keys: MessageKey[], done: boolean) {
    setActionError(null)
    setNotice(null)
    try {
      await api.markDone(keys.map(keyOnly), done)
      setChanged((c) => c + 1)
      setUndo(done ? keys.map(keyOnly) : null)
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
      {opened && (
        <ConversationView
          messageKey={opened}
          refresh={changed}
          onBack={back}
          onSave={(key, routine) => setSaving({ messages: [key], start: routine, chosen: false })}
          onDone={(key, done) => markDone([key], done)}
          onDelete={(key) => deleteMessages([key])}
          deleting={deleting}
        />
      )}
      <div hidden={opened !== null}>
        <div className="flex items-end justify-between gap-4">
          <div>
            <h1 className="text-[2rem] leading-tight font-bold tracking-[-0.025em]">Inbox</h1>
            <p className="mt-1 text-sm text-muted-foreground" aria-live="polite">
              {searching ? (
                result ? (
                  result.more ? (
                    `The newest ${messages.length} found · add a word to narrow it`
                  ) : messages.length === 0 ? (
                    "Nothing found"
                  ) : (
                    `${messages.length} found in your whole inbox`
                  )
                ) : (
                  "Searching…"
                )
              ) : (
                <>
                  Last 24 hours
                  {fetched.length > 0 && ` · ${unread} unread`}
                  {dealtWith > 0 && (
                    <>
                      {" · "}
                      <button
                        type="button"
                        aria-pressed={showDone}
                        onClick={() => chooseShowDone(!showDone)}
                        className="underline underline-offset-4 hover:text-foreground"
                      >
                        {showDone ? `Hide ${dealtWith} done` : `Show ${dealtWith} done`}
                      </button>
                    </>
                  )}
                </>
              )}
            </p>
          </div>
          <Button variant="outline" onClick={sync.start} disabled={running}>
            Refresh
          </Button>
        </div>

        {severalAccounts && (
          <div className="mt-5 flex flex-wrap gap-x-5 gap-y-1" role="group" aria-label="Which account">
            <Button
              variant="ghost"
              aria-pressed={current === "all"}
              onClick={() => show("all")}
              className={tab(current === "all")}
            >
              All
              {list && <Count n={list.messages.filter((m) => !m.seen).length} />}
            </Button>
            {accounts.map((a) => (
              <Button
                key={a.id}
                variant="ghost"
                aria-pressed={current === a.id}
                onClick={() => show(a.id)}
                className={tab(current === a.id)}
              >
                {a.name}
                {list && <Count n={unreadOf(a.id)} />}
              </Button>
            ))}
          </div>
        )}

        <div className="relative mt-5">
          {searching && !result && !searchError ? (
            <Spinner className="pointer-events-none absolute top-1/2 left-0 -translate-y-1/2 text-muted-foreground" />
          ) : (
            <Search
              aria-hidden
              className="pointer-events-none absolute top-1/2 left-0 size-4 -translate-y-1/2 text-muted-foreground"
            />
          )}
          <Input
            type="search"
            aria-label="Search your inbox by sender or subject"
            placeholder="Search sender or subject in your whole inbox"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => e.key === "Escape" && setQuery("")}
            className="h-11 rounded-none border-0 border-b-2 border-foreground/70 bg-transparent pr-9 pl-7 text-[0.9375rem] focus-visible:border-pen focus-visible:ring-0 dark:bg-transparent [&::-webkit-search-cancel-button]:hidden"
          />
          {query && (
            <button
              type="button"
              aria-label="Clear the search"
              onClick={() => setQuery("")}
              className="absolute top-1/2 right-1.5 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground"
            >
              <X aria-hidden className="size-4" />
            </button>
          )}
        </div>

        <div className="mt-3">
          <SyncLine status={sync.status} error={sync.error} />
        </div>

        {!searching && waiting.length > 0 && (
          <Waiting
            messages={waiting}
            showAccount={severalAccounts && current === "all"}
            names={names}
            onSave={saveFor}
            onOpen={open}
            onDone={(m) => markDone([m], true)}
          />
        )}

        {/* Hidden, not removed, while searching: an answer the AI is working on keeps going. */}
        <div hidden={searching}>
          <TriagePanel
            scope={current}
            accountNames={names}
            showAccount={severalAccounts && current === "all"}
            syncRunning={running}
            onGoToAI={onGoToAI}
            refresh={changed}
            onDone={(m) => markDone([m], true)}
            onOpen={open}
            onPicks={setPicked}
          />
        </div>

        {searchError && (
          <p role="alert" className="mt-6 text-sm text-destructive">
            The search did not work: {searchError}
          </p>
        )}
        {searching && result && messages.length === 0 && (
          <p className="mt-10 text-center text-sm text-muted-foreground">
            Nothing in your inbox has “{q}” in its sender or subject. Message text is not searched.
          </p>
        )}

        {load.kind === "loading" && <Working className="mt-6">Reading your inbox…</Working>}
        {load.kind === "error" && (
          <p role="alert" className="mt-6 text-sm text-destructive">
            Cockatoo could not read its stored mail: {load.message}
          </p>
        )}

        {!searching && list && fetched.length > 0 && messages.length === 0 && (
          <AllClear note="that's everything that matters">
            {current === "all"
              ? "Everything from the last 24 hours is done."
              : `Everything from the last 24 hours for ${names.get(current)} is done.`}
          </AllClear>
        )}

        {!searching && list && fetched.length === 0 && (
          <AllClear note={running ? undefined : "a quiet day"}>
            {running
              ? "Checking for mail…"
              : current === "all"
                ? "No mail in the last 24 hours."
                : `No mail in the last 24 hours for ${names.get(current)}.`}
          </AllClear>
        )}

        {messages.length > 0 && (
          <div className="sticky top-0 z-10 mt-6 flex min-h-12 flex-wrap items-center gap-3 border-b-2 border-foreground/80 bg-card py-2">
            <Checkbox
              aria-label={allChosen ? "Clear the selection" : "Choose all messages shown"}
              checked={allChosen}
              onCheckedChange={(c) => toggleAll(c === true)}
            />
            {chosenHere.length === 0 ? (
              <span className="min-w-0 flex-1 text-sm text-muted-foreground">
                Choose messages to save their attachments or mark them done
              </span>
            ) : (
              <>
                <span className="text-sm tabular-nums">{chosenHere.length} chosen</span>
                <Button size="sm" onClick={() => setSaving({ messages: chosenHere.map(keyOnly), chosen: true })}>
                  Save attachments
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={barBusy !== null}
                  onClick={async () => {
                    setBarBusy("done")
                    if (await markDone(chosenHere, true)) toggleAll(false)
                    setBarBusy(null)
                  }}
                >
                  {barBusy === "done" && <Spinner />}
                  Mark done
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={barBusy !== null}
                  onClick={async () => {
                    setBarBusy("delete")
                    await deleteMessages(chosenHere)
                    setBarBusy(null)
                  }}
                >
                  {barBusy === "delete" ? <Spinner /> : <Trash2 aria-hidden />}
                  {barBusy === "delete" ? "Moving to Trash…" : "Delete"}
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

        {notice && (
          <p role="status" className="mt-2 text-sm">
            {notice}
          </p>
        )}

        {undo && hideDone && (
          <p role="status" className="mt-2 flex items-center gap-2 text-sm">
            {undo.length === 1 ? "Marked done." : `Marked ${undo.length} done.`}
            <Button size="sm" variant="ghost" onClick={() => markDone(undo, false)}>
              Undo
            </Button>
          </p>
        )}

        {messages.length > 0 && (
          <ul className="divide-y">
            {messages.map((m) => (
              <Row
                key={keyOf(m)}
                message={m}
                account={severalAccounts && current === "all" ? names.get(m.accountId) : undefined}
                chosen={chosen.has(keyOf(m))}
                onChoose={(on) => toggle(m, on)}
                onDone={(done) => markDone([m], done)}
                onDelete={() => deleteMessages([m])}
                deleting={deleting.has(keyOf(m))}
                onSave={() => saveFor(m)}
                onOpen={() => open(m)}
                pick={picked.get(keyOf(m))}
              />
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
            if (saved) {
              if (saving.chosen) toggleAll(false)
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
  onDelete,
  deleting,
  onSave,
  onOpen,
  pick,
}: {
  message: MailMessage
  account?: string
  pick?: "now" | "today" // the AI picked it above: its subject wears the same highlighter
  chosen: boolean
  onChoose: (on: boolean) => void
  onDone: (done: boolean) => void
  onDelete: () => void
  deleting: boolean // on its way to Trash
  onSave: () => void
  onOpen: () => void
}) {
  const sender = m.fromName || m.fromAddr || "(unknown sender)"
  const h = m.handled
  return (
    <li
      aria-busy={deleting}
      className={cn(
        "flex items-baseline gap-3 px-2 py-3.5 transition-[opacity,background-color] duration-150",
        chosen && "bg-muted",
        deleting && "opacity-50",
      )}
    >
      <Checkbox
        aria-label={`Choose: ${m.subject || "(no subject)"} from ${sender}`}
        checked={chosen}
        onCheckedChange={(c) => onChoose(c === true)}
        className="translate-y-[3px]"
      />
      {h ? (
        <PenTick className="-mx-1 translate-y-[3px]" />
      ) : (
        <span
          aria-hidden
          className={cn("size-2 shrink-0 translate-y-[-1px] rounded-full", m.seen ? "bg-transparent" : "bg-primary")}
        />
      )}
      {/* The whole message area opens it; the subject is the button for the keyboard. */}
      <div className="min-w-0 flex-1 cursor-pointer" onClick={onOpen}>
        <div className="flex flex-col sm:flex-row sm:items-baseline sm:justify-between sm:gap-3">
          <p
            className={cn("truncate text-[0.9375rem]", m.seen || h ? "text-muted-foreground" : "font-semibold")}
            title={m.fromAddr}
          >
            {!m.seen && <span className="sr-only">Unread. </span>}
            {sender}
          </p>
          <time dateTime={m.receivedAt} className="shrink-0 text-xs tabular-nums text-muted-foreground">
            {when(m.receivedAt)}
          </time>
        </div>
        <OpenButton
          subject={m.subject}
          mark={h ? "struck" : pick}
          className={m.seen || h ? "text-muted-foreground" : "text-foreground"}
        />
        {h && <HandledLine h={h} />}
        {!h && m.routine && <RoutineLine r={m.routine} />}
        {account && <p className="mt-0.5 text-xs text-muted-foreground">{account}</p>}
      </div>
      {/* One width for every row, so the times line up whether or not a row has a button. */}
      <div className="flex w-28 shrink-0 items-center justify-end gap-1 self-center">
        {!h && m.routine && !m.routine.missing && (
          <Button size="sm" onClick={onSave}>
            Save
          </Button>
        )}
        {!h && (!m.routine || m.routine.missing) && (
          <Button size="sm" variant="outline" onClick={() => onDone(true)}>
            Done
          </Button>
        )}
        {h?.doneAt && (
          <Button size="sm" variant="ghost" onClick={() => onDone(false)}>
            Not done
          </Button>
        )}
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label={`Delete: ${m.subject || "(no subject)"}`}
          title={deleting ? "Moving to Trash…" : "Move to Trash"}
          disabled={deleting}
          onClick={onDelete}
        >
          {deleting ? <Spinner /> : <Trash2 aria-hidden />}
        </Button>
      </div>
    </li>
  )
}

// A message's subject, as the button that opens it. A subject the AI picked wears the highlighter;
// one you have dealt with is struck through in pen.
export function OpenButton({
  subject,
  mark,
  className,
}: {
  subject: string
  mark?: "now" | "today" | "struck"
  className?: string
}) {
  const text = subject || <span className="italic">(no subject)</span>
  return (
    <button
      type="button"
      aria-label={`Open: ${subject || "(no subject)"}`}
      className={cn("block w-full truncate text-left text-sm underline-offset-4 hover:underline", className)}
    >
      {mark === "struck" ? (
        <span className="struck">{text}</span>
      ) : mark ? (
        <span className={cn("hl", mark === "today" && "soft")}>{text}</span>
      ) : (
        text
      )}
    </button>
  )
}

// Mail a routine is waiting on, from the last 60 days, read or not: a monthly invoice stays here
// until its attachments are saved (or it is marked done), even when it is long out of the list.
function Waiting({
  messages,
  showAccount,
  names,
  onSave,
  onDone,
  onOpen,
}: {
  messages: MailMessage[]
  showAccount: boolean
  names: Map<number, string>
  onSave: (m: MailMessage) => void
  onDone: (m: MailMessage) => void
  onOpen: (m: MailMessage) => void
}) {
  return (
    <section aria-labelledby="waiting-heading" className="mt-8">
      <h2 id="waiting-heading" className="section-head">
        <Repeat aria-hidden className="size-3.5" />
        Waiting for you
        <span className="tabular-nums">{messages.length}</span>
      </h2>
      <p className="mt-1 text-sm text-muted-foreground">Mail your routines are for, not dealt with yet.</p>
      <ul className="mt-2 divide-y border-y">
        {messages.map((m) => (
          <li key={keyOf(m)} className="flex items-center gap-3 py-2.5">
            <div className="min-w-0 flex-1 cursor-pointer" onClick={() => onOpen(m)}>
              <div className="flex items-baseline justify-between gap-3">
                <p className="truncate text-sm font-semibold" title={m.fromAddr}>
                  {m.fromName || m.fromAddr || "(unknown sender)"}
                </p>
                <time dateTime={m.receivedAt} className="shrink-0 text-xs tabular-nums text-muted-foreground">
                  {new Date(m.receivedAt).toLocaleDateString([], { day: "numeric", month: "short" })}
                </time>
              </div>
              <OpenButton subject={m.subject} />
              {m.routine && <RoutineLine r={m.routine} />}
              {showAccount && <p className="mt-0.5 text-xs text-muted-foreground">{names.get(m.accountId)}</p>}
            </div>
            {m.routine && !m.routine.missing && (
              <Button size="sm" onClick={() => onSave(m)}>
                Save
              </Button>
            )}
            <Button size="sm" variant="outline" onClick={() => onDone(m)}>
              Done
            </Button>
          </li>
        ))}
      </ul>
    </section>
  )
}

// What a routine says to do with a message: "Routine: save to Expenses › 2026-11".
function RoutineLine({ r }: { r: RoutineMatch }) {
  return (
    <p className="mt-1 flex items-start gap-1.5 text-xs text-muted-foreground">
      <Repeat aria-hidden className="size-3.5 shrink-0 translate-y-px" />
      <span className="min-w-0 break-all">
        {r.missing ? "Routine: its save folder is gone. Fix it under Routines." : `Routine: save to ${matchWhere(r)}`}
      </span>
    </p>
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
    <p className="mt-1 flex items-start gap-1.5 text-xs font-medium text-done">
      <span className="min-w-0 break-all">{parts.join(" · ")}</span>
    </p>
  )
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
