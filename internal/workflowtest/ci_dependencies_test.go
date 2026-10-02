// Package workflowtest verifies repository workflows against their approved policies.
package workflowtest

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	dependabotAuthor  = "dependabot[bot]"
	orgInfraSHA       = "bd3718a218d649b093269fe4a979c4a4632dfad2"
	approvalScriptSHA = "3a2844b7e9c422d3c10d287c895573f7108da1b3"
	commentActionSHA  = "e8674b075228eee787fea43ef493e45ece1004c9"
	minimumReleaseAge = "24"
	minimumSHAPins    = 4
	reviewedHeadSHA   = "0123456789abcdef0123456789abcdef01234567"
)

type policyFixture struct {
	name                   string
	eventName              string
	author                 string
	depsReviewResult       string
	reviewConclusion       string
	omitReviewConclusion   bool
	dependabotReviewResult string
	risk                   string
	releaseAge             string
	dependency             string
	version                string
	omitDependency         bool
	omitVersion            bool
	reviews                []reviewFixture
	currentHeadSHA         string
	wantApproval           bool
	wantFailure            string
	wantThrown             string
}

type reviewFixture struct {
	ID          int64       `json:"id"`
	State       string      `json:"state"`
	SubmittedAt string      `json:"submitted_at"`
	User        userFixture `json:"user"`
}

type userFixture struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

type scriptResult struct {
	Approvals []struct {
		Event    string `json:"event"`
		Body     string `json:"body"`
		CommitID string `json:"commit_id"`
	} `json:"approvals"`
	Failures []string          `json:"failures"`
	Thrown   string            `json:"thrown"`
	Outputs  map[string]string `json:"outputs"`
	Sentinel string            `json:"sentinel"`
}

type reportFixture struct {
	name                    string
	depsReviewResult        string
	reviewConclusion        string
	dependabotReviewResult  string
	risk                    string
	dependency              string
	version                 string
	releaseAge              string
	existingReportCommentID int64
	wantEligibility         string
	wantReviewConclusion    string
	wantRisk                string
	wantDependency          string
	wantVersion             string
	wantReleaseAge          string
}

