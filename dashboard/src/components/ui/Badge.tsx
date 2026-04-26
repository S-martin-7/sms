// Badge maps a status string to a Spanish label + coloured pill.

const aliases: Record<string, { label: string; fg: string; bg: string }> = {
  active:      { label: 'Activo',       fg: 'text-emerald-700', bg: 'bg-emerald-100' },
  suspended:   { label: 'Suspendido',   fg: 'text-amber-700',   bg: 'bg-amber-100' },
  delivered:   { label: 'Entregado',    fg: 'text-emerald-700', bg: 'bg-emerald-100' },
  success:     { label: 'Exitoso',      fg: 'text-emerald-700', bg: 'bg-emerald-100' },
  sent:        { label: 'Enviado',      fg: 'text-emerald-600', bg: 'bg-emerald-50' },
  sending:     { label: 'Enviando',     fg: 'text-blue-700',    bg: 'bg-blue-100' },
  in_flight:   { label: 'En vuelo',     fg: 'text-blue-700',    bg: 'bg-blue-100' },
  queued:      { label: 'En cola',      fg: 'text-slate-700',   bg: 'bg-slate-100' },
  pending:     { label: 'Pendiente',    fg: 'text-slate-700',   bg: 'bg-slate-100' },
  undelivered: { label: 'No entregado', fg: 'text-amber-800',   bg: 'bg-amber-100' },
  rejected:    { label: 'Rechazado',    fg: 'text-red-700',     bg: 'bg-red-100' },
  failed:      { label: 'Fallido',      fg: 'text-red-700',     bg: 'bg-red-100' },
  dead:        { label: 'Muerto',       fg: 'text-red-100',     bg: 'bg-red-900' },
  revoked:     { label: 'Revocada',     fg: 'text-slate-100',   bg: 'bg-slate-700' },
}

export function Badge({ value }: { value: string }) {
  const v = value.toLowerCase()
  const cfg = aliases[v] ?? { label: value, fg: 'text-slate-700', bg: 'bg-slate-100' }
  return (
    <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${cfg.bg} ${cfg.fg}`}>
      {cfg.label}
    </span>
  )
}
