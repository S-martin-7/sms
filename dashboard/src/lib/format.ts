// Date formatting helpers. Browser locale is fine for an internal tool —
// the API returns RFC3339 timestamps so we can hand them straight to Date.

export function formatDate(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString()
}

export function formatRelative(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso).getTime()
  const diff = Date.now() - d
  const sec = Math.round(diff / 1000)
  if (sec < 60) return `${sec}s ago`
  const min = Math.round(sec / 60)
  if (min < 60) return `${min}m ago`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h ago`
  const day = Math.round(hr / 24)
  return `${day}d ago`
}

export function truncate(s: string | null | undefined, max = 10): string {
  if (!s) return ''
  if (s.length <= max) return s
  return s.slice(0, max) + '…'
}
