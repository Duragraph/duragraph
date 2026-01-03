import { X, Trash2 } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { Separator } from "@/components/ui/separator"
import { ScrollArea } from "@/components/ui/scroll-area"
import { useWorkflowStore } from "@/stores/workflow"
import { cn } from "@/lib/utils"

export function NodeInspector() {
  const { nodes, selectedNodeId, setSelectedNode, updateNodeData, deleteNode } =
    useWorkflowStore()

  const selectedNode = nodes.find((n) => n.id === selectedNodeId)

  if (!selectedNode) {
    return (
      <div className="flex h-full w-80 flex-col border-l bg-background">
        <div className="flex h-14 items-center justify-between border-b px-4">
          <h3 className="font-semibold">Node Inspector</h3>
        </div>
        <div className="flex flex-1 items-center justify-center p-4 text-center">
          <p className="text-sm text-muted-foreground">
            Select a node to view and edit its properties
          </p>
        </div>
      </div>
    )
  }

  const handleLabelChange = (value: string) => {
    updateNodeData(selectedNode.id, { label: value })
  }

  const handleConfigChange = (key: string, value: string) => {
    updateNodeData(selectedNode.id, {
      config: {
        ...selectedNode.data.config,
        [key]: value,
      },
    })
  }

  const getConfigFields = () => {
    switch (selectedNode.data.type) {
      case "llm":
        return [
          {
            key: "model",
            label: "Model",
            placeholder: "gpt-4",
            type: "text",
          },
          {
            key: "temperature",
            label: "Temperature",
            placeholder: "0.7",
            type: "text",
          },
          {
            key: "systemPrompt",
            label: "System Prompt",
            placeholder: "You are a helpful assistant...",
            type: "textarea",
          },
        ]
      case "tool":
        return [
          {
            key: "tool",
            label: "Function Name",
            placeholder: "get_weather",
            type: "text",
          },
          {
            key: "description",
            label: "Description",
            placeholder: "Get current weather for a location",
            type: "textarea",
          },
        ]
      case "conditional":
        return [
          {
            key: "condition",
            label: "Condition",
            placeholder: "state.value > 10",
            type: "text",
          },
        ]
      case "human":
        return [
          {
            key: "prompt",
            label: "Prompt",
            placeholder: "Please review and approve...",
            type: "textarea",
          },
          {
            key: "interruptType",
            label: "Interrupt Type",
            placeholder: "approval | input",
            type: "text",
          },
        ]
      default:
        return []
    }
  }

  const configFields = getConfigFields()

  return (
    <div className="flex h-full w-80 flex-col border-l bg-background">
      <div className="flex h-14 items-center justify-between border-b px-4">
        <h3 className="font-semibold">Node Inspector</h3>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8"
          onClick={() => setSelectedNode(null)}
        >
          <X className="h-4 w-4" />
        </Button>
      </div>

      <ScrollArea className="flex-1">
        <div className="space-y-6 p-4">
          {/* Node Info */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Node Info
            </h4>

            <div className="space-y-2">
              <Label htmlFor="nodeId" className="text-xs">
                ID
              </Label>
              <Input
                id="nodeId"
                value={selectedNode.id}
                disabled
                className="font-mono text-xs"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="nodeType" className="text-xs">
                Type
              </Label>
              <Input
                id="nodeType"
                value={selectedNode.data.type}
                disabled
                className="text-xs capitalize"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="nodeLabel" className="text-xs">
                Label
              </Label>
              <Input
                id="nodeLabel"
                value={selectedNode.data.label}
                onChange={(e) => handleLabelChange(e.target.value)}
                placeholder="Node label"
                className="text-xs"
              />
            </div>
          </div>

          {configFields.length > 0 && (
            <>
              <Separator />

              {/* Configuration */}
              <div className="space-y-4">
                <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                  Configuration
                </h4>

                {configFields.map((field) => (
                  <div key={field.key} className="space-y-2">
                    <Label htmlFor={field.key} className="text-xs">
                      {field.label}
                    </Label>
                    {field.type === "textarea" ? (
                      <Textarea
                        id={field.key}
                        value={
                          (selectedNode.data.config?.[field.key] as string) || ""
                        }
                        onChange={(e) =>
                          handleConfigChange(field.key, e.target.value)
                        }
                        placeholder={field.placeholder}
                        className="text-xs min-h-[80px]"
                      />
                    ) : (
                      <Input
                        id={field.key}
                        value={
                          (selectedNode.data.config?.[field.key] as string) || ""
                        }
                        onChange={(e) =>
                          handleConfigChange(field.key, e.target.value)
                        }
                        placeholder={field.placeholder}
                        className="text-xs"
                      />
                    )}
                  </div>
                ))}
              </div>
            </>
          )}

          {/* Status (for run visualization) */}
          {selectedNode.data.status && (
            <>
              <Separator />

              <div className="space-y-4">
                <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
                  Execution Status
                </h4>

                <div className="space-y-2">
                  <Label className="text-xs">Status</Label>
                  <div
                    className={cn(
                      "inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium",
                      {
                        "bg-gray-100 text-gray-700":
                          selectedNode.data.status === "initial",
                        "bg-blue-100 text-blue-700":
                          selectedNode.data.status === "loading",
                        "bg-green-100 text-green-700":
                          selectedNode.data.status === "success",
                        "bg-red-100 text-red-700":
                          selectedNode.data.status === "error",
                      }
                    )}
                  >
                    {selectedNode.data.status}
                  </div>
                </div>

                {selectedNode.data.error && (
                  <div className="space-y-2">
                    <Label className="text-xs">Error</Label>
                    <div className="rounded-md bg-red-50 p-2 text-xs text-red-700 dark:bg-red-950 dark:text-red-300">
                      {String(selectedNode.data.error)}
                    </div>
                  </div>
                )}

                {Boolean(selectedNode.data.output) && (
                  <div className="space-y-2">
                    <Label className="text-xs">Output</Label>
                    <pre className="rounded-md bg-muted p-2 text-xs overflow-auto max-h-40">
                      {JSON.stringify(selectedNode.data.output as object, null, 2)}
                    </pre>
                  </div>
                )}
              </div>
            </>
          )}

          <Separator />

          {/* Actions */}
          <div className="space-y-4">
            <h4 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
              Actions
            </h4>

            <Button
              variant="destructive"
              size="sm"
              className="w-full"
              onClick={() => {
                deleteNode(selectedNode.id)
                setSelectedNode(null)
              }}
            >
              <Trash2 className="h-4 w-4 mr-2" />
              Delete Node
            </Button>
          </div>
        </div>
      </ScrollArea>
    </div>
  )
}
