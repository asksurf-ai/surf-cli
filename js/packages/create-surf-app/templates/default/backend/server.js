const express = require('express');
const cors = require('cors');
const { Cron } = require('croner');
const fs = require('fs');
const path = require('path');
const { setupProxyRoutes } = require('./routes/proxy');

if (!process.env.PORT) {
  const envPath = path.join(__dirname, '.env');
  if (fs.existsSync(envPath)) {
    for (const line of fs.readFileSync(envPath, 'utf8').split(/\r?\n/)) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('#')) continue;
      const idx = trimmed.indexOf('=');
      if (idx === -1) continue;
      const key = trimmed.slice(0, idx).trim();
      const value = trimmed.slice(idx + 1).trim();
      if (key && !(key in process.env)) {
        process.env[key] = value;
      }
    }
  }
}

const app = express();
const PORT = Number.parseInt(process.env.PORT || '', 10);
if (!Number.isInteger(PORT)) {
  throw new Error('PORT env var is required');
}

// CORS: allow any origin (external frontends can connect)
app.use(cors());

// Forward /proxy/* → OutboundProxy (API key injection)
// NOTE: Must be registered BEFORE express.json() so that the raw request body
// stream is preserved for http-proxy-middleware to forward POST requests.
setupProxyRoutes(app);

// Parse JSON bodies (after proxy routes to avoid consuming body stream)
app.use(express.json());

// Health check
app.get('/api/health', (_req, res) => {
  res.json({ status: 'ok' });
});

// ── Auto-register API routes ─────────────────────────────────────────────
// Every .js file in routes/ (except proxy.js) is auto-mounted at /api/{name}.
// e.g. routes/todos.js → /api/todos, routes/auth.js → /api/auth
const routesDir = path.join(__dirname, 'routes');
for (const file of fs.readdirSync(routesDir)) {
  if (file === 'proxy.js' || !file.endsWith('.js')) continue;
  const name = file.replace('.js', '');
  try {
    app.use(`/api/${name}`, require(`./routes/${file}`));
    console.log(`Route registered: /api/${name}`);
  } catch (err) {
    console.error(`Failed to load route ${file}: ${err.message}`);
  }
}

// ── Auto-create DB tables from schema.js ─────────────────────────────────
// The agent writes db/schema.js AFTER the server starts, so we need two
// trigger paths: (1) POST /api/__sync-schema for explicit agent calls,
// (2) fs.watchFile as a fallback in case the agent doesn't call the endpoint.
//
// Both paths funnel into doSyncSchema() which has:
// - Concurrency lock (syncing flag) to prevent overlapping DDL
// - SyntaxError guard for half-written files
// - Retry with backoff for Neon cold-start errors
let schemaReady = false;
let syncing = false;

