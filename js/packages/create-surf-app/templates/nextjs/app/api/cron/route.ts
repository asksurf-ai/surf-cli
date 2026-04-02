import fs from 'node:fs'
import path from 'node:path'
import { randomUUID } from 'node:crypto'

const CRON_PATH = path.join(process.cwd(), 'cron.json')

function readCronConfig() {
  if (!fs.existsSync(CRON_PATH)) return []
  return JSON.parse(fs.readFileSync(CRON_PATH, 'utf-8'))
}

function writeCronConfig(config: unknown[]) {
  fs.writeFileSync(CRON_PATH, JSON.stringify(config, null, 2))
}

export async function GET() {
  return Response.json(readCronConfig())
}

export async function POST(request: Request) {
  const body = await request.json()
  const config = readCronConfig()
  const task = {
    id: randomUUID(),
    ...body,
    enabled: body.enabled ?? true,
  }
  config.push(task)
  writeCronConfig(config)
  return Response.json(task, { status: 201 })
}
