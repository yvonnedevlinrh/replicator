## Why

`TestScaffold_FileCount` in `internal/agentkit/agentkit_test.go` uses manual length-check plus raw slice indexing to categorize scaffold results by directory prefix:

```go
case len(r.Path) > 9 && r.Path[:9] == "commands/":
case len(r.Path) > 7 && r.Path[:7] == "skills/":
case len(r.Path) > 7 && r.Path[:7] == "agents/":
```

This pattern is fragile -- the integer constants (9, 7, 7) must be manually kept in sync with the prefix string lengths. An off-by-one error is easy to introduce, as happened during the `command/` to `commands/` rename where the constant changed from 8 to 9.

Go's `strings.HasPrefix` eliminates this entire class of error.

Addresses upstream issue unbound-force/replicator#22.

## What Changes

Replace manual `len()+slice` prefix matching in `TestScaffold_FileCount` with `strings.HasPrefix` calls.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `TestScaffold_FileCount`: Uses `strings.HasPrefix` for prefix matching, removing fragile manual length constants

### Removed Capabilities
- None

## Impact

- **Affected file**: `internal/agentkit/agentkit_test.go` (lines 132-138)
- **Risk**: Minimal -- test-only change, no production code affected
- **Behavioral change**: None -- identical logic, safer implementation

## Constitution Alignment

Assessed against the Unbound Force org constitution.

### I. Autonomous Collaboration

**Assessment**: N/A

This change is internal to a test file and does not affect artifact-based communication or inter-agent interfaces.

### II. Composability First

**Assessment**: N/A

No new dependencies introduced. `strings` is a Go standard library package already imported in the project.

### III. Observable Quality

**Assessment**: N/A

Test-only change. No effect on machine-parseable output or provenance metadata.

### IV. Testability

**Assessment**: PASS

The change improves test maintainability by removing fragile manual index calculations, making the test more robust against future directory renames.
