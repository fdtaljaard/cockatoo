import { cn } from "cn"

export { cn }

// A tab (which account is shown), underlined when it is the one shown, like a tab on a printout.
export const tabClass = (on: boolean) =>
  cn(
    "h-9 rounded-none border-0 border-b-2 px-0.5 hover:bg-transparent dark:hover:bg-transparent",
    on
      ? "border-foreground text-foreground"
      : "border-transparent font-medium text-muted-foreground hover:text-foreground",
  )
