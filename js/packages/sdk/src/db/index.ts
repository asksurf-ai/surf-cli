/**
 * @surf-ai/sdk/db — Database helpers via HTTP proxy to Neon PostgreSQL.
 *
 * All queries go through the Surf API proxy. There is no locally-instantiated
 * Drizzle ORM client — `drizzle-orm/pg-core` is used only to declare the
 * schema; reads and writes go through dbQuery().
 *
 * Usage:
 *   const { dbQuery } = require('@surf-ai/sdk/db')
 *   const { rows } = await dbQuery('SELECT * FROM users WHERE id = $1', [userId])
 */

import { get, post } from '../data/client'

// Re-export schema sync utilities for use in Next.js and other runtimes
export { syncSchema, watchSchema } from './schema-sync'
export type { SchemaSyncOptions } from './schema-sync'

/**
 * Provision a database for the current user via db/provision.
 * Returns connection metadata. Neon auto-creates the DB if it doesn't exist.
 */
export async function dbProvision(): Promise<{
  host: string
  database: string
  user: string
  password: string
}> {
  return post('db/provision', {})
}

/**
 * Execute a SQL query via db/query.
 *
 * Returns a pg-style result — use `result.rows` for the row data:
 *   const { rows } = await dbQuery('SELECT * FROM users')
 *   const [row] = (await dbQuery('INSERT ... RETURNING *', [...])).rows
 *
 * @param options.arrayMode - When true, each row is a positional array instead
 *   of an object keyed by column name. Default false (object rows).
 */
export async function dbQuery(
  sql: string,
  params?: any[],
  options?: { arrayMode?: boolean },
): Promise<{
  rows: any[]
  rowCount?: number
  fields?: Array<{ name: string; type: string }>
  truncated?: boolean
}> {
  return post('db/query', { sql, params, arrayMode: options?.arrayMode ?? false })
}

/**
 * List tables in the user's database.
 */
export async function dbTables(): Promise<string[]> {
  return get('db/tables')
}

/**
 * Get schema for a specific table.
 */
export async function dbTableSchema(table: string): Promise<any> {
  return get(`db/tables/${encodeURIComponent(table)}/schema`)
}

/**
 * Check database connection status.
 */
export async function dbStatus(): Promise<{ connected: boolean; database?: string }> {
  return get('db/status')
}