func TestCIDependenciesWorkflow_StructureEnforcesGuardedApproval(t *testing.T) {
	workflow := readDependencyWorkflow(t)

	assertMatches(t, workflow, `(?m)^# .+\n# --\n# .+`, "purpose header")
	assertMatches(t, workflow, `(?ms)^on:\s*\n\s+push:\s*\n\s+branches:\s*\[main\]`, "main push trigger")
	assertMatches(t, workflow, `(?ms)^on:.*?\n\s+pull_request:\s*\n\s+branches:\s*\[main\]`, "main pull-request trigger")
	if strings.Contains(workflow, "pull_request_target") {
		t.Fatal("workflow must not use the insecure pull_request_target event")
	}
	assertMatches(t, workflow, `(?ms)^permissions:\s*\n\s+contents:\s+read\s*\n\s+issues:\s+none\s*\n\s+pull-requests:\s+none`, "read-only workflow permissions")
	assertMatches(t, workflow, `(?ms)^concurrency:\s*\n\s+group:\s+.*github\.workflow.*github\.event\.pull_request\.number.*github\.ref.*\n\s+cancel-in-progress:\s+true`, "PR- or ref-scoped concurrency")

	jobs := childKeys(t, yamlSection(t, workflow, "jobs", 0), 2)
	wantJobs := []string{
		"approve_dependabot_prs",
		"call_dependabot_reviewer",
		"call_deps_reviewer",
		"comment_on_dependabot_prs",
	}
	sort.Strings(jobs)
	if strings.Join(jobs, ",") != strings.Join(wantJobs, ",") {
		t.Fatalf("workflow jobs = %v, want exactly %v", jobs, wantJobs)
	}

	depsJob := yamlSection(t, workflow, "call_deps_reviewer", 2)
	dependabotJob := yamlSection(t, workflow, "call_dependabot_reviewer", 2)
	commentJob := yamlSection(t, workflow, "comment_on_dependabot_prs", 2)
	approvalJob := yamlSection(t, workflow, "approve_dependabot_prs", 2)

	assertContains(t, depsJob, "reusable_deps_reviewer.yml@"+orgInfraSHA, "general reusable reviewer")
	assertContains(t, dependabotJob, "reusable_dependabot_reviewer.yml@"+orgInfraSHA, "Dependabot reusable reviewer")
	assertFullSHAPins(t, workflow)

	assertContains(t, commentJob, "always()", "failure-tolerant reporting condition")
	assertMatches(t, commentJob, `(?m)^\s+&& github\.event_name == 'pull_request'$`, "pull-request reporting guard")
	assertMatches(t, commentJob, `(?m)^\s+&& github\.event\.pull_request\.user\.login == 'dependabot\[bot\]'$`, "Dependabot-only reporting guard")
	assertContains(t, commentJob, "needs.call_deps_reviewer.result", "general review result wiring")
	for _, output := range []string{"risk_level", "dep_name", "dep_version", "release_age_hours"} {
		assertContains(t, commentJob, "needs.call_dependabot_reviewer.outputs."+output, output+" output wiring")
	}
	assertPermissions(t, commentJob, map[string]string{"issues": "read", "pull-requests": "write"})
	assertContains(t, commentJob, "peter-evans/create-or-update-comment@"+commentActionSHA, "pinned comment action")
	assertMatches(t, commentJob, `(?m)^\s+edit-mode:\s+replace\s*$`, "idempotent report replacement")
	assertIdempotentReport(t, commentJob)

	commentBody := yamlLiteralBlock(t, commentJob, "body")
	for _, evidence := range []string{
		"Dependency review conclusion",
		"Risk",
		"Dependency",
		"Version",
		"Release age",
		"github.com/${{ github.repository }}/actions/runs/${{ github.run_id }}",
	} {
		assertContains(t, commentBody, evidence, "review report evidence")
	}
	assertContains(t, commentBody, "steps.prepare_report.outputs.eligibility", "generated eligibility outcome")
	assertContains(t, commentJob, "Eligible for automated approval pending live review-state validation", "eligible report outcome")
	assertContains(t, commentJob, "Manual review required", "manual-review report outcome")
	if strings.Contains(strings.ToLower(commentBody), "approved") {
		t.Fatal("review report must describe eligibility without claiming approval")
	}

	assertMatches(t, approvalJob, `(?m)^\s+&& github\.event_name == 'pull_request'$`, "pull-request approval guard")
	assertMatches(t, approvalJob, `(?m)^\s+&& github\.event\.pull_request\.user\.login == 'dependabot\[bot\]'$`, "Dependabot-only approval guard")
	assertContains(t, approvalJob, "needs.call_deps_reviewer.result", "approval review result predicate")
	assertContains(t, approvalJob, "needs.call_dependabot_reviewer.outputs.risk", "approval risk predicate")
	assertContains(t, approvalJob, "needs.call_dependabot_reviewer.outputs.release_age", "approval release-age predicate")
	assertPermissions(t, approvalJob, map[string]string{"pull-requests": "write"})
	assertContains(t, approvalJob, "actions/github-script@"+approvalScriptSHA, "pinned approval action")
	for _, environmentName := range []string{
		"PR_AUTHOR",
		"DEPS_REVIEW_RESULT",
		"REVIEW_CONCLUSION",
		"DEPENDABOT_REVIEW_RESULT",
		"RISK",
		"RELEASE_AGE",
		"EXPECTED_HEAD_SHA",
	} {
		assertMatches(t, approvalJob, `(?m)^\s+`+environmentName+`:\s+[^\n]+$`, environmentName+" environment input")
	}
	assertMatches(t, approvalJob, `(?m)^\s+EVENT_NAME:\s+\$\{\{ github\.event_name \}\}$`, "trusted event-name wiring")
	ifThreshold := regexp.MustCompile(`fromJSON\([^)]+release_age[^)]*\)\s*>=\s*(\d+)`).FindStringSubmatch(approvalJob)
	envThreshold := regexp.MustCompile(`(?m)^\s+MIN_RELEASE_AGE_HOURS:\s*(\d+)\s*$`).FindStringSubmatch(approvalJob)
	workflowThreshold := regexp.MustCompile(`(?m)^  MIN_RELEASE_AGE_HOURS:\s*(\d+)\s*$`).FindStringSubmatch(workflow)
	if ifThreshold == nil {
		t.Fatal("approval job if expression: release-age threshold not found")
	}
	if envThreshold == nil {
		t.Fatal("approval job env: MIN_RELEASE_AGE_HOURS not found")
	}
	if workflowThreshold == nil {
		t.Fatal("workflow env: MIN_RELEASE_AGE_HOURS not found")
	}
	for source, threshold := range map[string]string{
		"approval if expression": ifThreshold[1],
		"approval job env":       envThreshold[1],
		"workflow env":           workflowThreshold[1],
		"test harness":           minimumReleaseAge,
	} {
		if threshold != minimumReleaseAge {
			t.Errorf("release-age threshold drift: %s uses %s, want %s", source, threshold, minimumReleaseAge)
		}
	}

	assertNoMutationCapabilities(t, workflow)
}

