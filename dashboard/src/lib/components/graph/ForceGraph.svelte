<script lang="ts">
  import * as d3 from 'd3';
  import type { NodeDatum, LinkDatum } from './types.js';

  // Types are imported from ./types.ts to keep the instance script clean.

  let {
    data = { nodes: [], links: [] } as { nodes: NodeDatum[]; links: LinkDatum[] },
    width = 800,
    height = 500
  } = $props();

  let container: HTMLDivElement | null = null;
  let svg: d3.Selection<SVGSVGElement, unknown, null, undefined>;
  let g: d3.Selection<SVGGElement, unknown, null, undefined>;
  let linkSel: d3.Selection<SVGLineElement, LinkDatum, SVGGElement, unknown>;
  let nodeSel: d3.Selection<SVGCircleElement, NodeDatum, SVGGElement, unknown>;
  let labelSel: d3.Selection<SVGTextElement, NodeDatum, SVGGElement, unknown>;
  let simulation: d3.Simulation<NodeDatum, LinkDatum>;

  const color = d3.scaleOrdinal(d3.schemeTableau10);

  $effect(() => {
    if (!container) return;

    // Clear container
    container.innerHTML = '';

    svg = d3
      .select(container)
      .append('svg')
      .attr('viewBox', `0 0 ${width} ${height}`)
      .attr('width', '100%')
      .attr('height', '100%')
      .attr('class', 'rounded-md border bg-card text-card-foreground');

    g = svg.append('g');

    // Zoom + pan
    const zoomed = (event: d3.D3ZoomEvent<SVGSVGElement, unknown>) => {
      g.attr('transform', event.transform.toString());
    };
    svg.call(d3.zoom<SVGSVGElement, unknown>().scaleExtent([0.25, 4]).on('zoom', zoomed));

    // Links
    linkSel = g
      .append('g')
      .attr('stroke', 'currentColor')
      .attr('stroke-opacity', 0.3)
      .selectAll('line')
      .data(data.links)
      .join('line')
      .attr('stroke-width', (d: LinkDatum) => Math.max(1, (d.value || 1)))
      .attr('class', 'transition-opacity');

    // Nodes
    nodeSel = g
      .append('g')
      .attr('stroke', '#fff')
      .attr('stroke-width', 1.5)
      .selectAll('circle')
      .data(data.nodes)
      .join('circle')
      .attr('r', 8)
      .attr('fill', (d: NodeDatum) => color(String(d.group ?? d.id)))
      .attr('class', 'cursor-grab transition-colors hover:opacity-80');

    // Labels
    labelSel = g
      .append('g')
      .selectAll('text')
      .data(data.nodes)
      .join('text')
      .attr('font-size', 11)
      .attr('dx', 10)
      .attr('dy', '0.35em')
      .attr('class', 'select-none fill-foreground')
      .text((d: NodeDatum) => d.id);

    // Drag behavior
    const dragstarted = (event: d3.D3DragEvent<SVGCircleElement, NodeDatum, unknown>, d: NodeDatum) => {
      if (!event.active) simulation.alphaTarget(0.3).restart();
      d.fx = d.x;
      d.fy = d.y;
    };
    const dragged = (event: d3.D3DragEvent<SVGCircleElement, NodeDatum, unknown>, d: NodeDatum) => {
      d.fx = event.x;
      d.fy = event.y;
    };
    const dragended = (event: d3.D3DragEvent<SVGCircleElement, NodeDatum, unknown>, d: NodeDatum) => {
      if (!event.active) simulation.alphaTarget(0);
      d.fx = null;
      d.fy = null;
    };
    nodeSel.call(
      d3
        .drag<SVGCircleElement, NodeDatum>()
        .on('start', dragstarted)
        .on('drag', dragged)
        .on('end', dragended)
    );

    // Force simulation
    simulation = d3
      .forceSimulation<NodeDatum>(data.nodes)
      .force(
        'link',
        d3
          .forceLink<NodeDatum, LinkDatum>(data.links)
          .id((d: NodeDatum) => d.id)
          .distance((d) => 60 + (d.value ? d.value * 10 : 0))
          .strength(0.2)
      )
      .force('charge', d3.forceManyBody().strength(-120))
      .force('center', d3.forceCenter(width / 2, height / 2))
      .force('collision', d3.forceCollide().radius(18))
      .on('tick', () => {
        linkSel
          .attr('x1', (d: LinkDatum) => (d.source as NodeDatum).x!)
          .attr('y1', (d: LinkDatum) => (d.source as NodeDatum).y!)
          .attr('x2', (d: LinkDatum) => (d.target as NodeDatum).x!)
          .attr('y2', (d: LinkDatum) => (d.target as NodeDatum).y!);

        nodeSel.attr('cx', (d: NodeDatum) => d.x!).attr('cy', (d: NodeDatum) => d.y!);

        labelSel.attr('x', (d: NodeDatum) => d.x!).attr('y', (d: NodeDatum) => d.y!);
      });

    return () => {
      simulation?.stop();
    };
  });
</script>

<div bind:this={container} class="w-full h-[520px]"></div>

<style>
  :global(svg) { touch-action: pinch-zoom; }
  :global(circle:active) { cursor: grabbing; }
  :global(text) { paint-order: stroke; stroke: var(--background); stroke-width: 3px; }
</style>
