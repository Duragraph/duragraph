<script lang="ts">
  import ForceGraph from '$lib/components/graph/ForceGraph.svelte';
  import sample from '$lib/sample-graph.json';
  import { Button } from '$lib/components/ui/button/index.js';

  let jsonText = JSON.stringify(sample, null, 2);
  let parseError: string | null = null;
  let data = sample as { nodes: any[]; links: any[] };

  function applyJson() {
    try {
      const parsed = JSON.parse(jsonText);
      if (!parsed || !Array.isArray(parsed.nodes) || !Array.isArray(parsed.links)) {
        throw new Error('JSON must have { nodes: [], links: [] }');
      }
      data = parsed;
      parseError = null;
    } catch (e: any) {
      parseError = e?.message ?? 'Invalid JSON';
    }
  }
</script>

<section class="space-y-4 p-4 max-w-6xl mx-auto">
  <header class="flex items-center justify-between">
    <h1 class="text-xl font-semibold">JSON Graph</h1>
    <a href="/" class="text-sm underline hover:opacity-80">Back</a>
  </header>

  <div class="grid md:grid-cols-2 gap-4">
    <div class="space-y-2">
      <label class="text-sm font-medium">Graph JSON</label>
      <textarea
        bind:value={jsonText}
        class="min-h-[300px] w-full rounded-md border bg-background p-3 font-mono text-sm"
        spellcheck="false"
      />
      <div class="flex items-center gap-2">
        <Button on:click={applyJson} variant="default">Apply</Button>
        <Button on:click={() => (jsonText = JSON.stringify(sample, null, 2))} variant="outline">
          Reset sample
        </Button>
      </div>
      {#if parseError}
        <p class="text-destructive text-sm">{parseError}</p>
      {/if}
      <p class="text-muted-foreground text-xs">
        Expected shape: {`{ nodes: [{ id }], links: [{ source, target }] }`}
      </p>
    </div>

    <div class="rounded-md border p-2">
      <ForceGraph {data} width={800} height={500} />
    </div>
  </div>
</section>

