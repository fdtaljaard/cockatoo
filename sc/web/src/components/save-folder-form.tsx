import { useState, type FormEvent } from "react"

import { api, type SaveFolder } from "@/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

// Adds a save folder: a name, and the path of a folder that already exists on this computer.
// Used on the Folders screen and inside the save dialog when none is set up yet.
export function SaveFolderForm({ onAdded, compact = false }: { onAdded: (f: SaveFolder) => void; compact?: boolean }) {
  const [name, setName] = useState("")
  const [path, setPath] = useState("")
  const [state, setState] = useState<{ kind: "idle" } | { kind: "busy" } | { kind: "error"; message: string }>({ kind: "idle" })

  async function add(e: FormEvent) {
    e.preventDefault()
    setState({ kind: "busy" })
    try {
      const f = await api.addSaveFolder(name, path)
      setName("")
      setPath("")
      setState({ kind: "idle" })
      onAdded(f)
    } catch (err) {
      setState({ kind: "error", message: (err as Error).message })
    }
  }

  const busy = state.kind === "busy"
  return (
    <form onSubmit={add} className="space-y-3">
      <div className={compact ? "space-y-3" : "grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]"}>
        <div className="space-y-1.5">
          <Label htmlFor="folder-name">Name</Label>
          <Input id="folder-name" placeholder="Expenses" value={name} onChange={(e) => setName(e.target.value)} disabled={busy} required />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="folder-path">Path</Label>
          <Input
            id="folder-path"
            placeholder="C:\Users\you\Documents\Expenses"
            value={path}
            onChange={(e) => setPath(e.target.value)}
            disabled={busy}
            required
            aria-describedby="folder-path-help"
          />
        </div>
      </div>
      <p id="folder-path-help" className="text-xs text-muted-foreground">
        A folder that already exists. In File Explorer, right-click it and choose Copy as path, then paste it here.
      </p>
      {state.kind === "error" && (
        <p role="alert" className="text-sm text-destructive">
          {state.message}
        </p>
      )}
      <Button type="submit" variant="outline" disabled={busy}>
        {busy ? "Adding…" : "Add folder"}
      </Button>
    </form>
  )
}
