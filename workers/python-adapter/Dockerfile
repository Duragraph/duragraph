# syntax=docker/dockerfile:1

FROM python:3.11-slim AS base

# Install Poetry
RUN pip install --no-cache-dir poetry==1.7.1

WORKDIR /app

# Copy project files
COPY pyproject.toml poetry.lock* ./
RUN poetry install --no-root --only main

COPY duragraph_pyworker ./duragraph_pyworker

# Final minimal runtime
FROM python:3.11-slim AS runtime

RUN pip install --no-cache-dir poetry==1.7.1

WORKDIR /app

COPY --from=base /root/.cache/pypoetry /root/.cache/pypoetry
COPY pyproject.toml poetry.lock* ./
RUN poetry install --no-root --only main

COPY duragraph_pyworker ./duragraph_pyworker

ENTRYPOINT ["poetry", "run", "python", "-m", "duragraph_pyworker"]