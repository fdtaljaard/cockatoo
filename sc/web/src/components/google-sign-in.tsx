import { useEffect, useRef, useState, type FormEvent } from "react"
import { ExternalLink } from "lucide-react"

import { api, type Account, type GoogleSettings, type SignInStart } from "@/api"
import { Button, buttonVariants } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

// Google's own sign-in, for work and school accounts (Google Workspace) that do not take an app
// password. The first time, it asks for the Google Cloud client cockatoo signs in with; after
// that it is one button. Sign-in happens in the default browser; this waits for it.
export function GoogleSignIn({ onSignedIn }: { onSignedIn: (account: Account) => void }) {
  const [settings, setSettings] = useState<GoogleSettings | null>(null)
  const [editing, setEditing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api.googleSettings().then(setSettings, (e: Error) => setError(e.message))
  }, [])

  if (error && !settings) {
    return (
      <p role="alert" className="text-sm text-destructive">
        {error}
      </p>
    )
  }
  if (!settings) return <p className="text-sm text-muted-foreground">Opening…</p>

  if (!settings.configured || editing) {
    return (
      <ClientForm
        settings={settings}
        onSaved={(s) => {
          setSettings(s)
          setEditing(false)
        }}
        onCancel={settings.configured ? () => setEditing(false) : undefined}
      />
    )
  }

  return (
    <div className="space-y-3">
      <SignInButton onSignedIn={onSignedIn} />
      <p className="text-xs text-muted-foreground">
        Signs in with the Google Cloud client {settings.clientId}.{" "}
        <button type="button" className="underline underline-offset-4" onClick={() => setEditing(true)}>
          Change it
        </button>
      </p>
    </div>
  )
}

// The Google Cloud client: the client ID and secret of a "Desktop app" OAuth client.
function ClientForm({
  settings,
  onSaved,
  onCancel,
}: {
  settings: GoogleSettings
  onSaved: (s: GoogleSettings) => void
  onCancel?: () => void
}) {
  const [clientId, setClientId] = useState(settings.clientId ?? "")
  const [secret, setSecret] = useState("")
  const [state, setState] = useState<{ kind: "idle" } | { kind: "busy" } | { kind: "error"; message: string }>({
    kind: "idle",
  })

  async function save(e: FormEvent) {
    e.preventDefault()
    setState({ kind: "busy" })
    try {
      onSaved(await api.saveGoogleClient(clientId, secret))
    } catch (err) {
      setState({ kind: "error", message: (err as Error).message })
    }
  }

  return (
    <form onSubmit={save} className="space-y-4">
      <p className="text-sm text-muted-foreground">
        Google sign-in needs a Google Cloud OAuth client of the type <span className="text-foreground">Desktop app</span>,
        made once by you or your Google Workspace admin. The steps are in cockatoo's readme, under Google sign-in.
      </p>
      <div className="space-y-2">
        <Label htmlFor="google-client-id">Client ID</Label>
        <Input
          id="google-client-id"
          autoComplete="off"
          required
          placeholder="…apps.googleusercontent.com"
          value={clientId}
          onChange={(e) => setClientId(e.target.value)}
          disabled={state.kind === "busy"}
        />
      </div>
      <div className="space-y-2">
        <Label htmlFor="google-client-secret">Client secret</Label>
        <Input
          id="google-client-secret"
          type="password"
          autoComplete="off"
          required
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          disabled={state.kind === "busy"}
        />
        <p className="text-xs text-muted-foreground">Encrypted with your Windows sign-in before it is stored.</p>
      </div>
      {state.kind === "error" && (
        <p role="alert" className="rounded-lg border border-destructive/40 px-4 py-3 text-sm text-destructive">
          {state.message}
        </p>
      )}
      <div className="flex gap-2">
        <Button type="submit" variant="outline" disabled={state.kind === "busy"}>
          {state.kind === "busy" ? "Saving…" : "Save Google client"}
        </Button>
        {onCancel && (
          <Button type="button" variant="ghost" onClick={onCancel} disabled={state.kind === "busy"}>
            Cancel
          </Button>
        )}
      </div>
    </form>
  )
}

type Flow =
  | { kind: "idle" }
  | { kind: "starting" }
  | { kind: "waiting"; start: SignInStart }
  | { kind: "failed"; message: string }

// One button: open Google's sign-in in the browser, then wait for it to come back. hint is the
// address, when signing an existing account in again.
export function SignInButton({
  hint,
  label = "Sign in with Google",
  onSignedIn,
}: {
  hint?: string
  label?: string
  onSignedIn: (account: Account) => void
}) {
  const [flow, setFlow] = useState<Flow>({ kind: "idle" })
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  function follow(start: SignInStart) {
    timer.current = window.setTimeout(() => {
      api.googleSignInStatus(start.state).then(
        (st) => {
          if (st.state === "done" && st.account) {
            setFlow({ kind: "idle" })
            onSignedIn(st.account)
          } else if (st.state === "failed") {
            setFlow({ kind: "failed", message: st.error ?? "Google sign-in did not finish." })
          } else {
            follow(start)
          }
        },
        () => follow(start), // the app may be busy; keep waiting
      )
    }, 1500)
  }

  async function begin() {
    setFlow({ kind: "starting" })
    try {
      const start = await api.startGoogleSignIn(hint)
      setFlow({ kind: "waiting", start })
      follow(start)
    } catch (e) {
      setFlow({ kind: "failed", message: (e as Error).message })
    }
  }

  function stop() {
    window.clearTimeout(timer.current)
    setFlow({ kind: "idle" })
  }

  if (flow.kind === "waiting") {
    return (
      <div aria-live="polite" className="space-y-3 rounded-lg border px-4 py-3 text-sm">
        <p>
          {flow.start.opened
            ? "Google's sign-in opened in your browser. Choose the account there and allow cockatoo to read your mail."
            : "Open Google's sign-in, choose the account and allow cockatoo to read your mail."}
        </p>
        <p className="text-muted-foreground">Waiting for Google…</p>
        <div className="flex flex-wrap gap-2">
          <a
            href={flow.start.url}
            target="_blank"
            rel="noopener noreferrer"
            className={cn(buttonVariants({ variant: flow.start.opened ? "ghost" : "default" }))}
          >
            <ExternalLink aria-hidden />
            {flow.start.opened ? "Open it again" : "Open Google's sign-in"}
          </a>
          <Button variant="ghost" onClick={stop}>
            Cancel
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-2">
      <Button size="lg" onClick={begin} disabled={flow.kind === "starting"}>
        {flow.kind === "starting" ? "Opening Google…" : label}
      </Button>
      {flow.kind === "failed" && (
        <p role="alert" className="text-sm text-destructive">
          {flow.message}
        </p>
      )}
    </div>
  )
}
