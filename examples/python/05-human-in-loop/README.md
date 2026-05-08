# Human-in-the-Loop Example

Demonstrates human approval workflows using `@human_node` for content review before publishing.

## Features

- **`@human_node`** decorator pauses execution for human review
- **`interrupt_before`** stops before node execution
- **Resume API** continues after approval/rejection
- **DuraGraph Studio** shows ApprovalDialog for reviewers

## Running

> Always use `uv`. Never `pip install`, never `python -m venv`, never `source .venv/bin/activate`.

```bash
DURAGRAPH_URL=http://localhost:18081 PYTHONUNBUFFERED=1 \
  uv run --with-editable /home/qwe/platform/duragraph-org/duragraph-python \
  python main.py
```

## Flow

```
generate_draft → review_draft (HITL) → publish
```

When served on the control plane, `review_draft` sets the run to `requires_action` status. Reviewers use Studio to approve or reject.
