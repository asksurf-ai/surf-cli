/**
 * @surf-ai/sdk/server — Express server runtime + data API client.
 *
 * Replaces scaffold server.js and lib/api.js.
 * Agent just calls createServer() and writes routes in routes/*.
 */

export { createServer } from './runtime'
export type { ServerOptions } from './runtime'

// Re-export data API client for backend route use
export { dataApi } from '../data/data-api'
