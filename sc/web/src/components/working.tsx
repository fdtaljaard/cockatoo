import { useEffect, useState } from "react"

import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"

// A line for work that takes a while: a spinner, what is happening, and, after a few seconds,
// how long it has taken, so a slow mail server or a long AI answer never looks stuck.
export function Working({ children, className }: { children: React.ReactNode; className?: string }) {
  const seconds = useSeconds()
  return (
    <p
      role="status"
      aria-live="polite"
      className={cn("flex items-center gap-2 text-sm text-muted-foreground", className)}
    >
      <Spinner />
      <span>
        {children}
        {seconds >= 3 && <span className="tabular-nums"> · {seconds} s</span>}
      </span>
    </p>
  )
}

// A bar for work whose size is known: how far it has got.
export function ProgressBar({ value, max, label }: { value: number; max: number; label: string }) {
  const pct = max > 0 ? Math.min(100, Math.round((value / max) * 100)) : 0
  return (
    <div
      role="progressbar"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-valuenow={value}
      className="h-1 w-full max-w-sm overflow-hidden rounded-full bg-muted"
    >
      <div className="h-full rounded-full bg-pen transition-[width] duration-500" style={{ width: `${pct}%` }} />
    </div>
  )
}

// Seconds since the component appeared.
function useSeconds() {
  const [start] = useState(() => Date.now())
  const [now, setNow] = useState(start)
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  return Math.floor((now - start) / 1000)
}
