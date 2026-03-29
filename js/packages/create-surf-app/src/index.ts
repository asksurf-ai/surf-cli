#!/usr/bin/env node
/**
 * create-surf-app — Scaffold a Surf app with @surf-ai/sdk.
 *
 * Usage:
 *   npx create-surf-app my-app --port <frontend-port> --backend-port <backend-port>
 *   npx create-surf-app . --port <frontend-port> --backend-port <backend-port>
 */

import fs from 'fs'
import path from 'path'

// ---------------------------------------------------------------------------
// CLI args
// ---------------------------------------------------------------------------

const args = process.argv.slice(2)
const projectName = args.find(a => !a.startsWith('--')) || '.'
const frontendPort = getFlag('--port') || process.env.VITE_PORT || '5173'
const backendPort = getFlag('--backend-port') || process.env.VITE_BACKEND_PORT || '3001'

function getFlag(name: string): string | undefined {
  const idx = args.indexOf(name)
  return idx >= 0 && args[idx + 1] ? args[idx + 1] : undefined
}

// ---------------------------------------------------------------------------
// Frontend package.json — full dependency list matching urania scaffold
// ---------------------------------------------------------------------------

const frontendPkg = {
  name: 'frontend',
  private: true,
  type: 'module',
  scripts: {
    dev: 'vite',
    build: 'vite build',
    preview: 'vite preview',
    lint: 'eslint .',
  },
  dependencies: {
    '@surf-ai/sdk': 'latest',
    '@surf-ai/theme': 'latest',
    '@radix-ui/react-accordion': '1.2.12',
    '@radix-ui/react-aspect-ratio': '1.1.8',
    '@radix-ui/react-avatar': '1.1.11',
    '@radix-ui/react-checkbox': '1.3.3',
    '@radix-ui/react-collapsible': '1.1.12',
    '@radix-ui/react-context-menu': '2.2.16',
    '@radix-ui/react-dialog': '1.1.15',
    '@radix-ui/react-dropdown-menu': '2.1.16',
    '@radix-ui/react-hover-card': '1.1.15',
    '@radix-ui/react-label': '2.1.8',
    '@radix-ui/react-menubar': '1.1.16',
    '@radix-ui/react-navigation-menu': '1.2.14',
    '@radix-ui/react-popover': '1.1.15',
    '@radix-ui/react-progress': '1.1.8',
    '@radix-ui/react-radio-group': '1.3.8',
    '@radix-ui/react-scroll-area': '1.2.10',
    '@radix-ui/react-select': '2.2.6',
    '@radix-ui/react-separator': '1.1.8',
    '@radix-ui/react-slider': '1.3.6',
    '@radix-ui/react-slot': '1.2.4',
    '@radix-ui/react-switch': '1.2.6',
    '@radix-ui/react-tabs': '1.1.13',
    '@radix-ui/react-toast': '1.2.15',
    '@radix-ui/react-toggle': '1.1.10',
    '@radix-ui/react-toggle-group': '1.1.11',
    '@radix-ui/react-tooltip': '1.2.8',
    '@tanstack/react-query': '5.94.5',
    'class-variance-authority': '0.7.1',
    'clsx': '2.1.1',
    'cmdk': '1.1.1',
    'echarts': '5.6.0',
    'echarts-for-react': '3.0.6',
    'embla-carousel-react': '8.6.0',
    'lucide-react': '0.454.0',
    'next-themes': '0.4.6',
    'react': '19.2.4',
    'react-dom': '19.2.4',
    'react-router-dom': '7.6.1',
    'sonner': '1.7.4',
    'tailwind-merge': '2.6.1',
    'vaul': '1.1.2',
    'zod': '3.25.76',
  },
  devDependencies: {
    '@eslint/js': '9.39.4',
    '@tailwindcss/vite': '4.2.2',
    '@types/node': '22.19.15',
    '@types/react': '19.2.14',
    '@types/react-dom': '19.2.3',
    '@vitejs/plugin-react': '4.7.0',
    'eslint': '9.39.4',
    'eslint-plugin-react-hooks': '5.2.0',
    'eslint-plugin-react-refresh': '0.4.26',
    'globals': '16.5.0',
    'tailwindcss': '4.2.2',
    'tw-animate-css': '1.4.0',
    'typescript': '5.9.3',
    'typescript-eslint': '8.57.1',
    'vite': '6.4.1',
  },
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

const templates: Record<string, string> = {
  // ── Backend ──────────────────────────────────────────────────────────────

  'backend/package.json': JSON.stringify({
    name: 'backend',
    private: true,
    scripts: {
      start: 'node server.js',
      dev: 'node --env-file=.env --watch server.js',
    },
    dependencies: {
      '@surf-ai/sdk': 'latest',
      'express': '^4.22.0',
    },
  }, null, 2),

  'backend/server.js': `const { createServer } = require('@surf-ai/sdk/server')
createServer().start()
`,

  'backend/routes/.gitkeep': '',

  'backend/db/schema.js': `// Define your Drizzle ORM tables here.
// Example:
//   const { pgTable, serial, text, timestamp } = require('drizzle-orm/pg-core')
//   exports.users = pgTable('users', {
//     id: serial('id').primaryKey(),
//     name: text('name').notNull(),
//     created_at: timestamp('created_at').defaultNow(),
//   })
`,

  'backend/.env': `PORT=${backendPort}
`,

  // ── Frontend ─────────────────────────────────────────────────────────────

  'frontend/package.json': JSON.stringify(frontendPkg, null, 2),

  'frontend/.env': `VITE_PORT=${frontendPort}
VITE_BACKEND_PORT=${backendPort}
VITE_BASE=${process.env.VITE_BASE || '/'}
`,

  'frontend/vite.config.ts': `import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

const env = loadEnv('', process.cwd(), '')

function requiredPort(name: string) {
  const value = env[name]
  if (!value) {
    throw new Error(\`Missing \${name}. Set it in frontend/.env or your shell environment.\`)
  }

  const port = Number.parseInt(value, 10)
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error(\`Invalid \${name}: \${value}\`)
  }

  return port
}

const FRONTEND_PORT = requiredPort('VITE_PORT')
const BACKEND_PORT = requiredPort('VITE_BACKEND_PORT')
const BASE = env.VITE_BASE || '/'

export default defineConfig({
  base: BASE,
  plugins: [react(), tailwindcss()],
  server: {
    port: FRONTEND_PORT,
    host: '0.0.0.0',
    proxy: {
      '/proxy': { target: \`http://127.0.0.1:\${BACKEND_PORT}\`, changeOrigin: true },
      '/api': { target: \`http://127.0.0.1:\${BACKEND_PORT}\`, changeOrigin: true },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
})
`,

  // index.html comes from templates/ directory (has cold-start guard script)

  'frontend/tsconfig.json': JSON.stringify({
    compilerOptions: {
      target: 'ES2020',
      module: 'ESNext',
      moduleResolution: 'bundler',
      jsx: 'react-jsx',
      strict: true,
      esModuleInterop: true,
      skipLibCheck: true,
      paths: { '@/*': ['./src/*'] },
    },
    include: ['src'],
  }, null, 2),

  'frontend/components.json': JSON.stringify({
    "$schema": "https://ui.shadcn.com/schema.json",
    "style": "default",
    "rsc": false,
    "tsx": true,
    "tailwind": {
      "config": "",
      "css": "src/index.css",
      "baseColor": "slate",
      "cssVariables": true,
    },
    "aliases": {
      "components": "@/components",
      "utils": "@surf-ai/sdk/react",
      "hooks": "@/hooks",
    },
  }, null, 2),

  // entry-client.tsx replaces main.tsx — comes from templates/ directory

  'frontend/src/App.tsx': `import { useMarketPrice } from '@surf-ai/sdk/react'

export default function App() {
  const { data, isLoading, error } = useMarketPrice({ symbol: 'BTC', time_range: '1d' })

  return (
    <div className="min-h-screen bg-background text-foreground p-10">
      <h1 className="text-3xl font-bold mb-4">Surf App</h1>
      {isLoading && <p className="text-muted-foreground">Loading BTC price...</p>}
      {error && <p className="text-destructive">Error: {(error as Error).message}</p>}
      {data?.data?.[0] && (
        <p className="text-4xl font-bold">
          BTC: <span className="text-primary">\${data.data[0].value?.toLocaleString()}</span>
        </p>
      )}
    </div>
  )
}
`,

  'frontend/src/index.css': `@import url('https://fonts.googleapis.com/css2?family=Lato:wght@400;600;700;900&family=Roboto+Mono:wght@400;500&display=swap');
@import "tailwindcss";
@import "tw-animate-css";
@import "@surf-ai/theme";
`,

  // cn() and useToast() are now in @surf-ai/sdk/react

  'frontend/src/db/schema.ts': `// Database schema definition — keep in sync with backend/db/schema.js.
// This file mirrors the backend schema for TypeScript type safety in the frontend.
//
// Example:
//   import { pgTable, serial, text, timestamp } from 'drizzle-orm/pg-core'
//   export const users = pgTable('users', {
//     id: serial('id').primaryKey(),
//     name: text('name').notNull(),
//     created_at: timestamp('created_at').defaultNow(),
//   })
`,

  'frontend/src/vite-env.d.ts': `/// <reference types="vite/client" />
`,

  // ── Root ──────────────────────────────────────────────────────────────────

  'CLAUDE.md': `# Project

Built with [Surf SDK](https://github.com/cyberconnecthq/urania/tree/main/packages/sdk).

## Imports from @surf-ai/sdk

Everything comes from \`@surf-ai/sdk\`. Do NOT create local utility files for these.

**Frontend (\`@surf-ai/sdk/react\`):**
\`\`\`tsx
import { useMarketPrice, useTokenHolders } from '@surf-ai/sdk/react'  // data hooks
import { cn } from '@surf-ai/sdk/react'                                // Tailwind class merge
import { useToast, toast } from '@surf-ai/sdk/react'                   // toast notifications
\`\`\`

**Backend (\`@surf-ai/sdk/server\`):**
\`\`\`js
const { dataApi } = require('@surf-ai/sdk/server')
const data = await dataApi.market.price({ symbol: 'BTC' })
const holders = await dataApi.token.holders({ address: '0x...', chain: 'ethereum' })
// Escape hatch for new endpoints:
const raw = await dataApi.get('newcategory/endpoint', { foo: 'bar' })
\`\`\`

## Structure

\`\`\`
frontend/src/App.tsx       - build your UI here
frontend/src/components/   - add components
frontend/src/db/schema.ts  - frontend DB schema mirror
backend/routes/*.js        - add API routes (auto-mounted at /api/{name})
backend/db/schema.js       - define database tables
\`\`\`

## Built-in Endpoints (from @surf-ai/sdk/server)

\`createServer()\` provides these automatically — do NOT create routes for them:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| \`/api/health\` | GET | Health check — \`{ status: 'ok' }\` |
| \`/api/__sync-schema\` | POST | Sync \`backend/db/schema.js\` tables to database |
| \`/api/cron\` | GET | List cron jobs with status and next run time |
| \`/api/cron\` | POST | Create a new cron task |
| \`/api/cron/:id\` | PATCH | Update a cron task (schedule, enabled, etc.) |
| \`/api/cron/:id\` | DELETE | Delete a cron task |
| \`/api/cron/:id/run\` | POST | Manually trigger a cron task |
| \`/proxy/*\` | ANY | Data API passthrough — \`/proxy/market/price\` → hermod |

Auto-registered from \`backend/routes/*.js\`:
| File | Endpoint |
|------|----------|
| \`routes/btc.js\` | \`/api/btc\` |
| \`routes/portfolio.js\` | \`/api/portfolio\` |

## Database

Define tables in \`backend/db/schema.js\` using Drizzle ORM:
\`\`\`js
const { pgTable, serial, text, timestamp } = require('drizzle-orm/pg-core')
exports.users = pgTable('users', {
  id: serial('id').primaryKey(),
  name: text('name').notNull(),
  created_at: timestamp('created_at').defaultNow(),
})
\`\`\`

Tables are auto-created on startup and when \`schema.js\` changes (file watcher).
The agent can also call \`POST /api/__sync-schema\` explicitly after editing.

## Dev Servers

- Frontend: \`cd frontend && npm run dev\` (port from \`.env\`, do NOT pass \`--port\`)
- Backend: \`cd backend && npm run dev\` (port from \`.env\`)
- After \`npm install\` new packages, do NOT restart servers — Vite auto-discovers deps, backend uses \`node --watch\`
- If a server crashes, restart with \`npm run dev\` in that directory
- NEVER use \`npx vite\` — always use \`npm run dev\`

## Do NOT modify

- \`vite.config.ts\` — proxy and build config
- \`frontend/.env\` — port configuration (auto-generated)
- \`backend/server.js\` — uses @surf-ai/sdk/server
- \`entry-client.tsx\` — app bootstrap with SSR hydration
- \`entry-server.tsx\` — SSR render for deploy
- \`index.html\` — cold-start guard and Surf badge
- \`eslint.config.*\` — lint rules
- \`index.css\` — only imports, do not add styles here (use Tailwind classes)

## Rules

- Use \`@surf-ai/sdk/react\` hooks in frontend, \`@surf-ai/sdk/server\` dataApi in backend
- Use Tailwind CSS classes for styling (Surf Design System theme via \`@surf-ai/theme\`)
- Use shadcn/ui components — install with \`npx shadcn@latest add button\`
- Use \`cn()\` from \`@surf-ai/sdk/react\` to merge Tailwind classes
- Frontend packages are pre-installed — check \`package.json\` before installing
- Dark theme is the default (configured in entry-client.tsx)
`,
}

// ---------------------------------------------------------------------------
// Write files
// ---------------------------------------------------------------------------

const root = path.resolve(projectName)
const name = path.basename(root)

console.log(`\n  Creating Surf app in ./${projectName === '.' ? '' : name}\n`)

fs.mkdirSync(root, { recursive: true })

for (const [relPath, content] of Object.entries(templates)) {
  const fullPath = path.join(root, relPath)
  fs.mkdirSync(path.dirname(fullPath), { recursive: true })
  fs.writeFileSync(fullPath, content)
  console.log(`  ${relPath}`)
}

// Copy template files that are too complex for inline strings (backticks, ${}, etc.)
const templatesDir = path.join(new URL('.', import.meta.url).pathname, 'templates')
if (fs.existsSync(templatesDir)) {
  copyDir(templatesDir, root)
}

function copyDir(src: string, dest: string) {
  for (const entry of fs.readdirSync(src, { withFileTypes: true })) {
    const srcPath = path.join(src, entry.name)
    const destPath = path.join(dest, entry.name)
    if (entry.isDirectory()) {
      fs.mkdirSync(destPath, { recursive: true })
      copyDir(srcPath, destPath)
    } else {
      fs.writeFileSync(destPath, fs.readFileSync(srcPath))
      console.log(`  ${path.relative(root, destPath)}`)
    }
  }
}

const cdStep = projectName === '.' ? '' : `  cd ${name}\n`
console.log(`
Done! Next steps:

${cdStep}  cd backend && npm install && cd ..
  cd frontend && npm install && cd ..

  # Start backend
  cd backend && npm run dev &

  # Start frontend
  cd frontend && npm run dev

  Open http://localhost:${frontendPort}
`)
