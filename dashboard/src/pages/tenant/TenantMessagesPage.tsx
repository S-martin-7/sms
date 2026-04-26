import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { Message } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatDate, truncate } from '@/lib/format'

// Tenant-scoped messages list. Same shape as the global MessagesPage but
// `tenant_id` is locked to the current workspace.
const STATUSES: Array<{ value: string; label: string }> = [
  { value: 'queued', label: 'En cola' },
  { value: 'sending', label: 'Enviando' },
  { value: 'sent', label: 'Enviado' },
  { value: 'delivered', label: 'Entregado' },
  { value: 'undelivered', label: 'No entregado' },
  { value: 'rejected', label: 'Rechazado' },
  { value: 'failed', label: 'Fallido' },
]

interface MessagesResp {
  messages: Message[]
  next_cursor: string | null
}

export function TenantMessagesPage() {
  const { tenant } = useTenant()

  const [filters, setFilters] = useState({ status: '', recipient: '', client_ref: '' })
  const [pages, setPages] = useState<Message[][]>([])
  const [cursor, setCursor] = useState<string | null>(null)
  const [hasMore, setHasMore] = useState(false)
  const [opened, setOpened] = useState<Message | null>(null)

  const buildURL = (c: string | null) => {
    const sp = new URLSearchParams()
    sp.set('tenant_id', String(tenant.id))
    if (filters.status) sp.set('status', filters.status)
    if (filters.recipient) sp.set('recipient', filters.recipient)
    if (filters.client_ref) sp.set('client_ref', filters.client_ref)
    sp.set('limit', '25')
    if (c) sp.set('cursor', c)
    return `/admin/messages?${sp.toString()}`
  }

  const list = useQuery({
    queryKey: ['tenant', tenant.id, 'messages-list', filters],
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
    setPages((p) => [...p, data.messages])
    setCursor(data.next_cursor)
    setHasMore(!!data.next_cursor)
  }

  const all = pages.flat()

  return (
    <TenantPage title="Mensajes">
      <Card>
        <CardBody>
          <div className="grid gap-3 sm:grid-cols-3">
            <div className="flex flex-col gap-1">
              <label className="text-sm font-medium text-slate-700">Estado</label>
              <select
                className="rounded-md border border-slate-300 px-3 py-2 text-sm"
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
            <div className="flex justify-center p-10"><Spinner /></div>
          ) : list.error ? (
            <div className="px-4 py-6 text-sm text-red-600">{errorMessage(list.error)}</div>
          ) : (
            <>
              <Table>
                <THead>
                  <TR>
                    <TH>ID</TH>
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
                      <TD className="text-xs">
                        <span className="text-slate-500">{m.sender}</span>
                        <span className="px-1 text-slate-300">→</span>
                        <span className="font-mono">{m.to}</span>
                      </TD>
                      <TD><Badge value={m.status} /></TD>
                      <TD className="text-slate-500">{formatDate(m.created_at)}</TD>
                      <TD className="text-slate-500">{formatDate(m.final_at)}</TD>
                    </TR>
                  ))}
                </TBody>
              </Table>
              {!all.length && (
                <div className="px-4 py-10 text-center text-sm text-slate-500">
                  Este cliente todavía no tiene mensajes que coincidan con los filtros.
                </div>
              )}
              {hasMore && (
                <div className="flex justify-center border-t border-slate-100 p-3">
                  <Button variant="secondary" onClick={loadMore}>Cargar más</Button>
                </div>
              )}
            </>
          )}
        </CardBody>
      </Card>

      {opened && (
        <Modal open onClose={() => setOpened(null)} title="Detalle del mensaje" width="lg">
          <pre className="overflow-x-auto rounded-md bg-slate-900 p-3 text-xs text-slate-100">
            {JSON.stringify(opened, null, 2)}
          </pre>
        </Modal>
      )}
    </TenantPage>
  )
}
