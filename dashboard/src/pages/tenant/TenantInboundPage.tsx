import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { InboundNumber } from '@/api/types'
import { Button } from '@/components/ui/Button'
import { Card, CardBody } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatDate } from '@/lib/format'

export function TenantInboundPage() {
  const { tenant } = useTenant()
  const qc = useQueryClient()
  const all = useQuery({
    queryKey: ['inbound-numbers', 'all'],
    queryFn: async () => {
      const { data } = await api.get<{ numbers: InboundNumber[] }>('/admin/inbound-numbers')
      return data.numbers
    },
  })
  const remove = useMutation({
    mutationFn: async (msisdn: string) => {
      await api.delete(`/admin/inbound-numbers/${encodeURIComponent(msisdn)}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['inbound-numbers', 'all'] }),
  })

  const mine = (all.data ?? []).filter((n) => n.tenant_id === tenant.id)
  const [adding, setAdding] = useState(false)

  return (
    <TenantPage
      title="Números entrantes"
      action={<Button onClick={() => setAdding(true)}>Asignar número</Button>}
    >
      <Card>
        <CardBody className="p-0">
          {all.isLoading ? (
            <div className="flex justify-center p-10"><Spinner /></div>
          ) : !mine.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              Este cliente no tiene números entrantes asignados todavía.
            </div>
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>MSISDN</TH>
                  <TH>Etiqueta</TH>
                  <TH>Asignado</TH>
                  <TH className="text-right">Acciones</TH>
                </TR>
              </THead>
              <TBody>
                {mine.map((n) => (
                  <TR key={n.msisdn}>
                    <TD className="font-mono">{n.msisdn}</TD>
                    <TD>{n.label || '—'}</TD>
                    <TD className="text-slate-500">{formatDate(n.created_at)}</TD>
                    <TD className="text-right">
                      <Button
                        variant="danger"
                        loading={remove.isPending && remove.variables === n.msisdn}
                        onClick={() => {
                          if (confirm(`¿Quitar la asignación del número ${n.msisdn}?`)) {
                            remove.mutate(n.msisdn)
                          }
                        }}
                      >
                        Quitar
                      </Button>
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </CardBody>
      </Card>

      {adding && (
        <AssignModal
          tenantId={tenant.id}
          onClose={() => setAdding(false)}
        />
      )}
    </TenantPage>
  )
}

function AssignModal({ tenantId, onClose }: { tenantId: number; onClose: () => void }) {
  const qc = useQueryClient()
  const [msisdn, setMsisdn] = useState('')
  const [label, setLabel] = useState('')
  const [err, setErr] = useState<string | null>(null)

  const assign = useMutation({
    mutationFn: async () => {
      await api.post('/admin/inbound-numbers', {
        msisdn,
        tenant_id: tenantId,
        label: label || undefined,
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['inbound-numbers', 'all'] })
      onClose()
    },
    onError: (e) => setErr(errorMessage(e)),
  })

  const onSubmit = (e: FormEvent) => { e.preventDefault(); setErr(null); assign.mutate() }

  return (
    <Modal
      open
      onClose={onClose}
      title="Asignar número entrante"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>Cancelar</Button>
          <Button onClick={onSubmit} loading={assign.isPending} disabled={!msisdn}>Asignar</Button>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-4">
        <Input
          label="MSISDN (número completo en formato internacional)"
          required
          value={msisdn}
          onChange={(e) => setMsisdn(e.target.value)}
          placeholder="569XXXXXXXX"
        />
        <Input
          label="Etiqueta (opcional)"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          placeholder="código corto marketing"
        />
        {err && <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>}
      </form>
    </Modal>
  )
}
