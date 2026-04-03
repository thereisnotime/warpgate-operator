# Contributing

## Commit Messages

This project uses [conventional commits](https://www.conventionalcommits.org/). Every commit message must follow the format:

```text
<type>(optional scope): <description>
```

Allowed types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`

Examples:

```text
feat(crd): add WarpgateRole custom resource
fix(controller): handle nil pointer on missing secret ref
docs: update quickstart guide
chore(deps): bump controller-runtime to v0.20
```

## Setting Up Pre-Commit Hooks

Install [pre-commit](https://pre-commit.com/) and set up the hooks:

```bash
pre-commit install --hook-type commit-msg --hook-type pre-push
```

This enforces conventional commit messages and runs basic linting before each commit.

## Running Lints

```bash
just lint        # run all linters (read-only)
just lint-fix    # run linters and auto-fix where possible
```

## Running Tests

```bash
just test
```

## Pull Requests

- Use a conventional commit style title (e.g. `feat(crd): add SSH target resource`).
- Make sure CI passes before requesting review.
- Keep PRs focused -- one logical change per PR when possible.
