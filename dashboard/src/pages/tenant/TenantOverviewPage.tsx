import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey, InboundNumber, Message, WebhookEndpoint } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatDate, formatRelative, truncate } from '@/lib/format'

interface QuotaResp {
  window_start: string
  tenants: Array<{
    tenant_id: number
    name: string
    daily_sms_limit: number
    sent_today: number
    remaining: number
    percent_used: number
  }>
}

// Tenant overview — KPIs + ultimos 5 mensajes + panel de sender IDs y cuota.
export function TenantOverviewPage() {
  const { tenant, refresh } = useTenant()

  const messages = useQuery({
    queryKey: ['tenant', tenant.id, 'messages-recent'],
    queryFn: async () => {
      const { data } = await api.get<{ messages: Message[] }>(
        `/admin/messages?tenant_id=${tenant.id}&limit=5`,
      )
      return data.messages
    },
  })
  const keys = useQuery({
    queryKey: ['tenant', tenant.id, 'keys'],
    queryFn: async () => {
      const { data } = await api.get<{ keys: APIKey[] }>(`/admin/tenants/${tenant.id}/api-keys`)
      return data.keys
    },
  })
  const webhooks = useQuery({
    queryKey: ['tenant', tenant.id, 'webhooks'],
    queryFn: async () => {
      const { data } = await api.get<{ endpoints: WebhookEndpoint[] }>(`/admin/tenants/${tenant.id}/webhooks`)
      return data.endpoints
    },
  })
  const inbound = useQuery({
    queryKey: ['inbound-numbers', 'all'],
    queryFn: async () => {
      const { data } = await api.get<{ numbers: InboundNumber[] }>('/admin/inbound-numbers')
      return data.numbers.filter((n) => n.tenant_id === tenant.id)
    },
  })

  // Sólo lo invocamos si el tenant tiene daily_sms_limit. Sin límite no
  // hay nada que graficar.
  const quotaEnabled = !!tenant.daily_sms_limit
  const quota = useQuery({
    queryKey: ['quota-today'],
    enabled: quotaEnabled,
    queryFn: async () => {
      const { data } = await api.get<QuotaResp>('/admin/stats/quota')
      return data
    },
    refetchInterval: 30_000,
  })
  const myQuota = quota.data?.tenants.find((t) => t.tenant_id === tenant.id)

  const activeKeys = keys.data?.filter((k) => !k.revoked_at).length ?? 0
  const activeWebhooks = webhooks.data?.filter((w) => w.active).length ?? 0
  const inboundCount = inbound.data?.length ?? 0

  return (
    <TenantPage title="Resumen del cliente">
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <Kpi label="Llaves API activas" value={keys.isLoading ? '…' : activeKeys} link="llaves" />
        <Kpi label="Webhooks activos" value={webhooks.isLoading ? '…' : activeWebhooks} link="webhooks" />
        <Kpi label="Números entrantes" value={inbound.isLoading ? '…' : inboundCount} link="numeros" />
        <Kpi
          label="Estado del cliente"
          value={<Badge value={tenant.status} />}
          subtitle={tenant.daily_sms_limit ? `Límite ${tenant.daily_sms_limit}/día` : 'Sin límite diario'}
        />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <QuotaCard
          enabled={quotaEnabled}
          limit={tenant.daily_sms_limit ?? null}
          sentToday={myQuota?.sent_today ?? 0}
          loading={quota.isLoading}
        />
        <AllowedSendersCard onChanged={refresh} />
      </div>

      <Card>
        <CardHeader>
          <h2 className="text-sm font-semibold">Últimos mensajes</h2>
          <Link to="../mensajes" relative="path" className="text-xs text-slate-500 hover:underline">
            Ver todos →
          </Link>
        </CardHeader>
        <CardBody className="p-0">
          {messages.isLoading ? (
            <div className="flex justify-center p-8"><Spinner /></div>
          ) : messages.error ? (
            <div className="px-4 py-6 text-sm text-red-600">{errorMessage(messages.error)}</div>
          ) : !messages.data?.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              Este cliente aún no ha enviado ni recibido mensajes.
            </div>
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>ID</TH>
                  <TH>Destinatario</TH>
                  <TH>Estado</TH>
                  <TH>Texto</TH>
                  <TH>Cuándo</TH>
                </TR>
              </THead>
              <TBody>
                {messages.data.map((m) => (
                  <TR key={m.id}>
                    <TD className="font-mono text-xs text-slate-500">{truncate(m.id, 8)}</TD>
                    <TD className="font-mono text-xs">{m.to}</TD>
                    <TD><Badge value={m.status} /></TD>
                    <TD className="max-w-xs truncate text-xs" title={m.text}>{m.text}</TD>
                    <TD className="text-xs text-slate-500" title={formatDate(m.created_at)}>
                      {formatRelative(m.created_at)}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </CardBody>
      </Card>
    </TenantPage>
  )
}

function QuotaCard({
  enabled, limit, sentToday, loading,
}: {
  enabled: boolean
  limit: number | null
  sentToday: number
  loading: boolean
}) {
  if (!enabled) {
    return (
      <Card>
        <CardHeader>
          <h2 className="text-sm font-semibold">Cuota diaria</h2>
        </CardHeader>
        <CardBody>
          <p className="text-sm text-ink-mute">
            Este cliente no tiene cuota diaria configurada (envíos ilimitados).
            Configurar una protege contra loops o claves filtradas.
          </p>
        </CardBody>
      </Card>
    )
  }
  if (loading || limit == null) {
    return (
      <Card>
        <CardHeader><h2 className="text-sm font-semibold">Cuota diaria</h2></CardHeader>
        <CardBody><Spinner /></CardBody>
      </Card>
    )
  }
  const pct = limit > 0 ? Math.min(100, (sentToday / limit) * 100) : 0
  const barColor =
    pct >= 100 ? 'bg-red-600' :
    pct >= 80 ? 'bg-amber-500' :
    'bg-emerald-500'
  const textColor =
    pct >= 100 ? 'text-red-700' :
    pct >= 80 ? 'text-amber-700' :
    'text-ink'
  return (
    <Card>
      <CardHeader>
        <h2 className="text-sm font-semibold">Cuota diaria · hoy</h2>
        <span className="text-xs text-ink-mute">resetea medianoche America/Santiago</span>
      </CardHeader>
      <CardBody>
        <div className="flex items-baseline justify-between">
          <div className={`text-3xl font-bold tabular ${textColor}`}>{sentToday}<span className="text-base text-ink-mute"> / {limit}</span></div>
          <div className={`text-sm font-semibold tabular ${textColor}`}>{pct.toFixed(1)}%</div>
        </div>
        <div className="mt-3 h-2 w-full overflow-hidden rounded-full bg-muted">
          <div className={`h-full transition-all ${barColor}`} style={{ width: `${pct}%` }} />
        </div>
        {pct >= 80 && pct < 100 && (
          <p className="mt-3 text-xs text-amber-700">
            Cerca del límite. Considera aumentar la cuota o investigar la causa del volumen.
          </p>
        )}
        {pct >= 100 && (
          <p className="mt-3 text-xs text-red-700">
            Límite alcanzado. Los próximos envíos reciben <code>429 daily_quota_exceeded</code>.
          </p>
        )}
      </CardBody>
    </Card>
  )
}

function AllowedSendersCard({ onChanged }: { onChanged: () => void }) {
  const { tenant } = useTenant()
  const qc = useQueryClient()
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState<string>(() => tenant.allowed_senders.join(', '))
  const [err, setErr] = useState<string | null>(null)

  // Sincroniza si el tenant cambia (e.g. tras refresh).
  useEffect(() => {
    if (!editing) setDraft(tenant.allowed_senders.join(', '))
  }, [tenant.allowed_senders, editing])

  const save = useMutation({
    mutationFn: async (senders: string[]) => {
      await api.put(`/admin/tenants/${tenant.id}/allowed-senders`, {
        allowed_senders: senders,
      })
    },
    onSuccess: () => {
      setEditing(false)
      setErr(null)
      qc.invalidateQueries({ queryKey: ['tenant', tenant.id] })
      onChanged()
    },
    onError: (e) => setErr(errorMessage(e)),
  })

  const onSave = () => {
    const list = draft
      .split(/[,\n]/)
      .map((s) => s.trim())
      .filter((s) => s.length > 0)
    save.mutate(list)
  }

  return (
    <Card>
      <CardHeader>
        <div>
          <h2 className="text-sm font-semibold">Sender IDs autorizados</h2>
          <p className="text-xs text-ink-mute">
            Si está vacío, se permite cualquier sender. Si tiene valores, los envíos
            con un <code>sender</code> distinto reciben <code>403 sender_not_allowed</code>.
          </p>
        </div>
        {!editing && (
          <Button variant="ghost" onClick={() => setEditing(true)}>Editar</Button>
        )}
      </CardHeader>
      <CardBody>
        {!editing ? (
          tenant.allowed_senders.length === 0 ? (
            <p className="text-sm text-ink-mute">Sin restricciones (cualquier sender permitido).</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {tenant.allowed_senders.map((s) => (
                <span key={s} className="inline-flex items-center rounded-full bg-muted px-3 py-1 font-mono text-xs text-ink">
                  {s}
                </span>
              ))}
            </div>
          )
        ) : (
          <div className="space-y-3">
            <Input
              label="Senders permitidos (separa con coma)"
              placeholder="Segtelco, AcmeAlertas"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              autoFocus
            />
            {err && <p className="text-sm text-red-600">{err}</p>}
            <div className="flex gap-2">
              <Button onClick={onSave} loading={save.isPending}>Guardar</Button>
              <Button
                variant="ghost"
                onClick={() => {
                  setEditing(false)
                  setDraft(tenant.allowed_senders.join(', '))
                  setErr(null)
                }}
              >
                Cancelar
              </Button>
            </div>
            <p className="text-xs text-ink-mute">
              Dejar vacío = sin restricción. Espacios y duplicados se limpian al guardar.
            </p>
          </div>
        )}
      </CardBody>
    </Card>
  )
}

function Kpi({
  label, value, subtitle, link,
}: {
  label: string
  value: React.ReactNode
  subtitle?: string
  link?: string
}) {
  const inner = (
    <div className="rounded-lg border border-slate-200 bg-white p-4 transition-colors hover:border-slate-300">
      <div className="text-xs font-medium uppercase tracking-wide text-slate-500">{label}</div>
      <div className="mt-1 text-2xl font-bold text-slate-900">{value}</div>
      {subtitle && <div className="mt-0.5 text-xs text-slate-500">{subtitle}</div>}
    </div>
  )
  return link ? (
    <Link to={link} relative="path">
      {inner}
    </Link>
  ) : (
    inner
  )
}