func TestCIDependenciesWorkflow_ApprovalScriptEvaluatesPolicyFixtures(t *testing.T) {
	workflow := readDependencyWorkflow(t)
	approvalJob := yamlSection(t, workflow, "approve_dependabot_prs", 2)
	script := yamlLiteralBlock(t, approvalJob, "script")

	fixtures := []policyFixture{
		{
			name:   "eligible update",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "24", dependency: "example.org/module", version: "1.2.3", wantApproval: true,
		},
		{
			name:   "high risk update",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "high", releaseAge: "72",
		},
		{
			name:   "vulnerability reviewer failure",
			author: dependabotAuthor, depsReviewResult: "failure", dependabotReviewResult: "success",
			risk: "low", releaseAge: "72",
		},
		{
			name:   "release younger than 24 hours",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "23.99",
		},
		{
			name:   "missing risk",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			releaseAge: "48",
		},
		{
			name:                   "missing review result",
			author:                 dependabotAuthor,
			dependabotReviewResult: "success",
			risk:                   "low", releaseAge: "48",
		},
		{
			name:   "missing release age",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low",
		},
		{
			name:   "missing dependency name",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48", version: "1.2.3", omitDependency: true,
		},
		{
			name:   "missing dependency version",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48", dependency: "example.org/module", omitVersion: true,
		},
		{
			name:   "malformed release age",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "medium", releaseAge: "unknown",
		},
		{
			name:   "malformed risk",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "unexpected-risk", releaseAge: "48",
		},
		{
			name:   "malformed reviewer result",
			author: dependabotAuthor, depsReviewResult: "unknown", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48",
		},
		{
			name:   "missing review conclusion",
			author: dependabotAuthor, depsReviewResult: "success", omitReviewConclusion: true,
			dependabotReviewResult: "success", risk: "low", releaseAge: "48",
		},
		{
			name:   "malformed review conclusion",
			author: dependabotAuthor, depsReviewResult: "success", reviewConclusion: "unknown",
			dependabotReviewResult: "success", risk: "low", releaseAge: "48",
		},
		{
			name:   "failed reviewer job",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "failure",
			risk: "low", releaseAge: "48",
		},
		{
			name:   "cancelled reviewer job",
			author: dependabotAuthor, depsReviewResult: "cancelled", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48",
		},
		{
			name:   "non-Dependabot author",
			author: "human-maintainer", depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48",
		},
		{
			name:      "untrusted pull request target event",
			eventName: "pull_request_target", author: dependabotAuthor, depsReviewResult: "success",
			dependabotReviewResult: "success", risk: "low", releaseAge: "48",
		},
		{
			name:   "active CHANGES_REQUESTED",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48",
			reviews:     []reviewFixture{review(1, "alice", "User", "CHANGES_REQUESTED", "2026-09-15T08:00:00Z")},
			wantFailure: "Manual review required: active change requests from alice.",
			wantThrown:  "An active human change request blocks automated approval.",
		},
		{
			name:   "later APPROVED clears veto",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48", dependency: "example.org/module", version: "1.2.3", wantApproval: true,
			reviews: []reviewFixture{
				review(1, "alice", "User", "CHANGES_REQUESTED", "2026-09-15T08:00:00Z"),
				review(2, "alice", "User", "APPROVED", "2026-09-15T09:00:00Z"),
			},
		},
		{
			name:   "reversed review order uses timestamps",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48", dependency: "example.org/module", version: "1.2.3", wantApproval: true,
			reviews: []reviewFixture{
				review(2, "alice", "User", "APPROVED", "2026-09-15T09:00:00Z"),
				review(1, "alice", "User", "CHANGES_REQUESTED", "2026-09-15T08:00:00Z"),
			},
		},
		{
			name:   "one reviewer approval does not clear another reviewer veto",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48", dependency: "example.org/module", version: "1.2.3",
			reviews: []reviewFixture{
				review(1, "bob", "User", "CHANGES_REQUESTED", "2026-09-15T08:00:00Z"),
				review(2, "alice", "User", "APPROVED", "2026-09-15T09:00:00Z"),
			},
			wantFailure: "Manual review required: active change requests from bob.",
			wantThrown:  "An active human change request blocks automated approval.",
		},
		{
			name:   "DISMISSED clears veto",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "medium", releaseAge: "48", dependency: "example.org/module", version: "1.2.3", wantApproval: true,
			reviews: []reviewFixture{
				review(1, "alice", "User", "CHANGES_REQUESTED", "2026-09-15T08:00:00Z"),
				review(2, "alice", "User", "DISMISSED", "2026-09-15T09:00:00Z"),
			},
		},
		{
			name:   "COMMENTED does not clear veto",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48",
			reviews: []reviewFixture{
				review(1, "alice", "User", "CHANGES_REQUESTED", "2026-09-15T08:00:00Z"),
				review(2, "alice", "User", "COMMENTED", "2026-09-15T09:00:00Z"),
			},
			wantFailure: "Manual review required: active change requests from alice.",
			wantThrown:  "An active human change request blocks automated approval.",
		},
		{
			name:   "bot veto is not a human veto",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48", dependency: "example.org/module", version: "1.2.3", wantApproval: true,
			reviews: []reviewFixture{review(1, "reviewer[bot]", "Bot", "CHANGES_REQUESTED", "2026-09-15T08:00:00Z")},
		},
		{
			name:   "pull request head changed after review",
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: "48", currentHeadSHA: "fedcba9876543210fedcba9876543210fedcba98",
			wantFailure: "Manual review required: pull request head changed after dependency review.",
			wantThrown:  "Pull request head changed after dependency review.",
		},
	}
	for _, releaseAge := range []string{"24e0", "0x18", "+24", "Infinity", "NaN"} {
		fixtures = append(fixtures, policyFixture{
			name:   fmt.Sprintf("reject non-decimal release age %q", releaseAge),
			author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
			risk: "low", releaseAge: releaseAge,
		})
	}
	fixtures = append(fixtures, policyFixture{
		name:   "reject decimal overflow",
		author: dependabotAuthor, depsReviewResult: "success", dependabotReviewResult: "success",
		risk: "low", releaseAge: strings.Repeat("9", 400),
	})
	for index := range fixtures {
		if fixtures[index].dependency == "" && !fixtures[index].omitDependency {
			fixtures[index].dependency = "example.org/module"
		}
		if fixtures[index].version == "" && !fixtures[index].omitVersion {
			fixtures[index].version = "1.2.3"
		}
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			result := executeApprovalScript(t, script, fixture)
			if fixture.wantApproval && (len(result.Failures) != 0 || result.Thrown != "") {
				t.Errorf("successful approval had failures=%v thrown=%q", result.Failures, result.Thrown)
			}
			if !fixture.wantApproval && (len(result.Failures) == 0 || result.Thrown == "") {
				t.Errorf("unsafe policy did not fail closed: failures=%v thrown=%q", result.Failures, result.Thrown)
			}
			if !fixture.wantApproval {
				wantFailure := fixture.wantFailure
				wantThrown := fixture.wantThrown
				if wantFailure == "" {
					wantFailure = "Manual review required: dependency approval signals are incomplete or unsafe."
					wantThrown = "Dependency approval signals are incomplete or unsafe."
				}
				if len(result.Failures) != 1 || result.Failures[0] != wantFailure {
					t.Errorf("failure messages = %v, want exactly [%q]", result.Failures, wantFailure)
				}
				if result.Thrown != wantThrown {
					t.Errorf("thrown = %q, want %q", result.Thrown, wantThrown)
				}
			}
			gotApproval := len(result.Approvals) > 0
			if gotApproval != fixture.wantApproval {
				t.Errorf("approval created = %t, want %t (failures=%v, thrown=%q)", gotApproval, fixture.wantApproval, result.Failures, result.Thrown)
			}
			if !fixture.wantApproval {
				return
			}
			if len(result.Approvals) != 1 {
				t.Fatalf("approval count = %d, want 1", len(result.Approvals))
			}
			approval := result.Approvals[0]
			if approval.Event != "APPROVE" {
				t.Errorf("review event = %q, want APPROVE", approval.Event)
			}
			if approval.CommitID != reviewedHeadSHA {
				t.Errorf("review commit_id = %q, want %q", approval.CommitID, reviewedHeadSHA)
			}
			for _, evidence := range []string{fixture.depsReviewResult, fixture.risk, fixture.releaseAge, fixture.dependency, fixture.version} {
				if !strings.Contains(approval.Body, evidence) {
					t.Errorf("approval body %q does not contain decision evidence %q", approval.Body, evidence)
				}
			}
		})
	}
}

