import { useState, type ChangeEvent, type FormEvent } from "react"

import { api, type Account, type AccountInput, type Inbox } from "@/api"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { inboxLine } from "@/lib/format"

type Result =
  | { kind: "none" }
  | { kind: "busy"; doing: "test" | "save" }
  | { kind: "ok"; inbox: Inbox }
  | { kind: "error"; message: string }

// The form that adds a Gmail account. `first` is the opening screen, when no account exists yet;
// otherwise it was opened from the account list and can be cancelled.
export function SetupScreen({
  first,
  onSaved,
  onCancel,
}: {
  first: boolean
  onSaved: (account: Account) => void
  onCancel?: () => void
}) {
  const [form, setForm] = useState<AccountInput>({ name: "", email: "", password: "" })
  const [result, setResult] = useState<Result>({ kind: "none" })
  const busy = result.kind === "busy"

  const set = (field: keyof AccountInput) => (e: ChangeEvent<HTMLInputElement>) => {
    setForm({ ...form, [field]: e.target.value })
    if (result.kind !== "none") setResult({ kind: "none" }) // a result is about the details as they were
  }

  async function test() {
    setResult({ kind: "busy", doing: "test" })
    try {
      setResult({ kind: "ok", inbox: await api.testAccount(form) })
    } catch (e) {
      setResult({ kind: "error", message: (e as Error).message })
    }
  }

  async function save(e: FormEvent) {
    e.preventDefault()
    setResult({ kind: "busy", doing: "save" })
    try {
      onSaved(await api.addAccount(form))
    } catch (e) {
      setResult({ kind: "error", message: (e as Error).message })
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-semibold tracking-tight">
        {first ? "Add your Gmail account" : "Add another Gmail account"}
      </h1>
      <p className="mt-2 text-sm text-muted-foreground">
        {first
          ? "Cockatoo needs one mail account to start. You can add more later."
          : "Each account signs in on its own."}
      </p>

      <form onSubmit={save} className="mt-8 space-y-5">
        <div className="space-y-2">
          <Label htmlFor="email">Gmail address</Label>
          <Input
            id="email"
            type="email"
            autoComplete="off"
            autoFocus
            required
            placeholder="you@gmail.com"
            value={form.email}
            onChange={set("email")}
            disabled={busy}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="password">App password</Label>
          <Input
            id="password"
            type="password"
            autoComplete="off"
            required
            placeholder="16 letters"
            value={form.password}
            onChange={set("password")}
            disabled={busy}
            aria-describedby="password-help"
          />
          <p id="password-help" className="text-sm text-muted-foreground">
            This is not your Google password. It is a 16-letter password Google makes for one app. Create one at{" "}
            <a
              className="text-foreground underline underline-offset-4"
              href="https://myaccount.google.com/apppasswords"
              target="_blank"
              rel="noreferrer"
            >
              myaccount.google.com/apppasswords
            </a>
            . Your Google account needs 2-Step Verification turned on first.
          </p>
        </div>

        <div className="space-y-2">
          <Label htmlFor="name">
            Name <span className="font-normal text-muted-foreground">(optional)</span>
          </Label>
          <Input
            id="name"
            autoComplete="off"
            placeholder="Personal"
            value={form.name}
            onChange={set("name")}
            disabled={busy}
          />
          <p className="text-sm text-muted-foreground">How this account is labelled in cockatoo.</p>
        </div>

        <div aria-live="polite">
          {result.kind === "ok" && (
            <p className="rounded-lg border px-4 py-3 text-sm">
              <span className="font-medium">Connected.</span> {inboxLine(result.inbox)}
            </p>
          )}
          {result.kind === "error" && (
            <p role="alert" className="rounded-lg border border-destructive/40 px-4 py-3 text-sm text-destructive">
              {result.message}
            </p>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Button type="submit" size="lg" disabled={busy}>
            {result.kind === "busy" && result.doing === "save" ? "Connecting…" : "Save account"}
          </Button>
          <Button type="button" size="lg" variant="outline" onClick={test} disabled={busy}>
            {result.kind === "busy" && result.doing === "test" ? "Connecting…" : "Test connection"}
          </Button>
          {onCancel && (
            <Button type="button" size="lg" variant="ghost" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
          )}
        </div>

        <p className="text-sm text-muted-foreground">
          Save signs in to Gmail first and keeps the account only when that works. The app password is encrypted
          with your Windows sign-in before it is stored, so only you, on this computer, can read it.
        </p>
      </form>
    </div>
  )
}
