/**
 * Express server runtime — replaces scaffold server.js + routes/proxy.js.
 *
 * Handles:
 *   - /proxy/* passthrough (env-aware: sandbox → OutboundProxy, deployed → hermod)
 *   - Auto-loading routes from routes/*.js → /api/{name}
 *   - Cron job system from cron.json
 *   - DB schema sync on startup
 *   - Health check at /api/health
 */

import express, { type Express, type Request, type Response } from 'express'
import cors from 'cors'
import fs from 'fs'
import path from 'path'
import { createProxyMiddleware, responseInterceptor } from 'http-proxy-middleware'
import { Cron } from 'croner'

export interface ServerOptions {
  /** Port to listen on (default: PORT env, fallback 3001) */
  port?: number
  /** Directory containing route files (default: ./routes) */
  routesDir?: string
  /** Directory containing cron.json (default: .) */
  cronDir?: string
  /** Enable /proxy/* passthrough (default: true) */
  proxy?: boolean
}

export function createServer(options: ServerOptions = {}) {
  const port = options.port || parseInt(process.env.PORT || '3001', 10)
  const routesDir = options.routesDir || path.join(process.cwd(), 'routes')
  const cronDir = options.cronDir || process.cwd()
  const enableProxy = options.proxy !== false

  const app: Express = express()

  // CORS
  app.use(cors())

  // /proxy/* passthrough — MUST be before express.json() to preserve body stream
  if (enableProxy) {
    setupProxy(app, port)
  }

  // Parse JSON bodies (after proxy to avoid consuming body stream)
  app.use(express.json())

  // Health check
  app.get('/api/health', (_req: Request, res: Response) => {
    res.json({ status: 'ok' })
  })

  // Auto-load routes from routes/*.js → /api/{name}
  if (fs.existsSync(routesDir)) {
    for (const file of fs.readdirSync(routesDir)) {
      if (!file.endsWith('.js') && !file.endsWith('.ts')) continue
      const name = file.replace(/\.(js|ts)$/, '')
      try {
        const route = require(path.join(routesDir, file))
        const handler = route.default || route
        if (typeof handler === 'function') {
          app.use(`/api/${name}`, handler)
          console.log(`Route registered: /api/${name}`)
        }
      } catch (err: any) {
        console.error(`Failed to load route ${file}: ${err.message}`)
      }
    }
  }

  // Schema sync endpoint
  const schemaDir = options.routesDir ? path.join(options.routesDir, '..', 'db') : path.join(process.cwd(), 'db')
  setupSchemaSync(app, schemaDir)

  // Cron system
  setupCron(app, cronDir)

  return {
    app,
    port,
    async start() {
      return new Promise<void>((resolve) => {
        app.listen(port, async () => {
          console.log(`Backend listening on port ${port}`)
          // Sync schema on startup, then load cron jobs
          await schemaSync.run()
          schemaSync.watch()
          resolve()
        })
      })
    },
  }
}

// ── Proxy setup ──────────────────────────────────────────────────────────

/** Read env var with SURF_ prefix priority, falling back to old name. */
function env(surfName: string, legacyName: string): string | undefined {
  return process.env[surfName] || process.env[legacyName]
}

