import { Flag, Trash2 } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Textarea } from "@/components/ui/textarea"
import { useEditorStore } from "@/stores/editor"

// Right rail of the workflow editor. Three panes share the chrome
// (a w-64 card column) and switch contents based on selection:
//   - `selectedNodeId` → node form (label, type-specific config,
//     entrypoint toggle, delete)
//   - `selectedEdgeId` → edge summary (from/to + delete)
//   - neither           → empty-state hint
//
// All inputs are controlled and write straight back into the editor
// store; no local state needed.

export function NodeProperties() {
  const {
    nodes,
    edges,
    selectedNodeId,
    selectedEdgeId,
    updateNode,
    removeNode,
    removeEdge,
    setEntrypoint,
  } = useEditorStore()

  const selectedNode = selectedNodeId
    ? nodes.find((n) => n.id === selectedNodeId)
    : null
  const selectedEdge = selectedEdgeId
    ? edges.find((e) => e.id === selectedEdgeId)
    : null

  if (selectedNode) {
    return (
      <PaneShell title="Node properties">
        <div className="grid gap-4 px-4 py-4">
          <FieldRow label="Label">
            <Input
              value={selectedNode.label}
              onChange={(e) =>
                updateNode(selectedNode.id, { label: e.target.value })
              }
              className="h-8 font-mono text-sm"
            />
          </FieldRow>

          <FieldRow label="Type">
            <Badge variant="outline" className="font-mono uppercase">
              {selectedNode.type}
            </Badge>
          </FieldRow>

          <FieldRow label="ID">
            <code className="text-xs text-muted-foreground">
              {selectedNode.id}
            </code>
          </FieldRow>

          <FieldRow label="Position">
            <code className="text-xs text-muted-foreground">
              x: {Math.round(selectedNode.x)} · y: {Math.round(selectedNode.y)}
            </code>
          </FieldRow>

          {selectedNode.type === "llm" && (
            <>
              <Separator />
              <FieldRow label="Model">
                <Input
                  value={(selectedNode.config.model as string) ?? ""}
                  onChange={(e) =>
                    updateNode(selectedNode.id, {
                      config: {
                        ...selectedNode.config,
                        model: e.target.value,
                      },
                    })
                  }
                  placeholder="gpt-4o-mini"
                  className="h-8 font-mono text-sm"
                />
              </FieldRow>
              <FieldRow label="System prompt">
                <Textarea
                  value={(selectedNode.config.system_prompt as string) ?? ""}
                  onChange={(e) =>
                    updateNode(selectedNode.id, {
                      config: {
                        ...selectedNode.config,
                        system_prompt: e.target.value,
                      },
                    })
                  }
                  placeholder="You are a helpful assistant."
                  rows={3}
                  className="resize-none font-mono text-xs"
                />
              </FieldRow>
            </>
          )}

          {selectedNode.type === "tool" && (
            <>
              <Separator />
              <FieldRow label="Tool name">
                <Input
                  value={(selectedNode.config.tool_name as string) ?? ""}
                  onChange={(e) =>
                    updateNode(selectedNode.id, {
                      config: {
                        ...selectedNode.config,
                        tool_name: e.target.value,
                      },
                    })
                  }
                  placeholder="web_search"
                  className="h-8 font-mono text-sm"
                />
              </FieldRow>
            </>
          )}

          {selectedNode.type === "human" && (
            <>
              <Separator />
              <FieldRow label="Prompt">
                <Input
                  value={(selectedNode.config.prompt as string) ?? ""}
                  onChange={(e) =>
                    updateNode(selectedNode.id, {
                      config: {
                        ...selectedNode.config,
                        prompt: e.target.value,
                      },
                    })
                  }
                  placeholder="Please review this response"
                  className="h-8 text-sm"
                />
              </FieldRow>
            </>
          )}

          <Separator />

          <div className="grid gap-2">
            {!selectedNode.isEntrypoint && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => setEntrypoint(selectedNode.id)}
              >
                <Flag className="size-4" />
                Set as entrypoint
              </Button>
            )}
            <Button
              variant="destructive"
              size="sm"
              onClick={() => removeNode(selectedNode.id)}
            >
              <Trash2 className="size-4" />
              Delete node
            </Button>
          </div>
        </div>
      </PaneShell>
    )
  }

  if (selectedEdge) {
    const src = nodes.find((n) => n.id === selectedEdge.source)
    const tgt = nodes.find((n) => n.id === selectedEdge.target)
    return (
      <PaneShell title="Edge properties">
        <div className="grid gap-4 px-4 py-4">
          <FieldRow label="From">
            <code className="rounded bg-muted px-2 py-1 text-xs">
              {src?.label ?? selectedEdge.source}
            </code>
          </FieldRow>
          <FieldRow label="To">
            <code className="rounded bg-muted px-2 py-1 text-xs">
              {tgt?.label ?? selectedEdge.target}
            </code>
          </FieldRow>
          <Separator />
          <Button
            variant="destructive"
            size="sm"
            onClick={() => removeEdge(selectedEdge.id)}
          >
            <Trash2 className="size-4" />
            Delete edge
          </Button>
        </div>
      </PaneShell>
    )
  }

  return (
    <PaneShell title="Properties">
      <p className="px-4 py-8 text-center text-xs text-muted-foreground">
        Select a node or edge to view properties
      </p>
    </PaneShell>
  )
}

function PaneShell({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <aside className="flex w-64 shrink-0 flex-col border-l bg-card">
      <div className="border-b p-4">
        <h2 className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
          {title}
        </h2>
      </div>
      <ScrollArea className="flex-1">{children}</ScrollArea>
    </aside>
  )
}

function FieldRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="grid gap-1.5">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
    </div>
  )
}
