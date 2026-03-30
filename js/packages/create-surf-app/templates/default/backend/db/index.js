/**
 * Drizzle ORM client (pg-proxy driver).
 *
 * Routes all SQL through hermod's DB proxy — works identically in
 * sandbox (OutboundProxy) and deployed mode (hermod gateway).
 *
 * Usage:
 *   const { db } = require('./db');
 *   const { todos } = require('./db/schema');
 *   const rows = await db.select().from(todos);
 */

const { drizzle } = require('drizzle-orm/pg-proxy');
const { dbQuery } = require('../lib/db');

const db = drizzle(async (sql, params, method) => {
  const result = await dbQuery(sql, params, { arrayMode: true });
  // pg-proxy expects rows as positional arrays — arrayMode makes hermod
  // return [[val1, val2, ...], ...] instead of [{col: val}, ...].
  return { rows: result.rows || [] };
});

module.exports = { db };
