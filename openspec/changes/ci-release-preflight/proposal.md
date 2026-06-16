## Why

The release pipeline (`release.yml`) triggers on any `v*` tag push and runs
GoReleaser immediately with no verification that CI passed on the tagged
commit. A tag pushed from a commit that never passed tests will produce and
distribute release binaries with unknown quality.

Additionally, CI workflows use mutable floating action tags
(`actions/checkout@v4`, `actions/setup-go@v5`) instead of commit SHA pins. The
project's own `severity.md` classifies unpinned CI actions on mutable tags as
HIGH severity supply-chain risk. All other repos in the org pin actions to full
commit SHAs.

Fixes: https://github.com/unbound-force/replicator/issues/15

A tag ruleset has been applied to restrict `v*` tag creation to org admins, but
this only limits who can trigger a release — it does not verify CI status or
address action pinning.

## What Changes

1. **`ci.yml`**: Add `permissions:` block, `concurrency:` group, rename the
   job from `test` to `Build and Test` (matching org convention), and pin all
   action references to full commit SHAs.

2. **`release.yml`**: Switch the trigger from `push: tags: ['v*']` to
   `workflow_dispatch` with a `tag` input. Add a `preflight` job that validates
   tag format, uniqueness, semver ordering, and CI status via the GitHub Checks
   API before allowing GoReleaser to run. Pin all action references to full
   commit SHAs. Scope permissions per-job instead of workflow-level. Replace
   all `GITHUB_REF_NAME` references with `inputs.tag` (since `workflow_dispatch`
   sets `GITHUB_REF_NAME` to the branch name, not the tag).

## Capabilities

### New Capabilities

- `release/preflight`: Pre-flight validation job that gates every release on
  tag format, uniqueness, semver ordering, CI status, and unreleased commits.

### Modified Capabilities

- `release/trigger`: Changes from automatic tag-push trigger to manual
  `workflow_dispatch` with explicit tag input.
- `ci/job-naming`: CI job renamed from `test` to `Build and Test` to match org
  convention and provide a stable check name for the preflight to query.
- `ci/permissions`: Explicit `permissions: contents: read` and `concurrency:`
  group added to CI workflow.

### Removed Capabilities

- None

## Impact

- `.github/workflows/ci.yml` — permissions, concurrency, job rename, SHA pins
- `.github/workflows/release.yml` — trigger change, preflight job, SHA pins,
  per-job permissions, `GITHUB_REF_NAME` -> `inputs.tag` replacement
- Release process: contributors must use the GitHub Actions UI or
  `gh workflow run release.yml -f tag=vX.Y.Z` instead of pushing a tag
- The preflight verifies only `"Build and Test"` — no security scan check is
  required until govulncheck is added to CI (tracked in #23)

## Constitution Alignment

Assessed against the Replicator constitution (`.specify/memory/constitution.md`),
which extends the Unbound Force org constitution v1.1.0.

### I. Autonomous Collaboration

**Assessment**: N/A

This change modifies CI/CD workflow files only. No MCP tools, inter-agent
communication, or tool outputs are affected. The change is purely
infrastructure-level and does not alter how heroes collaborate through
artifacts.

### II. Composability First

**Assessment**: PASS

The binary remains independently installable and usable without any external
services. The CI and release workflows are GitHub-specific infrastructure that
do not affect the standalone functionality of the replicator binary. Dewey
integration and graceful degradation are unaffected.

### III. Observable Quality

**Assessment**: PASS

The preflight job produces structured GitHub Actions output with clear
pass/fail status for each validation step (tag format, uniqueness, semver,
CI status, unreleased commits). Error messages use `::error::` annotations
for machine-parseable CI feedback. The CI job rename to `Build and Test`
provides a stable, human-readable check name for both the preflight and
branch protection rules.

### IV. Testability

**Assessment**: N/A

This change modifies GitHub Actions workflow files which cannot be tested
in isolation (they require the GitHub Actions runtime). The change does not
modify any Go source code, tests, or testable components. The preflight
logic is adapted from the canonical reference (`unbound-force/unbound-force`)
which has been validated in production. Post-merge verification will be
performed by triggering a `workflow_dispatch` with a test tag.

### Development Workflow Impact

The constitution's Development Workflow section (line ~101) states "Tag `v*`
triggers GoReleaser." This description becomes inaccurate after this change.
A documentation update task is included to correct this to reflect the new
`workflow_dispatch` trigger. This is a factual correction to a descriptive
statement, not a modification of a constitutional principle.
<!-- scaffolded by uf vdev -->
