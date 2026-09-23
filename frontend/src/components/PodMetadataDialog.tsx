import { useEffect, useRef } from 'react'
import type { PodSummary } from '../api/client'
import { formatCPU, formatMemory, formatOwner, formatTimestamp } from '../lib/format'

type PodMetadataDialogProps = {
  pod: PodSummary
  onClose: () => void
}

/**
 * Modal listing everything the API knows about one pod. Uses a native
 * <dialog> so focus trapping, Escape-to-close, and the backdrop come for
 * free; clicking the backdrop also closes it.
 */
export function PodMetadataDialog({ pod, onClose }: PodMetadataDialogProps) {
  const dialogRef = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    dialogRef.current?.showModal()
  }, [])

  const fields: [label: string, value: string][] = [
    ['namespace', pod.namespace],
    ['pod', pod.name],
    ['uid', pod.uid || '—'],
    ['owner', formatOwner(pod.ownerKind, pod.ownerName)],
    ['status', pod.status],
    ['restarts', String(pod.restartCount)],
    ['cpu', formatCPU(pod.cpu)],
    ['memory', formatMemory(pod.memory)],
    ['last seen', formatTimestamp(pod.lastSeen)],
  ]

  return (
    <dialog
      ref={dialogRef}
      onClose={onClose}
      onClick={(e) => {
        if (e.target === e.currentTarget) dialogRef.current?.close()
      }}
      aria-labelledby="pod-metadata-title"
      className="m-auto w-[min(32rem,calc(100vw-2rem))] rounded-lg border border-grid bg-panel p-0 text-ink backdrop:bg-scope-bg/70"
    >
      <div className="flex items-start justify-between gap-4 border-b border-grid px-5 py-4">
        <div className="min-w-0">
          <p className="truncate font-mono text-[11px] tracking-widest text-ink-faint uppercase">
            {pod.namespace}
          </p>
          <h2 id="pod-metadata-title" className="truncate font-mono text-sm text-ink">
            {pod.name}
          </h2>
        </div>
        <button
          type="button"
          onClick={() => dialogRef.current?.close()}
          aria-label="Close"
          className="shrink-0 rounded-full border border-grid px-2 py-0.5 font-mono text-xs text-ink-dim hover:text-ink"
        >
          ✕
        </button>
      </div>

      <dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 px-5 py-4 font-mono text-sm">
        {fields.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-[11px] leading-5 tracking-widest text-ink-faint uppercase">{label}</dt>
            <dd className="min-w-0 break-all text-ink">{value}</dd>
          </div>
        ))}
      </dl>
    </dialog>
  )
}
