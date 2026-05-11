import { useRef, useCallback, useState } from 'react'
import { useEditorStore } from '@/stores/editor'
import type { EditorNodeType } from '@/types/entities'

const NODE_W = 160
const NODE_H = 48

const NODE_COLORS: Record<string, { bg: string; border: string; text: string }> = {
  function: { bg: 'bg-blue-50', border: 'border-blue-400', text: 'text-blue-700' },
  llm: { bg: 'bg-purple-50', border: 'border-purple-400', text: 'text-purple-700' },
  tool: { bg: 'bg-green-50', border: 'border-green-400', text: 'text-green-700' },
  router: { bg: 'bg-orange-50', border: 'border-orange-400', text: 'text-orange-700' },
  human: { bg: 'bg-pink-50', border: 'border-pink-400', text: 'text-pink-700' },
  subgraph: { bg: 'bg-cyan-50', border: 'border-cyan-400', text: 'text-cyan-700' },
}

const NODE_TYPE_LABELS: Record<EditorNodeType, string> = {
  function: 'Function',
  llm: 'LLM',
  tool: 'Tool',
  router: 'Router',
  human: 'Human',
  subgraph: 'Subgraph',
}

export function EditorCanvas() {
  const {
    nodes,
    edges,
    selectedNodeId,
    connectingFrom,
    addNode,
    moveNode,
    selectNode,
    selectEdge,
    selectedEdgeId,
    setConnectingFrom,
    addEdge,
  } = useEditorStore()

  const canvasRef = useRef<HTMLDivElement>(null)
  const [dragState, setDragState] = useState<{
    nodeId: string
    offsetX: number
    offsetY: number
  } | null>(null)
  const [connectEnd, setConnectEnd] = useState<{ x: number; y: number } | null>(null)

  const getCanvasPos = useCallback(
    (e: React.MouseEvent) => {
      if (!canvasRef.current) return { x: 0, y: 0 }
      const rect = canvasRef.current.getBoundingClientRect()
      return { x: e.clientX - rect.left, y: e.clientY - rect.top }
    },
    [],
  )

  function handleCanvasClick(e: React.MouseEvent) {
    if (e.target === canvasRef.current) {
      selectNode(null)
      selectEdge(null)
      if (connectingFrom) {
        setConnectingFrom(null)
        setConnectEnd(null)
      }
    }
  }

  function handleCanvasDrop(e: React.DragEvent) {
    e.preventDefault()
    const nodeType = e.dataTransfer.getData('node-type') as EditorNodeType
    if (!nodeType) return
    const pos = getCanvasPos(e as unknown as React.MouseEvent)
    addNode(nodeType, pos.x - NODE_W / 2, pos.y - NODE_H / 2)
  }

  function handleNodeMouseDown(e: React.MouseEvent, nodeId: string) {
    e.stopPropagation()
    if (connectingFrom) {
      addEdge(connectingFrom, nodeId)
      setConnectingFrom(null)
      setConnectEnd(null)
      return
    }
    selectNode(nodeId)
    const node = nodes.find((n) => n.id === nodeId)
    if (!node) return
    const pos = getCanvasPos(e)
    setDragState({ nodeId, offsetX: pos.x - node.x, offsetY: pos.y - node.y })
  }

  function handleMouseMove(e: React.MouseEvent) {
    if (dragState) {
      const pos = getCanvasPos(e)
      moveNode(dragState.nodeId, pos.x - dragState.offsetX, pos.y - dragState.offsetY)
    }
    if (connectingFrom) {
      const pos = getCanvasPos(e)
      setConnectEnd(pos)
    }
  }

  function handleMouseUp() {
    setDragState(null)
  }

  function handleConnectStart(e: React.MouseEvent, nodeId: string) {
    e.stopPropagation()
    setConnectingFrom(nodeId)
  }

  return (
    <div
      ref={canvasRef}
      className="relative flex-1 overflow-auto bg-muted/30"
      onClick={handleCanvasClick}
      onDrop={handleCanvasDrop}
      onDragOver={(e) => e.preventDefault()}
      onMouseMove={handleMouseMove}
      onMouseUp={handleMouseUp}
      style={{ minHeight: '100%', cursor: connectingFrom ? 'crosshair' : 'default' }}
    >
      <svg className="absolute inset-0 w-full h-full pointer-events-none" style={{ zIndex: 0 }}>
        <defs>
          <marker id="editor-arrow" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <polygon points="0 0, 8 3, 0 6" className="fill-muted-foreground" />
          </marker>
        </defs>

        {edges.map((edge) => {
          const src = nodes.find((n) => n.id === edge.source)
          const tgt = nodes.find((n) => n.id === edge.target)
          if (!src || !tgt) return null
          const x1 = src.x + NODE_W / 2
          const y1 = src.y + NODE_H
          const x2 = tgt.x + NODE_W / 2
          const y2 = tgt.y
          const isSelected = selectedEdgeId === edge.id
          return (
            <g key={edge.id}>
              <line
                x1={x1}
                y1={y1}
                x2={x2}
                y2={y2}
                stroke="transparent"
                strokeWidth={12}
                style={{ pointerEvents: 'stroke', cursor: 'pointer' }}
                onClick={(e) => {
                  e.stopPropagation()
                  selectEdge(edge.id)
                }}
              />
              <line
                x1={x1}
                y1={y1}
                x2={x2}
                y2={y2}
                className={isSelected ? 'stroke-primary' : 'stroke-muted-foreground'}
                strokeWidth={isSelected ? 2.5 : 1.5}
                markerEnd="url(#editor-arrow)"
              />
              {edge.label && (
                <text
                  x={(x1 + x2) / 2}
                  y={(y1 + y2) / 2 - 6}
                  textAnchor="middle"
                  className="fill-muted-foreground text-[10px] font-mono"
                >
                  {edge.label}
                </text>
              )}
            </g>
          )
        })}

        {connectingFrom && connectEnd && (() => {
          const src = nodes.find((n) => n.id === connectingFrom)
          if (!src) return null
          return (
            <line
              x1={src.x + NODE_W / 2}
              y1={src.y + NODE_H}
              x2={connectEnd.x}
              y2={connectEnd.y}
              className="stroke-primary"
              strokeWidth={1.5}
              strokeDasharray="6 3"
            />
          )
        })()}
      </svg>

      {nodes.map((node) => {
        const colors = NODE_COLORS[node.type] ?? NODE_COLORS.function
        const isSelected = selectedNodeId === node.id
        return (
          <div
            key={node.id}
            className={`absolute flex flex-col items-center justify-center border-2 select-none ${colors.bg} ${colors.border} ${colors.text} ${
              isSelected ? 'ring-2 ring-primary ring-offset-1' : ''
            } ${node.isEntrypoint ? 'border-dashed' : ''}`}
            style={{
              left: node.x,
              top: node.y,
              width: NODE_W,
              height: NODE_H,
              zIndex: isSelected ? 20 : 10,
              cursor: dragState?.nodeId === node.id ? 'grabbing' : 'grab',
            }}
            onMouseDown={(e) => handleNodeMouseDown(e, node.id)}
          >
            <span className="text-xs font-semibold font-mono truncate max-w-[140px] px-1">
              {node.label}
            </span>
            <span className="text-[10px] opacity-60">
              {NODE_TYPE_LABELS[node.type]}
              {node.isEntrypoint ? ' (entry)' : ''}
            </span>
            <button
              className="absolute -bottom-2 left-1/2 -translate-x-1/2 w-4 h-4 bg-muted-foreground/60 hover:bg-primary transition-colors flex items-center justify-center"
              title="Drag to connect"
              onMouseDown={(e) => handleConnectStart(e, node.id)}
            >
              <span className="text-[8px] text-white font-bold">+</span>
            </button>
          </div>
        )
      })}

      {nodes.length === 0 && (
        <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
          <div className="text-center text-muted-foreground">
            <p className="text-lg font-semibold">Drop nodes here to start building</p>
            <p className="text-sm mt-1">Drag node types from the palette on the left</p>
          </div>
        </div>
      )}
    </div>
  )
}