function setupProxy(app: Express, port: number) {
  const gatewayUrl = env('SURF_DEPLOYED_GATEWAY_URL', 'GATEWAY_URL')
  const appToken = env('SURF_DEPLOYED_APP_TOKEN', 'APP_TOKEN')
  const proxyBase = env('SURF_SANDBOX_PROXY_BASE', 'DATA_PROXY_BASE')
  const isDeployed = Boolean(gatewayUrl && appToken)

  const bufferResponse = responseInterceptor(async (buf) => buf)

  if (isDeployed) {
    // Deployed: /proxy/* → hermod /gateway/v1/* with APP_TOKEN
    app.use('/proxy', createProxyMiddleware({
      target: gatewayUrl!,
      changeOrigin: true,
      selfHandleResponse: true,
      pathRewrite: (p) => '/gateway/v1' + p,
      headers: {
        Authorization: `Bearer ${appToken}`,
        'Accept-Encoding': 'identity',
      },
      on: { proxyRes: bufferResponse },
    }))

    // Backend routes use DATA_PROXY_BASE to call through our own /proxy middleware
    // Set both new and legacy vars so the data client resolves correctly
    const loopback = `http://127.0.0.1:${port}/proxy`
    process.env.SURF_SANDBOX_PROXY_BASE = loopback
    process.env.DATA_PROXY_BASE = loopback
  } else if (proxyBase) {
    // Sandbox: /proxy/* → OutboundProxy (set by urania executor)
    // Direct passthrough — no selfHandleResponse/responseInterceptor needed.
    // selfHandleResponse buffers the entire response body which breaks when
    // upstream returns chunked/compressed responses (causes 500 + empty body).
    const target = proxyBase.replace(/\/proxy$/, '')

    app.use(createProxyMiddleware({
      target,
      changeOrigin: true,
      pathFilter: '/proxy',
      on: {
        proxyReq: (proxyReq, req) => {
          console.log(`[proxy] >> ${req.method} ${req.originalUrl}`)
        },
        proxyRes: (proxyRes, req) => {
          console.log(`[proxy] << ${proxyRes.statusCode} ${req.method} ${req.originalUrl}`)
        },
        error: (err: Error, req: any, res: any) => {
          console.error(`[proxy] !! ${req.method} ${req.originalUrl} error: ${err.message}`)
          if (!res.headersSent) res.status(502).json({ error: err.message })
        },
      },
    }))
  }
}

// ── Schema sync ──────────────────────────────────────────────────────────

const schemaSync = { run: async () => {}, watch: () => {} }

