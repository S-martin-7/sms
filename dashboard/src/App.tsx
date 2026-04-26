import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from '@/auth/AuthContext'
import { ProtectedRoute } from '@/auth/ProtectedRoute'
import { Layout } from '@/components/Layout'
import { LoginPage } from '@/pages/LoginPage'
import { OverviewPage } from '@/pages/OverviewPage'
import { SendSMSPage } from '@/pages/SendSMSPage'
import { TenantsPage } from '@/pages/TenantsPage'
import { TenantDetailPage } from '@/pages/TenantDetailPage'
import { MessagesPage } from '@/pages/MessagesPage'
import { InboundNumbersPage } from '@/pages/InboundNumbersPage'
import { NotFoundPage } from '@/pages/NotFoundPage'

export default function App() {
  return (
    <AuthProvider>
      <HashRouter>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route
            element={
              <ProtectedRoute>
                <Layout />
              </ProtectedRoute>
            }
          >
            <Route path="/resumen" element={<OverviewPage />} />
            <Route path="/enviar" element={<SendSMSPage />} />
            <Route path="/clientes" element={<TenantsPage />} />
            <Route path="/clientes/:id" element={<TenantDetailPage />} />
            <Route path="/mensajes" element={<MessagesPage />} />
            <Route path="/numeros-entrantes" element={<InboundNumbersPage />} />
            <Route path="/" element={<Navigate to="/resumen" replace />} />
          </Route>
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </HashRouter>
    </AuthProvider>
  )
}
