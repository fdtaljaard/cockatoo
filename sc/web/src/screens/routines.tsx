import { useCallback, useEffect, useState } from "react"

import { api, type ListName, type Routine } from "@/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { SkillsSection } from "@/components/skills"
import { LISTS } from "@/lib/lists"
import { routineMail, routineWhere } from "@/lib/routine"
import { cn } from "@/lib/utils"

type RuleList = Exclude<ListName, "unsorted">
const RULE_LISTS = LISTS.filter((l) => l.id !== "unsorted")

// Rules: which mail goes to which list, by sender address or domain and, if given, words in the
// subject (SCOPE.md, section 8a). Most are made on a row after moving a message; here they are read,
// changed, removed, or added by hand. A Save rule made from the save dialog also says where the
// attachments go.
export function RoutinesScreen() {
  const [rules, setRules] = useState<Routine[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<number | null>(null)
  const [from, setFrom] = useState("")
  const [subject, setSubject] = useState("")
  const [list, setList] = useState<RuleList>("reply")
  const [adding, setAdding] = useState(false)

  const load = useCallback(() => {
    api.routines().then(setRules, (e: Error) => setError(e.message))
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

  // A rule's list is changed by making the rule again for the other list: it replaces the old one.
  async function save(r: { from: string; subject?: string }, to: RuleList) {
    setError(null)
    try {
      await api.addRoutine({ from: r.from, subject: r.subject ?? "", list: to, folderId: 0, path: "", dated: "" })
      load()
      return true
    } catch (e) {
      setError((e as Error).message)
      return false
    }
  }

  async function add(e: React.FormEvent) {
    e.preventDefault()
    setAdding(true)
    if (await save({ from, subject }, list)) {
      setFrom("")
      setSubject("")
    }
    setAdding(false)
  }

  return (
    <div className="max-w-3xl">
      <h1 className="text-[2rem] leading-tight font-bold tracking-[-0.025em]">Skills &amp; rules</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Your email agent sorts mail by your skills; rules are exact and come first. Correcting a message teaches both.
      </p>

      <div className="mt-8">
        <SkillsSection />
      </div>

      <h2 className="mt-12 border-t pt-8 text-lg font-bold tracking-[-0.01em]">Rules</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        Which mail goes to which list. A rule for an address beats one for its domain, and one with words in the subject
        beats one without, so a narrow rule can keep some of a sender's mail out of a list. Moving a message by hand
        always wins. Each time you move a message, cockatoo asks how to handle mail like it from now on.
      </p>

      {error && (
        <p role="alert" className="mt-4 text-sm text-destructive">
          {error}
        </p>
      )}

      {rules &&
        RULE_LISTS.map((l) => {
          const mine = rules.filter((r) => r.action === l.id)
          return (
            <section key={l.id} aria-labelledby={`rules-${l.id}`} className="mt-8">
              <h2 id={`rules-${l.id}`} className="section-head">
                <l.icon aria-hidden className="size-3.5" />
                {l.label}
                <span className="tabular-nums">{mine.length}</span>
              </h2>
              {mine.length === 0 ? (
                <p className="mt-2 text-sm text-muted-foreground">No rules for {l.label} yet.</p>
              ) : (
                <ul className="mt-2 divide-y border-y">
                  {mine.map((r) => (
                    <li key={r.id} className="py-3">
                      <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
                        <div className="min-w-0">
                          <p className="font-medium">{routineMail(r)}</p>
                          {r.action === "save" && r.folderId > 0 && (
                            <p className="text-sm text-muted-foreground">
                              Its attachments go to {routineWhere(r.folderName ?? "", r.path, r.dated)}
                            </p>
                          )}
                          {r.missing && (
                            <p className="text-sm text-destructive">
                              Its save folder is gone. Save one of these mails again to choose a new one.
                            </p>
                          )}
                        </div>
                        {confirming !== r.id && (
                          <div className="flex flex-wrap gap-1" role="group" aria-label="Move this rule to a list">
                            {RULE_LISTS.filter((o) => o.id !== r.action).map((o) => (
                              <Button key={o.id} size="sm" variant="ghost" onClick={() => save(r, o.id as RuleList)}>
                                <o.icon aria-hidden />
                                {o.label}
                              </Button>
                            ))}
                            <Button size="sm" variant="ghost" onClick={() => setConfirming(r.id)}>
                              Remove
                            </Button>
                          </div>
                        )}
                      </div>
                      {confirming === r.id && (
                        <div className="mt-3 flex flex-wrap items-center gap-2 text-sm">
                          <span>Remove this rule? Its mail goes back to Unsorted unless you moved it yourself.</span>
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
            </section>
          )
        })}

      <section aria-labelledby="add-rule" className="mt-10 border-t pt-8">
        <h2 id="add-rule" className="section-head">
          Add a rule
        </h2>
        <form onSubmit={add} className="mt-3 space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="rule-from">From</Label>
              <Input
                id="rule-from"
                placeholder="billing@acme.com or acme.com"
                value={from}
                onChange={(e) => setFrom(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="rule-subject">With in the subject (optional)</Label>
              <Input
                id="rule-subject"
                placeholder="invoice"
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
              />
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-sm">Goes to</span>
            <div className="flex gap-1" role="group" aria-label="Which list">
              {RULE_LISTS.map((o) => (
                <Button
                  key={o.id}
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-pressed={list === o.id}
                  onClick={() => setList(o.id as RuleList)}
                  className={cn(list === o.id ? "bg-secondary text-foreground" : "font-medium text-muted-foreground")}
                >
                  <o.icon aria-hidden />
                  {o.label}
                </Button>
              ))}
            </div>
            <Button type="submit" className="ml-auto" disabled={adding || !from.trim()}>
              Add rule
            </Button>
          </div>
        </form>
      </section>
    </div>
  )
}
