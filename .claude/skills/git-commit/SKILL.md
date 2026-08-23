---
name: git-commit
description: >
  Creates a well-formed git commit for the current changes in the repository,
  following Conventional Commits format. Use this skill whenever the user asks
  to commit changes, stage and commit, write a commit message, "git commit this",
  "commit my changes", "make a commit", or wants help describing what changed.
  Handles staging and message generation. NEVER push unless the user explicitly
  says "push" or "commit and push" — do not prompt or suggest pushing.
allowed-tools: "Bash(git:*) Bash(grep:*) Bash(echo:*) Bash(cat:*) Bash(head:*)"
metadata:
  author: maestro-runner
  version: 1.4.0
  category: git
  tags: [git, commit, conventional-commits, staging]
---

# Git Commit

Creates a well-formed git commit for the current working-tree changes, following
the Conventional Commits convention used in this repository.

## Commit Message Convention

This repo uses **Conventional Commits**:

```
<type>(<scope>): <short summary>

<optional body — HOW and WHY, wrapped at 72 chars>

<optional trailers>
```

**Types:**

| Type | When to use |
|------|-------------|
| `feat` | New feature or functionality |
| `fix` | Bug fix or issue resolution |
| `chore` | Maintenance, dependencies, tooling |
| `test` | Test additions or modifications |
| `docs` | Documentation updates |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `perf` | Performance improvement |
| `ci` | CI/CD configuration changes |
| `style` | Code formatting, linting (non-functional) |
| `security` | Security vulnerability fixes or hardening |

**Scope** (kebab-case, in parens): the subsystem affected, e.g. `ios`, `android`,
`typescript`, `python`, `server`, `cli`, `config`, `skills`.

**Subject line rules:**
- Lowercase, imperative mood — "add", "fix", "implement", not "adds" or "added"
- No period at the end
- Max 50 characters after the colon
- Specific and descriptive — state WHAT, not just "update code" or "fix bug"

## Workflow

### Step 0: Check for project-specific conventions

Before anything else, check if the project overrides these defaults:

```sh
cat .claude/CLAUDE.md 2>/dev/null | grep -A 20 -i "commit"
```

If a project format is specified, **use that format** instead of the defaults above.

### Step 1: Inspect the current state

```sh
git status
git diff --stat HEAD
```

If there are already staged changes (green in `git status`), note them. If
everything is unstaged (red), you'll stage as part of this workflow.

### Step 2: Understand what changed

```sh
# Summary of changed files
git diff --stat

# Per-file diff (for unstaged changes)
git diff

# Per-file diff (for already-staged changes)
git diff --cached
```

Read enough of the diff to understand the intent of the changes — not just file
names. The commit message should describe *why* or *what*, not just *which files*.

### Step 3: Stage the right files

If the user specifies what to include, stage only those. Otherwise, stage all
tracked changes:

```sh
# Stage specific files (preferred — be intentional)
git add <file1> <file2> ...

# Stage all tracked changes (does not add new untracked files)
git add -u

# Stage everything including new untracked files
git add .
```

After staging, verify:
```sh
git diff --cached --stat
```

**NEVER stage:**
- `.env`, `credentials.json`, API keys, tokens, secrets of any kind
- `node_modules/`, `__pycache__/`, `.venv/` — these should be in `.gitignore`
- Large binary files without explicit user approval

### Step 4: Generate the commit message

Based on what's staged:

1. Pick the `type` from the table above
2. Pick a `scope` for the area affected
3. Write a concise imperative summary (≤50 chars after colon)

Show the proposed message to the user and ask for quick confirmation before
committing, unless the user already said "just commit" or "commit and push".

**NEVER suggest or offer to push after committing. Only push if the user
explicitly requested it (e.g. said "push", "commit and push", "push it").**

**Good examples:**
```
feat(ios): add UDID env var support to e2e test setup
fix(auth): use hmac.compare_digest for secure key comparison
test(typescript): separate unit and device test suites
chore(skills): improve all skills with iOS coverage and eval test cases
refactor(template): consolidate filename sanitization logic
security(api): block dangerous URL schemes in validator
```

**Bad examples (avoid):**
```
update validation code    # no type, no scope, vague
feat: add stuff           # missing scope, too vague
fix(auth): fix bug        # circular, not specific
chore: make changes.      # missing scope, has period
```

### Step 5: Commit

> **IMPORTANT — NEVER use heredoc (`<<EOF`) for commit messages.** Heredocs are
> unreliable in terminal-based agents and cause quoting/apostrophe failures.
> **Always write the message to `/tmp/commit-msg.txt` first, then commit with
> `git commit -F /tmp/commit-msg.txt`.**

