"""
Tests for the chatbot example.

Since this is a worker example, we test the core logic without 
requiring a full DuraGraph deployment.
"""

import pytest
from unittest.mock import patch, MagicMock
from main import ConversationStore, ChatbotWithMemory


class TestConversationStore:
    """Test the conversation store functionality."""
    
    def test_empty_store(self):
        """Test store starts empty."""
        store = ConversationStore()
        messages = store.get_messages("test_thread")
        assert messages == []
    
    def test_add_and_get_messages(self):
        """Test adding and retrieving messages."""
        store = ConversationStore()
        
        # Add messages
        store.add_message("thread1", "user", "Hello")
        store.add_message("thread1", "assistant", "Hi there!")
        
        # Retrieve messages
        messages = store.get_messages("thread1")
        assert len(messages) == 2
        assert messages[0] == {"role": "user", "content": "Hello"}
        assert messages[1] == {"role": "assistant", "content": "Hi there!"}
    
    def test_thread_isolation(self):
        """Test that different threads have separate conversations."""
        store = ConversationStore()
        
        # Add to different threads
        store.add_message("thread1", "user", "Hello from thread 1")
        store.add_message("thread2", "user", "Hello from thread 2")
        
        # Check isolation
        messages1 = store.get_messages("thread1")
        messages2 = store.get_messages("thread2")
        
        assert len(messages1) == 1
        assert len(messages2) == 1
        assert messages1[0]["content"] == "Hello from thread 1"
        assert messages2[0]["content"] == "Hello from thread 2"
    
    def test_clear_conversation(self):
        """Test clearing conversation history."""
        store = ConversationStore()
        
        # Add messages
        store.add_message("test", "user", "Hello")
        assert len(store.get_messages("test")) == 1
        
        # Clear and check
        store.clear("test")
        assert len(store.get_messages("test")) == 0
    
    def test_get_messages_returns_copy(self):
        """Test that get_messages returns a copy, not reference."""
        store = ConversationStore()
        store.add_message("test", "user", "Hello")
        
        messages1 = store.get_messages("test")
        messages2 = store.get_messages("test")
        
        # Modify one copy
        messages1.append({"role": "assistant", "content": "Modified"})
        
        # Original should be unchanged
        assert len(messages2) == 1
        assert len(store.get_messages("test")) == 1


class TestChatbotGraph:
    """Test the chatbot graph nodes."""
    
    @pytest.fixture
    def chatbot(self):
        """Create chatbot instance for testing."""
        return ChatbotWithMemory()
    
    @pytest.fixture
    def sample_state(self):
        """Sample state for testing."""
        return {
            "thread_id": "test_thread",
            "input": "Hello there!",
            "messages": []
        }
    
    @pytest.mark.asyncio
    async def test_load_history(self, chatbot, sample_state):
        """Test loading conversation history."""
        # Add some history first
        conversation_store.add_message("test_thread", "user", "Previous message")
        
        # Load history
        result = await chatbot.load_history(sample_state)
        
        assert "messages" in result
        assert len(result["messages"]) >= 1
        assert result["messages"][-1]["content"] == "Previous message"
        
        # Cleanup
        conversation_store.clear("test_thread")
    
    @pytest.mark.asyncio
    async def test_add_user_message(self, chatbot, sample_state):
        """Test adding user message to conversation."""
        result = await chatbot.add_user_message(sample_state)
        
        assert "messages" in result
        assert len(result["messages"]) == 1
        assert result["messages"][0]["role"] == "user"
        assert result["messages"][0]["content"] == "Hello there!"
    
    @pytest.mark.asyncio
    async def test_add_user_message_empty_input(self, chatbot):
        """Test handling empty user input."""
        state = {"thread_id": "test", "input": "", "messages": []}
        result = await chatbot.add_user_message(state)
        
        # Should return unchanged state for empty input
        assert result == state
    
    @pytest.mark.asyncio
    async def test_generate_response(self, chatbot):
        """Test response generation."""
        state = {
            "thread_id": "test",
            "messages": [{"role": "user", "content": "Hello"}]
        }
        
        result = await chatbot.generate_response(state)
        
        assert "response" in result
        assert isinstance(result["response"], str)
        assert len(result["response"]) > 0
    
    @pytest.mark.asyncio
    async def test_generate_response_greeting(self, chatbot):
        """Test response to greeting."""
        state = {
            "messages": [{"role": "user", "content": "Hello"}]
        }
        
        result = await chatbot.generate_response(state)
        
        assert "hello" in result["response"].lower()
    
    @pytest.mark.asyncio
    async def test_generate_response_name_question(self, chatbot):
        """Test response to name question."""
        state = {
            "messages": [{"role": "user", "content": "What is your name?"}]
        }
        
        result = await chatbot.generate_response(state)
        
        assert "duragraph bot" in result["response"].lower()
    
    @pytest.mark.asyncio
    async def test_save_response(self, chatbot):
        """Test saving response to conversation store."""
        state = {
            "thread_id": "test_save",
            "input": "Test input",
            "response": "Test response",
            "messages": []
        }
        
        result = await chatbot.save_response(state)
        
        # Check that message was added to state
        assert len(result["messages"]) == 1
        assert result["messages"][0]["role"] == "assistant"
        assert result["messages"][0]["content"] == "Test response"
        
        # Check that it was saved to store
        stored_messages = conversation_store.get_messages("test_save")
        assert len(stored_messages) == 2  # user + assistant
        
        # Cleanup
        conversation_store.clear("test_save")
    
    @pytest.mark.asyncio
    async def test_save_response_empty_response(self, chatbot):
        """Test handling empty response."""
        state = {
            "thread_id": "test",
            "input": "Test",
            "response": "",
            "messages": []
        }
        
        result = await chatbot.save_response(state)
        
        # Should return unchanged state for empty response
        assert result == state


class TestIntegration:
    """Integration tests for the full workflow."""
    
    @pytest.mark.asyncio
    async def test_full_conversation_flow(self):
        """Test a complete conversation workflow."""
        chatbot = ChatbotWithMemory()
        thread_id = "integration_test"
        
        # Start with empty state
        state = {
            "thread_id": thread_id,
            "input": "Hello!",
            "messages": []
        }
        
        # Run through all nodes
        state = await chatbot.load_history(state)
        state = await chatbot.add_user_message(state)
        state = await chatbot.generate_response(state)
        state = await chatbot.save_response(state)
        
        # Verify final state
        assert len(state["messages"]) == 2  # user + assistant
        assert state["messages"][0]["role"] == "user"
        assert state["messages"][1]["role"] == "assistant"
        assert "response" in state
        
        # Verify persistence
        stored_messages = conversation_store.get_messages(thread_id)
        assert len(stored_messages) == 2
        
        # Second message in same thread
        state2 = {
            "thread_id": thread_id,
            "input": "How are you?",
            "messages": []
        }
        
        state2 = await chatbot.load_history(state2)
        # Should load previous conversation
        assert len(state2["messages"]) == 2
        
        # Cleanup
        conversation_store.clear(thread_id)


# Import the global store from main module
from main import conversation_store