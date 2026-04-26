import { Link } from 'react-router-dom'
import { Card, CardBody } from '@/components/ui/Card'
import { TenantPage } from '@/components/TenantWorkspaceLayout'

export function TenantReportsPage() {
  return (
    <TenantPage title="Reportes">
      <Card>
        <CardBody className="space-y-4 py-10 text-center">
          <div className="text-3xl">📊</div>
          <div className="text-base font-semibold text-slate-900">Próximamente</div>
          <p className="mx-auto max-w-md text-sm text-slate-500">
            Reportes descargables (PDF / CSV) y programados por correo para este cliente.
          </p>
          <ul className="mx-auto max-w-md space-y-2 text-left text-sm text-slate-600">
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Volumen diario/semanal/mensual con tasa de entrega</span>
            </li>
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Detalle por campaña (lote) con costos estimados</span>
            </li>
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Listado de números fallidos con razón</span>
            </li>
            <li className="flex gap-2">
              <span className="text-slate-400">•</span>
              <span>Programación: enviar el reporte por correo cada lunes 9 AM</span>
            </li>
          </ul>
          <div className="pt-3 text-xs text-slate-500">
            Por ahora puedes ver volumen agregado en{' '}
            <Link to="/resumen" className="text-slate-700 underline">Resumen global</Link>.
          </div>
        </CardBody>
      </Card>
    </TenantPage>
  )
}
