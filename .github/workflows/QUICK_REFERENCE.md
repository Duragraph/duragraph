# GitHub Workflows - Quick Reference Card

## 🚀 One-Page Reference

### Current Workflows

```
✅ ci.yml             - Main CI (tests, linting, build docs)
✅ conformance.yml    - LangGraph API compatibility tests ⭐
✅ contracts.yml      - API contract validation (OpenAPI, IR)
✅ release-please.yml - Automated releases & changelog

🆕 duragraph.yml      - Docker images (api, dashboard)
🆕 docs.yml           - Docs & website → Cloudflare Pages
🆕 devcontainer.yml   - Devcontainer image
```

---

### Workflow Triggers

| Workflow | Push Main | PR | Tags | Manual |
|----------|-----------|-----|------|--------|
| ci | ✅ | ✅ | - | - |
| conformance | ✅ | ✅ | - | - |
| contracts | ✅ | ✅ | - | - |
| duragraph | ✅ | ✅ | ✅ | - |
| docs | ✅ | ✅ | - | - |
| devcontainer | ✅ | ✅ | - | ✅ |
| release-please | ✅ | - | - | - |

---

### Secrets Required

```bash
# GitHub: Settings → Secrets → Actions

CLOUDFLARE_API_TOKEN=...      # For docs.yml
CLOUDFLARE_ACCOUNT_ID=...     # For docs.yml

# GITHUB_TOKEN is automatic (for duragraph.yml, devcontainer.yml)
```

---

### Published Artifacts

**Docker Images** (ghcr.io):
```
ghcr.io/YOUR_USERNAME/duragraph/api:latest
ghcr.io/YOUR_USERNAME/duragraph/dashboard:latest
ghcr.io/YOUR_USERNAME/duragraph/devcontainer:latest
```

**Documentation**:
```
https://duragraph-docs.pages.dev
```

---

### Quick Commands

```bash
# Setup Cloudflare secrets
# Go to: Repository → Settings → Secrets → New

# Test locally with Act
task act:setup
task act:ci

# Run conformance tests
task conformance

# View all tasks
task --list
```

---

### Commit Message Format (for releases)

```bash
feat: add feature      # → 1.0.0 to 1.1.0 (minor)
fix: fix bug           # → 1.0.0 to 1.0.1 (patch)
feat!: breaking change # → 1.0.0 to 2.0.0 (major)
docs: update docs      # → no version bump
chore: maintenance     # → no version bump
```

---

### Status Badges

```markdown
![CI](https://github.com/YOUR_USERNAME/duragraph/actions/workflows/ci.yml/badge.svg)
![Conformance](https://github.com/YOUR_USERNAME/duragraph/actions/workflows/conformance.yml/badge.svg)
![Docker](https://github.com/YOUR_USERNAME/duragraph/actions/workflows/duragraph.yml/badge.svg)
![Docs](https://github.com/YOUR_USERNAME/duragraph/actions/workflows/docs.yml/badge.svg)
```

---

### Troubleshooting

| Problem | Solution |
|---------|----------|
| Cloudflare fails | Check API token & account ID |
| Docker build fails | Check Dockerfile paths |
| Tests timeout | Increase wait time in workflow |
| Secret not found | Verify secret name (case-sensitive) |

---

### Documentation

- **Setup Guide**: [WORKFLOWS_SETUP.md](../WORKFLOWS_SETUP.md)
- **Full Reference**: [workflows/README.md](README.md)
- **Summary**: [../../WORKFLOWS_SUMMARY.md](../../WORKFLOWS_SUMMARY.md)
- **Act Guide**: [../../README.act.md](../../README.act.md)

---

**Last Updated**: 2025-11-22
