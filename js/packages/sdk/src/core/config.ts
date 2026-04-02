export const DEFAULT_API_BASE_URL = 'https://api.ask.surf/gateway/v1'

export interface SurfApiConfig {
  baseUrl: string
  apiKey?: string
}

function trimTrailingSlashes(value: string): string {
  return String(value || '').replace(/\/+$/, '')
}

export function readSurfApiConfig(): SurfApiConfig {
  return {
    baseUrl: trimTrailingSlashes(process.env.SURF_API_BASE_URL || DEFAULT_API_BASE_URL),
    apiKey: process.env.SURF_API_KEY,
  }
}

export function requireSurfApiConfig(): { baseUrl: string; apiKey: string } {
  const config = readSurfApiConfig()
  if (!config.apiKey) {
    throw new Error('SURF_API_KEY is required')
  }
  return { baseUrl: config.baseUrl, apiKey: config.apiKey }
}

export function readAdminApiKey(): string | undefined {
  return process.env.SURF_API_KEY
}
