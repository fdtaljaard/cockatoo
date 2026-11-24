import { useState } from "react"
import { Send } from "lucide-react"

import { api, type ReplyPreview, type ReplyResult } from "@/api"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"

// The reply at the foot of a conversation: plain text, to the sender of the newest message from
// someone else (or, with Reply all, everyone else it went to). The recipients are shown before
// anything is sent; Send (or Ctrl+Enter) is the only thing that sends.
export function ReplyBox({ reply, onSent }: { reply: ReplyPreview; onSent: (text: string, r: ReplyResult) => void }) {
  const [text, setText] = useState("")
  const [all, setAll] = useState(false)
  const [state, setState] = useState<{ kind: "idle" } | { kind: "sending" } | { kind: "error"; message: string }>({
    kind: "idle",
  })
  const sending = state.kind === "sending"

  async function send() {
    if (!text.trim() || sending) return
    setState({ kind: "sending" })
    try {
      const r = await api.reply({ message: reply.key, text, all })
      onSent(text.trim(), r)
      setText("")
      setState({ kind: "idle" })
    } catch (e) {
      setState({ kind: "error", message: (e as Error).message })
    }
  }

  return (
    <section aria-label="Reply" className="mt-8 rounded-lg border border-input p-3">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-1 text-xs text-muted-foreground">
        <span className="min-w-0 break-all">
          To {reply.to.join(", ")}
          {all && reply.cc.length > 0 && <> · Cc {reply.cc.join(", ")}</>}
        </span>
        {reply.cc.length > 0 && (
          <span className="flex gap-1" role="group" aria-label="Who to reply to">
            <Choice on={!all} onClick={() => setAll(false)}>
              Reply
            </Choice>
            <Choice on={all} onClick={() => setAll(true)}>
              Reply all
            </Choice>
          </span>
        )}
      </div>
      <textarea
        aria-label="Your reply"
        placeholder="Write a reply…"
        value={text}
        rows={4}
        disabled={sending}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
            e.preventDefault()
            send()
          }
        }}
        className="mt-2 block w-full resize-y rounded-none border-0 border-b-2 border-foreground/70 bg-transparent px-1 py-2 text-[0.9375rem] outline-none placeholder:text-muted-foreground focus-visible:border-pen disabled:opacity-50"
      />
      {state.kind === "error" && (
        <p role="alert" className="mt-2 px-1 text-sm text-destructive">
          {state.message}
        </p>
      )}
      <div className="mt-2 flex items-center justify-between gap-3 px-1">
        <p className="text-xs text-muted-foreground">
          Sent from your Gmail, with their message quoted below. Ctrl+Enter sends.
        </p>
        <Button onClick={send} disabled={!text.trim() || sending}>
          {sending ? <Spinner /> : <Send aria-hidden />}
          {sending ? "Sending…" : "Send"}
        </Button>
      </div>
    </section>
  )
}

function Choice({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={cn(
        "rounded-md border px-2 py-0.5",
        on ? "border-foreground bg-secondary font-semibold text-foreground" : "hover:bg-muted",
      )}
    >
      {children}
    </button>
  )
}
