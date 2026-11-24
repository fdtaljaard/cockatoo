import { useEffect, useRef, useState } from "react"
import { ArrowLeft, ExternalLink, Paperclip, Repeat, Trash2 } from "lucide-react"

import { api, type Conversation, type ConversationMessage, type MessageKey, type RoutineMatch } from "@/api"
import { ReplyBox } from "@/components/reply-box"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { PenTick } from "@/components/marks"
import { Working } from "@/components/working"
import { matchWhere } from "@/lib/routine"
import { cn } from "@/lib/utils"

type State = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; c: Conversation }

// An opened message and its conversation, as a chat: what each person wrote, oldest first, your
// own messages on the right. Quotes, signatures and pictures are left out; "Show all" on a
// message shows everything it said, as text. Read with a peek: opening marks nothing as read.
export function ConversationView({
  messageKey,
  refresh,
  onBack,
  onSave,
  onDone,
  onDelete,
  deleting,
}: {
  messageKey: MessageKey
  refresh: number // changes after a save or a mark, to show it
  onBack: () => void
  onSave: (key: MessageKey, routine?: RoutineMatch) => void
  onDone: (key: MessageKey, done: boolean) => void
  onDelete: (key: MessageKey) => void
  deleting: Set<string> // messages on their way to Trash, by key
}) {
  const [state, setState] = useState<State>({ kind: "loading" })
  // Replies sent from here, shown at once: the Sent folder brings them in at the next check.
  const [sent, setSent] = useState<ConversationMessage[]>([])
  const opened = useRef<HTMLLIElement>(null)
  const scrolled = useRef(false)

  useEffect(() => {
    let live = true
    api.conversation(messageKey).then(
      (c) => live && setState({ kind: "ready", c }),
      (e: Error) => live && setState({ kind: "error", message: e.message }),
    )
    return () => {
      live = false
    }
  }, [messageKey, refresh])

  // The opened message is brought into view once, when the conversation first shows.
  useEffect(() => {
    if (state.kind === "ready" && !scrolled.current && opened.current) {
      scrolled.current = true
      opened.current.scrollIntoView({ block: "center" })
    }
  }, [state])

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !document.querySelector("[role=dialog]")) onBack()
    }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [onBack])

  const c = state.kind === "ready" ? state.c : null
  const people = c ? new Set(c.messages.filter((m) => !m.mine).map((m) => m.fromAddr)).size : 0

  return (
    <div className="mx-auto max-w-3xl">
      <Button variant="ghost" className="-ml-3" onClick={onBack}>
        <ArrowLeft aria-hidden />
        Inbox
      </Button>

      {state.kind === "loading" && <Working className="mt-6">Reading the conversation from your mailbox…</Working>}
      {state.kind === "error" && (
        <p role="alert" className="mt-6 text-sm text-destructive">
          Cockatoo could not open this message: {state.message}
        </p>
      )}

      {c && (
        <>
          <div className="mt-2 flex flex-wrap items-end justify-between gap-x-4 gap-y-2">
            <div className="min-w-0">
              <h1 className="text-[1.6rem] leading-tight font-bold tracking-[-0.02em] text-balance">
                {c.subject || "(no subject)"}
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">
                {c.messages.length} {c.messages.length === 1 ? "message" : "messages"}
                {people > 1 && ` · ${people} people`}
              </p>
            </div>
            {c.messageId && (
              <a
                href={`https://mail.google.com/mail/u/?authuser=${encodeURIComponent(c.account)}#search/rfc822msgid%3A${encodeURIComponent(c.messageId)}`}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1.5 text-sm text-muted-foreground underline-offset-4 hover:text-foreground hover:underline"
              >
                Open in Gmail
                <ExternalLink aria-hidden className="size-3.5" />
              </a>
            )}
          </div>

          <ul className="mt-3 space-y-1 text-xs text-muted-foreground">
            {c.older > 0 && (
              <li>
                {c.older} older {c.older === 1 ? "message is" : "messages are"} left out.
              </li>
            )}
            {c.sentPartial && <li>Your Sent folder is not fully read yet, so some of your replies may be missing.</li>}
            <li>
              Opening does not mark mail as read. Pictures, signatures and quoted mail are left out; links show as text.
            </li>
          </ul>

          <ol className="mt-6 space-y-5">
            {c.messages.map((m, i) => (
              <Bubble
                key={i}
                m={m}
                ref={m.opened ? opened : undefined}
                onSave={() => m.key && onSave(m.key, m.routine)}
                onDone={(done) => m.key && onDone(m.key, done)}
                onDelete={() => m.key && onDelete(m.key)}
                deleting={!!m.key && deleting.has(`${m.key.accountId}-${m.key.uidValidity}-${m.key.uid}`)}
              />
            ))}
            {sent.map((m, i) => (
              <Bubble key={`sent-${i}`} m={m} onSave={() => {}} onDone={() => {}} onDelete={() => {}} />
            ))}
          </ol>

          {c.reply && (
            <ReplyBox
              reply={c.reply}
              onSent={(text, r) =>
                setSent((now) => [
                  ...now,
                  {
                    folder: "sent",
                    mine: true,
                    opened: false,
                    fromName: "",
                    fromAddr: c.account,
                    at: r.at,
                    seen: true,
                    text,
                    full: text,
                    cut: false,
                    attachments: [],
                    problem: undefined,
                  },
                ])
              }
            />
          )}
        </>
      )}
    </div>
  )
}

