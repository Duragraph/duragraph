import { NodePalette } from '@/components/editor/NodePalette'
import { EditorCanvas } from '@/components/editor/EditorCanvas'
import { NodeProperties } from '@/components/editor/NodeProperties'
import { EditorToolbar } from '@/components/editor/EditorToolbar'

export function EditorView() {
  return (
    <div className="flex h-full flex-col">
      <EditorToolbar />
      <div className="flex flex-1 overflow-hidden">
        <NodePalette />
        <EditorCanvas />
        <NodeProperties />
      </div>
    </div>
  )
}
