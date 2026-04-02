import { requireSurfApiConfig } from './config'

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export function normalizePath(path: string): string {
  return String(path || '')
    .replace(/^\/+/, '')
}

function buildUrl(path: string, params?: Record<string, any>): string {
  const { baseUrl } = requireSurfApiConfig()
  const url = new URL(`${baseUrl}/${normalizePath(path)}`)

  if (params) {
    for (const [key, value] of Object.entries(params)) {
      if (value != null) {
        url.searchParams.set(key, String(value))
      }
    }
  }

  return url.toString()
}

function buildHeaders(extra?: Record<string, string>): Headers {
  const { apiKey } = requireSurfApiConfig()
  const headers = new Headers(extra)
  headers.set('Authorization', `Bearer ${apiKey}`)
  return headers
}

async function fetchJson<T = any>(url: string, init: RequestInit, retries = 1): Promise<T> {
  for (let attempt = 0; attempt <= retries; attempt++) {
    const res = await fetch(url, init)
    if (!res.ok) {
      const text = await res.text()
      throw new Error(`API error ${res.status}: ${text.slice(0, 200)}`)
    }

    const text = await res.text()
    if (text) {
      return JSON.parse(text)
    }

    if (attempt < retries) {
      await sleep(1000)
    }
  }

  throw new Error(`Empty response from ${url}`)
}

export async function getJson<T = any>(path: string, params?: Record<string, any>): Promise<T> {
  return fetchJson<T>(buildUrl(path, params), {
    headers: buildHeaders(),
  })
}

export async function postJson<T = any>(path: string, body?: any): Promise<T> {
  return fetchJson<T>(buildUrl(path), {
    method: 'POST',
    headers: buildHeaders({
      'Content-Type': 'application/json',
    }),
    body: body ? JSON.stringify(body) : undefined,
  })
}
