<!--
Thanks for contributing! A few quick checks below — if you're sending
a one-line typo fix, feel free to delete sections that don't apply.
-->

## What and why

<!-- A sentence or two on what this changes and the motivation. Link an
issue if there is one. -->

## How to test

<!-- One curl command, one test name, or a 3-step manual procedure. -->

## Type of change

- [ ] Bug fix (non-breaking)
- [ ] New feature (non-breaking)
- [ ] Breaking change (gates a major version bump)
- [ ] Documentation only

## Checklist

- [ ] `go test ./...` passes
- [ ] `go vet ./...` is clean
- [ ] New behaviour is covered by a test
- [ ] Public-facing changes are documented in `docs/` or `README.md`
- [ ] No new dependencies *or* the PR description justifies the added one
- [ ] If touching `core/gateway/`, `core/keys/`, or `core/management/policy/`,
      I've requested a security-aware reviewer

## Related issues

<!-- Closes #123 -->
