import { useEffect, useState, type FormEvent } from "react"

import { api, type AIReply, type AISettings } from "@/api"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

type Load = { kind: "loading" } | { kind: "error"; message: string } | { kind: "ready"; settings: AISettings }

// AI settings: turn AI on, add the Anthropic API key, and try it with one message.
export function AIScreen() {
  const [load, setLoad] = useState<Load>({ kind: "loading" })
  const [toggleError, setToggleError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api
      .aiSettings()
      .then((settings) => setLoad({ kind: "ready", settings }))
      .catch((e: Error) => setLoad({ kind: "error", message: e.message }))
  }, [])

  if (load.kind === "loading") return <p className="text-sm text-muted-foreground">Opening…</p>
  if (load.kind === "error") {
    return (
      <p role="alert" className="rounded-lg border border-destructive/40 px-4 py-3 text-sm text-destructive">
        Cockatoo could not read its AI settings: {load.message}
      </p>
    )
  }

  const settings = load.settings
  const update = (next: AISettings) => setLoad({ kind: "ready", settings: next })

  async function toggle(on: boolean) {
    setBusy(true)
    setToggleError(null)
    try {
      update(await api.setAIEnabled(on))
    } catch (e) {
      setToggleError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <h1 className="text-2xl font-semibold tracking-tight">AI</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        Cockatoo uses Anthropic's Claude with your own API key. Mail text goes to Anthropic only when you use an AI
        feature, straight from this computer.
      </p>

      <div className="mt-8 flex items-start gap-3">
        <Checkbox
          id="ai-on"
          checked={settings.enabled}
          disabled={busy}
          onCheckedChange={(checked) => toggle(checked === true)}
          className="mt-0.5"
        />
        <div className="space-y-1">
          <Label htmlFor="ai-on">Use AI in cockatoo</Label>
          <p className="text-sm text-muted-foreground">
            {settings.enabled
              ? `Model: ${settings.model}`
              : settings.hasKey
                ? "AI is off. Your key stays saved for when you turn it back on."
                : "AI is off."}
          </p>
        </div>
      </div>
      {toggleError && (
        <p role="alert" className="mt-3 text-sm text-destructive">
          {toggleError}
        </p>
      )}

      {settings.enabled && (
        <div className="mt-8 space-y-8">
          <KeySection settings={settings} onChange={update} />
          {settings.hasKey && <TestMessage />}
        </div>
      )}
    </div>
  )
}

