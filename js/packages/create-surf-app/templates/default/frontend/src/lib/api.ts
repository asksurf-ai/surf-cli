/**
 * Crypto API client helpers (fallback — swagger fetch failed).
 */

export { API_BASE, normalizeProxyPath, fetchWithRetry, proxyGet, proxyPost } from './fetch'

export interface ApiResponse<T> {
  data: T[]
  meta?: { total?: number; limit?: number; offset?: number }
  error?: { code: string; message: string }
}

export interface ApiObjectResponse<T> {
  data: T
  meta?: { total?: number; limit?: number; offset?: number }
  error?: { code: string; message: string }
}

export interface CursorMeta {
  has_more: boolean
  next_cursor?: string
  limit?: number
  cached?: boolean
  credits_used?: number
}

export interface ApiCursorResponse<T> {
  data: T[]
  meta: CursorMeta
  error?: { code: string; message: string }
}
