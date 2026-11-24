import type { Routine, RoutineMatch } from "@/api"

// Personal mail services: a routine for one of these is for the address, never the whole domain.
const PERSONAL = new Set([
  "gmail.com",
  "googlemail.com",
  "outlook.com",
  "hotmail.com",
  "live.com",
  "msn.com",
  "yahoo.com",
  "icloud.com",
  "me.com",
  "aol.com",
  "proton.me",
  "protonmail.com",
  "gmx.com",
])

export const domainOf = (address: string) => address.slice(address.lastIndexOf("@") + 1).toLowerCase()

// Whether a routine can be for the sender's whole domain: not for gmail.com and the like.
export const domainAllowed = (address: string) => address.includes("@") && !PERSONAL.has(domainOf(address))

// "2026-11" is a month folder, "2026-11-24" a day folder: saved into one, a routine makes the
// folder for each mail's own month or day next to it.
export function datedOf(path: string): { parent: string; dated: "" | "month" | "day"; name: string } {
  const segs = path ? path.split("/") : []
  const last = segs[segs.length - 1] ?? ""
  const dated = /^\d{4}-\d{2}$/.test(last) ? "month" : /^\d{4}-\d{2}-\d{2}$/.test(last) ? "day" : ""
  return dated ? { parent: segs.slice(0, -1).join("/"), dated, name: last } : { parent: path, dated: "", name: "" }
}

const trail = (folder: string, path: string) => [folder, ...(path ? path.split("/") : [])].join(" › ")

// "Expenses › Bills › the mail's month" for a routine.
export function routineWhere(folderName: string, path: string, dated?: "" | "month" | "day") {
  const where = trail(folderName, path)
  return dated ? `${where} › a folder for the mail's ${dated}` : where
}

// "Expenses › Bills › 2026-11" for one message.
export function matchWhere(m: RoutineMatch) {
  return trail(m.folderName ?? "", m.name ? (m.parent ? `${m.parent}/${m.name}` : m.name) : m.parent)
}

// "Mail from acme.com with “invoice” in the subject".
export function routineMail(r: Pick<Routine, "from" | "subject">) {
  return `Mail from ${r.from}${r.subject ? ` with “${r.subject}” in the subject` : ""}`
}
