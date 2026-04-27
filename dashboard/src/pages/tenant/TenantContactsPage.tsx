import { useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Modal } from '@/components/ui/Modal'
import { Spinner } from '@/components/ui/Spinner'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'
import { formatRelative } from '@/lib/format'

interface Contact {
  id: number
  msisdn: string
  name: string | null
  notes: string | null
  opt_out: boolean
  opt_out_at: string | null
  created_at: string
  updated_at: string
}
interface ContactsResp {
  contacts: Contact[]
  next_cursor: string | null
  total: number
  opted_out: number
}
interface ContactList {
  id: number
  name: string
  description: string | null
  member_count: number
  created_at: string
}

// Editorial-density contacts page. Two-column layout: left rail with
// tenant-scoped lists, right pane is a refined table. Avoids "card upon
// card" nesting — instead uses hairline dividers and generous whitespace.
export function TenantContactsPage() {
  const { tenant } = useTenant()
  const qc = useQueryClient()

  const [q, setQ] = useState('')
  const [filter, setFilter] = useState<'all' | 'optedout'>('all')
  const [activeListID, setActiveListID] = useState<number | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())

  const buildURL = () => {
    const sp = new URLSearchParams()
    if (q.trim()) sp.set('q', q.trim())
    if (filter === 'optedout') sp.set('opt_out', 'true')
    if (activeListID) sp.set('list_id', String(activeListID))
    sp.set('limit', '100')
    return `/admin/tenants/${tenant.id}/contacts?${sp.toString()}`
  }

  const contacts = useQuery({
    queryKey: ['tenant', tenant.id, 'contacts', q, filter, activeListID],
    queryFn: async () => (await api.get<ContactsResp>(buildURL())).data,
  })
  const lists = useQuery({
    queryKey: ['tenant', tenant.id, 'lists'],
    queryFn: async () => (await api.get<{ lists: ContactList[] }>(`/admin/tenants/${tenant.id}/contact-lists`)).data.lists,
  })

  const [importOpen, setImportOpen] = useState(false)
  const [newContactOpen, setNewContactOpen] = useState(false)
  const [newListOpen, setNewListOpen] = useState(false)
  const [addToListOpen, setAddToListOpen] = useState(false)

  const setOptOut = useMutation({
    mutationFn: async ({ id, optOut }: { id: number; optOut: boolean }) => {
      await api.post(`/admin/contacts/${id}/opt-out`, { opt_out: optOut })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'contacts'] }),
  })

  const remove = useMutation({
    mutationFn: async (id: number) => api.delete(`/admin/contacts/${id}`),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'contacts'] }),
  })

  const total = contacts.data?.total ?? 0
  const optedOut = contacts.data?.opted_out ?? 0
  const visible = contacts.data?.contacts ?? []
  const allSelected = visible.length > 0 && visible.every((c) => selected.has(c.id))
  const toggleAll = () => {
    if (allSelected) setSelected(new Set())
    else setSelected(new Set(visible.map((c) => c.id)))
  }
  const toggleOne = (id: number) => {
    const next = new Set(selected)
    if (next.has(id)) next.delete(id)
    else next.add(id)
    setSelected(next)
  }

  return (
    <TenantPage title="Contactos">
      {/* Stats line — editorial header */}
      <div className="flex flex-wrap items-baseline gap-x-8 gap-y-2 border-b border-border pb-5">
        <Stat label="contactos" value={total} />
        <Stat label="con opt-out" value={optedOut} />
        <Stat label="listas" value={lists.data?.length ?? 0} />
        <div className="ml-auto flex flex-wrap gap-2">
          <SecondaryAction onClick={() => setImportOpen(true)}>Importar CSV</SecondaryAction>
          <SecondaryAction onClick={() => setNewListOpen(true)}>Nueva lista</SecondaryAction>
          <PrimaryAction onClick={() => setNewContactOpen(true)}>+ Contacto</PrimaryAction>
        </div>
      </div>

      <div className="grid gap-8 lg:grid-cols-[220px_minmax(0,1fr)]">
        {/* Left rail — listas + filtros */}
        <aside className="space-y-6 lg:sticky lg:top-6 lg:self-start">
          <div>
            <RailLabel>Vista</RailLabel>
            <RailItem
              active={filter === 'all' && activeListID === null}
              onClick={() => { setFilter('all'); setActiveListID(null) }}
            >
              Todos
              <RailCount>{total}</RailCount>
            </RailItem>
            <RailItem
              active={filter === 'optedout'}
              onClick={() => { setFilter('optedout'); setActiveListID(null) }}
            >
              Opt-out
              <RailCount>{optedOut}</RailCount>
            </RailItem>
          </div>

          <div>
            <RailLabel>Listas</RailLabel>
            {lists.isLoading ? (
              <div className="px-3 py-2"><Spinner /></div>
            ) : !lists.data?.length ? (
              <p className="px-3 py-2 text-xs text-ink-mute">
                Sin listas todavía. Crea una para segmentar.
              </p>
            ) : (
              lists.data.map((l) => (
                <RailItem
                  key={l.id}
                  active={activeListID === l.id}
                  onClick={() => { setActiveListID(l.id); setFilter('all') }}
                >
                  {l.name}
                  <RailCount>{l.member_count}</RailCount>
                </RailItem>
              ))
            )}
          </div>
        </aside>

        {/* Main pane — search + table */}
        <div className="space-y-5">
          <div className="flex items-center gap-3">
            <div className="relative flex-1">
              <input
                value={q}
                onChange={(e) => setQ(e.target.value)}
                placeholder="Buscar por nombre o número…"
                className="w-full rounded-md border border-border bg-surface px-4 py-2 pl-9 text-sm placeholder:text-ink-faint focus:border-ink focus:outline-none focus:ring-0"
              />
              <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-faint">🔎</span>
            </div>
            {selected.size > 0 && (
              <div className="flex items-center gap-2 rounded-md bg-muted px-3 py-2 text-xs text-ink-soft">
                <span className="tabular">{selected.size} seleccionado{selected.size === 1 ? '' : 's'}</span>
                <button
                  className="rounded bg-ink px-2 py-1 text-[11px] font-medium text-canvas hover:bg-ink-soft"
                  onClick={() => setAddToListOpen(true)}
                >
                  Añadir a lista
                </button>
                <button
                  className="text-[11px] text-ink-mute underline-offset-2 hover:underline"
                  onClick={() => setSelected(new Set())}
                >
                  Limpiar
                </button>
              </div>
            )}
          </div>

          {contacts.isLoading ? (
            <div className="flex justify-center py-16"><Spinner /></div>
          ) : contacts.error ? (
            <p className="rounded-md border border-danger-soft bg-danger-soft/40 px-4 py-3 text-sm text-danger-ink">
              {errorMessage(contacts.error)}
            </p>
          ) : !visible.length ? (
            <EmptyState onImport={() => setImportOpen(true)} onCreate={() => setNewContactOpen(true)} />
          ) : (
            <div className="overflow-hidden rounded-lg border border-border bg-surface">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-border bg-canvas">
                    <th className="px-3 py-2 text-left font-medium">
                      <input
                        type="checkbox"
                        checked={allSelected}
                        onChange={toggleAll}
                        className="cursor-pointer accent-ink"
                      />
                    </th>
                    <ThSerif>Nombre</ThSerif>
                    <ThSerif>Número</ThSerif>
                    <ThSerif>Estado</ThSerif>
                    <ThSerif>Agregado</ThSerif>
                    <th className="px-3 py-2"></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {visible.map((c) => (
                    <tr key={c.id} className="group hover:bg-muted/40">
                      <td className="px-3 py-3">
                        <input
                          type="checkbox"
                          checked={selected.has(c.id)}
                          onChange={() => toggleOne(c.id)}
                          className="cursor-pointer accent-ink"
                        />
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center gap-3">
                          <Avatar name={c.name ?? c.msisdn} />
                          <div>
                            <div className="font-medium text-ink">{c.name || <span className="text-ink-faint italic">sin nombre</span>}</div>
                            {c.notes && <div className="text-xs text-ink-mute">{c.notes}</div>}
                          </div>
                        </div>
                      </td>
                      <td className="px-3 py-3 font-mono text-xs tabular text-ink-soft">{c.msisdn}</td>
                      <td className="px-3 py-3">
                        {c.opt_out ? <Badge value="suspended" /> : <Badge value="active" />}
                      </td>
                      <td className="px-3 py-3 text-xs text-ink-mute">{formatRelative(c.created_at)}</td>
                      <td className="px-3 py-3 text-right">
                        <div className="invisible flex justify-end gap-1 group-hover:visible">
                          <button
                            onClick={() => setOptOut.mutate({ id: c.id, optOut: !c.opt_out })}
                            className="rounded px-2 py-1 text-xs text-ink-soft hover:bg-muted"
                            title={c.opt_out ? 'Reactivar' : 'Marcar opt-out'}
                          >
                            {c.opt_out ? 'Reactivar' : 'Opt-out'}
                          </button>
                          <button
                            onClick={() => {
                              if (confirm(`¿Eliminar ${c.msisdn}?`)) remove.mutate(c.id)
                            }}
                            className="rounded px-2 py-1 text-xs text-danger hover:bg-danger-soft"
                          >
                            Eliminar
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </div>

      {importOpen && <ImportCSVModal tenantId={tenant.id} onClose={() => setImportOpen(false)} onDone={() => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'contacts'] })} />}
      {newContactOpen && <NewContactModal tenantId={tenant.id} onClose={() => setNewContactOpen(false)} onDone={() => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'contacts'] })} />}
      {newListOpen && <NewListModal tenantId={tenant.id} onClose={() => setNewListOpen(false)} onDone={() => qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'lists'] })} />}
      {addToListOpen && (
        <AddToListModal
          lists={lists.data ?? []}
          contactIDs={Array.from(selected)}
          onClose={() => setAddToListOpen(false)}
          onDone={() => {
            setSelected(new Set())
            qc.invalidateQueries({ queryKey: ['tenant', tenant.id, 'lists'] })
          }}
        />
      )}
    </TenantPage>
  )
}

