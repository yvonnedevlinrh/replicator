## Context

Replicator's release pipeline has two supply-chain hygiene gaps identified in
issue #15:

1. `release.yml` triggers on any `v*` tag push and runs GoReleaser immediately
   with no CI preflight verification.
2. CI workflows use mutable floating action tags instead of commit SHA pins.

The canonical reference (`unbound-force/unbound-force/.github/workflows/release.yml`)
implements a robust preflight pattern that this change adapts for replicator.
A tag ruleset already restricts `v*` tag creation to org admins, providing a
partial mitigation for the trigger surface.

## Goals / Non-Goals

### Goals

- Add a preflight validation job to `release.yml` that gates releases on CI
  status, tag format, uniqueness, and semver ordering.
- Pin all GitHub Actions references to full commit SHAs in both `ci.yml` and
  `release.yml`.
- Rename the CI job to `Build and Test` to match org convention and provide a
  stable check name for the preflight to query via the Checks API.
- Add `permissions:` and `concurrency:` blocks to `ci.yml` for security
  hardening and duplicate-run prevention.

### Non-Goals

- Adding security scanning (`govulncheck`, OSV-Scanner) to CI — tracked in #23.
- Adding coverage ratchets to CI — tracked in #24.
- Adopting org-infra reusable workflows (MegaLinter, PR title checks) —
  tracked in #25.
- Modifying Go source code, tests, or the replicator binary.

## Decisions

### D1: Rename CI job to "Build and Test"

The preflight job queries the GitHub Checks API by check name string to verify
CI passed. Replicator's CI job is currently named `test`, which diverges from
the org convention of `Build and Test`. Renaming aligns with the canonical
reference and produces a more readable preflight. The visible impact (CI badge
name change) is a one-time cosmetic transition.

### D2: Use canonical action SHAs from unbound-force

The canonical reference (`unbound-force/unbound-force`) has updated to
`actions/checkout@v6.0.3` (`df4cb1c0...`) and `actions/setup-go@v6.4.0`
(`4a360112...`). Rather than pinning to the older v4/v5 SHAs currently in
use, we adopt the same SHAs as the canonical reference for consistency. This
is a version upgrade (v4 -> v6 for checkout, v5 -> v6 for setup-go) but both
are stable releases with no breaking changes for our usage.

Action SHA mapping:

| Action | Current | Pinned SHA | Version |
|--------|---------|------------|---------|
| `actions/checkout` | `@v4` | `df4cb1c069e1874edd31b4311f1884172cec0e10` | v6.0.3 |
| `actions/setup-go` | `@v5` | `4a3601121dd01d1626a1e23e37211e3254c1c06c` | v6.4.0 |
| `goreleaser/goreleaser-action` | `@v6` | `5daf1e915a5f0af01ddbcd89a43b8061ff4f1a89` | v7.2.2 |

### D3: Switch release trigger to workflow_dispatch

Replacing `on: push: tags: ['v*']` with `on: workflow_dispatch: inputs: tag:`
enables the preflight job to validate before the tag exists. The preflight
creates the tag after all validations pass. This is the same pattern used by
the canonical reference.

The preflight MUST validate that the workflow was triggered from the `main`
branch. Since `workflow_dispatch` can be triggered from any branch, without
this check a release could be built from a feature branch's HEAD.

Pre-release and build metadata suffixes (e.g., `-rc.1`, `+build.123`) are
intentionally excluded by the tag format regex. The release pipeline only
supports stable semver releases. Pre-release distribution, if needed, would
use a separate workflow.

**Breaking change**: After this change, releases can no longer be triggered by
pushing a tag. Contributors must use:
- GitHub Actions UI: Actions -> Release -> Run workflow -> enter tag
- CLI: `gh workflow run release.yml -f tag=vX.Y.Z`

### D4: Replace GITHUB_REF_NAME with inputs.tag

With `workflow_dispatch`, `GITHUB_REF_NAME` is the branch name (e.g., `main`),
not the tag. All references to `GITHUB_REF_NAME` in the `release` and
`sign-macos` jobs must be replaced with `inputs.tag`. We use a job-level
`env: RELEASE_TAG: ${{ inputs.tag }}` for consistency with the canonical
reference, then reference `$RELEASE_TAG` in shell scripts.

### D5: Omit security scan check from preflight

The canonical preflight verifies that at least one security scan check passed.
Replicator has no security scan CI step. Rather than adding a dummy check or
skipping validation of a non-existent check, we omit the security scan block
entirely. When govulncheck is added (issue #23), the preflight can be extended
to verify it.

### D6: Scope permissions per-job

The current `release.yml` has `permissions: contents: write` at workflow level,
which grants write access to all jobs. Following the canonical reference, we
set `permissions: {}` at workflow level and grant per-job permissions:
- `preflight`: `contents: write` (to push the tag), `checks: read` (to query
  CI status)
- `release`: `contents: write` (to create the release), `id-token: write`
  (for Cosign signing)
- `sign-macos`: `contents: write` (to upload signed assets)

## Risks / Trade-offs

- **Action version upgrade**: Moving from checkout v4 to v6, setup-go v5 to
  v6, and goreleaser-action v6 to v7 introduces upgrade risk. All are stable
  releases with no known breaking changes for our usage. The canonical
  reference has been running these versions in production. The GoReleaser
  binary version (`v2.14.1`) is already pinned and unchanged.
- **SHA maintenance**: Pinned SHAs require periodic updates when upstream
  actions release security patches. GitHub Dependabot with
  `package-ecosystem: github-actions` in `.github/dependabot.yml` is the
  recommended low-friction approach. This can be added as a follow-up.
  SHAs were verified by cross-referencing the canonical reference
  (`unbound-force/unbound-force`) against the official action repositories'
  release tags.
- **Release process change**: Contributors must learn the new release process
  (workflow_dispatch instead of tag push). The existing tag ruleset already
  restricts tag creation to org admins, so this change primarily affects the
  same small group.
- **No local testability**: GitHub Actions workflows cannot be tested locally.
  The preflight logic is adapted from a production-validated canonical
  reference, which mitigates this risk. Manual verification will be performed
  by triggering a `workflow_dispatch` with a test tag after merge.
- **CI check name dependency**: The preflight is tightly coupled to the CI
  job name `Build and Test`. If the CI job is renamed without updating the
  preflight, releases will be blocked. This coupling is intentional and
  matches the canonical pattern.
- **`sort -V` platform dependency**: The semver ordering check uses GNU
  `sort -V`, which is available on `ubuntu-latest` runners but not on macOS
  BSD sort by default. Since the preflight only runs on `ubuntu-latest`, this
  is an acceptable platform dependency.
- **Rollback**: If the new release workflow blocks releases, revert
  `release.yml` to the previous `push: tags` trigger and push a tag manually.
  The GoReleaser configuration is unchanged and works with either trigger.

**Gatekeeping note**: Action SHA changes fall under the Gatekeeping Value
Protection constraint in AGENTS.md. This change is authorized by the spec
author as an improvement (pinning is more secure than floating tags).
<!-- scaffolded by uf vdev -->
