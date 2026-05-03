import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from '@/auth/AuthContext'
import { ProtectedRoute } from '@/auth/ProtectedRoute'
import { Layout } from '@/components/Layout'
import { TenantWorkspaceLayout } from '@/components/TenantWorkspaceLayout'
import { LoginPage } from '@/pages/LoginPage'
import { OverviewPage } from '@/pages/OverviewPage'
import { TenantsPage } from '@/pages/TenantsPage'
import { MessagesPage } from '@/pages/MessagesPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { NotFoundPage } from '@/pages/NotFoundPage'
import { TenantOverviewPage } from '@/pages/tenant/TenantOverviewPage'
import { TenantSendPage } from '@/pages/tenant/TenantSendPage'
import { TenantMessagesPage } from '@/pages/tenant/TenantMessagesPage'
import { TenantKeysPage } from '@/pages/tenant/TenantKeysPage'
import { TenantWebhooksPage } from '@/pages/tenant/TenantWebhooksPage'
import { TenantDeliveriesPage } from '@/pages/tenant/TenantDeliveriesPage'
import { TenantInboundPage } from '@/pages/tenant/TenantInboundPage'
import { TenantContactsPage } from '@/pages/tenant/TenantContactsPage'
import { TenantReportsPage } from '@/pages/tenant/TenantReportsPage'
import { TenantScheduledPage } from '@/pages/tenant/TenantScheduledPage'

export default function App() {
  return (
    <AuthProvider>
      <HashRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />

          {/* Super-admin layout — visión cross-tenant. */}
          <Route
            element={
              <ProtectedRoute>
                <Layout />
              </ProtectedRoute>
            }
          >
            <Route path="/resumen" element={<OverviewPage />} />
            <Route path="/clientes" element={<TenantsPage />} />
            <Route path="/mensajes" element={<MessagesPage />} />
            <Route path="/cuenta" element={<SettingsPage />} />
            <Route path="/" element={<Navigate to="/resumen" replace />} />
          </Route>

          {/* Workspace de cliente — sidebar y operaciones scoped al tenant. */}
          <Route
            path="/clientes/:id"
            element={
              <ProtectedRoute>
                <TenantWorkspaceLayout />
              </ProtectedRoute>
            }
          >
            <Route index element={<Navigate to="resumen" replace />} />
            <Route path="resumen" element={<TenantOverviewPage />} />
            <Route path="enviar" element={<TenantSendPage />} />
            <Route path="programados" element={<TenantScheduledPage />} />
            <Route path="mensajes" element={<TenantMessagesPage />} />
            <Route path="contactos" element={<TenantContactsPage />} />
            <Route path="reportes" element={<TenantReportsPage />} />
            <Route path="llaves" element={<TenantKeysPage />} />
            <Route path="webhooks" element={<TenantWebhooksPage />} />
            <Route path="entregas" element={<TenantDeliveriesPage />} />
            <Route path="numeros" element={<TenantInboundPage />} />
          </Route>

          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </HashRouter>
    </AuthProvider>
  )
}
