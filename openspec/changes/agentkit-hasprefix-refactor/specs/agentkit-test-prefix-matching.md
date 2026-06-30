## ADDED Requirements

None.

## MODIFIED Requirements

### Requirement: Test prefix matching in TestScaffold_FileCount

The `TestScaffold_FileCount` test MUST use `strings.HasPrefix` for directory prefix categorization instead of manual length-check and slice indexing.

Previously: The test used `len(r.Path) > N && r.Path[:N] == "prefix/"` for each directory category, requiring manual synchronization of integer constants with prefix string lengths.

#### Scenario: Prefix matching uses strings.HasPrefix
- **GIVEN** the `TestScaffold_FileCount` function in `internal/agentkit/agentkit_test.go`
- **WHEN** categorizing scaffold results by directory prefix
- **THEN** all prefix checks MUST use `strings.HasPrefix(r.Path, "prefix/")` instead of manual length and slice operations

#### Scenario: Test behavior remains identical
- **GIVEN** the refactored `TestScaffold_FileCount` function
- **WHEN** the test suite is executed
- **THEN** the test MUST pass with the same assertions (commands=5, skills=7, agents=3)

#### Scenario: strings package is imported
- **GIVEN** the refactored test file
- **WHEN** `strings.HasPrefix` is used in the test
- **THEN** the `strings` package MUST be imported

## REMOVED Requirements

None.
