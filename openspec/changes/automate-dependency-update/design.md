## Context

Replicator already has weekly Dependabot entries for the `gomod` and `github-actions` ecosystems in `.github/dependabot.yml`. That configuration was delivered by `ci-security-hygiene` and needs no change. The missing behavior is a consumer workflow that evaluates dependency changes, reports the result consistently, and approves only sufficiently old, low- or medium-risk Dependabot updates that pass dependency review and have no active human change request in each reviewer's latest effective state.

The organization reference workflow in `unbound-force/unbound-force` supplies the intended job topology. At design time, the relevant tags resolve to:

- `complytime/org-infra` v0.8.0: `bd3718a218d649b093269fe4a979c4a4632dfad2`
- `peter-evans/create-or-update-comment` v5.0.0: `e8674b075228eee787fea43ef493e45ece1004c9`
- `actions/github-script` v9.0.0: `3a2844b7e9c422d3c10d287c895573f7108da1b3`

Implementation must re-resolve these tags before writing the workflow because CI-002 requires authoring-time verification. The workflow changes repository automation only; it does not change the Replicator binary, data model, MCP behavior, or runtime dependencies.

As established in the proposal's constitution assessment, this design preserves artifact-based collaboration through pull-request comments and reviews, keeps runtime use standalone, exposes review evidence, and defines local structural verification plus observable GitHub activation checks.

## Goals / Non-Goals

### Goals
- Add a convention-compliant `.github/workflows/ci_dependencies.yml` consumer workflow.
- Run the org-infra general and Dependabot-specific reusable reviewers using verified full-SHA references.
- Produce one replaceable, standardized review summary for Dependabot pull requests.
- Auto-approve only when all safety predicates pass, including an in-script human `CHANGES_REQUESTED` check.
- Keep all missing, malformed, or unsafe reviewer outputs on the manual-review path.
- Restrict write permissions to the comment and approval jobs that need them.
- Preserve the existing Dependabot proposal policy, CI quality gates, branch protection, and code-owner review.

### Non-Goals
- Adding or changing `.github/dependabot.yml`.
- Automatically merging or enabling auto-merge for any dependency update.
- Bypassing required checks, code-owner review, or repository branch protection.
- Replacing `govulncheck` or making dependency review a new required status check.
- Changing org-infra reusable workflow implementations.
- Adding production Go code or runtime dependencies.

## Decisions

### 1. Adapt the organization reference workflow without auto-merge

Create `ci_dependencies.yml` with the canonical four-job shape:

1. `call_deps_reviewer` invokes `reusable_deps_reviewer.yml`.
2. `call_dependabot_reviewer` invokes `reusable_dependabot_reviewer.yml`.
3. `comment_on_dependabot_prs` consumes both jobs' outputs and writes a standardized comment only for Dependabot pull requests.
4. `approve_dependabot_prs` consumes both jobs' outputs and may submit an approving review only for Dependabot pull requests.

The implementation will follow the conservative `unbound-force/unbound-force` behavior rather than the `complyctl` variant: it will not special-case organization-owned dependencies and will not enable auto-merge. This keeps issue #38's scope to review and approval and avoids weakening branch-protection intent.

### 2. Match repository event and concurrency conventions

The workflow will run on pushes to `main` and use `pull_request` for pull requests targeting `main`, matching the proven ComplyTime reference workflow without introducing the privileged `pull_request_target` event. Pull-request mutation jobs will have an explicit Dependabot actor condition and will not check out or execute pull-request code while holding write permission. Review jobs remain read-only, so push events and human-authored pull requests can run analysis without receiving Dependabot comments or approvals. A pull-request/ref concurrency group with `cancel-in-progress: true` will prevent redundant runs.

### 3. Use full-SHA pins and job-scoped least privilege

The workflow-level permissions will be read-only or explicitly none: `contents: read`, `issues: none`, and `pull-requests: none`. Reusable review jobs inherit only analysis permissions. The comment job receives `issues: read` and `pull-requests: write`; the approval job receives only `pull-requests: write`.

All `uses` references will be pinned to implementation-time-verified 40-character SHAs with accurate version comments. No secrets are added. This satisfies CI-001 through CI-003 and CI-020 through CI-022.

