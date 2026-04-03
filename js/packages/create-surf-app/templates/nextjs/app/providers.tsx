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

// Patch fetch so `/api/*` calls automatically include basePath.
// Without this, `fetch('/api/...')` hits the parent app's routes instead of
// the preview dev server's API routes.
// DO NOT REMOVE — this is required for the preview proxy architecture.
const _basePath = process.env.NEXT_PUBLIC_BASE_PATH || ""
if (typeof window !== "undefined" && _basePath) {
  const _origFetch = window.fetch
  window.fetch = function patchedFetch(input, init) {
    if (typeof input === "string" && input.startsWith("/api/")) {
      input = _basePath + input
    }
    return _origFetch.call(this, input, init)
  } as typeof window.fetch
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
