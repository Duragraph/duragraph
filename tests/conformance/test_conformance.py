import pytest
import requests
import os

BASE_URL = os.environ.get("API_BASE_URL", "http://localhost:8081/api/v1")


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
        """Create a run using LangGraph-compatible path: POST /threads/{thread_id}/runs"""
        data = {
            "assistant_id": assistant_id,
            "input": input_data or {"message": "hello"},
        }
        r = requests.post(f"{self.base_url}/threads/{thread_id}/runs", json=data)
        if r.status_code == 501:
            pytest.skip("Runs API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def get_run(self, thread_id, run_id):
        """Get a run using LangGraph-compatible path: GET /threads/{thread_id}/runs/{run_id}"""
        r = requests.get(f"{self.base_url}/threads/{thread_id}/runs/{run_id}")
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


def test_get_run():
    """Test getting run status"""
    client = APIClient()

    # Setup
    assistant = client.create_assistant()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    # Create and get run
    run = client.start_run(assistant["assistant_id"], thread_id)
    run_id = run["run_id"]

    # Get run (status may be queued, in_progress, completed, or failed)
    run_data = client.get_run(thread_id, run_id)
    assert run_data["run_id"] == run_id
    assert run_data["status"] in ["queued", "in_progress", "completed", "failed", "success", "error"]


# ============== Search and Count Tests ==============

def test_search_assistants():
    """Test searching for assistants"""
    client = APIClient()

    # Create a few assistants with metadata
    client.create_assistant({
        "name": "search-test-1",
        "model": "gpt-4",
        "metadata": {"category": "test"}
    })
    client.create_assistant({
        "name": "search-test-2",
        "model": "gpt-4",
        "metadata": {"category": "test"}
    })

    # Search for assistants
    r = requests.post(f"{BASE_URL}/assistants/search", json={
        "metadata": {"category": "test"},
        "limit": 10
    })
    if r.status_code == 501:
        pytest.skip("Search API not implemented yet")
    r.raise_for_status()

    results = r.json()
    assert "assistants" in results or isinstance(results, list)


def test_count_assistants():
    """Test counting assistants"""
    r = requests.post(f"{BASE_URL}/assistants/count", json={})
    if r.status_code == 501:
        pytest.skip("Count API not implemented yet")
    r.raise_for_status()

    result = r.json()
    assert "count" in result
    assert result["count"] >= 0


def test_search_threads():
    """Test searching for threads"""
    client = APIClient()

    # Create threads with metadata
    r = requests.post(f"{BASE_URL}/threads", json={
        "metadata": {"test_type": "conformance"}
    })
    r.raise_for_status()

    # Search for threads
    r = requests.post(f"{BASE_URL}/threads/search", json={
        "metadata": {"test_type": "conformance"},
        "limit": 10
    })
    if r.status_code == 501:
        pytest.skip("Thread search not implemented yet")
    r.raise_for_status()

    results = r.json()
    assert "threads" in results or isinstance(results, list)


def test_count_threads():
    """Test counting threads"""
    r = requests.post(f"{BASE_URL}/threads/count", json={})
    if r.status_code == 501:
        pytest.skip("Thread count not implemented yet")
    r.raise_for_status()

    result = r.json()
    assert "count" in result
    assert result["count"] >= 0


# ============== Thread State Tests ==============

def test_get_thread_state():
    """Test getting thread state"""
    client = APIClient()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    r = requests.get(f"{BASE_URL}/threads/{thread_id}/state")
    if r.status_code == 501:
        pytest.skip("Thread state not implemented yet")
    if r.status_code == 404:
        pytest.skip("No state for new thread")
    r.raise_for_status()

    state = r.json()
    assert "values" in state


def test_update_thread_state():
    """Test updating thread state"""
    client = APIClient()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    r = requests.post(f"{BASE_URL}/threads/{thread_id}/state", json={
        "values": {"test_key": "test_value"}
    })
    if r.status_code == 501:
        pytest.skip("Thread state update not implemented yet")
    r.raise_for_status()


def test_get_thread_history():
    """Test getting thread history"""
    client = APIClient()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    r = requests.get(f"{BASE_URL}/threads/{thread_id}/history")
    if r.status_code == 501:
        pytest.skip("Thread history not implemented yet")
    r.raise_for_status()

    history = r.json()
    assert isinstance(history, list)


# ============== Assistant Versioning Tests ==============

def test_get_assistant_versions():
    """Test getting assistant versions"""
    client = APIClient()
    assistant = client.create_assistant()
    assistant_id = assistant["assistant_id"]

    r = requests.get(f"{BASE_URL}/assistants/{assistant_id}/versions")
    if r.status_code == 501:
        pytest.skip("Assistant versions not implemented yet")
    r.raise_for_status()

    versions = r.json()
    assert isinstance(versions, list)


def test_get_assistant_schemas():
    """Test getting assistant schemas"""
    client = APIClient()
    assistant = client.create_assistant()
    assistant_id = assistant["assistant_id"]

    r = requests.get(f"{BASE_URL}/assistants/{assistant_id}/schemas")
    if r.status_code == 501:
        pytest.skip("Assistant schemas not implemented yet")
    r.raise_for_status()

    schemas = r.json()
    assert "input_schema" in schemas or "state_schema" in schemas


# ============== Run Lifecycle Tests ==============

def test_list_runs():
    """Test listing runs for a thread"""
    client = APIClient()
    assistant = client.create_assistant()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    # Create a run first
    client.start_run(assistant["assistant_id"], thread_id)

    # List runs
    r = requests.get(f"{BASE_URL}/threads/{thread_id}/runs")
    if r.status_code == 501:
        pytest.skip("List runs not implemented yet")
    r.raise_for_status()

    runs = r.json()
    assert isinstance(runs, list) or "runs" in runs


def test_delete_run():
    """Test deleting a run"""
    client = APIClient()
    assistant = client.create_assistant()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    run = client.start_run(assistant["assistant_id"], thread_id)
    run_id = run["run_id"]

    r = requests.delete(f"{BASE_URL}/threads/{thread_id}/runs/{run_id}")
    if r.status_code == 501:
        pytest.skip("Delete run not implemented yet")
    # Accept 200, 204, 404 (not found), or 409 (conflict - run still in progress)
    assert r.status_code in [200, 204, 404, 409]


def test_stateless_run():
    """Test creating a stateless run (POST /runs)"""
    client = APIClient()
    assistant = client.create_assistant()

    r = requests.post(f"{BASE_URL}/runs", json={
        "assistant_id": assistant["assistant_id"],
        "input": {"message": "test"}
    })
    if r.status_code == 501:
        pytest.skip("Stateless runs not implemented yet")
    r.raise_for_status()

    run = r.json()
    assert "run_id" in run


# ============== Interrupt Tests ==============

def test_run_with_interrupt_before():
    """Test creating a run with interrupt_before"""
    client = APIClient()
    assistant = client.create_assistant()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    r = requests.post(f"{BASE_URL}/threads/{thread_id}/runs", json={
        "assistant_id": assistant["assistant_id"],
        "input": {"message": "test"},
        "interrupt_before": ["some_node"]
    })
    if r.status_code == 501:
        pytest.skip("Run with interrupt_before not implemented yet")
    r.raise_for_status()

    run = r.json()
    assert "run_id" in run


def test_run_with_interrupt_after():
    """Test creating a run with interrupt_after"""
    client = APIClient()
    assistant = client.create_assistant()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    r = requests.post(f"{BASE_URL}/threads/{thread_id}/runs", json={
        "assistant_id": assistant["assistant_id"],
        "input": {"message": "test"},
        "interrupt_after": ["some_node"]
    })
    if r.status_code == 501:
        pytest.skip("Run with interrupt_after not implemented yet")
    r.raise_for_status()

    run = r.json()
    assert "run_id" in run


# ============== System Endpoints Tests ==============

def test_system_ok():
    """Test /ok endpoint"""
    r = requests.get(f"{BASE_URL.replace('/api/v1', '')}/ok")
    if r.status_code == 404:
        pytest.skip("/ok endpoint not implemented")
    r.raise_for_status()
    result = r.json()
    assert result.get("ok") == True


def test_system_info():
    """Test /info endpoint"""
    r = requests.get(f"{BASE_URL.replace('/api/v1', '')}/info")
    if r.status_code == 404:
        pytest.skip("/info endpoint not implemented")
    r.raise_for_status()
    result = r.json()
    assert "version" in result


# ============== Thread Copy Tests ==============

def test_copy_thread():
    """Test copying a thread"""
    client = APIClient()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    r = requests.post(f"{BASE_URL}/threads/{thread_id}/copy", json={})
    if r.status_code == 501:
        pytest.skip("Thread copy not implemented yet")
    r.raise_for_status()

    result = r.json()
    assert "thread_id" in result
    assert result["thread_id"] != thread_id  # Should be a new thread


# ============== Delete Tests ==============

def test_delete_assistant():
    """Test deleting an assistant"""
    client = APIClient()
    assistant = client.create_assistant({"name": "to-delete", "model": "test"})
    assistant_id = assistant["assistant_id"]

    r = requests.delete(f"{BASE_URL}/assistants/{assistant_id}")
    if r.status_code == 501:
        pytest.skip("Delete assistant not implemented yet")
    assert r.status_code in [200, 204]

    # Verify it's deleted
    r = requests.get(f"{BASE_URL}/assistants/{assistant_id}")
    assert r.status_code == 404


def test_delete_thread():
    """Test deleting a thread"""
    client = APIClient()
    thread = client.create_thread()
    thread_id = thread["thread_id"]

    r = requests.delete(f"{BASE_URL}/threads/{thread_id}")
    if r.status_code == 501:
        pytest.skip("Delete thread not implemented yet")
    assert r.status_code in [200, 204]

    # Verify it's deleted
    r = requests.get(f"{BASE_URL}/threads/{thread_id}")
    assert r.status_code == 404
