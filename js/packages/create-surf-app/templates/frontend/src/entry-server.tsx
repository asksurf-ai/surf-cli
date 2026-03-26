import { renderToString } from 'react-dom/server'

export function render() {
  return renderToString(
    <div
      data-surf-placeholder
      style={{
        minHeight: '100vh',
        background: '#ffffff',
      }}
    />
  )
}
