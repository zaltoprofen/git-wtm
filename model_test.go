package main

import "testing"

func TestFilterBaseRefSuggestionsMatchesCaseInsensitiveSubstring(t *testing.T) {
	refs := []string{"main", "origin/feature/foo", "origin/main"}
	filtered := filterBaseRefSuggestions(refs, "FEATURE")

	if len(filtered) != 1 {
		t.Fatalf("len(filtered) = %d, want 1", len(filtered))
	}
	if filtered[0] != "origin/feature/foo" {
		t.Fatalf("filtered[0] = %q, want %q", filtered[0], "origin/feature/foo")
	}
}

func TestAcceptSelectedBaseRefSuggestionSetsInputValue(t *testing.T) {
	m := newModel()
	m.create.allBaseRefs = []string{"main", "origin/feature/foo", "origin/main"}
	m.resetCreateForm()
	m.moveCreateFocus("down")
	m.create.inputs[1].SetValue("origin/f")
	m.refreshBaseRefSuggestions()

	if len(m.create.filteredBaseRefs) == 0 {
		t.Fatalf("expected suggestions")
	}

	m.acceptSelectedBaseRefSuggestion()

	if m.create.inputs[1].Value() != "origin/feature/foo" {
		t.Fatalf("base ref value = %q, want %q", m.create.inputs[1].Value(), "origin/feature/foo")
	}
}

func TestCurrentCreateModeUsesExistingLocalBranch(t *testing.T) {
	m := newModel()
	m.create.localBranches = map[string]struct{}{"feature/foo": {}}
	m.resetCreateForm()
	m.create.localBranches = map[string]struct{}{"feature/foo": {}}
	m.create.inputs[0].SetValue("feature/foo")

	if mode := m.currentCreateMode(); mode != createModeExistingLocalBranch {
		t.Fatalf("currentCreateMode = %v, want %v", mode, createModeExistingLocalBranch)
	}
	if effectiveBaseRef := m.effectiveBaseRef(); effectiveBaseRef != "" {
		t.Fatalf("effectiveBaseRef = %q, want empty", effectiveBaseRef)
	}
}

func TestCreateCommandPreviewUsesExistingLocalBranchMode(t *testing.T) {
	m := newModel()
	m.state.Repo.WorktreesDir = "/tmp/example.worktrees"
	m.create.localBranches = map[string]struct{}{"feature/foo": {}}
	m.create.inputs[0].SetValue("feature/foo")
	m.create.inputs[1].SetValue("origin/feature/foo")

	want := "git worktree add /tmp/example.worktrees/feature/foo feature/foo"
	if preview := m.createCommandPreview(); preview != want {
		t.Fatalf("createCommandPreview = %q, want %q", preview, want)
	}
}
