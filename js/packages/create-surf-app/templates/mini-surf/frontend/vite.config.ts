import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

const env = loadEnv('', process.cwd(), '')
const FRONTEND_PORT = parseInt(env.VITE_PORT || '5173', 10)
const BACKEND_PORT = parseInt(env.VITE_BACKEND_PORT || '3001', 10)

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: FRONTEND_PORT,
    host: '0.0.0.0',
    proxy: {
      '/proxy': { target: `http://127.0.0.1:${BACKEND_PORT}`, changeOrigin: true },
      '/api': { target: `http://127.0.0.1:${BACKEND_PORT}`, changeOrigin: true },
    },
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
})
