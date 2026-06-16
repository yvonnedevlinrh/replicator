---
tag: ci-release-preflight
author: jay-flowers
category: gotcha
created_at: 2026-06-16T10:59:42Z
identity: ci-release-preflight-20260616T105942-jay-flowers
tier: draft
---

When writing GitHub Actions workflow steps that reference context values like github.ref, github.event.pull_request.title, or inputs.tag, always pass them through env: bindings rather than interpolating them directly into shell scripts with ${{ }}. The ${{ }} syntax performs textual substitution before the shell runs, which creates a shell injection vector. The correct pattern is to bind the value to an environment variable in the step's env: block and reference it as $VAR_NAME in the shell. This was caught during the ci-release-preflight code review when the Adversary reviewer identified that github.ref was interpolated directly into a bash if-statement while inputs.tag was correctly passed via env: RELEASE_TAG in the same file — the inconsistency made the injection risk visible.
