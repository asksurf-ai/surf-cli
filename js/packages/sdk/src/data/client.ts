/**
 * Environment-aware data API client.
 *
 * Reads env vars to determine routing (prefixed vars take priority):
 *   - SURF_SANDBOX_PROXY_BASE → sandbox (OutboundProxy handles auth)
 *   - SURF_DEPLOYED_GATEWAY_URL + SURF_DEPLOYED_APP_TOKEN → deployed (hermod with Bearer)
 *   - Neither → public (api.ask.surf, caller provides own auth)
 *
 * Backward-compatible aliases:
 *   DATA_PROXY_BASE → SURF_SANDBOX_PROXY_BASE
 *   GATEWAY_URL     → SURF_DEPLOYED_GATEWAY_URL
 *   APP_TOKEN       → SURF_DEPLOYED_APP_TOKEN
 */

const DEFAULT_PUBLIC_URL = 'https://api.ask.surf/gateway/v1'

/** Read env var with SURF_ prefix priority, falling back to old name. */
function env(surfName: string, legacyName: string): string | undefined {
  return process.env[surfName] || process.env[legacyName]
}

interface ClientConfig {
  baseUrl: string
  headers: Record<string, string>
}

function resolveConfig(): ClientConfig {
  // Sandbox: OutboundProxy handles JWT injection
  const proxyBase = env('SURF_SANDBOX_PROXY_BASE', 'DATA_PROXY_BASE')
  if (proxyBase) {
    return { baseUrl: proxyBase, headers: {} }
  }

  // Deployed: direct to hermod with APP_TOKEN
  const gatewayUrl = env('SURF_DEPLOYED_GATEWAY_URL', 'GATEWAY_URL')
  const appToken = env('SURF_DEPLOYED_APP_TOKEN', 'APP_TOKEN')
  if (gatewayUrl && appToken) {
    return {
      baseUrl: `${gatewayUrl.replace(/\/$/, '')}/gateway/v1`,
      headers: { Authorization: `Bearer ${appToken}` },
    }
  }

  // Public: direct to api.ask.surf (no auth — caller must handle)
  return { baseUrl: DEFAULT_PUBLIC_URL, headers: {} }
}

/** Normalize path: strip leading slashes and redundant proxy/ prefix. */
function normalizePath(path: string): string {
  return String(path || '').replace(/^\/+/, '').replace(/^(?:proxy\/)+/, '')
}

/** Fetch JSON with retry on empty response. */
async function fetchJson<T = any>(url: string, init?: RequestInit, retries = 1): Promise<T> {
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

/** Low-level GET — escape hatch for endpoints not yet in the SDK. */
export async function get<T = any>(path: string, params?: Record<string, any>): Promise<T> {
  const config = resolveConfig()
  const cleaned: Record<string, string> = {}
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v != null) cleaned[k] = String(v)
    }
  }
  const qs = Object.keys(cleaned).length ? '?' + new URLSearchParams(cleaned).toString() : ''
  return fetchJson<T>(`${config.baseUrl}/${normalizePath(path)}${qs}`, {
    headers: config.headers,
  })
}

/** Low-level POST — escape hatch for endpoints not yet in the SDK. */
export async function post<T = any>(path: string, body?: any): Promise<T> {
  const config = resolveConfig()
  return fetchJson<T>(`${config.baseUrl}/${normalizePath(path)}`, {
    method: 'POST',
    headers: { ...config.headers, 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
}
