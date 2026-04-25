import { type ReactNode } from 'react'

interface TabsProps {
  active: string
  onChange: (key: string) => void
  tabs: { key: string; label: string }[]
}

export function Tabs({ active, onChange, tabs }: TabsProps) {
  return (
    <div className="flex gap-2 border-b border-slate-200">
      {tabs.map((t) => (
        <button
          key={t.key}
          type="button"
          onClick={() => onChange(t.key)}
          className={`-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors ${
            t.key === active
              ? 'border-slate-900 text-slate-900'
              : 'border-transparent text-slate-500 hover:text-slate-700'
          }`}
        >
          {t.label}
        </button>
      ))}
    </div>
  )
}

export function TabPanel({ active, key: panelKey, children }: { active: string; key: string; children: ReactNode }) {
  if (active !== panelKey) return null
  return <div className="pt-4">{children}</div>
}
