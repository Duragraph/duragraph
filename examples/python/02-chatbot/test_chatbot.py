"""
Tests for the chatbot example.

Tests core logic without requiring a running control plane.
"""

import pytest
from main import ConversationStore, ChatbotWithMemory, conversation_store


class TestConversationStore:
    def test_empty_store(self):
        store = ConversationStore()
        assert store.get_messages("test") == []

    def test_add_and_get_messages(self):
        store = ConversationStore()
        store.add_message("t1", "user", "Hello")
        store.add_message("t1", "assistant", "Hi!")

        messages = store.get_messages("t1")
        assert len(messages) == 2
        assert messages[0] == {"role": "user", "content": "Hello"}
        assert messages[1] == {"role": "assistant", "content": "Hi!"}

    def test_thread_isolation(self):
        store = ConversationStore()
        store.add_message("t1", "user", "From thread 1")
        store.add_message("t2", "user", "From thread 2")

        assert len(store.get_messages("t1")) == 1
        assert len(store.get_messages("t2")) == 1
        assert store.get_messages("t1")[0]["content"] == "From thread 1"

    def test_returns_copy(self):
        store = ConversationStore()
        store.add_message("t1", "user", "Hello")

        copy = store.get_messages("t1")
        copy.append({"role": "assistant", "content": "extra"})
        assert len(store.get_messages("t1")) == 1


class TestChatbotGraph:
    @pytest.fixture
    def chatbot(self):
        return ChatbotWithMemory()

    @pytest.mark.asyncio
    async def test_load_history(self, chatbot):
        tid = "test_load"
        conversation_store.add_message(tid, "user", "Prior msg")

        state = await chatbot.load_history({"thread_id": tid})
        assert len(state["messages"]) >= 1
        assert state["messages"][-1]["content"] == "Prior msg"

        conversation_store._store.pop(tid, None)

    @pytest.mark.asyncio
    async def test_add_user_message(self, chatbot):
        state = {"input": "Hello!", "messages": []}
        result = await chatbot.add_user_message(state)
        assert len(result["messages"]) == 1
        assert result["messages"][0]["role"] == "user"

    @pytest.mark.asyncio
    async def test_add_user_message_empty(self, chatbot):
        state = {"input": "", "messages": []}
        result = await chatbot.add_user_message(state)
        assert len(result["messages"]) == 0

    @pytest.mark.asyncio
    async def test_generate_response_greeting(self, chatbot):
        state = {"messages": [{"role": "user", "content": "Hello"}]}
        result = await chatbot.generate_response(state)
        assert "response" in result
        assert len(result["response"]) > 0

    @pytest.mark.asyncio
    async def test_generate_response_empty(self, chatbot):
        state = {"messages": []}
        result = await chatbot.generate_response(state)
        assert "response" in result

    @pytest.mark.asyncio
    async def test_save_response(self, chatbot):
        tid = "test_save"
        state = {
            "thread_id": tid,
            "input": "Test",
            "response": "Reply",
            "messages": [],
        }
        result = await chatbot.save_response(state)

        assert len(result["messages"]) == 1
        assert result["messages"][0]["role"] == "assistant"

        stored = conversation_store.get_messages(tid)
        assert len(stored) == 2

        conversation_store._store.pop(tid, None)


class TestIntegration:
    @pytest.mark.asyncio
    async def test_full_conversation(self):
        chatbot = ChatbotWithMemory()
        tid = "integration"

        state = {"thread_id": tid, "input": "Hello!", "messages": []}
        state = await chatbot.load_history(state)
        state = await chatbot.add_user_message(state)
        state = await chatbot.generate_response(state)
        state = await chatbot.save_response(state)

        assert len(state["messages"]) == 2
        assert state["messages"][0]["role"] == "user"
        assert state["messages"][1]["role"] == "assistant"

        state2 = {"thread_id": tid, "input": "How are you?", "messages": []}
        state2 = await chatbot.load_history(state2)
        assert len(state2["messages"]) == 2

        conversation_store._store.pop(tid, None)
