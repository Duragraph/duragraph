# Human-in-the-Loop Example

Demonstrates human approval workflows using `@human_node` for content review before publishing.

## Features

- **`@human_node`** decorator pauses execution for human review
- **`interrupt_before`** stops before node execution
- **Resume API** continues after approval/rejection
- **DuraGraph Studio** shows ApprovalDialog for reviewers

## Running

```bash
pip install duragraph
python main.py
```

## Flow

```
generate_draft → review_draft (HITL) → publish
```

When served on the control plane, `review_draft` sets the run to `requires_action` status. Reviewers use Studio to approve or reject.
