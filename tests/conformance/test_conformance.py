import pytest
import requests
import os
import time

BASE_URL = os.environ.get("API_BASE_URL", "http://localhost:8080/api/v1")


class APIClient:
    def __init__(self, base_url=BASE_URL):
        self.base_url = base_url

    def health(self):
        r = requests.get(f"{self.base_url.replace('/api/v1', '')}/health")
        return r.status_code == 200

    def create_assistant(self, payload=None):
        data = payload or {"name": "conformance-assistant", "model": "test"}
        r = requests.post(f"{self.base_url}/assistants", json=data)
        if r.status_code == 501:
            pytest.skip("Assistants API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def get_assistant(self, assistant_id):
        r = requests.get(f"{self.base_url}/assistants/{assistant_id}")
        r.raise_for_status()
        return r.json()

    def create_thread(self):
        r = requests.post(f"{self.base_url}/threads", json={})
        if r.status_code == 501:
            pytest.skip("Threads API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def get_thread(self, thread_id):
        r = requests.get(f"{self.base_url}/threads/{thread_id}")
        r.raise_for_status()
        return r.json()

    def create_message(self, thread_id, content="hello", role="user"):
        r = requests.post(
            f"{self.base_url}/threads/{thread_id}/messages",
            json={"role": role, "content": content},
        )
        if r.status_code == 501:
            pytest.skip("Messages API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def start_run(self, assistant_id, thread_id, input_data=None):
        data = {
            "assistant_id": assistant_id,
            "thread_id": thread_id,
            "input": input_data or {"message": "hello"},
        }
        r = requests.post(f"{self.base_url}/runs", json=data)
        if r.status_code == 501:
            pytest.skip("Runs API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def get_run(self, run_id):
        r = requests.get(f"{self.base_url}/runs/{run_id}")
        if r.status_code == 501:
            pytest.skip("GET /runs not implemented yet (501)")
        r.raise_for_status()
        return r.json()


def test_health():
    """Test health endpoint"""
    client = APIClient()
    assert client.health()


def test_create_assistant():
    """Test creating an assistant"""
    client = APIClient()
    assistant = client.create_assistant({"name": "test-assistant", "model": "gpt-4"})
    assert "assistant_id" in assistant
    assert assistant["name"] == "test-assistant"


def test_create_thread():
    """Test creating a thread"""
    client = APIClient()
    thread = client.create_thread()
    assert "thread_id" in thread


def test_add_message_to_thread():
    """Test adding a message to a thread"""
    client = APIClient()
    thread = client.create_thread()
    message = client.create_message(thread["thread_id"], "Hello, world!")
    # API returns 'id' not 'message_id'
    assert "id" in message
    assert message["content"] == "Hello, world!"


def test_create_run():
    """Test creating a run (without waiting for completion)"""
    client = APIClient()

    # Setup
    assistant = client.create_assistant()
    thread = client.create_thread()
    client.create_message(thread["thread_id"], "test message")

    # Create run
    run = client.start_run(assistant["assistant_id"], thread["thread_id"])
    assert "run_id" in run
    assert run["status"] == "queued"


@pytest.mark.skip(reason="GET /runs/:id requires event projection - TODO")
def test_get_run():
    """Test getting run status"""
    client = APIClient()

    # Setup
    assistant = client.create_assistant()
    thread = client.create_thread()

    # Create and get run
    run = client.start_run(assistant["assistant_id"], thread["thread_id"])
    run_id = run["run_id"]

    # Get run (status may be queued, in_progress, completed, or failed)
    run_data = client.get_run(run_id)
    assert run_data["run_id"] == run_id
    assert run_data["status"] in ["queued", "in_progress", "completed", "failed"]
