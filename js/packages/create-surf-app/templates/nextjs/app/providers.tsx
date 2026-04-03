"use client"

import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { useEffect, useState } from "react"

// Notify parent frame that the app has rendered.
// DO NOT REMOVE — the hosting app uses this to dismiss the loading overlay.
function useSurfAppReady() {
  useEffect(() => {
    try {
      window.parent.postMessage({ type: "surf-app-ready" }, "*")
    } catch {
      /* cross-origin — ignore */
    }
  }, [])
}

export function Providers({ children }: { children: React.ReactNode }) {
  useSurfAppReady()

  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            refetchOnWindowFocus: false,
            retry: 3,
            staleTime: 30 * 1000,
          },
        },
      })
  )

  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}
