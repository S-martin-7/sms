import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { StatsResp } from '@/api/stats'
import { Badge } from '@/components/ui/Badge'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { formatDate, formatRelative } from '@/lib/format'

const WINDOWS = [
  { hours: 1, label: '1 h' },
  { hours: 24, label: '24 h' },
  { hours: 168, label: '7 días' },
  { hours: 720, label: '30 días' },
]

export function OverviewPage() {
  const [hours, setHours] = useState(24)
  const stats = useQuery({
    queryKey: ['admin', 'stats', hours],
    queryFn: async () => {
      const { data } = await api.get<StatsResp>(`/admin/stats?hours=${hours}`)
      return data
    },
    refetchInterval: 30_000, // auto-refresh every 30s on this page
  })

  return (
    <div className="space-y-4">
      <div className="flex items-end justify-between">
        <div>
          <h1 className="text-xl font-semibold text-slate-900">Resumen</h1>
          <p className="text-sm text-slate-500">
            Estado de la plataforma — se actualiza cada 30 segundos.
          </p>
        </div>
        <div className="flex gap-1">
          {WINDOWS.map((w) => (
            <button
              key={w.hours}
              onClick={() => setHours(w.hours)}
              className={`rounded-md px-3 py-1.5 text-sm font-medium transition-colors ${
                hours === w.hours
                  ? 'bg-slate-900 text-white'
                  : 'bg-white text-slate-700 border border-slate-200 hover:bg-slate-50'
              }`}
            >
              {w.label}
            </button>
          ))}
        </div>
      </div>

      {stats.isLoading ? (
        <div className="flex justify-center p-10">
          <Spinner />
        </div>
      ) : stats.error ? (
        <Card>
          <CardBody className="text-sm text-red-600">{errorMessage(stats.error)}</CardBody>
        </Card>
      ) : !stats.data ? null : (
        <>
          <KPIGrid s={stats.data} />
          <div className="grid gap-4 lg:grid-cols-2">
            <TopTenantsCard rows={stats.data.top_tenants} />
            <RecentFailuresCard rows={stats.data.recent_failures} />
          </div>
          <StuckDeliveriesCard rows={stats.data.stuck_deliveries} />
        </>
      )}
    </div>
  )
}

function KPIGrid({ s }: { s: StatsResp }) {
  const ratePct = (s.totals.delivery_rate * 100).toFixed(1)
  const cards = [
    { label: 'Total mensajes', value: s.totals.total.toLocaleString(), tone: 'slate' },
    { label: 'Entregados', value: s.totals.delivered.toLocaleString(), tone: 'emerald' },
    { label: 'Rechazados / fallidos', value: s.totals.rejected.toLocaleString(), tone: 'red' },
    { label: 'En cola / enviando', value: (s.totals.queued + s.totals.sent).toLocaleString(), tone: 'blue' },
    { label: 'Tasa de entrega', value: s.totals.total ? `${ratePct}%` : '—', tone: 'emerald' },
  ]
  const toneClasses: Record<string, string> = {
    slate: 'bg-slate-50 border-slate-200 text-slate-900',
    emerald: 'bg-emerald-50 border-emerald-200 text-emerald-800',
    red: 'bg-red-50 border-red-200 text-red-800',
    blue: 'bg-blue-50 border-blue-200 text-blue-800',
  }
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
      {cards.map((c) => (
        <div
          key={c.label}
          className={`rounded-lg border p-4 ${toneClasses[c.tone]}`}
        >
          <div className="text-xs font-medium uppercase tracking-wide opacity-80">{c.label}</div>
          <div className="mt-1 text-2xl font-bold">{c.value}</div>
        </div>
      ))}
    </div>
  )
}

