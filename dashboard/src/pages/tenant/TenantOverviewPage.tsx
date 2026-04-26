import { Link } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey, InboundNumber, Message, WebhookEndpoint } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatDate, formatRelative, truncate } from '@/lib/format'

// Tenant overview — KPIs + ultimos 5 mensajes + estado de claves/webhooks/numeros.
// Reusa los endpoints que ya tienen filtro per-tenant en el admin API.
export function TenantOverviewPage() {
  const { tenant } = useTenant()

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
