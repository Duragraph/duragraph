# GitHub Workflows

This directory contains all CI/CD workflows for DuraGraph.

## 🔄 Active Workflows

### Core CI/CD

#### [ci.yml](ci.yml)
**Main CI Pipeline** - Runs on every push and PR
- ✅ Pre-commit checks
- ✅ Go unit tests
- ✅ Builds docs and website

**Triggers**: Push to main/develop, Pull requests

---

#### [conformance.yml](conformance.yml) ⭐
**LangGraph API Conformance Tests** - Ensures API compatibility
- ✅ Tests that DuraGraph API works exactly like LangGraph Cloud
- ✅ Validates assistants, threads, messages, runs, streaming
- ✅ Verifies event order and API behavior

**Why it's critical**: This is your competitive advantage - "drop-in replacement for LangGraph Cloud"

**Triggers**: Push to main, Pull requests

---

#### [contracts.yml](contracts.yml)
**API Contract Validation** - Prevents breaking changes
- ✅ Lints OpenAPI spec with Spectral
- ✅ Compares your API against LangGraph reference spec
- ✅ Validates IR (Intermediate Representation) schemas

**Files validated**:
- `schemas/openapi/duragraph.yaml` - Your OpenAPI spec
- `tests/reference/langgraph-platform.json` - LangGraph reference
- `schemas/ir/ir.schema.json` - IR schema

**Triggers**: Push to main, Pull requests

---

### Docker & Deployment

#### [duragraph.yml](duragraph.yml) 🆕
**Docker Image Publishing** - Builds and publishes container images
- 🐳 Builds `api` and `dashboard` images
- 🏗️ Multi-architecture builds (amd64, arm64)
- 🔒 Security scanning with Trivy
- 📦 Publishes to GitHub Container Registry (ghcr.io)

**Images published**:
- `ghcr.io/duragraph/duragraph/api:latest`
- `ghcr.io/duragraph/duragraph/dashboard:latest`

**Tags**:
- `latest` - Latest from main branch
- `v1.2.3` - Semantic version tags
- `main-abc123` - Commit SHA tags
- `pr-123` - Pull request preview tags

**Triggers**: Push to main/develop, Tags (v*.*.*), Pull requests

---

#### [docs.yml](docs.yml) 🆕
**Documentation & Website Deployment** - Deploys to Cloudflare Pages
- 📚 Builds Fumadocs (Next.js) documentation
- 🌐 Builds Vite website (landing page)
- 🔗 Merges website into docs
- ☁️ Deploys to Cloudflare Pages

**Build process**:
1. Build website → `website/dist/`
2. Build docs → `docs/out/`
3. Copy website into `docs/out/landing/`
4. Deploy to Cloudflare Pages

**URLs**:
- Production: `https://duragraph-docs.pages.dev`
- Preview: `https://[branch-name].duragraph-docs.pages.dev`

**Triggers**: Push to main, Changes to `docs/**` or `website/**`

**Secrets required**:
- `CLOUDFLARE_API_TOKEN` - Cloudflare API token
- `CLOUDFLARE_ACCOUNT_ID` - Cloudflare account ID

---

#### [github-pages.yml](github-pages.yml) 🆕
**GitHub Pages Deployment** - Free hosting on github.io
- 📚 Builds Fumadocs (Next.js) documentation
- 🌐 Builds Vite website (landing page)
- 🔗 Merges website and docs into unified site
- 🚀 Deploys to GitHub Pages

**Build process**:
1. Build website → `website/dist/`
2. Build docs → `docs/out/`
3. Merge: `docs/` at root, `landing/` for website
4. Deploy to GitHub Pages via `actions/deploy-pages@v4`

**URLs**:
- Root: `https://yourusername.github.io/duragraph/` → Redirects to `/docs`
- Docs: `https://yourusername.github.io/duragraph/docs`
- Landing: `https://yourusername.github.io/duragraph/landing`

**Triggers**: Push to main, Changes to `docs/**` or `website/**`, Manual workflow dispatch

**Setup required**:
1. Enable GitHub Pages: Settings → Pages → Source: **GitHub Actions**
2. No secrets needed (uses automatic `GITHUB_TOKEN`)

**Local testing**:
```bash
task act:github-pages        # Full build test
task act:github-pages:dry    # Dry run
```

See [GITHUB_PAGES_QUICKSTART.md](GITHUB_PAGES_QUICKSTART.md) and [ACT_GITHUB_PAGES_GUIDE.md](../ACT_GITHUB_PAGES_GUIDE.md)

---

#### [devcontainer.yml](devcontainer.yml) 🆕
**Development Container Image** - Builds pre-built devcontainer
- 🛠️ Builds from `.devcontainer/Dockerfile`
- 🚀 Speeds up developer onboarding
- 🔒 Security scanning with Trivy
- ✅ Tests devcontainer functionality

**Image published**:
- `ghcr.io/duragraph/duragraph/devcontainer:latest`

**Benefits**:
- Faster devcontainer startup (no local build needed)
- Consistent development environment
- Pre-installed tools and dependencies

**Triggers**: Push to main, Changes to `.devcontainer/**`, Manual workflow dispatch

---

### Releases

#### [release-please.yml](release-please.yml)
**Automated Release Management** - Creates releases from conventional commits
- 📝 Generates CHANGELOG.md automatically
- 🏷️ Bumps version numbers
- 🚀 Creates GitHub releases

