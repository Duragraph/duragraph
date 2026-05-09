import { useEditorStore } from '@/stores/editor'

export function EditorToolbar() {
  const { graphName, graphDescription, setGraphMeta, toDefinition, clear, isDirty, nodes, edges } =
    useEditorStore()

  function handleExport() {
    const def = toDefinition()
    const json = JSON.stringify(def, null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${def.id}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  function handleImport() {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.json'
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0]
      if (!file) return
      const text = await file.text()
      try {
        const def = JSON.parse(text)
        useEditorStore.getState().loadGraph(def)
      } catch {
        alert('Invalid graph definition file')
      }
    }
    input.click()
  }

  return (
    <div className="flex items-center gap-3 border-b border-border bg-card px-4 py-2">
      <input
        type="text"
        value={graphName}
        onChange={(e) => setGraphMeta(e.target.value, graphDescription)}
        className="border border-input bg-background px-2 py-1 text-sm font-semibold focus:outline-none focus:ring-2 focus:ring-ring w-48"
      />

      <input
        type="text"
        value={graphDescription}
        onChange={(e) => setGraphMeta(graphName, e.target.value)}
        placeholder="Description..."
        className="border border-input bg-background px-2 py-1 text-sm text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring w-64"
      />

      <div className="flex-1" />

      <span className="text-xs text-muted-foreground font-mono">
        {nodes.length} nodes, {edges.length} edges
        {isDirty && ' (unsaved)'}
      </span>

      <button
        onClick={handleImport}
        className="border border-input bg-background px-3 py-1 text-xs hover:bg-accent"
      >
        Import
      </button>

      <button
        onClick={handleExport}
        disabled={nodes.length === 0}
        className="border border-input bg-background px-3 py-1 text-xs hover:bg-accent disabled:opacity-50"
      >
        Export JSON
      </button>

      <button
        onClick={clear}
        disabled={nodes.length === 0}
        className="border border-destructive text-destructive px-3 py-1 text-xs hover:bg-destructive hover:text-destructive-foreground disabled:opacity-50"
      >
        Clear
      </button>
    </div>
  )
}