func TestCIDependenciesWorkflow_ReportScriptEvaluatesPolicyFixtures(t *testing.T) {
	workflow := readDependencyWorkflow(t)
	commentJob := yamlSection(t, workflow, "comment_on_dependabot_prs", 2)
	reportScript := yamlLiteralBlock(t, commentJob, "script")

	fixtures := []reportFixture{
		{
			name: "eligible update", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "24",
			wantEligibility:      "Eligible for automated approval pending live review-state validation",
			wantReviewConclusion: "success", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "24 hours",
		},
		{
			name: "eligible medium risk update", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "medium", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "48",
			wantEligibility:      "Eligible for automated approval pending live review-state validation",
			wantReviewConclusion: "success", wantRisk: "medium", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "48 hours",
		},
		{
			name: "high risk update", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "high", dependency: "example.org/module",
			version: "2.0.0", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "high", wantDependency: "example.org/module",
			wantVersion: "2.0.0", wantReleaseAge: "72 hours",
		},
		{
			name: "release younger than 24 hours", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "23.99", wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "23.99 hours",
		},
		{
			name: "unsuccessful review conclusion", depsReviewResult: "success", reviewConclusion: "failure",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "failure", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "72 hours",
		},
		{
			name: "failed general reviewer", depsReviewResult: "failure", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "unavailable", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "72 hours",
		},
		{
			name: "cancelled general reviewer", depsReviewResult: "cancelled", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "unavailable", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "72 hours",
		},
		{
			name: "missing or empty general reviewer result", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "unavailable", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "72 hours",
		},
		{
			name: "unexpected general reviewer result", depsReviewResult: "unknown", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "unavailable", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "72 hours",
		},
		{
			name: "failed Dependabot reviewer", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "failure", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "unavailable", wantDependency: "unavailable",
			wantVersion: "unavailable", wantReleaseAge: "unavailable",
		},
		{
			name: "cancelled Dependabot reviewer", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "cancelled", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "unavailable", wantDependency: "unavailable",
			wantVersion: "unavailable", wantReleaseAge: "unavailable",
		},
		{
			name: "missing or empty Dependabot reviewer result", depsReviewResult: "success", reviewConclusion: "success",
			risk: "low", dependency: "example.org/module", version: "1.2.3", releaseAge: "72",
			wantEligibility: "Manual review required", wantReviewConclusion: "success", wantRisk: "unavailable",
			wantDependency: "unavailable", wantVersion: "unavailable", wantReleaseAge: "unavailable",
		},
		{
			name: "unexpected Dependabot reviewer result", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "unknown", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "72", wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "unavailable", wantDependency: "unavailable",
			wantVersion: "unavailable", wantReleaseAge: "unavailable",
		},
		{
			name: "missing or empty review conclusion", depsReviewResult: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "48", wantEligibility: "Manual review required",
			wantReviewConclusion: "unavailable", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "48 hours",
		},
		{
			name: "malformed or unexpected review conclusion", depsReviewResult: "success", reviewConclusion: "unknown",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "48", wantEligibility: "Manual review required",
			wantReviewConclusion: "unknown", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "48 hours",
		},
		{
			name: "missing or empty risk", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", dependency: "example.org/module", version: "1.2.3",
			releaseAge: "48", wantEligibility: "Manual review required", wantReviewConclusion: "success",
			wantRisk: "unavailable", wantDependency: "example.org/module", wantVersion: "1.2.3",
			wantReleaseAge: "48 hours",
		},
		{
			name: "malformed or unexpected risk", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "unexpected-risk", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "48", wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "unexpected-risk", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "48 hours",
		},
		{
			name: "missing or empty dependency name", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", version: "1.2.3", releaseAge: "48",
			wantEligibility: "Manual review required", wantReviewConclusion: "success", wantRisk: "low",
			wantDependency: "unavailable", wantVersion: "1.2.3", wantReleaseAge: "48 hours",
		},
		{
			name: "missing or empty dependency version", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module", releaseAge: "48",
			wantEligibility: "Manual review required", wantReviewConclusion: "success", wantRisk: "low",
			wantDependency: "example.org/module", wantVersion: "unavailable", wantReleaseAge: "48 hours",
		},
		{
			name: "missing or empty release age", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "medium", dependency: "example.org/module",
			version: "1.2.3", wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "medium", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "unavailable",
		},
		{
			name: "malformed or unexpected release age", depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: "unknown", existingReportCommentID: 42,
			wantEligibility: "Manual review required", wantReviewConclusion: "success", wantRisk: "low",
			wantDependency: "example.org/module", wantVersion: "1.2.3", wantReleaseAge: "unavailable",
		},
	}
	for _, releaseAge := range []string{"24e0", "0x18", "+24", "Infinity", "NaN"} {
		fixtures = append(fixtures, reportFixture{
			name: "reject non-decimal release age " + releaseAge, depsReviewResult: "success", reviewConclusion: "success",
			dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
			version: "1.2.3", releaseAge: releaseAge, wantEligibility: "Manual review required",
			wantReviewConclusion: "success", wantRisk: "low", wantDependency: "example.org/module",
			wantVersion: "1.2.3", wantReleaseAge: "unavailable",
		})
	}
	fixtures = append(fixtures, reportFixture{
		name: "reject decimal overflow", depsReviewResult: "success", reviewConclusion: "success",
		dependabotReviewResult: "success", risk: "low", dependency: "example.org/module",
		version: "1.2.3", releaseAge: strings.Repeat("9", 400), wantEligibility: "Manual review required",
		wantReviewConclusion: "success", wantRisk: "low", wantDependency: "example.org/module",
		wantVersion: "1.2.3", wantReleaseAge: "unavailable",
	})

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			result := executeReportScript(t, reportScript, fixture)
			if result.Thrown != "" || len(result.Failures) != 0 {
				t.Fatalf("report script failed: failures=%v thrown=%q", result.Failures, result.Thrown)
			}
			assertOutputEquals(t, result.Outputs, "eligibility", fixture.wantEligibility)
			assertOutputEquals(t, result.Outputs, "review-conclusion", fixture.wantReviewConclusion)
			assertOutputEquals(t, result.Outputs, "risk", fixture.wantRisk)
			assertOutputEquals(t, result.Outputs, "dependency", fixture.wantDependency)
			assertOutputEquals(t, result.Outputs, "version", fixture.wantVersion)
			assertOutputEquals(t, result.Outputs, "release-age", fixture.wantReleaseAge)
			wantCommentID := ""
			if fixture.existingReportCommentID != 0 {
				wantCommentID = fmt.Sprint(fixture.existingReportCommentID)
			}
			assertOutputEquals(t, result.Outputs, "comment-id", wantCommentID)
		})
	}
}

