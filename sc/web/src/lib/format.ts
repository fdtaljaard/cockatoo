import type { Inbox } from "@/api"

const count = (n: number, one: string, many: string) => `${n.toLocaleString()} ${n === 1 ? one : many}`

// "1,284 messages in the inbox, 12 unread."
export function inboxLine(inbox: Inbox): string {
  return `${count(inbox.messages, "message", "messages")} in the inbox, ${inbox.unseen.toLocaleString()} unread.`
}

// "14:05" today, "Yesterday 14:05", "19 Nov" this year, "19 Nov 2025" before.
export function when(iso: string): string {
  const d = new Date(iso)
  const time = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
  const today = new Date()
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1)
  if (d.toDateString() === today.toDateString()) return time
  if (d.toDateString() === yesterday.toDateString()) return `Yesterday ${time}`
  return d.toLocaleDateString([], {
    day: "numeric",
    month: "short",
    year: d.getFullYear() === today.getFullYear() ? undefined : "numeric",
  })
}
