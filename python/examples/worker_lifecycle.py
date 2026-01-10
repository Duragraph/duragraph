"""Example demonstrating worker lifecycle with graceful shutdown."""

import asyncio
import sys
import time
from typing import Any

from duragraph import Graph, entrypoint, node
from duragraph.worker import Worker, WorkerStatus


# Define a sample graph that takes some time to execute
@Graph(id="long_running_task", description="A graph that simulates long-running work")
class LongRunningTask:
    """Graph that simulates processing that takes time."""

    @entrypoint
    @node()
    async def start_processing(self, state: dict[str, Any]) -> dict[str, Any]:
        """Start the processing task."""
        task_id = state.get("task_id", "unknown")
        print(f"  📋 Task {task_id}: Starting processing...")
        state["status"] = "processing"
        state["start_time"] = time.time()

        # Simulate some initial work
        await asyncio.sleep(1)

        return state

    @node()
    async def process_data(self, state: dict[str, Any]) -> dict[str, Any]:
        """Process the data (simulates time-consuming work)."""
        task_id = state.get("task_id", "unknown")
        duration = state.get("duration", 5)

        print(f"  ⚙️  Task {task_id}: Processing for {duration} seconds...")

        # Simulate processing in chunks to show progress
        for i in range(duration):
            await asyncio.sleep(1)
            progress = ((i + 1) / duration) * 100
            print(f"  ⚙️  Task {task_id}: {progress:.0f}% complete...")

        state["result"] = f"Processed {duration} seconds of work"
        state["status"] = "completed"
        return state

    @node()
    async def finalize(self, state: dict[str, Any]) -> dict[str, Any]:
        """Finalize the task."""
        task_id = state.get("task_id", "unknown")
        elapsed = time.time() - state.get("start_time", 0)

        state["elapsed_time"] = elapsed
        print(f"  ✅ Task {task_id}: Completed in {elapsed:.1f}s")

        return state

    # Define the flow
    start_processing >> process_data >> finalize


# Define a quick task for comparison
@Graph(id="quick_task", description="A quick task that completes fast")
class QuickTask:
    """Graph that completes quickly."""

    @entrypoint
    @node()
    def process(self, state: dict[str, Any]) -> dict[str, Any]:
        """Quick processing."""
        task_id = state.get("task_id", "unknown")
        print(f"  ⚡ Task {task_id}: Quick processing done!")
        state["result"] = "Quick task completed"
        return state


class DemoControlPlane:
    """Mock control plane for demonstration."""

    def __init__(self):
        self.pending_work = []
        self.worker_status = None
        self.heartbeat_count = 0
        self.last_heartbeat = None

    def add_work(self, run_id: str, graph_id: str, input_data: dict[str, Any]):
        """Add work to the queue."""
        self.pending_work.append(
            {
                "run_id": run_id,
                "graph_id": graph_id,
                "input": input_data,
                "thread_id": f"thread-{run_id}",
            }
        )

    def get_work(self) -> dict[str, Any] | None:
        """Get next work item."""
        if self.pending_work:
            return self.pending_work.pop(0)
        return None

    def record_heartbeat(self, status: str, metrics: dict[str, Any]):
        """Record heartbeat from worker."""
        self.heartbeat_count += 1
        self.worker_status = status
        self.last_heartbeat = time.time()
        print(
            f"💓 Heartbeat #{self.heartbeat_count}: status={status}, active_runs={metrics.get('active_runs', 0)}"
        )


async def simulate_control_plane(worker: Worker, control_plane: DemoControlPlane):
    """Simulate control plane interactions."""

    # Override worker methods to use mock control plane
    # Store originals for potential restoration (prefixed with _ as intentionally unused here)
    _original_poll = worker._poll_for_work
    _original_heartbeat = worker._heartbeat
    _original_send_event = worker._send_event
    _original_register = worker._register_with_control_plane

    # Mock registration
    async def mock_register(retry_count: int = 0):
        print("📝 Registering with mock control plane...")
        return "demo-worker-id"

    # Mock polling
    async def mock_poll():
        return control_plane.get_work()

    # Mock heartbeat
    async def mock_heartbeat():
        metrics = {
            "active_runs": len(worker._active_runs),
            "runs_completed": worker._health_metrics["runs_completed"],
            "runs_failed": worker._health_metrics["runs_failed"],
        }
        control_plane.record_heartbeat(worker._status.value, metrics)

    # Mock event sending
    async def mock_send_event(run_id: str, event_type: str, data: dict[str, Any]):
        print(f"  📤 Event: {event_type} for run {run_id}")

    # Apply mocks
    worker._register_with_control_plane = mock_register
    worker._poll_for_work = mock_poll
    worker._heartbeat = mock_heartbeat
    worker._send_event = mock_send_event

    # Also mock execute methods since we're not using real control plane
    _original_execute_llm = worker._execute_llm_node
    _original_execute_tool = worker._execute_tool_node

    async def mock_execute_llm(node_meta, state):
        return state

    async def mock_execute_tool(node_meta, state):
        return state

    worker._execute_llm_node = mock_execute_llm
    worker._execute_tool_node = mock_execute_tool