function KeySection({ settings, onChange }: { settings: AISettings; onChange: (s: AISettings) => void }) {
  const [editing, setEditing] = useState(!settings.hasKey)
  const [key, setKey] = useState("")
  const [state, setState] = useState<{ kind: "idle" } | { kind: "busy" } | { kind: "error"; message: string }>({
    kind: "idle",
  })
  const [confirmRemove, setConfirmRemove] = useState(false)
  const busy = state.kind === "busy"

  async function save(e: FormEvent) {
    e.preventDefault()
    setState({ kind: "busy" })
    try {
      onChange(await api.saveAIKey(key))
      setKey("")
      setEditing(false)
      setState({ kind: "idle" })
    } catch (err) {
      setState({ kind: "error", message: (err as Error).message })
    }
  }

  async function remove() {
    setState({ kind: "busy" })
    try {
      onChange(await api.removeAIKey())
      setConfirmRemove(false)
      setEditing(true)
      setState({ kind: "idle" })
    } catch (err) {
      setState({ kind: "error", message: (err as Error).message })
    }
  }

  if (settings.hasKey && !editing) {
    return (
      <section>
        <h2 className="text-sm font-medium">API key</h2>
        <p className="mt-2 text-sm">
          <span className="font-mono">{settings.keyHint}</span>{" "}
          <span className="text-muted-foreground">saved and encrypted</span>
        </p>
        {!confirmRemove && (
          <div className="mt-3 flex gap-2">
            <Button variant="outline" onClick={() => setEditing(true)}>
              Replace key
            </Button>
            <Button variant="ghost" onClick={() => setConfirmRemove(true)}>
              Remove key
            </Button>
          </div>
        )}
        {confirmRemove && (
          <div className="mt-3 rounded-lg border px-4 py-3 text-sm">
            <p>Remove the saved key from cockatoo? The key itself still works in your Anthropic account.</p>
            {state.kind === "error" && (
              <p role="alert" className="mt-2 text-destructive">
                {state.message}
              </p>
            )}
            <div className="mt-3 flex gap-2">
              <Button variant="destructive" onClick={remove} disabled={busy}>
                {busy ? "Removing…" : "Remove key"}
              </Button>
              <Button variant="ghost" onClick={() => setConfirmRemove(false)} disabled={busy}>
                Keep it
              </Button>
            </div>
          </div>
        )}
      </section>
    )
  }

  return (
    <section>
      <form onSubmit={save} className="space-y-3">
        <div className="space-y-2">
          <Label htmlFor="ai-key">{settings.hasKey ? "New Anthropic API key" : "Anthropic API key"}</Label>
          <Input
            id="ai-key"
            type="password"
            autoComplete="off"
            autoFocus
            required
            placeholder="sk-ant-…"
            value={key}
            onChange={(e) => {
              setKey(e.target.value)
              if (state.kind === "error") setState({ kind: "idle" })
            }}
            disabled={busy}
            aria-describedby="ai-key-help"
          />
          <p id="ai-key-help" className="text-sm text-muted-foreground">
            Create one in the Claude Console, under API keys. Cockatoo checks it with Anthropic before saving, which
            costs nothing, then encrypts it with your Windows sign-in.
          </p>
        </div>
        <div aria-live="polite">
          {state.kind === "error" && (
            <p role="alert" className="rounded-lg border border-destructive/40 px-4 py-3 text-sm text-destructive">
              {state.message}
            </p>
          )}
        </div>
        <div className="flex gap-2">
          <Button type="submit" size="lg" disabled={busy}>
            {busy ? "Checking…" : "Save key"}
          </Button>
          {settings.hasKey && (
            <Button type="button" size="lg" variant="ghost" onClick={() => setEditing(false)} disabled={busy}>
              Cancel
            </Button>
          )}
        </div>
        {settings.hasKey && (
          <p className="text-sm text-muted-foreground">The saved key stays until the new one is accepted.</p>
        )}
      </form>
    </section>
  )
}

function TestMessage() {
  const [state, setState] = useState<
    { kind: "idle" } | { kind: "busy" } | { kind: "ok"; reply: AIReply } | { kind: "error"; message: string }
  >({ kind: "idle" })

  async function send() {
    setState({ kind: "busy" })
    try {
      setState({ kind: "ok", reply: await api.aiTestMessage() })
    } catch (e) {
      setState({ kind: "error", message: (e as Error).message })
    }
  }

  return (
    <section>
      <h2 className="text-sm font-medium">Try it</h2>
      <p className="mt-1 text-sm text-muted-foreground">
        Sends one short message to Claude with your key. It costs a fraction of a cent.
      </p>
      <Button className="mt-3" variant="outline" onClick={send} disabled={state.kind === "busy"}>
        {state.kind === "busy" ? "Waiting for Claude…" : "Send a test message"}
      </Button>
      <div className="mt-3 text-sm" aria-live="polite">
        {state.kind === "ok" && (
          <div className="rounded-lg border px-4 py-3">
            <p>{state.reply.text}</p>
            <p className="mt-1 text-muted-foreground">Answered by {state.reply.model}</p>
          </div>
        )}
        {state.kind === "error" && (
          <p role="alert" className="rounded-lg border border-destructive/40 px-4 py-3 text-destructive">
            {state.message}
          </p>
        )}
      </div>
    </section>
  )
}