async function doSyncSchema() {
  if (syncing) return;
  syncing = true;
  try {
    // Clear require cache so we pick up the latest schema.js
    try { delete require.cache[require.resolve('./db/schema')]; } catch (_e) { /* not cached — ignore */ }

    // Try to load schema — SyntaxError means the file is half-written
    let schema;
    try {
      schema = require('./db/schema');
    } catch (err) {
      if (err instanceof SyntaxError) {
        console.log('DB: schema.js has syntax error, waiting for next change...');
        return;
      }
      if (err.message.includes('Cannot find module') || err.message.includes('is not a function')) {
        return; // No schema or no drizzle-orm — skip silently
      }
      throw err;
    }

    const { getTableConfig } = require('drizzle-orm/pg-core');
    const tables = Object.values(schema).filter(t =>
      t && typeof t === 'object' && Symbol.for('drizzle:Name') in t
    );
    if (tables.length === 0) return;

    const { dbTables, dbProvision, dbQuery, dbTableSchema } = require('./lib/db');
    await dbProvision('auto-sync schema tables');

    const existing = (await dbTables()).map(t => t.name);
    const missing = tables.filter(t => !existing.includes(getTableConfig(t).name));

    if (missing.length > 0) {
      // Use drizzle-kit to generate correct DDL (handles UNIQUE, FK, defaults, etc.)
      const { generateDrizzleJson, generateMigration } = require('drizzle-kit/api');
      const missingSchema = {};
      for (const t of missing) missingSchema[getTableConfig(t).name] = t;
      const sqls = await generateMigration(generateDrizzleJson({}), generateDrizzleJson(missingSchema));

      for (const sql of sqls) {
        for (let attempt = 0; attempt < 2; attempt++) {
          try {
            await dbQuery(sql, []);
            console.log(`DB: Executed: ${sql.slice(0, 80)}...`);
            break;
          } catch (err) {
            if (attempt === 0) {
              console.warn(`DB: Retrying after: ${err.message}`);
              await new Promise(r => setTimeout(r, 1500));
            } else {
              console.error(`DB: Failed: ${sql.slice(0, 80)}... — ${err.message}`);
            }
          }
        }
      }
    }

    // Check existing tables for missing columns (runs even when new tables were also created)
    const existingTables = tables.filter(t => existing.includes(getTableConfig(t).name));
    for (const t of existingTables) {
      const cfg = getTableConfig(t);
      try {
        const live = await dbTableSchema(cfg.name);
        const liveCols = new Set(live.columns.map(c => c.name));
        for (const col of cfg.columns) {
          if (!liveCols.has(col.name)) {
            const colType = col.getSQLType();
            const ddl = 'ALTER TABLE "' + cfg.name + '" ADD COLUMN IF NOT EXISTS "' + col.name + '" ' + colType;
            try {
              await dbQuery(ddl, []);
              console.log('DB: Added column ' + col.name + ' to ' + cfg.name);
            } catch (err) {
              console.warn('DB: Failed to add column ' + col.name + ' to ' + cfg.name + ': ' + err.message);
            }
          }
        }
      } catch (err) {
        console.warn('DB: Column check failed for ' + cfg.name + ': ' + err.message);
      }
    }
  } finally {
    syncing = false;
  }
}

// Retry wrapper for Neon cold-start transient errors.
async function syncSchemaWithRetry(retries = 3, delay = 2000) {
  for (let i = 0; i < retries; i++) {
    try {
      await doSyncSchema();
      return;
    } catch (err) {
      console.error(`DB schema sync attempt ${i + 1}/${retries} failed: ${err.message}`);
      if (i < retries - 1) await new Promise(r => setTimeout(r, delay * (i + 1)));
    }
  }
  console.error('DB schema sync failed after all retries');
}

// Explicit sync endpoint — agent calls this after writing schema.js.
app.post('/api/__sync-schema', async (_req, res) => {
  try {
    await syncSchemaWithRetry(2, 1500);
    res.json({ ok: true });
  } catch (err) {
    res.status(500).json({ ok: false, error: err.message });
  }
});

// ── Cron job system ──────────────────────────────────────────────────────
// Reads backend/cron.json, schedules tasks with croner, exposes CRUD API.
const _cronInstances = new Map();   // id -> Cron instance
const _cronState = new Map();       // id -> { lastRunAt, lastStatus, lastError }

