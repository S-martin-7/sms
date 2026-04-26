import { useState, type FormEvent } from 'react'
import { Navigate, useLocation, useNavigate } from 'react-router-dom'
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
  const [err, setErr] = useState<string | null>(null)

  if (isAuthenticated) {
    const from = (location.state as { from?: string } | null)?.from ?? '/resumen'
    return <Navigate to={from} replace />
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setErr(null)
    try {
      const res = await login.mutateAsync({ email, password })
      const expiresAt = new Date(res.expires_at).getTime()
      setSession({ email, role: res.role, expiresAt }, res.token)
      navigate('/resumen', { replace: true })
    } catch (e) {
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
          <p className="text-sm text-slate-500">Ingresa para administrar clientes y mensajes.</p>
        </div>
        <Input
          label="Correo"
          type="email"
          autoComplete="email"
          required
          value={email}
          onChange={(e) => setEmail(e.target.value)}
        />
        <Input
          label="Contraseña"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        {err && (
          <div className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700">
            {err}
          </div>
        )}
        <Button type="submit" loading={login.isPending} className="w-full">
          Ingresar
        </Button>
      </form>
    </div>
  )
}
