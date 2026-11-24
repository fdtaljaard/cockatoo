import { Loader2 } from "lucide-react"

import { cn } from "@/lib/utils"

// A small turning circle for work in progress. Still for people who ask for less motion.
export function Spinner({ className }: { className?: string }) {
  return <Loader2 aria-hidden className={cn("size-4 shrink-0 motion-safe:animate-spin", className)} />
}
