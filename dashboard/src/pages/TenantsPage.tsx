import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { Tenant } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { formatDate } from '@/lib/format'

interface ListResp {
  tenants: Tenant[]
}

async function fetchTenants(): Promise<Tenant[]> {
  const { data } = await api.get<ListResp>('/admin/tenants')
  return data.tenants
}

export function TenantsPage() {
  const qc = useQueryClient()
  const tenants = useQuery({ queryKey: ['tenants'], queryFn: fetchTenants })
  const [creating, setCreating] = useState(false)

  const setStatus = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: 'suspend' | 'activate' }) => {
      await api.post(`/admin/tenants/${id}/${action}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenants'] }),
  })

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader>
          <h1 className="text-base font-semibold">Clientes</h1>
          <Button onClick={() => setCreating(true)}>Nuevo cliente</Button>
        </CardHeader>
        <CardBody className="p-0">
          {tenants.isLoading ? (
            <div className="flex items-center justify-center py-10">
              <Spinner />
            </div>
          ) : tenants.error ? (
            <div className="px-4 py-6 text-sm text-red-600">{errorMessage(tenants.error)}</div>
          ) : !tenants.data?.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              No hay clientes todavía. Crea uno con el botón "Nuevo cliente".
            </div>
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>ID</TH>
                  <TH>Nombre</TH>
                  <TH>Estado</TH>
                  <TH>Límite diario</TH>
                  <TH>Creado</TH>
                  <TH className="text-right">Acciones</TH>
                </TR>
              </THead>
              <TBody>
                {tenants.data.map((t) => (
                  <TR key={t.id}>
                    <TD className="font-mono text-xs text-slate-500">{t.id}</TD>
                    <TD>
                      <Link to={`/clientes/${t.id}`} className="font-medium text-slate-900 hover:underline">
                        {t.name}
                      </Link>
                    </TD>
                    <TD>
                      <Badge value={t.status} />
                    </TD>
                    <TD>{t.daily_sms_limit ?? 'Sin límite'}</TD>
                    <TD className="text-slate-500">{formatDate(t.created_at)}</TD>
                    <TD className="text-right">
                      {t.status === 'active' ? (
                        <Button
                          variant="secondary"
                          loading={setStatus.isPending && setStatus.variables?.id === t.id}
                          onClick={() => setStatus.mutate({ id: t.id, action: 'suspend' })}
                        >
                          Suspender
                        </Button>
                      ) : (
                        <Button
                          variant="secondary"
                          loading={setStatus.isPending && setStatus.variables?.id === t.id}
                          onClick={() => setStatus.mutate({ id: t.id, action: 'activate' })}
                        >
                          Activar
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

      {creating && <NewTenantModal onClose={() => setCreating(false)} />}
    </div>
  )
}

function NewTenantModal({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  const [dailyLimit, setDailyLimit] = useState<string>('')
  const [err, setErr] = useState<string | null>(null)

  const create = useMutation({
    mutationFn: async () => {
      const body: { name: string; daily_sms_limit?: number } = { name }
      if (dailyLimit) body.daily_sms_limit = Number(dailyLimit)
      const { data } = await api.post<Tenant>('/admin/tenants', body)
      return data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tenants'] })
      onClose()
    },
    onError: (e) => setErr(errorMessage(e)),
  })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setErr(null)
    create.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title="Nuevo cliente"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={onSubmit} loading={create.isPending} disabled={!name}>
            Crear
          </Button>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-4">
        <Input
          label="Nombre"
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Acme Corp"
        />
        <Input
          label="Límite diario de SMS (opcional)"
          type="number"
          min={1}
          value={dailyLimit}
          onChange={(e) => setDailyLimit(e.target.value)}
          placeholder="dejar en blanco = sin límite"
        />
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
        )}
      </form>
    </Modal>
  )
}
