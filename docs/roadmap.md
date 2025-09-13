# Duragraph Roadmap

This roadmap outlines our planned milestones and priorities across the next three quarters.  
Each item links to a placeholder GitHub issue to track progress and discussion.  

---

## Q0 – Foundation & Smoke Test
- [x] [API parity MVP (runs, stream, simple workflows)](https://github.com/org/repo/issues/1)
- [x] [Initial hardcoded translator workflow](https://github.com/org/repo/issues/2)
- [x] [Bridge integration & smoke test SSE events](https://github.com/org/repo/issues/3)

## Q1 – Adapters & Extensibility
- [ ] [Python Eino adapter worker](https://github.com/org/repo/issues/4)
- [ ] [Shared UI tokens package](https://github.com/org/repo/issues/5)
- [ ] [Dashboard skeleton (SvelteKit + Tailwind)](https://github.com/org/repo/issues/6)

## Q2 – Reliability & Integrations
- [ ] [Webhooks delivery for project events](https://github.com/org/repo/issues/7)
- [ ] [Support continue-as-new runs](https://github.com/org/repo/issues/8)
- [ ] [Human-in-the-loop intervention support](https://github.com/org/repo/issues/9)

## Q3 – Data & Growth
- [ ] [Importer for external project definitions](https://github.com/org/repo/issues/10)
- [ ] [Expanded dashboard features (charts, tables, settings)](https://github.com/org/repo/issues/11)
- [ ] [Docs website & versioned docs with `mike`](https://github.com/org/repo/issues/12)

---

## Out of Scope
- Running untrusted user code inline in the API servers.
- Providing SLA-backed hosted services (focus is open source, self-host first).
- Proprietary model integrations (focus is on open schema, not locking into one vendor).

---

## Risks and Mitigations
| Risk | Mitigation |
|------|------------|
| Complexity of bridging runtime to different SDKs | Start with hardcoded translator + Python Eino adapter; modularize before adding others |
| Stability of early APIs | Use semantic versioning and release-please to manage changes clearly |
| Adoption dependent on docs and examples | Prioritize early docs (getting started, examples) and add versioned docs via mike |
| Webhook & integrations reliability | Implement retries, error logs, and observability early |

---