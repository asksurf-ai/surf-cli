import { useMarketPrice } from '@surf-ai/sdk/react'

export default function App() {
  const { data, isLoading, error } = useMarketPrice({ symbol: 'BTC', time_range: '1d' })

  return (
    <div className="min-h-screen bg-background text-foreground p-10">
      <h1 className="text-3xl font-bold mb-4">Surf App</h1>
      {isLoading && <p className="text-muted-foreground">Loading BTC price...</p>}
      {error && <p className="text-destructive">Error: {(error as Error).message}</p>}
      {data?.data?.[0] && (
        <p className="text-4xl font-bold">
          BTC: <span className="text-primary">${data.data[0].value?.toLocaleString()}</span>
        </p>
      )}
    </div>
  )
}
