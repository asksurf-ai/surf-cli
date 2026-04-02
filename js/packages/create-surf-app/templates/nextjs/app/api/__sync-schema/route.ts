import path from 'node:path'
import { syncSchema } from '@surf-ai/sdk/db'

export async function POST() {
  try {
    await syncSchema({
      schemaPath: path.join(process.cwd(), 'db', 'schema.ts'),
      retries: 2,
      retryDelay: 1500,
    })
    return Response.json({ ok: true })
  } catch (err) {
    return Response.json(
      { ok: false, error: String(err) },
      { status: 500 }
    )
  }
}
