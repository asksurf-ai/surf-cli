import './index.css'

// ---------------------------------------------------------------------------
// Cold-start guard: verify React is real BEFORE rendering.
//
// During Vite dep-optimization (cold start or after `npm install <pkg>`),
// Vite may serve placeholder "stub" modules where all React exports are
// undefined.  Rendering with stub hooks causes "Cannot read properties of
// null (reading 'useState')".
//
// Strategy:
//   1. Dynamic-import React and check if useState is a function.
//   2. If stub  → show a loading banner, schedule a reload with increasing
//      delay, and do NOT mount any React tree (avoids the error entirely).
//   3. If real  → dynamic-import App/ErrorBoundary and render normally.
//   4. Use createElement (not JSX) in this file so Vite doesn't inject a
//      static `import { jsx } from 'react/jsx-dev-runtime'` which would
//      itself be a stub and crash before our guard runs.
// ---------------------------------------------------------------------------

const RELOAD_KEY = '__dep_reload'
const MAX_RELOADS = 6
const RELOAD_WINDOW = 60_000 // 1 minute

function getReloads(): { c: number; t: number } {
  try {
    return JSON.parse(sessionStorage.getItem(RELOAD_KEY) || '{}') as { c: number; t: number }
  } catch { return { c: 0, t: 0 } }
}

async function boot() {
  const root = document.getElementById('root')!

  try {
    const React = await import('react')

    // If useState is not a function, React is a dep-optimization stub
    if (typeof React.useState !== 'function') throw new Error('dep-stub')

    const { createElement } = React
    const { createRoot, hydrateRoot } = await import('react-dom/client')
    const { QueryClient, QueryClientProvider } = await import('@tanstack/react-query')
    const { default: App } = await import('./App')

    const queryClient = new QueryClient({
      defaultOptions: { queries: {
        refetchOnWindowFocus: false,
        retry: 3,
        retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 10000),
        staleTime: 30_000,
      } },
    })

    const children = createElement(QueryClientProvider, { client: queryClient },
      createElement(App)
    )

    // Use hydrateRoot only when SSR rendered real app content. The scaffold's
    // server entry intentionally returns a lightweight placeholder shell so
    // SSR-incompatible libraries (for example echarts-for-react) cannot crash
    // deploy-time render. Placeholder markup must be client-rendered, not hydrated.
    const hasPlaceholder = !!root.querySelector('[data-surf-placeholder]')
    if (root.childNodes.length > 0 && root.innerHTML !== '<!--ssr-outlet-->' && !hasPlaceholder) {
      hydrateRoot(root, children)
    } else {
      root.innerHTML = ''
      createRoot(root).render(children)
    }

    // React rendered successfully — signal to index.html fallback & reset counter
    ;(window as any).__reactOk = true
    sessionStorage.removeItem(RELOAD_KEY)

    // Notify parent frame when real app content renders (not the placeholder).
    // DO NOT REMOVE — the hosting app uses this to dismiss the loading overlay.
    function notifyParentReady() {
      if (!document.querySelector('[data-surf-placeholder]')) {
        try { window.parent.postMessage({ type: 'surf-app-ready' }, '*') } catch { /* cross-origin — ignore */ }
      }
    }
    notifyParentReady()
    new MutationObserver(notifyParentReady).observe(root, { childList: true, subtree: true })
  } catch {
    // React is not ready — show loading banner and schedule reload
    const prev = getReloads()
    const count = (Date.now() - prev.t > RELOAD_WINDOW) ? 0 : prev.c

    if (count < MAX_RELOADS) {
      root.innerHTML = [
        '<div style=\"padding:24px;text-align:center;font-family:system-ui,sans-serif\">',
        '<p style=\"color:#3b82f6;font-weight:600;margin:0 0 4px\">Loading dependencies...</p>',
        '<p style=\"color:#3b82f6;opacity:0.7;font-size:12px;margin:0\">Reloading automatically</p>',
        '</div>',
      ].join('')
      sessionStorage.setItem(RELOAD_KEY, JSON.stringify({ c: count + 1, t: Date.now() }))
      // Increasing delay: 3s, 4s, 5s, ... gives Vite more time to finish
      setTimeout(() => location.reload(), 3000 + count * 1000)
    } else {
      root.innerHTML = [
        '<div style=\"padding:24px;text-align:center;font-family:system-ui,sans-serif\">',
        '<p style=\"color:#c0392b;font-weight:600;margin:0 0 4px\">Failed to load dependencies</p>',
        '<p style=\"color:#c0392b;opacity:0.8;font-size:12px;margin:0\">Please refresh the page</p>',
        '</div>',
      ].join('')
    }
  }
}

boot()
