// Typed client for the Go backend. Hand-written for now; generate from OpenAPI when drift hurts.

export interface Health {
  ok: boolean
  time: string
  version: string
}

// What the DuckDB file says about itself (GET /api/status).
export interface Status {
  path: string
  duckdbVersion: string
  schemaVersion: number
  tables: string[]
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path)
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
  return body as T
}

export const getHealth = () => get<Health>("/api/health")
export const getStatus = () => get<Status>("/api/status")
