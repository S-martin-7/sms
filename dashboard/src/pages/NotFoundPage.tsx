import { Link } from 'react-router-dom'

export function NotFoundPage() {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-3 text-center">
      <h1 className="text-2xl font-semibold text-slate-900">404</h1>
      <p className="text-slate-500">Esa página no existe.</p>
      <Link to="/resumen" className="text-sm text-slate-700 hover:underline">
        ← Volver al resumen
      </Link>
    </div>
  )
}
