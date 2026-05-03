import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { QRCodeSVG } from 'qrcode.react'
import { api, errorMessage } from '@/api/client'
import { Card, CardBody, CardHeader } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Spinner } from '@/components/ui/Spinner'

interface MeResponse {
  id: number
  email: string
  role: string
  totp_enabled: boolean
}

interface TOTPSetupResponse {
  secret: string
  uri: string
}

export function SettingsPage() {
  const qc = useQueryClient()
  const me = useQuery({
    queryKey: ['me'],
    queryFn: async () => (await api.get<MeResponse>('/admin/me')).data,
  })

  if (me.isLoading) return <Spinner />
  if (me.isError) return <p className="text-red-600">{errorMessage(me.error)}</p>
  if (!me.data) return null

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="font-display text-xl font-semibold text-ink">Mi cuenta</h1>
        <p className="text-sm text-ink-mute">{me.data.email} · rol {me.data.role}</p>
      </div>
      <TOTPSection enabled={me.data.totp_enabled} onChanged={() => qc.invalidateQueries({ queryKey: ['me'] })} />
    </div>
  )
}

function TOTPSection({ enabled, onChanged }: { enabled: boolean; onChanged: () => void }) {
  return (
    <Card>
      <CardHeader>
        <div>
          <h2 className="text-base font-semibold text-ink">Autenticación de dos factores (TOTP)</h2>
          <p className="text-xs text-ink-mute">
            {enabled
              ? 'Está activa. Cada inicio de sesión requiere un código de 6 dígitos.'
              : 'Recomendado. Protege tu cuenta incluso si alguien obtuviera tu contraseña.'}
          </p>
        </div>
      </CardHeader>
      <CardBody>
        {enabled ? <DisableTOTP onChanged={onChanged} /> : <EnrollTOTP onChanged={onChanged} />}
      </CardBody>
    </Card>
  )
}

function EnrollTOTP({ onChanged }: { onChanged: () => void }) {
  const [setup, setSetup] = useState<TOTPSetupResponse | null>(null)
  const [code, setCode] = useState('')
  const [err, setErr] = useState<string | null>(null)

  const start = useMutation({
    mutationFn: async () => (await api.post<TOTPSetupResponse>('/admin/me/totp/setup')).data,
    onSuccess: (d) => {
      setSetup(d)
      setErr(null)
    },
    onError: (e) => setErr(errorMessage(e)),
  })

  const enable = useMutation({
    mutationFn: async () => api.post('/admin/me/totp/enable', { code }),
    onSuccess: () => {
      setSetup(null)
      setCode('')
      setErr(null)
      onChanged()
    },
    onError: (e) => setErr(errorMessage(e)),
  })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    enable.mutate()
  }

  if (!setup) {
    return (
      <div className="space-y-3">
        <p className="text-sm text-ink-soft">
          Generaremos un secreto único. Escanéalo con Google Authenticator, Authy, 1Password u otra app TOTP.
        </p>
        <Button onClick={() => start.mutate()} loading={start.isPending}>
          Activar 2FA
        </Button>
        {err && <p className="text-sm text-red-600">{err}</p>}
      </div>
    )
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="flex flex-col items-center gap-3 sm:flex-row sm:items-start">
        <div className="rounded-md border border-border bg-canvas p-3">
          <QRCodeSVG value={setup.uri} size={176} />
        </div>
        <div className="space-y-2 text-sm">
          <p className="text-ink-soft">
            Escanea el código QR. Si no puedes, ingresa el secreto manualmente:
          </p>
          <code className="block break-all rounded bg-muted px-2 py-1 font-mono text-xs">
            {setup.secret}
          </code>
        </div>
      </div>
      <Input
        label="Código de verificación (6 dígitos)"
        type="text"
        inputMode="numeric"
        pattern="\d{6}"
        maxLength={6}
        autoFocus
        required
        value={code}
        onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
      />
      {err && <p className="text-sm text-red-600">{err}</p>}
      <div className="flex gap-2">
        <Button type="submit" loading={enable.isPending} disabled={code.length !== 6}>
          Confirmar y activar
        </Button>
        <Button
          type="button"
          variant="ghost"
          onClick={() => {
            setSetup(null)
            setCode('')
            setErr(null)
          }}
        >
          Cancelar
        </Button>
      </div>
    </form>
  )
}

function DisableTOTP({ onChanged }: { onChanged: () => void }) {
  const [code, setCode] = useState('')
  const [err, setErr] = useState<string | null>(null)

  const disable = useMutation({
    mutationFn: async () => api.post('/admin/me/totp/disable', { code }),
    onSuccess: () => {
      setCode('')
      setErr(null)
      onChanged()
    },
    onError: (e) => setErr(errorMessage(e)),
  })

  const onSubmit = (e: FormEvent) => {
    e.preventDefault()
    disable.mutate()
  }

  return (
    <form onSubmit={onSubmit} className="space-y-3">
      <p className="text-sm text-ink-soft">
        Para desactivar 2FA ingresa el código actual de tu app autenticadora.
      </p>
      <Input
        label="Código actual"
        type="text"
        inputMode="numeric"
        pattern="\d{6}"
        maxLength={6}
        required
        value={code}
        onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
      />
      {err && <p className="text-sm text-red-600">{err}</p>}
      <Button type="submit" variant="danger" loading={disable.isPending} disabled={code.length !== 6}>
        Desactivar 2FA
      </Button>
    </form>
  )
}
