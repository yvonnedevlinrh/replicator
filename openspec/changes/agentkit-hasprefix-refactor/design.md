## Context

`TestScaffold_FileCount` in `internal/agentkit/agentkit_test.go` categorizes scaffold results by directory prefix using manual length checks and slice indexing. This is fragile because the integer length constants must stay in sync with the prefix strings. The issue was surfaced during PR #20 review when the `command/` to `commands/` rename required updating a constant from 8 to 9.

Go's standard library provides `strings.HasPrefix` which eliminates this entire class of error by handling length checks internally.

## Goals / Non-Goals

### Goals
- Replace manual `len()+slice` prefix matching with `strings.HasPrefix` in `TestScaffold_FileCount`
- Ensure `strings` package is imported (add if not already present)
- Maintain identical test behavior -- same assertions, same pass/fail conditions

### Non-Goals
- Refactoring any production code (this is test-only)
- Changing the test's assertion values or logic
- Modifying any other test functions in the file

## Decisions

**Use `strings.HasPrefix` directly**: The simplest approach. Replace the three `case` clauses in the `switch` statement with `strings.HasPrefix` calls. No helper functions, no abstractions -- just a direct stdlib call.

**Rationale**: This is a one-line-per-case replacement. Introducing a helper or table-driven approach would be over-engineering for three static prefix checks. The constitution's Testability principle (IV) is upheld -- the test remains isolated and independently runnable.

## Risks / Trade-offs

- **Risk**: Near-zero. `strings.HasPrefix` is a well-established stdlib function with identical semantics to the manual check it replaces.
- **Trade-off**: None meaningful. The refactored code is shorter, clearer, and immune to length/index drift.
