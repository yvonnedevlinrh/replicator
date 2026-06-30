package agentkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffold_FreshDirectory(t *testing.T) {
	dir := t.TempDir()
	results, err := Scaffold(dir, false)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	// Expect 15 files: 5 commands + 7 skills + 3 agents.
	if len(results) != 15 {
		t.Errorf("Scaffold returned %d results, want 15", len(results))
		for _, r := range results {
			t.Logf("  %s: %s", r.Action, r.Path)
		}
	}

	// All should be "created".
	for _, r := range results {
		if r.Action != "created" {
			t.Errorf("result %s: action = %q, want %q", r.Path, r.Action, "created")
		}
	}

	// Spot-check a few files exist on disk.
	checks := []string{
		".opencode/commands/forge.md",
		".opencode/commands/org.md",
		".opencode/commands/inbox.md",
		".opencode/commands/forge-status.md",
		".opencode/commands/handoff.md",
		".opencode/skills/always-on-guidance/SKILL.md",
		".opencode/skills/forge-coordination/SKILL.md",
		".opencode/skills/replicator-cli/SKILL.md",
		".opencode/skills/testing-patterns/SKILL.md",
		".opencode/skills/system-design/SKILL.md",
		".opencode/skills/learning-systems/SKILL.md",
		".opencode/skills/forge-global/SKILL.md",
		".opencode/agents/coordinator.md",
		".opencode/agents/worker.md",
		".opencode/agents/background-worker.md",
	}
	for _, rel := range checks {
		full := filepath.Join(dir, rel)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected file %s to exist: %v", rel, err)
		}
	}
}

func TestScaffold_SkipsExisting(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a file that Scaffold would write.
	forgePath := filepath.Join(dir, ".opencode", "commands", "forge.md")
	os.MkdirAll(filepath.Dir(forgePath), 0o755)
	original := []byte("# custom content\n")
	os.WriteFile(forgePath, original, 0o644)

	results, err := Scaffold(dir, false)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	// Find the forge.md result — should be "skipped".
	var found bool
	for _, r := range results {
		if r.Path == filepath.Join("commands", "forge.md") {
			found = true
			if r.Action != "skipped" {
				t.Errorf("forge.md action = %q, want %q", r.Action, "skipped")
			}
		}
	}
	if !found {
		t.Error("forge.md not found in results")
	}

	// Verify file content was NOT overwritten.
	data, _ := os.ReadFile(forgePath)
	if string(data) != string(original) {
		t.Errorf("forge.md was overwritten: got %q", string(data))
	}
}

func TestScaffold_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a file that Scaffold would write.
	forgePath := filepath.Join(dir, ".opencode", "commands", "forge.md")
	os.MkdirAll(filepath.Dir(forgePath), 0o755)
	original := []byte("# custom content\n")
	os.WriteFile(forgePath, original, 0o644)

	results, err := Scaffold(dir, true)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	// Find the forge.md result — should be "overwritten".
	for _, r := range results {
		if r.Path == filepath.Join("commands", "forge.md") {
			if r.Action != "overwritten" {
				t.Errorf("forge.md action = %q, want %q", r.Action, "overwritten")
			}
		}
	}

	// Verify file content WAS overwritten with embedded content.
	data, _ := os.ReadFile(forgePath)
	if string(data) == string(original) {
		t.Error("forge.md was NOT overwritten despite force=true")
	}
}

func TestScaffold_FileCount(t *testing.T) {
	dir := t.TempDir()
	results, err := Scaffold(dir, false)
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	// Count by category.
	var commands, skills, agents int
	for _, r := range results {
		switch {
		case strings.HasPrefix(r.Path, "commands/"):
			commands++
		case strings.HasPrefix(r.Path, "skills/"):
			skills++
		case strings.HasPrefix(r.Path, "agents/"):
			agents++
		}
	}

	if commands != 5 {
		t.Errorf("commands = %d, want 5", commands)
	}
	if skills != 7 {
		t.Errorf("skills = %d, want 7", skills)
	}
	if agents != 3 {
		t.Errorf("agents = %d, want 3", agents)
	}
}
