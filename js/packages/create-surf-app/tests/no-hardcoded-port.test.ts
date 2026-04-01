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
      backendPort: '20042',
      previewBase: '/preview/local/test/',
      logger: () => {},
    })

    const expectedFiles = [
      'CLAUDE.md',
      'backend/server.js',
      'backend/routes/.gitkeep',
      'backend/db/schema.js',
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
      'VITE_BACKEND_PORT=20042\nVITE_BASE=/preview/local/test/\n',
    )

    const frontendPackageJson = JSON.parse(
      fs.readFileSync(path.join(projectDir, 'frontend/package.json'), 'utf8'),
    )
    assert.equal(frontendPackageJson.scripts.build,
      'npm run build:client && npm run build:server',
    )
    assert.equal(frontendPackageJson.scripts.dev, 'vite')
    assert.equal(frontendPackageJson.dependencies['@surf-ai/sdk'], undefined)

    const viteConfig = fs.readFileSync(path.join(projectDir, 'frontend/vite.config.ts'), 'utf8')
    assert.match(viteConfig, /defineConfig\(\(\{ mode \}\) =>/)
    assert.match(viteConfig, /const env = loadEnv\(mode, process\.cwd\(\)\)/)
    assert.match(viteConfig, /readRequiredPort\(env, 'VITE_BACKEND_PORT'\)/)
    assert.match(viteConfig, /\[\`\$\{apiBasePrefix\}\/api\`\]: backendProxy/)
    assert.match(viteConfig, /const apiBasePrefix = hasAbsBase \? base\.replace/)
    assert.doesNotMatch(viteConfig, /apiProxyKey/)
    assert.doesNotMatch(viteConfig, /\/proxy/)
    assert.doesNotMatch(viteConfig, /warmup:/)
    assert.doesNotMatch(viteConfig, /server:\s*\{[\s\S]*port:/)
    assert.doesNotMatch(viteConfig, /'5173'/)
    assert.doesNotMatch(viteConfig, /'3001'/)

    const backendPackageJson = JSON.parse(
      fs.readFileSync(path.join(projectDir, 'backend/package.json'), 'utf8'),
    )
    assert.equal(backendPackageJson.scripts.start, 'node server.js')
    assert.equal(backendPackageJson.scripts.dev, 'node --watch server.js')

    const backendServer = fs.readFileSync(path.join(projectDir, 'backend/server.js'), 'utf8')
    assert.match(backendServer, /createServer/)
    assert.match(backendServer, /proxy:\s*false/)
    assert.match(backendServer, /@surf-ai\/sdk\/server/)

    assert.equal(fs.existsSync(path.join(projectDir, 'backend/routes/proxy.js')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'backend/lib/db.js')), false)

    const appTsx = fs.readFileSync(path.join(projectDir, 'frontend/src/App.tsx'), 'utf8')
    assert.match(appTsx, /src\/lib\/api\.ts/)

    const apiHelper = fs.readFileSync(path.join(projectDir, 'frontend/src/lib/api.ts'), 'utf8')
    assert.match(apiHelper, /export function api\(path: string\)/)
    assert.match(apiHelper, /import\.meta\.env\.BASE_URL/)
    assert.match(apiHelper, /path\.replace\(/)

    const claudeMd = fs.readFileSync(path.join(projectDir, 'CLAUDE.md'), 'utf8')
    assert.match(claudeMd, /fetch\(api\('wallet'\)\)/)
    assert.match(claudeMd, /frontend\/src\/lib\/api\.ts/)
    assert.match(claudeMd, /Use the scaffolded `api\(path\)` helper/)
    assert.match(claudeMd, /Never use absolute `\/api\/\.\.\.` URLs in frontend fetch calls/)
    assert.match(claudeMd, /Use `@surf-ai\/sdk\/server` `dataApi` in backend code/)
    assert.doesNotMatch(claudeMd, /@surf-ai\/sdk\/react/)
    assert.doesNotMatch(claudeMd, /\/proxy\/\*/)
  })

  test('does not generate placeholder frontend API or schema files', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      backendPort: '20042',
      logger: () => {},
    })

    assert.equal(fs.existsSync(path.join(projectDir, 'frontend/src/lib/fetch.ts')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'frontend/src/db/schema.ts')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'frontend/src/lib/api-market.ts')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'backend/lib/api.js')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'frontend/src/lib/types-common.ts')), false)
  })

  test('uses env fallback ports when flags are omitted', async () => {
    const originalBackendPort = process.env.VITE_BACKEND_PORT
    const originalBase = process.env.VITE_BASE
    const projectDir = makeTempProject()
    process.env.VITE_BACKEND_PORT = '26000'
    process.env.VITE_BASE = '/preview/env/test/'

    try {
      await createSurfApp({ projectName: projectDir, logger: () => {} })
    } finally {
      process.env.VITE_BACKEND_PORT = originalBackendPort
      process.env.VITE_BASE = originalBase
    }

    const frontendEnv = fs.readFileSync(path.join(projectDir, 'frontend/.env'), 'utf8')
    assert.match(frontendEnv, /VITE_BACKEND_PORT=26000/)
    assert.match(frontendEnv, /VITE_BASE=\/preview\/env\/test\//)
    assert.doesNotMatch(frontendEnv, /VITE_PORT=/)
  })
})