func TestPolicyEnvironment_ReplacesApprovalScriptInputs(t *testing.T) {
	environment := policyEnvironment(policyFixture{})

	for _, value := range environment {
		if value == "DEPENDENCY=host-dependency" || value == "VERSION=host-version" || value == "UNRELATED=value" {
			t.Errorf("policy environment leaks host approval-script input %q", value)
		}
	}
}

func TestWorkflowScriptHarnesses_DoNotInheritHostEnvironment(t *testing.T) {
	t.Setenv("WORKFLOWTEST_HOST_SENTINEL", "must-not-reach-node")
	workflow := readDependencyWorkflow(t)
	approvalScript := yamlLiteralBlock(t, yamlSection(t, workflow, "approve_dependabot_prs", 2), "script")
	reportScript := yamlLiteralBlock(t, yamlSection(t, workflow, "comment_on_dependabot_prs", 2), "script")

	approvalResult := executeApprovalScript(t, approvalScript, policyFixture{})
	if approvalResult.Sentinel != "" {
		t.Errorf("approval harness inherited host sentinel %q", approvalResult.Sentinel)
	}
	reportResult := executeReportScript(t, reportScript, reportFixture{})
	if reportResult.Sentinel != "" {
		t.Errorf("report harness inherited host sentinel %q", reportResult.Sentinel)
	}
}

