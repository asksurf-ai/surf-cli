#!/usr/bin/env node

import { createSurfApp } from './index'

const VALUE_FLAGS = new Set([
  '--template',
  '--frontend-port',
  '--backend-port',
  '--preview-base',
])

function getFlag(args: string[], name: string) {
  const idx = args.indexOf(name)
  return idx >= 0 && args[idx + 1] ? args[idx + 1] : undefined
}

function parseCliArgs(args: string[]) {
  const positionalArgs: string[] = []

  for (let index = 0; index < args.length; index += 1) {
    const arg = args[index]
    if (!arg.startsWith('--')) {
      positionalArgs.push(arg)
      continue
    }

    if (!VALUE_FLAGS.has(arg)) {
      throw new Error(`Unknown flag: ${arg}`)
    }
    if (!args[index + 1] || args[index + 1].startsWith('--')) {
      throw new Error(`Missing value for flag: ${arg}`)
    }
    index += 1
  }

  if (positionalArgs.length > 1) {
    throw new Error(`Expected at most one project directory, got: ${positionalArgs.join(', ')}`)
  }

  return {
    projectName: positionalArgs[0] || '.',
    templateName: getFlag(args, '--template'),
    frontendPort: getFlag(args, '--frontend-port'),
    backendPort: getFlag(args, '--backend-port'),
    previewBase: getFlag(args, '--preview-base'),
  }
}

async function runCli() {
  const args = process.argv.slice(2)
  await createSurfApp(parseCliArgs(args))
}

runCli().catch(error => {
  console.error(error)
  process.exitCode = 1
})
