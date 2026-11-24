import type { Inbox } from "@/api"

const count = (n: number, one: string, many: string) => `${n.toLocaleString()} ${n === 1 ? one : many}`

// "1,284 messages in the inbox, 12 unread."
export function inboxLine(inbox: Inbox): string {
  return `${count(inbox.messages, "message", "messages")} in the inbox, ${inbox.unseen.toLocaleString()} unread.`
}
