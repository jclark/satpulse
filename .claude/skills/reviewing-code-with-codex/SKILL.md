---
name: reviewing-code-with-codex
description: This skill should be used when the user asks to "review code", "code review", "review changes", or "review with codex". It uses Codex AI agent in read-only mode to review code changes for correctness and plan consistency, then acts on the findings. Tests should already have been run before invoking this skill.
allowed-tools: Bash, Glob, Grep, Read, Edit, Write, AskUserQuestion
user-invocable: true
---

# Reviewing code with Codex

Invoke Codex AI for a non-interactive code review focused on correctness and plan consistency, then evaluate and act on the findings.

Prerequisites: tests should already have been run and passed before using this skill.

## Step 1: Determine change status

Run `git status --porcelain` to classify changes:
- **Staged**: index column has status (first column is not space/`?`)
- **Modified**: worktree column has status (second column is not space)
- **Both**: some staged, some modified

## Step 2: Find the plan

Check the conversation context for a plan file that was used for the current implementation. If the user mentioned a plan file or one was read during the session, use that path.

If no plan is apparent from context, check for the most recently modified file in the `plan/` directory:

```bash
ls -t plan/*.md | head -1
```

Ask the user to confirm whether this is the right plan before proceeding. If they say there is no plan, skip the plan consistency check.

## Step 3: Run Codex

**Use a 15-minute timeout** (timeout: 900000) on the Bash tool call, since Codex with xhigh reasoning effort can take several minutes.

Build the prompt and run. The prompt must tell Codex:
1. Whether changes are staged, modified, or both (so it uses the right git diff command)
2. Not to run tests and not to modify files
3. To review for correctness
4. If a plan exists, to check implementation against the plan

When there is a plan file at PLAN_PATH:

```bash
# 2>/dev/null suppresses reasoning/thinking token output on stderr
codex exec --sandbox read-only --model gpt-5.3-codex -c model_reasoning_effort=xhigh "$(cat <<'PROMPT'
Review the CHANGE_STATUS code changes (use git diff or git diff --cached as appropriate).

Do NOT run any tests. Do NOT modify any files.

Review for correctness: logic errors, off-by-one errors, nil pointer risks, error handling bugs, race conditions.

Also read the plan at PLAN_PATH and check whether the implementation is consistent with the plan. Flag anywhere the code diverges from or is missing something specified in the plan.

Be concise. Only flag real issues.
PROMPT
)" 2>/dev/null
```

When there is no plan:

```bash
# 2>/dev/null suppresses reasoning/thinking token output on stderr
codex exec --sandbox read-only --model gpt-5.3-codex -c model_reasoning_effort=xhigh "$(cat <<'PROMPT'
Review the CHANGE_STATUS code changes (use git diff or git diff --cached as appropriate).

Do NOT run any tests. Do NOT modify any files.

Review for correctness: logic errors, off-by-one errors, nil pointer risks, error handling bugs, race conditions.

Be concise. Only flag real issues.
PROMPT
)" 2>/dev/null
```

Replace CHANGE_STATUS with "staged", "modified (unstaged)", or "staged and modified (unstaged)".

## Step 4: Evaluate and act on findings

Read the Codex output and critically evaluate each issue raised. Do not assume the reviewer is correct.

For each issue:

- **Agree with the finding**: fix the code directly.
- **Disagree with the finding**: present both perspectives to the user (the Codex argument and why you disagree) and ask the user to decide.
- **Uncertain**: present it to the user for a decision.

After addressing all findings, re-run tests on affected packages to confirm nothing was broken by the fixes.
