import { useEffect, useState } from 'react'
import { listPods, type PodSummary } from '../api/client'

const REFRESH_INTERVAL_MS = 10_000

type UsePodsResult =
  | { status: 'loading' }
  | { status: 'error'; error: string }
  | { status: 'ready'; pods: PodSummary[] }

/**
 * Fetches the pod list on mount and every REFRESH_INTERVAL_MS after,
 * tracking its loading/error/ready state. A refresh that fails after
 * pods have already loaded surfaces as an error, replacing the stale list
 * rather than silently keeping it.
 */
export function usePods(): UsePodsResult {
  const [result, setResult] = useState<UsePodsResult>({ status: 'loading' })

  useEffect(() => {
    let cancelled = false

    const fetchPods = () => {
      listPods()
        .then((pods) => {
          if (!cancelled) setResult({ status: 'ready', pods })
        })
        .catch((err: unknown) => {
          if (!cancelled) {
            setResult({ status: 'error', error: err instanceof Error ? err.message : String(err) })
          }
        })
    }

    fetchPods()
    const intervalId = setInterval(fetchPods, REFRESH_INTERVAL_MS)

    return () => {
      cancelled = true
      clearInterval(intervalId)
    }
  }, [])

  return result
}
