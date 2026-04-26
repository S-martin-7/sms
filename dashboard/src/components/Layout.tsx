import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import { Button } from './ui/Button'

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  `block rounded-md px-3 py-2 text-sm font-medium transition-colors ${
    isActive
      ? 'bg-slate-900 text-white'
      : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
  }`

export function Layout() {
  const { logout, session } = useAuth()
  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-56 flex-shrink-0 border-r border-slate-200 bg-white sm:block">
        <div className="flex h-14 items-center border-b border-slate-200 px-4">
          <span className="text-base font-semibold text-slate-900">Pasarela SMS</span>
        </div>
        <nav className="flex flex-col gap-1 p-3">
          <NavLink to="/resumen" className={navLinkClass}>
            Resumen
          </NavLink>
          <NavLink to="/enviar" className={navLinkClass}>
            Enviar SMS
          </NavLink>
          <NavLink to="/clientes" className={navLinkClass}>
            Clientes
          </NavLink>
          <NavLink to="/mensajes" className={navLinkClass}>
            Mensajes
          </NavLink>
          <NavLink to="/numeros-entrantes" className={navLinkClass}>
            Números entrantes
          </NavLink>
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-slate-200 bg-white px-4">
          <div className="text-sm text-slate-500">Panel administrativo</div>
          <div className="flex items-center gap-3">
            {session?.email && (
              <span className="text-sm text-slate-600">{session.email}</span>
            )}
            <Button variant="ghost" onClick={logout}>
              Cerrar sesión
            </Button>
          </div>
        </header>
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
