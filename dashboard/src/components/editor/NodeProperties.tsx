import { useEditorStore } from '@/stores/editor'

export function NodeProperties() {
  const { nodes, edges, selectedNodeId, selectedEdgeId, updateNode, removeNode, removeEdge, setEntrypoint } =
    useEditorStore()

  const selectedNode = selectedNodeId ? nodes.find((n) => n.id === selectedNodeId) : null
  const selectedEdge = selectedEdgeId ? edges.find((e) => e.id === selectedEdgeId) : null

  if (selectedNode) {
    return (
      <div className="w-64 border-l border-border bg-card flex flex-col">
        <div className="border-b border-border p-4">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Node Properties
          </h2>
        </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">Label</label>
            <input
              type="text"
              value={selectedNode.label}
              onChange={(e) => updateNode(selectedNode.id, { label: e.target.value })}
              className="w-full border border-input bg-background px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
            />
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">Type</label>
            <div className="text-sm font-mono bg-muted px-2 py-1.5">{selectedNode.type}</div>
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">ID</label>
            <div className="text-xs font-mono text-muted-foreground">{selectedNode.id}</div>
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">Position</label>
            <div className="text-xs font-mono text-muted-foreground">
              x: {Math.round(selectedNode.x)}, y: {Math.round(selectedNode.y)}
            </div>
          </div>

          {selectedNode.type === 'llm' && (
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">Model</label>
              <input
                type="text"
                value={(selectedNode.config.model as string) ?? ''}
                onChange={(e) =>
                  updateNode(selectedNode.id, {
                    config: { ...selectedNode.config, model: e.target.value },
                  })
                }
                placeholder="gpt-4o-mini"
                className="w-full border border-input bg-background px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
              />
            </div>
          )}

          {selectedNode.type === 'llm' && (
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                System Prompt
              </label>
              <textarea
                value={(selectedNode.config.system_prompt as string) ?? ''}
                onChange={(e) =>
                  updateNode(selectedNode.id, {
                    config: { ...selectedNode.config, system_prompt: e.target.value },
                  })
                }
                placeholder="You are a helpful assistant."
                rows={3}
                className="w-full border border-input bg-background px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring resize-none font-mono"
              />
            </div>
          )}

          {selectedNode.type === 'tool' && (
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                Tool Name
              </label>
              <input
                type="text"
                value={(selectedNode.config.tool_name as string) ?? ''}
                onChange={(e) =>
                  updateNode(selectedNode.id, {
                    config: { ...selectedNode.config, tool_name: e.target.value },
                  })
                }
                placeholder="web_search"
                className="w-full border border-input bg-background px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
              />
            </div>
          )}

          {selectedNode.type === 'human' && (
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                Prompt
              </label>
              <input
                type="text"
                value={(selectedNode.config.prompt as string) ?? ''}
                onChange={(e) =>
                  updateNode(selectedNode.id, {
                    config: { ...selectedNode.config, prompt: e.target.value },
                  })
                }
                placeholder="Please review this response"
                className="w-full border border-input bg-background px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring font-mono"
              />
            </div>
          )}

          <div className="pt-2 space-y-2">
            {!selectedNode.isEntrypoint && (
              <button
                onClick={() => setEntrypoint(selectedNode.id)}
                className="w-full border border-input bg-background px-3 py-1.5 text-xs hover:bg-accent"
              >
                Set as Entrypoint
              </button>
            )}
            <button
              onClick={() => removeNode(selectedNode.id)}
              className="w-full border border-destructive text-destructive px-3 py-1.5 text-xs hover:bg-destructive hover:text-destructive-foreground"
            >
              Delete Node
            </button>
          </div>
        </div>
      </div>
    )
  }

  if (selectedEdge) {
    const src = nodes.find((n) => n.id === selectedEdge.source)
    const tgt = nodes.find((n) => n.id === selectedEdge.target)
    return (
      <div className="w-64 border-l border-border bg-card flex flex-col">
        <div className="border-b border-border p-4">
          <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
            Edge Properties
          </h2>
        </div>

        <div className="flex-1 overflow-y-auto p-4 space-y-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">From</label>
            <div className="text-sm font-mono bg-muted px-2 py-1.5">
              {src?.label ?? selectedEdge.source}
            </div>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">To</label>
            <div className="text-sm font-mono bg-muted px-2 py-1.5">
              {tgt?.label ?? selectedEdge.target}
            </div>
          </div>
          <div className="pt-2">
            <button
              onClick={() => removeEdge(selectedEdge.id)}
              className="w-full border border-destructive text-destructive px-3 py-1.5 text-xs hover:bg-destructive hover:text-destructive-foreground"
            >
              Delete Edge
            </button>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div className="w-64 border-l border-border bg-card flex flex-col">
      <div className="border-b border-border p-4">
        <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
          Properties
        </h2>
      </div>
      <div className="flex-1 flex items-center justify-center p-4">
        <p className="text-xs text-muted-foreground text-center">
          Select a node or edge to view properties
        </p>
      </div>
    </div>
  )
}
