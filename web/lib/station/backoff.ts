const BASE_BACKOFF_MS = 2000
const MAX_BACKOFF_MS = 60_000

export function backoffMs(attempts: number): number {
  return Math.min(BASE_BACKOFF_MS * 2 ** attempts, MAX_BACKOFF_MS)
}
