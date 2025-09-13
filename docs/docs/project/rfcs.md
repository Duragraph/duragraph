# Request for Comments (RFC) Process

RFCs are used for proposing and discussing significant changes to Duragraph.  
They provide a structured way to capture design decisions, alternatives, and risks.

---

## When to Write an RFC

You should write an RFC when proposing:
- Changes to the **IR schema**.
- Additions or modifications to **API endpoints**.
- Major system architecture changes (e.g., worker protocols, data retention).
- New integrations with external systems (Temporal, storage, model APIs).

---

## RFC Template

Each RFC should include:

1. **Title / Identifier**  
   Example: `RFC-0001: Streaming tokens via SSE`

2. **Problem Statement**  
   Describe the problem and why it needs to be solved.

3. **Proposal**  
   Outline the proposed solution, including architecture diagrams, sequence diagrams, or schema changes.

4. **API / IR Changes**  
   Document API contracts, schema updates, and compatibility considerations.

5. **Migration / Rollout Plan**  
   How the change can be adopted incrementally, and fallback/rollback options.

6. **Risks & Open Questions**  
   Highlight potential risks and unanswered questions.

---

## Workflow

1. Draft RFC in a branch under `docs/rfcs/`.
2. Open a Pull Request with label `rfc`.
3. Discussion phase: request feedback from maintainers and community.
4. Once consensus is reached, merge RFC into main branch.
5. Link RFCs from `docs/project/rfcs.md`.

---

## References

- [Contributing Guide](contributing.md)  
- [ADRs](adrs.md)  

---