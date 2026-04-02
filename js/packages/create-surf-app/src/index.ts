import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const DEFAULT_BACKEND_PORT = '3001'
const VALID_TEMPLATES = ['vite', 'nextjs'] as const
type Template = (typeof VALID_TEMPLATES)[number]

type CreateSurfAppOptions = {
  projectName?: string
  backendPort?: string
  previewBase?: string
  template?: string
  logger?: (line: string) => void
}

export async function createSurfApp({
  projectName = '.',
  backendPort = process.env.BACKEND_PORT || DEFAULT_BACKEND_PORT,
  previewBase = process.env.BASE_PATH,
  template: templateArg,
  logger = console.log,
}: CreateSurfAppOptions = {}) {
  const root = path.resolve(projectName)
  const name = path.basename(root)
  const template = validateTemplate(templateArg)
  const validatedBackendPort = validatePort('backend', backendPort)
  const templateDir = resolveTemplateDir(template)

  logger(`\n  Creating Surf app (${template}) in ${root}\n`)
  fs.mkdirSync(root, { recursive: true })

  copyDir(templateDir, root, root, logger)

  if (template === 'nextjs') {
    writeNextjsEnvFile(root, validatedBackendPort, previewBase)
    finalizePackageName(root, name)
    logger(`
Done! Next steps:

  cd ${name}
  npm install
  npm run dev

  Open http://localhost:3000
`)
  } else {
    writeEnvFiles(root, validatedBackendPort, previewBase)
    logger(`
Done! Next steps:

  cd ${name}
  cd backend && npm install && cd ..
  cd frontend && npm install && cd ..

  # Start backend
  cd backend && npm run dev &

  # Start frontend
  cd frontend && npm run dev

  Open the local URL printed by Vite
`)
  }

  return root
}

function validateTemplate(template?: string): Template {
  if (!template) return 'vite'
  if (!VALID_TEMPLATES.includes(template as Template)) {
    throw new Error(`Unknown template: ${template}. Valid templates: ${VALID_TEMPLATES.join(', ')}`)
  }
  return template as Template
}

function resolveTemplateDir(template: Template = 'vite') {
  const dirName = template === 'vite' ? 'default' : template
  const here = path.dirname(fileURLToPath(import.meta.url))
  const candidates = [
    path.join(here, 'templates', dirName),
    path.join(here, '..', 'templates', dirName),
  ]

  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) return candidate
  }

  throw new Error(`Could not find ${template} template near ${here}`)
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

function finalizePackageName(root: string, projectName: string) {
  const pkgPath = path.join(root, 'package.json')
  if (!fs.existsSync(pkgPath)) return
  const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'))
  pkg.name = projectName
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9._-]+/g, '-')
    .replace(/^-+|-+$/g, '') || 'surf-app'
  fs.writeFileSync(pkgPath, `${JSON.stringify(pkg, null, 2)}${os.EOL}`)
}

function writeNextjsEnvFile(root: string, port: string, basePath?: string) {
  const envPath = path.join(root, '.env')
  const envContent = [
    `FRONTEND_PORT=${port}`,
    `BASE_PATH=${basePath || ''}`,
    'SURF_API_KEY=',
  ].join(os.EOL)
  fs.writeFileSync(envPath, `${envContent}${os.EOL}`)
}

function writeEnvFiles(
  root: string,
  backendPort: string,
  previewBase?: string,
) {
  const backendEnvPath = path.join(root, 'backend', '.env')
  const frontendEnvPath = path.join(root, 'frontend', '.env')

  const backendEnv = [
    `BACKEND_PORT=${backendPort}`,
    'SURF_API_KEY=',
  ].join(os.EOL)

  fs.writeFileSync(backendEnvPath, `${backendEnv}${os.EOL}`)

  const frontendEnv = [
    'FRONTEND_PORT=5173',
    `BACKEND_PORT=${backendPort}`,
    `BASE_PATH=${previewBase || ''}`,
  ].join(os.EOL)
  fs.writeFileSync(frontendEnvPath, `${frontendEnv}${os.EOL}`)
}
