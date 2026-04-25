import { forwardRef, type InputHTMLAttributes } from 'react'

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string
  error?: string
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { label, error, className = '', id, ...rest },
  ref,
) {
  const inputId = id ?? rest.name
  return (
    <div className="flex flex-col gap-1">
      {label && (
        <label htmlFor={inputId} className="text-sm font-medium text-slate-700">
          {label}
        </label>
      )}
      <input
        ref={ref}
        id={inputId}
        className={`rounded-md border px-3 py-2 text-sm placeholder:text-slate-400 focus:outline-none focus:ring-2 focus:ring-slate-900/20 ${
          error ? 'border-red-400' : 'border-slate-300'
        } ${className}`}
        {...rest}
      />
      {error && <span className="text-xs text-red-600">{error}</span>}
    </div>
  )
})
