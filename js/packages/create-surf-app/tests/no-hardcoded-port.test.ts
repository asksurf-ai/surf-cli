import assert from 'node:assert/strict'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { afterEach, describe, test } from 'node:test'

import { createSurfApp } from '../src/index'

const tempDirs: string[] = []

afterEach(() => {
  while (tempDirs.length > 0) {
    fs.rmSync(tempDirs.pop()!, { recursive: true, force: true })
  }
})

function makeTempProject() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'create-surf-app-'))
  tempDirs.push(dir)
  return path.join(dir, 'app')
}

describe('create-surf-app', () => {
  test('generates the canonical scaffold and env files', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      frontendPort: '15042',
      backendPort: '20042',
      previewBase: '/preview/local/test/',
      logger: () => {},
    })

    const expectedFiles = [
      'CLAUDE.md',
      'backend/server.js',
      'backend/routes/proxy.js',
      'backend/lib/db.js',
      'frontend/index.html',
      'frontend/src/entry-client.tsx',
      'frontend/src/entry-server.tsx',
      'frontend/src/components/ui/button.tsx',
      'frontend/src/lib/api.ts',
      'frontend/vite.config.ts',
    ]

    for (const relPath of expectedFiles) {
      assert.equal(fs.existsSync(path.join(projectDir, relPath)), true)
    }

    assert.equal(fs.readFileSync(path.join(projectDir, 'backend/.env'), 'utf8'), 'PORT=20042\n')
    assert.equal(fs.readFileSync(path.join(projectDir, 'frontend/.env'), 'utf8'),
      'VITE_PORT=15042\nVITE_BACKEND_PORT=20042\nVITE_BASE=/preview/local/test/\n',
    )

    const frontendPackageJson = JSON.parse(
      fs.readFileSync(path.join(projectDir, 'frontend/package.json'), 'utf8'),
    )
    assert.equal(frontendPackageJson.scripts.build,
      'npm run build:client && npm run build:server',
    )
    assert.equal(frontendPackageJson.scripts.dev, 'vite')

    const viteConfig = fs.readFileSync(path.join(projectDir, 'frontend/vite.config.ts'), 'utf8')
    assert.match(viteConfig, /readRequiredPort\('VITE_PORT'\)/)
    assert.doesNotMatch(viteConfig, /'5173'/)
    assert.doesNotMatch(viteConfig, /'3001'/)

    const backendServer = fs.readFileSync(path.join(projectDir, 'backend/server.js'), 'utf8')
    assert.doesNotMatch(backendServer, /'3001'/)
    assert.match(backendServer, /PORT env var is required/)

    const backendDb = fs.readFileSync(path.join(projectDir, 'backend/lib/db.js'), 'utf8')
    assert.doesNotMatch(backendDb, /'3001'/)
    assert.match(backendDb, /PORT env var is required/)
  })

  test('does not generate swagger-derived API files', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      frontendPort: '15042',
      backendPort: '20042',
      logger: () => {},
    })

    assert.equal(fs.existsSync(path.join(projectDir, 'frontend/src/lib/api.ts')), true)
    assert.equal(fs.existsSync(path.join(projectDir, 'frontend/src/lib/api-market.ts')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'backend/lib/api.js')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'frontend/src/lib/types-common.ts')), false)
  })

  test('uses env fallback ports when flags are omitted', async () => {
    const originalFrontendPort = process.env.VITE_PORT
    const originalBackendPort = process.env.VITE_BACKEND_PORT
    const originalBase = process.env.VITE_BASE
    const projectDir = makeTempProject()
    process.env.VITE_PORT = '16000'
    process.env.VITE_BACKEND_PORT = '26000'
    process.env.VITE_BASE = '/preview/env/test/'

    try {
      await createSurfApp({ projectName: projectDir, logger: () => {} })
    } finally {
      process.env.VITE_PORT = originalFrontendPort
      process.env.VITE_BACKEND_PORT = originalBackendPort
      process.env.VITE_BASE = originalBase
    }

    const frontendEnv = fs.readFileSync(path.join(projectDir, 'frontend/.env'), 'utf8')
    assert.match(frontendEnv, /VITE_PORT=16000/)
    assert.match(frontendEnv, /VITE_BACKEND_PORT=26000/)
    assert.match(frontendEnv, /VITE_BASE=\/preview\/env\/test\//)
  })
})
