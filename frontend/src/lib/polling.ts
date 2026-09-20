export type PollOption = {
  id: string
  label: string
  /** Milliseconds between refreshes, or null to never refresh automatically. */
  ms: number | null
}

export const POLL_OPTIONS: PollOption[] = [
  { id: '30s', label: '30 sec', ms: 30_000 },
  { id: '1m', label: '1 min', ms: 60_000 },
  { id: '5m', label: '5 min', ms: 300_000 },
  { id: 'off', label: 'off', ms: null },
]

export const DEFAULT_POLL_MS = 30_000
