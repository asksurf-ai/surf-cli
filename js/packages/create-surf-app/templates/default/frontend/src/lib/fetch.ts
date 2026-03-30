/**
 * Core fetch utilities for proxy API access.
 * This file is part of the scaffold — do NOT modify manually.
 */

export const API_BASE = import.meta.env.VITE_API_URL || import.meta.env.BASE_URL.replace(/\/$/, '')

/** Normalize logical proxy paths; strips leading slashes and proxy/ prefix. */
export function normalizeProxyPath(path: string): string {
  const trimmed = String(path || '').replace(/^\/+/, '')
  return trimmed.replace(/^(?:proxy\/)+/, '')
}

/** Fetch with retry on empty response. */
export async function fetchWithRetry<T = any>(url: string, init?: RequestInit, retries = 1): Promise<T> {
  for (let attempt = 0; attempt <= retries; attempt++) {
    const res = await fetch(url, init)
    const text = await res.text()
    if (text) return JSON.parse(text)
    if (attempt < retries) await new Promise(r => setTimeout(r, 1000))
  }
  throw new Error(`Empty response from ${url.replace(API_BASE, '')}`)
}

/** Generic proxy GET \u2014 use for any /proxy/{category}/{endpoint} route. */
export async function proxyGet<T = any>(path: string, params?: Record<string, string>): Promise<T> {
  const qs = params ? '?' + new URLSearchParams(params).toString() : ''
  return fetchWithRetry<T>(`${API_BASE}/proxy/${normalizeProxyPath(path)}${qs}`)
}

/** Generic proxy POST \u2014 use for any /proxy/{category}/{endpoint} route. */
export async function proxyPost<T = any>(path: string, body?: any): Promise<T> {
  return fetchWithRetry<T>(`${API_BASE}/proxy/${normalizeProxyPath(path)}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
}
