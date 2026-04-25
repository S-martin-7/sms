// Badge maps a status string to a coloured pill. Backend statuses we
// know about live in tailwind.config.ts under colors.status.* — anything
// not whitelisted falls back to slate.

const KNOWN = new Set([
  'queued',
  'sending',
  'sent',
  'delivered',
  'undelivered',
  'rejected',
  'failed',
  'pending',
  'in_flight',
  'success',
  'dead',
  'active',
  'suspended',
])

const aliases: Record<string, { fg: string; bg: string }> = {
  active:    { fg: 'text-emerald-700', bg: 'bg-emerald-100' },
  suspended: { fg: 'text-amber-700',   bg: 'bg-amber-100' },
  delivered: { fg: 'text-emerald-700', bg: 'bg-emerald-100' },
  success:   { fg: 'text-emerald-700', bg: 'bg-emerald-100' },
  sent:      { fg: 'text-emerald-600', bg: 'bg-emerald-50' },
  sending:   { fg: 'text-blue-700',    bg: 'bg-blue-100' },
  in_flight: { fg: 'text-blue-700',    bg: 'bg-blue-100' },
  queued:    { fg: 'text-slate-700',   bg: 'bg-slate-100' },
  pending:   { fg: 'text-slate-700',   bg: 'bg-slate-100' },
  undelivered: { fg: 'text-amber-800', bg: 'bg-amber-100' },
  rejected:  { fg: 'text-red-700',     bg: 'bg-red-100' },
  failed:    { fg: 'text-red-700',     bg: 'bg-red-100' },
  dead:      { fg: 'text-red-100',     bg: 'bg-red-900' },
}

export function Badge({ value }: { value: string }) {
  const v = value.toLowerCase()
  const colors = (KNOWN.has(v) && aliases[v]) || { fg: 'text-slate-700', bg: 'bg-slate-100' }
  return (
    <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${colors.bg} ${colors.fg}`}>
      {value}
    </span>
  )
}
