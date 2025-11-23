# Contributing to DuraGraph

Thank you for your interest in contributing to DuraGraph! We welcome contributions from the community.

## 🚨 Important: Branch Policy

**Do not create branches directly in the main repository.** All contributions must come through pull requests from your fork.

## 🎯 Finding Tasks to Work On

We use [GitHub Projects](https://github.com/duragraph/duragraph/projects) to organize tasks. Here's how to find something to work on:

### For First-Time Contributors

1. Check the [**Good First Issue**](https://github.com/duragraph/duragraph/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22) label
   - These are well-defined, beginner-friendly tasks
   - Perfect for getting familiar with the codebase

2. Look at the [**Project Board**](https://github.com/duragraph/duragraph/projects)
   - **📋 Backlog** - Future work and ideas
   - **🆕 Good First Issue** - Easy tasks for newcomers
   - **🔧 Ready to Work** - Well-defined tasks ready to be picked up
   - **🚧 In Progress** - Currently being worked on (don't pick these)
   - **👀 In Review** - PRs awaiting review
   - **✅ Done** - Completed tasks

3. Filter by labels:
   - `good first issue` - Great for newcomers
   - `help wanted` - We need community help
   - `difficulty: easy` - Low complexity
   - `documentation` - Docs improvements

### Claiming a Task

1. **Comment on the issue**: "I'd like to work on this"
2. Wait for maintainer approval (we'll assign it to you)
3. If no response within 24 hours, go ahead and start working
4. **Don't work on assigned issues** - respect others' claims

### How to Contribute

1. **Fork the repository**
   - Click the "Fork" button at the top right of the repository page
   - This creates your own copy of the repository

2. **Clone your fork**
   ```bash
   git clone https://github.com/YOUR_USERNAME/duragraph.git
   cd duragraph
   ```

3. **Add upstream remote**
   ```bash
   git remote add upstream https://github.com/Duragraph/duragraph.git
   ```

4. **Create a feature branch in your fork**
   ```bash
   git checkout -b feature/your-amazing-feature
   ```

5. **Make your changes**
   - Write clear, commented code
   - Follow the existing code style
   - Add tests for new features
   - Update documentation as needed

6. **Test your changes**
   ```bash
   task test
   task lint
   ```

7. **Commit your changes**
   ```bash
   git add .
   git commit -m "feat: add amazing feature"
   ```

   Follow [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat:` - New feature
   - `fix:` - Bug fix
   - `docs:` - Documentation changes
   - `test:` - Adding tests
   - `refactor:` - Code refactoring
   - `chore:` - Maintenance tasks

8. **Push to your fork**
   ```bash
   git push origin feature/your-amazing-feature
   ```

9. **Create a Pull Request**
   - Go to the original repository on GitHub
   - Click "New Pull Request"
   - Select "compare across forks"
   - Choose your fork and branch
   - Fill out the PR template
   - Wait for review

## 📋 Pull Request Guidelines

- **One feature per PR** - Keep changes focused
- **Write clear descriptions** - Explain what and why
- **Reference issues** - Use "Closes #123" if applicable
- **Pass all checks** - Tests, linting, and CI must pass
- **Be responsive** - Address review feedback promptly

## 🧪 Development Setup

See our [Development Guide](https://duragraph.dev/docs/development) for detailed setup instructions.

Quick start:
```bash
# Install dependencies
task install

# Start all services
task up

# Run tests
task test

# Run linting
task lint
```

## ✅ Code Style

- **Go**: Follow [Effective Go](https://golang.org/doc/effective_go.html) and run `gofmt`
- **TypeScript/JavaScript**: Use ESLint and Prettier
- **Python**: Follow PEP 8
- **Documentation**: Use clear, concise language

## 🐛 Reporting Bugs

Use the [GitHub Issues](https://github.com/Duragraph/duragraph/issues) page:

1. Check if the issue already exists
2. Use the bug report template
3. Include reproduction steps
4. Provide environment details
5. Add relevant logs/screenshots

## 💡 Feature Requests

Use [GitHub Discussions](https://github.com/Duragraph/duragraph/discussions):

1. Describe the feature clearly
2. Explain the use case
3. Consider implementation approach
4. Discuss alternatives

## 📝 Documentation

Documentation improvements are always welcome! Our docs are in the `docs/` directory using Fumadocs.

```bash
cd docs
pnpm install
pnpm dev
# Visit http://localhost:3000
```

## ❓ Questions

- **General questions**: [GitHub Discussions](https://github.com/Duragraph/duragraph/discussions)
- **Bug reports**: [GitHub Issues](https://github.com/Duragraph/duragraph/issues)
- **Documentation**: [duragraph.dev/docs](https://duragraph.dev/docs)

## 📜 License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).

## 🙏 Thank You!

Your contributions make DuraGraph better for everyone. We appreciate your time and effort!

---

**Questions?** Open a [discussion](https://github.com/Duragraph/duragraph/discussions) or reach out to the maintainers.
