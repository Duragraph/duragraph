# Multi-Agent Example

Demonstrates a supervisor pattern where multiple specialist agents (researcher, writer, reviewer) are composed into a parent graph using `SubgraphNode`.

## Features

- **Supervisor graph** coordinates specialist agents
- **SubgraphNode** composes child graphs as single nodes
- **State passing** flows between parent and child graphs
- Sequential pipeline: plan → research → write → review → summarize

## Running

```bash
pip install duragraph
python main.py
```

## Architecture

```
Supervisor
├── plan (entrypoint)
├── research (SubgraphNode → Researcher)
├── write (SubgraphNode → Writer)
├── review (SubgraphNode → Reviewer)
└── summarize
```
