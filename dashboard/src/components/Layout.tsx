import { useEffect, useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '@/auth/AuthContext'

const SECTION_LABEL = 'mt-4 mb-1 px-3 text-[10px] font-semibold uppercase tracking-[0.14em] text-ink-faint'

export function Layout() {
  const { logout, session } = useAuth()
  const [mobileOpen, setMobileOpen] = useState(false)
  // Cerrar sidebar al cambiar ruta (en móvil) — escucha popstate del HashRouter.
  useEffect(() => {
    const onHash = () => setMobileOpen(false)
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  return (
    <div className="flex min-h-screen">
      {/* Sidebar — fija en desktop, drawer overlay en móvil */}
      <aside
        className={`fixed inset-y-0 left-0 z-40 w-60 transform border-r border-border bg-surface transition-transform sm:relative sm:translate-x-0 ${
          mobileOpen ? 'translate-x-0' : '-translate-x-full sm:translate-x-0'
        }`}
      >
        <SidebarContent onClose={() => setMobileOpen(false)} />
      </aside>

      {/* Backdrop oscurece en móvil cuando sidebar abierto */}
      {mobileOpen && (
        <div
          className="fixed inset-0 z-30 bg-ink/40 sm:hidden"
          onClick={() => setMobileOpen(false)}
          aria-hidden
        />
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between border-b border-border bg-surface px-4">
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => setMobileOpen(true)}
              className="rounded-md p-1.5 text-ink-soft hover:bg-muted hover:text-ink sm:hidden"
              aria-label="Abrir menú"
            >
              <svg className="h-5 w-5" fill="none" stroke="currentColor" strokeWidth="2" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </button>
            <span className="font-display text-base font-semibold text-ink sm:hidden">Pasarela SMS</span>
            <div className="hidden text-sm text-ink-mute sm:block">Panel administrativo</div>
          </div>
          <div className="flex items-center gap-3">
            {session?.email && <span className="hidden text-sm text-ink-soft md:inline">{session.email}</span>}
            <button
              onClick={logout}
              className="rounded-md px-2.5 py-1 text-sm text-ink-soft hover:bg-muted hover:text-ink"
            >
              Salir
            </button>
          </div>
        </header>
        <main className="flex-1 overflow-y-auto p-4 sm:p-6">
          <Outlet />
        </main>
      </div>
    </div>
  )
}

// Compartido para que el drawer móvil tenga exactamente el mismo contenido
// que el sidebar desktop.
function SidebarContent({ onClose }: { onClose: () => void }) {
  const linkClass = ({ isActive }: { isActive: boolean }) =>
    `block rounded-md px-3 py-2 text-sm font-medium transition-colors ${
      isActive ? 'bg-ink text-canvas' : 'text-ink-soft hover:bg-muted hover:text-ink'
    }`

  return (
    <div className="flex h-full flex-col">
      <div className="flex h-14 items-center justify-between border-b border-border px-4">
        <span className="font-display text-base font-semibold text-ink">Pasarela SMS</span>
        <button
          type="button"
          onClick={onClose}
          className="rounded-md p-1 text-ink-faint hover:bg-muted hover:text-ink sm:hidden"
          aria-label="Cerrar menú"
        >
          ✕
        </button>
      </div>
      <nav className="flex flex-col gap-0.5 p-3">
        <NavLink to="/resumen" className={linkClass}>Resumen global</NavLink>
        <NavLink to="/clientes" className={linkClass}>Clientes</NavLink>
        <div className={SECTION_LABEL}>Auditoría</div>
        <NavLink to="/mensajes" className={linkClass}>Mensajes (todos)</NavLink>
      </nav>
      <div className="mt-auto px-3 pb-4">
        <div className="rounded-md border border-border bg-canvas px-3 py-2 text-xs text-ink-mute">
          <strong className="block text-ink-soft">Tip</strong>
          Para configurar webhooks, mandar SMS o ver mensajes de un cliente,
          entra desde <span className="font-medium text-ink">Clientes</span>.
        </div>
      </div>
    </div>
  )
}
