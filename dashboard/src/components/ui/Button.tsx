import { type ButtonHTMLAttributes, type ReactNode } from 'react'

type Variant = 'primary' | 'secondary' | 'danger' | 'ghost'

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant
  loading?: boolean
  children: ReactNode
}

const styles: Record<Variant, string> = {
  primary:
    'bg-slate-900 text-white hover:bg-slate-800 disabled:bg-slate-300 disabled:text-slate-500',
  secondary:
    'bg-white border border-slate-300 text-slate-700 hover:bg-slate-50 disabled:bg-slate-100',
  danger:
    'bg-red-600 text-white hover:bg-red-700 disabled:bg-red-300',
  ghost:
    'bg-transparent text-slate-600 hover:bg-slate-100 disabled:text-slate-400',
}

export function Button({
  variant = 'primary',
  loading = false,
  className = '',
  children,
  disabled,
  ...rest
}: ButtonProps) {
  return (
    <button
      className={`inline-flex items-center justify-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-slate-900/20 ${styles[variant]} ${className}`}
      disabled={disabled || loading}
      {...rest}
    >
      {loading && (
        <svg className="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none">
          <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" className="opacity-25" />
          <path d="M4 12a8 8 0 018-8" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
        </svg>
      )}
      {children}
    </button>
  )
}