func readDependencyWorkflow(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: could not determine test source path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", ".github", "workflows", "ci_dependencies.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dependency workflow %q: %v", path, err)
	}
	return string(content)
}

func review(id int64, login, userType, state, submittedAt string) reviewFixture {
	return reviewFixture{
		ID:          id,
		State:       state,
		SubmittedAt: submittedAt,
		User:        userFixture{Login: login, Type: userType},
	}
}

func executeApprovalScript(t *testing.T, script string, fixture policyFixture) scriptResult {
	t.Helper()
	nodePath := requireNode(t)

	currentHeadSHA := fixture.currentHeadSHA
	if currentHeadSHA == "" {
		currentHeadSHA = reviewedHeadSHA
	}
	fixtureJSON, err := json.Marshal(map[string]any{
		"author":         fixture.author,
		"reviews":        fixture.reviews,
		"currentHeadSHA": currentHeadSHA,
	})
	if err != nil {
		t.Fatalf("marshal policy fixture: %v", err)
	}

	harness := fmt.Sprintf(`
const fixture = JSON.parse(process.env.POLICY_FIXTURE);
const approvals = [];
const failures = [];
const github = {
  paginate: async () => fixture.reviews || [],
  request: async (route, request) => {
    if (String(route).startsWith("GET ")) return {data: fixture.reviews || []};
    if (String(route).startsWith("POST ")) { approvals.push(request); return {data: request}; }
    throw new Error("unexpected GitHub API route: " + route);
  },
  rest: {
    pulls: {
      get: async () => ({data: {head: {sha: fixture.currentHeadSHA}}}),
      listReviews: async () => ({data: fixture.reviews || []}),
      createReview: async (request) => { approvals.push(request); return {data: request}; }
    }
  }
};
const context = {
  repo: {owner: "unbound-force", repo: "replicator"},
  issue: {number: 38},
  payload: {pull_request: {number: 38, user: {login: fixture.author}}}
};
const core = {
  setFailed: (message) => failures.push(String(message)),
  info: () => {}, warning: () => {}, notice: () => {}, debug: () => {}
};
const console = {log: () => {}, error: () => {}, warn: () => {}};
(async () => {
  let thrown = "";
  try {
%s
  } catch (error) {
    thrown = String(error && error.message ? error.message : error);
  }
  process.stdout.write(JSON.stringify({approvals, failures, thrown, sentinel: process.env.WORKFLOWTEST_HOST_SENTINEL || ""}));
})();
`, indent(script, "    "))

	cmd := exec.Command(nodePath, "-e", harness)
	cmd.Env = append(policyEnvironment(fixture), "POLICY_FIXTURE="+string(fixtureJSON))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execute approval github-script policy: %v\n%s", err, output)
	}

	var result scriptResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode approval github-script result: %v\n%s", err, output)
	}
	return result
}

