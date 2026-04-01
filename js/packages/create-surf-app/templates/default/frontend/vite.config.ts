import path from 'path'
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

function readRequiredPort(
  env: Record<string, string>,
  name: 'VITE_BACKEND_PORT' | 'VITE_PORT',
) {
  const value = env[name]
  const port = Number.parseInt(value || '', 10)
  if (!Number.isInteger(port)) {
    throw new Error(`Missing required ${name} in frontend/.env`)
  }
  return port
}

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd())
  const backendPort = readRequiredPort(env, 'VITE_BACKEND_PORT')
  const base = env.VITE_BASE || './'
  const hasAbsBase = base.startsWith('/')
  const apiBasePrefix = hasAbsBase ? base.replace(/\/$/, '') : ''

  const backendProxy = {
    target: `http://127.0.0.1:${backendPort}`,
    changeOrigin: true,
    ...(hasAbsBase && {
      rewrite: (requestPath: string) => requestPath.replace(base, '/'),
    }),
  }

  return {
    plugins: [react(), tailwindcss()],
    server: {
      port: readRequiredPort(env, 'VITE_PORT'),
      host: '0.0.0.0',
      proxy: {
        [`${apiBasePrefix}/api`]: backendProxy,
      },
      // Keep the HMR socket under the preview base path.
      hmr: {
        path: 'ws/vite-hmr',
      },
    },
    resolve: {
      alias: {
        '@': path.resolve(__dirname, './src'),
      },
      dedupe: ['react', 'react-dom'],
      preserveSymlinks: true,
    },
    // Pre-bundle the deps touched during the initial boot path so cold starts
    // do not race Vite's lazy dependency optimizer.
    optimizeDeps: {
      include: [
        'react',
        'react-dom',
        'react-dom/client',
        'react/jsx-dev-runtime',
        'react/jsx-runtime',
        '@tanstack/react-query',
        '@tanstack/query-core',
      ],
    },
    base,
  }
})
