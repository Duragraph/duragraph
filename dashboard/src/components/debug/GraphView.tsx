import { useMemo } from 'react'

interface GraphNode {
  id: string
  type: string
}

interface GraphEdge {
  source: string
  target: string
}

interface GraphTopologyProps {
  nodes: GraphNode[]
  edges: GraphEdge[]
  activeNodeId?: string
  completedNodeIds?: string[]
}

const NODE_COLORS: Record<string, string> = {
  function: 'fill-blue-100 stroke-blue-400',
  llm: 'fill-purple-100 stroke-purple-400',
  tool: 'fill-green-100 stroke-green-400',
  router: 'fill-orange-100 stroke-orange-400',
  human: 'fill-pink-100 stroke-pink-400',
  subgraph: 'fill-cyan-100 stroke-cyan-400',
}

const NODE_TEXT_COLORS: Record<string, string> = {
  function: 'fill-blue-700',
  llm: 'fill-purple-700',
  tool: 'fill-green-700',
  router: 'fill-orange-700',
  human: 'fill-pink-700',
  subgraph: 'fill-cyan-700',
}

export function GraphTopology({
  nodes,
  edges,
  activeNodeId,
  completedNodeIds = [],
}: GraphTopologyProps) {
  const layout = useMemo(() => {
    const nodeWidth = 140
    const nodeHeight = 40
    const gapX = 60
    const gapY = 80
    const cols = Math.ceil(Math.sqrt(nodes.length))

    const positions = new Map<string, { x: number; y: number }>()
    nodes.forEach((node, i) => {
      const col = i % cols
      const row = Math.floor(i / cols)
      positions.set(node.id, {
        x: 40 + col * (nodeWidth + gapX),
        y: 40 + row * (nodeHeight + gapY),
      })
    })

    const width = 40 + cols * (nodeWidth + gapX)
    const height = 40 + Math.ceil(nodes.length / cols) * (nodeHeight + gapY)

    return { positions, nodeWidth, nodeHeight, width, height }
  }, [nodes])

  return (
    <svg
      viewBox={`0 0 ${layout.width} ${layout.height}`}
      className="w-full h-auto"
      style={{ maxHeight: '400px' }}
    >
      <defs>
        <marker
          id="arrowhead"
          markerWidth="8"
          markerHeight="6"
          refX="8"
          refY="3"
          orient="auto"
        >
          <polygon points="0 0, 8 3, 0 6" className="fill-muted-foreground" />
        </marker>
      </defs>

      {edges.map((edge, i) => {
        const from = layout.positions.get(edge.source)
        const to = layout.positions.get(edge.target)
        if (!from || !to) return null
        return (
          <line
            key={`edge-${i}`}
            x1={from.x + layout.nodeWidth / 2}
            y1={from.y + layout.nodeHeight}
            x2={to.x + layout.nodeWidth / 2}
            y2={to.y}
            className="stroke-muted-foreground"
            strokeWidth={1.5}
            markerEnd="url(#arrowhead)"
          />
        )
      })}

      {nodes.map((node) => {
        const pos = layout.positions.get(node.id)
        if (!pos) return null
        const isActive = node.id === activeNodeId
        const isCompleted = completedNodeIds.includes(node.id)
        const colorClass = NODE_COLORS[node.type] ?? NODE_COLORS['function']
        const textClass = NODE_TEXT_COLORS[node.type] ?? NODE_TEXT_COLORS['function']

        return (
          <g key={node.id}>
            <rect
              x={pos.x}
              y={pos.y}
              width={layout.nodeWidth}
              height={layout.nodeHeight}
              className={colorClass}
              strokeWidth={isActive ? 3 : 1.5}
              rx={0}
            />
            {isActive && (
              <rect
                x={pos.x - 2}
                y={pos.y - 2}
                width={layout.nodeWidth + 4}
                height={layout.nodeHeight + 4}
                className="fill-none stroke-orange-500"
                strokeWidth={2}
                strokeDasharray="4 2"
              >
                <animate
                  attributeName="stroke-dashoffset"
                  from="0"
                  to="12"
                  dur="1s"
                  repeatCount="indefinite"
                />
              </rect>
            )}
            {isCompleted && (
              <circle
                cx={pos.x + layout.nodeWidth - 8}
                cy={pos.y + 8}
                r={5}
                className="fill-green-500"
              />
            )}
            <text
              x={pos.x + layout.nodeWidth / 2}
              y={pos.y + layout.nodeHeight / 2 + 1}
              textAnchor="middle"
              dominantBaseline="middle"
              className={`text-xs font-mono font-semibold ${textClass}`}
            >
              {node.id.length > 16 ? node.id.slice(0, 14) + '…' : node.id}
            </text>
          </g>
        )
      })}
    </svg>
  )
}

interface StatePanelProps {
  state: Record<string, unknown>
  previousState?: Record<string, unknown>
}

export function StatePanel({ state, previousState }: StatePanelProps) {
  const keys = Object.keys(state)

  return (
    <div>
      <h3 className="mb-2 text-sm font-semibold">State Inspector</h3>
      {keys.length === 0 ? (
        <p className="text-xs text-muted-foreground">Empty state</p>
      ) : (
        <div className="space-y-1">
          {keys.map((key) => {
            const changed =
              previousState !== undefined &&
              JSON.stringify(state[key]) !== JSON.stringify(previousState[key])
            const isNew = previousState !== undefined && !(key in previousState)

            return (
              <div
                key={key}
                className={`flex items-start gap-2 p-1.5 text-xs font-mono ${
                  changed
                    ? 'bg-yellow-50 border-l-2 border-yellow-400'
                    : isNew
                      ? 'bg-green-50 border-l-2 border-green-400'
                      : ''
                }`}
              >
                <span className="font-semibold text-foreground min-w-[80px] shrink-0">
                  {key}
                </span>
                <span className="text-muted-foreground break-all">
                  {typeof state[key] === 'string'
                    ? `"${state[key]}"`
                    : JSON.stringify(state[key])}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
