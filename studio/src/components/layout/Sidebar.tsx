import { useChatStore } from '@/stores/chat'
import { useAssistants } from '@/api/assistants'
import { useThreads, useCreateThread } from '@/api/threads'

export function Sidebar() {
  const { selectedThreadId, selectedAssistantId, setThread, setAssistant, setMessages } =
    useChatStore()
  const { data: assistants } = useAssistants()
  const { data: threads } = useThreads()
  const createThread = useCreateThread()

  async function handleNewThread() {
    try {
      const thread = await createThread.mutateAsync({})
      setThread(thread.thread_id)
      setMessages([])
    } catch {
      // silently fail
    }
  }

  return (
    <aside className="flex w-64 flex-col border-r border-border bg-card">
      <div className="border-b border-border p-4">
        <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
          Settings
        </h2>
      </div>

      <div className="flex-1 overflow-y-auto p-4 space-y-5">
        {/* Assistant selector */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
            Assistant
          </label>
          <select
            value={selectedAssistantId ?? ''}
            onChange={(e) => setAssistant(e.target.value || null)}
            className="w-full border border-input bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
          >
            <option value="">Select assistant...</option>
            {assistants?.map((a) => (
              <option key={a.assistant_id} value={a.assistant_id}>
                {a.name || a.graph_id}
              </option>
            ))}
          </select>
        </div>

        {/* Thread selector */}
        <div>
          <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
            Thread
          </label>
          <select
            value={selectedThreadId ?? ''}
            onChange={(e) => {
              setThread(e.target.value || null)
              setMessages([])
            }}
            className="w-full border border-input bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
          >
            <option value="">New thread</option>
            {threads?.map((t) => (
              <option key={t.thread_id} value={t.thread_id}>
                {t.thread_id.slice(0, 8)}...
                {t.metadata?.name ? ` (${t.metadata.name})` : ''}
              </option>
            ))}
          </select>

          <button
            onClick={handleNewThread}
            disabled={createThread.isPending}
            className="mt-2 w-full border border-input bg-background px-3 py-1.5 text-xs hover:bg-accent disabled:opacity-50"
          >
            {createThread.isPending ? 'Creating...' : '+ New Thread'}
          </button>
        </div>

        {/* Thread list */}
        {threads && threads.length > 0 && (
          <div>
            <label className="mb-1.5 block text-xs font-medium text-muted-foreground">
              Recent Threads
            </label>
            <div className="space-y-1">
              {threads.slice(0, 10).map((t) => (
                <button
                  key={t.thread_id}
                  onClick={() => {
                    setThread(t.thread_id)
                    setMessages([])
                  }}
                  className={`w-full px-2 py-1.5 text-left text-xs hover:bg-accent transition-colors ${
                    selectedThreadId === t.thread_id ? 'bg-accent font-medium' : ''
                  }`}
                >
                  <span className="font-mono">{t.thread_id.slice(0, 12)}</span>
                  <span className="ml-1 text-muted-foreground">
                    {new Date(t.created_at).toLocaleDateString()}
                  </span>
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </aside>
  )
}
