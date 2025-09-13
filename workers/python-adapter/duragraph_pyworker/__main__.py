import asyncio
import json
import logging
import os
from typing import Any, Dict

from temporalio import activity, worker, client

logger = logging.getLogger(__name__)
logging.basicConfig(level=logging.INFO)


# ---- Activity Stubs ----
@activity.defn
async def llm_call(args: Dict[str, Any]) -> Dict[str, Any]:
    """Stub for an LLM call. For now, returns dummy tokens and simulates streaming via signals."""
    logger.info("llm_call invoked with args=%s", args)

    # Simulate streaming tokens via signal (TODO: implement Temporal signal fanout)
    output = {"response": "stub LLM output"}
    return output


@activity.defn
async def tool(args: Dict[str, Any]) -> Dict[str, Any]:
    """Stub for dynamic tool invocation."""
    tool_name = args.get("name", "unknown")
    logger.info("tool '%s' called with args=%s", tool_name, args.get("input"))
    # TODO: dynamic import by name
    return {"tool": tool_name, "status": "completed"}


# ---- Checkpoint Hooks (stubs) ----
def checkpoint_before(node_id: str, data: Dict[str, Any]) -> None:
    # TODO: write checkpoint JSON to S3
    # TODO: write metadata to Postgres
    logger.debug("checkpoint before node=%s data=%s", node_id, data)


def checkpoint_after(node_id: str, data: Dict[str, Any]) -> None:
    # TODO: write checkpoint JSON to S3
    # TODO: write metadata to Postgres
    logger.debug("checkpoint after node=%s data=%s", node_id, data)


# ---- Entrypoint ----
async def main() -> None:
    temporal_target = os.getenv("TEMPORAL_HOSTPORT", "localhost:7233")
    namespace = os.getenv("NAMESPACE", "default")

    c = await client.Client.connect(temporal_target, namespace=namespace)

    task_queue = "python-adapter"
    w = worker.Worker(
        c,
        task_queue=task_queue,
        activities=[llm_call, tool],
    )

    logger.info("Starting python worker on task queue '%s'", task_queue)

    await w.run()


if __name__ == "__main__":
    asyncio.run(main())