export function Spinner({ size = 4 }: { size?: number }) {
  return (
    <svg
      className={`h-${size} w-${size} animate-spin text-slate-400`}
      viewBox="0 0 24 24"
      fill="none"
    >
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" className="opacity-25" />
      <path d="M4 12a8 8 0 018-8" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  )
}