// ── design primitives just for this page ─────────────────────────────

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="flex items-baseline gap-2">
      <span className="font-display text-3xl font-medium tabular tracking-tightest text-ink">
        {value.toLocaleString('es-CL')}
      </span>
      <span className="text-xs uppercase tracking-wider text-ink-mute">{label}</span>
    </div>
  )
}

function PrimaryAction({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className="rounded-md bg-ink px-3.5 py-1.5 text-sm font-medium text-canvas transition-colors hover:bg-ink-soft"
    >
      {children}
    </button>
  )
}
function SecondaryAction({ onClick, children }: { onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className="rounded-md border border-border bg-surface px-3.5 py-1.5 text-sm font-medium text-ink-soft transition-colors hover:border-ink hover:text-ink"
    >
      {children}
    </button>
  )
}

function ThSerif({ children }: { children: React.ReactNode }) {
  return (
    <th className="px-3 py-2 text-left text-[11px] font-medium uppercase tracking-wider text-ink-mute">
      {children}
    </th>
  )
}

function RailLabel({ children }: { children: React.ReactNode }) {
  return (
    <div className="mb-2 px-3 text-[10px] font-semibold uppercase tracking-[0.14em] text-ink-faint">
      {children}
    </div>
  )
}

function RailItem({
  active, onClick, children,
}: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={`flex w-full items-center justify-between rounded-md px-3 py-1.5 text-left text-sm transition-colors ${
        active
          ? 'bg-ink text-canvas'
          : 'text-ink-soft hover:bg-muted hover:text-ink'
      }`}
    >
      {children}
    </button>
  )
}

