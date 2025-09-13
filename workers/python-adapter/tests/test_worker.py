import pytest
import asyncio
from duragraph_pyworker import __main__


@pytest.mark.asyncio
async def test_llm_call_returns_stub():
    args = {"prompt": "hi"}
    result = await __main__.llm_call(args)
    assert "response" in result
    assert result["response"] == "stub LLM output"


@pytest.mark.asyncio
async def test_tool_returns_stub():
    args = {"name": "echo", "input": {"x": 42}}
    result = await __main__.tool(args)
    assert result["tool"] == "echo"
    assert result["status"] == "completed"


def test_checkpoint_hooks_do_not_error(caplog):
    caplog.set_level("DEBUG")
    __main__.checkpoint_before("node1", {"foo": "bar"})
    __main__.checkpoint_after("node1", {"foo": "baz"})
    messages = [rec.message for rec in caplog.records]
    assert any("checkpoint before" in m for m in messages)
    assert any("checkpoint after" in m for m in messages)