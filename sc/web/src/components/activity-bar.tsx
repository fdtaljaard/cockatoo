import { useEffect, useState } from "react"

import { onActivity } from "@/api"

// A thin bar across the top of the window while cockatoo waits on Gmail or Claude for something
// you asked for. It shows only when the wait is longer than a moment, so quick answers do not
// flicker it.
export function ActivityBar() {
  const [busy, setBusy] = useState(false)
  const [shown, setShown] = useState(false)
  useEffect(() => onActivity((n) => setBusy(n > 0)), [])
  useEffect(() => {
    if (!busy) {
      const t = setTimeout(() => setShown(false), 0)
      return () => clearTimeout(t)
    }
    const t = setTimeout(() => setShown(true), 400)
    return () => clearTimeout(t)
  }, [busy])
  if (!shown) return null
  return (
    <div
      role="progressbar"
      aria-label="Working"
      className="fixed inset-x-0 top-0 z-50 h-[3px] overflow-hidden"
    >
      <div className="activity-bar h-full w-1/3 rounded-full bg-pen" />
    </div>
  )
}
