import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, errorMessage } from '@/api/client'
import type { Message, Tenant } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { MessageTimeline } from '@/components/MessageTimeline'
import { formatDate, truncate } from '@/lib/format'

interface Filters {
  tenant_id: string
  status: string
  recipient: string
  client_ref: string
}

interface MessagesResp {
  messages: Message[]
  next_cursor: string | null
}

const STATUSES: Array<{ value: string; label: string }> = [
  { value: 'queued', label: 'En cola' },
  { value: 'sending', label: 'Enviando' },
  { value: 'sent', label: 'Enviado' },
  { value: 'delivered', label: 'Entregado' },
  { value: 'undelivered', label: 'No entregado' },
  { value: 'rejected', label: 'Rechazado' },
  { value: 'failed', label: 'Fallido' },
]

export function MessagesPage() {
  const tenants = useQuery({
    queryKey: ['tenants'],
    queryFn: async () => {
      const { data } = await api.get<{ tenants: Tenant[] }>('/admin/tenants')
      return data.tenants
    },
  })

  const [filters, setFilters] = useState<Filters>({
    tenant_id: '',
    status: '',
    recipient: '',
    client_ref: '',
  })
  const [pages, setPages] = useState<Message[][]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(false)
  const [opened, setOpened] = useState<Message | null>(null)

  const buildURL = (c: string | null) => {
    const sp = new URLSearchParams()
    if (filters.tenant_id) sp.set('tenant_id', filters.tenant_id)
    if (filters.status) sp.set('status', filters.status)
    if (filters.recipient) sp.set('recipient', filters.recipient)
    if (filters.client_ref) sp.set('client_ref', filters.client_ref)
    sp.set('limit', '25')
    if (c) sp.set('cursor', c)
    return `/admin/messages?${sp.toString()}`
  }

  const list = useQuery({
    queryKey: ['admin', 'messages', filters],
    queryFn: async () => {
      const { data } = await api.get<MessagesResp>(buildURL(null))
      setPages([data.messages])
      setCursor(data.next_cursor)
      setHasMore(!!data.next_cursor)
      return data
    },
  })

  const loadMore = async () => {
    if (!cursor) return
    const { data } = await api.get<MessagesResp>(buildURL(cursor))
    setPages((prev) => [...prev, data.messages])
    setCursor(data.next_cursor)
    setHasMore(!!data.next_cursor)
  }

  const all = pages.flat()
  const tenantName = (id: number) => tenants.data?.find((t) => t.id === id)?.name ?? `id=${id}`

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <h1 className="text-base font-semibold">Mensajes</h1>
        </CardHeader>
        <CardBody>
          <div className="grid gap-3 sm:grid-cols-4">
            <div className="flex flex-col gap-1">
              <label className="text-sm font-medium text-slate-700">Cliente</label>
              <select
                className="rounded-md border border-slate-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-slate-900/20"
                value={filters.tenant_id}
                onChange={(e) => setFilters({ ...filters, tenant_id: e.target.value })}
              >
                <option value="">— todos —</option>
                {tenants.data?.map((t) => (
                  <option key={t.id} value={t.id}>{t.name}</option>
                ))}
              </select>
            </div>
            <div className="flex flex-col gap-1">
              <label className="text-sm font-medium text-slate-700">Estado</label>
              <select
                className="rounded-md border border-slate-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-slate-900/20"
                value={filters.status}
                onChange={(e) => setFilters({ ...filters, status: e.target.value })}
              >
                <option value="">— todos —</option>
                {STATUSES.map((s) => (
                  <option key={s.value} value={s.value}>{s.label}</option>
                ))}
              </select>
            </div>
            <Input
              label="Destinatario"
              value={filters.recipient}
              onChange={(e) => setFilters({ ...filters, recipient: e.target.value })}
              placeholder="569..."
            />
            <Input
              label="Referencia cliente"
              value={filters.client_ref}
              onChange={(e) => setFilters({ ...filters, client_ref: e.target.value })}
            />
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardBody className="p-0">
          {list.isLoading ? (
            <div className="flex justify-center p-10">
              <Spinner />
            </div>
          ) : list.error ? (
            <div className="px-4 py-6 text-sm text-red-600">{errorMessage(list.error)}</div>
          ) : (
            <>
              <Table>
                <THead>
                  <TR>
                    <TH>ID</TH>
                    <TH>Cliente</TH>
                    <TH>Remitente → Destino</TH>
                    <TH>Estado</TH>
                    <TH>Creado</TH>
                    <TH>Final</TH>
                  </TR>
                </THead>
                <TBody>
                  {all.map((m) => (
                    <TR key={m.id} onClick={() => setOpened(m)}>
                      <TD className="font-mono text-xs text-slate-500">{truncate(m.id, 8)}</TD>
                      <TD className="text-xs">{tenantName(m.tenant_id)}</TD>
                      <TD className="text-xs">
                        <span className="text-slate-500">{m.sender}</span>
                        <span className="px-1 text-slate-300">→</span>
                        <span className="font-mono">{m.to}</span>
                      </TD>
                      <TD>
                        <Badge value={m.status} />
                      </TD>
                      <TD className="text-slate-500">{formatDate(m.created_at)}</TD>
                      <TD className="text-slate-500">{formatDate(m.final_at)}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
              {!all.length && (
                <div className="px-4 py-10 text-center text-sm text-slate-500">
                  Sin mensajes que coincidan con los filtros.
                </div>
              )}
              {hasMore && (
                <div className="flex justify-center border-t border-slate-100 p-3">
                  <Button variant="secondary" onClick={loadMore}>
                    Cargar más
                  </Button>
                </div>
              )}
            </>
          )}
        </CardBody>
      </Card>

      {opened && <MessageDetailModal msg={opened} tenantName={tenantName(opened.tenant_id)} onClose={() => setOpened(null)} />}
    </div>
  )
}

function MessageDetailModal({
  msg,
  tenantName,
  onClose,
}: {
  msg: Message
  tenantName: string
  onClose: () => void
}) {
  return (
    <Modal open onClose={onClose} title="Detalle del mensaje" width="lg">
      <div className="space-y-6">
        {/* Courier-tracking timeline at the top — most important visual */}
        <MessageTimeline msg={msg} />
        <DetailGrid msg={msg} tenantName={tenantName} />
        <div>
          <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-ink-mute">Texto enviado</div>
          <div className="whitespace-pre-wrap rounded-md border border-border bg-canvas p-3 text-sm">
            {msg.text}
          </div>
        </div>
      </div>
    </Modal>
  )
}

function DetailGrid({ msg, tenantName }: { msg: Message; tenantName: string }) {
  const items: Array<[string, React.ReactNode]> = [
    ['ID', <span key="id" className="font-mono text-xs">{msg.id}</span>],
    ['Cliente', <Link key="t" to={`/clientes/${msg.tenant_id}`} className="hover:underline">{tenantName}</Link>],
    ['Remitente', msg.sender],
    ['Destino', <span key="to" className="font-mono">{msg.to}</span>],
    ['Estado', <Badge key="s" value={msg.status} />],
    ['Codificación', `${msg.dcs} · ${msg.num_parts} parte${msg.num_parts === 1 ? '' : 's'}`],
    ['Intentos', msg.attempts],
    ['Referencia cliente', msg.client_ref ?? '—'],
    ['Horisen msgId', msg.horisen_msg_id ? <span key="h" className="font-mono text-xs">{msg.horisen_msg_id}</span> : '—'],
  ]
  return (
    <div className="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
      {items.map(([k, v]) => (
        <div key={k} className="flex flex-col">
          <span className="text-xs font-medium uppercase tracking-wide text-slate-500">{k}</span>
          <span className="text-slate-900">{v}</span>
        </div>
      ))}
    </div>
  )
}

// Timeline reemplazado por MessageTimeline (componente compartido).

