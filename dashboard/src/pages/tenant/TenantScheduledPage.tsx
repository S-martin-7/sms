import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, errorMessage } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'

interface ScheduledSend {
  id: number
  name?: string
  sender: string
  text: string
  recipients?: string[]
  list_id?: number
  recipient_count: number
  send_at: string
  recurrence?: string
  recurrence_days?: number[]
  timezone: string
  status: string
  last_run_at?: string
  total_runs: number
  last_error?: string
  created_at: string
}

const DAYS = ['D', 'L', 'M', 'X', 'J', 'V', 'S']

export function TenantScheduledPage() {
  const { tenant } = useTenant()
  const qc = useQueryClient()

  const items = useQuery({
    queryKey: ['tenant', tenant.id, 'scheduled'],
    queryFn: async () => (await api.get<{ scheduled: ScheduledSend[] }>(`/admin/tenants/${tenant.id}/scheduled`)).data.scheduled,
    refetchInterval: 30_000,
  })

  const togglePause = useMutation({
    mutationFn: async ({ id, pause }: { id: number; pause: boolean }) => {
      await api.post(`/admin/scheduled/${id}/pause`, { paused: pause })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'scheduled'] }),
  })

  const remove = useMutation({
    mutationFn: async (id: number) => api.delete(`/admin/scheduled/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'scheduled'] }),
  })

  return (
    <TenantPage
      title="Envíos programados"
      action={
        <Link
          to="../enviar"
          relative="path"
          className="rounded-md bg-ink px-3.5 py-1.5 text-sm font-medium text-canvas transition-colors hover:bg-ink-soft"
        >
          + Programar envío
        </Link>
      }
    >
      {items.isLoading ? (
        <div className="flex justify-center py-16"><Spinner /></div>
      ) : items.error ? (
        <div className="rounded-md border border-danger/30 bg-danger-soft px-4 py-3 text-sm text-danger-ink">
          {errorMessage(items.error)}
        </div>
      ) : !items.data?.length ? (
        <Empty />
      ) : (
        <div className="space-y-3">
          {items.data.map((s) => (
            <ScheduledCard
              key={s.id}
              s={s}
              onToggle={() => togglePause.mutate({ id: s.id, pause: s.status === 'pending' })}
              onDelete={() => {
                if (confirm(`¿Eliminar la programación "${s.name || `id=${s.id}`}"?`)) remove.mutate(s.id)
              }}
            />
          ))}
        </div>
      )}
    </TenantPage>
  )
}

function ScheduledCard({ s, onToggle, onDelete }: { s: ScheduledSend; onToggle: () => void; onDelete: () => void }) {
  const isWeekly = s.recurrence === 'weekly'
  const isPaused = s.status === 'paused'
  const isCompleted = s.status === 'completed'
  const isFailed = s.status === 'failed'

  return (
    <article className={`flex items-stretch overflow-hidden rounded-lg border bg-surface ${
      isFailed ? 'border-danger/40' : 'border-border'
    }`}>
      {/* Left vertical accent bar */}
      <div className={`w-1 ${
        isPaused ? 'bg-ink-faint' : isFailed ? 'bg-danger' : isCompleted ? 'bg-success' : 'bg-accent'
      }`} />
      <div className="flex flex-1 flex-col gap-3 p-4 sm:flex-row sm:items-center sm:gap-6">
        {/* Title + badges */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="font-display text-lg font-medium text-ink truncate">
              {s.name || <span className="text-ink-mute italic">Sin etiqueta</span>}
            </h3>
            {isPaused && <Pill tone="muted">Pausado</Pill>}
            {isCompleted && <Pill tone="success">Completado</Pill>}
            {isFailed && <Pill tone="danger">Fallido</Pill>}
            {isWeekly && !isPaused && !isCompleted && <Pill tone="accent">Recurrente</Pill>}
          </div>
          <p className="mt-1 truncate text-sm text-ink-soft" title={s.text}>
            <span className="text-ink-mute">»</span> {s.text}
          </p>

          {/* Day-of-week bubbles for weekly schedules */}
          {isWeekly && (
            <div className="mt-3 flex items-center gap-1.5">
              {DAYS.map((label, i) => {
                const active = (s.recurrence_days ?? []).includes(i)
                return (
                  <span
                    key={i}
                    className={`flex h-7 w-7 items-center justify-center rounded-full text-xs font-semibold ${
                      active ? 'bg-accent text-canvas' : 'border border-border bg-canvas text-ink-faint'
                    }`}
                  >
                    {label}
                  </span>
                )
              })}
            </div>
          )}

          <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-mute">
            <Meta icon="🕐">{new Date(s.send_at).toLocaleTimeString('es-CL', { hour: '2-digit', minute: '2-digit' })}</Meta>
            <Meta icon="📨">
              {s.recipient_count.toLocaleString('es-CL')} destinatario{s.recipient_count === 1 ? '' : 's'}
            </Meta>
            <Meta icon="⏵">
              {isCompleted
                ? 'Sin más ejecuciones'
                : `Próximo: ${formatNextRun(s.send_at)}`}
            </Meta>
            {s.total_runs > 0 && (
              <Meta icon="✓">{s.total_runs} ejecución{s.total_runs === 1 ? '' : 'es'}</Meta>
            )}
          </div>

          {s.last_error && (
            <div className="mt-2 rounded border border-danger/30 bg-danger-soft px-2 py-1 text-xs text-danger-ink">
              Último error: {s.last_error}
            </div>
          )}
        </div>

        {/* Actions */}
        <div className="flex items-center gap-2">
          {!isCompleted && (
            <button
              onClick={onToggle}
              role="switch"
              aria-checked={!isPaused}
              className={`relative h-6 w-11 rounded-full transition-colors ${
                isPaused ? 'bg-border' : 'bg-info'
              }`}
              title={isPaused ? 'Reanudar' : 'Pausar'}
            >
              <span
                className={`absolute top-0.5 h-5 w-5 rounded-full bg-canvas shadow transition-transform ${
                  isPaused ? 'left-0.5' : 'left-[22px]'
                }`}
              />
            </button>
          )}
          <button
            onClick={onDelete}
            className="rounded-md border border-border px-2 py-1 text-xs text-danger hover:bg-danger-soft hover:border-danger"
            title="Eliminar"
          >
            Eliminar
          </button>
        </div>
      </div>
    </article>
  )
}

function Meta({ icon, children }: { icon: string; children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span aria-hidden>{icon}</span>
      <span>{children}</span>
    </span>
  )
}

function Pill({ tone, children }: { tone: 'success' | 'danger' | 'muted' | 'accent'; children: React.ReactNode }) {
  const map = {
    success: 'bg-success-soft text-success-ink',
    danger:  'bg-danger-soft text-danger-ink',
    muted:   'bg-muted text-ink-soft',
    accent:  'bg-accent-soft text-accent-ink',
  }[tone]
  return <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider ${map}`}>{children}</span>
}

function formatNextRun(iso: string): string {
  const d = new Date(iso)
  const now = new Date()
  const sameYear = d.getFullYear() === now.getFullYear()
  const fmt: Intl.DateTimeFormatOptions = sameYear
    ? { day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }
    : { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }
  return d.toLocaleString('es-CL', fmt)
}

function Empty() {
  return (
    <div className="rounded-lg border border-dashed border-border bg-surface px-6 py-16 text-center">
      <div className="mx-auto inline-flex h-14 w-14 items-center justify-center rounded-full bg-muted text-2xl">⏰</div>
      <h3 className="mt-4 font-display text-2xl text-ink">Aún no hay envíos programados</h3>
      <p className="mx-auto mt-2 max-w-md text-sm text-ink-mute">
        Programa envíos para una fecha futura, o configura recurrencias semanales (recordatorios,
        boletines). También puedes subir un Excel con N filas para crear un lote completo.
      </p>
      <div className="mt-5">
        <Link
          to="../enviar"
          relative="path"
          className="rounded-md bg-ink px-4 py-2 text-sm font-medium text-canvas hover:bg-ink-soft"
        >
          Programar primer envío
        </Link>
      </div>
      <p className="mx-auto mt-4 max-w-md text-xs text-ink-mute">
        ¿Integras desde otra app? Llama a <code className="rounded bg-muted px-1 font-mono text-[11px]">POST /v1/sms</code> con
        tu llave API + parámetro <code className="rounded bg-muted px-1 font-mono text-[11px]">send_at</code> (RFC3339) y se
        agendará automáticamente.
      </p>
      <p className="mx-auto mt-2 max-w-md text-xs text-ink-mute">
        El último mensaje creado quedará visible en{' '}
        <Link to="../mensajes" relative="path" className="underline">Mensajes</Link>.
      </p>
    </div>
  )
}
