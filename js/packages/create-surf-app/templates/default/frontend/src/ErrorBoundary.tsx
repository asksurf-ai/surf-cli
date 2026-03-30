import { Component, type ReactNode } from 'react'

interface Props { children: ReactNode; fallback?: ReactNode }
interface State { error: Error | null }

// Auto-reload config for transient dep-optimization errors.
// Shares the same key as entry-client's cold-start guard so reload
// attempts are counted together, preventing double reload loops.
const RELOAD_KEY = '__dep_reload'
const MAX_RELOADS = 6
// If the last reload was more than this many ms ago, reset the counter.
// This prevents stale counters from blocking legitimate retries on a
// later visit, while still capping rapid reload loops.
const RELOAD_WINDOW_MS = 60_000

// Patterns that indicate React modules loaded as stubs (dep optimization in progress)
const DEP_OPT_PATTERNS = [
  "reading 'useState'",
  "reading 'useEffect'",
  "reading 'useRef'",
  "reading 'useCallback'",
  "reading 'useMemo'",
  "reading 'useContext'",
  "reading 'useReducer'",
]

function isDepOptError(msg: string): boolean {
  return DEP_OPT_PATTERNS.some((p) => msg.includes(p))
}

// Shared format with entry-client: { c: count, t: timestamp }
function getReloadState(): { c: number; t: number } {
  try {
    const raw = sessionStorage.getItem(RELOAD_KEY)
    if (raw) return JSON.parse(raw)
  } catch { /* ignore parse errors */ }
  return { c: 0, t: 0 }
}

export default class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  componentDidCatch(error: Error) {
    if (!isDepOptError(error.message)) return
    // Vite dep optimization may serve React stubs during cold start.
    // Auto-reload so the browser fetches the real modules once ready.
    const prev = getReloadState()
    // Reset counter if outside the rapid-reload window (stale from earlier visit)
    const count = (Date.now() - prev.t > RELOAD_WINDOW_MS) ? 0 : prev.c
    if (count < MAX_RELOADS) {
      sessionStorage.setItem(RELOAD_KEY, JSON.stringify({ c: count + 1, t: Date.now() }))
      setTimeout(() => location.reload(), 3000)
    }
  }

  render() {
    if (this.state.error) {
      if (this.props.fallback) return this.props.fallback

      // Show a friendlier message for dep optimization errors that will auto-reload
      if (isDepOptError(this.state.error.message)) {
        const { c, t } = getReloadState()
        const fresh = (Date.now() - t <= RELOAD_WINDOW_MS)
        if (!fresh || c < MAX_RELOADS) {
          return (
            <div style={{
              padding: '24px',
              margin: '16px',
              borderRadius: '12px',
              background: 'rgba(59,130,246,0.06)',
              border: '1px solid rgba(59,130,246,0.15)',
              color: '#3b82f6',
              fontSize: '13px',
              fontFamily: 'system-ui, sans-serif',
              textAlign: 'center',
            }}>
              <p style={{ fontWeight: 600, marginBottom: '4px' }}>Loading dependencies...</p>
              <p style={{ opacity: 0.7, fontSize: '12px' }}>Reloading automatically</p>
            </div>
          )
        }
      }

      return (
        <div style={{
          padding: '24px',
          margin: '16px',
          borderRadius: '12px',
          background: 'rgba(245,34,45,0.06)',
          border: '1px solid rgba(245,34,45,0.15)',
          color: '#c0392b',
          fontSize: '13px',
          fontFamily: 'monospace',
        }}>
          <p style={{ fontWeight: 600, marginBottom: '8px' }}>Component Error</p>
          <p style={{ opacity: 0.8 }}>{this.state.error.message}</p>
        </div>
      )
    }
    return this.props.children
  }
}
