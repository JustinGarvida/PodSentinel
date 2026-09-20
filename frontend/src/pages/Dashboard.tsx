import { useState } from 'react'
import { PollIntervalSelect } from '../components/PollIntervalSelect'
import { usePods } from '../hooks/usePods'
import { formatCPU, formatMemory } from '../lib/format'
import { DEFAULT_POLL_MS } from '../lib/polling'

export function Dashboard() {
  const [pollMs, setPollMs] = useState<number | null>(DEFAULT_POLL_MS)
  const result = usePods(pollMs)

  return (
    <div className="min-h-screen bg-scope-bg text-ink">
      <header className="mx-auto flex max-w-6xl items-center justify-between px-6 py-6">
        <a href="/" className="font-mono text-sm tracking-widest uppercase">
          <span className="text-ink">pod</span>
          <span className="text-trace">sentinel</span>
        </a>
        <div className="rounded-full border border-grid px-3 py-1 font-mono text-[11px] tracking-widest text-ink-dim uppercase">
          pod list
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 pb-20">
        <div className="flex items-center justify-between gap-4">
          <h1 className="font-display text-xs tracking-[0.25em] text-ink-dim uppercase">
            pods
          </h1>
          <PollIntervalSelect value={pollMs} onChange={setPollMs} />
        </div>

        {result.status === 'loading' && (
          <p className="mt-6 font-mono text-sm text-ink-dim">loading pods…</p>
        )}

        {result.status === 'error' && (
          <p className="mt-6 rounded-lg border border-alert/40 bg-alert/5 p-4 font-mono text-sm text-alert">
            failed to load pods: {result.error}
          </p>
        )}

        {result.status === 'ready' && result.pods.length === 0 && (
          <p className="mt-6 font-mono text-sm text-ink-dim">
            no pods have reported a metric in the last hour.
          </p>
        )}

        {result.status === 'ready' && result.pods.length > 0 && (
          <div className="mt-6 overflow-x-auto rounded-lg border border-grid">
            <table className="w-full border-collapse font-mono text-sm">
              <thead>
                <tr className="border-b border-grid text-left text-[11px] tracking-widest text-ink-faint uppercase">
                  <th className="px-4 py-3 font-normal">namespace</th>
                  <th className="px-4 py-3 font-normal">pod</th>
                  <th className="px-4 py-3 font-normal">status</th>
                  <th className="px-4 py-3 font-normal">restarts</th>
                  <th className="px-4 py-3 font-normal">cpu</th>
                  <th className="px-4 py-3 font-normal">memory</th>
                </tr>
              </thead>
              <tbody>
                {result.pods.map((pod) => (
                  <tr key={`${pod.namespace}/${pod.name}`} className="border-b border-grid last:border-0">
                    <td className="px-4 py-3 text-ink-dim">{pod.namespace}</td>
                    <td className="px-4 py-3 text-ink">{pod.name}</td>
                    <td className="px-4 py-3 text-ink-dim">{pod.status}</td>
                    <td className="px-4 py-3 text-ink-dim">{pod.restartCount}</td>
                    <td className="px-4 py-3 text-ink">{formatCPU(pod.cpu)}</td>
                    <td className="px-4 py-3 text-ink">{formatMemory(pod.memory)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>
    </div>
  )
}
