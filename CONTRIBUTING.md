# Contributing to PicoClip

Thank you for contributing.

## Before starting

- Read `AGENTS.md`, `docs/PROJECT_MAP.md`, and `docs/DOCUMENTATION_POLICY.md`.
- For product features, epics, architecture changes, or new implementation scope, obtain explicit maintainer approval before coding.
- For security vulnerabilities, use the private process in `SECURITY.md` instead of a public issue.

## Development workflow

1. Create a branch from the latest `main`.
2. Keep changes focused and preserve local-first, low-resource design goals.
3. Use tests first for behavior changes and bug fixes.
4. Update the canonical documentation for every changed API, operation, UI flow, storage contract, runtime, or developer command.
5. Run the proportional checks; relevant changes should normally pass:

```sh
make check
```

6. Open a pull request. Direct pushes to `main` are blocked.

## Pull requests

- Explain what changed and why.
- Include exact validation commands and results.
- Do not include credentials, tokens, databases, generated evidence, `graphify-out/`, or machine-specific files.
- Resolve review conversations and update the branch before merging.
- The required `Check` status must pass.

## Commit style

Use concise Conventional Commit-style subjects when practical, for example:

```text
fix(runtime): reject unsafe config path
feat(api): add task status endpoint
docs: document recovery procedure
```

## Reporting bugs

Public bug reports should include a minimal reproduction, affected version or commit, environment, and sanitized logs. Never include secrets or private data.

## License

The repository does not currently declare a license. Do not assume permission beyond GitHub's terms until the maintainer selects and adds one.
