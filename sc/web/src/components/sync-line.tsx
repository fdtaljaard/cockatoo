import type { SyncStatus } from "@/api"

// One quiet line under a heading: what the sync is doing, and anything that went wrong.
export function SyncLine({ status, error }: { status: SyncStatus | null; error: string | null }) {
  if (error) {
    return (
      <p role="alert" className="text-sm text-destructive">
        Could not check for new mail: {error}
      </p>
    )
  }
  if (!status) return null

  const reading = status.accounts.find((a) => a.state === "reading" && a.toRead > 0)
  const failed = status.accounts.filter((a) => a.state === "failed")
  const skipped = status.accounts.reduce((n, a) => n + a.skipped, 0)

  return (
    <div aria-live="polite" className="space-y-1 text-sm">
      {status.running && (
        <p className="text-muted-foreground">
          {reading
            ? `Reading your mailbox: ${reading.read.toLocaleString()} of ${reading.toRead.toLocaleString()} messages${
                status.accounts.length > 1 ? ` (${reading.email})` : ""
              }`
            : "Checking for new mail…"}
        </p>
      )}
      {failed.map((a) => (
        <p key={a.accountId} role="alert" className="text-destructive">
          {status.accounts.length > 1 ? `${a.email}: ` : ""}
          {a.error}
        </p>
      ))}
      {!status.running && skipped > 0 && (
        <p className="text-muted-foreground">
          {skipped.toLocaleString()} {skipped === 1 ? "message" : "messages"} could not be read from the server and
          {skipped === 1 ? " is" : " are"} left out.
        </p>
      )}
    </div>
  )
}
