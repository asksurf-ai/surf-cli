/**
 * 1.0 data API client.
 *
 * Reads:
 *   - SURF_API_BASE_URL (default: https://api.asksurf.ai/gateway/v1)
 *   - SURF_API_KEY      (required)
 *
 * All requests are sent directly to the configured API base URL using
 * Authorization: Bearer <SURF_API_KEY>.
 */

import { getJson, postJson } from '../core/transport'

/** Low-level GET — escape hatch for endpoints not yet in the SDK. */
export async function get<T = any>(path: string, params?: Record<string, any>): Promise<T> {
  return getJson<T>(path, params)
}

/** Low-level POST — escape hatch for endpoints not yet in the SDK. */
export async function post<T = any>(path: string, body?: any): Promise<T> {
  return postJson<T>(path, body)
}
