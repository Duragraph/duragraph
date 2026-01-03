# Mock worker Dockerfile for E2E testing
FROM python:3.12-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy source
COPY . .

# Environment defaults
ENV CONTROL_PLANE_URL=http://localhost:8081
ENV MOCK_WORKER_GRAPH=simple_echo
ENV MOCK_WORKER_DELAY_MS=100
ENV LOG_LEVEL=INFO

# Run worker
CMD ["python", "-m", "mock_worker.main"]