function _loadCronJobs() {
  // Stop all existing cron jobs
  for (const [, job] of _cronInstances) {
    try { job.stop(); } catch (_e) { /* already stopped — ignore */ }
  }
  _cronInstances.clear();

  const cronPath = path.join(__dirname, 'cron.json');
  if (!fs.existsSync(cronPath)) return;

  let tasks;
  try {
    tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8'));
  } catch (err) {
    console.error('Cron: failed to parse cron.json:', err.message);
    return;
  }

  if (!Array.isArray(tasks)) {
    console.error('Cron: cron.json must be a JSON array');
    return;
  }

  for (const task of tasks) {
    if (!task.enabled) continue;
    if (!task.id || !task.schedule || !task.handler) {
      console.warn('Cron: skipping task with missing fields:', JSON.stringify(task));
      continue;
    }

    let handlerMod;
    try {
      const handlerPath = path.resolve(__dirname, task.handler);
      delete require.cache[handlerPath];
      handlerMod = require(handlerPath);
    } catch (err) {
      console.error(`Cron: failed to load handler ${task.handler}: ${err.message}`);
      continue;
    }

    if (typeof handlerMod.handler !== 'function') {
      console.warn(`Cron: handler ${task.handler} does not export a handler function`);
      continue;
    }

    const timeoutMs = (task.timeout || 300) * 1000;

    // Initialize runtime state if not present
    if (!_cronState.has(task.id)) {
      _cronState.set(task.id, { lastRunAt: null, lastStatus: null, lastError: null });
    }

    const runHandler = async () => {
      const state = _cronState.get(task.id);
      state.lastRunAt = new Date().toISOString();
      try {
        await Promise.race([
          handlerMod.handler(),
          new Promise((_, reject) => setTimeout(() => reject(new Error('timeout')), timeoutMs)),
        ]);
        state.lastStatus = 'success';
        state.lastError = null;
        console.log(`Cron: task ${task.id} completed successfully`);
      } catch (err) {
        state.lastStatus = 'error';
        state.lastError = err.message;
        console.error(`Cron: task ${task.id} failed: ${err.message}`);
      }
    };

    try {
      const job = new Cron(task.schedule, runHandler);
      _cronInstances.set(task.id, job);
      console.log(`Cron: scheduled ${task.id} (${task.name || task.id}) with schedule "${task.schedule}"`);
    } catch (err) {
      console.error(`Cron: invalid schedule for ${task.id}: ${err.message}`);
    }
  }
}

// ── Cron CRUD API routes (before schema readiness guard) ─────────────────
// Auth middleware for /api/cron routes
app.use('/api/cron', (req, res, next) => {
  const appToken = process.env.APP_TOKEN;
  if (!appToken) return next(); // dev mode: skip auth
  const auth = req.headers.authorization;
  if (!auth || auth !== `Bearer ${appToken}`) {
    return res.status(401).json({ error: 'Unauthorized' });
  }
  next();
});

app.get('/api/cron', (_req, res) => {
  const cronPath = path.join(__dirname, 'cron.json');
  let tasks = [];
  try {
    if (fs.existsSync(cronPath)) {
      tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8'));
    }
  } catch { tasks = []; }

  const result = tasks.map(t => {
    const state = _cronState.get(t.id) || { lastRunAt: null, lastStatus: null, lastError: null };
    const job = _cronInstances.get(t.id);
    return {
      ...t,
      lastRunAt: state.lastRunAt,
      lastStatus: state.lastStatus,
      lastError: state.lastError,
      nextRun: job ? job.nextRun()?.toISOString() || null : null,
    };
  });
  res.json(result);
});

app.post('/api/cron', (req, res) => {
  const { id, name, schedule, handler, enabled = true, timeout = 300 } = req.body || {};
  if (!id || !name || !schedule || !handler) {
    return res.status(400).json({ error: 'Missing required fields: id, name, schedule, handler' });
  }

  // Validate cron expression and minimum interval (>= 1 minute)
  try {
    const testJob = new Cron(schedule);
    const next1 = testJob.nextRun();
    const next2 = testJob.nextRun(next1);
    testJob.stop();
    if (next1 && next2 && (next2.getTime() - next1.getTime()) < 60000) {
      return res.status(400).json({ error: 'Minimum interval between runs must be at least 1 minute' });
    }
  } catch (err) {
    return res.status(400).json({ error: `Invalid cron expression: ${err.message}` });
  }

  const cronPath = path.join(__dirname, 'cron.json');
  let tasks = [];
  try {
    if (fs.existsSync(cronPath)) tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8'));
  } catch { tasks = []; }

  if (tasks.some(t => t.id === id)) {
    return res.status(409).json({ error: `Task with id "${id}" already exists` });
  }

  const newTask = { id, name, schedule, handler, enabled, timeout };
  tasks.push(newTask);
  fs.writeFileSync(cronPath, JSON.stringify(tasks, null, 2));
  _loadCronJobs();
  res.status(201).json(newTask);
});