function RailCount({ children }: { children: React.ReactNode }) {
  return <span className="ml-2 tabular text-[11px] opacity-70">{children}</span>
}

function Avatar({ name }: { name: string }) {
  // Initials box — warm tones derived from a hash of the name so each
  // contact gets a stable but distinct fill.
  const initials = name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((s) => s[0])
    .join('')
    .toUpperCase() || '·'
  const palette = [
    'bg-amber-100 text-amber-900',
    'bg-emerald-100 text-emerald-900',
    'bg-rose-100 text-rose-900',
    'bg-indigo-100 text-indigo-900',
    'bg-stone-200 text-stone-800',
  ]
  const hash = [...name].reduce((a, c) => (a * 31 + c.charCodeAt(0)) >>> 0, 0)
  const cls = palette[hash % palette.length]
  return (
    <div className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-[11px] font-semibold ${cls}`}>
      {initials}
    </div>
  )
}

function EmptyState({ onImport, onCreate }: { onImport: () => void; onCreate: () => void }) {
  return (
    <div className="rounded-lg border border-dashed border-border bg-surface px-6 py-16 text-center">
      <div className="mx-auto inline-flex h-14 w-14 items-center justify-center rounded-full bg-muted text-2xl">
        ✦
      </div>
      <h3 className="mt-4 font-display text-2xl text-ink">Aún no hay contactos</h3>
      <p className="mx-auto mt-2 max-w-md text-sm text-ink-mute">
        Carga un CSV con tus números o agrega contactos uno por uno. Después puedes
        agruparlos en listas y enviar campañas a un grupo completo.
      </p>
      <div className="mt-5 flex justify-center gap-2">
        <SecondaryAction onClick={onImport}>Importar CSV</SecondaryAction>
        <PrimaryAction onClick={onCreate}>Agregar contacto</PrimaryAction>
      </div>
    </div>
  )
}

// ── Modals ───────────────────────────────────────────────────────────

function NewContactModal({ tenantId, onClose, onDone }: { tenantId: number; onClose: () => void; onDone: () => void }) {
  const [msisdn, setMsisdn] = useState('')
  const [name, setName] = useState('')
  const [notes, setNotes] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const create = useMutation({
    mutationFn: async () => api.post(`/admin/tenants/${tenantId}/contacts`, { msisdn, name, notes }),
    onSuccess: () => { onDone(); onClose() },
    onError: (e) => setErr(errorMessage(e)),
  })
  const onSubmit = (e: FormEvent) => { e.preventDefault(); setErr(null); create.mutate() }
  return (
    <Modal
      open
      onClose={onClose}
      title="Nuevo contacto"
      footer={
        <>
          <SecondaryAction onClick={onClose}>Cancelar</SecondaryAction>
          <PrimaryAction onClick={() => create.mutate()}>Crear</PrimaryAction>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-3">
        <Field label="Número (MSISDN)" required>
          <input
            value={msisdn}
            onChange={(e) => setMsisdn(e.target.value)}
            placeholder="569XXXXXXXX"
            className="w-full rounded-md border border-border px-3 py-2 font-mono text-sm focus:border-ink focus:outline-none"
          />
        </Field>
        <Field label="Nombre">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded-md border border-border px-3 py-2 text-sm focus:border-ink focus:outline-none"
          />
        </Field>
        <Field label="Notas">
          <textarea
            value={notes}
            onChange={(e) => setNotes(e.target.value)}
            rows={2}
            className="w-full rounded-md border border-border px-3 py-2 text-sm focus:border-ink focus:outline-none"
          />
        </Field>
        {err && <p className="rounded border border-danger-soft bg-danger-soft/40 px-3 py-2 text-xs text-danger-ink">{err}</p>}
      </form>
    </Modal>
  )
}

function NewListModal({ tenantId, onClose, onDone }: { tenantId: number; onClose: () => void; onDone: () => void }) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const create = useMutation({
    mutationFn: async () => api.post(`/admin/tenants/${tenantId}/contact-lists`, { name, description }),
    onSuccess: () => { onDone(); onClose() },
    onError: (e) => setErr(errorMessage(e)),
  })
  const onSubmit = (e: FormEvent) => { e.preventDefault(); setErr(null); create.mutate() }
  return (
    <Modal
      open
      onClose={onClose}
      title="Nueva lista"
      footer={
        <>
          <SecondaryAction onClick={onClose}>Cancelar</SecondaryAction>
          <PrimaryAction onClick={() => create.mutate()}>Crear lista</PrimaryAction>
        </>
      }
    >
      <form onSubmit={onSubmit} className="space-y-3">
        <Field label="Nombre" required>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="VIP, Sucursales, Clientes morosos…"
            className="w-full rounded-md border border-border px-3 py-2 text-sm focus:border-ink focus:outline-none"
          />
        </Field>
        <Field label="Descripción">
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            className="w-full rounded-md border border-border px-3 py-2 text-sm focus:border-ink focus:outline-none"
          />
        </Field>
        {err && <p className="rounded border border-danger-soft bg-danger-soft/40 px-3 py-2 text-xs text-danger-ink">{err}</p>}
      </form>
    </Modal>
  )
}

function AddToListModal({
  lists, contactIDs, onClose, onDone,
}: { lists: ContactList[]; contactIDs: number[]; onClose: () => void; onDone: () => void }) {
  const [listID, setListID] = useState<number | null>(lists[0]?.id ?? null)
  const [err, setErr] = useState<string | null>(null)
  const add = useMutation({
    mutationFn: async () => {
      if (!listID) return
      await api.post(`/admin/contact-lists/${listID}/members`, { contact_ids: contactIDs })
    },
    onSuccess: () => { onDone(); onClose() },
    onError: (e) => setErr(errorMessage(e)),
  })
  return (
    <Modal
      open
      onClose={onClose}
      title="Añadir a lista"
      footer={
        <>
          <SecondaryAction onClick={onClose}>Cancelar</SecondaryAction>
          <PrimaryAction onClick={() => { setErr(null); add.mutate() }}>Añadir {contactIDs.length}</PrimaryAction>
        </>
      }
    >
      <div className="space-y-3">
        <Field label="Lista de destino">
          <select
            value={listID ?? ''}
            onChange={(e) => setListID(Number(e.target.value))}
            className="w-full rounded-md border border-border px-3 py-2 text-sm focus:border-ink focus:outline-none"
          >
            {lists.length === 0 ? (
              <option>No hay listas — crea una primero</option>
            ) : (
              lists.map((l) => (
                <option key={l.id} value={l.id}>
                  {l.name} ({l.member_count} miembros)
                </option>
              ))
            )}
          </select>
        </Field>
        {err && <p className="rounded border border-danger-soft bg-danger-soft/40 px-3 py-2 text-xs text-danger-ink">{err}</p>}
      </div>
    </Modal>
  )
}

function ImportCSVModal({ tenantId, onClose, onDone }: { tenantId: number; onClose: () => void; onDone: () => void }) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [csvText, setCsvText] = useState('')
  const [err, setErr] = useState<string | null>(null)
  const [result, setResult] = useState<{ imported: number; skipped: number; errors?: string[] } | null>(null)

  const upload = useMutation({
    mutationFn: async () => {
      const { data } = await api.post(
        `/admin/tenants/${tenantId}/contacts/import`,
        csvText,
        { headers: { 'Content-Type': 'text/csv' } },
      )
      return data as { imported: number; skipped: number; errors?: string[] }
    },
    onSuccess: (r) => { setResult(r); onDone() },
    onError: (e) => setErr(errorMessage(e)),
  })

  const onFile = async (f: File | null) => {
    if (!f) return
    const txt = await f.text()
    setCsvText(txt)
  }

  return (
    <Modal
      open
      onClose={onClose}
      title="Importar contactos desde CSV"
      width="lg"
      footer={
        <>
          <SecondaryAction onClick={onClose}>{result ? 'Cerrar' : 'Cancelar'}</SecondaryAction>
          {!result && (
            <PrimaryAction onClick={() => { setErr(null); upload.mutate() }}>
              Importar
            </PrimaryAction>
          )}
        </>
      }
    >
      <div className="space-y-3">
        {!result && (
          <>
            <p className="text-sm text-ink-soft">
              CSV con header opcional. Columnas reconocidas:{' '}
              <code className="rounded bg-muted px-1 font-mono text-xs">msisdn</code> (obligatoria),{' '}
              <code className="rounded bg-muted px-1 font-mono text-xs">name</code>,{' '}
              <code className="rounded bg-muted px-1 font-mono text-xs">notes</code>.
              Sin header se asume <em>número, nombre</em>.
            </p>
            <div className="grid gap-3 sm:grid-cols-2">
              <button
                onClick={() => fileRef.current?.click()}
                className="rounded-md border border-dashed border-border bg-canvas px-4 py-6 text-center text-sm text-ink-soft hover:border-ink hover:text-ink"
              >
                <div className="text-2xl">📂</div>
                <div className="mt-1">Elegir archivo CSV</div>
              </button>
              <div className="rounded-md border border-border bg-canvas p-4 text-xs text-ink-mute">
                <div className="mb-1 font-mono">Ejemplo</div>
                <pre className="whitespace-pre-wrap">{`msisdn,name,notes
56987654321,Pablo,VIP
56999000111,Lucia,
56933445566,Mateo,Cliente mayorista`}</pre>
              </div>
            </div>
            <input
              ref={fileRef}
              type="file"
              accept=".csv,text/csv"
              className="hidden"
              onChange={(e) => onFile(e.target.files?.[0] ?? null)}
            />
            {csvText && (
              <div>
                <div className="mb-1 text-xs text-ink-mute">Vista previa ({csvText.split('\n').length} líneas)</div>
                <pre className="max-h-32 overflow-auto rounded-md border border-border bg-canvas p-3 font-mono text-xs">
                  {csvText.split('\n').slice(0, 8).join('\n')}
                  {csvText.split('\n').length > 8 && '\n…'}
                </pre>
              </div>
            )}
            {err && <p className="rounded border border-danger-soft bg-danger-soft/40 px-3 py-2 text-xs text-danger-ink">{err}</p>}
          </>
        )}
        {result && (
          <div className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <ResultStat value={result.imported} label="Importados / actualizados" tone="success" />
              <ResultStat value={result.skipped} label="Saltados" tone="warning" />
            </div>
            {result.errors && result.errors.length > 0 && (
              <div>
                <div className="mb-1 text-xs text-ink-mute">Errores en estas filas:</div>
                <ul className="max-h-40 space-y-1 overflow-auto rounded-md border border-border bg-canvas p-3 text-xs text-danger-ink">
                  {result.errors.map((e, i) => <li key={i}>{e}</li>)}
                </ul>
              </div>
            )}
          </div>
        )}
      </div>
    </Modal>
  )
}

function ResultStat({ value, label, tone }: { value: number; label: string; tone: 'success' | 'warning' }) {
  const ring = tone === 'success' ? 'border-success/30 bg-success-soft/40 text-success-ink' : 'border-warning/30 bg-warning-soft/40 text-warning-ink'
  return (
    <div className={`rounded-md border p-3 ${ring}`}>
      <div className="font-display text-3xl font-medium tabular tracking-tightest">{value.toLocaleString('es-CL')}</div>
      <div className="text-xs uppercase tracking-wider opacity-80">{label}</div>
    </div>
  )
}

function Field({ label, required, children }: { label: string; required?: boolean; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-ink-soft">
        {label}{required && <span className="text-danger"> *</span>}
      </span>
      {children}
    </label>
  )
}
