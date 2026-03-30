/**
 * Server-side database helpers.
 *
 * All SQL execution happens here in the backend — NEVER in frontend code.
 * Calls /proxy/db/* via loopback through the Express proxy middleware,
 * which handles both sandbox mode (OutboundProxy) and deployed mode (hermod).
 */

const PORT = Number.parseInt(process.env.PORT || '', 10);
if (!Number.isInteger(PORT)) {
  throw new Error('PORT env var is required');
}
const BASE = `http://127.0.0.1:${PORT}/proxy/db`;

async function request(path, options = {}) {
  const res = await fetch(`${BASE}${path}`, options);
  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: res.statusText }));
    const err = new Error(body.detail || body.error || body.message || `DB request failed: ${res.status}`);
    err.status = res.status;
    throw err;
  }
  return res.json();
}

/** Provision a database for the current user (idempotent). */
async function dbProvision(reason) {
  return request('/provision', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ reason: reason || '' }),
  });
}

/**
 * Execute a parameterized SQL query.
 * ALWAYS use $1, $2, ... placeholders — NEVER concatenate user input.
 * @param {string} sql
 * @param {any[]} params
 * @param {{ arrayMode?: boolean }} [options] - Pass { arrayMode: true } for Drizzle pg-proxy
 */
async function dbQuery(sql, params = [], options = {}) {
  return request('/query', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sql, params, ...options }),
  });
}

/** List all tables with approximate row counts. */
async function dbTables() {
  const data = await request('/tables');
  return data.tables;
}

/** Get column definitions for a table. */
async function dbTableSchema(name) {
  const data = await request(`/tables/${encodeURIComponent(name)}/schema`);
  return data.columns;
}

/** Check database connection status and usage. */
async function dbStatus() {
  return request('/status');
}

module.exports = { dbProvision, dbQuery, dbTables, dbTableSchema, dbStatus };
