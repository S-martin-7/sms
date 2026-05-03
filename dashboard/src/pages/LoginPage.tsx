import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
import axios from 'axios'
import { useAuth } from '@/auth/AuthContext'
import { useLogin } from '@/auth/useLogin'
import { errorMessage } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'

export function LoginPage() {
  const { isAuthenticated, setSession } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const login = useLogin()

  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [totpCode, setTotpCode] = useState('')
  const [needTotp, setNeedTotp] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  if (isAuthenticated) {
    const from = (location.state as { from?: string } | null)?.from ?? '/resumen'
    return <Navigate to={from} replace />
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setErr(null)
    try {
      const res = await login.mutateAsync({
        email,
        password,
        totp_code: needTotp ? totpCode : undefined,
      })
      const expiresAt = new Date(res.expires_at).getTime()
      setSession({ email, role: res.role, expiresAt }, res.token)
      navigate('/resumen', { replace: true })
    } catch (e) {
      // 403 totp_required means password was OK; ask for the second factor.
      if (axios.isAxiosError(e) && e.response?.status === 403) {
        const code = (e.response.data as { error?: { code?: string } } | undefined)?.error?.code
        if (code === 'totp_required') {
          setNeedTotp(true)
          setErr(null)
          return
        }
      }
      setErr(errorMessage(e))
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-slate-50 p-6">
      <form
        onSubmit={onSubmit}
        className="w-full max-w-sm space-y-5 rounded-lg border border-slate-200 bg-white p-6 shadow-sm"
      >
        <div>
          <h1 className="text-lg font-semibold text-slate-900">Pasarela SMS · Admin</h1>
          <p className="text-sm text-slate-500">
            {needTotp
              ? 'Ingresa el código de 6 dígitos de tu app autenticadora.'
              : 'Ingresa para administrar clientes y mensajes.'}
          </p>
        </div>
        <Input
          label="Correo"
          type="email"
          autoComplete="email"
          required
          disabled={needTotp}
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <Input
          label="Contraseña"
          type="password"
          autoComplete="current-password"
          required
          disabled={needTotp}
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {needTotp && (
          <Input
            label="Código 2FA"
            type="text"
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern="\d{6}"
            maxLength={6}
            required
            autoFocus
            value={totpCode}
            onChange={(e) => setTotpCode(e.target.value.replace(/\D/g, ''))}
          />
        )}
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {err}
          </div>
        )}
        <Button type="submit" loading={login.isPending} className="w-full">
          {needTotp ? 'Verificar código' : 'Ingresar'}
        </Button>
        {needTotp && (
          <button
            type="button"
            className="block w-full text-xs text-slate-500 hover:text-slate-700"
            onClick={() => {
              setNeedTotp(false)
              setTotpCode('')
              setErr(null)
            }}
          >
            ← Volver
          </button>
        )}
      </form>
    </div>
  )
}
