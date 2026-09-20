const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080'

export type PodSummary = {
  namespace: string
  name: string
  status: string
  restartCount: number
  cpu: number
  memory: number
}

/** Fetches the Go agent's pod list (GET /api/v1/pods). Throws on a non-2xx response. */
export async function listPods(): Promise<PodSummary[]> {
  const res = await fetch(`${API_BASE_URL}/api/v1/pods`)
  if (!res.ok) {
    throw new Error(`GET /api/v1/pods failed: ${res.status} ${res.statusText}`)
  }
  return res.json()
}