**How it works**:
1. Write conventional commits:
   ```bash
   feat: add new feature      # → Minor version bump (1.0.0 → 1.1.0)
   fix: fix bug               # → Patch version bump (1.0.0 → 1.0.1)
   feat!: breaking change     # → Major version bump (1.0.0 → 2.0.0)
   ```
2. Release Please creates a "Release PR" with changelog
3. Merge PR → GitHub release created automatically

**Triggers**: Push to main

---

## 🔧 Secrets Required

### GitHub Secrets (Repository Settings)

```bash
# Cloudflare Pages (for docs deployment)
CLOUDFLARE_API_TOKEN=...
CLOUDFLARE_ACCOUNT_ID=...

# Optional: Docker Hub (if publishing to Docker Hub)
DOCKER_USERNAME=...
DOCKER_PASSWORD=...

# Note: GITHUB_TOKEN is provided automatically by GitHub Actions
```

### Setting Up Secrets

1. Go to repository **Settings** → **Secrets and variables** → **Actions**
2. Click **New repository secret**
3. Add the secrets above

**Getting Cloudflare credentials**:
1. Log in to Cloudflare Dashboard
2. Go to **Account** → **API Tokens**
3. Create token with "Cloudflare Pages" permissions
4. Copy Account ID from Account Home

---

## 📊 Workflow Triggers Summary

| Workflow | Push Main | PR | Tags | Manual |
|----------|-----------|-----|------|--------|
| ci.yml | ✅ | ✅ | ❌ | ❌ |
| conformance.yml | ✅ | ✅ | ❌ | ❌ |
| contracts.yml | ✅ | ✅ | ❌ | ❌ |
| duragraph.yml | ✅ | ✅ | ✅ | ❌ |
| docs.yml | ✅ | ✅ | ❌ | ❌ |
| github-pages.yml | ✅ | ✅ | ❌ | ✅ |
| devcontainer.yml | ✅ | ✅ | ❌ | ✅ |
| release-please.yml | ✅ | ❌ | ❌ | ❌ |

---

## 🧪 Testing Workflows Locally

Use [Act](https://github.com/nektos/act) to test workflows locally before pushing:

```bash
# Setup Act (first time only)
task act:setup

# List all workflows
task act:list

# Test GitHub Pages deployment
task act:github-pages          # Full build
task act:github-pages:dry      # Dry run (preview)

# Test Cloudflare Pages deployment
task act:docs                  # Full build
task act:docs:dry              # Dry run

# Test CI workflow
task act:ci                    # Full CI
task act:ci:dry                # Dry run

# Test specific job
task act:job -- build

# Test specific workflow
task act:workflow -- github-pages.yml

# Clean up Act containers
task act:clean
```

**Quick Start Guides**:
- 📚 GitHub Pages: [GITHUB_PAGES_QUICKSTART.md](GITHUB_PAGES_QUICKSTART.md)
- 📖 Full Guide: [ACT_GITHUB_PAGES_GUIDE.md](../ACT_GITHUB_PAGES_GUIDE.md)
- 🎭 Act Setup: [README.act.md](../../README.act.md)

---

## 📈 Monitoring Workflows

### View Workflow Runs
- Go to **Actions** tab in GitHub repository
- Click on a workflow to see runs
- Click on a run to see logs

### Workflow Badges
Add badges to README.md:

```markdown
![CI](https://github.com/duragraph/duragraph/actions/workflows/ci.yml/badge.svg)
![Conformance](https://github.com/duragraph/duragraph/actions/workflows/conformance.yml/badge.svg)
![Docker](https://github.com/duragraph/duragraph/actions/workflows/duragraph.yml/badge.svg)
```

### Failed Workflows
- Check logs in GitHub Actions tab
- Use Act to debug locally: `task act:ci`
- Check secrets are set correctly

---

## 🔄 Workflow Dependencies

```
┌─────────────────────────────────────────────┐
│              Push to main                   │
└──────────────┬──────────────────────────────┘
               │
       ┌───────┼────────┬─────────┬──────────┐
       │       │        │         │          │
       ▼       ▼        ▼         ▼          ▼
     ci.yml  conformance contracts duragraph docs
               │        │         │          │
               │        │         ├─ Build API image
               │        │         ├─ Build dashboard image
               │        │         └─ Security scan
               │        │
               │        └─ Lint OpenAPI spec
               │        └─ Validate IR schemas
               │
               └─ Test API compatibility
                  with LangGraph Cloud
```

---

## 🆘 Troubleshooting

### Workflow fails with "Secret not found"
- Check secrets are set in repository settings
- Secret names must match exactly (case-sensitive)

### Docker build fails
- Check Dockerfile paths are correct
- Ensure base images are accessible
- Check disk space on runner

### Cloudflare deployment fails
- Verify `CLOUDFLARE_API_TOKEN` has correct permissions
- Check `CLOUDFLARE_ACCOUNT_ID` is correct
- Ensure project name matches in Cloudflare Pages

### Conformance tests fail
- Check Docker Compose services start correctly
- Verify API endpoints are accessible
- Check test data is valid

---

## 📚 Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Docker Build Push Action](https://github.com/docker/build-push-action)
- [Cloudflare Pages Action](https://github.com/cloudflare/pages-action)
- [Release Please](https://github.com/google-github-actions/release-please-action)
- [Act - Local GitHub Actions](https://github.com/nektos/act)

---

**Last Updated**: 2025-11-22
