import path from 'path'
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Use loadEnv to read .env before config evaluation.
// Vite does NOT load .env files until after the config is resolved,
// so process.env.VITE_* is unavailable here without this.
const env = loadEnv('development', process.cwd())

function readRequiredPort(name: 'VITE_BACKEND_PORT' | 'VITE_PORT') {
  const value = env[name]
  const port = Number.parseInt(value || '', 10)
  if (!Number.isInteger(port)) {
    throw new Error(`Missing required ${name} in frontend/.env`)
  }
  return port
}

const BACKEND_PORT = readRequiredPort('VITE_BACKEND_PORT')
const BASE = env.VITE_BASE || './'

// Vite proxy middleware runs BEFORE base-path stripping. When base is
// non-relative (e.g. /preview/staging/{sid}/), proxy keys must be prefixed
// with base to match incoming requests.
// See: https://vite.dev/config/server-options#server-proxy
const hasAbsBase = BASE.startsWith('/')
const proxyKey = hasAbsBase ? `${BASE}proxy` : '/proxy'
const apiProxyKey = hasAbsBase ? `${BASE}api` : '/api'

const backendProxy = {
  target: `http://127.0.0.1:${BACKEND_PORT}`,
  changeOrigin: true,
  // Strip base prefix so Express receives clean paths
  ...(hasAbsBase && {
    rewrite: (path: string) => path.replace(BASE, '/'),
  }),
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: readRequiredPort('VITE_PORT'),
    host: '0.0.0.0',
    proxy: {
      // /proxy/* → Express → OutboundProxy (crypto data APIs)
      [proxyKey]: backendProxy,
      // /api/* → Express (custom backend routes: auth, CRUD, etc.)
      [apiProxyKey]: backendProxy,
    },
    hmr: {
      // Relative path so Vite prepends the base (VITE_BASE) to construct
      // the full WebSocket URL: wss://host/{base}/ws/vite-hmr
      // This ensures HMR goes through the same preview proxy chain
      // (frontend rewrite → Muninn → Urania → Vite dev server).
      // An absolute path (starting with /) would bypass the proxy entirely.
      path: 'ws/vite-hmr',
    },
    // Pre-transform entry files at startup so deps are optimized before the
    // first browser request.  Reduces the window where Vite serves dep stubs
    // (partially-optimized modules) during cold start after session respawn.
    warmup: {
      clientFiles: ['./src/entry-client.tsx', './src/App.tsx'],
    },
  },
  // Force Vite to always resolve a single copy of React.
  // Without this, dep re-optimization after `npm install <new-pkg>` can create
  // a new React bundle (different hash) while dynamic imports still reference
  // the old one → two React instances → "Invalid hook call" errors.
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
    dedupe: ['react', 'react-dom'],
    // CRITICAL: node_modules is a tmpfs mount for performance.
    // With preserveSymlinks=false (default), esbuild and Vite may resolve
    // paths through the tmpfs layer differently after npm install modifies
    // the directory — old dep bundles reference the previous real path
    // → two React instances → "Cannot read properties of null
    // (reading 'useState')".
    // With preserveSymlinks=true, module identity uses the logical path
    // (frontend/node_modules/react) which stays constant regardless of
    // underlying mount changes → single React instance always.
    preserveSymlinks: true,
  },
  // Eagerly pre-bundle ALL scaffold deps at startup.
  // Without this, Vite discovers deps lazily by crawling imports. When the agent
  // rewrites code between turns, new imports trigger dep re-optimization which
  // serves "stub" modules (all exports undefined) → "Cannot read properties of
  // null (reading 'useEffect')" and "Loading dependencies..." reload loops.
  // Pre-bundling everything eliminates lazy discovery entirely.
  optimizeDeps: {
    include: [
      // React core
      'react',
      'react-dom',
      'react-dom/client',
      'react/jsx-dev-runtime',
      'react/jsx-runtime',
      // Data fetching
      '@tanstack/react-query',
      '@tanstack/query-core',
      // Charts
      'echarts',
      'echarts-for-react',
      'echarts/core',
      'echarts/charts',
      'echarts/components',
      'echarts/renderers',
      // UI primitives (Radix)
      '@radix-ui/react-accordion',
      '@radix-ui/react-aspect-ratio',
      '@radix-ui/react-avatar',
      '@radix-ui/react-checkbox',
      '@radix-ui/react-collapsible',
      '@radix-ui/react-context-menu',
      '@radix-ui/react-dialog',
      '@radix-ui/react-dropdown-menu',
      '@radix-ui/react-hover-card',
      '@radix-ui/react-label',
      '@radix-ui/react-menubar',
      '@radix-ui/react-navigation-menu',
      '@radix-ui/react-popover',
      '@radix-ui/react-progress',
      '@radix-ui/react-radio-group',
      '@radix-ui/react-scroll-area',
      '@radix-ui/react-select',
      '@radix-ui/react-separator',
      '@radix-ui/react-slider',
      '@radix-ui/react-slot',
      '@radix-ui/react-switch',
      '@radix-ui/react-tabs',
      '@radix-ui/react-toast',
      '@radix-ui/react-toggle',
      '@radix-ui/react-toggle-group',
      '@radix-ui/react-tooltip',
      // UI components
      'sonner',
      'cmdk',
      'vaul',
      'embla-carousel-react',
      'react-day-picker',
      'react-resizable-panels',
      // Forms & validation
      'react-hook-form',
      '@hookform/resolvers',
      'zod',
      // Utilities
      'lucide-react',
      'next-themes',
      'class-variance-authority',
      'clsx',
      'tailwind-merge',
      'date-fns',
      'scheduler',
    ],
  },
  // Base path set dynamically to match external preview URL.
  // This ensures Vite's internal modules (/@vite/client, /@react-refresh, etc.)
  // are served under the correct path through the multi-layer proxy chain.
  base: BASE,
})
