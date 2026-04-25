import { useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { APIKey, InboundNumber, Tenant, WebhookDelivery, WebhookEndpoint } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Input } from '@/components/ui/Input'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { Tabs } from '@/components/ui/Tabs'
import { formatDate } from '@/lib/format'

export function TenantDetailPage() {
  const { id: idParam } = useParams<{ id: string }>()
  const tenantId = Number(idParam)
  const qc = useQueryClient()

  const tenant = useQuery({
    queryKey: ['tenant', tenantId],
    queryFn: async () => {
      const { data } = await api.get<Tenant>(`/admin/tenants/${tenantId}`)
      return data
    },
  })

  const setStatus = useMutation({
    mutationFn: async (action: 'suspend' | 'activate') => {
      await api.post(`/admin/tenants/${tenantId}/${action}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId] }),
  })

  const [tab, setTab] = useState<'keys' | 'webhooks' | 'deliveries' | 'inbound'>('keys')

  if (tenant.isLoading) {
    return (
      <div className="flex justify-center p-10">
        <Spinner />
      </div>
    )
  }
  if (tenant.error || !tenant.data) {
    return <div className="text-sm text-red-600">{errorMessage(tenant.error)}</div>
  }

  const t = tenant.data
  return (
    <div className="space-y-4">
      <div className="flex items-start justify-between">
        <div>
          <Link to="/tenants" className="text-sm text-slate-500 hover:underline">
            ← Tenants
          </Link>
          <h1 className="mt-1 text-xl font-semibold text-slate-900">{t.name}</h1>
          <div className="mt-1 flex items-center gap-3 text-sm text-slate-500">
            <span>id={t.id}</span>
            <Badge value={t.status} />
            <span>created {formatDate(t.created_at)}</span>
          </div>
        </div>
        <div>
          {t.status === 'active' ? (
            <Button
              variant="secondary"
              loading={setStatus.isPending}
              onClick={() => setStatus.mutate('suspend')}
            >
              Suspend
            </Button>
          ) : (
            <Button
              variant="secondary"
              loading={setStatus.isPending}
              onClick={() => setStatus.mutate('activate')}
            >
              Activate
            </Button>
          )}
        </div>
      </div>

      <Card>
        <CardHeader>
          <Tabs
            active={tab}
            onChange={(k) => setTab(k as typeof tab)}
            tabs={[
              { key: 'keys', label: 'API keys' },
              { key: 'webhooks', label: 'Webhooks' },
              { key: 'deliveries', label: 'Deliveries' },
              { key: 'inbound', label: 'Inbound numbers' },
            ]}
          />
          <span />
        </CardHeader>
        <CardBody className="p-0">
          {tab === 'keys' && <KeysTab tenantId={tenantId} />}
          {tab === 'webhooks' && <WebhooksTab tenantId={tenantId} />}
          {tab === 'deliveries' && <DeliveriesTab tenantId={tenantId} />}
          {tab === 'inbound' && <InboundTab tenantId={tenantId} />}
        </CardBody>
      </Card>
    </div>
  )
}

// ===== Tabs =====

function KeysTab({ tenantId }: { tenantId: number }) {
  const qc = useQueryClient()
  const keys = useQuery({
    queryKey: ['tenant', tenantId, 'keys'],
    queryFn: async () => {
      const { data } = await api.get<{ keys: APIKey[] }>(`/admin/tenants/${tenantId}/api-keys`)
      return data.keys
    },
  })
  const revoke = useMutation({
    mutationFn: async (id: number) => {
      await api.post(`/admin/api-keys/${id}/revoke`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId, 'keys'] }),
  })

  const [issueOpen, setIssueOpen] = useState(false)
  const [justIssued, setJustIssued] = useState<APIKey | null>(null)

  return (
    <div>
      <div className="flex items-center justify-end border-b border-slate-100 px-4 py-2">
        <Button onClick={() => setIssueOpen(true)}>Issue new key</Button>
      </div>
      {keys.isLoading ? (
        <div className="flex justify-center p-10">
          <Spinner />
        </div>
      ) : (
        <Table>
          <THead>
            <TR>
              <TH>Prefix</TH>
              <TH>Name</TH>
              <TH>Created</TH>
              <TH>Last used</TH>
              <TH>Status</TH>
              <TH className="text-right">Actions</TH>
            </TR>
          </THead>
          <TBody>
            {keys.data?.map((k) => (
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
                      onClick={() => revoke.mutate(k.id)}
                    >
                      Revoke
                    </Button>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      )}

      {issueOpen && (
        <IssueKeyModal
          tenantId={tenantId}
          onClose={() => setIssueOpen(false)}
          onIssued={(k) => {
            setIssueOpen(false)
            setJustIssued(k)
            qc.invalidateQueries({ queryKey: ['tenant', tenantId, 'keys'] })
          }}
        />
      )}
      {justIssued && <RevealKeyModal apiKey={justIssued} onClose={() => setJustIssued(null)} />}
    </div>
  )
}

function IssueKeyModal({
  tenantId,
  onClose,
  onIssued,
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

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    setErr(null)
    issue.mutate()
  }

  return (
    <Modal
      open
      onClose={onClose}
      title="Issue API key"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={onSubmit} loading={issue.isPending}>
            Issue
          </Button>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-4">
        <Input
          label="Label (optional)"
          placeholder="prod-server"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">{err}</div>
        )}
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
    <Modal
      open
      onClose={onClose}
      title="API key issued"
      width="lg"
      footer={
        <Button onClick={onClose}>Done</Button>
      }
    >
      <div className="space-y-3">
        <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
          Copy this token now. It will <strong>never be shown again</strong>.
        </div>
        <div className="flex items-center gap-2 rounded-md border border-slate-200 bg-slate-50 p-3 font-mono text-sm break-all">
          {apiKey.token}
        </div>
        <div className="flex justify-end">
          <Button variant="secondary" onClick={copy}>
            {copied ? 'Copied ✓' : 'Copy'}
          </Button>
        </div>
        <div className="text-xs text-slate-500">
          Prefix <span className="font-mono">{apiKey.prefix}</span> · created {formatDate(apiKey.created_at)}
        </div>
      </div>
    </Modal>
  )
}

function WebhooksTab({ tenantId }: { tenantId: number }) {
  const eps = useQuery({
    queryKey: ['tenant', tenantId, 'webhooks'],
    queryFn: async () => {
      const { data } = await api.get<{ endpoints: WebhookEndpoint[] }>(
        `/admin/tenants/${tenantId}/webhooks`,
      )
      return data.endpoints
    },
  })
  if (eps.isLoading) {
    return (
      <div className="flex justify-center p-10">
        <Spinner />
      </div>
    )
  }
  if (!eps.data?.length) {
    return <div className="px-4 py-6 text-sm text-slate-500">No webhook endpoints registered.</div>
  }
  return (
    <Table>
      <THead>
        <TR>
          <TH>ID</TH>
          <TH>URL</TH>
          <TH>Events</TH>
          <TH>Active</TH>
          <TH>Created</TH>
        </TR>
      </THead>
      <TBody>
        {eps.data.map((ep) => (
          <TR key={ep.id}>
            <TD className="font-mono text-xs text-slate-500">{ep.id}</TD>
            <TD className="break-all font-mono text-xs">{ep.url}</TD>
            <TD>
              <div className="flex flex-wrap gap-1">
                {ep.events.map((e) => (
                  <span key={e} className="rounded bg-slate-100 px-1.5 py-0.5 text-xs text-slate-700">
                    {e}
                  </span>
                ))}
              </div>
            </TD>
            <TD>
              <Badge value={ep.active ? 'active' : 'suspended'} />
            </TD>
            <TD className="text-slate-500">{formatDate(ep.created_at)}</TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}

function DeliveriesTab({ tenantId }: { tenantId: number }) {
  const qc = useQueryClient()
  const dels = useQuery({
    queryKey: ['tenant', tenantId, 'deliveries'],
    queryFn: async () => {
      const { data } = await api.get<{ deliveries: WebhookDelivery[] }>(
        `/admin/tenants/${tenantId}/webhook-deliveries?limit=50`,
      )
      return data.deliveries
    },
  })
  const retry = useMutation({
    mutationFn: async (id: number) => {
      await api.post(`/admin/webhook-deliveries/${id}/retry`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId, 'deliveries'] }),
  })

  if (dels.isLoading) {
    return (
      <div className="flex justify-center p-10">
        <Spinner />
      </div>
    )
  }
  if (!dels.data?.length) {
    return <div className="px-4 py-6 text-sm text-slate-500">No deliveries yet.</div>
  }
  return (
    <Table>
      <THead>
        <TR>
          <TH>ID</TH>
          <TH>Event</TH>
          <TH>Status</TH>
          <TH>Attempts</TH>
          <TH>Last HTTP</TH>
          <TH>Last error</TH>
          <TH>Created</TH>
          <TH className="text-right">Actions</TH>
        </TR>
      </THead>
      <TBody>
        {dels.data.map((d) => (
          <TR key={d.id}>
            <TD className="font-mono text-xs text-slate-500">{d.id}</TD>
            <TD className="text-xs">{d.event_type}</TD>
            <TD>
              <Badge value={d.status} />
            </TD>
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
                Retry
              </Button>
            </TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}

function InboundTab({ tenantId }: { tenantId: number }) {
  const all = useQuery({
    queryKey: ['inbound-numbers', 'all'],
    queryFn: async () => {
      const { data } = await api.get<{ numbers: InboundNumber[] }>('/admin/inbound-numbers')
      return data.numbers
    },
  })
  if (all.isLoading) {
    return (
      <div className="flex justify-center p-10">
        <Spinner />
      </div>
    )
  }
  const mine = (all.data ?? []).filter((n) => n.tenant_id === tenantId)
  if (!mine.length) {
    return (
      <div className="px-4 py-6 text-sm text-slate-500">
        No inbound numbers assigned to this tenant.
      </div>
    )
  }
  return (
    <Table>
      <THead>
        <TR>
          <TH>MSISDN</TH>
          <TH>Label</TH>
          <TH>Created</TH>
        </TR>
      </THead>
      <TBody>
        {mine.map((n) => (
          <TR key={n.msisdn}>
            <TD className="font-mono">{n.msisdn}</TD>
            <TD>{n.label || '—'}</TD>
            <TD className="text-slate-500">{formatDate(n.created_at)}</TD>
          </TR>
        ))}
      </TBody>
    </Table>
  )
}
