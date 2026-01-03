#!/usr/bin/env python3
"""Mock worker entry point.

Starts the mock worker that connects to DuraGraph control plane,
registers available graphs, and executes runs as assigned.
"""

import asyncio
import signal
import sys

import structlog

from .config import config
from .worker import Worker
from .graphs import list_graphs

# Configure structured logging
structlog.configure(
    processors=[
        structlog.stdlib.filter_by_level,
        structlog.stdlib.add_logger_name,
        structlog.stdlib.add_log_level,
        structlog.stdlib.PositionalArgumentsFormatter(),
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.StackInfoRenderer(),
        structlog.processors.format_exc_info,
        structlog.processors.UnicodeDecoder(),
        structlog.dev.ConsoleRenderer() if sys.stdout.isatty() else structlog.processors.JSONRenderer(),
    ],
    wrapper_class=structlog.stdlib.BoundLogger,
    context_class=dict,
    logger_factory=structlog.stdlib.LoggerFactory(),
    cache_logger_on_first_use=True,
)

log = structlog.get_logger()


async def main():
    """Main entry point."""
    log.info(
        "Starting mock worker",
        control_plane=config.control_plane_url,
        graph=config.mock_graph,
        graphs_available=list_graphs(),
        delay_ms=config.mock_delay_ms,
        max_concurrent=config.max_concurrent_runs,
    )

    # Create worker
    worker = Worker(
        control_plane_url=config.control_plane_url,
        worker_name=config.worker_name,
    )

    # Handle shutdown signals
    loop = asyncio.get_running_loop()
    stop_event = asyncio.Event()

    def signal_handler():
        log.info("Received shutdown signal")
        stop_event.set()

    for sig in (signal.SIGTERM, signal.SIGINT):
        loop.add_signal_handler(sig, signal_handler)

    try:
        # Start worker
        await worker.start()

        # Wait for shutdown signal
        await stop_event.wait()

    except KeyboardInterrupt:
        log.info("Keyboard interrupt received")
    except Exception as e:
        log.error("Worker error", error=str(e))
        raise
    finally:
        # Stop worker gracefully
        await worker.stop()


def run():
    """Run the worker."""
    try:
        # Use uvloop if available for better performance
        try:
            import uvloop
            asyncio.set_event_loop_policy(uvloop.EventLoopPolicy())
            log.info("Using uvloop for event loop")
        except ImportError:
            pass

        asyncio.run(main())
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    run()
