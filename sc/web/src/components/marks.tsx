import bird from "@/assets/cockatoo-bird.webp"
import { cn } from "@/lib/utils"

// A ballpoint tick in the margin: this one is dealt with. Drawn once when it appears.
export function PenTick({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 20 20" aria-hidden className={cn("tick size-4 shrink-0 text-pen", className)}>
      <path
        d="M2.8 10.6c1.7 1.3 3.1 2.9 4.5 4.9C9.6 10.7 12.9 6.8 17.4 3.6"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.3"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}

// The page when there is nothing left: the cockatoo, holding the post, and one line.
export function AllClear({ children, note }: { children: React.ReactNode; note?: React.ReactNode }) {
  return (
    <div className="mt-10 flex flex-col items-center text-center">
      <img src={bird} alt="" className="h-28 w-auto" />
      <p className="mt-4 text-base font-semibold">{children}</p>
      {note && <p className="pen mt-1 text-xl">{note}</p>}
    </div>
  )
}
