import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey, WebhookEndpoint } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatDate } from '@/lib/format'

const ALL_WEBHOOK_EVENTS = [
  { value: 'sms.delivered', label: 'SMS entregado' },
  { value: 'sms.undelivered', label: 'SMS no entregado' },
  { value: 'sms.rejected', label: 'SMS rechazado' },
  { value: 'sms.inbound', label: 'SMS entrante' },
]

export function TenantWebhooksPage() {
  const { tenant } = useTenant()
  const qc = useQueryClient()
  const eps = useQuery({
    queryKey: ['tenant', tenant.id, 'webhooks'],
    queryFn: async () => {
      const { data } = await api.get<{ endpoints: WebhookEndpoint[] }>(
        `/admin/tenants/${tenant.id}/webhooks`,
      )
      return data.endpoints
    },
  })

  const [creating, setCreating] = useState(false)
  const [justCreated, setJustCreated] = useState<WebhookEndpoint | null>(null)

  return (
    <TenantPage
      title="Webhooks"
      action={<Button onClick={() => setCreating(true)}>Crear webhook</Button>}
    >
      <Card>
        <CardBody className="p-0">
          {eps.isLoading ? (
            <div className="flex justify-center p-10"><Spinner /></div>
          ) : !eps.data?.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              Sin webhooks registrados. Crea uno para que tu sistema reciba notificaciones de cada SMS.
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
                          <span key={e} className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700">{e}</span>
                        ))}
                      </div>
                    </TD>
                    <TD><Badge value={ep.active ? 'active' : 'suspended'} /></TD>
                    <TD className="text-slate-500">{formatDate(ep.created_at)}</TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </CardBody>
      </Card>

      {creating && (
        <CreateWebhookModal
          tenantId={tenant.id}
          onClose={() => setCreating(false)}
          onCreated={(ep) => {
            setCreating(false)
            setJustCreated(ep)
            qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'webhooks'] })
          }}
        />
      )}
      {justCreated && <RevealSecretModal endpoint={justCreated} onClose={() => setJustCreated(null)} />}
    </TenantPage>
  )
}

function CreateWebhookModal({
  tenantId, onClose, onCreated,
}: {
  tenantId: number
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
      // Misma técnica que TenantSendPage: emitir llave temporal, usarla,
      // revocarla. El admin nunca expone llaves de larga vida al frontend.
      const { data: key } = await api.post<APIKey>(
        `/admin/tenants/${tenantId}/api-keys`,
        { name: `dashboard-temp-webhook-${Date.now()}` },
      )
      try {
        const { data: ep } = await api.post<WebhookEndpoint>(
          '/v1/webhooks',
          { url, events },
          { headers: { 'X-API-Key': key.token } },
        )
        return ep
      } finally {
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
            onClick={() => { setErr(null); create.mutate() }}
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
          placeholder="https://tu-app.com/webhooks/sms"
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
          <code className="mx-1 font-mono">X-Signature</code> que tu servidor debe validar contra el secreto.
        </div>
        {err && <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}
      </div>
    </Modal>
  )
}

function RevealSecretModal({ endpoint, onClose }: { endpoint: WebhookEndpoint; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    if (!endpoint.secret) return
    await navigator.clipboard.writeText(endpoint.secret)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }
  return (
    <Modal open onClose={onClose} title="Webhook creado" width="lg" footer={<Button onClick={onClose}>Listo</Button>}>
      <div className="space-y-3">
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          Guarda este secreto ahora. <strong>No se mostrará nuevamente.</strong>
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
          <Button variant="secondary" onClick={copy}>{copied ? 'Copiado ✓' : 'Copiar secreto'}</Button>
        </div>
      </div>
    </Modal>
  )
}
