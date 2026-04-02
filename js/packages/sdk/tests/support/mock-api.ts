export interface MockApiRequest {
  method: string
  pathname: string
  search: string
  headers: Record<string, string>
  bodyText: string
  bodyJson?: any
}

export interface MockApiServer {
  baseUrl: string
  requests: MockApiRequest[]
  clear(): void
  stop(): void
}

function json(data: any, status = 200): Response {
  return Response.json(data, { status })
}

export function startMockApiServer(): MockApiServer {
  const requests: MockApiRequest[] = []

  const server = Bun.serve({
    port: 0,
    async fetch(req) {
      const url = new URL(req.url)
      const bodyText = await req.text()
      let bodyJson: any

      if (bodyText) {
        try {
          bodyJson = JSON.parse(bodyText)
        } catch {
          bodyJson = undefined
        }
      }

      requests.push({
        method: req.method,
        pathname: url.pathname,
        search: url.search,
        headers: Object.fromEntries(req.headers.entries()),
        bodyText,
        bodyJson,
      })

      const path = url.pathname.replace(/^\/gateway\/v1\/?/, '')

      if (path === 'market/price') {
        return json({
          data: [
            {
              symbol: url.searchParams.get('symbol') || 'BTC',
              value: 100000,
            },
          ],
        })
      }

      if (path === 'market/ranking') {
        const limit = Number(url.searchParams.get('limit') || '3')
        return json({
          data: Array.from({ length: limit }, (_, index) => ({
            symbol: `COIN${index + 1}`,
            value: 1000 - index,
          })),
        })
      }

      if (path === 'token/holders') {
        return json({
          data: [
            {
              address: url.searchParams.get('address') || '0x0',
              holder: '0xholder',
            },
          ],
        })
      }

      if (path === 'db/provision') {
        return json({
          host: 'localhost',
          database: 'test_db',
          user: 'tester',
          password: 'secret',
        })
      }

      if (path === 'db/query') {
        return json({
          rows: [],
        })
      }

      if (path === 'db/tables') {
        return json([
          { name: 'users' },
        ])
      }

      if (path === 'db/table-schema') {
        return json({
          columns: [
            { name: 'id' },
            { name: 'created_at' },
          ],
        })
      }

      if (path === 'db/status') {
        return json({
          connected: true,
          database: 'test_db',
        })
      }

      return json({ error: `Unhandled mock path: ${url.pathname}` }, 404)
    },
  })

  return {
    baseUrl: `http://127.0.0.1:${server.port}/gateway/v1`,
    requests,
    clear() {
      requests.length = 0
    },
    stop() {
      server.stop(true)
    },
  }
}
