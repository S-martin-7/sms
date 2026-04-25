import { type ReactNode, type TdHTMLAttributes, type ThHTMLAttributes } from 'react'

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full divide-y divide-slate-200 text-sm">{children}</table>
    </div>
  )
}

export function THead({ children }: { children: ReactNode }) {
  return <thead className="bg-slate-50">{children}</thead>
}

export function TBody({ children }: { children: ReactNode }) {
  return <tbody className="divide-y divide-slate-100 bg-white">{children}</tbody>
}

export function TR({ children, className = '', onClick }: { children: ReactNode; className?: string; onClick?: () => void }) {
  return (
    <tr
      className={`${onClick ? 'cursor-pointer hover:bg-slate-50' : ''} ${className}`}
      onClick={onClick}
    >
      {children}
    </tr>
  )
}

export function TH({ children, className = '', ...rest }: ThHTMLAttributes<HTMLTableCellElement> & { children: ReactNode }) {
  return (
    <th
      className={`px-3 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500 ${className}`}
      {...rest}
    >
      {children}
    </th>
  )
}

export function TD({ children, className = '', ...rest }: TdHTMLAttributes<HTMLTableCellElement> & { children: ReactNode }) {
  return (
    <td className={`px-3 py-2 text-slate-700 ${className}`} {...rest}>
      {children}
    </td>
  )
}
