import { useState } from 'react'
import { PodMetadataDialog } from '../components/PodMetadataDialog'
import { PollIntervalSelect } from '../components/PollIntervalSelect'
import { usePods } from '../hooks/usePods'
import { formatCPU, formatMemory, formatOwner } from '../lib/format'
import { DEFAULT_POLL_MS } from '../lib/polling'

export function Dashboard() {
  const [pollMs, setPollMs] = useState<number | null>(DEFAULT_POLL_MS)
  const result = usePods(pollMs)
  // Keyed by namespace/name rather than holding the pod object, so the open
  // dialog shows fresh values as polling refreshes the list.
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const selectedPod =
    result.status === 'ready'
      ? result.pods.find((pod) => `${pod.namespace}/${pod.name}` === selectedKey)
      : undefined

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
                  <th className="px-4 py-3 font-normal">owner</th>
                  <th className="px-4 py-3 font-normal">status</th>
                  <th className="px-4 py-3 font-normal">restarts</th>
                  <th className="px-4 py-3 font-normal">cpu</th>
                  <th className="px-4 py-3 font-normal">memory</th>
                  <th className="px-4 py-3 font-normal">
                    <span className="sr-only">details</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {result.pods.map((pod) => (
                  <tr key={`${pod.namespace}/${pod.name}`} className="border-b border-grid last:border-0">
                    <td className="px-4 py-3 text-ink-dim">{pod.namespace}</td>
                    <td className="px-4 py-3 text-ink">{pod.name}</td>
                    <td className="px-4 py-3 text-ink-dim">{formatOwner(pod.ownerKind, pod.ownerName)}</td>
                    <td className="px-4 py-3 text-ink-dim">{pod.status}</td>
                    <td className="px-4 py-3 text-ink-dim">{pod.restartCount}</td>
                    <td className="px-4 py-3 text-ink">{formatCPU(pod.cpu)}</td>
                    <td className="px-4 py-3 text-ink">{formatMemory(pod.memory)}</td>
                    <td className="px-2 py-3 text-right">
                      <button
                        type="button"
                        onClick={() => setSelectedKey(`${pod.namespace}/${pod.name}`)}
                        aria-label={`View metadata for ${pod.namespace}/${pod.name}`}
                        title="View metadata"
                        className="inline-flex h-6 w-6 items-center justify-center rounded-full border border-grid text-ink-dim transition-colors hover:border-trace hover:text-trace"
                      >
                        <svg viewBox="0 0 16 16" aria-hidden="true" className="h-3.5 w-3.5 fill-current">
                          <circle cx="8" cy="4" r="1.25" />
                          <rect x="7" y="6.5" width="2" height="6.5" rx="1" />
                        </svg>
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </main>

      {selectedPod && (
        <PodMetadataDialog
          key={selectedKey}
          pod={selectedPod}
          onClose={() => setSelectedKey(null)}
        />
      )}
    </div>
  )
}
