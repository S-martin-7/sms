import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'

interface Bucket { ts: string; total: number; delivered: number; failed: number }
interface TopRecipient { recipient: string; total: number; delivered: number }
interface Totals { total: number; delivered: number; failed: number; delivery_rate: number }
interface ReportResp {
  from: string; to: string; bucket: string
  totals: Totals
  previous?: Totals
  series: Bucket[]
  top_recipients: TopRecipient[]
}

const RANGES = [
  { hours: 24,    label: '24 h' },
  { hours: 24*7,  label: '7 d' },
  { hours: 24*30, label: '30 d' },
  { hours: 24*90, label: '90 d' },
]

// Editorial reports — big serif headline number, sparklines, area chart
// (custom SVG, no chart lib), top recipients with bar fills.
export function TenantReportsPage() {
  const { tenant } = useTenant()
  const [hours, setHours] = useState(24)

  const report = useQuery({
    queryKey: ['tenant', tenant.id, 'report', hours],
    queryFn: async () => (await api.get<ReportResp>(`/admin/tenants/${tenant.id}/report?hours=${hours}`)).data,
    refetchInterval: 30_000,
  })

  return (
    <TenantPage
      title="Reportes"
      action={
        <div className="flex gap-1.5">
          {RANGES.map((r) => (
            <button
              key={r.hours}
              onClick={() => setHours(r.hours)}
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                hours === r.hours
                  ? 'bg-ink text-canvas'
                  : 'border border-border bg-surface text-ink-soft hover:border-ink hover:text-ink'
              }`}
            >
              {r.label}
            </button>
          ))}
          <ExportButton tenantId={tenant.id} hours={hours} disabled={!report.data?.totals.total} />
        </div>
      }
    >
      {report.isLoading ? (
        <div className="flex justify-center py-20"><Spinner /></div>
      ) : report.error ? (
        <p className="rounded-md border border-danger-soft bg-danger-soft/40 px-4 py-3 text-sm text-danger-ink">
          {errorMessage(report.error)}
        </p>
      ) : !report.data ? null : (
        <>
          <Headline data={report.data} />
          <KPIRow data={report.data} />
          <VolumeChart data={report.data} />
          <TopRecipientsCard rows={report.data.top_recipients} />
        </>
      )}
    </TenantPage>
  )
}

// ── Headline ────────────────────────────────────────────────────────

function Headline({ data }: { data: ReportResp }) {
  const total = data.totals.total
  const prev = data.previous?.total ?? 0
  const delta = prev === 0 ? null : ((total - prev) / prev) * 100
  const since = formatRangeLabel(data.from, data.to)
  return (
    <section className="border-b border-border pb-8">
      <p className="text-xs uppercase tracking-[0.2em] text-ink-mute">Mensajes enviados {since}</p>
      <div className="mt-2 flex flex-wrap items-end gap-x-6 gap-y-2">
        <span className="font-display text-7xl font-medium tabular leading-none tracking-tightest text-ink">
          {total.toLocaleString('es-CL')}
        </span>
        {delta !== null && (
          <span
            className={`rounded-full px-2.5 py-1 text-sm font-medium tabular ${
              delta > 0 ? 'bg-success-soft text-success-ink' : delta < 0 ? 'bg-danger-soft text-danger-ink' : 'bg-muted text-ink-mute'
            }`}
            title={`Período anterior: ${prev.toLocaleString('es-CL')}`}
          >
            {delta > 0 ? '↗' : delta < 0 ? '↘' : '·'} {Math.abs(delta).toFixed(1)}%
          </span>
        )}
      </div>
    </section>
  )
}

function formatRangeLabel(fromISO: string, toISO: string): string {
  const from = new Date(fromISO)
  const to = new Date(toISO)
  const hours = (to.getTime() - from.getTime()) / 3_600_000
  if (hours <= 25) return 'en las últimas 24 horas'
  if (hours <= 24 * 8) return 'en los últimos 7 días'
  if (hours <= 24 * 31) return 'en los últimos 30 días'
  return `entre ${from.toLocaleDateString('es-CL')} y ${to.toLocaleDateString('es-CL')}`
}

// ── KPI Row ─────────────────────────────────────────────────────────

function KPIRow({ data }: { data: ReportResp }) {
  const t = data.totals
  const prev = data.previous
  const ratePct = (t.delivery_rate * 100).toFixed(1)
  const prevRatePct = prev ? (prev.delivery_rate * 100).toFixed(1) : null

  // Build sparkline series for delivered + failed for visual interest.
  const deliveredSpark = data.series.map((b) => b.delivered)
  const failedSpark = data.series.map((b) => b.failed)
  const totalSpark = data.series.map((b) => b.total)

  return (
    <section className="grid gap-px overflow-hidden rounded-lg border border-border bg-border sm:grid-cols-3">
      <KPI
        label="Entregados"
        value={t.delivered.toLocaleString('es-CL')}
        sub={prev ? `${prev.delivered.toLocaleString('es-CL')} antes` : undefined}
        spark={deliveredSpark}
        sparkColor="#15803d"
      />
      <KPI
        label="No entregados / rechazados"
        value={t.failed.toLocaleString('es-CL')}
        sub={prev ? `${prev.failed.toLocaleString('es-CL')} antes` : undefined}
        spark={failedSpark}
        sparkColor="#b91c1c"
      />
      <KPI
        label="Tasa de entrega"
        value={t.total ? `${ratePct}%` : '—'}
        sub={prev?.delivery_rate ? `${prevRatePct}% antes` : undefined}
        spark={totalSpark}
        sparkColor="#b45309"
      />
    </section>
  )
}

function KPI({
  label, value, sub, spark, sparkColor,
}: { label: string; value: string; sub?: string; spark: number[]; sparkColor: string }) {
  return (
    <div className="bg-surface p-5">
      <p className="text-[11px] uppercase tracking-wider text-ink-mute">{label}</p>
      <p className="mt-1.5 font-display text-4xl font-medium tabular tracking-tightest text-ink">{value}</p>
      <div className="mt-3 flex items-end justify-between gap-3">
        <p className="text-xs text-ink-mute">{sub ?? ' '}</p>
        <Sparkline data={spark} color={sparkColor} />
      </div>
    </div>
  )
}

function Sparkline({ data, color }: { data: number[]; color: string }) {
  if (data.length === 0) return <span className="text-ink-faint text-xs">sin datos</span>
  const w = 96
  const h = 32
  const max = Math.max(...data, 1)
  const step = data.length > 1 ? w / (data.length - 1) : 0
  const pts = data.map((v, i) => `${(i * step).toFixed(1)},${(h - (v / max) * h).toFixed(1)}`).join(' ')
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="opacity-80">
      <polyline points={pts} fill="none" stroke={color} strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  )
}

// ── Volume area chart ────────────────────────────────────────────────

function VolumeChart({ data }: { data: ReportResp }) {
  const series = data.series
  const padding = { top: 16, right: 12, bottom: 28, left: 36 }
  const w = 800, h = 220
  const innerW = w - padding.left - padding.right
  const innerH = h - padding.top - padding.bottom

  const max = Math.max(1, ...series.map((b) => b.total))
  const tickCount = 4

  const path = useMemo(() => {
    if (series.length === 0) return { area: '', line: '' }
    const step = series.length > 1 ? innerW / (series.length - 1) : 0
    const points = series.map((b, i) => {
      const x = padding.left + i * step
      const y = padding.top + innerH - (b.delivered / max) * innerH
      return [x, y] as const
    })
    const line = points.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${x},${y}`).join(' ')
    const area = `${line} L${points[points.length - 1][0]},${padding.top + innerH} L${points[0][0]},${padding.top + innerH} Z`
    return { area, line }
  }, [series, innerW, innerH, max, padding.left, padding.top])

  const fmtX = (iso: string) => {
    const d = new Date(iso)
    if (data.bucket === 'hour') return d.toLocaleTimeString('es-CL', { hour: '2-digit', minute: '2-digit' })
    return d.toLocaleDateString('es-CL', { day: '2-digit', month: 'short' })
  }

  return (
    <section className="rounded-lg border border-border bg-surface p-5">
      <header className="mb-3 flex items-baseline justify-between">
        <h2 className="font-display text-xl font-medium text-ink">Volumen entregado</h2>
        <span className="text-xs uppercase tracking-wider text-ink-mute">
          {data.bucket === 'hour' ? 'por hora' : 'por día'}
        </span>
      </header>
      {series.length === 0 ? (
        <div className="flex h-40 items-center justify-center text-sm text-ink-mute">
          Sin actividad en este rango.
        </div>
      ) : (
        <svg viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" className="h-56 w-full">
          <defs>
            <linearGradient id="areaFill" x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor="#b45309" stopOpacity="0.18" />
              <stop offset="100%" stopColor="#b45309" stopOpacity="0" />
            </linearGradient>
          </defs>
          {/* Y grid + ticks */}
          {Array.from({ length: tickCount + 1 }).map((_, i) => {
            const y = padding.top + (innerH * i) / tickCount
            const v = Math.round(max - (max * i) / tickCount)
            return (
              <g key={i}>
                <line x1={padding.left} x2={w - padding.right} y1={y} y2={y} stroke="#e7e5e0" strokeWidth="0.5" />
                <text x={padding.left - 6} y={y + 3} textAnchor="end" className="fill-ink-faint" style={{ fontSize: 10, fontFamily: 'Geist Mono, monospace' }}>
                  {v}
                </text>
              </g>
            )
          })}
          {/* Area + line */}
          <path d={path.area} fill="url(#areaFill)" />
          <path d={path.line} fill="none" stroke="#b45309" strokeWidth="1.6" strokeLinejoin="round" strokeLinecap="round" />
          {/* X ticks: 4 evenly distributed */}
          {[0, 0.33, 0.66, 1].map((t) => {
            const idx = Math.min(series.length - 1, Math.round(t * (series.length - 1)))
            const x = padding.left + (series.length > 1 ? idx * (innerW / (series.length - 1)) : innerW / 2)
            return (
              <text key={t} x={x} y={h - 8} textAnchor="middle" className="fill-ink-mute" style={{ fontSize: 10, fontFamily: 'Geist, sans-serif' }}>
                {fmtX(series[idx].ts)}
              </text>
            )
          })}
        </svg>
      )}
    </section>
  )
}

// ── Top Recipients ──────────────────────────────────────────────────

function TopRecipientsCard({ rows }: { rows: TopRecipient[] }) {
  if (!rows.length) {
    return (
      <section className="rounded-lg border border-border bg-surface p-5">
        <h2 className="font-display text-xl font-medium text-ink">Top destinatarios</h2>
        <p className="mt-2 text-sm text-ink-mute">Sin destinatarios en este rango.</p>
      </section>
    )
  }
  const max = Math.max(...rows.map((r) => r.total))
  return (
    <section className="overflow-hidden rounded-lg border border-border bg-surface">
      <header className="flex items-baseline justify-between border-b border-border px-5 py-3">
        <h2 className="font-display text-xl font-medium text-ink">Top destinatarios</h2>
        <span className="text-[11px] uppercase tracking-wider text-ink-mute">
          Mostrando {rows.length}
        </span>
      </header>
      <div className="divide-y divide-border">
        {rows.map((r, i) => {
          const ratePct = r.total > 0 ? (r.delivered / r.total) * 100 : 0
          return (
            <div key={r.recipient} className="grid grid-cols-[28px_minmax(0,1fr)_120px_80px] items-center gap-4 px-5 py-3">
              <span className="font-display text-sm text-ink-faint tabular">{String(i + 1).padStart(2, '0')}</span>
              <div className="flex flex-col">
                <span className="font-mono text-sm tabular text-ink">{r.recipient}</span>
                <div className="mt-1 h-1 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full bg-accent"
                    style={{ width: `${(r.total / max) * 100}%` }}
                  />
                </div>
              </div>
              <span className="text-right tabular text-sm text-ink">
                {r.total.toLocaleString('es-CL')} <span className="text-ink-mute text-xs">msgs</span>
              </span>
              <span className={`text-right tabular text-xs ${ratePct >= 90 ? 'text-success-ink' : ratePct >= 50 ? 'text-warning-ink' : 'text-danger-ink'}`}>
                {ratePct.toFixed(0)}%
              </span>
            </div>
          )
        })}
      </div>
    </section>
  )
}

// ── Export ──────────────────────────────────────────────────────────

function ExportButton({ tenantId, hours, disabled }: { tenantId: number; hours: number; disabled: boolean }) {
  const onExport = async () => {
    // Fetch the full message list for the tenant in the window via the
    // existing /admin/messages endpoint (we already have tenant_id and
    // from filters there). Then turn into CSV client-side. No new backend.
    const from = new Date(Date.now() - hours * 3_600_000).toISOString()
    const url = `/admin/messages?tenant_id=${tenantId}&from=${encodeURIComponent(from)}&limit=200`
    const all: any[] = []
    let cursor: string | null = null
    while (true) {
      const u: string = cursor ? `${url}&cursor=${encodeURIComponent(cursor)}` : url
      const resp = await api.get<{ messages: any[]; next_cursor: string | null }>(u)
      all.push(...resp.data.messages)
      cursor = resp.data.next_cursor
      if (!cursor || all.length >= 5000) break
    }

    const headers = ['id', 'created_at', 'sent_at', 'final_at', 'sender', 'to', 'status', 'horisen_msg_id', 'error_code', 'error_message', 'client_ref']
    const escape = (v: any) => {
      if (v == null) return ''
      const s = String(v).replace(/"/g, '""')
      return /[",\n]/.test(s) ? `"${s}"` : s
    }
    const rows = all.map((m) => headers.map((h) => escape(m[h])).join(','))
    const csv = [headers.join(','), ...rows].join('\n')
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
    const link = document.createElement('a')
    link.href = URL.createObjectURL(blob)
    link.download = `mensajes-tenant${tenantId}-${new Date().toISOString().slice(0, 10)}.csv`
    link.click()
    URL.revokeObjectURL(link.href)
  }

  return (
    <button
      onClick={onExport}
      disabled={disabled}
      className="rounded-md border border-border bg-surface px-3.5 py-1.5 text-sm font-medium text-ink-soft transition-colors hover:border-ink hover:text-ink disabled:cursor-not-allowed disabled:opacity-50"
    >
      Exportar CSV
    </button>
  )
}
