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
  const { logout } = useAuth()
  return (
    <div className="flex min-h-screen">
      <aside className="hidden w-56 flex-shrink-0 border-r border-slate-200 bg-white sm:block">
        <div className="flex h-14 items-center border-b border-slate-200 px-4">
          <span className="text-base font-semibold text-slate-900">SMS Gateway</span>
        </div>
        <nav className="flex flex-col gap-1 p-3">
          <NavLink to="/tenants" className={navLinkClass}>
            Tenants
          </NavLink>
          <NavLink to="/messages" className={navLinkClass}>
            Messages
          </NavLink>
          <NavLink to="/inbound-numbers" className={navLinkClass}>
            Inbound numbers
          </NavLink>
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-slate-200 bg-white px-4">
          <div className="text-sm text-slate-500">Admin dashboard</div>
          <Button variant="ghost" onClick={logout}>
            Logout
          </Button>
        </header>
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}
