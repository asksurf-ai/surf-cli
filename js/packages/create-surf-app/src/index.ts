import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

import { FALLBACK_API_TS, generateApiFiles } from './api-codegen'

const DEFAULT_FRONTEND_PORT = '5173'
const DEFAULT_BACKEND_PORT = '3001'
const DEFAULT_TEMPLATE = 'default'

type CreateSurfAppOptions = {
  projectName?: string
  frontendPort?: string
  backendPort?: string
  previewBase?: string
  swaggerUrl?: string
  logger?: (line: string) => void
}

export async function createSurfApp({
  projectName = '.',
  frontendPort = process.env.VITE_PORT || DEFAULT_FRONTEND_PORT,
  backendPort = process.env.VITE_BACKEND_PORT || DEFAULT_BACKEND_PORT,
  previewBase = process.env.VITE_BASE,
  swaggerUrl,
  logger = console.log,
}: CreateSurfAppOptions = {}) {
  const root = path.resolve(projectName)
  const name = path.basename(root)
  const validatedFrontendPort = validatePort('frontend', frontendPort)
  const validatedBackendPort = validatePort('backend', backendPort)
  const templateDir = resolveTemplateDir(DEFAULT_TEMPLATE)

  logger(`\n  Creating Surf app in ${root}\n`)
  fs.mkdirSync(root, { recursive: true })

  copyDir(templateDir, root, root, logger)
  writeEnvFiles(root, validatedFrontendPort, validatedBackendPort, previewBase)
  await generateAndWriteApiFiles(root, swaggerUrl, logger)

  logger(`
Done! Next steps:

  cd ${name}
  cd backend && npm install && cd ..
  cd frontend && npm install && cd ..

  # Start backend
  cd backend && PORT=${validatedBackendPort} npm run dev &

  # Start frontend
  cd frontend && npm run dev

  Open http://localhost:${validatedFrontendPort}
`)

  return root
}

function resolveTemplateDir(templateName: string) {
  const here = path.dirname(fileURLToPath(import.meta.url))
  const candidates = [
    path.join(here, 'templates', templateName),
    path.join(here, '..', 'templates', templateName),
  ]

  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) return candidate
  }

  throw new Error(`Could not find template "${templateName}" near ${here}`)
}

function copyDir(src: string, dest: string, root: string, logger: (line: string) => void) {
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const srcPath = path.join(src, entry.name)
    const destPath = path.join(dest, entry.name)

    if (entry.isDirectory()) {
      fs.mkdirSync(destPath, { recursive: true })
      copyDir(srcPath, destPath, root, logger)
      continue
    }

    fs.mkdirSync(path.dirname(destPath), { recursive: true })
    fs.writeFileSync(destPath, fs.readFileSync(srcPath))
    logger(`  ${path.relative(root, destPath)}`)
  }
}

function validatePort(label: string, value: string) {
  const port = Number.parseInt(value, 10)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(`Invalid ${label} port: ${value}`)
  }
  return String(port)
}

function writeEnvFiles(
  root: string,
  frontendPort: string,
  backendPort: string,
  previewBase?: string,
) {
  const backendEnvPath = path.join(root, 'backend', '.env')
  const frontendEnvPath = path.join(root, 'frontend', '.env')

  fs.writeFileSync(backendEnvPath, `PORT=${backendPort}${os.EOL}`)

  let frontendEnv = `VITE_PORT=${frontendPort}${os.EOL}VITE_BACKEND_PORT=${backendPort}${os.EOL}`
  if (previewBase) {
    frontendEnv += `VITE_BASE=${previewBase}${os.EOL}`
  }
  fs.writeFileSync(frontendEnvPath, frontendEnv)
}

async function generateAndWriteApiFiles(
  root: string,
  swaggerUrl: string | undefined,
  logger: (line: string) => void,
) {
  const frontendLibDir = path.join(root, 'frontend', 'src', 'lib')
  const backendLibDir = path.join(root, 'backend', 'lib')

  fs.mkdirSync(frontendLibDir, { recursive: true })
  fs.mkdirSync(backendLibDir, { recursive: true })

  removeGeneratedApiFiles(frontendLibDir, backendLibDir)

  try {
    const apiFiles = await generateApiFiles({ swaggerUrl, logger })
    for (const [filename, content] of Object.entries(apiFiles)) {
      if (filename.startsWith('backend/')) {
        const relPath = filename.slice('backend/'.length)
        const filePath = path.join(backendLibDir, relPath)
        fs.mkdirSync(path.dirname(filePath), { recursive: true })
        fs.writeFileSync(filePath, content)
      } else {
        const filePath = path.join(frontendLibDir, filename)
        fs.mkdirSync(path.dirname(filePath), { recursive: true })
        fs.writeFileSync(filePath, content)
      }
    }
  } catch (error) {
    logger(`  failed to generate API files from swagger, using fallback: ${String(error)}`)
    fs.writeFileSync(path.join(frontendLibDir, 'api.ts'), FALLBACK_API_TS)
  }
}

function removeGeneratedApiFiles(frontendLibDir: string, backendLibDir: string) {
  if (fs.existsSync(frontendLibDir)) {
    for (const entry of fs.readdirSync(frontendLibDir)) {
      if (entry === 'fetch.ts' || entry === 'utils.ts') continue
      if (entry === 'api.ts' || entry === 'API_INDEX.md' || /^api-.*\.ts$/.test(entry) || /^types-.*\.ts$/.test(entry)) {
        fs.rmSync(path.join(frontendLibDir, entry), { force: true, recursive: true })
      }
    }
  }

  if (fs.existsSync(backendLibDir)) {
    for (const entry of fs.readdirSync(backendLibDir)) {
      if (entry === 'db.js') continue
      if (entry === 'api.js' || entry === 'API_INDEX.md' || /^api-.*\.js$/.test(entry)) {
        fs.rmSync(path.join(backendLibDir, entry), { force: true, recursive: true })
      }
    }
  }
}
