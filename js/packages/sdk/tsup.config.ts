import { defineConfig } from 'tsup'

export default defineConfig([
  // Server: CJS + ESM (backend routes use require(), but ESM also supported)
  {
    entry: { 'server/index': 'src/server/index.ts' },
    format: ['cjs', 'esm'],
    dts: true,
    clean: true,
    outDir: 'dist',
  },
  // React: ESM only (frontend is always ESM)
  {
    entry: { 'react/index': 'src/react/index.ts' },
    format: ['esm'],
    dts: true,
    outDir: 'dist',
    external: ['react', 'react-dom', '@tanstack/react-query', 'clsx', 'tailwind-merge'],
  },
  // DB: CJS + ESM
  {
    entry: { 'db/index': 'src/db/index.ts' },
    format: ['cjs', 'esm'],
    dts: true,
    outDir: 'dist',
  },
])
