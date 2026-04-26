// Types for /admin/stats. Mirror of internal/httpapi/handlers_admin_stats.go.

export interface StatsTotals {
  total: number
  queued: number
  sent: number
  delivered: number
  undelivered: number
  rejected: number
  delivery_rate: number  // 0..1
}

export interface StatsByTenant {
  tenant_id: number
  name: string
  total: number
  delivered: number
  rejected: number
}

export interface StatsRecentFailure {
  id: string
  tenant_id: number
  recipient: string
  status: string
  error_code?: string | null
  error_message?: string | null
  created_at: string
}

export interface StatsStuckDelivery {
  id: number
  tenant_id: number
  endpoint_id: number
  event_type: string
  status: string
  attempts: number
  last_status?: number | null
  last_error?: string | null
  created_at: string
}

export interface StatsResp {
  window_hours: number
  totals: StatsTotals
  top_tenants: StatsByTenant[]
  recent_failures: StatsRecentFailure[]
  stuck_deliveries: StatsStuckDelivery[]
}
