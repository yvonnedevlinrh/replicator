<!--
  [P] marks tasks eligible for parallel execution.
  Add [P] when a task: (a) touches different files from
  other [P] tasks in the group, (b) has no dependency
  on prior tasks in the group, (c) can safely execute
  without ordering constraints.
  Do NOT add [P] when tasks modify the same file —
  parallel workers will cause merge conflicts.
  Tasks without [P] run sequentially first, then [P]
  tasks run in parallel.
-->

## 1. Harden CI Workflow

All tasks in this group modify `.github/workflows/ci.yml`.

- [x] 1.1 Add `permissions: contents: read` block after `on:` section.
- [x] 1.2 Add `concurrency:` group (`${{ github.workflow }}-${{ github.ref }}`) with `cancel-in-progress: true`.
- [x] 1.3 Rename job key from `test` to `build-and-test` and add `name: Build and Test`.
- [x] 1.4 Pin `actions/checkout@v4` to `actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10` with `# v6.0.3` comment.
- [x] 1.5 Pin `actions/setup-go@v5` to `actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c` with `# v6.4.0` comment.

## 2. Add Preflight Job to Release Workflow

All tasks in this group modify `.github/workflows/release.yml`.

- [x] 2.1 Replace `on: push: tags: ['v*']` with `on: workflow_dispatch:` with `inputs: tag:` (required, type string, description `'Release tag (e.g., v0.2.0)'`).
- [x] 2.2 Replace workflow-level `permissions: contents: write` with `permissions: {}`.
- [x] 2.3 Add `concurrency: group: release-${{ github.ref }}, cancel-in-progress: false`.
- [x] 2.4 Add `preflight` job before the existing `release` job with `permissions: contents: write` and `checks: read`, `timeout-minutes: 10`, and `env: RELEASE_TAG: ${{ inputs.tag }}`.
- [x] 2.5 Add preflight step: Checkout with `fetch-depth: 0` using pinned checkout SHA.
- [x] 2.6 Add preflight step: Validate branch — reject if not triggered from `main` (`github.ref != 'refs/heads/main'`).
- [x] 2.7 Add preflight step: Validate tag format (`^v[0-9]+\.[0-9]+\.[0-9]+$`).
- [x] 2.8 Add preflight step: Check tag uniqueness via `git ls-remote --tags origin`. If the tag exists and points to HEAD, pass (re-run case). If it exists on a different commit, fail.
- [x] 2.9 Add preflight step: Verify semver ordering using `sort -V` comparison against latest tag. If no existing tags, skip (first release).
- [x] 2.10 Add preflight step: Verify CI passed on HEAD — query GitHub Checks API for `"Build and Test"` check with `success` conclusion. Fail if API call fails or check has not concluded.
- [x] 2.11 Add preflight step: Verify unreleased commits since last tag.
- [x] 2.12 Add preflight step: Create and push annotated tag (idempotent — check `git ls-remote` first, skip if tag already exists on HEAD).
- [x] 2.13 Move `check-secrets` step from `release` job into `preflight` job. Rename step output from `has_secrets` to `has_signing_secrets` for clarity. Add `has_signing_secrets` as preflight job output.

## 3. Update Release Job

All tasks in this group modify `.github/workflows/release.yml`.

- [x] 3.1 Add `needs: preflight` to the `release` job.
- [x] 3.2 Add per-job permissions: `contents: write`, `id-token: write`.
- [x] 3.3 Add `env: RELEASE_TAG: ${{ inputs.tag }}` at job level.
- [x] 3.4 Add `ref: ${{ inputs.tag }}` to the checkout step.
- [x] 3.5 Add `GORELEASER_CURRENT_TAG: ${{ inputs.tag }}` to the GoReleaser step env.
- [x] 3.6 Pin `actions/checkout` to `df4cb1c069e1874edd31b4311f1884172cec0e10` with `# v6.0.3` comment.
- [x] 3.7 Pin `actions/setup-go` to `4a3601121dd01d1626a1e23e37211e3254c1c06c` with `# v6.4.0` comment.
- [x] 3.8 Pin `goreleaser/goreleaser-action` to `5daf1e915a5f0af01ddbcd89a43b8061ff4f1a89` with `# v7.2.2` comment.
- [x] 3.9 Replace `${GITHUB_REF_NAME}` with `$RELEASE_TAG` in the cask upload step.
- [x] 3.10 Forward `has_signing_secrets` output from preflight via `outputs: has_signing_secrets: ${{ needs.preflight.outputs.has_signing_secrets }}`.

## 4. Update sign-macos Job

All tasks in this group modify `.github/workflows/release.yml`.

- [x] 4.1 Add per-job permissions: `contents: write`.
- [x] 4.2 Add `env: RELEASE_TAG: ${{ inputs.tag }}` at job level.
- [x] 4.3 Replace all `${GITHUB_REF_NAME}` references with `$RELEASE_TAG` in: download step, sign step, asset replacement step, and Homebrew cask update step.
- [x] 4.4 Update the `if:` condition to reference `needs.release.outputs.has_signing_secrets`.

## 5. Verification

- [x] 5.1 Verify all `uses:` references in both workflow files use full commit SHAs (no floating tags remain).
- [x] 5.2 Verify no `GITHUB_REF_NAME` or `github.ref_name` references remain in `release.yml` (except in the `concurrency:` section where `github.ref` is intentionally used for per-branch serialization).
- [x] 5.3 Verify `has_signing_secrets` output name is consistent across preflight job output, release job forwarding, and sign-macos `if:` condition.
- [x] 5.4 Verify the `release` job has `needs: preflight` and `sign-macos` has `needs: release`.
- [x] 5.5 Verify constitution alignment: Observable Quality — preflight produces `::error::` annotations for machine-parseable feedback. Composability First — binary functionality is unaffected by workflow changes.
- [x] 5.6 Run YAML validation on both workflow files (`actionlint` or `yamllint` if available).
- [x] 5.7 Verify website documentation gate exemption: this is a CI/CD pipeline change (exempt per AGENTS.md).

## 6. Documentation Updates

- [x] 6.1 Update `.specify/memory/constitution.md` Development Workflow section (line ~101) to reflect the new release trigger: replace "Tag `v*` triggers GoReleaser" with "workflow_dispatch with tag input triggers preflight validation then GoReleaser."
<!-- spec-review: passed -->
<!-- code-review: passed -->
<!-- scaffolded by uf vdev -->
