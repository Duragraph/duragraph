# Act Commands Quick Reference

Quick reference for all available Act testing commands.

## 🎯 Most Common Commands

```bash
# First time setup
task act:setup

# Test GitHub Pages deployment (RECOMMENDED)
task act:github-pages

# Test with preview (no actual execution)
task act:github-pages:dry

# List all workflows
task act:list
```

---

## 📚 Deployment Testing

### GitHub Pages

```bash
# Full build test
task act:github-pages

# Dry run (preview steps)
task act:github-pages:dry
```

**Output:** `_site/` directory with merged docs and landing page

### Cloudflare Pages

```bash
# Full build test
task act:docs

# Dry run
task act:docs:dry
```

**Output:** `docs/out/` directory with Cloudflare Pages structure

---

## 🔧 CI/CD Testing

### CI Workflow

```bash
# Run full CI
task act:ci

# Dry run
task act:ci:dry
```

**Tests:** Pre-commit, Go tests, builds

### Specific Jobs

```bash
# List all jobs
act -l

# Run specific job
task act:job -- build              # Build job
task act:job -- pre-commit         # Pre-commit checks
task act:job -- unit_go            # Go tests
task act:job -- build-and-deploy   # Docs deployment
```

### Specific Workflows

```bash
# Run specific workflow file
task act:workflow -- github-pages.yml
task act:workflow -- ci.yml
task act:workflow -- docs.yml
task act:workflow -- duragraph.yml
```

---

## 🎭 Event Simulation

```bash
# Simulate push to main
task act:push

# Simulate pull request
task act:pr
```

---

## 🧹 Cleanup

```bash
# Clean up Act containers and cache
task act:clean
```

---

## 🐛 Debugging

### Verbose Output

```bash
act -W .github/workflows/github-pages.yml -j build --verbose
```

### Dry Run (Preview Steps)

```bash
act -W .github/workflows/github-pages.yml -j build --dryrun
```

### List Workflows and Jobs

```bash
# List all workflows
act -l

# List jobs in specific workflow
act -W .github/workflows/github-pages.yml -l
```

### Use Different Docker Image

```bash
# Use full Ubuntu image (more tools, slower)
act -W .github/workflows/github-pages.yml -j build \
  -P ubuntu-latest=catthehacker/ubuntu:full-latest
```

---

## 📋 All Available Workflows

From `act -l` output:

| Workflow | Job | Command |
|----------|-----|---------|
| **GitHub Pages** | build | `task act:github-pages` |
| **Cloudflare Pages** | build-and-deploy | `task act:docs` |
| **CI** | pre-commit | `task act:job -- pre-commit` |
| **CI** | unit_go | `task act:job -- unit_go` |
| **CI** | build_docs | `task act:job -- build_docs` |
| **Conformance** | conformance | `task act:workflow -- conformance.yml` |
| **Contracts** | openapi_lint | `task act:workflow -- contracts.yml` |
| **Docker Images** | build-and-push | `task act:workflow -- duragraph.yml` |
| **Devcontainer** | build-and-push | `task act:workflow -- devcontainer.yml` |

---

## 🔍 Advanced Usage

### Run with Secrets and Env

```bash
act -W .github/workflows/github-pages.yml \
  --secret-file .github/workflows/.secrets \
  --env-file .github/workflows/.env \
  -j build
```

### Run Specific Event

```bash
# Push event
act push -W .github/workflows/github-pages.yml

# Pull request event
act pull_request -W .github/workflows/github-pages.yml

# Workflow dispatch (manual trigger)
act workflow_dispatch -W .github/workflows/github-pages.yml
```

### Skip Certain Jobs

```bash
# Run only build job (skip deploy)
act -W .github/workflows/github-pages.yml -j build
```

---

## ⚙️ Configuration Files

### `.github/workflows/.secrets`
Contains sensitive tokens (never commit!)

```bash
CLOUDFLARE_API_TOKEN=...
CLOUDFLARE_ACCOUNT_ID=...
GITHUB_TOKEN=...  # For local testing
```

### `.github/workflows/.env`
Contains environment variables

```bash
NODE_ENV=production
NODE_VERSION=20
GITHUB_PAGES_BASE_PATH=/duragraph
```

---

## 🚨 Common Issues

### "workflow file not found"
**Solution:** Run from repository root: `cd /workspace`

### "secrets not found"
**Solution:** Run `task act:setup` first

### Docker errors
**Solution:** Verify Docker is running: `docker ps`

### Out of memory
**Solution:** Increase Docker memory limit (4GB recommended)

### Build fails
**Solution:** Test builds separately:
```bash
cd website && pnpm build
cd docs && pnpm build
```

---

## 📚 Documentation

- **Quick Start:** [GITHUB_PAGES_QUICKSTART.md](GITHUB_PAGES_QUICKSTART.md)
- **Complete Guide:** [../ACT_GITHUB_PAGES_GUIDE.md](../ACT_GITHUB_PAGES_GUIDE.md)
- **Workflows:** [README.md](README.md)

---

## 🎓 Examples

### Test GitHub Pages before deploying

```bash
# 1. Setup (first time)
task act:setup

# 2. Preview what will happen
task act:github-pages:dry

# 3. Run full build
task act:github-pages

# 4. Verify output
ls -la _site/

# 5. Preview locally
cd _site && python3 -m http.server 8000

# 6. Clean up
task act:clean
```

### Test multiple workflows

```bash
# Test CI
task act:ci:dry

# Test GitHub Pages
task act:github-pages:dry

# Test Cloudflare Pages
task act:docs:dry

# If all look good, run full builds
task act:ci
task act:github-pages
task act:docs
```

### Debug failing workflow

```bash
# Run with verbose output
act -W .github/workflows/github-pages.yml -j build --verbose

# Or run in dry-run mode to see steps
act -W .github/workflows/github-pages.yml -j build --dryrun

# Check specific step output
act -W .github/workflows/github-pages.yml -j build | grep "Build website"
```

---

**Pro Tip:** Always run `task act:github-pages:dry` first to preview steps before running the full build. It's faster and helps catch configuration errors early!
