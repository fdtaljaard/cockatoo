import { useCallback, useEffect, useState } from "react"

import { api, type Skill } from "@/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Spinner } from "@/components/ui/spinner"
import { when } from "@/lib/format"

type Draft = { id: number; name: string; applies: string; instructions: string }

// The email agent's skills: what it knows about how you work, in your own words (SCOPE.md, section 7a).
// It reads them every time it sorts or suggests; it learns new ones from your corrections, but only
// with your OK. Every change is kept as a new version.
export function SkillsSection() {
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)
  const [confirming, setConfirming] = useState<number | null>(null)

  const load = useCallback(() => {
    api.skills().then(setSkills, (e: Error) => setError(e.message))
  }, [])
  useEffect(load, [load])

  async function save(d: Draft) {
    setSaving(true)
    setError(null)
    try {
      await api.saveSkill({ ...d, note: d.id ? "edited" : "written", fromAgent: false })
      setEditing(null)
      load()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  async function remove(id: number) {
    try {
      await api.removeSkill(id)
      setConfirming(null)
      load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <section aria-labelledby="skills-heading">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <h2 id="skills-heading" className="text-lg font-bold tracking-[-0.01em]">
          Your agent's skills
        </h2>
        {!editing && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => setEditing({ id: 0, name: "", applies: "", instructions: "" })}
          >
            Add a skill
          </Button>
        )}
      </div>
      <p className="mt-1 max-w-prose text-sm text-muted-foreground">
        What your email agent knows about how you work, in your own words. It reads them every time it sorts or
        suggests, and learns new ones when you correct it, but only with your OK.
      </p>
      {error && (
        <p role="alert" className="mt-3 text-sm text-destructive">
          {error}
        </p>
      )}

      {editing && editing.id === 0 && (
        <SkillForm
          draft={editing}
          saving={saving}
          onChange={setEditing}
          onSave={save}
          onCancel={() => setEditing(null)}
        />
      )}

      {skills && skills.length === 0 && !editing && (
        <p className="mt-4 text-sm text-muted-foreground">
          No skills yet. Correct a message with Not here, and your agent will propose one.
        </p>
      )}

      {skills && skills.length > 0 && (
        <ul className="mt-4 divide-y border-y">
          {skills.map((k) =>
            editing && editing.id === k.id ? (
              <li key={k.id} className="py-3">
                <SkillForm
                  draft={editing}
                  saving={saving}
                  onChange={setEditing}
                  onSave={save}
                  onCancel={() => setEditing(null)}
                />
              </li>
            ) : (
              <li key={k.id} className="py-4">
                <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
                  <div className="min-w-0 max-w-prose">
                    <p className="font-semibold">{k.name}</p>
                    {k.applies && <p className="text-sm text-muted-foreground">For {lower(k.applies)}</p>}
                    <p className="mt-1.5 text-[0.9375rem] leading-relaxed">{k.instructions}</p>
                    <p className="mt-1.5 text-xs text-muted-foreground">
                      {k.madeBy === "cockatoo"
                        ? "A starter: change it to how you work"
                        : k.madeBy === "agent"
                          ? `Learned from your correction, approved by you · ${when(k.updatedAt)}`
                          : `In your words · ${when(k.updatedAt)}`}
                    </p>
                  </div>
                  {confirming !== k.id && (
                    <div className="flex gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() =>
                          setEditing({ id: k.id, name: k.name, applies: k.applies, instructions: k.instructions })
                        }
                      >
                        Edit
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => setConfirming(k.id)}>
                        Remove
                      </Button>
                    </div>
                  )}
                </div>
                {confirming === k.id && (
                  <div className="mt-3 flex flex-wrap items-center gap-2 text-sm">
                    <span>Remove this skill? Your agent stops using it; its history is kept.</span>
                    <Button size="sm" variant="destructive" onClick={() => remove(k.id)}>
                      Remove
                    </Button>
                    <Button size="sm" variant="ghost" onClick={() => setConfirming(null)}>
                      Keep it
                    </Button>
                  </div>
                )}
              </li>
            ),
          )}
        </ul>
      )}
    </section>
  )
}

const lower = (s: string) => s.charAt(0).toLowerCase() + s.slice(1)

function SkillForm({
  draft,
  saving,
  onChange,
  onSave,
  onCancel,
}: {
  draft: Draft
  saving: boolean
  onChange: (d: Draft) => void
  onSave: (d: Draft) => void
  onCancel: () => void
}) {
  const id = `skill-${draft.id}`
  return (
    <form
      className="mt-3 space-y-3 rounded-lg border px-4 py-4"
      onSubmit={(e) => {
        e.preventDefault()
        onSave(draft)
      }}
    >
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-name`}>Name</Label>
          <Input
            id={`${id}-name`}
            placeholder="Shops I buy from"
            value={draft.name}
            onChange={(e) => onChange({ ...draft, name: e.target.value })}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={`${id}-applies`}>For</Label>
          <Input
            id={`${id}-applies`}
            placeholder="Mail from online shops"
            value={draft.applies}
            onChange={(e) => onChange({ ...draft, applies: e.target.value })}
          />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${id}-text`}>What to do, in your words</Label>
        <textarea
          id={`${id}-text`}
          rows={3}
          placeholder="Invoices go to Save. Order confirmations and shipping notices need nothing."
          value={draft.instructions}
          onChange={(e) => onChange({ ...draft, instructions: e.target.value })}
          className="block w-full resize-y rounded-md border border-input bg-card px-3 py-2 text-[0.9375rem] outline-none placeholder:text-muted-foreground focus-visible:border-pen focus-visible:ring-3 focus-visible:ring-ring/40 dark:bg-transparent"
        />
      </div>
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={saving || !draft.name.trim() || !draft.instructions.trim()}>
          {saving && <Spinner />}
          Save skill
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </form>
  )
}
