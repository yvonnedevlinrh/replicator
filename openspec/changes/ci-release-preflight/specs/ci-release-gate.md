## ADDED Requirements

### Requirement: Release Preflight Validation

The `release.yml` workflow MUST include a `preflight` job that runs before
the `release` job. The `release` job MUST declare `needs: preflight`. The
preflight job MUST validate all of the following before proceeding:

1. **Branch validation**: The workflow MUST verify it was triggered from the
   default branch (`main`). Releases from feature branches MUST be rejected.
2. **Tag format**: The input tag MUST match `^v[0-9]+\.[0-9]+\.[0-9]+$`.
   Pre-release suffixes (e.g., `-rc.1`, `-beta`) and build metadata (e.g.,
   `+build.123`) are intentionally excluded — the release pipeline only
   supports stable semver releases.
3. **Tag uniqueness**: The input tag MUST NOT already exist on the remote,
   UNLESS the existing tag points to the current HEAD commit (indicating a
   re-run of a previously successful preflight). In the re-run case, the
   uniqueness check MUST pass and the tag creation step MUST be skipped.
4. **Semver ordering**: The input tag MUST be greater than the latest
   existing release tag (using GNU `sort -V`, available on `ubuntu-latest`
   runners). If no existing tags are found (first release), this check MUST
   pass unconditionally.
5. **CI status**: The `Build and Test` check MUST have concluded with
   `success` on the HEAD commit, verified via the GitHub Checks API. If the
   API call fails or the check has not yet concluded, the preflight MUST
   fail with a descriptive error.
6. **Unreleased commits**: At least one commit MUST exist since the last
   release tag.

If any validation fails, the preflight MUST exit with a non-zero status and
produce an `::error::` annotation describing the failure. Error messages
SHOULD include the tag value and relevant context (e.g., comparison values
for semver ordering failures).

After all validations pass, the preflight MUST create an annotated tag and
push it to the remote. Tag creation MUST be idempotent — if the tag already
exists and points to HEAD (e.g., from a re-run after partial failure), the
step MUST skip creation.

#### Scenario: All validations pass

- **GIVEN** the workflow is triggered from the `main` branch, the HEAD
  commit has a passing `Build and Test` check, there are unreleased commits
  since the last tag, and the input tag is a valid semver greater than the
  latest tag
- **WHEN** the preflight job runs with tag `v1.2.0`
- **THEN** all validation steps succeed, the tag `v1.2.0` is created and
  pushed, and the `release` job proceeds

#### Scenario: Triggered from non-default branch

- **GIVEN** the workflow is triggered from branch `feature/foo`
- **WHEN** the preflight job runs with any tag
- **THEN** the preflight fails with `::error::` mentioning "must be
  triggered from main" and the `release` job does not run

#### Scenario: Invalid tag format

- **GIVEN** the input tag is `v1.2` (or `v1.2.0-beta`, `1.2.0`, `vfoo`)
- **WHEN** the preflight job validates the tag format
- **THEN** the preflight fails with `::error::` mentioning "Invalid tag
  format" and the `release` job does not run

#### Scenario: CI has not passed on HEAD

- **GIVEN** the HEAD commit has no passing `Build and Test` check
- **WHEN** the preflight job runs with any valid tag
- **THEN** the preflight fails with `::error::` mentioning "Required check
  'Build and Test' has not passed" and the `release` job does not run

#### Scenario: CI check is still in progress

- **GIVEN** the HEAD commit has a `Build and Test` check that has not yet
  concluded (status is pending or in_progress)
- **WHEN** the preflight job queries the Checks API
- **THEN** the preflight fails with `::error::` mentioning "has not passed"
  and the `release` job does not run

#### Scenario: Checks API failure

- **GIVEN** the GitHub Checks API returns an error or is unreachable
- **WHEN** the preflight job queries CI status
- **THEN** the preflight fails with `::error::` mentioning the API error
  and the `release` job does not run

#### Scenario: Tag already exists on different commit

- **GIVEN** tag `v1.1.0` already exists on the remote and points to a
  commit other than HEAD
- **WHEN** the preflight job runs with tag `v1.1.0`
- **THEN** the preflight fails at the tag uniqueness step with `::error::`
  mentioning "already exists" and the `release` job does not run

#### Scenario: Tag is not greater than latest

- **GIVEN** the latest existing tag is `v1.2.0`
- **WHEN** the preflight job runs with tag `v1.1.0`
- **THEN** the preflight fails at the semver ordering step with `::error::`
  mentioning "not greater than" and the `release` job does not run

#### Scenario: First release (no existing tags)

- **GIVEN** no `v*` tags exist on the remote
- **WHEN** the preflight job runs with tag `v0.1.0`
- **THEN** the semver ordering check passes (no previous tag to compare
  against) and the preflight proceeds

#### Scenario: No unreleased commits

- **GIVEN** there are zero commits since the latest tag
- **WHEN** the preflight job runs
- **THEN** the preflight fails with `::error::` mentioning "No unreleased
  commits" and the `release` job does not run

