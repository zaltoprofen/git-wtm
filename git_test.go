package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildRepoInfoUsesParentWorktreesDirectory(t *testing.T) {
	info := buildRepoInfo("/tmp/example/.worktrees/feature-a", "/tmp/example/.git")

	if info.RepoDir != "/tmp/example" {
		t.Fatalf("RepoDir = %q, want %q", info.RepoDir, "/tmp/example")
	}
	if info.RepoName != "example" {
		t.Fatalf("RepoName = %q, want %q", info.RepoName, "example")
	}
	if info.ParentDir != "/tmp" {
		t.Fatalf("ParentDir = %q, want %q", info.ParentDir, "/tmp")
	}
	if info.WorktreesDir != "/tmp/example.worktrees" {
		t.Fatalf("WorktreesDir = %q, want %q", info.WorktreesDir, "/tmp/example.worktrees")
	}
}

func TestParseWorktreeListMarksCurrentAndPrimary(t *testing.T) {
	repo := repoInfo{
		RepoDir:     "/tmp/example",
		CurrentRoot: filepath.Clean("/tmp/example.worktrees/feature-a"),
	}

	raw := "worktree /tmp/example\nHEAD aaaaaaa\nbranch refs/heads/main\n\nworktree /tmp/example.worktrees/feature-a\nHEAD bbbbbbb\nbranch refs/heads/feature-a\n\nworktree /tmp/example.worktrees/review\nHEAD ccccccc\ndetached"

	worktrees, err := parseWorktreeList(raw, repo)
	if err != nil {
		t.Fatalf("parseWorktreeList returned error: %v", err)
	}
	if len(worktrees) != 3 {
		t.Fatalf("len(worktrees) = %d, want 3", len(worktrees))
	}

	if !worktrees[0].IsCurrent {
		t.Fatalf("first worktree should be current")
	}
	if worktrees[0].Branch != "feature-a" {
		t.Fatalf("current worktree branch = %q, want %q", worktrees[0].Branch, "feature-a")
	}
	if !worktrees[1].IsMain {
		t.Fatalf("second worktree should be primary")
	}
	if worktrees[2].Branch != "(detached)" {
		t.Fatalf("detached worktree branch = %q, want %q", worktrees[2].Branch, "(detached)")
	}
}

func TestCreateWorktreeUsesBaseRefForNewBranch(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, "git-bin")
	logPath := filepath.Join(tmpDir, "git.log")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	script := "#!/bin/sh\nprintf '%s\n' \"$*\" >> \"$TEST_GIT_LOG\"\ncase \"$1\" in\n  check-ref-format)\n    exit 0\n    ;;\n  show-ref)\n    exit 1\n    ;;\n  worktree)\n    exit 0\n    ;;\n  *)\n    exit 0\n    ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(gitDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	oldPath := os.Getenv("PATH")
	oldLog := os.Getenv("TEST_GIT_LOG")
	if err := os.Setenv("PATH", gitDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatalf("Setenv PATH returned error: %v", err)
	}
	if err := os.Setenv("TEST_GIT_LOG", logPath); err != nil {
		t.Fatalf("Setenv TEST_GIT_LOG returned error: %v", err)
	}
	defer func() {
		_ = os.Setenv("PATH", oldPath)
		_ = os.Setenv("TEST_GIT_LOG", oldLog)
	}()

	repo := repoInfo{WorktreesDir: filepath.Join(tmpDir, "example.worktrees")}
	path, err := createWorktree("feature/demo", "origin/main", repo)
	if err != nil {
		t.Fatalf("createWorktree returned error: %v", err)
	}

	wantPath := filepath.Join(repo.WorktreesDir, "feature", "demo")
	if path != wantPath {
		t.Fatalf("path = %q, want %q", path, wantPath)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	want := "check-ref-format --branch feature/demo\nshow-ref --verify --quiet refs/heads/feature/demo\nworktree add -b feature/demo " + wantPath + " origin/main\n"
	if string(data) != want {
		t.Fatalf("git log = %q, want %q", string(data), want)
	}
}

func TestListBaseRefsReturnsLocalAndRemoteBranches(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, "git-bin")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	script := "#!/bin/sh\nif [ \"$1\" = \"for-each-ref\" ]; then\n  printf 'main\nfeature/demo\norigin/HEAD\norigin/main\norigin/feature/demo\n'\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(gitDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", gitDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatalf("Setenv PATH returned error: %v", err)
	}
	defer func() {
		_ = os.Setenv("PATH", oldPath)
	}()

	refs, err := listBaseRefs()
	if err != nil {
		t.Fatalf("listBaseRefs returned error: %v", err)
	}

	want := []string{"feature/demo", "main", "origin/feature/demo", "origin/main"}
	if len(refs) != len(want) {
		t.Fatalf("len(refs) = %d, want %d", len(refs), len(want))
	}
	for index := range want {
		if refs[index] != want[index] {
			t.Fatalf("refs[%d] = %q, want %q", index, refs[index], want[index])
		}
	}
}

func TestListLocalBranchesReturnsOnlyLocalBranches(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, "git-bin")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}

	script := "#!/bin/sh\nif [ \"$1\" = \"for-each-ref\" ]; then\n  printf 'main\nfeature/demo\nmain\n'\n  exit 0\nfi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(gitDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", gitDir+string(os.PathListSeparator)+oldPath); err != nil {
		t.Fatalf("Setenv PATH returned error: %v", err)
	}
	defer func() {
		_ = os.Setenv("PATH", oldPath)
	}()

	branches, err := listLocalBranches()
	if err != nil {
		t.Fatalf("listLocalBranches returned error: %v", err)
	}

	want := []string{"feature/demo", "main"}
	if len(branches) != len(want) {
		t.Fatalf("len(branches) = %d, want %d", len(branches), len(want))
	}
	for index := range want {
		if branches[index] != want[index] {
			t.Fatalf("branches[%d] = %q, want %q", index, branches[index], want[index])
		}
	}
}