### 4. Treat reusable workflow outputs as untrusted approval inputs

The approval step's outer expression will gate obvious ineligible cases: general review must equal `success`, risk must be low or medium, and release age must be present and at least 24 hours. The script will receive reviewer outputs through environment variables rather than interpolate them into JavaScript source.

The script will validate the expected values again and list pull-request reviews. It will group non-bot reviews by reviewer login, sort them chronologically, ignore non-decisive `COMMENTED` records, and evaluate each reviewer's latest decisive state. A latest `CHANGES_REQUESTED` state blocks approval; a later `APPROVED` or `DISMISSED` state clears that reviewer's earlier veto. This second validation keeps malformed values and current API-derived vetoes on the fail-closed path. The created review body will include risk, review conclusion, and release age as decision evidence.

### 5. Keep reporting idempotent and explicit

The reporting job will use `create-or-update-comment` with `edit-mode: replace` and a job condition equivalent to `always()` plus the Dependabot pull-request guard. It therefore runs when either reusable reviewer fails or is cancelled. Its table will include the general review conclusion, calculated risk, dependency name/version, release age, and workflow-log link, substituting explicit unavailable values for absent outputs. When static predicates pass, the summary will say `Eligible for automated approval pending live review-state validation`; it will never claim that approval occurred. Unknown release age, failed dependencies, absent outputs, or any failed predicate will say `Manual review required`.

The approval script and the resulting GitHub review are authoritative for the live human-veto decision. The comment describes reviewer-output eligibility only and directs maintainers to the run logs. No second parent comment is introduced, and the approval job remains unable to run successfully when either reusable reviewer fails or is cancelled.

### 6. Verify structure locally and behavior after activation

Before completion, implementation will:

- Add an automated offline regression test before the workflow implementation and confirm that it initially fails.
- Make the test parse or structurally evaluate `ci_dependencies.yml` and exercise fixtures for eligible updates, high-risk or vulnerable updates, releases younger than 24 hours, missing or malformed outputs, failed or cancelled reviewer jobs, non-Dependabot authors, idempotent reporting, active human vetoes, later approvals that clear earlier vetoes, dismissed vetoes, and comments that do not clear active vetoes.
- Run a pinned workflow linter or equivalent parser against `ci_dependencies.yml`.
- Assert the expected triggers, four jobs, actor guards, output wiring, permission blocks, `always()` reporting behavior, 24-hour threshold, fail-closed predicates, latest-effective-review lookup, eligibility wording, and absence of merge commands.
- Re-resolve every tag-to-SHA mapping and compare it with the written references.
- Run the repository's CI-equivalent commands derived from `.github/workflows/ci.yml`, including vet, pinned `govulncheck`, Homebrew script tests, race-enabled Go tests, coverage ratchets, and build.
- Run `openspec validate automate-dependency-update`.

After the workflow is pushed, maintainers will verify one normal pull request and representative Dependabot outcomes through GitHub Actions. GitHub-hosted comment and approval mutations cannot be executed in an isolated local test, so activation evidence must be recorded in the implementing pull request rather than simulated with live external writes during local tests.

## Risks / Trade-offs

- **Write-token exposure**: Comment and approval jobs require `pull-requests: write`. Job-level permissions and Dependabot-only conditions limit exposure; no checkout or execution of pull-request code occurs in those jobs.
- **Reusable workflow drift**: SHA pinning prevents unreviewed upstream changes but requires deliberate upgrades. Dependabot's existing GitHub Actions ecosystem will propose pin updates.
- **Output contract drift**: Renamed or malformed org-infra outputs could break the comment or approval expression. Explicit checks fail closed and leave the pull request for manual review.
- **Approval versus branch protection**: A bot approval may not satisfy code-owner review, and repository settings may prohibit Actions approvals. This is acceptable: the workflow reports failure and branch protection remains authoritative.
- **Comment/approval timing**: A human may request changes after bot approval. Branch protection and code-owner requirements still govern merge; this workflow does not dismiss reviews or merge automatically.
- **Hosted behavior cannot be fully tested offline**: Static linting and predicate checks cover configuration regressions, while the implementing pull request must capture post-activation evidence for GitHub-only side effects.
<!-- scaffolded by uf v0.17.0 -->