function setupSchemaSync(app: Express, schemaDir: string) {
  let syncing = false
  let schemaReady = false

  async function doSyncSchema() {
    if (syncing) return
    syncing = true
    try {
      const schemaPath = path.join(schemaDir, 'schema.js')
      if (!fs.existsSync(schemaPath)) return

      // Clear require cache to pick up latest schema
      try { delete require.cache[require.resolve(schemaPath)] } catch { /* not cached */ }

      let schema: any
      try {
        schema = require(schemaPath)
      } catch (err: any) {
        if (err instanceof SyntaxError) {
          console.log('DB: schema.js has syntax error, waiting for next change...')
          return
        }
        if (err.message.includes('Cannot find module') || err.message.includes('is not a function')) {
          return
        }
        throw err
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

      const { get: dbGet, post: dbPost } = await import('../data/client')

      // Provision database
      await dbPost('db/provision')

      // Get existing tables
      const existing: string[] = ((await dbGet('db/tables')) as any[]).map((t: any) => t.name)
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
              await dbPost('db/query', { sql, params: [] })
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
          const live: any = await dbGet('db/table-schema', { table: cfg.name })
          const liveCols = new Set((live.columns || []).map((c: any) => c.name))
          for (const col of cfg.columns) {
            if (!liveCols.has(col.name)) {
              const colType = col.getSQLType()
              const ddl = `ALTER TABLE "${cfg.name}" ADD COLUMN IF NOT EXISTS "${col.name}" ${colType}`
              try {
                await dbPost('db/query', { sql: ddl, params: [] })
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

  async function syncWithRetry(retries = 3, delay = 2000) {
    for (let i = 0; i < retries; i++) {
      try {
        await doSyncSchema()
        return
      } catch (err: any) {
        console.error(`DB schema sync attempt ${i + 1}/${retries} failed: ${err.message}`)
        if (i < retries - 1) await new Promise(r => setTimeout(r, delay * (i + 1)))
      }
    }
    console.error('DB schema sync failed after all retries')
  }

  // Explicit sync endpoint
  app.post('/api/__sync-schema', async (_req: Request, res: Response) => {
    try {
      await syncWithRetry(2, 1500)
      res.json({ ok: true })
    } catch (err: any) {
      res.status(500).json({ ok: false, error: err.message })
    }
  })

  // Block /api/* until initial schema sync completes
  app.use('/api', (req: Request, res: Response, next: any) => {
    if (schemaReady || req.path === '/health' || req.path === '/__sync-schema' || req.path.startsWith('/cron')) return next()
    res.status(503).json({ error: 'Database schema initializing...' })
  })

  // Wire up for startup
  schemaSync.run = async () => {
    try {
      await syncWithRetry()
      schemaReady = true
      console.log('Schema sync complete, API ready')
    } catch {
      schemaReady = true // don't block forever
      console.warn('Schema sync failed, proceeding anyway')
    }
  }

  schemaSync.watch = () => {
    const schemaPath = path.join(schemaDir, 'schema.js')
    if (!fs.existsSync(schemaPath)) return
    let debounce: any = null
    fs.watchFile(schemaPath, { interval: 2000 }, () => {
      if (debounce) clearTimeout(debounce)
      debounce = setTimeout(async () => {
        console.log('DB: schema.js changed, re-syncing tables...')
        try {
          await syncWithRetry(2, 1500)
          console.log('DB: schema re-sync complete')
        } catch (err: any) {
          console.error(`DB: schema re-sync failed: ${err.message}`)
        }
      }, 1000)
    })
  }
}

// ── Cron setup ───────────────────────────────────────────────────────────

function setupCron(app: Express, cronDir: string) {
  const cronJobs = new Map<string, Cron>()
  const cronState = new Map<string, { lastRunAt: string | null; lastStatus: string | null; lastError: string | null }>()
  let cronTasks: any[] = []

  function loadCronJobs() {
    for (const [, job] of cronJobs) { try { job.stop() } catch (_) {} }
    cronJobs.clear()

    const cronPath = path.join(cronDir, 'cron.json')
    if (!fs.existsSync(cronPath)) return

    let tasks: any[]
    try { tasks = JSON.parse(fs.readFileSync(cronPath, 'utf-8')) }
    catch (e: any) { console.error('Failed to parse cron.json:', e.message); return }
    if (!Array.isArray(tasks)) { console.error('cron.json must be an array'); return }
    cronTasks = tasks

    for (const task of tasks) {
      if (!task.enabled) continue
      const timeoutMs = (task.timeout || 300) * 1000
      let handlerMod: any
      try { handlerMod = require(path.join(cronDir, task.handler)) }
      catch (e: any) { console.error(`Failed to load cron handler ${task.handler}:`, e.message); continue }
      if (typeof handlerMod.handler !== 'function') { console.error(`Cron handler ${task.handler} has no handler function`); continue }

      if (!cronState.has(task.id)) {
        cronState.set(task.id, { lastRunAt: null, lastStatus: null, lastError: null })
      }

      const job = new Cron(task.schedule, async () => {
        const state = cronState.get(task.id)!
        state.lastRunAt = new Date().toISOString()
        try {
          await Promise.race([
            Promise.resolve(handlerMod.handler()),
            new Promise((_, reject) => setTimeout(() => reject(new Error('Cron task timed out')), timeoutMs)),
          ])
          state.lastStatus = 'success'
          state.lastError = null
          console.log(`Cron [${task.id}] ${task.name}: success`)
        } catch (e: any) {
          state.lastStatus = 'error'
          state.lastError = e.message
          console.error(`Cron [${task.id}] ${task.name}: ${e.message}`)
        }
      })
      cronJobs.set(task.id, job)
      console.log(`Cron registered: [${task.id}] ${task.name} (${task.schedule})`)
    }
  }

  // Cron auth middleware — requires APP_TOKEN in deployed mode, skips in dev
  const envFn = (s: string, l: string) => process.env[s] || process.env[l]
  app.use('/api/cron', (req: Request, res: Response, next: any) => {
    const appToken = envFn('SURF_DEPLOYED_APP_TOKEN', 'APP_TOKEN')
    if (!appToken) return next() // dev mode: skip auth
    const auth = req.headers.authorization
    if (!auth || auth !== `Bearer ${appToken}`) {
      return res.status(401).json({ error: 'Unauthorized' })
    }
    next()
  })

  // GET /api/cron — list all cron jobs with status
  app.get('/api/cron', (_req: Request, res: Response) => {
    res.json(cronTasks.map(t => {
      const state = cronState.get(t.id) || { lastRunAt: null, lastStatus: null, lastError: null }
      const job = cronJobs.get(t.id)
      return {
        ...t,
        lastRunAt: state.lastRunAt,
        lastStatus: state.lastStatus,
        lastError: state.lastError,
        nextRun: job ? (job as any).nextRun()?.toISOString() || null : null,
      }
    }))
  })

  // POST /api/cron — create a new cron task
  app.post('/api/cron', (req: Request, res: Response) => {
    const { id, name, schedule, handler, enabled = true, timeout = 300 } = req.body || {}
    if (!id || !name || !schedule || !handler) {
      return res.status(400).json({ error: 'Missing required fields: id, name, schedule, handler' })
    }

    // Validate cron expression and minimum interval (>= 1 minute)
    try {
      const testJob = new Cron(schedule)
      const next1 = testJob.nextRun()
      const next2 = next1 ? testJob.nextRun(next1) : null
      testJob.stop()
      if (next1 && next2 && (next2.getTime() - next1.getTime()) < 60000) {
        return res.status(400).json({ error: 'Minimum interval between runs must be at least 1 minute' })
      }
    } catch (err: any) {
      return res.status(400).json({ error: `Invalid cron expression: ${err.message}` })
    }

    const cronPath = path.join(cronDir, 'cron.json')
    let tasks: any[] = []
    try { if (fs.existsSync(cronPath)) tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8')) } catch { tasks = [] }

    if (tasks.some((t: any) => t.id === id)) {
      return res.status(409).json({ error: `Task with id "${id}" already exists` })
    }

    const newTask = { id, name, schedule, handler, enabled, timeout }
    tasks.push(newTask)
    fs.writeFileSync(cronPath, JSON.stringify(tasks, null, 2))
    loadCronJobs()
    res.status(201).json(newTask)
  })

  // PATCH /api/cron/:id — update a cron task
  app.patch('/api/cron/:id', (req: Request, res: Response) => {
    const cronPath = path.join(cronDir, 'cron.json')
    let tasks: any[] = []
    try { if (fs.existsSync(cronPath)) tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8')) } catch { tasks = [] }

    const idx = tasks.findIndex((t: any) => t.id === req.params.id as string)
    if (idx === -1) return res.status(404).json({ error: 'Task not found' })

    const updates = req.body || {}
    if (updates.schedule) {
      try {
        const testJob = new Cron(updates.schedule)
        const next1 = testJob.nextRun()
        const next2 = next1 ? testJob.nextRun(next1) : null
        testJob.stop()
        if (next1 && next2 && (next2.getTime() - next1.getTime()) < 60000) {
          return res.status(400).json({ error: 'Minimum interval between runs must be at least 1 minute' })
        }
      } catch (err: any) {
        return res.status(400).json({ error: `Invalid cron expression: ${err.message}` })
      }
    }

    tasks[idx] = { ...tasks[idx], ...updates, id: req.params.id as string }
    fs.writeFileSync(cronPath, JSON.stringify(tasks, null, 2))
    loadCronJobs()
    res.json(tasks[idx])
  })

  // DELETE /api/cron/:id — delete a cron task
  app.delete('/api/cron/:id', (req: Request, res: Response) => {
    const cronPath = path.join(cronDir, 'cron.json')
    let tasks: any[] = []
    try { if (fs.existsSync(cronPath)) tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8')) } catch { tasks = [] }

    const idx = tasks.findIndex((t: any) => t.id === req.params.id as string)
    if (idx === -1) return res.status(404).json({ error: 'Task not found' })

    tasks.splice(idx, 1)
    fs.writeFileSync(cronPath, JSON.stringify(tasks, null, 2))
    cronState.delete(req.params.id as string)
    loadCronJobs()
    res.json({ ok: true })
  })

  // POST /api/cron/:id/run — manually trigger a cron task
  app.post('/api/cron/:id/run', async (req: Request, res: Response) => {
    const job = cronJobs.get(req.params.id as string)
    if (!job) return res.status(404).json({ error: 'Task not found or not enabled' })
    try {
      job.trigger()
      res.json({ ok: true, message: `Task ${req.params.id as string} triggered` })
    } catch (err: any) {
      res.status(500).json({ error: err.message })
    }
  })

  // Load on startup
  loadCronJobs()
}
