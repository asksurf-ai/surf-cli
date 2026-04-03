import type { NextConfig } from 'next'

const nextConfig: NextConfig = {
  output: 'standalone',
  basePath: process.env.BASE_PATH!.replace(/\/+$/, ''),
  serverExternalPackages: ['@surf-ai/sdk', 'drizzle-orm', 'drizzle-kit', 'croner'],
  logging: {
    browserToTerminal: true,
  },
}

export default nextConfig
