---
name: merge-upstream
description: >
  Merges the upstream main branch from https://github.com/devicelab-dev/maestro-runner
  into the current branch. Use this skill when you need to sync your local branch
  with the latest changes from the official maestro-runner repository, pull in
  upstream changes, check if the branch is out of sync with upstream, upstream
  has new commits you need, or to cherry-pick features from the official repo.
  Handles conflict resolution, provides merge status, and ensures a clean merge workflow.
allowed-tools: "Bash(git:*) Bash(grep:*) Bash(echo:*) Bash(make:*) Bash(go:*) Bash(npm:*) Bash(python:*) Bash(pytest:*)"
metadata:
  author: maestro-runner
  version: 1.0.0
  category: git
  tags: [git, merge, upstream, sync]
---

# Merge Upstream

Syncs your local branch with the latest changes from the official maestro-runner
upstream repository at `https://github.com/devicelab-dev/maestro-runner`.

## Prerequisites

- Git installed and configured with credentials
- Local repository initialized with remote named `origin`
- No uncommitted changes in your working directory; stash them first if needed:

```sh
git stash        # save uncommitted changes
# ... run merge ...
git stash pop    # restore uncommitted changes afterwards
```

## Quick Start

### 1. Check Current Status

```sh
# Show current branch and upstream status
git status

# Show remote configuration
git remote -v
```

### 2. Configure Upstream Remote (if not already set up)

```sh
# Add upstream remote if it doesn't exist
git remote add upstream https://github.com/devicelab-dev/maestro-runner

# Or update existing upstream remote
git remote set-url upstream https://github.com/devicelab-dev/maestro-runner

# Verify setup
git remote -v
```

### 3. Fetch Upstream Changes

```sh
# Fetch all upstream branches
git fetch upstream

# Show upstream main branch info
git log --oneline upstream/main -10
```

### 4. Merge Upstream Main into Current Branch

```sh
# Create a merge commit combining upstream/main with current branch
git merge upstream/main --no-ff

# Or use rebase strategy (replays your commits on top of upstream/main)
# This creates a cleaner, linear history but should only be used if not yet pushed
git rebase upstream/main
```

## Handling Merge Conflicts

If conflicts occur during merge:

### 1. Check Conflict Status

```sh
# Show files with conflicts
git status

# Show detailed conflict diff
git diff --name-only --diff-filter=U
```

### 2. Resolve Conflicts

```sh
# Open conflicted file in your editor and look for conflict markers:
# <<<<<<< HEAD
# (your current changes)
# =======
# (upstream changes)
# >>>>>>> upstream/main

# After manual resolution, stage the fixed files
git add <file1> <file2> ...

# Or stage all resolved files
git add .
```

### 3. Complete the Merge

```sh
# Finish the merge with a commit message
git commit -m "Merge upstream/main into $(git rev-parse --abbrev-ref HEAD)"

# Or abort if you want to start over
git merge --abort
```

## Verify Merge

```sh
# Show merge history
git log --oneline --graph -10

# Confirm changes were integrated
git diff origin/<your-branch>..HEAD
```

## Show What Changed From Upstream

After merging, summarize which features/fixes were pulled in.

```sh
# Files changed by the upstream merge (or latest upstream sync)
git diff --name-status HEAD@{1}..HEAD

# Commit-level summary of what arrived from upstream
git log --oneline --no-merges HEAD@{1}..HEAD

# Optional: richer summary grouped by commit and touched files
git log --stat --no-merges HEAD@{1}..HEAD
```

If the merge result is "Already up to date", use this comparison to inspect recent
upstream changes and verify there is nothing new missing on your branch:

```sh
git log --oneline --no-merges HEAD..upstream/main
```

## Run Tests After Merge

Always run tests after a merge to catch integration regressions early. If upstream added or updated dependencies, refresh them before running tests.

```sh
# 1) Go server tests (from repo root)
go mod tidy                # refresh Go deps if go.mod/go.sum changed upstream
make test                  # all Go tests
make test-race             # optional: race detector
make test-coverage-check   # optional: enforce 80% coverage threshold

# 2) TypeScript client unit tests
cd client/typescript
npm install                # refresh npm deps if package.json changed upstream
npm run test:unit

# 3) Python client unit tests
cd client/python
pip install -e ".[dev]"    # refresh Python deps if pyproject.toml changed upstream
./.venv/bin/python -m pytest tests/test_client.py tests/test_models.py -v
```

## Troubleshooting

### Upstream Remote Not Found
```sh
# Ensure upstream is configured
git remote add upstream https://github.com/devicelab-dev/maestro-runner
git fetch upstream
```

### "Everything up-to-date"
```sh
# Your branch is already in sync with upstream/main
# No merge needed
git log --oneline upstream/main -5
```

### Merge Conflict Too Complex
```sh
# Abort the merge and start fresh
git merge --abort

# Try rebase instead (if branch not yet pushed)
git rebase upstream/main
```

### Stash Uncommitted Changes (Optional)

If you have uncommitted work:

```sh
# Stash your changes temporarily
git stash

# Perform merge as above
git merge upstream/main --no-ff

# Restore your changes after merge
git stash pop
```

## Upstream Repository

**Official Repository:** `https://github.com/devicelab-dev/maestro-runner`

**Main Branch:** `main` (stable, production-ready)

**Clone URL:** `git clone https://github.com/devicelab-dev/maestro-runner`

## References

- [Git Remote Documentation](https://git-scm.com/book/en/v2/Git-Basics-Working-with-Remotes)
- [Git Merge Documentation](https://git-scm.com/docs/git-merge)
- [GitHub Syncing Fork](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/working-with-forks/syncing-a-fork)
