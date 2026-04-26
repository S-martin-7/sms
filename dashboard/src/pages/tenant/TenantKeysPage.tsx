import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatDate } from '@/lib/format'

export function TenantKeysPage() {
  const { tenant } = useTenant()
  const qc = useQueryClient()
  const keys = useQuery({
    queryKey: ['tenant', tenant.id, 'keys'],
    queryFn: async () => {
      const { data } = await api.get<{ keys: APIKey[] }>(`/admin/tenants/${tenant.id}/api-keys`)
      return data.keys
    },
  })
  const revoke = useMutation({
    mutationFn: async (id: number) => { await api.post(`/admin/api-keys/${id}/revoke`) },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'keys'] }),
  })

  const [issueOpen, setIssueOpen] = useState(false)
  const [justIssued, setJustIssued] = useState<APIKey | null>(null)

  return (
    <TenantPage
      title="Llaves API"
      action={<Button onClick={() => setIssueOpen(true)}>Emitir nueva llave</Button>}
    >
      <Card>
        <CardBody className="p-0">
          {keys.isLoading ? (
            <div className="flex justify-center p-10"><Spinner /></div>
          ) : !keys.data?.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              Este cliente aún no tiene llaves emitidas.
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
        </CardBody>
      </Card>

      {issueOpen && (
        <IssueKeyModal
          tenantId={tenant.id}
          onClose={() => setIssueOpen(false)}
          onIssued={(k) => {
            setIssueOpen(false)
            setJustIssued(k)
            qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'keys'] })
          }}
        />
      )}
      {justIssued && <RevealKeyModal apiKey={justIssued} onClose={() => setJustIssued(null)} />}
    </TenantPage>
  )
}

function IssueKeyModal({
  tenantId, onClose, onIssued,
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
  const onSubmit = (e: FormEvent) => { e.preventDefault(); setErr(null); issue.mutate() }
  return (
    <Modal
      open
      onClose={onClose}
      title="Emitir llave API"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>Cancelar</Button>
          <Button onClick={onSubmit} loading={issue.isPending}>Emitir</Button>
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
        {err && <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}
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
    <Modal open onClose={onClose} title="Llave API emitida" width="lg" footer={<Button onClick={onClose}>Listo</Button>}>
      <div className="space-y-3">
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          Copia esta llave ahora. <strong>No se mostrará nuevamente.</strong>
        </div>
        <div className="flex items-center gap-2 rounded-md border border-slate-200 bg-slate-50 p-3 font-mono text-sm break-all">
          {apiKey.token}
        </div>
        <div className="flex justify-end">
          <Button variant="secondary" onClick={copy}>{copied ? 'Copiada ✓' : 'Copiar'}</Button>
        </div>
        <div className="text-xs text-slate-500">
          Prefijo <span className="font-mono">{apiKey.prefix}</span> · creada {formatDate(apiKey.created_at)}
        </div>
      </div>
    </Modal>
  )
}
