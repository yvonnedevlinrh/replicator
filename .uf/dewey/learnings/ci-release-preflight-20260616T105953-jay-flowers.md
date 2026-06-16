---
tag: ci-release-preflight
author: jay-flowers
category: pattern
created_at: 2026-06-16T10:59:53Z
identity: ci-release-preflight-20260616T105953-jay-flowers
tier: draft
---

When switching a GitHub Actions release workflow from push-tag trigger to workflow_dispatch, there are several non-obvious impacts that the spec review council will flag: (1) GITHUB_REF_NAME becomes the branch name instead of the tag — all references must change to inputs.tag, (2) github.ref in concurrency groups becomes the branch ref not the tag — this is actually correct for serializing releases from the same branch, but must be documented, (3) the workflow can be triggered from any branch, not just main — a branch validation step is essential to prevent releases from feature branches, (4) the constitution or project documentation that describes the release process becomes stale. The spec review found 7 HIGH-severity gaps in the initial spec that all related to these implicit consequences of the trigger change.
