package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunInit_FreshDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := runInit(dir, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	// Verify .uf/replicator/ directory created.
	replicatorDir := filepath.Join(dir, ".uf", "replicator")
	info, err := os.Stat(replicatorDir)
	if err != nil {
		t.Fatalf(".uf/replicator/ not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal(".uf/replicator/ is not a directory")
	}

	// Verify cells.json created with empty array.
	cellsPath := filepath.Join(replicatorDir, "cells.json")
	data, err := os.ReadFile(cellsPath)
	if err != nil {
		t.Fatalf("cells.json not created: %v", err)
	}
	if string(data) != "[]\n" {
		t.Errorf("cells.json content = %q, want %q", string(data), "[]\n")
	}

	// Verify agent kit files created (16 total: 1 cells.json + 15 agent kit).
	agentKitFiles := []string{
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
	for _, rel := range agentKitFiles {
		full := filepath.Join(dir, rel)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected agent kit file %s to exist: %v", rel, err)
		}
	}
}

func TestRunInit_AgentKitSkipsExisting(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a file that init would scaffold.
	forgePath := filepath.Join(dir, ".opencode", "commands", "forge.md")
	os.MkdirAll(filepath.Dir(forgePath), 0o755)
	original := []byte("# my custom forge\n")
	os.WriteFile(forgePath, original, 0o644)

	if err := runInit(dir, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	// Verify the pre-existing file was NOT overwritten.
	data, _ := os.ReadFile(forgePath)
	if string(data) != string(original) {
		t.Errorf("forge.md was overwritten: got %q, want %q", string(data), string(original))
	}

	// Verify other agent kit files were still created.
	orgPath := filepath.Join(dir, ".opencode", "commands", "org.md")
	if _, err := os.Stat(orgPath); err != nil {
		t.Errorf("org.md should have been created: %v", err)
	}
}

func TestRunInit_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a file that init would scaffold.
	forgePath := filepath.Join(dir, ".opencode", "commands", "forge.md")
	os.MkdirAll(filepath.Dir(forgePath), 0o755)
	original := []byte("# my custom forge\n")
	os.WriteFile(forgePath, original, 0o644)

	if err := runInit(dir, true); err != nil {
		t.Fatalf("runInit with force: %v", err)
	}

	// Verify the pre-existing file WAS overwritten.
	data, _ := os.ReadFile(forgePath)
	if string(data) == string(original) {
		t.Error("forge.md was NOT overwritten despite force=true")
	}
}

func TestRunInit_AlreadyInitialized(t *testing.T) {
	dir := t.TempDir()

	// First init.
	if err := runInit(dir, false); err != nil {
		t.Fatalf("first runInit: %v", err)
	}

	// Write something to cells.json to verify it's not overwritten.
	cellsPath := filepath.Join(dir, ".uf", "replicator", "cells.json")
	os.WriteFile(cellsPath, []byte(`[{"id":"test"}]`), 0o644)

	// Second init — should be idempotent.
	if err := runInit(dir, false); err != nil {
		t.Fatalf("second runInit: %v", err)
	}

	// Verify cells.json was NOT overwritten.
	data, _ := os.ReadFile(cellsPath)
	if string(data) != `[{"id":"test"}]` {
		t.Errorf("cells.json was overwritten: got %q", string(data))
	}

	// Verify agent kit files still exist (scaffolded on first run, skipped on second).
	forgePath := filepath.Join(dir, ".opencode", "commands", "forge.md")
	if _, err := os.Stat(forgePath); err != nil {
		t.Errorf("agent kit files should exist after second init: %v", err)
	}
}

func TestRunInit_CustomPath(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "myproject")
	os.MkdirAll(target, 0o755)

	if err := runInit(target, false); err != nil {
		t.Fatalf("runInit with custom path: %v", err)
	}

	cellsPath := filepath.Join(target, ".uf", "replicator", "cells.json")
	if _, err := os.Stat(cellsPath); err != nil {
		t.Fatalf("cells.json not created at custom path: %v", err)
	}

	forgePath := filepath.Join(target, ".opencode", "commands", "forge.md")
	if _, err := os.Stat(forgePath); err != nil {
		t.Fatalf("agent kit not created at custom path: %v", err)
	}
}

func TestRunInit_InvalidPath(t *testing.T) {
	err := runInit("/nonexistent/path/that/cannot/exist", false)
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}
