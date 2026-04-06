"""Tests for the DSPy integration.

All tests mock DSPy modules so no LLM API keys are needed.
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from duragraph.dspy.module import (
    DspyNodeConfig,
    _parse_signature_fields,
    build_dspy_module,
    execute_dspy_module,
)
from duragraph.nodes import NodeMetadata, dspy_node


class TestParseSignatureFields:
    def test_simple_signature(self):
        inputs, outputs = _parse_signature_fields("question -> answer")
        assert inputs == ["question"]
        assert outputs == ["answer"]

    def test_multiple_fields(self):
        inputs, outputs = _parse_signature_fields(
            "context, question -> answer, confidence"
        )
        assert inputs == ["context", "question"]
        assert outputs == ["answer", "confidence"]

    def test_typed_fields(self):
        inputs, outputs = _parse_signature_fields(
            "sentence -> sentiment: bool, confidence: float"
        )
        assert inputs == ["sentence"]
        assert outputs == ["sentiment", "confidence"]

    def test_list_typed_fields(self):
        inputs, outputs = _parse_signature_fields(
            "context: list[str], question: str -> answer: str"
        )
        assert inputs == ["context", "question"]
        assert outputs == ["answer"]

    def test_invalid_signature_no_arrow(self):
        with pytest.raises(ValueError, match="Invalid DSPy signature"):
            _parse_signature_fields("question answer")


class TestDspyNodeConfig:
    def test_defaults(self):
        config = DspyNodeConfig(signature="q -> a")
        assert config.module == "ChainOfThought"
        assert config.temperature == 0.7
        assert config.max_tokens is None
        assert config.tools == []
        assert config.input_map is None
        assert config.output_map is None
        assert config.optimized_path is None

    def test_custom_values(self):
        config = DspyNodeConfig(
            signature="q -> a",
            module="ReAct",
            model="gpt-4o",
            temperature=0.3,
            max_tokens=1000,
            input_map={"q": "query"},
            output_map={"a": "answer"},
        )
        assert config.module == "ReAct"
        assert config.model == "gpt-4o"
        assert config.temperature == 0.3
        assert config.max_tokens == 1000


class TestBuildDspyModule:
    @patch("duragraph.dspy.module.dspy", create=True)
    def test_build_chain_of_thought(self, mock_dspy):
        mock_module = MagicMock()
        mock_dspy.ChainOfThought = MagicMock(return_value=mock_module)

        config = DspyNodeConfig(signature="question -> answer")

        with patch.dict("sys.modules", {"dspy": mock_dspy}):
            build_dspy_module(config)

        mock_dspy.ChainOfThought.assert_called_once_with(
            "question -> answer"
        )

    @patch("duragraph.dspy.module.dspy", create=True)
    def test_build_predict(self, mock_dspy):
        mock_module = MagicMock()
        mock_dspy.Predict = MagicMock(return_value=mock_module)

        config = DspyNodeConfig(signature="q -> a", module="Predict")

        with patch.dict("sys.modules", {"dspy": mock_dspy}):
            build_dspy_module(config)

        mock_dspy.Predict.assert_called_once_with("q -> a")

    @patch("duragraph.dspy.module.dspy", create=True)
    def test_build_react_with_tools(self, mock_dspy):
        mock_module = MagicMock()
        mock_dspy.ReAct = MagicMock(return_value=mock_module)

        def my_tool(x: str) -> str:
            return x

        config = DspyNodeConfig(
            signature="question -> answer",
            module="ReAct",
            tools=[my_tool],
        )

        with patch.dict("sys.modules", {"dspy": mock_dspy}):
            build_dspy_module(config)

        mock_dspy.ReAct.assert_called_once_with(
            signature="question -> answer",
            tools=[my_tool],
        )

    def test_build_unknown_module_raises(self):
        mock_dspy = MagicMock()
        mock_dspy.NonExistentModule = None
        delattr(mock_dspy, "NonExistentModule")

        config = DspyNodeConfig(
            signature="q -> a", module="NonExistentModule"
        )

        with patch.dict("sys.modules", {"dspy": mock_dspy}), pytest.raises(ValueError, match="Unknown DSPy module"):
            build_dspy_module(config)

    @patch("duragraph.dspy.module.dspy", create=True)
    def test_build_with_optimized_path(self, mock_dspy):
        mock_module = MagicMock()
        mock_dspy.ChainOfThought = MagicMock(return_value=mock_module)

        config = DspyNodeConfig(
            signature="q -> a",
            optimized_path="/tmp/optimized.json",
        )

        with patch.dict("sys.modules", {"dspy": mock_dspy}):
            build_dspy_module(config)

        mock_module.load.assert_called_once_with("/tmp/optimized.json")


class TestExecuteDspyModule:
    @pytest.mark.asyncio
    async def test_execute_basic(self):
        mock_module = MagicMock()
        mock_prediction = MagicMock()
        mock_prediction.answer = "Paris"
        mock_prediction.reasoning = "France's capital"
        mock_module.return_value = mock_prediction

        config = DspyNodeConfig(signature="question -> answer")
        state = {"question": "What is the capital of France?"}

        result = await execute_dspy_module(
            config, state, dspy_module=mock_module
        )

        mock_module.assert_called_once_with(
            question="What is the capital of France?"
        )
        assert result["answer"] == "Paris"
        assert result["reasoning"] == "France's capital"
        assert result["question"] == "What is the capital of France?"

    @pytest.mark.asyncio
    async def test_execute_with_input_map(self):
        mock_module = MagicMock()
        mock_prediction = MagicMock()
        mock_prediction.answer = "42"
        mock_prediction.reasoning = None
        mock_module.return_value = mock_prediction

        config = DspyNodeConfig(
            signature="question -> answer",
            input_map={"question": "user_query"},
        )
        state = {"user_query": "What is 6*7?"}

        result = await execute_dspy_module(
            config, state, dspy_module=mock_module
        )

        mock_module.assert_called_once_with(question="What is 6*7?")
        assert result["answer"] == "42"

    @pytest.mark.asyncio
    async def test_execute_with_output_map(self):
        mock_module = MagicMock()
        mock_prediction = MagicMock()
        mock_prediction.answer = "Blue"
        mock_prediction.reasoning = None
        mock_module.return_value = mock_prediction

        config = DspyNodeConfig(
            signature="question -> answer",
            output_map={"answer": "response"},
        )
        state = {"question": "What color is the sky?"}

        result = await execute_dspy_module(
            config, state, dspy_module=mock_module
        )

        assert result["response"] == "Blue"
        assert "answer" not in result

    @pytest.mark.asyncio
    async def test_execute_multiple_outputs(self):
        mock_module = MagicMock()
        mock_prediction = MagicMock()
        mock_prediction.sentiment = True
        mock_prediction.confidence = 0.95
        mock_prediction.reasoning = None
        mock_module.return_value = mock_prediction

        config = DspyNodeConfig(
            signature="sentence -> sentiment: bool, confidence: float"
        )
        state = {"sentence": "I love DuraGraph!"}

        result = await execute_dspy_module(
            config, state, dspy_module=mock_module
        )

        assert result["sentiment"] is True
        assert result["confidence"] == 0.95

    @pytest.mark.asyncio
    async def test_execute_missing_input_skipped(self):
        mock_module = MagicMock()
        mock_prediction = MagicMock()
        mock_prediction.answer = "Fallback"
        mock_prediction.reasoning = None
        mock_module.return_value = mock_prediction

        config = DspyNodeConfig(
            signature="context, question -> answer"
        )
        state = {"question": "What?"}

        result = await execute_dspy_module(
            config, state, dspy_module=mock_module
        )

        mock_module.assert_called_once_with(question="What?")
        assert result["answer"] == "Fallback"

    @pytest.mark.asyncio
    async def test_execute_preserves_existing_state(self):
        mock_module = MagicMock()
        mock_prediction = MagicMock()
        mock_prediction.answer = "Yes"
        mock_prediction.reasoning = None
        mock_module.return_value = mock_prediction

        config = DspyNodeConfig(signature="question -> answer")
        state = {"question": "Is this preserved?", "existing_key": "keep_me"}

        result = await execute_dspy_module(
            config, state, dspy_module=mock_module
        )

        assert result["existing_key"] == "keep_me"
        assert result["answer"] == "Yes"


class TestDspyNodeDecorator:
    def test_creates_node_descriptor(self):
        @dspy_node("question -> answer")
        async def my_node(self, state):
            return state

        assert hasattr(my_node, "metadata")
        assert my_node.metadata.node_type == "dspy"
        assert my_node.metadata.config["signature"] == "question -> answer"
        assert my_node.metadata.config["module"] == "ChainOfThought"

    def test_custom_module(self):
        @dspy_node("question -> answer", module="Predict")
        async def my_node(self, state):
            return state

        assert my_node.metadata.config["module"] == "Predict"

    def test_react_with_tools(self):
        def search(q: str) -> str:
            return q

        @dspy_node(
            "question -> answer",
            module="ReAct",
            tools=[search],
        )
        async def research(self, state):
            return state

        assert research.metadata.config["module"] == "ReAct"
        assert len(research.metadata.config["tools"]) == 1

    def test_custom_name(self):
        @dspy_node("q -> a", name="classifier")
        async def classify(self, state):
            return state

        assert classify.metadata.name == "classifier"

    def test_model_override(self):
        @dspy_node("q -> a", model="gpt-4o", temperature=0.3)
        async def analyze(self, state):
            return state

        assert analyze.metadata.config["model"] == "gpt-4o"
        assert analyze.metadata.config["temperature"] == 0.3

    def test_input_output_maps(self):
        @dspy_node(
            "question -> answer",
            input_map={"question": "user_input"},
            output_map={"answer": "response"},
        )
        async def mapped(self, state):
            return state

        assert mapped.metadata.config["input_map"] == {
            "question": "user_input"
        }
        assert mapped.metadata.config["output_map"] == {
            "answer": "response"
        }

    def test_sync_function(self):
        @dspy_node("q -> a")
        def sync_node(self, state):
            return state

        assert sync_node.metadata.is_async is False

    def test_async_function(self):
        @dspy_node("q -> a")
        async def async_node(self, state):
            return state

        assert async_node.metadata.is_async is True

    def test_optimized_path(self):
        @dspy_node("q -> a", optimized_path="/tmp/opt.json")
        async def opt_node(self, state):
            return state

        assert opt_node.metadata.config["optimized_path"] == "/tmp/opt.json"


class TestNodeMetadataDspyType:
    def test_metadata_node_type(self):
        meta = NodeMetadata(
            node_type="dspy",
            name="test",
            config={"signature": "q -> a"},
        )
        assert meta.node_type == "dspy"


class TestExecutorIntegration:
    @pytest.mark.asyncio
    async def test_execute_node_dispatches_dspy(self):
        from duragraph.executor import execute_node

        metadata = NodeMetadata(
            node_type="dspy",
            name="test_node",
            config={
                "signature": "question -> answer",
                "module": "ChainOfThought",
                "model": None,
                "temperature": 0.7,
                "max_tokens": None,
                "tools": [],
                "input_map": None,
                "output_map": None,
                "optimized_path": None,
            },
        )

        mock_prediction = MagicMock()
        mock_prediction.answer = "42"
        mock_prediction.reasoning = None

        async def node_method(state):
            return state

        with patch(
            "duragraph.executor.execute_dspy_node"
        ) as mock_exec:
            mock_exec.return_value = {"question": "test", "answer": "42"}
            result = await execute_node(
                "test_node", metadata, node_method, {"question": "test"}
            )
            mock_exec.assert_called_once()
            assert result["answer"] == "42"