function Bubble({
  m,
  ref,
  onSave,
  onDone,
  onDelete,
  deleting = false,
}: {
  m: ConversationMessage
  ref?: React.Ref<HTMLLIElement>
  onSave: () => void
  onDone: (done: boolean) => void
  onDelete: () => void
  deleting?: boolean
}) {
  const [all, setAll] = useState(false)
  const more = m.full !== "" && m.full !== m.text
  const who = m.mine ? "You" : m.fromName || m.fromAddr || "(unknown sender)"
  const h = m.handled
  return (
    <li ref={ref} className={cn("flex flex-col gap-1", m.mine ? "items-end" : "items-start")}>
      <p className="px-1 text-xs text-muted-foreground">
        <span className={cn(!m.mine && "font-medium text-foreground")} title={m.fromAddr}>
          {who}
        </span>
        {" · "}
        <time dateTime={m.at}>{when(m.at)}</time>
        {m.key && !m.seen && " · unread"}
        {m.opened && (
          <span className="pen ml-2 inline-flex items-center gap-1 text-base">
            <ArrowLeft aria-hidden className="size-3.5" strokeWidth={2.5} />
            you opened this one
          </span>
        )}
      </p>
      <div
        className={cn(
          "max-w-[88%] rounded-lg px-4 py-3 text-[0.9375rem] leading-relaxed",
          m.mine ? "bg-[color-mix(in_oklch,var(--pen)_16%,var(--card))]" : "bg-muted",
        )}
      >
        {m.problem ? (
          <p className="italic">{m.problem}</p>
        ) : (
          <div className="break-words whitespace-pre-wrap">{linkify(all ? m.full : m.text) || <i>(no text)</i>}</div>
        )}
        {m.cut && !m.problem && (
          <p className="mt-2 text-xs opacity-70">This message is long; the start of it is shown.</p>
        )}
        {more && !m.problem && (
          <button
            type="button"
            onClick={() => setAll(!all)}
            className="mt-2 text-xs underline underline-offset-4 opacity-80 hover:opacity-100"
          >
            {all ? "Show less" : "Show all"}
          </button>
        )}
        {m.attachments.length > 0 && (
          <ul className="mt-3 flex flex-wrap gap-1.5">
            {m.attachments.map((a, j) => (
              <li
                key={j}
                className={cn(
                  "inline-flex max-w-full items-center gap-1.5 rounded-md border px-2 py-1 text-xs",
                  "border-border bg-card",
                )}
              >
                <Paperclip aria-hidden className="size-3.5 shrink-0" />
                <span className="truncate">{a.name}</span>
                <span className="shrink-0 tabular-nums opacity-70">{size(a.size)}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
      {m.key && (
        <div className="flex flex-wrap items-center gap-2 px-1">
          {h && (
            <p className="flex items-center gap-1.5 text-xs">
              <PenTick className="size-3.5" />
              {h.savedFiles ? `Saved to ${(h.savedTo ?? []).join(", ")}` : h.doneAt ? "Done" : "Replied"}
            </p>
          )}
          {!h && m.routine && !m.routine.missing && (
            <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <Repeat aria-hidden className="size-3.5" />
              Routine: save to {matchWhere(m.routine)}
            </p>
          )}
          {m.attachments.length > 0 && !h?.savedFiles && (
            <Button size="sm" variant={m.routine ? "default" : "outline"} onClick={onSave}>
              Save attachments
            </Button>
          )}
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
          <Button size="sm" variant="ghost" onClick={onDelete} disabled={deleting}>
            {deleting ? <Spinner /> : <Trash2 aria-hidden />}
            {deleting ? "Moving to Trash…" : "Delete"}
          </Button>
        </div>
      )}
    </li>
  )
}

// Web addresses in the text become links that open in the browser; nothing else does.
function linkify(text: string): React.ReactNode[] {
  return text.split(/(https?:\/\/[^\s<>"')\]]+)/g).map((part, i) =>
    i % 2 === 1 ? (
      <a key={i} href={part} target="_blank" rel="noreferrer noopener" className="underline underline-offset-2">
        {part}
      </a>
    ) : (
      part
    ),
  )
}

function size(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

// "Today 14:05", "Mon 23 Nov 09:12", "23 Nov 2025".
function when(iso: string): string {
  const d = new Date(iso)
  const today = new Date()
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  if (d.toDateString() === today.toDateString()) return `Today ${time}`
  if (d.getFullYear() === today.getFullYear())
    return `${d.toLocaleDateString([], { weekday: "short", day: "numeric", month: "short" })} ${time}`
  return d.toLocaleDateString([], { day: "numeric", month: "short", year: "numeric" })
}
