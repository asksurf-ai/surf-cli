/**
 * Frontend fetch utilities — calls /proxy/* on own backend (same-origin).
 *
 * Unlike the server data client, this does NOT read env vars.
 * All requests go to /proxy/* which Express proxies to the right upstream.
 *
 * BASE_URL is set by Vite from vite.config.ts `base` option.
 * In urania sandbox: /preview/staging/{session_id}/
 * Locally or deployed: / (empty string after trailing slash strip)
 */

const BASE = typeof window !== 'undefined'
  ? ((import.meta as any).env?.BASE_URL || '/').replace(/\/$/, '')
  : ''

/** Normalize path: strip leading slashes and redundant proxy/ prefix. */
function normalizePath(path: string): string {
  return String(path || '').replace(/^\/+/, '').replace(/^(?:proxy\/)+/, '')
}

/** Fetch JSON with retry on empty response. */
async function fetchWithRetry<T = any>(url: string, init?: RequestInit, retries = 1): Promise<T> {
  for (let attempt = 0; attempt <= retries; attempt++) {
    const res = await fetch(url, init)
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`API error ${res.status}: ${text.slice(0, 200)}`)
    }
    const text = await res.text()
    if (text) return JSON.parse(text)
    if (attempt < retries) await new Promise(r => setTimeout(r, 1000))
  }
  throw new Error(`Empty response from ${url}`)
}

/** Proxy GET — calls /proxy/{path} on the backend. */
export async function proxyGet<T = any>(path: string, params?: Record<string, any>): Promise<T> {
  const cleaned: Record<string, string> = {}
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v != null) cleaned[k] = String(v)
    }
  }
  const qs = Object.keys(cleaned).length ? '?' + new URLSearchParams(cleaned).toString() : ''
  return fetchWithRetry<T>(`${BASE}/proxy/${normalizePath(path)}${qs}`)
}

/** Proxy POST — calls /proxy/{path} on the backend. */
export async function proxyPost<T = any>(path: string, body?: any): Promise<T> {
  return fetchWithRetry<T>(`${BASE}/proxy/${normalizePath(path)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
}
