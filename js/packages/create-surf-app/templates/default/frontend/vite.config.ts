import path from 'path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig(() => {
  const frontendPort = Number.parseInt(process.env.PORT || '', 10)
  const backendPort = Number.parseInt(process.env.BACKEND_PORT || '', 10)
  const base = process.env.BASE_PATH || './'
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
      host: '0.0.0.0',
      port: frontendPort || undefined,
      proxy: {
        [`${apiBasePrefix}/api`]: backendProxy,
      },
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
