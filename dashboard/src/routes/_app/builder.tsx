import { createFileRoute } from "@tanstack/react-router"
import { NodePalette } from "@/components/editor/NodePalette"
import { EditorCanvas } from "@/components/editor/EditorCanvas"
import { NodeProperties } from "@/components/editor/NodeProperties"
import { EditorToolbar } from "@/components/editor/EditorToolbar"

// Builder route — the in-app visual workflow editor ported from
// studio. Working state lives in `@/stores/editor` (zustand); the
// `toDefinition()` / `loadGraph()` methods on that store convert
// between the editor-native (x, y) shape and the persisted Graph
// schema once the save-to-backend path lands.
//
// The page deliberately takes over the full app pane (-m-6 cancels
// the AppLayout's p-6) so the canvas can expand edge-to-edge.

export const Route = createFileRoute("/_app/builder")({
  component: BuilderPage,
})

function BuilderPage() {
  return (
    <div className="flex h-full flex-col -m-6">
      <EditorToolbar />
      <div className="flex flex-1 overflow-hidden">
        <NodePalette />
        <EditorCanvas />
        <NodeProperties />
      </div>
    </div>
  )
}
