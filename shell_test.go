package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResolveShellPathFallsBackToSh(t *testing.T) {
	path := resolveShellPath("/path/that/does/not/exist")
	if path != "/bin/sh" {
		t.Fatalf("resolveShellPath returned %q, want %q", path, "/bin/sh")
	}
}

func TestResolveShellPathUsesLookPath(t *testing.T) {
	tmpDir := t.TempDir()
	shellPath := filepath.Join(tmpDir, "custom-shell")
	if err := os.WriteFile(shellPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", tmpDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatalf("Setenv returned error: %v", err)
	}
	defer func() {
		_ = os.Setenv("PATH", oldPath)
	}()

	resolved := resolveShellPath("custom-shell")
	if resolved != shellPath {
		t.Fatalf("resolveShellPath returned %q, want %q", resolved, shellPath)
	}
}

func TestUpdateListEnterQueuesShellOpen(t *testing.T) {
	m := newModel()
	m.busy = false
	m.state.Worktrees = []worktreeEntry{{Path: "/tmp/example", Name: "example", Branch: "main"}}

	updated, cmd := m.updateList(tea.KeyMsg{Type: tea.KeyEnter})
	nextModel := updated.(model)

	if nextModel.exitAction != exitActionOpenShell {
		t.Fatalf("exitAction = %v, want %v", nextModel.exitAction, exitActionOpenShell)
	}
	if nextModel.exitPath != "/tmp/example" {
		t.Fatalf("exitPath = %q, want %q", nextModel.exitPath, "/tmp/example")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
}
