/**
 * Shared schema sync logic — used by both Express runtime (createServer)
 * and Next.js template (instrumentation.ts).
 *
 * Reads a Drizzle schema file, provisions the database, creates missing
 * tables, and adds missing columns to existing tables.
 */

import fs from 'fs'
import path from 'path'
import { get, post } from '../data/client'

export interface SchemaSyncOptions {
  /** Path to the schema file (e.g., 'db/schema.js' or 'db/schema.ts') */
  schemaPath: string
  /** Number of retries on failure (default: 3) */
  retries?: number
  /** Base delay between retries in ms (default: 2000) */
  retryDelay?: number
}

let syncing = false

/**
 * Sync a Drizzle schema file to the database.
 *
 * - Provisions the database via the Surf proxy
 * - Creates missing tables using drizzle-kit DDL generation
 * - Adds missing columns to existing tables via ALTER TABLE
 */
export async function syncSchema(options: SchemaSyncOptions): Promise<void> {
  const { schemaPath, retries = 3, retryDelay = 2000 } = options

  for (let i = 0; i < retries; i++) {
    try {
      await doSyncSchema(schemaPath)
      return
    } catch (err: any) {
      console.error(`DB schema sync attempt ${i + 1}/${retries} failed: ${err.message}`)
      if (i < retries - 1) await new Promise(r => setTimeout(r, retryDelay * (i + 1)))
    }
  }
  console.error('DB schema sync failed after all retries')
}

async function doSyncSchema(schemaPath: string): Promise<void> {
  if (syncing) return
  syncing = true
  try {
    if (!fs.existsSync(schemaPath)) return

    let schema: any
    if (schemaPath.endsWith('.ts')) {
      // ESM / Next.js: use dynamic import with cache-busting query string
      try {
        schema = await import(`${schemaPath}?t=${Date.now()}`)
      } catch (err: any) {
        if (err instanceof SyntaxError) {
          console.log('DB: schema file has syntax error, waiting for next change...')
          return
        }
        if (err.message.includes('Cannot find module') || err.message.includes('is not a function')) {
          return
        }
        throw err
      }
    } else {
      // CJS / Express: use require with cache clearing
      try { delete require.cache[require.resolve(schemaPath)] } catch { /* not cached */ }
      try {
        schema = require(schemaPath)
      } catch (err: any) {
        if (err instanceof SyntaxError) {
          console.log('DB: schema file has syntax error, waiting for next change...')
          return
        }
        if (err.message.includes('Cannot find module') || err.message.includes('is not a function')) {
          return
        }
        throw err
      }
    }

    let getTableConfig: any
    try {
      getTableConfig = require('drizzle-orm/pg-core').getTableConfig
    } catch {
      return // drizzle-orm not installed
    }

    const tables = Object.values(schema).filter((t: any) =>
      t && typeof t === 'object' && Symbol.for('drizzle:Name') in t
    )
    if (tables.length === 0) return

    // Provision database
    await post('db/provision', {})

    // Get existing tables
    const existing: string[] = ((await get('db/tables')) as any[]).map((t: any) => t.name)
    const missing = tables.filter((t: any) => !existing.includes(getTableConfig(t).name))

    if (missing.length > 0) {
      // Generate DDL with drizzle-kit
      const { generateDrizzleJson, generateMigration } = require('drizzle-kit/api')
      const missingSchema: any = {}
      for (const t of missing) missingSchema[getTableConfig(t as any).name] = t
      const sqls: string[] = await generateMigration(generateDrizzleJson({}), generateDrizzleJson(missingSchema))

      for (const sql of sqls) {
        for (let attempt = 0; attempt < 2; attempt++) {
          try {
            await post('db/query', { sql, params: [] })
            console.log(`DB: Executed: ${sql.slice(0, 80)}...`)
            break
          } catch (err: any) {
            if (attempt === 0) {
              console.warn(`DB: Retrying after: ${err.message}`)
              await new Promise(r => setTimeout(r, 1500))
            } else {
              console.error(`DB: Failed: ${sql.slice(0, 80)}... — ${err.message}`)
            }
          }
        }
      }
    }

    // Check existing tables for missing columns
    const existingTables = tables.filter((t: any) => existing.includes(getTableConfig(t).name))
    for (const t of existingTables) {
      const cfg = getTableConfig(t as any)
      try {
        const live: any = await get('db/table-schema', { table: cfg.name })
        const liveCols = new Set((live.columns || []).map((c: any) => c.name))
        for (const col of cfg.columns) {
          if (!liveCols.has(col.name)) {
            const colType = col.getSQLType()
            const ddl = `ALTER TABLE "${cfg.name}" ADD COLUMN IF NOT EXISTS "${col.name}" ${colType}`
            try {
              await post('db/query', { sql: ddl, params: [] })
              console.log(`DB: Added column ${col.name} to ${cfg.name}`)
            } catch (err: any) {
              console.warn(`DB: Failed to add column ${col.name} to ${cfg.name}: ${err.message}`)
            }
          }
        }
      } catch (err: any) {
        console.warn(`DB: Column check failed for ${cfg.name}: ${err.message}`)
      }
    }
  } finally {
    syncing = false
  }
}

/**
 * Watch a schema file for changes and re-sync on edit.
 * Returns a cleanup function to stop watching.
 */
export function watchSchema(
  schemaPath: string,
  options?: { debounceMs?: number; retries?: number; retryDelay?: number }
): () => void {
  const { debounceMs = 1000, retries = 2, retryDelay = 1500 } = options || {}
  if (!fs.existsSync(schemaPath)) return () => {}

  let debounce: ReturnType<typeof setTimeout> | null = null

  fs.watchFile(schemaPath, { interval: 2000 }, () => {
    if (debounce) clearTimeout(debounce)
    debounce = setTimeout(async () => {
      console.log('DB: schema file changed, re-syncing tables...')
      try {
        await syncSchema({ schemaPath, retries, retryDelay })
        console.log('DB: schema re-sync complete')
      } catch (err: any) {
        console.error(`DB: schema re-sync failed: ${err.message}`)
      }
    }, debounceMs)
  })

  return () => {
    fs.unwatchFile(schemaPath)
    if (debounce) clearTimeout(debounce)
  }
}
