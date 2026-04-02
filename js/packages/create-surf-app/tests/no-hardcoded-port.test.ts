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

    const backendEnv = fs.readFileSync(path.join(projectDir, 'backend/.env'), 'utf8')
    assert.match(backendEnv, /^BACKEND_PORT=20042/m)
    assert.match(backendEnv, /SURF_API_KEY=/)
    assert.equal(fs.readFileSync(path.join(projectDir, 'frontend/.env'), 'utf8'),
      'FRONTEND_PORT=5173\nBACKEND_PORT=20042\nBASE_PATH=/preview/local/test/\n',
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
    assert.match(viteConfig, /const env = loadEnv\(mode, process\.cwd\(\), ''\)/)
    assert.match(viteConfig, /readRequiredPort\(env, 'BACKEND_PORT'\)/)
    assert.match(viteConfig, /\[\`\$\{apiBasePrefix\}\/api\`\]: backendProxy/)
    assert.match(viteConfig, /const apiBasePrefix = hasAbsBase \? base\.replace/)
    assert.doesNotMatch(viteConfig, /apiProxyKey/)
    assert.doesNotMatch(viteConfig, /\/proxy/)
    assert.doesNotMatch(viteConfig, /warmup:/)
    assert.match(viteConfig, /port: frontendPort/)
    assert.doesNotMatch(viteConfig, /'5173'/)
    assert.doesNotMatch(viteConfig, /'3001'/)

    const backendPackageJson = JSON.parse(
      fs.readFileSync(path.join(projectDir, 'backend/package.json'), 'utf8'),
    )
    assert.equal(backendPackageJson.scripts.start, 'node --env-file=.env server.js')
    assert.equal(backendPackageJson.scripts.dev, 'node --env-file=.env --watch server.js')
    assert.equal(backendPackageJson.dependencies['@surf-ai/sdk'], '1.0.0-alpha.0')

    const backendServer = fs.readFileSync(path.join(projectDir, 'backend/server.js'), 'utf8')
    assert.match(backendServer, /createServer/)
    assert.match(backendServer, /createServer\(\)\.start\(\)/)
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

  test('vite scaffold env files contain all required env vars', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      backendPort: '4000',
      logger: () => {},
    })

    const backendEnv = fs.readFileSync(path.join(projectDir, 'backend/.env'), 'utf8')
    assert.match(backendEnv, /^BACKEND_PORT=4000/m, 'backend .env must have BACKEND_PORT')
    assert.match(backendEnv, /^SURF_API_KEY=/m, 'backend .env must have SURF_API_KEY')
    assert.doesNotMatch(backendEnv, /SURF_API_BASE_URL/, 'backend .env must not have optional vars')

    const frontendEnv = fs.readFileSync(path.join(projectDir, 'frontend/.env'), 'utf8')
    assert.match(frontendEnv, /^FRONTEND_PORT=5173/m, 'frontend .env must have FRONTEND_PORT')
    assert.match(frontendEnv, /^BACKEND_PORT=4000/m, 'frontend .env must have BACKEND_PORT')
  })

  test('generates nextjs template with correct structure', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      template: 'nextjs',
      logger: () => {},
    })

    // Core files must exist
    const expectedFiles = [
      'CLAUDE.md',
      'package.json',
      'next.config.ts',
      'tsconfig.json',
      'instrumentation.ts',
      'app/layout.tsx',
      'app/page.tsx',
      'app/providers.tsx',
      'app/globals.css',
      'app/api/health/route.ts',
      'app/api/__sync-schema/route.ts',
      'app/api/cron/route.ts',
      'app/api/market/price/route.ts',
      'db/index.ts',
      'db/schema.ts',
      'lib/boot.ts',
      'lib/utils.ts',
      'hooks/use-toast.ts',
      'components/ui/button.tsx',
      'components/ui/dialog.tsx',
      'components/ui/form.tsx',
    ]

    for (const relPath of expectedFiles) {
      assert.equal(
        fs.existsSync(path.join(projectDir, relPath)),
        true,
        `Expected ${relPath} to exist`,
      )
    }

    // Must NOT have Vite template artifacts
    assert.equal(fs.existsSync(path.join(projectDir, 'frontend')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'backend')), false)
    assert.equal(fs.existsSync(path.join(projectDir, 'vite.config.ts')), false)
  })

  test('nextjs scaffold env file contains all required env vars', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      template: 'nextjs',
      backendPort: '5000',
      logger: () => {},
    })

    const envFile = fs.readFileSync(path.join(projectDir, '.env'), 'utf8')
    assert.match(envFile, /^FRONTEND_PORT=5000/m, '.env must have FRONTEND_PORT')
    assert.match(envFile, /^SURF_API_KEY=/m, '.env must have SURF_API_KEY')
    assert.doesNotMatch(envFile, /SURF_API_BASE_URL/, '.env must not have optional vars')
  })

  test('nextjs scaffold package.json has correct name and deps', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      template: 'nextjs',
      logger: () => {},
    })

    const pkg = JSON.parse(fs.readFileSync(path.join(projectDir, 'package.json'), 'utf8'))
    assert.match(pkg.scripts.dev, /next dev/)
    assert.match(pkg.scripts.build, /next build/)
    assert.match(pkg.scripts.start, /next start/)
    assert.equal(pkg.dependencies['@surf-ai/sdk'], '1.0.0-alpha.0')
    assert.equal(pkg.dependencies.next != null, true, 'must have next dependency')
    assert.equal(pkg.dependencies.react != null, true, 'must have react dependency')
    assert.equal(pkg.dependencies['drizzle-orm'] != null, true, 'must have drizzle-orm')
    assert.equal(pkg.dependencies.croner != null, true, 'must have croner')
    assert.equal(pkg.dependencies['@tanstack/react-query'] != null, true, 'must have react-query')
  })

  test('nextjs CLAUDE.md has correct agent instructions', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      template: 'nextjs',
      logger: () => {},
    })

    const claudeMd = fs.readFileSync(path.join(projectDir, 'CLAUDE.md'), 'utf8')
    assert.match(claudeMd, /@surf-ai\/sdk\/server/, 'must reference SDK server import')
    assert.match(claudeMd, /dataApi/, 'must reference dataApi')
    assert.match(claudeMd, /use client/, 'must mention use client directive')
    assert.match(claudeMd, /app\/api\//, 'must reference API route pattern')
    assert.match(claudeMd, /Do NOT modify/, 'must have do-not-modify section')
    assert.match(claudeMd, /instrumentation\.ts/, 'must list instrumentation.ts as do-not-modify')
    assert.match(claudeMd, /db\/schema\.ts/, 'must reference schema file')
  })

  test('nextjs instrumentation.ts checks for SURF_API_KEY', async () => {
    const projectDir = makeTempProject()

    await createSurfApp({
      projectName: projectDir,
      template: 'nextjs',
      logger: () => {},
    })

    const instrumentation = fs.readFileSync(path.join(projectDir, 'instrumentation.ts'), 'utf8')
    assert.match(instrumentation, /SURF_API_KEY/, 'must check for SURF_API_KEY')
    assert.match(instrumentation, /syncSchema/, 'must call syncSchema')
    assert.match(instrumentation, /watchSchema/, 'must call watchSchema')
    assert.match(instrumentation, /startCron/, 'must call startCron')
  })

  test('rejects unknown templates', async () => {
    await assert.rejects(
      () => createSurfApp({
        projectName: makeTempProject(),
        template: 'nope',
        logger: () => {},
      }),
      /Unknown template: nope/,
    )
  })

  test('uses env fallback ports when flags are omitted', async () => {
    const originalBackendPort = process.env.BACKEND_PORT
    const originalBase = process.env.BASE_PATH
    const projectDir = makeTempProject()
    process.env.BACKEND_PORT = '26000'
    process.env.BASE_PATH = '/preview/env/test/'

    try {
      await createSurfApp({ projectName: projectDir, logger: () => {} })
    } finally {
      process.env.BACKEND_PORT = originalBackendPort
      process.env.BASE_PATH = originalBase
    }

    const frontendEnv = fs.readFileSync(path.join(projectDir, 'frontend/.env'), 'utf8')
    assert.match(frontendEnv, /FRONTEND_PORT=5173/)
    assert.match(frontendEnv, /BACKEND_PORT=26000/)
    assert.match(frontendEnv, /BASE_PATH=\/preview\/env\/test\//)
  })
})
