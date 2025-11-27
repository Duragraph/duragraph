import pytest
import requests
import sseclient
import os

BASE_URL = os.environ.get("API_BASE_URL", "http://localhost:8080/api/v1")

class APIClient:
    def __init__(self, base_url=BASE_URL):
        self.base_url = base_url

    def create_assistant(self, payload=None):
        data = payload or {"name": "conformance-assistant", "model": "test"}
        r = requests.post(f"{self.base_url}/assistants", json=data)
        if r.status_code == 501:
            pytest.skip("Assistants API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def create_thread(self):
        r = requests.post(f"{self.base_url}/threads", json={})
        if r.status_code == 501:
            pytest.skip("Threads API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def create_message(self, thread_id, content="hello", role="user"):
        r = requests.post(f"{self.base_url}/threads/{thread_id}/messages",
                          json={"role": role, "content": content})
        if r.status_code == 501:
            pytest.skip("Messages API not implemented yet (501)")
        r.raise_for_status()
        return r.json()

    def start_run(self, assistant_id, thread_id, input_data=None):
        data = {
            "assistant_id": assistant_id,
            "thread_id": thread_id,
            "input": input_data or {"message": "hello"}
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

    def subscribe_stream(self, run_id):
        url = f"{self.base_url}/stream?run_id={run_id}"
        r = requests.get(url, stream=True)
        if r.status_code == 501:
            pytest.skip("Stream API not implemented yet (501)")
        r.raise_for_status()
        return sseclient.SSEClient(r)


@pytest.mark.conformance
def test_run_lifecycle():
    client = APIClient()

    # Create assistant
    assistant = client.create_assistant()
    assert "assistant_id" in assistant

    # Create thread and message
    thread = client.create_thread()
    assert "thread_id" in thread
    client.create_message(thread["thread_id"], "hello world")

    # Start run
    run = client.start_run(assistant["assistant_id"], thread["thread_id"], {"message": "hello world"})
    assert "run_id" in run
    run_id = run["run_id"]

    # Subscribe to stream and assert event order
    events = []
    sse_client = client.subscribe_stream(run_id)
    for event in sse_client.events():
        events.append(event.event)
        if event.event == "run_completed":
            break

    # Ensure order: run_started -> message_delta* -> run_completed
    assert events[0] == "run_started"
    assert events[-1] == "run_completed"
    assert any(ev == "message_delta" for ev in events)

    # Verify run completed with metadata
    run_data = client.get_run(run_id)
    assert run_data["status"] == "completed"
    assert "steps" in run_data
