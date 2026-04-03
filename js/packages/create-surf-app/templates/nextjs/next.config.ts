import type { NextConfig } from 'next'

const nextConfig: NextConfig = {
  output: 'standalone',
  basePath: process.env.BASE_PATH!.replace(/\/+$/, ''),
  env: {
    NEXT_PUBLIC_BASE_PATH: process.env.BASE_PATH!.replace(/\/+$/, ''),
  },
  serverExternalPackages: ['@surf-ai/sdk', 'drizzle-orm', 'drizzle-kit', 'croner'],
}

export default nextConfig
