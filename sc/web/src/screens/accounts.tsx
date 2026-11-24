import { useCallback, useEffect, useState } from "react"

import { api, type Account, type Inbox } from "@/api"
import { SignInButton } from "@/components/google-sign-in"
import { Button } from "@/components/ui/button"
import { inboxLine } from "@/lib/format"

// The saved accounts, each with a live sign-in that proves the connection still works.
export function AccountsScreen({
  accounts,
  onAdd,
  onRemoved,
}: {
  accounts: Account[]
  onAdd: () => void
  onRemoved: () => void
}) {
  return (
    <div className="max-w-xl">
      <div className="flex items-center justify-between gap-4">
        <h1 className="text-2xl font-semibold tracking-tight">Accounts</h1>
        <Button size="lg" variant="outline" onClick={onAdd}>
          Add account
        </Button>
      </div>
      <ul className="mt-6 divide-y rounded-lg border">
        {accounts.map((account) => (
          <AccountRow key={account.id} account={account} onRemoved={onRemoved} />
        ))}
      </ul>
    </div>
  )
}

type Connection = { kind: "checking" } | { kind: "ok"; inbox: Inbox } | { kind: "error"; message: string }

function AccountRow({ account, onRemoved }: { account: Account; onRemoved: () => void }) {
  const [connection, setConnection] = useState<Connection>({ kind: "checking" })
  const [confirming, setConfirming] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [removeError, setRemoveError] = useState<string | null>(null)

  const connect = useCallback(() => {
    api
      .inbox(account.id)
      .then((inbox) => setConnection({ kind: "ok", inbox }))
      .catch((e: Error) => setConnection({ kind: "error", message: e.message }))
  }, [account.id])

  useEffect(connect, [connect])

  function retry() {
    setConnection({ kind: "checking" })
    connect()
  }

  async function remove() {
    setRemoving(true)
    setRemoveError(null)
    try {
      await api.removeAccount(account.id)
      onRemoved()
    } catch (e) {
      setRemoveError((e as Error).message)
      setRemoving(false)
    }
  }

  return (
    <li className="px-4 py-4">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <p className="truncate font-medium">{account.name}</p>
          {account.name !== account.email && (
            <p className="truncate text-sm text-muted-foreground">{account.email}</p>
          )}
          <p className="text-xs text-muted-foreground">
            {account.auth === "google" ? "Signed in with Google" : "App password"}
          </p>
        </div>
        {!confirming && (
          <Button variant="ghost" onClick={() => setConfirming(true)}>
            Remove
          </Button>
        )}
      </div>

      <div className="mt-2 text-sm" aria-live="polite">
        {connection.kind === "checking" && <p className="text-muted-foreground">Connecting to Gmail…</p>}
        {connection.kind === "ok" && (
          <p>
            <span className="font-medium">Connected.</span>{" "}
            <span className="text-muted-foreground">{inboxLine(connection.inbox)}</span>
          </p>
        )}
        {connection.kind === "error" && (
          <div className="space-y-2">
            <p role="alert" className="text-destructive">
              Not connected. {connection.message}
            </p>
            <div className="flex flex-wrap items-start gap-2">
              {account.auth === "google" && (
                <SignInButton hint={account.email} label="Sign in with Google again" onSignedIn={retry} />
              )}
              <Button size="sm" variant="outline" onClick={retry}>
                Try again
              </Button>
            </div>
          </div>
        )}
      </div>

      {confirming && (
        <div className="mt-4 rounded-lg border px-4 py-3 text-sm">
          <p>
            Remove {account.email} from cockatoo? Its saved {account.auth === "google" ? "Google sign-in" : "password"} is
            deleted. Your mail in Gmail is not touched.
          </p>
          {removeError && (
            <p role="alert" className="mt-2 text-destructive">
              {removeError}
            </p>
          )}
          <div className="mt-3 flex gap-2">
            <Button variant="destructive" onClick={remove} disabled={removing}>
              {removing ? "Removing…" : "Remove account"}
            </Button>
            <Button variant="ghost" onClick={() => setConfirming(false)} disabled={removing}>
              Keep it
            </Button>
          </div>
        </div>
      )}
    </li>
  )
}
