import { useCallback, useEffect, useState } from "react"

import { api, type SaveFolder } from "@/api"
import { SaveFolderForm } from "@/components/save-folder-form"
import { Button } from "@/components/ui/button"

// Save folders: the places on disk attachments are saved to, each a name and a path.
export function FoldersScreen() {
  const [folders, setFolders] = useState<SaveFolder[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [confirming, setConfirming] = useState<number | null>(null)

  const load = useCallback(() => {
    api.saveFolders().then(setFolders, (e: Error) => setError(e.message))
  }, [])
  useEffect(load, [load])

  async function remove(id: number) {
    try {
      await api.removeSaveFolder(id)
      setConfirming(null)
      load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-semibold tracking-tight">Folders</h1>
      <p className="mt-1 text-sm text-muted-foreground">
        Where attachments are saved. Choose messages in your Inbox, then Save attachments.
      </p>

      {error && (
        <p role="alert" className="mt-4 text-sm text-destructive">
          {error}
        </p>
      )}

      {folders && folders.length > 0 && (
        <ul className="mt-6 divide-y rounded-lg border">
          {folders.map((f) => (
            <li key={f.id} className="px-4 py-3">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <p className="truncate font-medium">{f.name}</p>
                  <p className="truncate text-sm text-muted-foreground" title={f.path}>
                    {f.path}
                  </p>
                  {f.missing && (
                    <p className="text-sm text-destructive">This folder is no longer there. Remove it, or make it again.</p>
                  )}
                </div>
                {confirming !== f.id && (
                  <Button variant="ghost" onClick={() => setConfirming(f.id)}>
                    Remove
                  </Button>
                )}
              </div>
              {confirming === f.id && (
                <div className="mt-3 flex flex-wrap items-center gap-2 text-sm">
                  <span>Remove {f.name} from cockatoo? The folder and its files stay on disk.</span>
                  <Button size="sm" variant="destructive" onClick={() => remove(f.id)}>
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
      {folders && folders.length === 0 && (
        <p className="mt-6 text-sm text-muted-foreground">No folders yet. Add one to start saving attachments.</p>
      )}

      <section className="mt-8 border-t pt-6" aria-labelledby="add-folder">
        <h2 id="add-folder" className="mb-3 text-sm font-medium">
          Add a folder
        </h2>
        <SaveFolderForm onAdded={load} />
      </section>
    </div>
  )
}
