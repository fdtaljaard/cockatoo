import { Eye, EyeOff, Inbox, Paperclip, Reply, type LucideIcon } from "lucide-react"

import type { ListName } from "@/api"

// The lists mail is sorted into, by what it asks of you (SCOPE.md, section 8a), in the order the
// navigation shows them.
export const LISTS: { id: ListName; label: string; icon: LucideIcon; what: string }[] = [
  { id: "reply", label: "Reply", icon: Reply, what: "Someone is waiting on an answer from you." },
  { id: "attention", label: "Attention", icon: Eye, what: "Read it and act on it; nobody expects an answer." },
  { id: "save", label: "Save", icon: Paperclip, what: "It carries something to keep: an invoice, a statement." },
  { id: "unsorted", label: "Unsorted", icon: Inbox, what: "Not in a list yet. Put each one in a list, or ask the AI." },
  {
    id: "ignore",
    label: "Nothing to do",
    icon: EyeOff,
    what: "Mail that needs nothing from you. It stays here, out of your other lists.",
  },
]

// The AI's "nothing to do" suggestion is the Nothing to do list.
export const listLabel = (id: ListName | "done") => LISTS.find((l) => l.id === (id === "done" ? "ignore" : id))!.label

export const isList = (v: string): v is ListName => LISTS.some((l) => l.id === v)
