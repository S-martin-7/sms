import { useMemo, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'

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

// Send-as-this-tenant. Mirror of the global SendSMSPage but the tenant is fixed
// from the workspace context, so the form skips the dropdown.
export function TenantSendPage() {
  const { tenant } = useTenant()

  const [sender, setSender] = useState('')
  const [recipientsText, setRecipientsText] = useState('')
  const [text, setText] = useState('')
  const [result, setResult] = useState<BulkResp | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const recipients = useMemo(
    () => recipientsText.split(/[\s,;]+/).map((s) => s.trim()).filter(Boolean),
    [recipientsText],
  )

  const send = useMutation({
    mutationFn: async () => {
      // Same one-shot key dance as the global Send page so the dashboard
      // never holds long-lived credentials.
      const { data: key } = await api.post<APIKey>(
        `/admin/tenants/${tenant.id}/api-keys`,
        { name: `dashboard-temp-send-${Date.now()}` },
      )
      try {
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
        await api.post(`/admin/api-keys/${key.id}/revoke`).catch(() => {})
      }
    },
    onSuccess: setResult,
    onError: (e) => setErr(errorMessage(e)),
  })

  const canSend =
    sender.trim() !== '' && recipients.length > 0 && text.trim() !== '' && tenant.status === 'active'

  return (
    <TenantPage title="Enviar SMS">
      <Card>
        <CardBody className="space-y-4">
          {tenant.status !== 'active' && (
            <div className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
              El cliente está suspendido. Reactívalo para poder enviar mensajes.
            </div>
          )}
          <Input
            label="Remitente (sender)"
            required
            value={sender}
            onChange={(e) => setSender(e.target.value)}
            placeholder="MiMarca o un número"
          />

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
              Enviar como {tenant.name} {recipients.length > 0 ? `(${recipients.length})` : ''}
            </Button>
          </div>
        </CardBody>
      </Card>

      {result && (
        <Card>
          <CardBody className="p-0">
            <div className="border-b border-slate-200 px-4 py-3 text-sm">
              Lote <span className="font-mono">{result.batch_id}</span> — aceptados:{' '}
              <strong className="text-emerald-700">{result.accepted}</strong> · rechazados:{' '}
              <strong className="text-red-700">{result.rejected}</strong>
            </div>
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
          </CardBody>
        </Card>
      )}
    </TenantPage>
  )
}
