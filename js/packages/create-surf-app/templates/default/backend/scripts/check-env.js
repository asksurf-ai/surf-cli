/**
 * Validates required env vars before running a command.
 * Loads .env via --env-file, checks vars, then execs the actual command.
 *
 * Dev/start: BACKEND_PORT, SURF_API_KEY
 */
const { execSync } = require('node:child_process')

const args = process.argv.slice(2)

const required = ['BACKEND_PORT', 'SURF_API_KEY']
const missing = required.filter(k => !process.env[k])

if (missing.length > 0) {
  console.error(`\n❌ Missing required env vars in .env: ${missing.join(', ')}\n`)
  process.exit(1)
}

try {
  execSync(args.join(' '), { stdio: 'inherit', env: process.env })
} catch (e) {
  process.exit(e.status || 1)
}
