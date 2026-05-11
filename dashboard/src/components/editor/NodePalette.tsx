import type { EditorNodeType } from '@/types/entities'

const NODE_TYPES: { type: EditorNodeType; label: string; description: string; color: string }[] = [
  { type: 'llm', label: 'LLM', description: 'Language model call', color: 'border-purple-400 bg-purple-50 text-purple-700' },
  { type: 'function', label: 'Function', description: 'Custom logic', color: 'border-blue-400 bg-blue-50 text-blue-700' },
  { type: 'tool', label: 'Tool', description: 'External tool call', color: 'border-green-400 bg-green-50 text-green-700' },
  { type: 'router', label: 'Router', description: 'Conditional branch', color: 'border-orange-400 bg-orange-50 text-orange-700' },
  { type: 'human', label: 'Human', description: 'Human-in-the-loop', color: 'border-pink-400 bg-pink-50 text-pink-700' },
  { type: 'subgraph', label: 'Subgraph', description: 'Nested graph', color: 'border-cyan-400 bg-cyan-50 text-cyan-700' },
]

export function NodePalette() {
  function handleDragStart(e: React.DragEvent, type: EditorNodeType) {
    e.dataTransfer.setData('node-type', type)
    e.dataTransfer.effectAllowed = 'copy'
  }

  return (
    <div className="w-56 border-r border-border bg-card flex flex-col">
      <div className="border-b border-border p-4">
        <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
          Node Types
        </h2>
      </div>

      <div className="flex-1 overflow-y-auto p-3 space-y-2">
        {NODE_TYPES.map(({ type, label, description, color }) => (
          <div
            key={type}
            draggable
            onDragStart={(e) => handleDragStart(e, type)}
            className={`border-2 p-2.5 cursor-grab active:cursor-grabbing hover:shadow-sm transition-shadow ${color}`}
          >
            <div className="text-xs font-semibold font-mono">{label}</div>
            <div className="text-[10px] opacity-70 mt-0.5">{description}</div>
          </div>
        ))}
      </div>
    </div>
  )
}
