// Example agent-written frontend using SDK React hooks.
import { useMarketPrice, useMarketRanking } from '@surf-ai/sdk/react'

export default function App() {
  const { data: priceData, isLoading: priceLoading, error: priceError } = useMarketPrice({ symbol: 'BTC', time_range: '1d' })
  const { data: rankData, isLoading: rankLoading } = useMarketRanking({ limit: '10', metric: 'market_cap' })

  return (
    <div style={{ padding: 20, fontFamily: 'monospace' }}>
      <h1>BTC Price (via @surf-ai/sdk)</h1>

      {priceLoading && <p>Loading price...</p>}
      {priceError && <p style={{ color: 'red' }}>Error: {priceError.message}</p>}
      {priceData?.data?.[0] && (
        <div>
          <p>Price: ${priceData.data[0].value?.toLocaleString()}</p>
          <p>Points: {priceData.data.length}</p>
        </div>
      )}

      <h2>Market Ranking</h2>
      {rankLoading && <p>Loading ranking...</p>}
      {rankData?.data && (
        <table>
          <thead>
            <tr><th>#</th><th>Symbol</th></tr>
          </thead>
          <tbody>
            {rankData.data.slice(0, 5).map((item: any, i: number) => (
              <tr key={i}><td>{i + 1}</td><td>{item.symbol}</td></tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