func executeReportScript(t *testing.T, script string, fixture reportFixture) scriptResult {
	t.Helper()
	nodePath := requireNode(t)

	comments := []map[string]any{}
	if fixture.existingReportCommentID != 0 {
		comments = append(comments, map[string]any{
			"id":   fixture.existingReportCommentID,
			"user": map[string]string{"login": "github-actions[bot]"},
			"body": "<!-- dependency-review-report -->",
		})
	}
	fixtureJSON, err := json.Marshal(map[string]any{"comments": comments})
	if err != nil {
		t.Fatalf("marshal report fixture: %v", err)
	}

	harness := fmt.Sprintf(`
const fixture = JSON.parse(process.env.REPORT_FIXTURE);
const failures = [];
const outputs = {};
const github = {
  paginate: async () => fixture.comments || [],
  rest: {issues: {listComments: async () => ({data: fixture.comments || []})}}
};
const context = {
  repo: {owner: "unbound-force", repo: "replicator"},
  issue: {number: 38}
};
const core = {
  setOutput: (name, value) => { outputs[String(name)] = String(value); },
  setFailed: (message) => failures.push(String(message))
};
(async () => {
  let thrown = "";
  try {
%s
  } catch (error) {
    thrown = String(error && error.message ? error.message : error);
  }
  process.stdout.write(JSON.stringify({failures, outputs, thrown, sentinel: process.env.WORKFLOWTEST_HOST_SENTINEL || ""}));
})();
`, indent(script, "    "))

	cmd := exec.Command(nodePath, "-e", harness)
	cmd.Env = append(reportEnvironment(fixture), "REPORT_FIXTURE="+string(fixtureJSON))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("execute report github-script policy: %v\n%s", err, output)
	}
	var result scriptResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("decode report github-script result: %v\n%s", err, output)
	}
	return result
}

func requireNode(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("workflow JavaScript policy execution is skipped in short mode")
	}
	nodePath, err := exec.LookPath("node")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("node is required in CI to execute GitHub workflow policy: %v", err)
		}
		t.Skipf("node is unavailable; CI provides Node to execute GitHub workflow policy: %v", err)
	}
	return nodePath
}

