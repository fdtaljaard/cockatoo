import type { SyncStatus } from "@/api"
import { Spinner } from "@/components/ui/spinner"
import { ProgressBar } from "@/components/working"

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

  const reading = status.accounts.find((a) => (a.state === "reading" || a.state === "sent") && a.toRead > 0)
  const sentErrors = status.accounts.filter((a) => a.sentError)
  const failed = status.accounts.filter((a) => a.state === "failed")
  const skipped = status.accounts.reduce((n, a) => n + a.skipped, 0)

  return (
    <div aria-live="polite" className="space-y-1 text-sm">
      {status.running && (
        <div className="space-y-1.5">
          <p className="flex items-center gap-2 text-muted-foreground">
            <Spinner />
            {reading
              ? `${reading.state === "sent" ? "Reading your sent mail" : "Reading your mailbox"}: ${reading.read.toLocaleString()} of ${reading.toRead.toLocaleString()} messages${
                  status.accounts.length > 1 ? ` (${reading.email})` : ""
                }`
              : "Checking for new mail…"}
          </p>
          {reading && (
            <ProgressBar
              value={reading.read}
              max={reading.toRead}
              label={reading.state === "sent" ? "Reading your sent mail" : "Reading your mailbox"}
            />
          )}
        </div>
      )}
      {failed.map((a) => (
        <p key={a.accountId} role="alert" className="text-destructive">
          {status.accounts.length > 1 ? `${a.email}: ` : ""}
          {a.error}
        </p>
      ))}
      {!status.running &&
        sentErrors.map((a) => (
          <p key={`sent-${a.accountId}`} className="text-muted-foreground">
            {status.accounts.length > 1 ? `${a.email}: ` : ""}Your sent mail could not be read, so cockatoo cannot yet
            tell who you write to. {a.sentError}
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
