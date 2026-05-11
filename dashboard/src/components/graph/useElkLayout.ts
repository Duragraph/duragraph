import { useEffect, useRef, useState } from "react"
import ELK, { type ElkNode } from "elkjs/lib/elk.bundled.js"
import type { Edge, Node } from "@xyflow/react"

// useElkLayout — async hook that runs ELK's layered layout algorithm
// and returns the same nodes with `position` populated.
//
// Why ELK and not Dagre / D3-force:
//   * Dagre — only does layered DAG; same boxy "brick" feel my hand-
//     rolled BFS placer produced.
//   * D3-force — physics simulation, very organic, but settles
//     non-deterministically and runs continuously. Wrong fit for a
//     read-only run-trace view where the layout should be stable.
//   * ELK — layered by default but with `nodePlacement.strategy:
//     NETWORK_SIMPLEX` + edge-routing `ORTHOGONAL → POLYLINE`
//     produces softer angles and balanced columns. Deterministic
//     (no animation), single async pass on mount.
//
// Per xyflow's auto-layout guide (reactflow.dev), the standard
// pattern is: wait for `useNodesInitialized()` to fire so each
// node's measured.width/height is known, then run the layout, then
// `setNodes()` with the laid-out positions. The hook below assumes
// the caller knows the nodes are initialized (we call it from
// inside the parent which gates on that).

const elk = new ELK()

const DEFAULT_OPTIONS = {
  "elk.algorithm": "layered",
  // Top-down: matches the "workflow flows from start at the top"
  // mental model. Switch to "RIGHT" for left-to-right reading order.
  "elk.direction": "DOWN",
  // Distance between vertically adjacent layers (rows).
  "elk.layered.spacing.nodeNodeBetweenLayers": "80",
  // Distance between sibling nodes within a layer.
  "elk.spacing.nodeNode": "60",
  // NETWORK_SIMPLEX produces evenly-distributed columns; the
  // alternative SIMPLE crams nodes against the left edge.
  "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
  // POLYLINE edge routing softens the right-angle "brick" feel of
  // the default ORTHOGONAL setting.
  "elk.edgeRouting": "POLYLINE",
} as const

/** Run ELK on a snapshot of nodes + edges; resolves with the
 * positioned nodes. Width/height defaults are conservative — when
 * the caller has measured nodes via xyflow's `node.measured`, the
 * real values override these. */
export async function layoutWithElk<NodeType extends Node, EdgeType extends Edge>(
  nodes: NodeType[],
  edges: EdgeType[],
): Promise<NodeType[]> {
  if (nodes.length === 0) return nodes

  const graph: ElkNode = {
    id: "root",
    layoutOptions: DEFAULT_OPTIONS,
    children: nodes.map((n) => ({
      id: n.id,
      width: n.measured?.width ?? 220,
      height: n.measured?.height ?? 80,
    })),
    edges: edges.map((e) => ({
      id: e.id,
      sources: [e.source],
      targets: [e.target],
    })),
  }

  const laid = await elk.layout(graph)

  const positionById = new Map<string, { x: number; y: number }>()
  for (const c of laid.children ?? []) {
    positionById.set(c.id, { x: c.x ?? 0, y: c.y ?? 0 })
  }

  return nodes.map((n) => {
    const pos = positionById.get(n.id)
    return pos ? { ...n, position: pos } : n
  })
}

/** topologyKey returns a stable string derived from node IDs + edge
 * (source,target) pairs. Two renders with the same topology produce
 * the same key; two renders that differ only in node DATA (status
 * badges, label changes) produce the same key too. Using this as
 * the effect dep means ELK runs once per shape change, not once per
 * parent re-render — which is what was freezing user drags before. */
function topologyKey<NodeType extends Node, EdgeType extends Edge>(
  nodes: NodeType[],
  edges: EdgeType[],
): string {
  const n = nodes
    .map((x) => x.id)
    .sort()
    .join("|")
  const e = edges
    .map((x) => `${x.source}>${x.target}`)
    .sort()
    .join("|")
  return `${n}::${e}`
}

/** React hook variant — runs the layout once per topology change.
 *
 * Returns laid-out nodes (or the original nodes on the first render,
 * before the async layout resolves).
 *
 * IMPORTANT: this hook does NOT track xyflow's drag updates. Drag is
 * handled by xyflow's own internal state once nodes are passed in
 * via `useNodesState`. Use this hook only when feeding the INITIAL
 * positions for a topology you haven't laid out before. The parent
 * is responsible for calling `setNodes(laidOut)` once and then
 * letting xyflow own positions from there. */
export function useElkLayout<NodeType extends Node, EdgeType extends Edge>(
  nodes: NodeType[],
  edges: EdgeType[],
): NodeType[] {
  const [laid, setLaid] = useState<NodeType[]>(nodes)

  // Snapshot the latest inputs into a ref so the effect can read the
  // current values without re-running when data (vs topology) changes.
  const latest = useRef({ nodes, edges })
  latest.current = { nodes, edges }

  const key = topologyKey(nodes, edges)

  useEffect(() => {
    let cancelled = false
    layoutWithElk(latest.current.nodes, latest.current.edges).then((next) => {
      if (!cancelled) setLaid(next)
    })
    return () => {
      cancelled = true
    }
    // Re-run ONLY when topology changes. Status updates and label
    // changes leave key identical → effect skips → user-dragged
    // positions are preserved.
  }, [key])

  return laid
}
