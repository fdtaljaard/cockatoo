import { useEffect, useState } from "react"

import { api, type AISettings, type MessageKey, type TriagePick, type TriageResult } from "@/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Label } from "@/components/ui/label"
import { Spinner } from "@/components/ui/spinner"
import { Working } from "@/components/working"
import { cn } from "@/lib/utils"

type State =
  | { kind: "loading" }
  | { kind: "ready"; result: TriageResult | null }
  | { kind: "asking"; result: TriageResult | null }
  | { kind: "error"; message: string; result: TriageResult | null }

const n = (v: number) => v.toLocaleString()

// Whether the first lines of each message go with the question, kept between visits.
const TEXT_KEY = "cockatoo.triage.withText"

// "What needs you now": the AI ranks the unread mail of the last 30 days and says why. The last
// answer shows at once; asking again is the user's click, billed to their key. Only senders,
// subjects and dates are sent, never bodies.
export function TriagePanel({
  scope,
  accountNames,
  showAccount,
  syncRunning,
  onGoToAI,
  refresh,
  onDone,
  onOpen,
  onPicks,
}: {
  scope: "all" | number
  accountNames: Map<number, string>
  showAccount: boolean
  syncRunning: boolean
  onGoToAI: () => void
  refresh: number // changes when you mark or save something: picks you dealt with drop out
  onDone: (m: MessageKey) => Promise<boolean>
  onOpen: (m: MessageKey) => void
  onPicks?: (picks: Map<string, "now" | "today">) => void // which messages the AI picked, for the list below
}) {
  const [state, setState] = useState<State>({ kind: "loading" })
  const [aiReady, setAiReady] = useState<boolean | null>(null)
  const [withText, setWithText] = useState(() => localStorage.getItem(TEXT_KEY) !== "false")
  const key = String(scope)

  function chooseText(on: boolean) {
    setWithText(on)
    localStorage.setItem(TEXT_KEY, String(on))
  }

  useEffect(() => {
    api.aiSettings().then(
      (s: AISettings) => setAiReady(s.enabled && s.hasKey),
      () => setAiReady(false),
    )
  }, [])

  useEffect(() => {
    let live = true
    api
      .latestTriage(key)
      .then((result) => live && setState({ kind: "ready", result }))
      .catch((e: Error) => live && setState({ kind: "error", message: e.message, result: null }))
    return () => {
      live = false
    }
  }, [key, refresh])

  const result = state.kind === "loading" ? null : state.result
  const asking = state.kind === "asking"

  async function ask() {
    setState({ kind: "asking", result })
    try {
      setState({ kind: "ready", result: await api.runTriage(key, withText) })
    } catch (e) {
      setState({ kind: "error", message: (e as Error).message, result })
    }
  }

  useEffect(() => {
    const picks = new Map<string, "now" | "today">()
    for (const p of result?.picks ?? []) {
      const m = p.message
      picks.set(`${m.accountId}-${m.uidValidity}-${m.uid}`, p.priority === "now" ? "now" : "today")
    }
    onPicks?.(picks)
  }, [result, onPicks])

  const now = result?.picks.filter((p) => p.priority === "now") ?? []
  const today = result?.picks.filter((p) => p.priority === "today") ?? []

  return (
    <section aria-labelledby="triage-heading" className="mt-8">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3">
        <div className="contents">
          <h2 id="triage-heading" className="self-center text-lg font-bold tracking-[-0.01em]">
            What needs you now
          </h2>
          <p className="col-span-2 mt-1.5 max-w-prose text-xs leading-relaxed text-muted-foreground">
            Cockatoo first leaves out newsletters, notifications and mail you have already dealt with (replied, saved or
            marked done). The rest of your unread mail from the last 30 days goes to Anthropic with your key: sender,
            subject, date
            {withText ? ", and the first lines of each message (cockatoo does not keep the text)" : ""}.
          </p>
          <div className="col-span-2 mt-2 flex items-center gap-2">
            <Checkbox
              id="triage-text"
              checked={withText}
              onCheckedChange={(c) => chooseText(c === true)}
              disabled={asking}
            />
            <Label htmlFor="triage-text" className="text-xs font-normal">
              Include the first lines of each message
            </Label>
          </div>
        </div>
        {aiReady === false ? (
          <Button variant="outline" onClick={onGoToAI} className="col-start-2 row-start-1">
            Turn on AI
          </Button>
        ) : (
          <Button
            className="col-start-2 row-start-1"
            onClick={ask}
            disabled={asking || syncRunning || aiReady === null}
            title={syncRunning ? "Wait until cockatoo has finished checking your mail." : undefined}
          >
            {asking && <Spinner />}
            {asking ? "Asking…" : result ? "Ask again" : "Ask AI"}
          </Button>
        )}
      </div>

      <div aria-live="polite">
        {aiReady === false && (
          <p className="mt-3 text-sm text-muted-foreground">AI is off. Turn it on and add your Anthropic key first.</p>
        )}
        {asking && (
          <Working className="mt-3">
            {withText ? "Reading the first lines and asking Claude…" : "Asking Claude…"}
          </Working>
        )}
        {state.kind === "error" && (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {state.message}
          </p>
        )}

        {result && !asking && (
          <div className="mt-4">
            <p className="pen text-[1.35rem]">{result.summary}</p>

            {now.length > 0 && (
              <Group
                title="Now"
                picks={now}
                accountNames={accountNames}
                showAccount={showAccount}
                onDone={onDone}
                onOpen={onOpen}
              />
            )}
            {today.length > 0 && (
              <Group
                soft
                title="Today"
                picks={today}
                accountNames={accountNames}
                showAccount={showAccount}
                onDone={onDone}
                onOpen={onOpen}
              />
            )}

            <p className="mt-4 text-xs text-muted-foreground">
              {leftLine(result)}
              {result.id === 0
                ? " Nothing was sent."
                : ` Asked ${new Date(result.askedAt).toLocaleString([], { dateStyle: "medium", timeStyle: "short" })} · ${n(result.sent)} messages sent${result.textSent ? " with their first lines" : ""}${result.leftOut > 0 ? `, ${n(result.leftOut)} older left out` : ""} · ${result.model} · ${n(result.inputTokens)} tokens in, ${n(result.outputTokens)} out`}
              {result.dropped > 0 && ` · ${n(result.dropped)} read or gone since`}
            </p>
            {result.textError && <p className="mt-1 text-xs text-muted-foreground">{result.textError}</p>}
            {result.signalsPartial && (
              <p className="mt-1 text-xs text-muted-foreground">
                Your sent mail is not read yet, so cockatoo could not tell who you write to or what you already
                answered.
              </p>
            )}
            {result.partial && (
              <p className="mt-1 text-xs text-muted-foreground">
                Cockatoo had not finished reading a mailbox, so older unread mail may be missing.
              </p>
            )}
          </div>
        )}
      </div>
    </section>
  )
}

