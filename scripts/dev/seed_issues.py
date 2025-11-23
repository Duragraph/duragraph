#!/usr/bin/env python3
"""
Seed Issues Script

This script generates mock issue data for development and testing.
It outputs the issues to a JSON file by default, but can also print
to stdout if desired.

Usage:
    python scripts/dev/seed_issues.py --output issues.json
"""

import argparse
import json
import random
import sys
from datetime import datetime, timedelta

ISSUE_TITLES = [
    "Fix login bug",
    "Improve API performance",
    "Update documentation",
    "Refactor database models",
    "UI layout issue on mobile",
    "Add unit tests",
    "Implement caching",
    "Upgrade dependencies",
    "Accessibility improvements",
    "Search functionality not working"
]

ISSUE_STATUSES = ["open", "closed"]
ISSUE_PRIORITIES = ["low", "medium", "high"]


def generate_issue(index: int) -> dict:
    """Generate a single mock issue dictionary."""
    created_at = datetime.utcnow() - timedelta(days=random.randint(0, 365))
    closed_at = None
    status = random.choice(ISSUE_STATUSES)
    if status == "closed":
        closed_at = created_at + timedelta(days=random.randint(1, 60))

    return {
        "id": index,
        "title": random.choice(ISSUE_TITLES),
        "status": status,
        "priority": random.choice(ISSUE_PRIORITIES),
        "created_at": created_at.isoformat() + "Z",
        "closed_at": closed_at.isoformat() + "Z" if closed_at else None,
    }


def generate_issues(count: int) -> list:
    """Generate a list of mock issues."""
    return [generate_issue(i + 1) for i in range(count)]


def main():
    parser = argparse.ArgumentParser(description="Seed mock issue data for development.")
    parser.add_argument("--count", type=int, default=10, help="Number of issues to generate.")
    parser.add_argument("--output", type=str, default=None, help="Path to output JSON file.")
    args = parser.parse_args()

    issues = generate_issues(args.count)

    if args.output:
        with open(args.output, "w", encoding="utf-8") as f:
            json.dump(issues, f, indent=2)
        print(f"Generated {args.count} issues to {args.output}")
    else:
        json.dump(issues, sys.stdout, indent=2)


if __name__ == "__main__":
    main()
