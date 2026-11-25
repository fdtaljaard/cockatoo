import { useCallback, useEffect, useState } from "react"
import { Bot, Brush, FolderOpen, Inbox as InboxIcon, Repeat, Users } from "lucide-react"

import { api, useMock, type Account, type ListName, type ListsView } from "@/api"
import { ActivityBar } from "@/components/activity-bar"
import { Button } from "@/components/ui/button"
import { Working } from "@/components/working"
import { LISTS, isList } from "@/lib/lists"
import { cn } from "@/lib/utils"
import { AccountsScreen } from "@/screens/accounts"
import { AIScreen } from "@/screens/ai"
import { CleanupScreen } from "@/screens/cleanup"
import { FoldersScreen } from "@/screens/folders"
import { InboxScreen } from "@/screens/inbox"
import { ListScreen } from "@/screens/lists"
import { RoutinesScreen } from "@/screens/routines"
import { SetupScreen } from "@/screens/setup"
import mark from "@/assets/cockatoo-mark.png"

type Accounts = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; list: Account[] }
type View = ListName | "inbox" | "cleanup" | "folders" | "routines" | "accounts" | "ai"

// The rest of the app, below the lists.
const TOOLS: { id: View; label: string; short?: string; icon: typeof InboxIcon }[] = [
  { id: "inbox", label: "Inbox", icon: InboxIcon },
  { id: "cleanup", label: "Clean up", icon: Brush },
  { id: "folders", label: "Folders", icon: FolderOpen },
  { id: "routines", label: "Skills & rules", short: "Skills", icon: Repeat },
  { id: "accounts", label: "Mail accounts", short: "Accounts", icon: Users },
  { id: "ai", label: "AI", icon: Bot },
]

const VIEW_KEY = "cockatoo.view"

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
  const [view, setViewState] = useState<View>(() => {
    const saved = localStorage.getItem(VIEW_KEY) ?? ""
    return isList(saved) || TOOLS.some((t) => t.id === saved) ? (saved as View) : "reply"
  })
  const [version, setVersion] = useState("")
  const [counts, setCounts] = useState<ListsView["counts"] | null>(null)

  const setView = (v: View) => {
    setViewState(v)
    localStorage.setItem(VIEW_KEY, v)
    window.scrollTo(0, 0)
  }

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

  // The counts beside the lists. A list screen keeps them up to date; elsewhere they are read once
  // each time the screen changes.
  const ready = accounts.kind === "ready" && accounts.list.length > 0
  useEffect(() => {
    if (!ready || isList(view)) return
    api
      .lists("unsorted", { all: localStorage.getItem("cockatoo.lists.all") === "true", account: "all", done: false })
      .then(
        (v) => setCounts(v.counts),
        () => {},
      )
  }, [ready, view])

  const onCounts = useCallback((c: ListsView["counts"]) => setCounts(c), [])

  // A page of mail reads best narrow; Clean up's tables need the width.
  const width = view === "cleanup" ? "max-w-[76rem]" : "max-w-[60rem]"

  return (
    <div className="min-h-svh md:grid md:grid-cols-[15.5rem_minmax(0,1fr)]">
      <ActivityBar />

      {/* The navigation: down the left side on a wide window, across the top on a narrow one. */}
      <aside className="border-b bg-card md:sticky md:top-0 md:flex md:h-svh md:flex-col md:border-r md:border-b-0">
        <div className="flex items-center gap-2.5 px-4 py-3 text-lg font-bold tracking-[-0.02em] md:px-5 md:pt-5 md:pb-6">
          <img src={mark} alt="" className="h-8 w-auto" />
          cockatoo
        </div>
        <nav
          aria-label="Sections"
          className="flex gap-0.5 overflow-x-auto px-2 pb-2 md:flex-col md:gap-5 md:overflow-y-auto md:px-3 md:pb-5"
        >
          <div className="flex gap-0.5 md:flex-col" role="group" aria-label="Your lists">
            <p className="section-head hidden px-2.5 pb-1.5 md:block">Your mail</p>
            {LISTS.map((l) => (
              <NavItem
                key={l.id}
                current={view === l.id}
                onClick={() => setView(l.id)}
                icon={l.icon}
                count={ready ? counts?.[l.id] : undefined}
                strong={l.id !== "ignore"}
              >
                {l.label}
              </NavItem>
            ))}
          </div>
          <div className="flex gap-0.5 md:flex-col md:border-t md:pt-4">
            {TOOLS.map((t) => (
              <NavItem key={t.id} current={view === t.id} onClick={() => setView(t.id)} icon={t.icon}>
                {t.short ? (
                  <>
                    <span className="md:hidden">{t.short}</span>
                    <span className="hidden md:inline">{t.label}</span>
                  </>
                ) : (
                  t.label
                )}
              </NavItem>
            ))}
          </div>
        </nav>
        {version && (
          <p className="mt-auto hidden px-5 pb-4 text-xs text-muted-foreground md:block">
            Only the emails that matter.
            <br />
            Build {version}
          </p>
        )}
      </aside>

      <main className={cn("mx-auto w-full px-4 pt-6 pb-16 sm:px-6 md:pt-8", width)}>
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

          {isList(view) && ready && (
            <ListScreen
              key={view}
              list={view}
              accounts={accounts.list}
              onCounts={onCounts}
              onGo={setView}
              onGoToAI={() => setView("ai")}
            />
          )}

          {view === "cleanup" && ready && <CleanupScreen accounts={accounts.list} />}

          {view === "inbox" && ready && <InboxScreen accounts={accounts.list} onGoToAI={() => setView("ai")} />}

          {view === "accounts" && ready && adding && (
            <SetupScreen
              first={false}
              onSaved={() => {
                setAdding(false)
                load()
              }}
              onCancel={() => setAdding(false)}
            />
          )}

          {view === "accounts" && ready && !adding && (
            <AccountsScreen accounts={accounts.list} onAdd={() => setAdding(true)} onRemoved={load} />
          )}
        </div>
      </main>
    </div>
  )
}

// One place to go: a list with its count of open mail, or another screen.
function NavItem({
  current,
  onClick,
  icon: Icon,
  count,
  strong = false,
  children,
}: {
  current: boolean
  onClick: () => void
  icon: typeof InboxIcon
  count?: number
  strong?: boolean
  children: React.ReactNode
}) {
  return (
    <Button
      variant="ghost"
      aria-current={current ? "page" : undefined}
      onClick={onClick}
      className={cn(
        "shrink-0 justify-start gap-2.5 px-2.5 md:w-full",
        strong && "md:h-10 md:text-[0.9375rem]",
        current ? "bg-secondary text-foreground" : "font-medium text-muted-foreground",
      )}
    >
      <Icon aria-hidden className={cn(strong && "md:size-[1.05rem]")} />
      <span className="min-w-0 truncate">{children}</span>
      {count !== undefined && (
        <span
          className={cn(
            "ml-auto pl-2 text-xs tabular-nums",
            count > 0 && strong ? "font-bold text-foreground" : "font-medium text-muted-foreground",
          )}
        >
          {count.toLocaleString()}
          <span className="sr-only"> open</span>
        </span>
      )}
    </Button>
  )
}