function Group({
  soft = false,
  title,
  picks,
  accountNames,
  showAccount,
  onDone,
  onOpen,
}: {
  soft?: boolean // what can wait until later today gets a lighter stroke
  title: string
  picks: TriagePick[]
  accountNames: Map<number, string>
  showAccount: boolean
  onDone: (m: MessageKey) => Promise<boolean>
  onOpen: (m: MessageKey) => void
}) {
  return (
    <div className="mt-5">
      <h3 className="section-head">{title}</h3>
      <ul className="mt-1 divide-y border-y">
        {picks.map((p) => {
          const m = p.message
          return (
            <li key={`${m.accountId}-${m.uidValidity}-${m.uid}`} className="flex items-center gap-4 py-3.5">
              <div className="min-w-0 flex-1 cursor-pointer" onClick={() => onOpen(m)}>
                <div className="flex items-baseline justify-between gap-3">
                  <p className="truncate text-[0.9375rem] font-semibold" title={m.fromAddr}>
                    {m.fromName || m.fromAddr || "(unknown sender)"}
                  </p>
                  <span
                    className={cn(
                      "shrink-0 text-xs tabular-nums",
                      p.overdue ? "font-semibold text-foreground" : "text-muted-foreground",
                    )}
                  >
                    {ago(m.receivedAt, p.overdue)}
                  </span>
                </div>
                <button
                  type="button"
                  aria-label={`Open: ${m.subject || "(no subject)"}`}
                  className="mt-0.5 block w-full text-left text-[0.9375rem] leading-snug underline-offset-4 hover:underline"
                >
                  <span className={cn("hl", soft && "soft")}>
                    {m.subject || <span className="italic">(no subject)</span>}
                  </span>
                </button>
                <p className="pen mt-1 text-xl">{p.reason}</p>
                {p.signals && <p className="mt-0.5 text-xs text-muted-foreground">{signalLine(p.signals)}</p>}
                {showAccount && <p className="mt-0.5 text-xs text-muted-foreground">{accountNames.get(m.accountId)}</p>}
              </div>
              <div className="flex w-28 shrink-0 justify-end">
                <Button size="sm" variant="outline" onClick={() => onDone(m)}>
                  Done
                </Button>
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}

// "14:05" today; "Unread 3 days" when it has waited.
function ago(iso: string, overdue: boolean): string {
  const d = new Date(iso)
  if (!overdue) return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  const days = Math.max(1, Math.floor((Date.now() - d.getTime()) / 86_400_000))
  return `Unread ${days} ${days === 1 ? "day" : "days"}`
}

// "Left out first: 312 automated or bulk, 4 already dealt with."
function leftLine(r: TriageResult): string {
  const parts = []
  if (r.leftAutomated > 0) parts.push(`${n(r.leftAutomated)} automated or bulk`)
  if (r.leftAnswered > 0) parts.push(`${n(r.leftAnswered)} already dealt with`)
  return parts.length ? `Left out first: ${parts.join(", ")}.` : ""
}

// "Answers your message · To you · You've written to them 14 times".
function signalLine(s: NonNullable<TriagePick["signals"]>): string {
  const parts = []
  if (s.routine) parts.push(`Your routine: ${s.routine}`)
  if (s.repliesToYou) parts.push("Answers your message")
  parts.push(s.toYou ? "To you" : s.ccYou ? "You're on CC" : "Not addressed to you by name")
  parts.push(
    s.wrote > 0
      ? `You've written to them ${n(s.wrote)} ${s.wrote === 1 ? "time" : "times"}`
      : "You've never written to them",
  )
  return parts.join(" · ")
}
