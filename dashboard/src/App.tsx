import { HashRouter, Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider } from '@/auth/AuthContext'
import { ProtectedRoute } from '@/auth/ProtectedRoute'
import { Layout } from '@/components/Layout'
import { LoginPage } from '@/pages/LoginPage'
import { TenantsPage } from '@/pages/TenantsPage'
import { TenantDetailPage } from '@/pages/TenantDetailPage'
import { MessagesPage } from '@/pages/MessagesPage'
import { InboundNumbersPage } from '@/pages/InboundNumbersPage'
import { NotFoundPage } from '@/pages/NotFoundPage'

// HashRouter — clean URL routing requires nginx try_files which IS already
// in the planned /dashboard/ block, but hash routes are simpler to debug
// and keep the bundle agnostic to any subpath rewrites.
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
            <Route path="/tenants" element={<TenantsPage />} />
            <Route path="/tenants/:id" element={<TenantDetailPage />} />
            <Route path="/messages" element={<MessagesPage />} />
            <Route path="/inbound-numbers" element={<InboundNumbersPage />} />
            <Route path="/" element={<Navigate to="/tenants" replace />} />
          </Route>
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </HashRouter>
    </AuthProvider>
  )
}
