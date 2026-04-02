import { dataApi } from '@surf-ai/sdk/server'

export async function GET(request: Request) {
  const { searchParams } = new URL(request.url)
  const symbol = searchParams.get('symbol') || 'BTC'

  const data = await dataApi.market.price({ symbol })
  return Response.json(data)
}
