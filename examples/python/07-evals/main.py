"""
DuraGraph Evals Example

Demonstrates:
- Building a test harness for evaluating graph outputs
- Defining test cases with expected outputs and assertions
- Running graphs against test suites and collecting metrics
- Pass/fail reporting with detailed failure information
"""

import os
import time
from dataclasses import dataclass, field
from typing import Any

from duragraph import Graph, entrypoint, node


# ---------------------------------------------------------------------------
# Graph under test
# ---------------------------------------------------------------------------

@Graph(id="classifier")
class Classifier:
    """A simple text classifier used as the evaluation target."""

    @entrypoint
    @node()
    async def classify(self, state: dict[str, Any]) -> dict[str, Any]:
        """Classify text into a category based on keywords."""
        text = state.get("input", "").lower()

        categories = {
            "technical": ["api", "code", "server", "database", "deploy", "bug", "error"],
            "billing": ["invoice", "payment", "charge", "refund", "subscription", "price"],
            "general": ["hello", "help", "thanks", "question", "info"],
        }

        scores: dict[str, int] = {}
        for category, keywords in categories.items():
            scores[category] = sum(1 for kw in keywords if kw in text)

        if max(scores.values()) == 0:
            state["category"] = "general"
            state["confidence"] = 0.5
        else:
            best = max(scores, key=lambda k: scores[k])
            state["category"] = best
            state["confidence"] = min(scores[best] / 3, 1.0)

        return state

    @node()
    async def format_response(self, state: dict[str, Any]) -> dict[str, Any]:
        """Format the classification result."""
        category = state.get("category", "unknown")
        confidence = state.get("confidence", 0.0)
        state["response"] = f"Category: {category} (confidence: {confidence:.0%})"
        return state

    classify >> format_response


# ---------------------------------------------------------------------------
# Evaluation framework
# ---------------------------------------------------------------------------

@dataclass
class TestCase:
    """A single evaluation test case."""

    name: str
    input: dict[str, Any]
    assertions: dict[str, Any]
    description: str = ""


@dataclass
class TestResult:
    """Result of running a single test case."""

    name: str
    passed: bool
    duration_ms: float
    failures: list[str] = field(default_factory=list)
    output: dict[str, Any] = field(default_factory=dict)


@dataclass
class EvalReport:
    """Aggregated evaluation report."""

    results: list[TestResult]
    total: int = 0
    passed: int = 0
    failed: int = 0
    duration_ms: float = 0.0

    def summary(self) -> str:
        lines = [
            f"Eval Report: {self.passed}/{self.total} passed "
            f"({self.passed / self.total * 100:.0f}%) in {self.duration_ms:.0f}ms",
            "",
        ]
        for r in self.results:
            status = "PASS" if r.passed else "FAIL"
            lines.append(f"  [{status}] {r.name} ({r.duration_ms:.0f}ms)")
            for f in r.failures:
                lines.append(f"         {f}")
        return "\n".join(lines)


def check_assertions(output: dict[str, Any], assertions: dict[str, Any]) -> list[str]:
    """Check output against expected assertions. Returns list of failure messages."""
    failures = []
    for key, expected in assertions.items():
        actual = output.get(key)

        if callable(expected):
            try:
                if not expected(actual):
                    failures.append(f"{key}: custom check failed (got {actual!r})")
            except Exception as e:
                failures.append(f"{key}: custom check raised {e}")

        elif isinstance(expected, dict) and "$gte" in expected:
            if actual is None or actual < expected["$gte"]:
                failures.append(f"{key}: expected >= {expected['$gte']}, got {actual!r}")

        elif isinstance(expected, dict) and "$contains" in expected:
            if expected["$contains"] not in str(actual):
                failures.append(f"{key}: expected to contain {expected['$contains']!r}, got {actual!r}")

        elif actual != expected:
            failures.append(f"{key}: expected {expected!r}, got {actual!r}")

    return failures


def run_eval(graph: Any, test_cases: list[TestCase]) -> EvalReport:
    """Run all test cases against a graph and produce a report."""
    results = []
    start_all = time.monotonic()

    for tc in test_cases:
        start = time.monotonic()
        run_result = graph.run(tc.input)
        duration = (time.monotonic() - start) * 1000

        output = run_result.output or {}
        failures = check_assertions(output, tc.assertions)

        results.append(TestResult(
            name=tc.name,
            passed=len(failures) == 0,
            duration_ms=duration,
            failures=failures,
            output=output,
        ))

    total_duration = (time.monotonic() - start_all) * 1000
    passed = sum(1 for r in results if r.passed)

    return EvalReport(
        results=results,
        total=len(results),
        passed=passed,
        failed=len(results) - passed,
        duration_ms=total_duration,
    )


# ---------------------------------------------------------------------------
# Test suite
# ---------------------------------------------------------------------------

TEST_CASES = [
    TestCase(
        name="technical_classification",
        description="Should classify API-related text as technical",
        input={"input": "The API server returns a database error on deploy"},
        assertions={
            "category": "technical",
            "confidence": {"$gte": 0.5},
            "response": {"$contains": "technical"},
        },
    ),
    TestCase(
        name="billing_classification",
        description="Should classify payment-related text as billing",
        input={"input": "I need a refund for my subscription payment"},
        assertions={
            "category": "billing",
            "confidence": {"$gte": 0.5},
            "response": {"$contains": "billing"},
        },
    ),
    TestCase(
        name="general_classification",
        description="Should classify greetings as general",
        input={"input": "Hello, I have a question"},
        assertions={
            "category": "general",
            "confidence": {"$gte": 0.5},
            "response": {"$contains": "general"},
        },
    ),
    TestCase(
        name="ambiguous_input",
        description="Should default to general for ambiguous text",
        input={"input": "something completely unrelated"},
        assertions={
            "category": "general",
            "confidence": lambda c: c == 0.5,
        },
    ),
    TestCase(
        name="response_format",
        description="Response should contain category and confidence",
        input={"input": "Deploy the code to the server"},
        assertions={
            "response": {"$contains": "Category:"},
        },
    ),
    TestCase(
        name="high_confidence_technical",
        description="Multiple technical keywords should yield high confidence",
        input={"input": "Fix the bug in the API code and deploy the server database"},
        assertions={
            "category": "technical",
            "confidence": {"$gte": 0.9},
        },
    ),
]


def main() -> None:
    agent = Classifier()

    print("=== DuraGraph Evals Example ===\n")
    report = run_eval(agent, TEST_CASES)
    print(report.summary())
    print()

    if report.failed > 0:
        print(f"{report.failed} test(s) failed.")
    else:
        print("All tests passed.")

    print()
    control_plane = os.getenv("DURAGRAPH_URL", "http://localhost:8081")
    print(f"=== Serving on {control_plane} ===")
    print("Press Ctrl+C to stop\n")
    agent.serve(control_plane, worker_name="classifier-worker")


if __name__ == "__main__":
    main()
