// Mirrors of the Admin API response shapes. Keep in sync with internal/httpapi/handlers_admin_*.go.

export interface LoginResponse {
  token: string
  expires_at: string
  role: string
}

export interface Tenant {
  id: number
  name: string
  status: string                    // "active" | "suspended"
  daily_sms_limit?: number | null
  created_at: string
  updated_at: string
}

export interface APIKey {
  id: number
  tenant_id: number
  prefix: string
  name?: string | null
  scopes: string[]
  last_used_at?: string | null
  revoked_at?: string | null
  created_at: string
  // Only present in the response of POST (issue), never in GET.
  token?: string
}

export interface Message {
  id: string
  tenant_id: number
  sender: string
  to: string
  text: string
  dcs: string                       // "GSM" | "UCS"
  num_parts: number
  status: string
  horisen_msg_id?: string | null
  error_code?: string | null
  error_message?: string | null
  client_ref?: string | null
  attempts: number
  created_at: string
  sent_at?: string | null
  final_at?: string | null
}

export interface WebhookEndpoint {
  id: number
  url: string
  events: string[]
  active: boolean
  created_at: string
  // Only on create.
  secret?: string
}

export interface WebhookDelivery {
  id: number
  endpoint_id: number
  tenant_id: number
  event_id: string
  event_type: string
  status: string                    // pending|in_flight|success|failed|dead
  attempts: number
  next_attempt_at: string
  last_status?: number | null
  last_error?: string | null
  created_at: string
  delivered_at?: string | null
}

export interface InboundNumber {
  msisdn: string
  tenant_id: number
  label?: string
  created_at: string
}

// Generic paginated response shape. The collection key varies by endpoint
// (messages/tenants/keys/deliveries/...) so each page-using component
// declares its own concrete response interface.
export interface Page<T> {
  next_cursor: string | null
  [key: string]: T[] | string | null | undefined
}
