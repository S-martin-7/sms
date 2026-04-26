import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'
import { Button } from './ui/Button'

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  `block rounded-md px-3 py-2 text-sm font-medium transition-colors ${
    isActive
      ? 'bg-slate-900 text-white'
      : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
  }`

const sectionLabel =
  'mt-4 mb-1 px-3 text-[10px] font-semibold uppercase tracking-wider text-slate-400'

export function Layout() {
  const { logout, session } = useAuth()
  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-56 flex-shrink-0 border-r border-slate-200 bg-white sm:block">
        <div className="flex h-14 items-center border-b border-slate-200 px-4">
          <span className="text-base font-semibold text-slate-900">Pasarela SMS</span>
        </div>
        <nav className="flex flex-col gap-0.5 p-3">
          <NavLink to="/resumen" className={navLinkClass}>Resumen global</NavLink>
          <NavLink to="/clientes" className={navLinkClass}>Clientes</NavLink>
          <div className={sectionLabel}>Auditoría</div>
          <NavLink to="/mensajes" className={navLinkClass}>Mensajes (todos)</NavLink>
        </nav>
        <div className="px-3 pb-3 pt-6">
          <div className="rounded-md border border-slate-200 bg-slate-50 px-3 py-2 text-xs text-slate-500">
            <strong className="block text-slate-700">Tip</strong>
            Para configurar webhooks, mandar SMS o ver mensajes de un cliente,
            entra desde <span className="font-medium">Clientes</span>.
          </div>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-slate-200 bg-white px-4">
          <div className="text-sm text-slate-500">Panel administrativo</div>
          <div className="flex items-center gap-3">
            {session?.email && (
              <span className="text-sm text-slate-600">{session.email}</span>
            )}
            <Button variant="ghost" onClick={logout}>Cerrar sesión</Button>
          </div>
        </header>
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
