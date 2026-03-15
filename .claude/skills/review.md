---
name: review
description: Review branch diff against main for bugs, correctness issues, and testing gaps, then fix found issues
argument-hint:
disable-model-invocation: false
allowed-tools: Read, Write, Edit, Glob, Grep, Bash, Task, TodoWrite
---

# Review Branch Diff

Review the current branch's diff against main, identify real issues, and fix them. This skill runs an iterative review-fix loop up to 5 times or until no new issues are found.

## Review Scope

Focus ONLY on:

- New bugs introduced by this diff
- Correctness issues (wrong logic, off-by-one, nil dereference, race conditions)
- Testing gaps (new code without test coverage, missing edge cases)
- Opportunities for refactoring that reduce complexity or improve clarity
- Problems that are specific to THIS diff

Do NOT flag:

- Issues that exist in the codebase already and are not introduced by this diff
- Suggestions for future improvements or nice-to-haves
- Style preferences that don't affect correctness
- Issues in generated files (*.pb.go, *_gen.go, wire_gen.go)

## Protocol

### Step 0: Determine the Diff

Get the merge base and full diff between main and the current branch:

```bash
git fetch origin main 2>/dev/null || true
MERGE_BASE=$(git merge-base origin/main HEAD)
git diff "$MERGE_BASE"...HEAD
```

Also get the list of changed files:

```bash
git diff --name-only "$MERGE_BASE"...HEAD
```

If there is no diff (branch is identical to main), inform the user and stop.

### Step 1: Review Loop

Set iteration = 1 and max_iterations = 5.

#### 1a: Analyze the Diff

Read each changed file in full (not just the diff hunks) to understand context. For each file, review the changes and identify issues in these categories:

- **Bugs** — Logic errors, nil pointer risks, missing error handling, incorrect assumptions
- **Correctness** — Wrong algorithm, off-by-one errors, race conditions, resource leaks
- **Testing gaps** — New exported functions/methods without tests, new behavior without regression tests, missing edge cases in existing tests
- **Refactoring opportunities** — Duplicated code introduced by the diff, unnecessarily complex logic, dead code added by the diff

For each issue found, record:

- File path and line number(s)
- Category (bug, correctness, testing gap, refactoring)
- Description of the problem
- Suggested fix

#### 1b: Report Issues

List all found issues clearly with file paths, line numbers, and descriptions.

If 0 issues found: Report that the review is clean and stop. No further iterations needed.

#### 1c: Fix Issues

For each issue found:

1. Implement the fix
2. Compile-check the affected package: `go build -mod=vendor ./path/to/package/...`
3. Run targeted tests: `go test -mod=vendor ./path/to/package/...`
4. If tests fail, fix the failure before moving to the next issue

After all issues are fixed, run the full test suite:

```bash
make test
```

If the full suite fails, fix the failures.

#### 1d: Iterate

Increment iteration. If iteration > max_iterations, stop and report the summary.

Otherwise, re-diff against main (the diff has changed because fixes were applied):

```bash
git diff "$MERGE_BASE"...HEAD
```

Go back to Step 1a with the updated diff. Only look for **new** issues — do not re-flag issues that were already found and fixed in previous iterations.

### Step 2: Summary

When the loop ends (either 0 issues found or max iterations reached), provide a summary:

- Number of review iterations completed
- Total issues found and fixed across all iterations
- Issues by category (bugs, correctness, testing gaps, refactoring)
- Any remaining concerns (if max iterations was reached with issues still being found)

## Important Notes

- **Read full files, not just diffs.** Diff hunks lack context. Read the complete file to understand what the changed code interacts with.
- **Test after every fix.** Never batch fixes without verification. A fix can introduce new problems.
- **Do not fix pre-existing issues.** If a bug exists on main and the diff doesn't touch it, leave it alone.
- **Generated files are off-limits.** Skip wire_gen.go, *.pb.go, *_gen.go, and similar generated files entirely.
- **Refactoring must be safe.** Only suggest refactoring when it clearly reduces complexity without changing behavior. When in doubt, skip it.
