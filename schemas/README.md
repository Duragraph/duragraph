# Schemas

This directory contains the formal specifications for DuraGraph's APIs and intermediate representations (IR).

## Contracts-First Development

DuraGraph follows a **contracts-first** approach: specifications are defined before implementing features. OpenAPI and JSON Schema files here serve as the canonical source of truth for API behaviors and data structures.

Advantages:
- Guarantees alignment between frontend, backend, and tooling.
- Enables code generation, validation, and conformance testing.
- Provides a stable contract for integrators and external developers.

## Versioning

- All schemas are versioned explicitly via a `version` field in the documents.
- Breaking changes increment the **major** version.
- Backward-compatible changes increment the **minor** version.
- Patches update documentation or schema metadata without changing behavior.

## Structure

- **openapi/**  
  Contains OpenAPI 3.1 specifications for HTTP APIs.  
  - `duragraph.yaml`: Defines endpoints for assistants, threads, messages, runs, events, and webhooks.

- **ir/**  
  Contains schema definitions for IR (Intermediate Representation) workflows.  
  - `ir.schema.json`: Draft 2020-12 JSON Schema for representing execution graphs.  
  - `examples/hello.json`: Minimal example IR showing input → llm_call → end.

## Usage

- Validate IR files with `ajv` or other JSON Schema validators.  
- Generate server stubs and SDK clients from OpenAPI specs.  
- Keep all schema modifications backward-compatible unless performing a major release.