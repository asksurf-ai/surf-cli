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
  // DB: CJS + ESM
  {
    entry: { 'db/index': 'src/db/index.ts' },
    format: ['cjs', 'esm'],
    dts: true,
    outDir: 'dist',
  },
])
