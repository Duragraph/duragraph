"""DSPy integration for DuraGraph.

Provides a bridge between DSPy modules and DuraGraph graph nodes,
allowing declarative LM programs (Predict, ChainOfThought, ReAct, etc.)
to be used as first-class graph nodes.

Requires: ``pip install duragraph[dspy]``
"""

from duragraph.dspy.bridge import DuraGraphLM, configure_from_provider
from duragraph.dspy.module import (
    DspyNodeConfig,
    build_dspy_module,
    execute_dspy_module,
)

__all__ = [
    "DuraGraphLM",
    "DspyNodeConfig",
    "build_dspy_module",
    "configure_from_provider",
    "execute_dspy_module",
]
