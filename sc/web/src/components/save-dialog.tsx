import { useEffect, useState, type FormEvent } from "react"
import { ChevronRight, Folder, FolderPlus } from "lucide-react"

import { api, type FolderBrowse, type MessageKey, type SaveFolder, type SaveResult } from "@/api"
import { SaveFolderForm } from "@/components/save-folder-form"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"

type Step =
  | { kind: "choose" }
  | { kind: "browse"; at: FolderBrowse }
  | { kind: "saving"; at: FolderBrowse }
  | { kind: "done"; result: SaveResult }

const pad = (n: number) => String(n).padStart(2, "0")

// Saves the attachments of chosen messages: pick a save folder, go into or make a subfolder (the
// month and the day are one click), save. Works for any list that can hand it message keys.
export function SaveDialog({ messages, onClose }: { messages: MessageKey[]; onClose: (saved: boolean) => void }) {
  const [folders, setFolders] = useState<SaveFolder[] | null>(null)
  const [step, setStep] = useState<Step>({ kind: "choose" })
  const [error, setError] = useState<string | null>(null)
  const [newName, setNewName] = useState("")

  useEffect(() => {
    api.saveFolders().then(setFolders, (e: Error) => setError(e.message))
  }, [])

  async function go(folderId: number, path: string) {
    setError(null)
    try {
      setStep({ kind: "browse", at: await api.browseFolder(folderId, path) })
    } catch (e) {
      setError((e as Error).message)
    }
  }

  async function make(at: FolderBrowse, name: string, e?: FormEvent) {
    e?.preventDefault()
    setError(null)
    try {
      setStep({ kind: "browse", at: await api.makeSubfolder(at.folderId, at.path, name) })
      setNewName("")
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function save(at: FolderBrowse) {
    setError(null)
    setStep({ kind: "saving", at })
    try {
      setStep({ kind: "done", result: await api.saveAttachments(at.folderId, at.path, messages) })
    } catch (e) {
      setError((e as Error).message)
      setStep({ kind: "browse", at })
    }
  }

  // Today's month and day, the quick names for a new folder: worked out once, when the dialog opens.
  const [{ month, day }] = useState(() => {
    const now = new Date()
    const m = `${now.getFullYear()}-${pad(now.getMonth() + 1)}`
    return { month: m, day: `${m}-${pad(now.getDate())}` }
  })
  const count = `${messages.length} ${messages.length === 1 ? "message" : "messages"}`

  return (
    <Dialog open onOpenChange={(open) => !open && step.kind !== "saving" && onClose(step.kind === "done")}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{step.kind === "done" ? "Saved" : "Save attachments"}</DialogTitle>
          <DialogDescription>
            {step.kind === "done" ? step.result.folder : `The attachments of ${count}. Nothing is marked as read.`}
          </DialogDescription>
        </DialogHeader>

        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}

        {step.kind === "choose" && (
          <div className="space-y-4">
            {folders === null && !error && <p className="text-sm text-muted-foreground">Opening…</p>}
            {folders && folders.length > 0 && (
              <ul className="divide-y rounded-lg border">
                {folders.map((f) => (
                  <li key={f.id}>
                    <button
                      type="button"
                      onClick={() => go(f.id, "")}
                      disabled={f.missing}
                      className="flex w-full items-center gap-3 px-3 py-2.5 text-left hover:bg-muted/50 disabled:opacity-50"
                    >
                      <Folder aria-hidden className="size-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">{f.name}</span>
                        <span className="block truncate text-xs text-muted-foreground">
                          {f.missing ? "No longer there" : f.path}
                        </span>
                      </span>
                      <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {folders && folders.length === 0 && (
              <div className="space-y-3">
                <p className="text-sm text-muted-foreground">Add a folder to save attachments to.</p>
                <SaveFolderForm compact onAdded={(f) => setFolders([f])} />
              </div>
            )}
          </div>
        )}

        {(step.kind === "browse" || step.kind === "saving") && (
          <div className="space-y-4">
            <nav aria-label="Where" className="flex flex-wrap items-center gap-1 text-sm">
              <button type="button" className="font-medium underline-offset-4 hover:underline" onClick={() => setStep({ kind: "choose" })}>
                Folders
              </button>
              <ChevronRight aria-hidden className="size-3.5 text-muted-foreground" />
              <button type="button" className="underline-offset-4 hover:underline" onClick={() => go(step.at.folderId, "")}>
                {step.at.name}
              </button>
              {step.at.path &&
                step.at.path.split("/").map((seg, i, all) => (
                  <span key={i} className="flex items-center gap-1">
                    <ChevronRight aria-hidden className="size-3.5 text-muted-foreground" />
                    <button
                      type="button"
                      className="underline-offset-4 hover:underline"
                      onClick={() => go(step.at.folderId, all.slice(0, i + 1).join("/"))}
                    >
                      {seg}
                    </button>
                  </span>
                ))}
            </nav>

            {step.at.folders.length > 0 ? (
              <ul className="max-h-56 divide-y overflow-y-auto rounded-lg border">
                {step.at.folders.map((name) => (
                  <li key={name}>
                    <button
                      type="button"
                      onClick={() => go(step.at.folderId, step.at.path ? `${step.at.path}/${name}` : name)}
                      className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-muted/50"
                    >
                      <Folder aria-hidden className="size-4 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">{name}</span>
                      <ChevronRight aria-hidden className="size-4 shrink-0 text-muted-foreground" />
                    </button>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground">No folders inside this one.</p>
            )}

            <form onSubmit={(e) => make(step.at, newName, e)} className="space-y-2">
              <p className="text-xs text-muted-foreground">New folder here</p>
              <div className="flex flex-wrap gap-2">
                <Button type="button" size="sm" variant="outline" onClick={() => make(step.at, month)}>
                  <FolderPlus aria-hidden />
                  {month}
                </Button>
                <Button type="button" size="sm" variant="outline" onClick={() => make(step.at, day)}>
                  <FolderPlus aria-hidden />
                  {day}
                </Button>
              </div>
              <div className="flex gap-2">
                <Input
                  aria-label="New folder name"
                  placeholder="Or type a name"
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  className="h-7"
                />
                <Button type="submit" size="sm" variant="outline" disabled={!newName.trim()}>
                  Make
                </Button>
              </div>
            </form>
          </div>
        )}

        {step.kind === "done" && <SaveSummary result={step.result} />}

        <DialogFooter>
          {(step.kind === "browse" || step.kind === "saving") && (
            <>
              <Button variant="ghost" onClick={() => onClose(false)} disabled={step.kind === "saving"}>
                Cancel
              </Button>
              <Button onClick={() => save(step.at)} disabled={step.kind === "saving"}>
                {step.kind === "saving" ? "Saving…" : `Save to ${step.at.path ? step.at.path.split("/").pop() : step.at.name}`}
              </Button>
            </>
          )}
          {step.kind === "choose" && (
            <Button variant="ghost" onClick={() => onClose(false)}>
              Cancel
            </Button>
          )}
          {step.kind === "done" && <Button onClick={() => onClose(true)}>Done</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// What was saved, and what was not and why, per message.
function SaveSummary({ result }: { result: SaveResult }) {
  const name = (p: string) => p.split(/[\\/]/).pop()
  return (
    <div className="space-y-3 text-sm">
      <p>
        {result.files === 0
          ? "No files were saved."
          : `${result.files} ${result.files === 1 ? "file" : "files"} saved.`}
      </p>
      <ul className="max-h-64 space-y-2 overflow-y-auto">
        {result.messages.map((m, i) => (
          <li key={i} className="rounded-lg border px-3 py-2">
            <p className="truncate font-medium">{m.subject || "(no subject)"}</p>
            {m.files.map((f) => (
              <p key={f.path} className="truncate text-muted-foreground" title={f.path}>
                {name(f.path)}
              </p>
            ))}
            {m.problem && <p className="text-muted-foreground">{m.problem}</p>}
            {m.skipped?.map((s, j) => (
              <p key={j} className="text-xs text-muted-foreground">
                Not saved: {s}
              </p>
            ))}
          </li>
        ))}
      </ul>
    </div>
  )
}
