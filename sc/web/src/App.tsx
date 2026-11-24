import { useCallback, useEffect, useState } from "react"

import { api, useMock, type Account } from "@/api"
import { Button } from "@/components/ui/button"
import { AccountsScreen } from "@/screens/accounts"
import { AIScreen } from "@/screens/ai"
import { CleanupScreen } from "@/screens/cleanup"
import { InboxScreen } from "@/screens/inbox"
import { SetupScreen } from "@/screens/setup"

type Accounts = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; list: Account[] }
type View = "inbox" | "cleanup" | "accounts" | "ai"

export default function App() {
  // Keep one event stream open for as long as the page is shown. The exe stops a few
  // seconds after the last window drops it, so closing the window closes the app.
  useEffect(() => {
    if (useMock) return // no backend to keep alive
    const events = new EventSource("/api/events")
    return () => events.close()
  }, [])

  const [accounts, setAccounts] = useState<Accounts>({ kind: "loading" })
  const [adding, setAdding] = useState(false)
  const [view, setView] = useState<View>("inbox")
  const [version, setVersion] = useState("")

  const load = useCallback(() => {
    api
      .listAccounts()
      .then((list) => setAccounts({ kind: "ready", list }))
      .catch((e: Error) => setAccounts({ kind: "error", message: e.message }))
  }, [])

  useEffect(() => {
    load()
    api.health().then((h) => setVersion(h.version), () => {})
  }, [load])

  return (
    <div className="mx-auto flex min-h-svh max-w-5xl flex-col px-6">
      <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 py-6">
        <span className="text-sm font-semibold tracking-tight">cockatoo</span>
        <nav aria-label="Sections" className="flex flex-wrap gap-1">
          <NavButton current={view === "inbox"} onClick={() => setView("inbox")}>
            Inbox
          </NavButton>
          <NavButton current={view === "cleanup"} onClick={() => setView("cleanup")}>
            Clean up
          </NavButton>
          <NavButton current={view === "accounts"} onClick={() => setView("accounts")}>
            Mail accounts
          </NavButton>
          <NavButton current={view === "ai"} onClick={() => setView("ai")}>
            AI
          </NavButton>
        </nav>
      </header>

      <main className="flex-1 pb-16 pt-6">
        {view === "ai" && <AIScreen />}

        {view !== "ai" && accounts.kind === "loading" && <p className="text-sm text-muted-foreground">Opening…</p>}

        {view !== "ai" && accounts.kind === "error" && (
          <div className="space-y-3">
            <p role="alert" className="rounded-lg border border-destructive/40 px-4 py-3 text-sm text-destructive">
              Cockatoo could not read its accounts: {accounts.message}
            </p>
            <Button variant="outline" onClick={load}>
              Try again
            </Button>
          </div>
        )}

        {/* No account yet: the app opens on the form. */}
        {view !== "ai" && accounts.kind === "ready" && accounts.list.length === 0 && <SetupScreen first onSaved={load} />}

        {view === "cleanup" && accounts.kind === "ready" && accounts.list.length > 0 && (
          <CleanupScreen accounts={accounts.list} />
        )}

        {view === "inbox" && accounts.kind === "ready" && accounts.list.length > 0 && (
          <InboxScreen accounts={accounts.list} />
        )}

        {view === "accounts" && accounts.kind === "ready" && accounts.list.length > 0 && adding && (
          <SetupScreen
            first={false}
            onSaved={() => {
              setAdding(false)
              load()
            }}
            onCancel={() => setAdding(false)}
          />
        )}

        {view === "accounts" && accounts.kind === "ready" && accounts.list.length > 0 && !adding && (
          <AccountsScreen accounts={accounts.list} onAdd={() => setAdding(true)} onRemoved={load} />
        )}
      </main>

      {version && <footer className="py-4 text-xs text-muted-foreground">Build {version}</footer>}
    </div>
  )
}

function NavButton({ current, onClick, children }: { current: boolean; onClick: () => void; children: string }) {
  return (
    <Button variant={current ? "secondary" : "ghost"} aria-current={current ? "page" : undefined} onClick={onClick}>
      {children}
    </Button>
  )
}
