import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey, InboundNumber, Tenant, WebhookDelivery, WebhookEndpoint } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { Tabs } from '@/components/ui/Tabs'
import { formatDate } from '@/lib/format'

export function TenantDetailPage() {
  const { id: idParam } = useParams<{ id: string }>()
  const tenantId = Number(idParam)
  const qc = useQueryClient()

  const tenant = useQuery({
    queryKey: ['tenant', tenantId],
    queryFn: async () => {
      const { data } = await api.get<Tenant>(`/admin/tenants/${tenantId}`)
      return data
    },
  })

  const setStatus = useMutation({
    mutationFn: async (action: 'suspend' | 'activate') => {
      await api.post(`/admin/tenants/${tenantId}/${action}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId] }),
  })

  const [tab, setTab] = useState<'keys' | 'webhooks' | 'deliveries' | 'inbound'>('keys')

  if (tenant.isLoading) {
    return (
      <div className="flex justify-center p-10">
        <Spinner />
      </div>
    )
  }
  if (tenant.error || !tenant.data) {
    return <div className="text-sm text-red-600">{errorMessage(tenant.error)}</div>
  }

  const t = tenant.data
  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between">
        <div>
          <Link to="/clientes" className="text-sm text-slate-500 hover:underline">
            ← Clientes
          </Link>
          <h1 className="mt-1 text-xl font-semibold text-slate-900">{t.name}</h1>
          <div className="mt-1 flex items-center gap-3 text-sm text-slate-500">
            <span>id={t.id}</span>
            <Badge value={t.status} />
            <span>creado {formatDate(t.created_at)}</span>
          </div>
        </div>
        <div>
          {t.status === 'active' ? (
            <Button
              variant="secondary"
              loading={setStatus.isPending}
              onClick={() => setStatus.mutate('suspend')}
            >
              Suspender
            </Button>
          ) : (
            <Button
              variant="secondary"
              loading={setStatus.isPending}
              onClick={() => setStatus.mutate('activate')}
            >
              Activar
            </Button>
          )}
        </div>
      </div>

      <Card>
        <CardHeader>
          <Tabs
            active={tab}
            onChange={(k) => setTab(k as typeof tab)}
            tabs={[
              { key: 'keys', label: 'Llaves API' },
              { key: 'webhooks', label: 'Webhooks' },
              { key: 'deliveries', label: 'Entregas' },
              { key: 'inbound', label: 'Números entrantes' },
            ]}
          />
          <span />
        </CardHeader>
        <CardBody className="p-0">
          {tab === 'keys' && <KeysTab tenantId={tenantId} />}
          {tab === 'webhooks' && <WebhooksTab tenantId={tenantId} />}
          {tab === 'deliveries' && <DeliveriesTab tenantId={tenantId} />}
          {tab === 'inbound' && <InboundTab tenantId={tenantId} />}
        </CardBody>
      </Card>
    </div>
  )
}

// ===== Llaves API =====

function KeysTab({ tenantId }: { tenantId: number }) {
  const qc = useQueryClient()
  const keys = useQuery({
    queryKey: ['tenant', tenantId, 'keys'],
    queryFn: async () => {
      const { data } = await api.get<{ keys: APIKey[] }>(`/admin/tenants/${tenantId}/api-keys`)
      return data.keys
    },
  })
  const revoke = useMutation({
    mutationFn: async (id: number) => {
      await api.post(`/admin/api-keys/${id}/revoke`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId, 'keys'] }),
  })

  const [issueOpen, setIssueOpen] = useState(false)
  const [justIssued, setJustIssued] = useState<APIKey | null>(null)

  return (
    <div>
      <div className="flex items-center justify-end border-b border-slate-100 px-4 py-2">
        <Button onClick={() => setIssueOpen(true)}>Emitir nueva llave</Button>
      </div>
      {keys.isLoading ? (
        <div className="flex justify-center p-10">
          <Spinner />
        </div>
      ) : !keys.data?.length ? (
        <div className="px-4 py-10 text-center text-sm text-slate-500">
          Aún no se ha emitido ninguna llave.
        </div>
      ) : (
        <Table>
          <THead>
            <TR>
              <TH>Prefijo</TH>
              <TH>Etiqueta</TH>
              <TH>Creada</TH>
              <TH>Último uso</TH>
              <TH>Estado</TH>
              <TH className="text-right">Acciones</TH>
            </TR>
          </THead>
          <TBody>
            {keys.data.map((k) => (
              <TR key={k.id}>
                <TD className="font-mono text-xs text-slate-700">{k.prefix}</TD>
                <TD>{k.name ?? '—'}</TD>
                <TD className="text-slate-500">{formatDate(k.created_at)}</TD>
                <TD className="text-slate-500">{formatDate(k.last_used_at)}</TD>
                <TD>{k.revoked_at ? <Badge value="revoked" /> : <Badge value="active" />}</TD>
                <TD className="text-right">
                  {!k.revoked_at && (
                    <Button
                      variant="danger"
                      loading={revoke.isPending && revoke.variables === k.id}
                      onClick={() => {
                        if (confirm(`¿Revocar la llave ${k.prefix}? Las apps que la usen dejarán de funcionar.`)) {
                          revoke.mutate(k.id)
                        }
                      }}
                    >
                      Revocar
                    </Button>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      {issueOpen && (
        <IssueKeyModal
          tenantId={tenantId}
          onClose={() => setIssueOpen(false)}
          onIssued={(k) => {
            setIssueOpen(false)
            setJustIssued(k)
            qc.invalidateQueries({ queryKey: ['tenant', tenantId, 'keys'] })
          }}
        />
      )}
      {justIssued && <RevealKeyModal apiKey={justIssued} onClose={() => setJustIssued(null)} />}
    </div>
  )
}

function IssueKeyModal({
  tenantId,
  onClose,
  onIssued,
}: {
  tenantId: number
  onClose: () => void
  onIssued: (k: APIKey) => void
}) {
  const [name, setName] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const issue = useMutation({
    mutationFn: async () => {
      const { data } = await api.post<APIKey>(`/admin/tenants/${tenantId}/api-keys`, { name })
      return data
    },
    onSuccess: onIssued,
    onError: (e) => setErr(errorMessage(e)),
  })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setErr(null)
    issue.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title="Emitir llave API"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={onSubmit} loading={issue.isPending}>
            Emitir
          </Button>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-4">
        <Input
          label="Etiqueta (opcional)"
          placeholder="servidor-prod"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
        )}
      </form>
    </Modal>
  )
}

function RevealKeyModal({ apiKey, onClose }: { apiKey: APIKey; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    if (!apiKey.token) return
    await navigator.clipboard.writeText(apiKey.token)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <Modal
      open
      onClose={onClose}
      title="Llave API emitida"
      width="lg"
      footer={<Button onClick={onClose}>Listo</Button>}
    >
      <div className="space-y-3">
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          Copia esta llave ahora. <strong>No se mostrará nuevamente.</strong>
        </div>
        <div className="flex items-center gap-2 rounded-md border border-slate-200 bg-slate-50 p-3 font-mono text-sm break-all">
          {apiKey.token}
        </div>
        <div className="flex justify-end">
          <Button variant="secondary" onClick={copy}>
            {copied ? 'Copiada ✓' : 'Copiar'}
          </Button>
        </div>
        <div className="text-xs text-slate-500">
          Prefijo <span className="font-mono">{apiKey.prefix}</span> · creada {formatDate(apiKey.created_at)}
        </div>
      </div>
    </Modal>
  )
}

// ===== Webhooks =====

const ALL_WEBHOOK_EVENTS = [
  { value: 'sms.delivered', label: 'SMS entregado' },
  { value: 'sms.undelivered', label: 'SMS no entregado' },
  { value: 'sms.rejected', label: 'SMS rechazado' },
  { value: 'sms.inbound', label: 'SMS entrante' },
]

function WebhooksTab({ tenantId }: { tenantId: number }) {
  const qc = useQueryClient()
  const eps = useQuery({
    queryKey: ['tenant', tenantId, 'webhooks'],
    queryFn: async () => {
      const { data } = await api.get<{ endpoints: WebhookEndpoint[] }>(
        `/admin/tenants/${tenantId}/webhooks`,
      )
      return data.endpoints
    },
  })

  const [creating, setCreating] = useState(false)
  const [justCreated, setJustCreated] = useState<WebhookEndpoint | null>(null)

  return (
    <div>
      <div className="flex items-center justify-end border-b border-slate-100 px-4 py-2">
        <Button onClick={() => setCreating(true)}>Crear webhook</Button>
      </div>
      {eps.isLoading ? (
        <div className="flex justify-center p-10">
          <Spinner />
        </div>
      ) : !eps.data?.length ? (
        <div className="px-4 py-10 text-center text-sm text-slate-500">
          No hay webhooks registrados.
        </div>
      ) : (
        <Table>
          <THead>
            <TR>
              <TH>ID</TH>
              <TH>URL</TH>
              <TH>Eventos</TH>
              <TH>Estado</TH>
              <TH>Creado</TH>
            </TR>
          </THead>
          <TBody>
            {eps.data.map((ep) => (
              <TR key={ep.id}>
                <TD className="font-mono text-xs text-slate-500">{ep.id}</TD>
                <TD className="break-all font-mono text-xs">{ep.url}</TD>
                <TD>
                  <div className="flex flex-wrap gap-1">
                    {ep.events.map((e) => (
                      <span key={e} className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700">
                        {e}
                      </span>
                    ))}
                  </div>
                </TD>
                <TD>
                  <Badge value={ep.active ? 'active' : 'suspended'} />
                </TD>
                <TD className="text-slate-500">{formatDate(ep.created_at)}</TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      {creating && (
        <CreateWebhookModal
          tenantApiKey={null}
          onClose={() => setCreating(false)}
          onCreated={(ep) => {
            setCreating(false)
            setJustCreated(ep)
            qc.invalidateQueries({ queryKey: ['tenant', tenantId, 'webhooks'] })
          }}
          tenantId={tenantId}
        />
      )}
      {justCreated && <RevealWebhookSecretModal endpoint={justCreated} onClose={() => setJustCreated(null)} />}
    </div>
  )
}

// CreateWebhookModal calls the tenant-scoped POST /v1/webhooks endpoint
// using a freshly issued admin-side API key. To keep admin flow self-
// contained, we issue a temporary key, create the webhook, then revoke
// the key in the same flow. This way the dashboard can register webhooks
// without exposing /admin/* CRUD for them (which the backend doesn't
// expose today).
function CreateWebhookModal({
  tenantId,
  onClose,
  onCreated,
}: {
  tenantId: number
  tenantApiKey: string | null
  onClose: () => void
  onCreated: (ep: WebhookEndpoint) => void
}) {
  const [url, setUrl] = useState('https://')
  const [events, setEvents] = useState<string[]>([
    'sms.delivered', 'sms.undelivered', 'sms.rejected', 'sms.inbound',
  ])
  const [err, setErr] = useState<string | null>(null)

  const create = useMutation({
    mutationFn: async () => {
      // 1. Issue a one-shot admin key for this tenant.
      const { data: key } = await api.post<APIKey>(
        `/admin/tenants/${tenantId}/api-keys`,
        { name: `dashboard-temp-webhook-${Date.now()}` },
      )
      try {
        // 2. Use it to create the webhook on the tenant API.
        const { data: ep } = await api.post<WebhookEndpoint>(
          '/v1/webhooks',
          { url, events },
          { headers: { 'X-API-Key': key.token } },
        )
        return ep
      } finally {
        // 3. Revoke the temp key so it never lives past this flow.
        await api.post(`/admin/api-keys/${key.id}/revoke`).catch(() => {})
      }
    },
    onSuccess: onCreated,
    onError: (e) => setErr(errorMessage(e)),
  })

  const toggle = (v: string) =>
    setEvents((prev) => (prev.includes(v) ? prev.filter((e) => e !== v) : [...prev, v]))

  return (
    <Modal
      open
      onClose={onClose}
      title="Crear webhook"
      width="lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>Cancelar</Button>
          <Button
            onClick={() => {
              setErr(null)
              create.mutate()
            }}
            loading={create.isPending}
            disabled={!url || events.length === 0}
          >
            Crear
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <Input
          label="URL del webhook"
          required
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://tucliente.com/webhooks/sms"
        />
        <div>
          <div className="mb-1 text-sm font-medium text-slate-700">Eventos suscritos</div>
          <div className="space-y-1">
            {ALL_WEBHOOK_EVENTS.map((evt) => (
              <label key={evt.value} className="flex items-center gap-2 text-sm text-slate-700">
                <input
                  type="checkbox"
                  checked={events.includes(evt.value)}
                  onChange={() => toggle(evt.value)}
                />
                <span className="font-mono text-xs text-slate-500">{evt.value}</span>
                <span>· {evt.label}</span>
              </label>
            ))}
          </div>
        </div>
        <div className="rounded-md border border-blue-200 bg-blue-50 p-3 text-xs text-blue-800">
          La URL debe ser HTTPS. Cada POST llevará una firma HMAC-SHA256 en el header
          <code className="mx-1 font-mono">X-Signature</code> que tu servidor debe validar contra el
          secreto compartido (te lo mostraremos una vez al crear).
        </div>
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
        )}
      </div>
    </Modal>
  )
}

function RevealWebhookSecretModal({ endpoint, onClose }: { endpoint: WebhookEndpoint; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    if (!endpoint.secret) return
    await navigator.clipboard.writeText(endpoint.secret)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <Modal
      open
      onClose={onClose}
      title="Webhook creado"
      width="lg"
      footer={<Button onClick={onClose}>Listo</Button>}
    >
      <div className="space-y-3">
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          Guarda este secreto ahora. <strong>No se mostrará nuevamente.</strong> Lo necesitas para validar la firma HMAC de cada webhook entrante.
        </div>
        <div className="space-y-1 text-xs text-slate-600">
          <div>URL: <span className="font-mono">{endpoint.url}</span></div>
          <div>ID: <span className="font-mono">{endpoint.id}</span></div>
          <div>Eventos: {endpoint.events.join(', ')}</div>
        </div>
        <div className="flex items-center gap-2 rounded-md border border-slate-200 bg-slate-50 p-3 font-mono text-sm break-all">
          {endpoint.secret}
        </div>
        <div className="flex justify-end">
          <Button variant="secondary" onClick={copy}>
            {copied ? 'Copiado ✓' : 'Copiar secreto'}
          </Button>
        </div>
      </div>
    </Modal>
  )
}

// ===== Entregas =====

function DeliveriesTab({ tenantId }: { tenantId: number }) {
  const qc = useQueryClient()
  const dels = useQuery({
    queryKey: ['tenant', tenantId, 'deliveries'],
    queryFn: async () => {
      const { data } = await api.get<{ deliveries: WebhookDelivery[] }>(
        `/admin/tenants/${tenantId}/webhook-deliveries?limit=50`,
      )
      return data.deliveries
    },
  })
  const retry = useMutation({
    mutationFn: async (id: number) => {
      await api.post(`/admin/webhook-deliveries/${id}/retry`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId, 'deliveries'] }),
  })

  if (dels.isLoading) {
    return (
      <div className="flex justify-center p-10">
        <Spinner />
      </div>
    )
  }
  if (!dels.data?.length) {
    return <div className="px-4 py-10 text-center text-sm text-slate-500">No hay entregas todavía.</div>
  }
  return (
    <Table>
      <THead>
        <TR>
          <TH>ID</TH>
          <TH>Evento</TH>
          <TH>Estado</TH>
          <TH>Intentos</TH>
          <TH>Último HTTP</TH>
          <TH>Último error</TH>
          <TH>Creado</TH>
          <TH className="text-right">Acciones</TH>
        </TR>
      </THead>
      <TBody>
        {dels.data.map((d) => (
          <TR key={d.id}>
            <TD className="font-mono text-xs text-slate-500">{d.id}</TD>
            <TD className="text-xs">{d.event_type}</TD>
            <TD>
              <Badge value={d.status} />
            </TD>
            <TD>{d.attempts}</TD>
            <TD>{d.last_status ?? '—'}</TD>
            <TD className="max-w-xs truncate text-xs text-red-600" title={d.last_error ?? undefined}>
              {d.last_error ?? '—'}
            </TD>
            <TD className="text-slate-500">{formatDate(d.created_at)}</TD>
            <TD className="text-right">
              <Button
                variant="secondary"
                loading={retry.isPending && retry.variables === d.id}
                onClick={() => retry.mutate(d.id)}
              >
                Reintentar
              </Button>
            </TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}

// ===== Números entrantes =====

function InboundTab({ tenantId }: { tenantId: number }) {
  const all = useQuery({
    queryKey: ['inbound-numbers', 'all'],
    queryFn: async () => {
      const { data } = await api.get<{ numbers: InboundNumber[] }>('/admin/inbound-numbers')
      return data.numbers
    },
  })
  if (all.isLoading) {
    return (
      <div className="flex justify-center p-10">
        <Spinner />
      </div>
    )
  }
  const mine = (all.data ?? []).filter((n) => n.tenant_id === tenantId)
  if (!mine.length) {
    return (
      <div className="px-4 py-10 text-center text-sm text-slate-500">
        Este cliente no tiene números entrantes asignados.
      </div>
    )
  }
  return (
    <Table>
      <THead>
        <TR>
          <TH>MSISDN</TH>
          <TH>Etiqueta</TH>
          <TH>Asignado</TH>
        </TR>
      </THead>
      <TBody>
        {mine.map((n) => (
          <TR key={n.msisdn}>
            <TD className="font-mono">{n.msisdn}</TD>
            <TD>{n.label || '—'}</TD>
            <TD className="text-slate-500">{formatDate(n.created_at)}</TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}
