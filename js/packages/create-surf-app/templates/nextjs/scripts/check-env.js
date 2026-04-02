/**
 * Validates required env vars before running a command.
 * Loads .env, checks vars, then execs the actual command.
 *
 * Build: BASE_PATH, SURF_API_KEY
 * Dev/start: FRONTEND_PORT, BASE_PATH, SURF_API_KEY
 */
const fs = require('node:fs')
const path = require('node:path')
const { execSync } = require('node:child_process')

// Load .env manually
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
const isBuild = args.includes('build')

// Vars that must be non-empty
const requiredNonEmpty = isBuild
  ? []
  : ['FRONTEND_PORT', 'SURF_API_KEY']

// Vars that must be defined (empty is ok — e.g. BASE_PATH="" means root)
const requiredDefined = ['BASE_PATH']

const missingNonEmpty = requiredNonEmpty.filter(k => !process.env[k])
const missingDefined = requiredDefined.filter(k => process.env[k] === undefined)
const missing = [...missingNonEmpty, ...missingDefined]

if (missing.length > 0) {
  console.error(`\n❌ Missing required env vars: ${missing.join(', ')}`)
  console.error(`   Set them in your environment or copy .env.example to .env\n`)
  process.exit(1)
}

// Pass FRONTEND_PORT as PORT so Next.js uses it
if (process.env.FRONTEND_PORT) {
  process.env.PORT = process.env.FRONTEND_PORT
}

// Run the actual command
try {
  execSync(args.join(' '), { stdio: 'inherit', env: process.env })
} catch (e) {
  process.exit(e.status || 1)
}