app.patch('/api/cron/:id', (req, res) => {
  const cronPath = path.join(__dirname, 'cron.json');
  let tasks = [];
  try {
    if (fs.existsSync(cronPath)) tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8'));
  } catch { tasks = []; }

  const idx = tasks.findIndex(t => t.id === req.params.id);
  if (idx === -1) return res.status(404).json({ error: 'Task not found' });

  const updates = req.body || {};

  // If schedule is being updated, validate it
  if (updates.schedule) {
    try {
      const testJob = new Cron(updates.schedule);
      const next1 = testJob.nextRun();
      const next2 = testJob.nextRun(next1);
      testJob.stop();
      if (next1 && next2 && (next2.getTime() - next1.getTime()) < 60000) {
        return res.status(400).json({ error: 'Minimum interval between runs must be at least 1 minute' });
      }
    } catch (err) {
      return res.status(400).json({ error: `Invalid cron expression: ${err.message}` });
    }
  }

  tasks[idx] = { ...tasks[idx], ...updates, id: req.params.id };
  fs.writeFileSync(cronPath, JSON.stringify(tasks, null, 2));
  _loadCronJobs();
  res.json(tasks[idx]);
});

app.delete('/api/cron/:id', (req, res) => {
  const cronPath = path.join(__dirname, 'cron.json');
  let tasks = [];
  try {
    if (fs.existsSync(cronPath)) tasks = JSON.parse(fs.readFileSync(cronPath, 'utf8'));
  } catch { tasks = []; }

  const idx = tasks.findIndex(t => t.id === req.params.id);
  if (idx === -1) return res.status(404).json({ error: 'Task not found' });

  tasks.splice(idx, 1);
  fs.writeFileSync(cronPath, JSON.stringify(tasks, null, 2));
  _cronState.delete(req.params.id);
  _loadCronJobs();
  res.json({ ok: true });
});

app.post('/api/cron/:id/run', async (req, res) => {
  const job = _cronInstances.get(req.params.id);
  if (!job) return res.status(404).json({ error: 'Task not found or not enabled' });
  try {
    job.trigger();
    res.json({ ok: true, message: `Task ${req.params.id} triggered` });
  } catch (err) {
    res.status(500).json({ error: err.message });
  }
});

// Block /api/* (except health + sync + cron) until initial schema sync completes.
app.use('/api', (req, res, next) => {
  if (schemaReady || req.path === '/health' || req.path === '/__sync-schema' || req.path.startsWith('/cron')) return next();
  res.status(503).json({ error: 'Database schema initializing...' });
});

function startServer(retries = 10, delay = 1000) {
  const server = app.listen(PORT, '0.0.0.0', async () => {
    console.log(`Backend listening on port ${PORT}`);
    await syncSchemaWithRetry();
    schemaReady = true;
    console.log('Schema sync complete, API ready');

    // Load cron jobs after DB is ready
    _loadCronJobs();

    // Fallback: watch schema.js for changes in case agent doesn't call __sync-schema.
    const schemaPath = path.join(__dirname, 'db', 'schema.js');
    let syncDebounce = null;
    fs.watchFile(schemaPath, { interval: 2000 }, () => {
      if (syncDebounce) clearTimeout(syncDebounce);
      syncDebounce = setTimeout(async () => {
        console.log('DB: schema.js changed, re-syncing tables...');
        try {
          await syncSchemaWithRetry(2, 1500);
          console.log('DB: schema re-sync complete');
        } catch (err) {
          console.error(`DB: schema re-sync failed: ${err.message}`);
        }
      }, 1000);
    });
  });
  server.on('error', (err) => {
    if (err.code === 'EADDRINUSE' && retries > 0) {
      console.log(`Port ${PORT} in use, retrying in ${delay}ms... (${retries} left)`);
      setTimeout(() => startServer(retries - 1, delay), delay);
    } else {
      console.error(`Failed to start server: ${err.message}`);
      process.exit(1);
    }
  });
}
startServer();
