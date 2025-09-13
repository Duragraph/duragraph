# Documentation

This directory contains the Markdown sources for the DuraGraph documentation site, built with [MkDocs Material](https://squidfunk.github.io/mkdocs-material/).

## Building the Docs

To preview the documentation locally:

```bash
pip install mkdocs-material mike mkdocs-minify-plugin mkdocs-git-revision-date-localized-plugin
mkdocs serve
```

This will launch a local dev server at `http://127.0.0.1:8000`.

## Versioning the Docs

We use [mike](https://github.com/jimporter/mike) to manage versioned documentation.

```bash
mike deploy 0.1 latest
mike set-default latest
```

This makes `0.1` available under `/0.1/` and sets `latest` as the default version.

## Plugins

Enabled plugins in `mkdocs.yml`:
- `search`
- `mike`
- `minify`
- `git-revision-date-localized`
- `renderers.openapi`

## Structure

Documentation is organized into sections:

- **Overview**
- **Getting Started** (includes Quickstart using Docker Compose)
- **Architecture** (system overview, control plane, Temporal design, IR spec, workers)
- **Operations** (observability, security, SLOs, runbooks)
- **Project** (roadmap, contributing, RFCs, ADR index)

Stub Markdown files are provided for each section. Expand them as development progresses.