func policyEnvironment(fixture policyFixture) []string {
	eventName := fixture.eventName
	if eventName == "" {
		eventName = "pull_request"
	}
	reviewConclusion := fixture.reviewConclusion
	if reviewConclusion == "" && !fixture.omitReviewConclusion {
		reviewConclusion = fixture.depsReviewResult
	}
	values := map[string]string{
		"EVENT_NAME":               eventName,
		"PR_AUTHOR":                fixture.author,
		"EXPECTED_HEAD_SHA":        reviewedHeadSHA,
		"DEPS_REVIEW_RESULT":       fixture.depsReviewResult,
		"REVIEW_CONCLUSION":        reviewConclusion,
		"DEPENDABOT_REVIEW_RESULT": fixture.dependabotReviewResult,
		"RISK":                     fixture.risk,
		"DEPENDENCY":               fixture.dependency,
		"VERSION":                  fixture.version,
		"RELEASE_AGE":              fixture.releaseAge,
		"MIN_RELEASE_AGE_HOURS":    minimumReleaseAge,
	}
	environment := make([]string, 0, len(values))
	for name, value := range values {
		if value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func reportEnvironment(fixture reportFixture) []string {
	values := map[string]string{
		"DEPS_REVIEW_RESULT":       fixture.depsReviewResult,
		"REVIEW_CONCLUSION":        fixture.reviewConclusion,
		"DEPENDABOT_REVIEW_RESULT": fixture.dependabotReviewResult,
		"RISK":                     fixture.risk,
		"DEPENDENCY":               fixture.dependency,
		"VERSION":                  fixture.version,
		"RELEASE_AGE":              fixture.releaseAge,
		"MIN_RELEASE_AGE_HOURS":    minimumReleaseAge,
	}
	environment := make([]string, 0, len(values))
	for name, value := range values {
		if value != "" {
			environment = append(environment, name+"="+value)
		}
	}
	return environment
}

func assertOutputEquals(t *testing.T, outputs map[string]string, name, want string) {
	t.Helper()
	if got := outputs[name]; got != want {
		t.Errorf("output %q = %q, want %q", name, got, want)
	}
}

func yamlSection(t *testing.T, content, key string, indentWidth int) string {
	t.Helper()
	lines := strings.Split(content, "\n")
	prefix := strings.Repeat(" ", indentWidth) + key + ":"
	start := -1
	for index, line := range lines {
		if line == prefix {
			start = index
			break
		}
	}
	if start < 0 {
		t.Fatalf("workflow does not contain YAML section %q at indentation %d", key, indentWidth)
	}
	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if leadingSpaces(line) <= indentWidth {
			end = index
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func childKeys(t *testing.T, section string, indentation int) []string {
	t.Helper()
	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^ {%d}([A-Za-z0-9_-]+):(?:\s.*)?$`, indentation))
	matches := pattern.FindAllStringSubmatch(section, -1)
	keys := make([]string, 0, len(matches))
	for _, match := range matches {
		keys = append(keys, match[1])
	}
	if len(keys) == 0 {
		t.Fatalf("section has no child keys at indentation %d", indentation)
	}
	return keys
}

func yamlLiteralBlock(t *testing.T, section, key string) string {
	t.Helper()
	lines := strings.Split(section, "\n")
	pattern := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `:\s*[|>][-+]?\s*$`)
	start := -1
	keyIndent := 0
	for index, line := range lines {
		if pattern.MatchString(line) {
			start = index + 1
			keyIndent = leadingSpaces(line)
			break
		}
	}
	if start < 0 {
		t.Fatalf("YAML section does not contain literal block %q", key)
	}
	end := len(lines)
	blockIndent := -1
	for index := start; index < len(lines); index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			continue
		}
		indentation := leadingSpaces(line)
		if indentation <= keyIndent {
			end = index
			break
		}
		if blockIndent < 0 || indentation < blockIndent {
			blockIndent = indentation
		}
	}
	if blockIndent < 0 {
		t.Fatalf("YAML literal block %q is empty", key)
	}
	block := make([]string, 0, end-start)
	for _, line := range lines[start:end] {
		if len(line) >= blockIndent {
			line = line[blockIndent:]
		}
		block = append(block, line)
	}
	return strings.Join(block, "\n")
}

func leadingSpaces(value string) int {
	return len(value) - len(strings.TrimLeft(value, " "))
}

func assertFullSHAPins(t *testing.T, workflow string) {
	t.Helper()
	usesPattern := regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s+([^\s@]+)@([^\s#]+)(?:\s+#\s+(.+))?$`)
	matches := usesPattern.FindAllStringSubmatch(workflow, -1)
	if len(matches) < minimumSHAPins {
		t.Fatalf("uses references = %d, want at least %d pinned references", len(matches), minimumSHAPins)
	}
	shaPattern := regexp.MustCompile(`^[0-9a-f]{40}$`)
	versionPattern := regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	for _, match := range matches {
		if !shaPattern.MatchString(match[2]) {
			t.Errorf("uses reference %s@%s is not pinned to a full commit SHA", match[1], match[2])
		}
		if !versionPattern.MatchString(strings.TrimSpace(match[3])) {
			t.Errorf("uses reference %s@%s has missing or inaccurate-looking version comment %q", match[1], match[2], match[3])
		}
	}
}

func assertPermissions(t *testing.T, job string, want map[string]string) {
	t.Helper()
	section := yamlSection(t, job, "permissions", 4)
	keys := childKeys(t, section, 6)
	if len(keys) != len(want) {
		t.Fatalf("permission keys = %v, want exactly %v", keys, want)
	}
	for _, key := range keys {
		value, ok := want[key]
		if !ok {
			t.Errorf("unexpected permission %q", key)
			continue
		}
		assertMatches(t, section, `(?m)^\s+`+regexp.QuoteMeta(key)+`:\s+`+regexp.QuoteMeta(value)+`\s*$`, key+" permission")
	}
}

func assertIdempotentReport(t *testing.T, commentJob string) {
	t.Helper()
	markerPattern := regexp.MustCompile(`<!--\s*dependency-review-report\s*-->`)
	if !markerPattern.MatchString(commentJob) {
		t.Fatal("review report must contain the stable <!-- dependency-review-report --> replacement marker")
	}
	commentIDPattern := regexp.MustCompile(`(?m)^\s+comment-id:\s+\$\{\{\s*steps\.([A-Za-z0-9_-]+)\.outputs\.comment-id\s*}}\s*$`)
	match := commentIDPattern.FindStringSubmatch(commentJob)
	if match == nil {
		t.Fatal("comment action must consume a discovered comment-id output for replacement")
	}
	assertMatches(t, commentJob, `(?m)^\s+id:\s+`+regexp.QuoteMeta(match[1])+`\s*$`, "comment lookup step")
	assertContains(t, commentJob, "listComments", "existing report lookup")
	assertContains(t, commentJob, "setOutput", "comment-id output publication")
}

func assertNoMutationCapabilities(t *testing.T, workflow string) {
	t.Helper()
	prohibited := []string{
		`(?i)uses:[^\n]*actions/checkout`,
		`(?i)pulls\.merge`,
		`(?i)enablePullRequestAutoMerge`,
		`(?i)disablePullRequestAutoMerge`,
		`(?i)gh\s+pr\s+merge`,
		`(?i)merge-method\s*:`,
		`(?i)uses:[^\n]*(auto-merge|automerge)`,
		`(?i)dismissReview|dismiss_review`,
		`(?i)(repos|branches)\.(update|delete).*(protection|required|statusCheck)`,
	}
	for _, expression := range prohibited {
		if regexp.MustCompile(expression).MatchString(workflow) {
			t.Errorf("workflow contains prohibited merge or repository-governance mutation matching %q", expression)
		}
	}
}

func assertContains(t *testing.T, content, expected, description string) {
	t.Helper()
	if !strings.Contains(content, expected) {
		t.Errorf("workflow is missing %s %q", description, expected)
	}
}

func assertMatches(t *testing.T, content, expression, description string) {
	t.Helper()
	if !regexp.MustCompile(expression).MatchString(content) {
		t.Errorf("workflow is missing %s matching %q", description, expression)
	}
}

func indent(value, prefix string) string {
	return prefix + strings.ReplaceAll(value, "\n", "\n"+prefix)
}
