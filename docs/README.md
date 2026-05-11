# DuraGraph Documentation

[![Deploy](https://github.com/Duragraph/duragraph/actions/workflows/docs-deploy.yml/badge.svg)](https://github.com/Duragraph/duragraph/actions/workflows/docs-deploy.yml)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Official documentation, blog, and landing page for DuraGraph — durable, replayable agent workflows.

**Live site:** [duragraph.ai](https://duragraph.ai)

## Tech Stack

- **Framework:** [Astro](https://astro.build/) 5
- **Docs:** [Starlight](https://starlight.astro.build/)
- **Styling:** TailwindCSS
- **Diagrams:** Mermaid
- **Deployment:** Cloudflare Pages

## Local Development

```bash
# Install dependencies
pnpm install

# Start dev server
pnpm dev

# Build for production
pnpm build

# Preview production build
pnpm preview

# Validate internal links
pnpm validate-links
```

## Project Structure

```
src/
├── assets/              # Images and logos
├── components/
│   └── landing/         # Landing page components
├── content/
│   ├── docs/            # Documentation (MDX)
│   └── blog/            # Blog posts (MDX)
├── pages/               # Custom pages
└── styles/              # Global styles
```

## Content

### Documentation

Add new docs in `src/content/docs/`:

```mdx
---
title: My Page
description: Page description for SEO
---

Content here...
```

### Blog Posts

Add new posts in `src/content/blog/`:

```mdx
---
title: My Post
date: 2025-01-01
authors:
  - name: Author Name
description: Post description
---

Content here...
```

## Related modules

The docs site source lives in the [`duragraph`](https://github.com/Duragraph/duragraph) monorepo alongside the engine and SDKs:

| Path                                                                     | Description               |
| ------------------------------------------------------------------------ | ------------------------- |
| [`/`](https://github.com/Duragraph/duragraph)                            | Core engine (Go)          |
| [`python/`](https://github.com/Duragraph/duragraph/tree/main/python)     | Python SDK                |
| [`go-sdk/`](https://github.com/Duragraph/duragraph/tree/main/go-sdk)     | Go SDK                    |
| [`studio/`](https://github.com/Duragraph/duragraph/tree/main/studio)     | Visual workflow editor    |
| [`examples/`](https://github.com/Duragraph/duragraph/tree/main/examples) | Runnable example projects |

## Contributing

See [CONTRIBUTING.md](https://github.com/Duragraph/duragraph/blob/main/CONTRIBUTING.md) for guidelines.

## License

Apache 2.0 - See [LICENSE](LICENSE) for details.
