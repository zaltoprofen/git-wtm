package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type repoInfo struct {
	RepoDir      string
	RepoName     string
	ParentDir    string
	WorktreesDir string
	CurrentRoot  string
	CommonGitDir string
}

type worktreeEntry struct {
	Name      string
	Path      string
	Branch    string
	Head      string
	Detached  bool
	IsCurrent bool
	IsMain    bool
}

type appState struct {
	Repo      repoInfo
	Worktrees []worktreeEntry
}

func loadState() (appState, error) {
	repo, err := loadRepoInfo()
	if err != nil {
		return appState{}, err
	}

	raw, err := gitOutput("worktree", "list", "--porcelain")
	if err != nil {
		return appState{}, err
	}

	worktrees, err := parseWorktreeList(raw, repo)
	if err != nil {
		return appState{}, err
	}

	return appState{Repo: repo, Worktrees: worktrees}, nil
}

func loadRepoInfo() (repoInfo, error) {
	currentRoot, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return repoInfo{}, err
	}

	commonGitDir, err := gitOutput("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return repoInfo{}, err
	}

	return buildRepoInfo(currentRoot, commonGitDir), nil
}

func listBaseRefs() ([]string, error) {
	raw, err := gitOutput("for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, err
	}

	if raw == "" {
		return nil, nil
	}

	refs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(raw, "\n") {
		ref := strings.TrimSpace(line)
		if ref == "" || strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}

	sort.Strings(refs)
	return refs, nil
}

func listLocalBranches() ([]string, error) {
	raw, err := gitOutput("for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}

	if raw == "" {
		return nil, nil
	}

	branches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, line := range strings.Split(raw, "\n") {
		branch := strings.TrimSpace(line)
		if branch == "" {
			continue
		}
		if _, ok := seen[branch]; ok {
			continue
		}
		seen[branch] = struct{}{}
		branches = append(branches, branch)
	}

	sort.Strings(branches)
	return branches, nil
}

func buildRepoInfo(currentRoot string, commonGitDir string) repoInfo {
	cleanRoot := filepath.Clean(currentRoot)
	cleanCommonDir := filepath.Clean(commonGitDir)
	repoDir := filepath.Dir(cleanCommonDir)
	parentDir := filepath.Dir(repoDir)
	repoName := filepath.Base(repoDir)

	return repoInfo{
		RepoDir:      repoDir,
		RepoName:     repoName,
		ParentDir:    parentDir,
		WorktreesDir: filepath.Join(parentDir, repoName+".worktrees"),
		CurrentRoot:  cleanRoot,
		CommonGitDir: cleanCommonDir,
	}
}

func parseWorktreeList(raw string, repo repoInfo) ([]worktreeEntry, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	blocks := strings.Split(trimmed, "\n\n")
	worktrees := make([]worktreeEntry, 0, len(blocks))
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		entry := worktreeEntry{}
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				entry.Path = filepath.Clean(strings.TrimPrefix(line, "worktree "))
			case strings.HasPrefix(line, "branch "):
				entry.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
			case strings.HasPrefix(line, "HEAD "):
				entry.Head = strings.TrimPrefix(line, "HEAD ")
			case line == "detached":
				entry.Detached = true
			}
		}

		if entry.Path == "" {
			return nil, errors.New("failed to parse git worktree list output")
		}

		entry.Name = filepath.Base(entry.Path)
		entry.IsCurrent = entry.Path == filepath.Clean(repo.CurrentRoot)
		entry.IsMain = entry.Path == filepath.Clean(repo.RepoDir)
		if entry.Branch == "" {
			if entry.Detached {
				entry.Branch = "(detached)"
			} else {
				entry.Branch = "(unknown)"
			}
		}

		worktrees = append(worktrees, entry)
	}

	sort.Slice(worktrees, func(i, j int) bool {
		if worktrees[i].IsCurrent != worktrees[j].IsCurrent {
			return worktrees[i].IsCurrent
		}
		if worktrees[i].IsMain != worktrees[j].IsMain {
			return worktrees[i].IsMain
		}
		return worktrees[i].Path < worktrees[j].Path
	})

	return worktrees, nil
}

func createWorktree(branchName string, baseRef string, repo repoInfo) (string, error) {
	branchName = strings.TrimSpace(branchName)
	baseRef = strings.TrimSpace(baseRef)
	if branchName == "" {
		return "", errors.New("worktree name is required")
	}

	if err := validateBranchName(branchName); err != nil {
		return "", err
	}

	path := filepath.Join(repo.WorktreesDir, filepath.FromSlash(branchName))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("failed to create worktree parent directory: %w", err)
	}

	branchExists, err := localBranchExists(branchName)
	if err != nil {
		return "", err
	}

	args := []string{"worktree", "add"}
	if branchExists {
		args = append(args, path, branchName)
	} else {
		args = append(args, "-b", branchName, path)
		if baseRef != "" {
			args = append(args, baseRef)
		}
	}

	if _, err := gitOutput(args...); err != nil {
		return "", err
	}

	return path, nil
}

func removeWorktree(entry worktreeEntry) error {
	if entry.IsMain {
		return errors.New("primary worktree cannot be removed")
	}
	if entry.IsCurrent {
		return errors.New("current worktree cannot be removed")
	}

	_, err := gitOutput("worktree", "remove", entry.Path)
	return err
}

func validateBranchName(name string) error {
	cmd := exec.Command("git", "check-ref-format", "--branch", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = fmt.Sprintf("invalid branch name: %s", name)
		}
		return errors.New(message)
	}

	return nil
}

func localBranchExists(name string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return "", errors.New(message)
	}

	return strings.TrimSpace(stdout.String()), nil
}
