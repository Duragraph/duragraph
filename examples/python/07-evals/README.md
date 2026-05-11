# Evals

Demonstrates how to build a test harness for evaluating DuraGraph graph outputs.

## What This Example Demonstrates

- Defining `TestCase` objects with inputs and assertions
- Running a graph against a test suite with `run_eval()`
- Assertion operators: exact match, `$gte`, `$contains`, custom callables
- Pass/fail reporting with timing metrics
- Using evals to validate graph behavior before deployment

## Prerequisites

- Python 3.11+

## Quick Start

1. **Run the evals:**

   > Always use `uv`. Never `pip install`, never `python -m venv`, never `source .venv/bin/activate`.

   ```bash
   DURAGRAPH_URL=http://localhost:18081 PYTHONUNBUFFERED=1 \
     uv run --with-editable ../../../python \
     python main.py
   ```

## Architecture

The example defines a `Classifier` graph as the evaluation target:

```
classify → format_response
```

The eval framework runs test cases against it:

```python
TestCase(
    name="technical_classification",
    input={"input": "The API server returns a database error"},
    assertions={
        "category": "technical",
        "confidence": {"$gte": 0.5},
        "response": {"$contains": "technical"},
    },
)
```

## Assertion Types

| Type | Syntax | Description |
|------|--------|-------------|
| Exact match | `"category": "technical"` | Value must equal expected |
| Greater/equal | `"confidence": {"$gte": 0.5}` | Numeric comparison |
| Contains | `"response": {"$contains": "text"}` | Substring check |
| Custom | `"confidence": lambda c: c > 0.3` | Arbitrary callable |

## Expected Output

```
Eval Report: 6/6 passed (100%) in <N>ms

  [PASS] technical_classification (<N>ms)
  [PASS] billing_classification (<N>ms)
  [PASS] general_classification (<N>ms)
  [PASS] ambiguous_input (<N>ms)
  [PASS] response_format (<N>ms)
  [PASS] high_confidence_technical (<N>ms)

All tests passed.
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `DURAGRAPH_URL` | `http://localhost:8081` | Control plane URL |

## Next Steps

- Extend the eval framework with custom assertion operators
- Add test cases for edge cases and error handling
- Integrate evals into CI/CD pipelines
- [01-hello-world](../01-hello-world) — Start from the basics
