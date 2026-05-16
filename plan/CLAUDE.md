# Plan / issue workflow

Each plan file in this directory describes the design for a piece of
work tracked by a GitHub issue.

## Plan file conventions

- Plan filenames are short, lowercase, hyphen-separated (e.g.
  `clock-bias-drift.md`).
- The first line is a `#` heading ending with the primary issue
  number in parentheses: `# Title (#N)`. Use this exact format -- not
  `Issue: #N`, not `Fixes: #N`, not an inline link.
- One plan, one primary issue. If a plan touches related issues,
  mention them in the body (e.g. "Related: #126"), but only the
  primary goes in the heading.
- When the work described by a plan is fully implemented, move the
  file to `plan/archive/` (use `git mv`).

## Issue conventions

- An issue whose primary plan lives in `plan/` has the `has plan`
  label. The label means there is a dedicated plan file for the
  issue -- not that the issue is merely mentioned by some plan.
- If a plan is split across multiple issues, only the primary issue
  carries the label. Related issues mentioned by the plan do not.
- Don't create a plan file without filing the issue first (or
  alongside). The heading must reference a real, open issue.

## What goes in the issue vs the plan

The issue is short. The plan is where the detail lives. A reader who
skims the issue should understand *what* and *why*; a reader who
needs to do the work goes to the plan for *how*.

Issue body, in roughly this order:
- One short paragraph on the problem or motivation (current state if
  not obvious from the code).
- One short paragraph on the proposed direction, at the level of
  "what changes, where", not implementation steps.
- Dependencies on other issues, by number.
- A line pointing to the plan: `See `plan/foo.md`.`

Keep out of the issue:
- Implementation steps, code snippets, struct definitions, file
  paths, function names -- all belong in the plan.
- Speculation that isn't in the plan. If you find yourself inventing
  details to flesh out the issue, stop and check the plan.
- Negative framing of the current code ("primitive", "ad-hoc",
  "tangled") unless that's literally how the plan describes it.
- Claims that overstate what's missing. Read the relevant code
  before asserting "X doesn't exist" or "the handlers discard Y".

Title:
- Describes the problem or the change, not the solution mechanism.
- Sentence case, no trailing period.
- Avoid stuffing implementation detail into the title.

Plan side:
- The plan can be as long as the design needs. Code snippets,
  alternatives considered, phasing, open decisions, testing strategy
  all live here.
- When the plan changes meaningfully (clarification, scope shift),
  update the plan first, then revise the issue if its summary is now
  inaccurate.

## When the issue number is wrong

If a plan's heading references a closed umbrella issue or a stale
number, fix the heading rather than working around it. The heading
is the source of truth for what issue the plan tracks.
