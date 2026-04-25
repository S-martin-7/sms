import axios, { AxiosError } from 'axios'

const STORAGE_KEY = 'sms_jwt'

export const api = axios.create({
  baseURL: '/',
  headers: { 'Content-Type': 'application/json' },
})

api.interceptors.request.use((config) => {
  const token = localStorage.getItem(STORAGE_KEY)
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

api.interceptors.response.use(
  (r) => r,
  (err: AxiosError) => {
    // 401 means token is stale or missing — clear and bounce to /login.
    // Don't loop if we ARE the login request.
    if (err.response?.status === 401) {
      const url = err.config?.url ?? ''
      if (!url.includes('/admin/login')) {
        localStorage.removeItem(STORAGE_KEY)
        if (!window.location.hash.startsWith('#/login')) {
          window.location.hash = '#/login'
        }
      }
    }
    return Promise.reject(err)
  },
)

export const tokenStorage = {
  get: () => localStorage.getItem(STORAGE_KEY),
  set: (v: string) => localStorage.setItem(STORAGE_KEY, v),
  clear: () => localStorage.removeItem(STORAGE_KEY),
}

// errorMessage extracts a human-readable message from an axios error so
// we can surface backend "{error: {code, message}}" responses cleanly.
export function errorMessage(err: unknown): string {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data as
      | { error?: { message?: string } }
      | undefined
    if (data?.error?.message) return data.error.message
    return err.message
  }
  if (err instanceof Error) return err.message
  return String(err)
}
