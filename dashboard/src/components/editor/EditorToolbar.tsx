import { Download, Trash2, Upload } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { useEditorStore } from "@/stores/editor"

// Editor toolbar — graph metadata inputs on the left, status + actions
// on the right. Plain DOM file pickers for import/export because
// downloading a synthesised JSON file from React state doesn't need a
// shadcn primitive beyond the trigger button.

export function EditorToolbar() {
  const {
    graphName,
    graphDescription,
    setGraphMeta,
    toDefinition,
    clear,
    isDirty,
    nodes,
    edges,
  } = useEditorStore()

  function handleExport() {
    const def = toDefinition()
    const json = JSON.stringify(def, null, 2)
    const blob = new Blob([json], { type: "application/json" })
    const url = URL.createObjectURL(blob)
    const a = document.createElement("a")
    a.href = url
    a.download = `${def.id}.json`
    a.click()
    URL.revokeObjectURL(url)
  }

  function handleImport() {
    const input = document.createElement("input")
    input.type = "file"
    input.accept = ".json"
    input.onchange = async (e) => {
      const file = (e.target as HTMLInputElement).files?.[0]
      if (!file) return
      const text = await file.text()
      try {
        const def = JSON.parse(text)
        useEditorStore.getState().loadGraph(def)
      } catch {
        // TODO: replace with shadcn Sonner toast once we wire the
        // app-level <Toaster />; alert() unblocks until then.
        alert("Invalid graph definition file")
      }
    }
    input.click()
  }

  const empty = nodes.length === 0

  return (
    <div className="flex items-center gap-3 border-b bg-card px-4 py-2">
      <Input
        value={graphName}
        onChange={(e) => setGraphMeta(e.target.value, graphDescription)}
        placeholder="Graph name"
        className="h-8 w-48 font-semibold"
      />

      <Input
        value={graphDescription}
        onChange={(e) => setGraphMeta(graphName, e.target.value)}
        placeholder="Description…"
        className="h-8 w-64"
      />

      <div className="flex-1" />

      <span className="font-mono text-xs text-muted-foreground">
        {nodes.length} nodes · {edges.length} edges
      </span>
      {isDirty && (
        <Badge variant="outline" className="text-[10px] uppercase">
          Unsaved
        </Badge>
      )}

      <Separator orientation="vertical" className="h-6" />

      <Button variant="outline" size="sm" onClick={handleImport}>
        <Upload className="size-4" />
        Import
      </Button>

      <Button
        variant="outline"
        size="sm"
        onClick={handleExport}
        disabled={empty}
      >
        <Download className="size-4" />
        Export JSON
      </Button>

      <Button
        variant="destructive"
        size="sm"
        onClick={clear}
        disabled={empty}
      >
        <Trash2 className="size-4" />
        Clear
      </Button>
    </div>
  )
}
