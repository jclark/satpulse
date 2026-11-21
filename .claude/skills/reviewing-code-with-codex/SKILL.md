---
name: reviewing-code-with-codex
description: Use when the user asks for a review by Codex, in words such as "review this with Codex", "get Codex Sol to review the branch", "ask Codex Astra to check the error handling". Runs a read-only Codex review through the Codex plugin, optionally with a model and focus wording taken from the request, then evaluates the findings and reports them without fixing anything. Tests should already have been run.
allowed-tools: Bash, Read, Grep, Glob
user-invocable: true
---

# Reviewing code with Codex

Run a read-only Codex review through the companion script of the Codex plugin for Claude Code
(`codex@openai-codex`, from `openai/codex-plugin-cc`), then evaluate what Codex found.
Do not ask the user anything before running it: take everything from the request.
The user can run the plugin's own `/codex:review` and `/codex:adversarial-review` directly;
those are user-only, which is why this skill calls the script.

## Step 1: Read the request

- **Model**: "Sol" means `gpt-6.2-sol`; "Astra" means `gpt-6.2-astra`.
  A full model id given by the user is used as is.
  With no model named, use `gpt-6.2-astra`.
- **Focus**: any wording about what to look at or question ("focus on the partial-failure handling",
  "challenge whether the mask belongs in phc").
  Keep the user's words; do not rewrite them.
- **Plan**: if the work being reviewed was implemented from a plan file that is apparent from the
  conversation, the review also checks consistency with it.
  Do not guess a plan from `plan/`.
- **Target**: what to review, if the user says ("the branch", "PR 544", "against master").

## Step 2: Choose the target

- If the user named a target, use it: a branch or PR is `--base <ref>`, with the PR's base from
  `gh pr view <n> --json baseRefName -q .baseRefName`.
- Otherwise, if `git status --porcelain` shows changes, review the working tree: `--scope working-tree`.
  This covers staged, unstaged and untracked changes together; the script cannot review only staged
  changes, so if the user asks for that, say so instead of running it.
- Otherwise review the branch against its PR's base (`gh pr view --json baseRefName -q .baseRefName`),
  or `master` if the branch has no PR: `--base <ref>`.
  Always pass the base explicitly: the script's own default is the default branch, which is wrong for
  a branch stacked on another PR.

## Step 3: Run Codex

Use the helper next to this file, which finds the installed plugin's companion script:

```
.claude/skills/reviewing-code-with-codex/codex-companion.sh review --model <model> <target>
.claude/skills/reviewing-code-with-codex/codex-companion.sh adversarial-review --model <model> <target> "<focus>"
```

- With no focus and no plan, use `review`: Codex's built-in reviewer, which takes no instructions.
- Otherwise use `adversarial-review`, the only review that takes instructions.
  It is framed to question the approach and design as well as the code.
  The focus text is the user's wording, then, if there is a plan, `Also check that the implementation
  is consistent with the plan in <path>, and flag where it diverges or omits something.`,
  then `Do not run tests. Do not modify files.`
- Neither command takes a reasoning effort: Codex uses the user's configured default.

Run it with Bash in the background (`run_in_background: true`): reviews take minutes.
Tell the user what is running (command, model, target), then wait for the completion notification;
do not poll.
If the helper reports that the plugin is missing or Codex is not set up or logged in,
pass that on and stop.

## Step 4: Evaluate the findings

Read the output and check each finding against the code. Do not assume Codex is right.
Report the findings as a numbered list, most severe first, each with Codex's file and line and,
for each one, whether you agree, disagree or are unsure, and why.
If Codex found nothing, say so.

Then stop. Do not fix anything until the user says which findings to fix.
After fixing, run the tests for the affected packages.
