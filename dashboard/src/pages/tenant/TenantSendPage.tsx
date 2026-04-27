import { useMemo, useRef, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { api, errorMessage } from '@/api/client'
import type { APIKey } from '@/api/types'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { TBody, TD, TH, THead, TR, Table } from '@/components/ui/Table'
import { TenantPage, useTenant } from '@/components/TenantWorkspaceLayout'

interface ContactList { id: number; name: string; member_count: number }

interface BulkRow {
  index: number
  id?: string
  to: string
  status: string
  error?: string
  error_code?: string
}
interface BulkResp {
  batch_id: string
  accepted: number
  rejected: number
  messages: BulkRow[]
}

type RecipientMode = 'paste' | 'list' | 'xlsx'
type TimingMode = 'now' | 'once' | 'weekly'

const DAYS = [
  { key: 1, label: 'L' }, { key: 2, label: 'M' }, { key: 3, label: 'X' },
  { key: 4, label: 'J' }, { key: 5, label: 'V' }, { key: 6, label: 'S' },
  { key: 0, label: 'D' },
]

export function TenantSendPage() {
  const { tenant } = useTenant()
  const lists = useQuery({
    queryKey: ['tenant', tenant.id, 'lists'],
    queryFn: async () => (await api.get<{ lists: ContactList[] }>(`/admin/tenants/${tenant.id}/contact-lists`)).data.lists,
  })

  const [recipientMode, setRecipientMode] = useState<RecipientMode>('paste')
  const [timingMode, setTimingMode] = useState<TimingMode>('now')

  const [sender, setSender] = useState('')
  const [text, setText] = useState('')
  const [recipientsText, setRecipientsText] = useState('')
  const [listID, setListID] = useState<number | null>(null)
  const xlsxRef = useRef<HTMLInputElement>(null)
  const [xlsxFile, setXlsxFile] = useState<File | null>(null)

  // Programación
  const [whenDate, setWhenDate] = useState(() => new Date(Date.now() + 60 * 60 * 1000).toISOString().slice(0, 10))
  const [whenTime, setWhenTime] = useState(() => '09:00')
  const [recurDays, setRecurDays] = useState<number[]>([1, 2, 3, 4, 5])
  const [scheduleName, setScheduleName] = useState('')

  const recipients = useMemo(
    () => recipientsText.split(/[\s,;]+/).map((s) => s.trim()).filter(Boolean),
    [recipientsText],
  )

  type ImportResult = { imported: number; skipped: number; errors?: string[] }
  type ScheduledResult = { scheduled_id: number; send_at: string; status: string }
  const [result, setResult] = useState<BulkResp | ScheduledResult | ImportResult | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const send = useMutation({
    mutationFn: async () => {
      // Excel-only path: separate endpoint that imports N scheduled rows.
      if (recipientMode === 'xlsx' && xlsxFile) {
        const buf = await xlsxFile.arrayBuffer()
        const sp = new URLSearchParams()
        if (sender) sp.set('default_sender', sender)
        const { data } = await api.post(
          `/admin/tenants/${tenant.id}/scheduled/import?${sp.toString()}`,
          buf,
          { headers: { 'Content-Type': 'application/octet-stream' } },
        )
        return data as { imported: number; skipped: number; errors?: string[] }
      }

      // Otherwise, issue a one-shot key and use either /v1/sms/bulk
      // (immediate) or /admin/tenants/:id/scheduled (programada).
      const { data: key } = await api.post<APIKey>(
        `/admin/tenants/${tenant.id}/api-keys`,
        { name: `dashboard-temp-send-${Date.now()}` },
      )
      try {
        // Resolve recipients up-front so the JSON we POST carries the same set
        // for both immediate and scheduled flows.
        let toList: string[] = []
        if (recipientMode === 'paste') {
          toList = recipients
        } else if (recipientMode === 'list' && listID) {
          // Resolve list members via the contacts admin endpoint (we already
          // have a list-filter there).
          const { data } = await api.get<{ contacts: { msisdn: string }[] }>(
            `/admin/tenants/${tenant.id}/contacts?list_id=${listID}&limit=200`,
          )
          toList = data.contacts.map((c) => c.msisdn)
        }
        if (toList.length === 0) {
          throw new Error('No hay destinatarios. Pega números, elige una lista no vacía o sube un Excel.')
        }

        if (timingMode === 'now') {
          const { data } = await api.post<BulkResp>(
            '/v1/sms/bulk',
            { default_sender: sender, messages: toList.map((to) => ({ to, text })) },
            { headers: { 'X-API-Key': key.token } },
          )
          return data
        }

        // Programada: arma payload con send_at + recurrencia opcional.
        const sendAt = isoFromDateTime(whenDate, whenTime)
        const body: any = {
          name: scheduleName || undefined,
          sender,
          text,
          recipients: toList,
          send_at: sendAt,
        }
        if (timingMode === 'weekly') {
          if (recurDays.length === 0) throw new Error('Elige al menos un día de la semana')
          body.recurrence = 'weekly'
          body.recurrence_days = recurDays
        }
        const { data } = await api.post(`/admin/tenants/${tenant.id}/scheduled`, body)
        return data as { scheduled_id: number; send_at: string; status: string }
      } finally {
        await api.post(`/admin/api-keys/${key.id}/revoke`).catch(() => {})
      }
    },
    onSuccess: setResult,
    onError: (e) => setErr(errorMessage(e)),
  })

  const recipientCount = recipientMode === 'paste'
    ? recipients.length
    : recipientMode === 'list' && listID
      ? lists.data?.find((l) => l.id === listID)?.member_count ?? 0
      : 0

  const canSend =
    sender.trim() !== '' &&
    text.trim() !== '' &&
    tenant.status === 'active' &&
    (
      (recipientMode === 'paste' && recipients.length > 0) ||
      (recipientMode === 'list' && listID !== null) ||
      (recipientMode === 'xlsx' && xlsxFile !== null)
    )

  return (
    <TenantPage title="Enviar SMS">
      {tenant.status !== 'active' && (
        <div className="rounded-md border border-warning/30 bg-warning-soft px-4 py-2 text-sm text-warning-ink">
          El cliente está suspendido. Reactívalo para poder enviar mensajes.
        </div>
      )}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
        {/* Left: form */}
        <div className="space-y-6">
          {/* Mode tabs — recipient source */}
          <section>
            <SectionLabel>Destinatarios</SectionLabel>
            <ModeTabs
              value={recipientMode}
              options={[
                { key: 'paste', label: 'Pegar números' },
                { key: 'list',  label: 'Elegir lista' },
                { key: 'xlsx',  label: 'Subir Excel' },
              ]}
              onChange={setRecipientMode}
            />
            <div className="mt-4">
              {recipientMode === 'paste' && (
                <textarea
                  rows={3}
                  className="w-full rounded-md border border-border bg-surface px-3 py-2 font-mono text-sm focus:border-ink focus:outline-none"
                  value={recipientsText}
                  onChange={(e) => setRecipientsText(e.target.value)}
                  placeholder="569XXXXXXXX, 569XXXXXXXX, 569XXXXXXXX"
                />
              )}
              {recipientMode === 'list' && (
                <div className="space-y-2">
                  {lists.isLoading ? (
                    <Spinner />
                  ) : !lists.data?.length ? (
                    <p className="text-sm text-ink-mute">
                      No hay listas todavía.{' '}
                      <Link to="../contactos" relative="path" className="text-ink underline">
                        Crear una en Contactos
                      </Link>.
                    </p>
                  ) : (
                    <select
                      className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-ink focus:outline-none"
                      value={listID ?? ''}
                      onChange={(e) => setListID(Number(e.target.value))}
                    >
                      <option value="">— elige una lista —</option>
                      {lists.data.map((l) => (
                        <option key={l.id} value={l.id}>
                          {l.name} ({l.member_count.toLocaleString('es-CL')} miembros)
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              )}
              {recipientMode === 'xlsx' && (
                <div className="space-y-2">
                  <button
                    onClick={() => xlsxRef.current?.click()}
                    className="block w-full rounded-md border border-dashed border-border bg-canvas px-4 py-6 text-center text-sm text-ink-soft hover:border-ink hover:text-ink"
                  >
                    {xlsxFile ? (
                      <>
                        <div className="text-2xl">📄</div>
                        <div className="mt-1 font-medium">{xlsxFile.name}</div>
                        <div className="text-xs text-ink-mute">{(xlsxFile.size / 1024).toFixed(1)} KB · click para cambiar</div>
                      </>
                    ) : (
                      <>
                        <div className="text-2xl">📂</div>
                        <div className="mt-1">Elegir archivo .xlsx</div>
                      </>
                    )}
                  </button>
                  <input
                    ref={xlsxRef}
                    type="file"
                    accept=".xlsx,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
                    className="hidden"
                    onChange={(e) => setXlsxFile(e.target.files?.[0] ?? null)}
                  />
                  <p className="text-xs text-ink-mute leading-relaxed">
                    Cada fila = un envío programado. Columnas requeridas:{' '}
                    <code className="rounded bg-muted px-1 font-mono">msisdn</code>,{' '}
                    <code className="rounded bg-muted px-1 font-mono">text</code>,{' '}
                    <code className="rounded bg-muted px-1 font-mono">send_at</code>.
                    Opcionales:{' '}
                    <code className="rounded bg-muted px-1 font-mono">sender</code>,{' '}
                    <code className="rounded bg-muted px-1 font-mono">name</code>,{' '}
                    <code className="rounded bg-muted px-1 font-mono">recurrence</code>,{' '}
                    <code className="rounded bg-muted px-1 font-mono">days</code>.
                    Si una fila no trae sender, se usa el Remitente del formulario.
                  </p>
                </div>
              )}
            </div>
          </section>

          {/* Sender + text — siempre visibles */}
          <section className="grid gap-4 sm:grid-cols-2">
            <Field label="Remitente (sender)" required>
              <input
                value={sender}
                onChange={(e) => setSender(e.target.value)}
                placeholder="MiMarca o un número"
                className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-ink focus:outline-none"
              />
            </Field>
            {recipientMode !== 'xlsx' && (
              <Field label="Etiqueta (sólo programados)">
                <input
                  value={scheduleName}
                  onChange={(e) => setScheduleName(e.target.value)}
                  placeholder="Recordatorio mensual…"
                  className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-ink focus:outline-none"
                />
              </Field>
            )}
          </section>

          {recipientMode !== 'xlsx' && (
            <section>
              <SectionLabel>Texto del mensaje</SectionLabel>
              <textarea
                rows={4}
                value={text}
                onChange={(e) => setText(e.target.value)}
                maxLength={1600}
                className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-ink focus:outline-none"
                placeholder="Hola, este es un recordatorio…"
              />
              <p className="mt-1 text-xs text-ink-mute tabular">
                {text.length} caracteres · si incluyes emojis o tildes el SMS se codifica como UCS-2
                y cuenta como ~70 caracteres por parte.
              </p>
            </section>
          )}

          {/* Timing — sólo si no es xlsx (xlsx ya trae fechas por fila) */}
          {recipientMode !== 'xlsx' && (
            <section>
              <SectionLabel>Cuándo enviar</SectionLabel>
              <ModeTabs
                value={timingMode}
                options={[
                  { key: 'now',    label: 'Ahora' },
                  { key: 'once',   label: 'Programar fecha' },
                  { key: 'weekly', label: 'Recurrente (semanal)' },
                ]}
                onChange={setTimingMode}
              />
              {timingMode !== 'now' && (
                <div className="mt-4 rounded-md border border-border bg-canvas p-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field label={timingMode === 'weekly' ? 'Primera ejecución (fecha)' : 'Fecha de envío'} required>
                      <input
                        type="date"
                        value={whenDate}
                        onChange={(e) => setWhenDate(e.target.value)}
                        className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-ink focus:outline-none"
                      />
                    </Field>
                    <Field label="Hora" required>
                      <input
                        type="time"
                        value={whenTime}
                        onChange={(e) => setWhenTime(e.target.value)}
                        className="w-full rounded-md border border-border bg-surface px-3 py-2 text-sm focus:border-ink focus:outline-none"
                      />
                    </Field>
                  </div>
                  {timingMode === 'weekly' && (
                    <div className="mt-3">
                      <SectionLabel>Días de la semana</SectionLabel>
                      <DayBubbles selected={recurDays} onChange={setRecurDays} />
                    </div>
                  )}
                  <p className="mt-3 text-xs text-ink-mute">
                    Zona horaria: <span className="font-mono">America/Santiago</span>
                  </p>
                </div>
              )}
            </section>
          )}

          {err && (
            <div className="rounded-md border border-danger/30 bg-danger-soft px-3 py-2 text-sm text-danger-ink">{err}</div>
          )}

          {/* Action */}
          <div className="flex items-center justify-end gap-3">
            <button
              onClick={() => { setErr(null); setResult(null); send.mutate() }}
              disabled={!canSend || send.isPending}
              className="rounded-md bg-ink px-4 py-2 text-sm font-medium text-canvas transition-colors hover:bg-ink-soft disabled:cursor-not-allowed disabled:bg-ink-faint"
            >
              {send.isPending && '⏳ '}
              {actionLabel(recipientMode, timingMode, recipientCount)}
            </button>
          </div>

          {/* Result */}
          {result && 'batch_id' in result && (
            <ResultBulk r={result} />
          )}
          {result && 'scheduled_id' in result && (
            <ResultScheduled r={result} />
          )}
          {result && 'imported' in (result as any) && (
            <ResultImport r={result as any} />
          )}
        </div>

        {/* Right rail — preview / summary */}
        <aside className="lg:sticky lg:top-6 lg:self-start">
          <div className="rounded-lg border border-border bg-surface p-4">
            <SectionLabel>Resumen</SectionLabel>
            <dl className="mt-2 space-y-2 text-sm">
              <SumLine k="Cliente" v={tenant.name} />
              <SumLine k="Modo" v={recipientModeLabel(recipientMode)} />
              <SumLine k="Destinatarios" v={recipientMode === 'xlsx' ? 'depende del Excel' : `${recipientCount.toLocaleString('es-CL')}`} />
              <SumLine k="Timing" v={timingLabel(timingMode, whenDate, whenTime, recurDays)} />
              {sender && <SumLine k="Remitente" v={<span className="font-mono">{sender}</span>} />}
              {text && (
                <li className="border-t border-border pt-3">
                  <div className="text-xs uppercase tracking-wider text-ink-mute">Vista previa</div>
                  <div className="mt-1 rounded-md bg-canvas p-2 text-xs leading-snug">{text}</div>
                </li>
              )}
            </dl>
          </div>
        </aside>
      </div>
    </TenantPage>
  )
}

// ── helpers de UI ──────────────────────────────────────────────────

function SectionLabel({ children }: { children: React.ReactNode }) {
  return <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-ink-mute">{children}</div>
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

function SumLine({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <li className="flex items-baseline justify-between gap-3">
      <span className="text-xs uppercase tracking-wider text-ink-mute">{k}</span>
      <span className="text-right text-ink">{v}</span>
    </li>
  )
}

function ModeTabs<T extends string>({
  value, options, onChange,
}: {
  value: T
  options: { key: T; label: string }[]
  onChange: (v: T) => void
}) {
  return (
    <div className="inline-flex rounded-md border border-border bg-surface p-1">
      {options.map((o) => (
        <button
          key={o.key}
          onClick={() => onChange(o.key)}
          className={`rounded px-3 py-1 text-sm transition-colors ${
            value === o.key
              ? 'bg-ink text-canvas font-medium'
              : 'text-ink-soft hover:text-ink'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

function DayBubbles({ selected, onChange }: { selected: number[]; onChange: (d: number[]) => void }) {
  const toggle = (k: number) => {
    onChange(selected.includes(k) ? selected.filter((x) => x !== k) : [...selected, k].sort())
  }
  return (
    <div className="flex gap-1.5">
      {DAYS.map((d) => {
        const active = selected.includes(d.key)
        return (
          <button
            key={d.key}
            onClick={() => toggle(d.key)}
            className={`h-9 w-9 rounded-full text-sm font-semibold transition-colors ${
              active ? 'bg-accent text-canvas' : 'border border-border bg-surface text-ink-soft hover:border-ink'
            }`}
            title={['Domingo','Lunes','Martes','Miércoles','Jueves','Viernes','Sábado'][d.key]}
          >
            {d.label}
          </button>
        )
      })}
    </div>
  )
}

function ResultBulk({ r }: { r: BulkResp }) {
  return (
    <div className="overflow-hidden rounded-lg border border-border bg-surface">
      <div className="border-b border-border px-4 py-3 text-sm">
        Lote <span className="font-mono">{r.batch_id}</span> — aceptados:{' '}
        <strong className="text-success-ink">{r.accepted}</strong> · rechazados:{' '}
        <strong className="text-danger-ink">{r.rejected}</strong>
      </div>
      <Table>
        <THead><TR><TH>#</TH><TH>Destino</TH><TH>Estado</TH><TH>ID</TH><TH>Error</TH></TR></THead>
        <TBody>
          {r.messages.map((m) => (
            <TR key={m.index}>
              <TD className="text-xs">{m.index + 1}</TD>
              <TD className="font-mono text-xs">{m.to}</TD>
              <TD><Badge value={m.status} /></TD>
              <TD className="font-mono text-xs text-ink-mute">{m.id ?? '—'}</TD>
              <TD className="text-xs text-danger-ink">
                {m.error_code ? `[${m.error_code}] ` : ''}{m.error ?? '—'}
              </TD>
            </TR>
          ))}
        </TBody>
      </Table>
    </div>
  )
}

function ResultScheduled({ r }: { r: { scheduled_id: number; send_at: string; status: string } }) {
  return (
    <div className="rounded-lg border border-success/30 bg-success-soft/40 p-4 text-sm text-success-ink">
      <div className="font-medium">✓ Envío programado</div>
      <div className="mt-1">
        ID <span className="font-mono">{r.scheduled_id}</span> · próximo disparo:{' '}
        <strong>{new Date(r.send_at).toLocaleString('es-CL')}</strong>
      </div>
      <div className="mt-2 text-xs">
        <Link to="../programados" relative="path" className="underline">Ver programados</Link>
      </div>
    </div>
  )
}

function ResultImport({ r }: { r: { imported: number; skipped: number; errors?: string[] } }) {
  return (
    <div className="space-y-3 rounded-lg border border-border bg-surface p-4">
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="rounded-md border border-success/30 bg-success-soft/40 p-3">
          <div className="font-display text-3xl font-medium tabular text-success-ink">{r.imported}</div>
          <div className="text-xs uppercase tracking-wider text-success-ink/80">Programados creados</div>
        </div>
        <div className="rounded-md border border-warning/30 bg-warning-soft/40 p-3">
          <div className="font-display text-3xl font-medium tabular text-warning-ink">{r.skipped}</div>
          <div className="text-xs uppercase tracking-wider text-warning-ink/80">Filas saltadas</div>
        </div>
      </div>
      {r.errors && r.errors.length > 0 && (
        <ul className="max-h-40 space-y-1 overflow-auto rounded-md border border-border bg-canvas p-3 text-xs text-danger-ink">
          {r.errors.map((e, i) => <li key={i}>{e}</li>)}
        </ul>
      )}
    </div>
  )
}

// ── helpers de lógica ──────────────────────────────────────────────

function isoFromDateTime(date: string, time: string): string {
  // Construct local datetime then convert to ISO with the local timezone
  // offset so the backend interprets it in America/Santiago semantics.
  const [y, m, d] = date.split('-').map(Number)
  const [hh, mm] = time.split(':').map(Number)
  const local = new Date(y, m - 1, d, hh, mm, 0)
  return local.toISOString()
}

function recipientModeLabel(m: RecipientMode) {
  return m === 'paste' ? 'Pegar números' : m === 'list' ? 'Lista de contactos' : 'Importar Excel'
}

function timingLabel(m: TimingMode, date: string, time: string, days: number[]) {
  if (m === 'now') return 'Ahora'
  if (m === 'once') return `${date} ${time}`
  const names = ['D', 'L', 'M', 'X', 'J', 'V', 'S']
  const sel = days.sort((a, b) => a - b).map((d) => names[d]).join(' ')
  return `${time} · ${sel}`
}

function actionLabel(rm: RecipientMode, tm: TimingMode, count: number) {
  if (rm === 'xlsx') return 'Procesar Excel y programar'
  if (tm === 'now') return `Enviar ahora${count > 0 ? ` (${count})` : ''}`
  if (tm === 'once') return 'Programar envío único'
  return 'Programar recurrente'
}
