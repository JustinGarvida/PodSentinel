import { useCallback, useEffect, useState } from 'react'
import { listPods, type PodSummary } from '../api/client'

type UsePodsResult =
  | { status: 'loading' }
  | { status: 'error'; error: string }
  | { status: 'ready'; pods: PodSummary[] }

/**
 * Fetches the pod list on mount, then again every `intervalMs` (never, if
 * null), tracking its loading/error/ready state. Changing `intervalMs` only
 * restarts the timer; it doesn't trigger an immediate fetch. A refresh that
 * fails after pods have already loaded surfaces as an error, replacing the
 * stale list rather than silently keeping it.
 */
export function usePods(intervalMs: number | null): UsePodsResult {
  const [result, setResult] = useState<UsePodsResult>({ status: 'loading' })

  const fetchPods = useCallback((isCancelled: () => boolean) => {
    listPods()
      .then((pods) => {
        if (!isCancelled()) setResult({ status: 'ready', pods })
      })
      .catch((err: unknown) => {
        if (!isCancelled()) {
          setResult({ status: 'error', error: err instanceof Error ? err.message : String(err) })
        }
      })
  }, [])

  useEffect(() => {
    let cancelled = false
    fetchPods(() => cancelled)
    return () => {
      cancelled = true
    }
  }, [fetchPods])

  useEffect(() => {
    if (intervalMs === null) return

    let cancelled = false
    const intervalId = setInterval(() => fetchPods(() => cancelled), intervalMs)

    return () => {
      cancelled = true
      clearInterval(intervalId)
    }
  }, [fetchPods, intervalMs])

  return result
}
