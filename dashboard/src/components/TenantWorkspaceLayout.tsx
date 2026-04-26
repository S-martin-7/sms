import { createContext, useContext, type ReactNode } from 'react'
import { Link, NavLink, Outlet, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, errorMessage } from '@/api/client'
import type { Tenant } from '@/api/types'
import { Badge } from './ui/Badge'
import { Button } from './ui/Button'
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
    isActive
      ? 'bg-slate-900 text-white'
      : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900'
  }`

const sectionLabel =
  'mt-4 mb-1 px-3 text-[10px] font-semibold uppercase tracking-wider text-slate-400'

export function TenantWorkspaceLayout() {
  const { id: idParam } = useParams<{ id: string }>()
  const tenantId = Number(idParam)
  const qc = useQueryClient()

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
        <Link to="/clientes" className="text-sm text-slate-500 hover:underline">
          ← Volver a Clientes
        </Link>
        <div className="mt-4 rounded-md border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
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

  return (
    <TenantContext.Provider value={ctx}>
      <div className="flex min-h-screen">
        {/* Tenant-scoped sidebar */}
        <aside className="hidden w-60 flex-shrink-0 border-r border-slate-200 bg-white sm:block">
          <div className="border-b border-slate-200 p-3">
            <Link
              to="/clientes"
              className="text-xs text-slate-500 hover:underline"
            >
              ← Clientes
            </Link>
            <div className="mt-2 truncate text-base font-semibold text-slate-900" title={t.name}>
              {t.name}
            </div>
            <div className="mt-1 flex items-center gap-2 text-xs text-slate-500">
              <span>id={t.id}</span>
              <Badge value={t.status} />
            </div>
          </div>

          <nav className="flex flex-col gap-0.5 p-3 pb-6">
            <NavLink to="resumen" className={navLinkClass}>Resumen</NavLink>

            <div className={sectionLabel}>Operación</div>
            <NavLink to="enviar" className={navLinkClass}>Enviar SMS</NavLink>
            <NavLink to="mensajes" className={navLinkClass}>Mensajes</NavLink>
            <NavLink to="contactos" className={navLinkClass}>Contactos</NavLink>
            <NavLink to="reportes" className={navLinkClass}>Reportes</NavLink>

            <div className={sectionLabel}>Configuración</div>
            <NavLink to="llaves" className={navLinkClass}>Llaves API</NavLink>
            <NavLink to="webhooks" className={navLinkClass}>Webhooks</NavLink>
            <NavLink to="entregas" className={navLinkClass}>Entregas webhook</NavLink>
            <NavLink to="numeros" className={navLinkClass}>Números entrantes</NavLink>
          </nav>
        </aside>

        <div className="flex min-w-0 flex-1 flex-col">
          {/* Tenant header bar */}
          <header className="flex h-14 items-center justify-between border-b border-slate-200 bg-white px-4">
            <div className="text-sm text-slate-500">
              Trabajando como cliente: <span className="font-medium text-slate-900">{t.name}</span>
            </div>
            <div className="flex items-center gap-2">
              {t.status === 'active' ? (
                <Button
                  variant="secondary"
                  loading={setStatus.isPending}
                  onClick={() => {
                    if (confirm(`¿Suspender al cliente "${t.name}"? Sus llaves API dejarán de funcionar inmediatamente.`)) {
                      setStatus.mutate('suspend')
                    }
                  }}
                >
                  Suspender cliente
                </Button>
              ) : (
                <Button
                  variant="secondary"
                  loading={setStatus.isPending}
                  onClick={() => setStatus.mutate('activate')}
                >
                  Reactivar cliente
                </Button>
              )}
            </div>
          </header>
          <main className="flex-1 overflow-y-auto p-6">
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
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold text-slate-900">{title}</h1>
        {action}
      </div>
      {children}
    </div>
  )
}