function TopTenantsCard({ rows }: { rows: StatsResp['top_tenants'] }) {
  return (
    <Card>
      <CardHeader>
        <h2 className="text-sm font-semibold">Clientes con mayor volumen</h2>
      </CardHeader>
      <CardBody className="p-0">
        {rows.length === 0 ? (
          <div className="px-4 py-6 text-sm text-slate-500">Sin actividad en este periodo.</div>
        ) : (
          <Table>
            <THead>
              <TR>
                <TH>Cliente</TH>
                <TH className="text-right">Total</TH>
                <TH className="text-right">Entregados</TH>
                <TH className="text-right">Rechazados</TH>
              </TR>
            </THead>
            <TBody>
              {rows.map((r) => (
                <TR key={r.tenant_id}>
                  <TD>
                    <Link to={`/clientes/${r.tenant_id}`} className="font-medium text-slate-900 hover:underline">
                      {r.name}
                    </Link>
                  </TD>
                  <TD className="text-right">{r.total.toLocaleString()}</TD>
                  <TD className="text-right text-emerald-700">{r.delivered.toLocaleString()}</TD>
                  <TD className="text-right text-red-700">{r.rejected.toLocaleString()}</TD>
                </TR>
              ))}
            </TBody>
          </Table>
        )}
      </CardBody>
    </Card>
  )
}

function RecentFailuresCard({ rows }: { rows: StatsResp['recent_failures'] }) {
  return (
    <Card>
      <CardHeader>
        <h2 className="text-sm font-semibold">Mensajes con problemas</h2>
      </CardHeader>
      <CardBody className="p-0">
        {rows.length === 0 ? (
          <div className="px-4 py-6 text-sm text-slate-500">Ningún mensaje fallido en este periodo. 🎉</div>
        ) : (
          <Table>
            <THead>
              <TR>
                <TH>Cliente</TH>
                <TH>Destino</TH>
                <TH>Estado</TH>
                <TH>Error</TH>
                <TH>Cuándo</TH>
              </TR>
            </THead>
            <TBody>
              {rows.map((r) => (
                <TR key={r.id}>
                  <TD className="font-mono text-xs">{r.tenant_id}</TD>
                  <TD className="font-mono text-xs">{r.recipient}</TD>
                  <TD><Badge value={r.status} /></TD>
                  <TD className="max-w-xs truncate text-xs text-red-600" title={r.error_message ?? undefined}>
                    {r.error_code ? `[${r.error_code}] ` : ''}{r.error_message ?? '—'}
                  </TD>
                  <TD className="text-xs text-slate-500" title={formatDate(r.created_at)}>
                    {formatRelative(r.created_at)}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        )}
      </CardBody>
    </Card>
  )
}

function StuckDeliveriesCard({ rows }: { rows: StatsResp['stuck_deliveries'] }) {
  return (
    <Card>
      <CardHeader>
        <h2 className="text-sm font-semibold">Webhooks con problemas</h2>
      </CardHeader>
      <CardBody className="p-0">
        {rows.length === 0 ? (
          <div className="px-4 py-6 text-sm text-slate-500">
            Todas las entregas de webhooks están al día.
          </div>
        ) : (
          <Table>
            <THead>
              <TR>
                <TH>Cliente</TH>
                <TH>Endpoint</TH>
                <TH>Tipo</TH>
                <TH>Estado</TH>
                <TH>Intentos</TH>
                <TH>Último HTTP</TH>
                <TH>Error</TH>
                <TH>Cuándo</TH>
              </TR>
            </THead>
            <TBody>
              {rows.map((r) => (
                <TR key={r.id}>
                  <TD className="font-mono text-xs">{r.tenant_id}</TD>
                  <TD className="font-mono text-xs">{r.endpoint_id}</TD>
                  <TD className="text-xs">{r.event_type}</TD>
                  <TD><Badge value={r.status} /></TD>
                  <TD>{r.attempts}</TD>
                  <TD>{r.last_status ?? '—'}</TD>
                  <TD className="max-w-xs truncate text-xs text-red-600" title={r.last_error ?? undefined}>
                    {r.last_error ?? '—'}
                  </TD>
                  <TD className="text-xs text-slate-500" title={formatDate(r.created_at)}>
                    {formatRelative(r.created_at)}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        )}
      </CardBody>
    </Card>
  )
}
