import { useState } from "react"

import { api, type FolderBrowse } from "@/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { datedOf, domainAllowed, domainOf, routineMail, routineWhere } from "@/lib/routine"
import { cn } from "@/lib/utils"

type State = { kind: "idle" } | { kind: "busy" } | { kind: "saved" } | { kind: "error"; message: string }

// Offered after a save: do the same every time mail from this sender comes in. The sender can be
// the whole domain (the default, except for gmail.com and the like) or the one address; the
// subject text is optional and starts empty, since a subject such as "Your invoice for November"
// would never match next month's.
export function RoutineOffer({ at, from }: { at: FolderBrowse; from: string }) {
  const domainOk = domainAllowed(from)
  const [byDomain, setByDomain] = useState(domainOk)
  const [subject, setSubject] = useState("")
  const [state, setState] = useState<State>({ kind: "idle" })
  const { parent, dated } = datedOf(at.path)
  const match = byDomain ? domainOf(from) : from

  async function remember() {
    setState({ kind: "busy" })
    try {
      await api.addRoutine({ from: match, subject, folderId: at.folderId, path: parent, dated })
      setState({ kind: "saved" })
    } catch (e) {
      setState({ kind: "error", message: (e as Error).message })
    }
  }

  if (state.kind === "saved") {
    return (
      <p className="rounded-lg border px-3 py-2 text-sm">
        Remembered. {routineMail({ from: match, subject: subject.trim().toLowerCase() })} will be pointed out in your
        Inbox. See it under Routines.
      </p>
    )
  }

  return (
    <div className="space-y-2 rounded-lg border px-3 py-2.5 text-sm">
      <p className="font-medium">Do this every time?</p>
      <div className="flex flex-wrap items-center gap-1.5" role="group" aria-label="Which mail">
        <span className="text-muted-foreground">Mail from</span>
        {domainOk && (
          <Choice on={byDomain} onClick={() => setByDomain(true)}>
            {domainOf(from)}
          </Choice>
        )}
        <Choice on={!byDomain} onClick={() => setByDomain(false)}>
          {from}
        </Choice>
      </div>
      <Input
        aria-label="Only when the subject contains (optional)"
        placeholder="Only when the subject contains… (optional)"
        value={subject}
        onChange={(e) => setSubject(e.target.value)}
        className="h-7"
      />
      <p className="text-muted-foreground">
        Saves to {routineWhere(at.name, parent, dated)}. Cockatoo points it out; you press Save.
      </p>
      {state.kind === "error" && (
        <p role="alert" className="text-destructive">
          {state.message}
        </p>
      )}
      <Button size="sm" variant="outline" onClick={remember} disabled={state.kind === "busy"}>
        {state.kind === "busy" ? "Remembering…" : "Remember"}
      </Button>
    </div>
  )
}

function Choice({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={cn(
        "rounded-md border px-2 py-0.5 text-sm",
        on ? "border-foreground bg-secondary font-medium" : "text-muted-foreground hover:bg-muted/50",
      )}
    >
      {children}
    </button>
  )
}
