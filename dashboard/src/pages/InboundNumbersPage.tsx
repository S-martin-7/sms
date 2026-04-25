import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { InboundNumber } from '@/api/types'
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
          <h1 className="text-base font-semibold">Inbound numbers</h1>
          <Button onClick={() => setAdding(true)}>Assign number</Button>
        </CardHeader>
        <CardBody className="p-0">
          {list.isLoading ? (
            <div className="flex justify-center p-10">
              <Spinner />
            </div>
          ) : !list.data?.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              No inbound numbers assigned.
            </div>
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>MSISDN</TH>
                  <TH>Tenant</TH>
                  <TH>Label</TH>
                  <TH>Created</TH>
                  <TH className="text-right">Actions</TH>
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
                        onClick={() => remove.mutate(n.msisdn)}
                      >
                        Unassign
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
      title="Assign inbound number"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={onSubmit} loading={assign.isPending} disabled={!msisdn || !tenantId}>
            Assign
          </Button>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-4">
        <Input
          label="MSISDN"
          required
          value={msisdn}
          onChange={(e) => setMsisdn(e.target.value)}
          placeholder="569XXXXXXXX"
        />
        <Input
          label="Tenant ID"
          required
          type="number"
          min={1}
          value={tenantId}
          onChange={(e) => setTenantId(e.target.value)}
        />
        <Input
          label="Label (optional)"
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          placeholder="marketing shortcode"
        />
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
        )}
      </form>
    </Modal>
  )
}