async def demo_graceful_shutdown():
    """Demonstrate graceful shutdown behavior."""

    print("=" * 60)
    print("Worker Lifecycle Demo - Graceful Shutdown")
    print("=" * 60)
    print()
    print("This demo shows:")
    print("1. Worker heartbeat every 5 seconds")
    print("2. Concurrent task execution")
    print("3. Graceful shutdown on Ctrl+C")
    print("4. Draining in-progress tasks before exit")
    print()
    print("Press Ctrl+C to trigger graceful shutdown")
    print("=" * 60)
    print()

    # Create worker with shorter intervals for demo
    worker = Worker(
        control_plane_url="http://localhost:8080",
        name="demo-worker",
        heartbeat_interval=5.0,  # Shorter for demo
        poll_interval=0.5,  # Faster polling for demo
        max_concurrent_runs=3,
        shutdown_timeout=30.0,
    )

    # Register our graphs
    long_task = LongRunningTask()
    quick_task = QuickTask()

    worker.register_graph(long_task._get_definition())
    worker.register_graph(quick_task._get_definition())

    # Create mock control plane
    control_plane = DemoControlPlane()

    # Set up the simulation
    await simulate_control_plane(worker, control_plane)

    # Add some work items
    print("📋 Adding work items to queue...")
    control_plane.add_work("run-001", "long_running_task", {"task_id": "001", "duration": 8})
    control_plane.add_work("run-002", "quick_task", {"task_id": "002"})
    control_plane.add_work("run-003", "long_running_task", {"task_id": "003", "duration": 6})
    control_plane.add_work("run-004", "quick_task", {"task_id": "004"})
    control_plane.add_work("run-005", "long_running_task", {"task_id": "005", "duration": 10})
    print("✅ Added 5 work items (3 long, 2 quick)\n")

    # Create a task to add more work periodically
    async def add_more_work():
        """Add more work items periodically."""
        run_counter = 6
        while worker._status not in (WorkerStatus.DRAINING, WorkerStatus.STOPPED):
            await asyncio.sleep(8)
            if worker._status not in (WorkerStatus.DRAINING, WorkerStatus.STOPPED):
                run_id = f"run-{run_counter:03d}"
                is_long = run_counter % 3 != 0
                if is_long:
                    control_plane.add_work(
                        run_id,
                        "long_running_task",
                        {"task_id": f"{run_counter:03d}", "duration": 5},
                    )
                    print(f"➕ Added new long task: {run_id}")
                else:
                    control_plane.add_work(run_id, "quick_task", {"task_id": f"{run_counter:03d}"})
                    print(f"➕ Added new quick task: {run_id}")
                run_counter += 1

    # Start the work adder
    work_adder = asyncio.create_task(add_more_work())

    try:
        # Run the worker
        print("🚀 Starting worker...\n")
        await worker.arun()
    except KeyboardInterrupt:
        print("\n⚠️  Interrupt received!")
        await worker._graceful_shutdown()
    finally:
        work_adder.cancel()
        try:
            await work_adder
        except asyncio.CancelledError:
            pass

    print("\n" + "=" * 60)
    print("Demo completed!")
    print("Final stats:")
    print(f"  - Heartbeats sent: {control_plane.heartbeat_count}")
    print(f"  - Runs completed: {worker._health_metrics['runs_completed']}")
    print(f"  - Runs failed: {worker._health_metrics['runs_failed']}")
    print(f"  - Final status: {worker._status.value}")
    print("=" * 60)


def main():
    """Run the demo."""
    try:
        asyncio.run(demo_graceful_shutdown())
    except KeyboardInterrupt:
        print("\nDemo interrupted by user")
        sys.exit(0)


if __name__ == "__main__":
    main()
