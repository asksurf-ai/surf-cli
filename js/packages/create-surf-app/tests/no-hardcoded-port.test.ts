import { describe, expect, test } from 'bun:test'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const packageRoot = path.resolve(__dirname, '..')

const hardcodedPortPatterns = [
  /getFlag\('--port'\s*,/g,
  /getFlag\('--backend-port'\s*,/g,
  /--port\s+\d+\b/g,
  /--backend-port\s+\d+\b/g,
  /\bport\s*:\s*\d+\b/g,
  /localhost:\d+\b/g,
  /127\.0\.0\.1:\d+\b/g,
  /VITE_PORT\s*\|\|\s*['"`]\d+['"`]/g,
  /VITE_BACKEND_PORT\s*\|\|\s*['"`]\d+['"`]/g,
]

const allowedMatches = new Map<string, string[]>([
  ['src/index.ts', [
    'npx create-surf-app my-app --port <frontend-port> --backend-port <backend-port>',
    'npx create-surf-app . --port <frontend-port> --backend-port <backend-port>',
    'http://127.0.0.1:${BACKEND_PORT}',
    'Open http://localhost:${frontendPort}',
  ]],
])

function collectFiles(dir: string): string[] {
  const files: string[] = []

  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...collectFiles(fullPath))
      continue
    }
    files.push(fullPath)
  }

  return files
}

function findUnexpectedMatches(relPath: string, content: string): string[] {
  const allowed = allowedMatches.get(relPath) || []
  const failures: string[] = []

  for (const pattern of hardcodedPortPatterns) {
    for (const match of content.matchAll(pattern)) {
      const value = match[0]
      if (!allowed.includes(value)) {
        failures.push(value)
      }
    }
  }

  return failures
}

describe('create-surf-app', () => {
  test('does not hardcode ports in source or templates', () => {
    const roots = [
      path.join(packageRoot, 'src'),
      path.join(packageRoot, 'templates'),
    ]

    const failures: string[] = []

    for (const root of roots) {
      for (const file of collectFiles(root)) {
        const relPath = path.relative(packageRoot, file)
        const content = fs.readFileSync(file, 'utf8')
        const matches = findUnexpectedMatches(relPath, content)
        for (const match of matches) {
          failures.push(`${relPath}: ${match}`)
        }
      }
    }

    expect(failures).toEqual([])
  })
})
