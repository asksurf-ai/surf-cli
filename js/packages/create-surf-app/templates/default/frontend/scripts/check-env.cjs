/**
 * Validates required env vars before running a command.
 * Loads .env manually, checks vars, then execs the actual command.
 *
 * Build: BASE_PATH (defined, can be empty), BACKEND_PORT
 * Dev: FRONTEND_PORT, BACKEND_PORT, BASE_PATH
 */
const fs = require('node:fs')
const path = require('node:path')
const { execSync } = require('node:child_process')

// Load .env manually (Vite doesn't load non-VITE_ vars into process.env)
const envPath = path.join(process.cwd(), '.env')
if (fs.existsSync(envPath)) {
  for (const line of fs.readFileSync(envPath, 'utf8').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const eq = trimmed.indexOf('=')
    if (eq < 0) continue
    const key = trimmed.slice(0, eq)
    const val = trimmed.slice(eq + 1)
    if (!process.env[key]) process.env[key] = val
  }
}

const args = process.argv.slice(2)
const isBuild = args.some(a => a.includes('build'))

// Vars that must be non-empty
const requiredNonEmpty = isBuild
  ? ['BACKEND_PORT']
  : ['FRONTEND_PORT', 'BACKEND_PORT']

// Vars that must be defined (empty is ok — BASE_PATH="" means root)
const requiredDefined = ['BASE_PATH']

const missingNonEmpty = requiredNonEmpty.filter(k => !process.env[k])
const missingDefined = requiredDefined.filter(k => process.env[k] === undefined)
const missing = [...missingNonEmpty, ...missingDefined]

if (missing.length > 0) {
  console.error(`\n❌ Missing required env vars in .env: ${missing.join(', ')}\n`)
  process.exit(1)
}

try {
  execSync(args.join(' '), { stdio: 'inherit', env: process.env })
} catch (e) {
  process.exit(e.status || 1)
}
