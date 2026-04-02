// Database access — import this in your API routes.
// Schema sync runs automatically via instrumentation.ts on server boot.
//
// Usage in route handlers:
//   import { db } from '@/db'
//   const result = await db('SELECT * FROM users')

export { dbQuery as db, dbProvision, dbTables, dbTableSchema, dbStatus } from '@surf-ai/sdk/db'
