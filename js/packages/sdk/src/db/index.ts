/**
 * @surf-ai/sdk/db — Drizzle ORM + Neon PostgreSQL database helpers.
 *
 * Replaces scaffold lib/db.js and db/index.js.
 *
 * Usage:
 *   const { db, dbQuery, dbProvision } = require('@surf-ai/sdk/db')
 *
 *   // In a backend route:
 *   const users = await db.select().from(schema.users)
 *
 *   // Raw SQL:
 *   const result = await dbQuery('SELECT * FROM users WHERE id = $1', [userId])
 */

import { get, post } from '../data/client'

/**
 * Provision a database for the current user via /proxy/db/provision.
 * Returns connection metadata. Neon auto-creates the DB if it doesn't exist.
 */
export async function dbProvision(): Promise<{
  host: string
  database: string
  user: string
  password: string
}> {
  return post('db/provision')
}

/**
 * Execute a SQL query via /proxy/db/query.
 * Uses pg-proxy driver under the hood — Drizzle ORM calls this automatically.
 *
 * @param options.arrayMode - When true, rows are returned as positional arrays
 *   instead of objects. Required for Drizzle ORM pg-proxy compatibility.
 */
export async function dbQuery(
  sql: string,
  params?: any[],
  options?: { arrayMode?: boolean },
): Promise<any> {
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
  return get('db/table-schema', { table })
}

/**
 * Check database connection status.
 */
export async function dbStatus(): Promise<{ connected: boolean; database?: string }> {
  return get('db/status')
}
