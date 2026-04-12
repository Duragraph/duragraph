"""DSPy module construction and execution for DuraGraph nodes."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass
class DspyNodeConfig:
    """Configuration for a ``@dspy_node()``-decorated method.

    Attributes:
        signature: DSPy signature string (e.g. ``"question -> answer"``).
        module: DSPy module class name (``"Predict"``, ``"ChainOfThought"``,
            ``"ReAct"``, ``"ProgramOfThought"``).
        model: LM model identifier. When set, a per-node ``dspy.LM`` is
            created; otherwise the globally-configured LM is used.
        temperature: Sampling temperature.
        max_tokens: Maximum generation tokens.
        tools: Tool callables for ``ReAct`` modules.
        input_map: Mapping from state keys to DSPy signature input field
            names.  ``None`` means auto-detect from the signature.
        output_map: Mapping from DSPy output field names to state keys.
            ``None`` means output fields are written to state as-is.
        optimized_path: Path to a saved, optimised DSPy module state
            (produced by ``module.save(path)``).  Loaded at build time.
    """

    signature: str
    module: str = "ChainOfThought"
    model: str | None = None
    temperature: float = 0.7
    max_tokens: int | None = None
    tools: list[Any] = field(default_factory=list)
    input_map: dict[str, str] | None = None
    output_map: dict[str, str] | None = None
    optimized_path: str | None = None


def _parse_signature_fields(signature: str) -> tuple[list[str], list[str]]:
    """Parse input/output field names from a DSPy inline signature.

    Args:
        signature: e.g. ``"context, question -> answer"``

    Returns:
        Tuple of (input_field_names, output_field_names).
    """
    parts = signature.split("->")
    if len(parts) != 2:
        raise ValueError(f"Invalid DSPy signature '{signature}': expected 'inputs -> outputs'")

    def _field_names(raw: str) -> list[str]:
        names: list[str] = []
        for token in raw.split(","):
            token = token.strip()
            if not token:
                continue
            name = token.split(":")[0].strip()
            names.append(name)
        return names

    return _field_names(parts[0]), _field_names(parts[1])


def build_dspy_module(config: DspyNodeConfig) -> Any:
    """Construct a DSPy module instance from :class:`DspyNodeConfig`.

    Returns a callable DSPy module (e.g. ``dspy.ChainOfThought``).
    """
    try:
        import dspy
    except ImportError as exc:
        raise ImportError(
            "dspy is required for @dspy_node(). Install it with: pip install duragraph[dspy]"
        ) from exc

    module_cls_name = config.module
    module_cls = getattr(dspy, module_cls_name, None)
    if module_cls is None:
        raise ValueError(f"Unknown DSPy module: {module_cls_name}")

    kwargs: dict[str, Any] = {}
    if config.temperature != 0.7:
        kwargs["temperature"] = config.temperature
    if config.max_tokens is not None:
        kwargs["max_tokens"] = config.max_tokens

    if module_cls_name == "ReAct":
        module = module_cls(
            signature=config.signature,
            tools=config.tools,
            **kwargs,
        )
    else:
        module = module_cls(config.signature, **kwargs)

    if config.optimized_path:
        module.load(config.optimized_path)

    return module


async def execute_dspy_module(
    config: DspyNodeConfig,
    state: dict[str, Any],
    *,
    dspy_module: Any | None = None,
) -> dict[str, Any]:
    """Execute a DSPy module against the current graph state.

    1. Extract input fields from *state* using *config.input_map* (or
       auto-detect from the signature).
    2. Call the DSPy module.
    3. Write output fields back to *state* using *config.output_map*.

    Args:
        config: The DSPy node configuration.
        state: Current graph state dict.
        dspy_module: Pre-built DSPy module. Built on-the-fly if ``None``.

    Returns:
        Updated state dict with DSPy outputs merged in.
    """
    if dspy_module is None:
        dspy_module = build_dspy_module(config)

    input_fields, output_fields = _parse_signature_fields(config.signature)

    input_map = config.input_map or {f: f for f in input_fields}
    output_map = config.output_map or {f: f for f in output_fields}

    call_kwargs: dict[str, Any] = {}
    for sig_field, state_key in input_map.items():
        if state_key in state:
            call_kwargs[sig_field] = state[state_key]

    lm_context: Any = None
    if config.model:
        try:
            import dspy

            lm = dspy.LM(
                config.model if "/" in config.model else f"openai/{config.model}",
                temperature=config.temperature,
                max_tokens=config.max_tokens or 4096,
            )
            lm_context = dspy.context(lm=lm)
        except Exception:
            lm_context = None

    if lm_context is not None:
        with lm_context:
            prediction = dspy_module(**call_kwargs)
    else:
        prediction = dspy_module(**call_kwargs)

    result = dict(state)
    for sig_field, state_key in output_map.items():
        value = getattr(prediction, sig_field, None)
        if value is not None:
            result[state_key] = value

    if hasattr(prediction, "reasoning"):
        result.setdefault("reasoning", prediction.reasoning)

    return result
