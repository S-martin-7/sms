import { Link } from 'react-router-dom'
import { Card, CardBody } from '@/components/ui/Card'
import { TenantPage } from '@/components/TenantWorkspaceLayout'

export function TenantContactsPage() {
  return (
    <TenantPage title="Contactos">
      <Card>
        <CardBody className="space-y-4 py-10 text-center">
          <div className="text-3xl">📇</div>
          <div className="text-base font-semibold text-slate-900">Próximamente</div>
          <p className="mx-auto max-w-md text-sm text-slate-500">
            Aquí podrás cargar la libreta de contactos de este cliente, agruparlos en listas
            (por ejemplo "Clientes Premium" o "Sucursales"), y enviar campañas masivas
            seleccionando una lista en lugar de pegar números a mano.
          </p>
          <ul className="mx-auto max-w-md space-y-2 text-left text-sm text-slate-600">
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Importación desde CSV</span>
            </li>
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Listas y segmentación con etiquetas</span>
            </li>
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Anti-duplicados y validación E.164 al cargar</span>
            </li>
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Bajas voluntarias (opt-out) marcadas automáticamente</span>
            </li>
          </ul>
          <div className="pt-3 text-xs text-slate-500">
            Mientras tanto, puedes pegar destinatarios a mano en{' '}
            <Link to="../enviar" relative="path" className="text-slate-700 underline">
              Enviar SMS
            </Link>
            .
          </div>
        </CardBody>
      </Card>
    </TenantPage>
  )
}
