# Getting Started

Welcome to the project! This guide will walk you through the basics of setting up your environment, running your first workflow, and exploring the system architecture.

---

## 1. Installation

1. Clone the repository:
   ```bash
   git clone https://github.com/your-org/your-repo.git
   cd your-repo
   ```

2. Set up your Python environment:
   ```bash
   python -m venv venv
   source venv/bin/activate
   pip install -r requirements.txt
   ```

3. (Optional) Build and run with Docker:
   ```bash
   docker compose up --build
   ```

---

## 2. Running a Hello World Workflow

1. Start the services:
   ```bash
   ./init_structure.sh
   ```

2. Run the example workflow defined in [`schemas/ir/examples/hello.json`](../schemas/ir/examples/hello.json).

3. Check logs/output to see the results.

---

## 3. Next Steps

To better understand the system, explore the architecture documentation:

- [Overview](architecture/overview.md) – components, data flows, sequence diagrams
- [API Shim](architecture/api-shim.md) – REST/SSE endpoints, idempotency, error model
- [Temporal](architecture/temporal.md) – workflows, activities, signals/queries, retries, versioning
- [Intermediate Representation (IR)](architecture/ir.md) – schema fields, examples, validation
- [Workers](architecture/workers.md) – adapters, task queues, streaming, checkpointing

---

## 4. Contributing

Please read the [Contributing Guide](../CONTRIBUTING.md), [Code of Conduct](../CODE_OF_CONDUCT.md), and [Security Policy](../SECURITY.md) before contributing.

---

You are now ready to dive deeper into the system!