#!/usr/bin/env python3
"""Validate IR example files against the IR JSON schema."""

import json
import sys
from pathlib import Path

try:
    import jsonschema
except ImportError:
    print("Error: jsonschema package not installed. Run: pip install jsonschema")
    sys.exit(1)


def main():
    # Find schema and examples
    repo_root = Path(__file__).parent.parent.parent
    schema_path = repo_root / "schemas" / "ir" / "ir.schema.json"
    examples_dir = repo_root / "schemas" / "ir" / "examples"

    if not schema_path.exists():
        print(f"Error: Schema not found at {schema_path}")
        sys.exit(1)

    # Load schema
    with open(schema_path) as f:
        schema = json.load(f)

    # Find all example files
    example_files = list(examples_dir.glob("*.json"))

    if not example_files:
        print(f"Warning: No example files found in {examples_dir}")
        sys.exit(0)

    print(f"Validating {len(example_files)} IR example(s) against schema...")

    errors = []
    for example_file in example_files:
        try:
            with open(example_file) as f:
                example = json.load(f)

            jsonschema.validate(example, schema)
            print(f"  ✓ {example_file.name}")
        except jsonschema.ValidationError as e:
            print(f"  ✗ {example_file.name}: {e.message}")
            errors.append((example_file.name, e.message))
        except json.JSONDecodeError as e:
            print(f"  ✗ {example_file.name}: Invalid JSON - {e}")
            errors.append((example_file.name, f"Invalid JSON: {e}"))

    if errors:
        print(f"\n{len(errors)} validation error(s) found.")
        sys.exit(1)

    print(f"\nAll {len(example_files)} example(s) validated successfully!")
    sys.exit(0)


if __name__ == "__main__":
    main()