#### Scenario: Re-run after partial failure

- **GIVEN** a previous `workflow_dispatch` run with tag `v1.2.0` created
  and pushed the tag to the remote (pointing to HEAD), but the `release`
  job failed
- **WHEN** a new `workflow_dispatch` is triggered with the same tag `v1.2.0`
- **THEN** the tag uniqueness check detects the tag exists and points to
  HEAD, so it passes. The tag creation step detects the tag exists and
  skips creation. All other validations succeed and the `release` job
  proceeds

### Requirement: Release Trigger via workflow_dispatch

The `release.yml` workflow MUST use `on: workflow_dispatch` with a required
`tag` input of type `string` instead of `on: push: tags: ['v*']`.

Previously: The workflow triggered automatically on any `v*` tag push.

#### Scenario: Manual release trigger

- **GIVEN** a contributor wants to create a release
- **WHEN** they run `gh workflow run release.yml -f tag=v1.2.0`
- **THEN** the workflow starts with `inputs.tag` set to `v1.2.0`

### Requirement: Signing Secrets Detection

The `preflight` job MUST check for the presence of macOS signing secrets and
output `has_signing_secrets` (true/false). The `release` job MUST forward
this output. The `sign-macos` job MUST be conditional on
`has_signing_secrets == 'true'`.

#### Scenario: Signing secrets available

- **GIVEN** the `MACOS_SIGN_P12` secret is configured
- **WHEN** the preflight job checks for signing secrets
- **THEN** `has_signing_secrets` is set to `true` and the `sign-macos` job
  runs after `release` completes

#### Scenario: Signing secrets unavailable

- **GIVEN** no `MACOS_SIGN_P12` secret is configured
- **WHEN** the preflight job checks for signing secrets
- **THEN** `has_signing_secrets` is set to `false` and the `sign-macos` job
  is skipped

## MODIFIED Requirements

### Requirement: CI Job Naming

The `ci.yml` workflow's job MUST be named `Build and Test` using the `name:`
field. The job key SHOULD be `build-and-test`.

Previously: The job was named `test` with no explicit `name:` field, appearing
in the GitHub UI as `test`.

#### Scenario: CI check appears with correct name

- **GIVEN** the `ci.yml` job has `name: Build and Test`
- **WHEN** CI runs on a push or pull request
- **THEN** the check appears in the GitHub UI as `Build and Test`

### Requirement: Action SHA Pinning

All GitHub Actions `uses:` references in `ci.yml` and `release.yml` MUST
use full commit SHAs instead of mutable tags. Each pinned reference SHOULD
include a trailing comment with the human-readable version.

Previously: Actions used mutable floating tags (`@v4`, `@v5`, `@v6`).

#### Scenario: Pinned action reference format

- **GIVEN** a workflow step uses `actions/checkout`
- **WHEN** the workflow file is reviewed
- **THEN** the reference is in the format
  `actions/checkout@<40-char-sha>  # v6.0.3` (SHA + version comment)

### Requirement: CI Workflow Hardening

The `ci.yml` workflow MUST include:
- `permissions: contents: read` at workflow level
- `concurrency:` group with `cancel-in-progress: true` to prevent duplicate
  runs on rapid pushes

Previously: The workflow had no `permissions:` or `concurrency:` blocks.

#### Scenario: Concurrent CI runs are cancelled

- **GIVEN** a PR has a CI run in progress
- **WHEN** a new commit is pushed to the same PR
- **THEN** the in-progress run is cancelled and a new run starts

### Requirement: Per-Job Permissions in Release Workflow

The `release.yml` workflow MUST set `permissions: {}` at workflow level and
declare permissions per-job:
- `preflight`: `contents: write`, `checks: read`
- `release`: `contents: write`, `id-token: write`
- `sign-macos`: `contents: write`

Previously: The workflow had `permissions: contents: write` at workflow level,
granting write access to all jobs including those that only need read access.

#### Scenario: Preflight job has minimal permissions

- **GIVEN** the `preflight` job only needs to push a tag and read check
  statuses
- **WHEN** the job runs
- **THEN** it has only `contents: write` and `checks: read` permissions

### Requirement: Release Tag Reference

All references to `GITHUB_REF_NAME` or `github.ref_name` in the `release` and
`sign-macos` jobs MUST be replaced with `inputs.tag` (via a job-level
`env: RELEASE_TAG`) since `workflow_dispatch` sets `GITHUB_REF_NAME` to the
branch name, not the tag.

Previously: Jobs used `${{ github.ref_name }}` and `${GITHUB_REF_NAME}` which
resolved to the tag name under the `push: tags:` trigger.

#### Scenario: Release job uses correct tag

- **GIVEN** the workflow is triggered via `workflow_dispatch` with
  `tag: v1.2.0` from the `main` branch
- **WHEN** the release job runs
- **THEN** all tag references resolve to `v1.2.0`, not `main`

## REMOVED Requirements

None.
<!-- scaffolded by uf vdev -->
