# Edge Operators

The DuraGraph Python SDK provides a convenient `>>` operator for defining edges between nodes in your graph. This allows you to specify the flow of execution directly in the class body when defining your graph.

## Basic Usage

The `>>` operator creates edges between nodes, defining the execution flow:

```python
from duragraph import Graph, node, entrypoint

@Graph(id="simple_flow")
class SimpleFlow:
    @entrypoint
    @node()
    def start(self, state):
        return state
    
    @node()
    def process(self, state):
        return state
    
    @node()
    def finish(self, state):
        return state
    
    # Define the flow
    start >> process >> finish
```

## Chaining Multiple Nodes

You can chain multiple nodes together in a single expression:

```python
@Graph(id="pipeline")
class DataPipeline:
    @entrypoint
    @node()
    def load(self, state):
        return state
    
    @node()
    def validate(self, state):
        return state
    
    @node()
    def transform(self, state):
        return state
    
    @node()
    def save(self, state):
        return state
    
    # Create a linear pipeline
    load >> validate >> transform >> save
```

## Branching and Merging

You can create multiple paths that branch and merge:

```python
@Graph(id="parallel_processing")
class ParallelProcessing:
    @entrypoint
    @node()
    def start(self, state):
        return state
    
    @node()
    def process_a(self, state):
        return state
    
    @node()
    def process_b(self, state):
        return state
    
    @node()
    def merge(self, state):
        return state
    
    @node()
    def finish(self, state):
        return state
    
    # Create parallel branches that merge
    start >> process_a >> merge
    start >> process_b >> merge
    merge >> finish
```

## With LLM Nodes

The `>>` operator works seamlessly with LLM nodes:

```python
from duragraph import Graph, llm_node, entrypoint

@Graph(id="ai_assistant")
class AIAssistant:
    @entrypoint
    @llm_node(model="gpt-4o-mini", temperature=0.3)
    def understand(self, state):
        """Understand user intent."""
        return state
    
    @llm_node(model="gpt-4o-mini", temperature=0.7)
    def generate(self, state):
        """Generate response."""
        return state
    
    @llm_node(model="gpt-4o-mini", temperature=0.5)
    def refine(self, state):
        """Refine the response."""
        return state
    
    # Define AI processing pipeline
    understand >> generate >> refine
```

## Async Support

The `>>` operator works with both synchronous and asynchronous nodes:

```python
@Graph(id="async_flow")
class AsyncFlow:
    @entrypoint
    @node()
    async def fetch_data(self, state):
        # Async operation
        return state
    
    @node()
    def process_data(self, state):
        # Sync operation
        return state
    
    @node()
    async def save_data(self, state):
        # Async operation
        return state
    
    # Mix async and sync nodes
    fetch_data >> process_data >> save_data
```

## Router Nodes

For conditional routing, use router nodes with the edge builder API:

```python
from duragraph import Graph, router_node, node, entrypoint, edge

@Graph(id="conditional_flow")
class ConditionalFlow:
    @entrypoint
    @node()
    def start(self, state):
        return state
    
    @router_node()
    def router(self, state):
        # Return the name of the next node
        if state.get("condition"):
            return "path_a"
        return "path_b"
    
    @node()
    def path_a(self, state):
        return state
    
    @node()
    def path_b(self, state):
        return state
    
    @node()
    def end(self, state):
        return state
    
    # Connect to router
    start >> router
    
    # Router outputs are connected using the edge builder
    # (The >> operator doesn't support conditional routing)
    edge("router").to_conditional({
        "path_a": "path_a",
        "path_b": "path_b"
    })
    
    # Both paths lead to end
    path_a >> end
    path_b >> end
```

## Important Notes

1. **Class Definition Time**: The `>>` operator works at class definition time, not at runtime. Edges are defined once when the class is created.

2. **Node Names**: The operator uses the method names as node identifiers. Make sure your method names are unique within the graph.

3. **Order Matters**: Nodes must be defined before they can be referenced in edge definitions.

4. **No Cycles**: Be careful not to create cycles unless your graph logic specifically handles them.

5. **Router Limitations**: The `>>` operator creates simple edges. For conditional routing based on runtime values, use the `edge()` builder with `to_conditional()`.

## Alternative: Edge Builder API

If you prefer more explicit control or need conditional routing, you can use the edge builder API:

```python
from duragraph import Graph, node, entrypoint, edge

@Graph(id="explicit_edges")
class ExplicitEdges:
    @entrypoint
    @node()
    def start(self, state):
        return state
    
    @node()
    def middle(self, state):
        return state
    
    @node()
    def end(self, state):
        return state
    
    # Using edge builder instead of >>
    edges = [
        edge("start").to("middle"),
        edge("middle").to("end")
    ]
```

Both approaches achieve the same result, but the `>>` operator provides a more intuitive and readable way to define simple sequential flows.