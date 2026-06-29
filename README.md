# git-wtm

git-wtm is a Go TUI application for listing, creating, and removing Git worktrees, and for opening a working shell directly in the selected worktree.

It operates on the Git repository in the current shell's working directory.

[日本語版 README](README-ja.md)

## Current Features

- List worktrees associated with a Git repository
- Move the selection with `↑` / `↓` or `j` / `k`
- Create a new worktree with `c`
- Remove the selected worktree with `d`
- Reload the list with `r`
- Start a shell in the selected worktree with `Enter` or `o`
- Complete existing local branches and fetched remote branch candidates when entering a base ref
- Automatically switch the create mode based on the branch name and show the command that will be run

The list identifies both the current worktree and the primary worktree.

## Requirements

- Git must be available
- An interactive shell must be available
- Go 1.24 or later is required when installing with `go install` or running from source

When starting a shell, git-wtm prefers the `SHELL` environment variable and falls back to `/bin/sh` if it cannot be resolved.

## Installation

### Install with go install

```sh
go install github.com/zaltoprofen/git-wtm
```

### Run directly from source

Run the following from inside a Git repository:

```sh
go run .
```

### Build and use a binary

```sh
go build
./git-wtm
```

Example for placing the binary somewhere on your `PATH`:

```sh
go build -o "$HOME/bin/git-wtm"
```

Then run `git-wtm` from inside any Git repository.

## Usage

1. Move into the Git repository whose worktrees you want to manage.
2. Start `git-wtm`.
3. Select a worktree from the list.
4. Create, remove, or open a shell as needed.

## Key Bindings

- `↑` / `↓` or `j` / `k`: Move the selection
- `Enter` or `o`: Open a shell in the selected worktree
- `c`: Open the new worktree creation screen
- `d`: Open the removal confirmation for the selected worktree
- `r`: Reload the list
- `q`: Quit

On the creation screen, use the following keys:

- `Tab`: Apply the selected base ref candidate to the input field, or move to the next input field
- `Shift+Tab`: Move to the previous input field
- `↑` / `↓`: Select a base ref candidate, or switch input fields
- `Enter`: Create the worktree
- `Esc`: Cancel creation

On the removal confirmation screen, use the following keys:

- `y` or `Enter`: Confirm removal
- `n` or `Esc`: Cancel removal

## Worktree Creation Behavior

The creation screen accepts the following two fields:

- Branch name: Required
- Base ref: Optional

When you enter a branch name, git-wtm automatically determines the create mode based on whether a local branch with the same name exists.

- If no local branch with the same name exists: new branch creation mode
- If a local branch with the same name exists: existing local branch mode

The creation screen shows the current mode and the command that will be run.

While the base ref input field is focused, existing local branches and fetched remote-tracking branches are shown as candidates. Candidates are filtered by the entered text and can be applied with `Tab`.

The default destination for a new worktree is `$PARENT_DIR/$REPO_NAME.worktrees/<branch>`, where `$REPO_NAME` is the repository name and `$PARENT_DIR` is the repository's parent directory.

If the branch does not exist yet, git-wtm creates it in one of the following forms:

- Without a base ref: `git worktree add -b <branch> <path>`
- With a base ref: `git worktree add -b <branch> <path> <base-ref>`

If a local branch with the same name already exists, the creation screen switches to existing local branch mode, ignores the base ref, and creates the worktree in the following form:

- `git worktree add <path> <branch>`

## Worktree Removal Behavior

Removal uses `git worktree remove <path>`.

The UI prevents removal of the following worktrees:

- The current worktree
- The primary worktree

## Shell Launch Behavior

When you press `Enter` or `o` on the list screen, git-wtm moves to the selected worktree directory and replaces the current process with an interactive shell.

This does not return to the TUI. When you exit the shell, you return to the original parent shell.

However, the parent shell's current directory is not changed. If you want to move the parent shell itself into that worktree, you need a separate integration such as a shell function or wrapper script.

## Limitations

- git-wtm does not work outside a Git repository
- git-wtm cannot change the parent shell's current directory
- Shell launch replaces the application process; it does not move a new parent shell
