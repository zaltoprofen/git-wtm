package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func openShellInWorktree(path string) error {
	if path == "" {
		return fmt.Errorf("worktree path is required")
	}

	if err := os.Chdir(path); err != nil {
		return fmt.Errorf("failed to change directory to %s: %w", path, err)
	}

	shellPath := resolveShellPath(os.Getenv("SHELL"))
	args := []string{filepath.Base(shellPath), "-i"}
	return syscall.Exec(shellPath, args, os.Environ())
}

func resolveShellPath(shellEnv string) string {
	if shellEnv != "" {
		if resolved, err := exec.LookPath(shellEnv); err == nil {
			return resolved
		}
	}

	return "/bin/sh"
}