Simple change (one line — `-m` is fine only when there is no body):
```sh
git commit -m "type(scope): summary"
```

Complex change (with body explaining HOW and WHY) — **write to a temp file**:
```sh
printf 'type(scope): summary\n\nExplain the motivation and approach taken.\n- Use bullet points for multiple items\n- Wrap at 72 characters per line\n\nFixes #123\n' > /tmp/commit-msg.txt
git commit -F /tmp/commit-msg.txt
```

Or equivalently using `echo -e` for readability on multi-line messages:
```sh
echo "type(scope): summary

Explain the motivation and approach taken.
- Use bullet points for multiple items
- Wrap at 72 characters per line

Fixes #123" > /tmp/commit-msg.txt
git commit -F /tmp/commit-msg.txt
```

#### Breaking changes

For incompatible API/behavior changes, use `!` after the scope or a
`BREAKING CHANGE:` footer — always via temp file:

```sh
printf 'feat(api)!: change session response format to JSON:API\n\nBREAKING CHANGE: Response envelope changed from { data } to\n{ data: { type, id, attributes } }.\n' > /tmp/commit-msg.txt
git commit -F /tmp/commit-msg.txt
```

### Step 5a: Verify commit success (CRITICAL)

**BEFORE attempting any alternative commit method or retry:**

```sh
git log -1 --oneline
```

**Check the output:**
- If you see a recent commit that matches the message you just created, **STOP**. The commit succeeded.
- If the exit code was 0 and the commit hash appears in logs, **DO NOT attempt to commit again**.
- Only retry with a different method if `git status` shows changes are still staged AND the commit truly failed (exit code non-zero or error message present).

**This prevents duplicate commits.** If a previous attempt succeeded with exit code 0, trust that result.

#### Git trailers (optional)

Add at the end of the body after a blank line:

| Trailer | Purpose |
|---------|---------|
| `Fixes #N` | Links and closes issue on merge |
| `Closes #N` | Same as Fixes |
| `Co-authored-by: Name <email>` | Credit co-contributors |

### Step 6: Verify the commit

```sh
git log -1 --format="%h %s"
git show --stat HEAD
```

### Step 6.5: Check for explicit push request (CRITICAL)

**Before even considering a push, verify the user's intent:**

```
Check the user's original message for these exact keywords:
- "push"
- "commit and push"
- "push it"
- Any grammatically similar explicit push request
```

**Decision logic:**
```
IF user message contains "push" keyword:
  → Proceed to Step 7 (push)
ELSE:
  → STOP. Report commit success and stop at Step 6.
  → Wait for explicit user request before pushing.
```

**Why this matters:** Accidentally pushing commits violates the skill's core
principle. This check prevents defaulting to push after successful commit.
It forces an explicit wait-for-user-intent at every fork point.

**Example responses (no push requested):**
```
✓ "Commit 071b569 created. Ready to push when you ask."
✓ "Committed successfully. Branch is clean."
❌ "Pushing now..." (without explicit user request)
```

### Step 7: Push (ONLY if explicitly requested)

> **NEVER push automatically. NEVER ask "should I push?". NEVER offer to push.**
> Only run the push commands below when the user's message explicitly includes
> "push", "commit and push", or "push it".

```sh
git push
```

If the branch has no upstream yet:

```sh
git push -u origin $(git rev-parse --abbrev-ref HEAD)
```

### Step 7.5: Verify push success (CRITICAL)

**After running push, verify it succeeded:**

```sh
git log -1 --format="%h %s (pushed to remote)"
git status    # should show "Your branch is up to date with 'origin/...'"
```

**If git status shows "ahead of ... commit" after push:**
```
→ STOP. Push may have failed. Check error messages above.
→ Retry or investigate before proceeding.
```

**If status shows synced/up-to-date:**
```
→ Push succeeded. Report success.
→ Do not attempt alternative push methods.
```

### Mixed changes (only commit some of them)

If `git status` shows changes across multiple unrelated concerns, ask the user
which files belong to this commit. Don't lump unrelated changes into one commit.

```sh
git add <specific-files>
git diff --cached --stat   # confirm what's going in
```

### Nothing to commit

```sh
git status   # shows "nothing to commit, working tree clean"
```

Report this clearly: "There's nothing to commit — the working tree is already clean."

### Untracked files

`git add -u` will not pick up brand new files. If `git status` shows untracked
files that belong to the commit, include them explicitly or use `git add .`.
Always tell the user which new files are being included.

### Already-staged changes

If the user already ran `git add`, respect what's staged — don't unstage or
re-stage unless asked. Skip straight to Step 4.
</content>
</invoke>