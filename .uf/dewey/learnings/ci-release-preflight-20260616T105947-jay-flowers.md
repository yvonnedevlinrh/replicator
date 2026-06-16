---
tag: ci-release-preflight
author: jay-flowers
category: pattern
created_at: 2026-06-16T10:59:47Z
identity: ci-release-preflight-20260616T105947-jay-flowers
tier: draft
---

When adapting a canonical reference workflow from another repo (e.g., unbound-force/unbound-force release.yml), the spec review council will catch gaps that the canonical reference handles implicitly. In the ci-release-preflight change, the canonical reference's tag uniqueness check works because the preflight creates the tag — but the spec initially had a contradiction between "tag must not exist" and "tag creation must be idempotent for re-runs." The resolution was to make the uniqueness check HEAD-aware: if the tag exists and points to HEAD, it's a re-run (pass). If it points to a different commit, it's a genuine duplicate (fail). Always specify the re-run idempotency mechanism explicitly when adapting preflight patterns, because the canonical reference may handle it through implicit ordering that isn't obvious from reading the workflow file alone.
