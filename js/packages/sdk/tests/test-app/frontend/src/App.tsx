// Example agent-written frontend using SDK React utilities.
import { cn, toast } from '@surf-ai/sdk/react'

export default function App() {
  return (
    <div className={cn('p-6', 'font-mono')}>
      <h1>SDK React utilities</h1>
      <button onClick={() => toast({ title: 'Saved from test app' })}>
        Trigger toast
      </button>
    </div>
  )
}
