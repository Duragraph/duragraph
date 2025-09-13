# Contributing to Duragraph

Thank you for your interest in contributing to **Duragraph**! We welcome contributions from the community and are excited to build together.  

---

## 🛠 Development Environment Setup

### Prerequisites
- **Git** ≥ 2.30  
- **Go** ≥ 1.21  
- **Python** ≥ 3.10  
- **Node.js** ≥ 18  
- **Docker** ≥ 20  

### Setup
```bash
git clone https://github.com/YOUR_ORG/duragraph.git
cd duragraph
make deps
```

---

## ✨ Contribution Workflow

1. Fork the repository
2. Create your feature branch (`git checkout -b feat/amazing-feature`)
3. Commit your changes using **Conventional Commits** (see below)
4. Push to your branch
5. Open a Pull Request

---

## 📝 Commit Conventions

Duragraph follows the **Conventional Commits v1.0.0** specification.

Example:
```
feat(runtime): add bridge worker
fix(docs): correct typo in quickstart section
chore: update dependencies
```

Types include:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation
- `style`: Formatting
- `refactor`: Refactoring code
- `test`: Adding missing tests
- `chore`: Maintenance tasks

---

## 🚀 Quickstart

After cloning and installing dependencies:

```bash
make dev   # start development env
make test  # run test suite
```

---

## 📌 Guidelines

- Open an issue before starting large changes.  
- Include tests whenever possible.  
- Ensure **linting** passes before committing.  
- Follow the **Code of Conduct**.  

---

## 🔒 Security

Please see [SECURITY.md](SECURITY.md) for guidance on reporting vulnerabilities.