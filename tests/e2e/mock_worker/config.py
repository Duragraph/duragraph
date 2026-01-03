"""Mock worker configuration."""

from pydantic_settings import BaseSettings
from pydantic import Field
from typing import Optional


class Config(BaseSettings):
    """Mock worker configuration from environment variables."""

    # Control plane connection
    control_plane_url: str = Field(
        default="http://localhost:8081",
        description="Control plane API URL",
    )

    # Worker identity
    worker_id: Optional[str] = Field(
        default=None,
        description="Worker ID (auto-generated if not provided)",
    )
    worker_name: str = Field(
        default="mock-worker",
        description="Worker name for identification",
    )

    # Graph configuration
    mock_graph: str = Field(
        default="simple_echo",
        alias="MOCK_WORKER_GRAPH",
        description="Which graph pattern to use",
    )
    mock_delay_ms: int = Field(
        default=100,
        alias="MOCK_WORKER_DELAY_MS",
        description="Delay between nodes in milliseconds",
    )
    mock_fail_at_node: Optional[str] = Field(
        default=None,
        alias="MOCK_WORKER_FAIL_AT_NODE",
        description="Node to fail at (for testing errors)",
    )
    mock_interrupt_at_node: Optional[str] = Field(
        default=None,
        alias="MOCK_WORKER_INTERRUPT_AT_NODE",
        description="Node to interrupt at (for testing human-in-loop)",
    )
    mock_token_count: int = Field(
        default=100,
        alias="MOCK_WORKER_TOKEN_COUNT",
        description="Simulated token count per LLM call",
    )

    # Worker behavior
    heartbeat_interval_seconds: int = Field(
        default=10,
        description="Heartbeat interval in seconds",
    )
    max_concurrent_runs: int = Field(
        default=5,
        description="Maximum concurrent runs this worker handles",
    )

    # Logging
    log_level: str = Field(
        default="INFO",
        description="Log level",
    )

    model_config = {
        "env_prefix": "",
        "env_file": ".env",
        "extra": "ignore",
    }


config = Config()
