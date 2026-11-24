import { useState, type ChangeEvent, type FormEvent } from "react"

import { api, type Account, type AccountInput, type Inbox } from "@/api"
import { GoogleSignIn } from "@/components/google-sign-in"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { inboxLine } from "@/lib/format"
import bird from "@/assets/cockatoo-bird.webp"

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
    <div className="max-w-xl">
      {first && (
        <div className="mb-8 flex items-end gap-5 border-b pb-6">
          <img src={bird} alt="" className="h-24 w-auto" />
          <div className="pb-1">
            <p className="text-3xl font-bold tracking-[-0.03em]">cockatoo</p>
            <p className="text-base font-medium text-muted-foreground">Only the emails that matter.</p>
          </div>
        </div>
      )}
      <h1 className="text-[2rem] leading-tight font-bold tracking-[-0.025em]">
        {first ? "Add your Gmail account" : "Add another Gmail account"}
      </h1>
      <p className="mt-2 text-sm text-muted-foreground">
        {first
          ? "Cockatoo needs one mail account to start. You can add more later."
          : "Each account signs in on its own."}
      </p>

      <section aria-labelledby="google-heading" className="mt-8">
        <h2 id="google-heading" className="section-head">
          Work or school account
        </h2>
        <p className="mt-1 mb-4 text-sm text-muted-foreground">
          Google Workspace accounts sign in with Google itself. You choose the account and allow cockatoo in your
          browser.
        </p>
        <GoogleSignIn onSignedIn={onSaved} />
      </section>

      <h2 className="section-head mt-10 border-t pt-8">Personal Gmail, with an app password</h2>

      <form onSubmit={save} className="mt-4 space-y-5">
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
            {result.kind === "busy" && result.doing === "save" && <Spinner />}
            {result.kind === "busy" && result.doing === "save" ? "Connecting…" : "Save account"}
          </Button>
          <Button type="button" size="lg" variant="outline" onClick={test} disabled={busy}>
            {result.kind === "busy" && result.doing === "test" && <Spinner />}
            {result.kind === "busy" && result.doing === "test" ? "Connecting…" : "Test connection"}
          </Button>
          {onCancel && (
            <Button type="button" size="lg" variant="ghost" onClick={onCancel} disabled={busy}>
              Cancel
            </Button>
          )}
        </div>

        <p className="text-sm text-muted-foreground">
          Save signs in to Gmail first and keeps the account only when that works. The app password is encrypted with
          your Windows sign-in before it is stored, so only you, on this computer, can read it.
        </p>
      </form>
    </div>
  )
}
