import type { Message } from '@/api/types'
import { formatDate } from '@/lib/format'

// Courier-tracking style: 4 nodes connected by a line. Each completed
// step is a filled circle with a checkmark; pending steps are hollow
// rings; failed/rejected breaks the line into a dashed segment with a
// red node. Inspired by tracking UIs (Pedido → En tránsito → Entregado).
interface Step {
  key: string
  label: string
  description: string
  at?: string | null
  state: 'done' | 'fail' | 'pending'
}

export function MessageTimeline({ msg }: { msg: Message }) {
  const steps: Step[] = buildSteps(msg)
  return (
    <div className="grid grid-cols-4 gap-0">
      {steps.map((s, i) => (
        <Node key={s.key} step={s} isLast={i === steps.length - 1} />
      ))}
    </div>
  )
}

function buildSteps(m: Message): Step[] {
  const failed = m.status === 'rejected' || m.status === 'failed' || m.status === 'undelivered'

  const created: Step = {
    key: 'created', label: 'Creado',
    description: 'El mensaje se recibió en la cola interna',
    at: m.created_at,
    state: 'done',
  }
  const sent: Step = {
    key: 'sent', label: 'Enviado',
    description: 'Aceptado por la pasarela Horisen',
    at: m.sent_at,
    state: m.sent_at ? 'done' : (failed && m.status === 'rejected' ? 'fail' : 'pending'),
  }
  const enroute: Step = {
    key: 'enroute', label: 'En entrega',
    description: 'En camino al teléfono del destinatario',
    at: m.sent_at,
    state: m.sent_at && !failed ? 'done' : 'pending',
  }

  let lastLabel = 'Entregado'
  let lastDesc = 'El destinatario recibió el SMS'
  let lastState: Step['state'] = 'pending'
  if (m.status === 'delivered') {
    lastState = 'done'
  } else if (m.status === 'undelivered') {
    lastLabel = 'No entregado'
    lastDesc = errLine(m) || 'El operador no pudo entregarlo'
    lastState = 'fail'
  } else if (m.status === 'rejected') {
    lastLabel = 'Rechazado'
    lastDesc = errLine(m) || 'Horisen rechazó el envío'
    lastState = 'fail'
  } else if (m.status === 'failed') {
    lastLabel = 'Fallido'
    lastDesc = errLine(m) || 'Falla técnica al enviar'
    lastState = 'fail'
  }
  const last: Step = {
    key: 'last', label: lastLabel, description: lastDesc, at: m.final_at, state: lastState,
  }
  return [created, sent, enroute, last]
}

function errLine(m: Message): string | null {
  if (!m.error_code && !m.error_message) return null
  return `${m.error_code ? `[${m.error_code}] ` : ''}${m.error_message ?? ''}`.trim()
}

function Node({ step, isLast }: { step: Step; isLast: boolean }) {
  const stateStyles = {
    done:    { ring: 'bg-success text-canvas border-success', line: 'bg-success' },
    fail:    { ring: 'bg-danger text-canvas border-danger',  line: 'bg-danger' },
    pending: { ring: 'bg-canvas text-ink-faint border-border', line: 'bg-border' },
  }[step.state]

  return (
    <div className="relative flex flex-col items-center text-center">
      {/* Connector to the next node — drawn behind the circle. */}
      {!isLast && (
        <div className="absolute left-1/2 top-5 h-0.5 w-full">
          <div className={`h-full ${step.state === 'done' ? 'bg-success' : step.state === 'fail' ? 'bg-danger' : 'bg-border'}`} />
        </div>
      )}
      {/* Node circle */}
      <div className={`relative z-10 flex h-10 w-10 items-center justify-center rounded-full border-2 ${stateStyles.ring}`}>
        {step.state === 'done' && (
          <svg viewBox="0 0 20 20" className="h-5 w-5 stroke-current" fill="none" strokeWidth="2.4" strokeLinecap="round" strokeLinejoin="round">
            <path d="M5 10 l3 3 7-7" />
          </svg>
        )}
        {step.state === 'fail' && (
          <svg viewBox="0 0 20 20" className="h-4 w-4 stroke-current" fill="none" strokeWidth="2.6" strokeLinecap="round">
            <path d="M5 5 l10 10 M15 5 l-10 10" />
          </svg>
        )}
        {step.state === 'pending' && (
          <span className="h-2 w-2 rounded-full bg-ink-faint/50" />
        )}
      </div>
      {/* Label */}
      <div className="mt-3 px-2">
        <div className={`text-sm font-semibold uppercase tracking-wider ${
          step.state === 'done' ? 'text-success-ink' : step.state === 'fail' ? 'text-danger-ink' : 'text-ink-mute'
        }`}>
          {step.label}
        </div>
        <p className={`mt-1 text-xs leading-snug ${step.state === 'pending' ? 'text-ink-faint' : 'text-ink-soft'}`}>
          {step.description}
        </p>
        {step.at && (
          <p className="mt-1 text-[11px] tabular text-ink-mute">
            {formatDate(step.at)}
          </p>
        )}
      </div>
    </div>
  )
}
