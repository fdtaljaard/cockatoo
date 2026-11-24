import { useCallback, useEffect, useRef, useState } from "react"

import { api, type SyncStatus } from "@/api"

// useSync starts a sync of every account's inbox when the screen opens and follows it while it
// runs. `tick` changes whenever progress moves, so a screen can re-read its data from the copy.
export function useSync() {
  const [status, setStatus] = useState<SyncStatus | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tick, setTick] = useState(0)
  const timer = useRef<number | undefined>(undefined)
  const mounted = useRef(false)

  const start = useCallback(() => {
    function follow(s: SyncStatus) {
      if (!mounted.current) return
      setStatus(s)
      setError(null)
      setTick((t) => t + 1)
      window.clearTimeout(timer.current)
      if (s.running) {
        timer.current = window.setTimeout(() => api.syncStatus().then(follow, fail), 1000)
      }
    }
    function fail(e: Error) {
      if (mounted.current) setError(e.message)
    }
    api.startSync().then(follow, fail)
  }, [])

  useEffect(() => {
    mounted.current = true
    start()
    return () => {
      mounted.current = false
      window.clearTimeout(timer.current)
    }
  }, [start])

  return { status, error, tick, start }
}
