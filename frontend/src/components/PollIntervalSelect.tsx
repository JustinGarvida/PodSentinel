import { POLL_OPTIONS } from '../lib/polling'

type PollIntervalSelectProps = {
  value: number | null
  onChange: (ms: number | null) => void
}

export function PollIntervalSelect({ value, onChange }: PollIntervalSelectProps) {
  const selected = POLL_OPTIONS.find((option) => option.ms === value)

  return (
    <label className="flex items-center gap-2 font-mono text-[11px] tracking-widest text-ink-dim uppercase">
      refresh
      <select
        value={selected?.id}
        onChange={(e) => {
          const option = POLL_OPTIONS.find((o) => o.id === e.target.value)
          if (option) onChange(option.ms)
        }}
        className="rounded-full border border-grid bg-scope-bg px-3 py-1 font-mono text-[11px] tracking-widest text-ink uppercase"
      >
        {POLL_OPTIONS.map((option) => (
          <option key={option.id} value={option.id}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  )
}
