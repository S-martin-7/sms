import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey, Tenant } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'

interface BulkRow {
  index: number
  id?: string
  to: string
  status: string
  client_ref?: string
  error?: string
  error_code?: string
}
interface BulkResp {
  batch_id: string
  accepted: number
  rejected: number
  messages: BulkRow[]
}

export function SendSMSPage() {
  const tenants = useQuery({
    queryKey: ['tenants'],
    queryFn: async () => {
      const { data } = await api.get<{ tenants: Tenant[] }>('/admin/tenants')
      return data.tenants
    },
  })

  const [tenantId, setTenantId] = useState<string>('')
  const [sender, setSender] = useState('')
  // recipients separados por coma o salto de línea
  const [recipientsText, setRecipientsText] = useState('')
  const [text, setText] = useState('')
  const [result, setResult] = useState<BulkResp | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const recipients = useMemo(
    () =>
      recipientsText
        .split(/[\s,;]+/)
        .map((s) => s.trim())
        .filter(Boolean),
    [recipientsText],
  )

  const send = useMutation({
    mutationFn: async () => {
      // 1. Issue a one-shot admin key for the chosen tenant.
      const { data: key } = await api.post<APIKey>(
        `/admin/tenants/${tenantId}/api-keys`,
        { name: `dashboard-temp-send-${Date.now()}` },
      )
      try {
        // 2. Use it to call /v1/sms/bulk for partial-accept semantics
        //    even when only one recipient is specified.
        const { data } = await api.post<BulkResp>(
          '/v1/sms/bulk',
          {
            default_sender: sender,
            messages: recipients.map((to) => ({ to, text })),
          },
          { headers: { 'X-API-Key': key.token } },
        )
        return data
      } finally {
        // 3. Revoke immediately so the key can't be reused.
        await api.post(`/admin/api-keys/${key.id}/revoke`).catch(() => {})
      }
    },
    onSuccess: setResult,
    onError: (e) => setErr(errorMessage(e)),
  })

  const canSend =
    tenantId !== '' && sender.trim() !== '' && recipients.length > 0 && text.trim() !== ''

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <h1 className="text-base font-semibold">Enviar SMS</h1>
        </CardHeader>
        <CardBody className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-1">
              <label className="text-sm font-medium text-slate-700">Cliente</label>
              {tenants.isLoading ? (
                <Spinner />
              ) : (
                <select
                  className="rounded-md border border-slate-300 px-3 py-2 text-sm"
                  value={tenantId}
                  onChange={(e) => setTenantId(e.target.value)}
                >
                  <option value="">— elige un cliente —</option>
                  {tenants.data
                    ?.filter((t) => t.status === 'active')
                    .map((t) => (
                      <option key={t.id} value={t.id}>
                        {t.name}
                      </option>
                    ))}
                </select>
              )}
              <span className="text-xs text-slate-500">
                Solo se listan clientes activos. Se emite y revoca una llave temporal por envío.
              </span>
            </div>
            <Input
              label="Remitente (sender)"
              required
              value={sender}
              onChange={(e) => setSender(e.target.value)}
              placeholder="MiMarca o un número"
            />
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-slate-700">
              Destinatarios — separados por coma, espacio o nueva línea
            </label>
            <textarea
              rows={3}
              className="rounded-md border border-slate-300 px-3 py-2 font-mono text-sm focus:outline-none focus:ring-2 focus:ring-slate-900/20"
              value={recipientsText}
              onChange={(e) => setRecipientsText(e.target.value)}
              placeholder="569XXXXXXXX, 569XXXXXXXX"
            />
            <span className="text-xs text-slate-500">
              {recipients.length} destinatario{recipients.length === 1 ? '' : 's'} detectado{recipients.length === 1 ? '' : 's'}.
            </span>
          </div>

          <div className="flex flex-col gap-1">
            <label className="text-sm font-medium text-slate-700">Texto del mensaje</label>
            <textarea
              rows={3}
              className="rounded-md border border-slate-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-slate-900/20"
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="Hola, este es un mensaje de prueba."
              maxLength={1600}
            />
            <span className="text-xs text-slate-500">
              {text.length} caracteres. Si incluyes emojis o tildes el SMS se codifica como UCS-2 y cuenta como ~70 chars/parte.
            </span>
          </div>

          {err && (
            <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
          )}

          <div className="flex justify-end">
            <Button
              onClick={() => {
                setErr(null)
                setResult(null)
                send.mutate()
              }}
              loading={send.isPending}
              disabled={!canSend}
            >
              Enviar {recipients.length > 0 ? `(${recipients.length})` : ''}
            </Button>
          </div>
        </CardBody>
      </Card>

      {result && (
        <Card>
          <CardHeader>
            <div>
              <h2 className="text-sm font-semibold">Resultado del envío</h2>
              <div className="text-xs text-slate-500">
                lote <span className="font-mono">{result.batch_id}</span> — aceptados:{' '}
                <strong className="text-emerald-700">{result.accepted}</strong> · rechazados:{' '}
                <strong className="text-red-700">{result.rejected}</strong>
              </div>
            </div>
          </CardHeader>
          <CardBody className="p-0">
            <Table>
              <THead>
                <TR>
                  <TH>#</TH>
                  <TH>Destino</TH>
                  <TH>Estado</TH>
                  <TH>ID</TH>
                  <TH>Error</TH>
                </TR>
              </THead>
              <TBody>
                {result.messages.map((m) => (
                  <TR key={m.index}>
                    <TD className="text-xs">{m.index + 1}</TD>
                    <TD className="font-mono text-xs">{m.to}</TD>
                    <TD><Badge value={m.status} /></TD>
                    <TD className="font-mono text-xs text-slate-500">{m.id ?? '—'}</TD>
                    <TD className="text-xs text-red-600">
                      {m.error_code ? `[${m.error_code}] ` : ''}{m.error ?? '—'}
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
            <div className="border-t border-slate-100 px-4 py-2 text-xs text-slate-500">
              Tip: revisa el estado de cada mensaje en{' '}
              <a href="#/mensajes" className="text-slate-700 underline">
                Mensajes
              </a>{' '}
              para ver cuándo entrega Horisen.
            </div>
          </CardBody>
        </Card>
      )}
    </div>
  )
}
