---
name: write-tests
description: Write and refactor tests for Air Cover. Use when adding tests, changing tests, improving coverage, choosing fakes versus integration tests, or discussing test tooling such as mockery.
---

# Write Tests

## Coverage

The project keeps a strict, filtered `100.0%` coverage gate in `make test`.
Generated Go files and templ output are excluded from the denominator; application
and handwritten adapter code are not.

Treat uncovered lines as a prompt to test observable behavior, simplify
unreachable code, or isolate adapter-specific failure handling with a focused
test.

## Test Boundaries

Write tests around public behavior at package boundaries:

- Domain tests cover invariants and value behavior in `internal/domain`.
- App service tests call public service methods and use small handwritten fakes
  for app-owned ports.
- Outbound adapters prove shared behavior through reusable contract tests under
  `internal/app/contracttest`, with adapter-specific persistence and security
  checks kept beside the adapter.
- Inbound HTTP tests prefer fake app services for routing, validation, status,
  redirect, and response behavior. Keep SQLite-backed tests for representative
  end-to-end flows.
- Composition tests use dependency fakes for wiring, lifecycle, and startup
  behavior.
- Architecture tests protect import direction and ownership rules.

## Style

Prefer Arrange, Act, Assert structure. In short tests, whitespace is enough; in
longer tests, use `// Arrange`, `// Act`, and `// Assert` comments when they make
the behavior easier to scan.

Handwritten fakes are the default. Tools such as
[`mockery`](https://github.com/vektra/mockery) are useful when a reused interface
has many methods or strict interaction assertions become important, but avoid
repo-wide generated mocks unless a narrow pilot proves they reduce test noise.
After the current test cleanup, the app-owned ports are still small enough that
no mock generation pilot is warranted.
