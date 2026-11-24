import { useCallback, useEffect, useState } from "react"

import { api, useMock, type Account } from "@/api"
import { ActivityBar } from "@/components/activity-bar"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { AccountsScreen } from "@/screens/accounts"
import { AIScreen } from "@/screens/ai"
import { CleanupScreen } from "@/screens/cleanup"
import { FoldersScreen } from "@/screens/folders"
import { RoutinesScreen } from "@/screens/routines"
import { InboxScreen } from "@/screens/inbox"
import { SetupScreen } from "@/screens/setup"
import { Working } from "@/components/working"
import mark from "@/assets/cockatoo-mark.png"

type Accounts = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; list: Account[] }
type View = "inbox" | "cleanup" | "folders" | "routines" | "accounts" | "ai"

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
    api.health().then(
      (h) => setVersion(h.version),
      () => {},
    )
  }, [load])

  // A page of mail reads best narrow; Clean up's tables need the width.
  const width = view === "cleanup" ? "max-w-[76rem]" : "max-w-[60rem]"

  return (
    <div className="flex min-h-svh flex-col">
      <ActivityBar />
      <header className="border-b bg-card">
        <div className={cn("mx-auto flex items-center justify-between gap-x-3 px-4 py-3 sm:gap-x-6 sm:px-6", width)}>
          <span className="flex shrink-0 items-center gap-2.5 text-lg font-bold tracking-[-0.02em]">
            <img src={mark} alt="cockatoo" className="h-8 w-auto" />
            <span aria-hidden>
              cockatoo
            </span>
          </span>
          <nav aria-label="Sections" className="flex min-w-0 gap-0.5 overflow-x-auto">
            <NavButton current={view === "inbox"} onClick={() => setView("inbox")}>
              Inbox
            </NavButton>
            <NavButton current={view === "cleanup"} onClick={() => setView("cleanup")}>
              Clean up
            </NavButton>
            <NavButton current={view === "folders"} onClick={() => setView("folders")}>
              Folders
            </NavButton>
            <NavButton current={view === "routines"} onClick={() => setView("routines")}>
              Routines
            </NavButton>
            <NavButton current={view === "accounts"} onClick={() => setView("accounts")}>
              <span className="sm:hidden">Accounts</span>
              <span className="hidden sm:inline">Mail accounts</span>
            </NavButton>
            <NavButton current={view === "ai"} onClick={() => setView("ai")}>
              AI
            </NavButton>
          </nav>
        </div>
      </header>

      <main className={cn("mx-auto w-full flex-1 px-4 pt-6 pb-16 sm:px-6 sm:pt-8", width)}>
        <div className="sheet -mx-4 px-4 py-7 sm:mx-0 sm:rounded-lg sm:px-12 sm:py-10">
          {view === "ai" && <AIScreen />}
          {view === "folders" && <FoldersScreen />}
          {view === "routines" && <RoutinesScreen />}

          {view !== "ai" && accounts.kind === "loading" && <Working>Opening…</Working>}

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
          {view !== "ai" && accounts.kind === "ready" && accounts.list.length === 0 && (
            <SetupScreen first onSaved={load} />
          )}

          {view === "cleanup" && accounts.kind === "ready" && accounts.list.length > 0 && (
            <CleanupScreen accounts={accounts.list} />
          )}

          {view === "inbox" && accounts.kind === "ready" && accounts.list.length > 0 && (
            <InboxScreen accounts={accounts.list} onGoToAI={() => setView("ai")} />
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
        </div>
      </main>

      {version && (
        <footer className={cn("mx-auto w-full px-4 pb-6 text-xs text-muted-foreground sm:px-6", width)}>
          cockatoo · Only the emails that matter. · Build {version}
        </footer>
      )}
    </div>
  )
}

function NavButton({
  current,
  onClick,
  children,
}: {
  current: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <Button
      variant="ghost"
      aria-current={current ? "page" : undefined}
      onClick={onClick}
      className={cn("px-2 sm:px-3.5", current ? "bg-secondary text-foreground" : "font-medium text-muted-foreground")}
    >
      {children}
    </Button>
  )
}
