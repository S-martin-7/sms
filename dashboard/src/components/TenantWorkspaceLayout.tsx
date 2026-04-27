import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { Link, NavLink, Outlet, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { Tenant } from '@/api/types'
import { Badge } from './ui/Badge'
import { Spinner } from './ui/Spinner'

// TenantContext — pages mounted under /clientes/:id/* read this instead of
// repeatedly calling /admin/tenants/:id. Provided by TenantWorkspaceLayout.
interface TenantCtx {
  tenant: Tenant
  refresh: () => void
}
const TenantContext = createContext<TenantCtx | null>(null)
export function useTenant(): TenantCtx {
  const ctx = useContext(TenantContext)
  if (!ctx) throw new Error('useTenant must be used inside <TenantWorkspaceLayout>')
  return ctx
}

const navLinkClass = ({ isActive }: { isActive: boolean }) =>
  `block rounded-md px-3 py-2 text-sm font-medium transition-colors ${
    isActive ? 'bg-ink text-canvas' : 'text-ink-soft hover:bg-muted hover:text-ink'
  }`

const SECTION_LABEL = 'mt-4 mb-1 px-3 text-[10px] font-semibold uppercase tracking-[0.14em] text-ink-faint'

export function TenantWorkspaceLayout() {
  const { id: idParam } = useParams<{ id: string }>()
  const tenantId = Number(idParam)
  const qc = useQueryClient()

  const [mobileOpen, setMobileOpen] = useState(false)
  useEffect(() => {
    const onHash = () => setMobileOpen(false)
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const tenant = useQuery({
    queryKey: ['tenant', tenantId],
    queryFn: async () => {
      const { data } = await api.get<Tenant>(`/admin/tenants/${tenantId}`)
      return data
    },
  })

  const setStatus = useMutation({
    mutationFn: async (action: 'suspend' | 'activate') => {
      await api.post(`/admin/tenants/${tenantId}/${action}`)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId] }),
  })

  if (tenant.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner />
      </div>
    )
  }
  if (tenant.error || !tenant.data) {
    return (
      <div className="p-6">
        <Link to="/clientes" className="text-sm text-ink-mute hover:underline">
          ← Volver a Clientes
        </Link>
        <div className="mt-4 rounded-md border border-danger/30 bg-danger-soft px-4 py-3 text-sm text-danger-ink">
          {errorMessage(tenant.error)}
        </div>
      </div>
    )
  }

  const t = tenant.data
  const ctx: TenantCtx = {
    tenant: t,
    refresh: () => qc.invalidateQueries({ queryKey: ['tenant', tenantId] }),
  }

  const sidebar = (
    <div className="flex h-full flex-col">
      <div className="border-b border-border p-3">
        <Link to="/clientes" className="text-xs text-ink-mute hover:underline">← Clientes</Link>
        <div className="mt-2 truncate font-display text-base font-semibold text-ink" title={t.name}>
          {t.name}
        </div>
        <div className="mt-1 flex items-center gap-2 text-xs text-ink-mute">
          <span className="tabular">id={t.id}</span>
          <Badge value={t.status} />
        </div>
      </div>
      <nav className="flex flex-col gap-0.5 overflow-y-auto p-3 pb-6">
        <NavLink to="resumen" className={navLinkClass}>Resumen</NavLink>
        <div className={SECTION_LABEL}>Operación</div>
        <NavLink to="enviar" className={navLinkClass}>Enviar SMS</NavLink>
        <NavLink to="programados" className={navLinkClass}>Programados</NavLink>
        <NavLink to="mensajes" className={navLinkClass}>Mensajes</NavLink>
        <NavLink to="contactos" className={navLinkClass}>Contactos</NavLink>
        <NavLink to="reportes" className={navLinkClass}>Reportes</NavLink>
        <div className={SECTION_LABEL}>Configuración</div>
        <NavLink to="llaves" className={navLinkClass}>Llaves API</NavLink>
        <NavLink to="webhooks" className={navLinkClass}>Webhooks</NavLink>
        <NavLink to="entregas" className={navLinkClass}>Entregas webhook</NavLink>
        <NavLink to="numeros" className={navLinkClass}>Números entrantes</NavLink>
      </nav>
    </div>
  )

  return (
    <TenantContext.Provider value={ctx}>
      <div className="flex min-h-screen">
        {/* Sidebar — fija en desktop, drawer en móvil */}
        <aside
          className={`fixed inset-y-0 left-0 z-40 w-60 transform border-r border-border bg-surface transition-transform sm:relative sm:translate-x-0 ${
            mobileOpen ? 'translate-x-0' : '-translate-x-full sm:translate-x-0'
          }`}
        >
          {sidebar}
        </aside>
        {mobileOpen && (
          <div className="fixed inset-0 z-30 bg-ink/40 sm:hidden" onClick={() => setMobileOpen(false)} aria-hidden />
        )}

        <div className="flex min-w-0 flex-1 flex-col">
          <header className="flex h-14 items-center justify-between gap-3 border-b border-border bg-surface px-4">
            <div className="flex min-w-0 items-center gap-3">
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
              <div className="min-w-0 truncate text-sm text-ink-mute">
                <span className="hidden md:inline">Cliente: </span>
                <span className="font-medium text-ink">{t.name}</span>
              </div>
            </div>
            <div className="flex items-center gap-2">
              {t.status === 'active' ? (
                <button
                  disabled={setStatus.isPending}
                  onClick={() => {
                    if (confirm(`¿Suspender al cliente "${t.name}"? Sus llaves API dejarán de funcionar.`)) {
                      setStatus.mutate('suspend')
                    }
                  }}
                  className="rounded-md border border-border bg-surface px-3 py-1 text-xs text-ink-soft hover:border-ink hover:text-ink sm:text-sm"
                >
                  Suspender
                </button>
              ) : (
                <button
                  disabled={setStatus.isPending}
                  onClick={() => setStatus.mutate('activate')}
                  className="rounded-md border border-border bg-surface px-3 py-1 text-xs text-ink-soft hover:border-ink hover:text-ink sm:text-sm"
                >
                  Reactivar
                </button>
              )}
            </div>
          </header>
          <main className="flex-1 overflow-y-auto p-4 sm:p-6">
            <Outlet />
          </main>
        </div>
      </div>
    </TenantContext.Provider>
  )
}

// PageContainer — light wrapper used by tenant subpages so they share padding
// and a heading layout without each page reinventing it.
export function TenantPage({ title, action, children }: {
  title: string
  action?: ReactNode
  children: ReactNode
}) {
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="font-display text-2xl font-medium tracking-tight text-ink">{title}</h1>
        {action}
      </div>
      {children}
    </div>
  )
}
