import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { WebhookDelivery } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatDate } from '@/lib/format'

export function TenantDeliveriesPage() {
  const { tenant } = useTenant()
  const qc = useQueryClient()
  const dels = useQuery({
    queryKey: ['tenant', tenant.id, 'deliveries'],
    queryFn: async () => {
      const { data } = await api.get<{ deliveries: WebhookDelivery[] }>(
        `/admin/tenants/${tenant.id}/webhook-deliveries?limit=50`,
      )
      return data.deliveries
    },
  })
  const retry = useMutation({
    mutationFn: async (id: number) => { await api.post(`/admin/webhook-deliveries/${id}/retry`) },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'deliveries'] }),
  })

  return (
    <TenantPage title="Entregas de webhook">
      <Card>
        <CardBody className="p-0">
          {dels.isLoading ? (
            <div className="flex justify-center p-10"><Spinner /></div>
          ) : dels.error ? (
            <div className="px-4 py-6 text-sm text-red-600">{errorMessage(dels.error)}</div>
          ) : !dels.data?.length ? (
            <div className="px-4 py-10 text-center text-sm text-slate-500">
              Aún no se han disparado entregas para este cliente.
            </div>
          ) : (
            <Table>
              <THead>
                <TR>
                  <TH>ID</TH>
                  <TH>Evento</TH>
                  <TH>Estado</TH>
                  <TH>Intentos</TH>
                  <TH>Último HTTP</TH>
                  <TH>Último error</TH>
                  <TH>Creado</TH>
                  <TH className="text-right">Acciones</TH>
                </TR>
              </THead>
              <TBody>
                {dels.data.map((d) => (
                  <TR key={d.id}>
                    <TD className="font-mono text-xs text-slate-500">{d.id}</TD>
                    <TD className="text-xs">{d.event_type}</TD>
                    <TD><Badge value={d.status} /></TD>
                    <TD>{d.attempts}</TD>
                    <TD>{d.last_status ?? '—'}</TD>
                    <TD className="max-w-xs truncate text-xs text-red-600" title={d.last_error ?? undefined}>
                      {d.last_error ?? '—'}
                    </TD>
                    <TD className="text-slate-500">{formatDate(d.created_at)}</TD>
                    <TD className="text-right">
                      <Button
                        variant="secondary"
                        loading={retry.isPending && retry.variables === d.id}
                        onClick={() => retry.mutate(d.id)}
                      >
                        Reintentar
                      </Button>
                    </TD>
                  </TR>
                ))}
              </TBody>
            </Table>
          )}
        </CardBody>
      </Card>
    </TenantPage>
  )
}
