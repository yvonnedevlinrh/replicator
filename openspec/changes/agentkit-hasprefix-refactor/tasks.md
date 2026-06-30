## 1. Refactor prefix matching

- [x] 1.1 Add `"strings"` to the import block in `internal/agentkit/agentkit_test.go` (if not already present)
- [x] 1.2 Replace the three manual `len()+slice` prefix checks in `TestScaffold_FileCount` (lines 132-138) with `strings.HasPrefix` calls:
  - `case len(r.Path) > 9 && r.Path[:9] == "commands/"` → `case strings.HasPrefix(r.Path, "commands/")`
  - `case len(r.Path) > 7 && r.Path[:7] == "skills/"` → `case strings.HasPrefix(r.Path, "skills/")`
  - `case len(r.Path) > 7 && r.Path[:7] == "agents/"` → `case strings.HasPrefix(r.Path, "agents/")`

## 2. Verification

- [x] 2.1 Run `go test ./internal/agentkit/...` and confirm `TestScaffold_FileCount` passes with identical assertions (commands=5, skills=7, agents=3)
- [x] 2.2 Run `go vet ./internal/agentkit/...` to confirm no issues
- [x] 2.3 Run `goimports` or confirm import ordering is correct (stdlib group)
- [x] 2.4 Verify constitution alignment: Testability (IV) -- test remains isolated, uses `t.TempDir()`, no external dependencies
