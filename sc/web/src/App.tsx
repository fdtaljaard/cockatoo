import { useEffect, useState } from "react"

import { getHealth, getStatus, type Health, type Status } from "@/api"

export default function App() {
  // Keep one event stream open for as long as the page is shown. The exe stops a few
  // seconds after the last window drops it, so closing the window closes the app.
  useEffect(() => {
    const events = new EventSource("/api/events")
    return () => events.close()
  }, [])

  const [health, setHealth] = useState<Health | null>(null)
  const [status, setStatus] = useState<Status | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    Promise.all([getHealth(), getStatus()])
      .then(([h, s]) => {
        setHealth(h)
        setStatus(s)
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  return (
    <main className="mx-auto max-w-2xl px-6 py-16">
      <h1 className="text-3xl font-semibold tracking-tight">cockatoo</h1>
      <p className="mt-2 text-muted-foreground">Email client. The scaffold is running.</p>

      {error && (
        <p role="alert" className="mt-8 rounded-lg border border-destructive/40 px-4 py-3 text-sm text-destructive">
          The backend did not answer: {error}
        </p>
      )}

      {health && status && (
        <dl className="mt-8 divide-y rounded-lg border text-sm">
          <Row label="Backend" value={`Go, build ${health.version}`} />
          <Row label="DuckDB" value={status.duckdbVersion} />
          <Row label="Data file" value={status.path} />
          <Row label="Schema version" value={String(status.schemaVersion)} />
          <Row label="Tables" value={status.tables.join(", ")} />
        </dl>
      )}
    </main>
  )
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex gap-4 px-4 py-3">
      <dt className="w-36 shrink-0 text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{value}</dd>
    </div>
  )
}
