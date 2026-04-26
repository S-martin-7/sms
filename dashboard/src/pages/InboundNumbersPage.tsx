import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { InboundNumber, Tenant } from '@/api/types'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { formatDate } from '@/lib/format'

export function InboundNumbersPage() {
  const qc = useQueryClient()
  const list = useQuery({
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

  const [adding, setAdding] = useState(false)

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <h1 className="text-base font-semibold">Números entrantes</h1>
          <Button onClick={() => setAdding(true)}>Asignar número</Button>
        </CardHeader>
        <CardBody className="p-0">
          {list.isLoading ? (
            <div className="flex justify-center p-10">
              <Spinner />
            </div>
          ) : !list.data?.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              No hay números entrantes asignados todavía.
            </div>
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>MSISDN</TH>
                  <TH>Cliente</TH>
                  <TH>Etiqueta</TH>
                  <TH>Asignado</TH>
                  <TH className="text-right">Acciones</TH>
                </TR>
              </THead>
              <TBody>
                {list.data.map((n) => (
                  <TR key={n.msisdn}>
                    <TD className="font-mono">{n.msisdn}</TD>
                    <TD className="font-mono text-xs">{n.tenant_id}</TD>
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

      {adding && <AssignModal onClose={() => setAdding(false)} />}
    </div>
  )
}

function AssignModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [msisdn, setMsisdn] = useState('')
  const [tenantId, setTenantId] = useState('')
  const [label, setLabel] = useState('')
  const [err, setErr] = useState<string | null>(null)

  // Fetch tenants so the operator picks from a dropdown rather than
  // remembering ids.
  const tenants = useQuery({
    queryKey: ['tenants'],
    queryFn: async () => {
      const { data } = await api.get<{ tenants: Tenant[] }>('/admin/tenants')
      return data.tenants
    },
  })

  const assign = useMutation({
    mutationFn: async () => {
      await api.post('/admin/inbound-numbers', {
        msisdn,
        tenant_id: Number(tenantId),
        label: label || undefined,
      })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['inbound-numbers', 'all'] })
      onClose()
    },
    onError: (e) => setErr(errorMessage(e)),
  })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setErr(null)
    assign.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title="Asignar número entrante"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={onSubmit} loading={assign.isPending} disabled={!msisdn || !tenantId}>
            Asignar
          </Button>
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
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-slate-700">Cliente</label>
          <select
            required
            className="rounded-md border border-slate-300 px-3 py-2 text-sm"
            value={tenantId}
            onChange={(e) => setTenantId(e.target.value)}
          >
            <option value="">— elige un cliente —</option>
            {tenants.data?.map((t) => (
              <option key={t.id} value={t.id}>
                {t.name} (id={t.id})
              </option>
            ))}
          </select>
        </div>
        <Input
          label="Etiqueta (opcional)"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          placeholder="código corto marketing"
        />
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
        )}
      </form>
    </Modal>
  )
}
