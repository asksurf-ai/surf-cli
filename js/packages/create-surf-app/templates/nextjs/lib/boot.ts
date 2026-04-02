import fs from 'node:fs'
import path from 'node:path'

export async function syncSchema() {
  const schemaPath = path.join(process.cwd(), 'db', 'schema.ts')
  if (!fs.existsSync(schemaPath)) return

  try {
    const { syncSchema: sync } = await import('@surf-ai/sdk/db')
    await sync({ schemaPath })
    console.log('[surf] Schema sync complete')
  } catch (err) {
    console.warn('[surf] Schema sync failed (will retry on next request):', err)
  }
}

export async function watchSchema() {
  const schemaPath = path.join(process.cwd(), 'db', 'schema.ts')
  if (!fs.existsSync(schemaPath)) return

  try {
    const { watchSchema: watch } = await import('@surf-ai/sdk/db')
    watch(schemaPath)
    console.log('[surf] Watching db/schema.ts for changes')
  } catch (err) {
    console.warn('[surf] Failed to set up schema watcher:', err)
  }
}

export async function startCron() {
  const cronPath = path.join(process.cwd(), 'cron.json')
  if (!fs.existsSync(cronPath)) return

  try {
    const { Cron } = await import('croner')
    const cronConfig = JSON.parse(fs.readFileSync(cronPath, 'utf-8'))

    for (const task of cronConfig) {
      if (!task.enabled) continue
      const handler = await import(/* webpackIgnore: true */ path.resolve(task.handler))
      const fn = handler.default || handler

      new Cron(task.schedule, { name: task.name }, async () => {
        try {
          await fn()
        } catch (err) {
          console.error(`[surf] Cron task "${task.name}" failed:`, err)
        }
      })

      console.log(`[surf] Cron task "${task.name}" scheduled: ${task.schedule}`)
    }
  } catch (err) {
    console.warn('[surf] Cron setup failed:', err)
  }
}
