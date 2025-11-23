# GitHub Pages Quick Start

Quick reference for testing and deploying to GitHub Pages with Act.

## 🚀 Quick Commands

```bash
# 1. Setup Act (first time only)
task act:setup

# 2. Test build locally (dry run)
task act:github-pages:dry

# 3. Test full build locally
task act:github-pages

# 4. Preview built site
cd _site && python3 -m http.server 8000
# Visit: http://localhost:8000

# 5. Clean up
task act:clean
```

## 📁 Expected Output

After successful build:

```
_site/
├── index.html        # Root redirect
├── .nojekyll        # GitHub Pages marker
├── docs/            # Documentation (Fumadocs)
└── landing/         # Landing page (website)
```

## 🌐 URLs After Deployment

- **Root:** `https://yourusername.github.io/duragraph/` → Redirects to docs
- **Docs:** `https://yourusername.github.io/duragraph/docs`
- **Landing:** `https://yourusername.github.io/duragraph/landing`

## ✅ Pre-Deploy Checklist

- [ ] `task act:github-pages` succeeds
- [ ] `_site/` directory created
- [ ] Local preview works
- [ ] Links work
- [ ] Assets load

## 🔧 Troubleshooting

| Issue | Solution |
|-------|----------|
| Secrets not found | `task act:setup` |
| Docker errors | `docker ps` to verify |
| Build fails | Check `docs/` and `website/` build separately |
| Out of memory | Increase Docker memory limit |

## 📚 Full Documentation

See [ACT_GITHUB_PAGES_GUIDE.md](../ACT_GITHUB_PAGES_GUIDE.md) for complete guide.

## 🎯 Deploy to Production

```bash
# Commit and push
git add .
git commit -m "feat: deploy to GitHub Pages"
git push origin main

# Enable GitHub Pages in repo settings:
# Settings → Pages → Source: GitHub Actions
```

## 🔗 More Commands

```bash
task act:list                    # List all workflows
task act:docs                    # Test Cloudflare Pages
task act:workflow -- ci.yml      # Test specific workflow
task act:clean                   # Clean up containers
```
