import { useCallback, useEffect, useState } from "react"

import { api, type Routine } from "@/api"
import { Button } from "@/components/ui/button"
import { routineMail, routineWhere } from "@/lib/routine"

// Routines: what you do every time a certain mail comes in. Made from the save dialog; seen and
// removed here.
export function RoutinesScreen() {
  const [routines, setRoutines] = useState<Routine[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<number | null>(null)

  const load = useCallback(() => {
    api.routines().then(setRoutines, (e: Error) => setError(e.message))
  }, [])
  useEffect(load, [load])

  async function remove(id: number) {
    try {
      await api.removeRoutine(id)
      setConfirming(null)
      load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <div className="max-w-2xl">
      <h1 className="text-[2rem] leading-tight font-bold tracking-[-0.025em]">Routines</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        What you do every time a certain mail comes in. Cockatoo points that mail out in your Inbox, and tells the AI;
        you press Save.
      </p>

      {error && (
        <p role="alert" className="mt-4 text-sm text-destructive">
          {error}
        </p>
      )}

      {routines && routines.length > 0 && (
        <ul className="mt-6 divide-y border-y">
          {routines.map((r) => (
            <li key={r.id} className="px-4 py-3">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <p className="font-medium">{routineMail(r)}</p>
                  <p className="text-sm text-muted-foreground">
                    Save its attachments to {routineWhere(r.folderName ?? "", r.path, r.dated)}
                  </p>
                  {r.missing && (
                    <p className="text-sm text-destructive">
                      Its save folder is gone. Remove this routine, then save one of these mails again to make a new
                      one.
                    </p>
                  )}
                </div>
                {confirming !== r.id && (
                  <Button variant="ghost" onClick={() => setConfirming(r.id)}>
                    Remove
                  </Button>
                )}
              </div>
              {confirming === r.id && (
                <div className="mt-3 flex flex-wrap items-center gap-2 text-sm">
                  <span>Remove this routine? Files already saved stay where they are.</span>
                  <Button size="sm" variant="destructive" onClick={() => remove(r.id)}>
                    Remove
                  </Button>
                  <Button size="sm" variant="ghost" onClick={() => setConfirming(null)}>
                    Keep it
                  </Button>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      {routines && routines.length === 0 && (
        <p className="mt-6 text-sm text-muted-foreground">
          No routines yet. Save the attachments of a mail you get every month, such as an invoice, and choose Remember
          under “Do this every time?”.
        </p>
      )}
    </div>
  )
}
