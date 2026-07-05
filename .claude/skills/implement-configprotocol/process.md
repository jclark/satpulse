# Working process and loop artifacts

The CASIC implementation ran as an autonomous loop of bounded
increments over ~36 iterations. This structure is what made it work
across context compaction and owner absence; reuse it.

## The artifact set

- `GOAL.md` (repo root, untracked): the assignment. Names the
  governing documents and orders them read before acting; working
  rules (branch, build, commit discipline); a checkable definition of
  done; the loop protocol itself. This is the entry point - everything
  else hangs off it.
- `plan/<proto>-config.md` (committed): the staged design plan.
  Stage 0 is resolving the design-shaping unknowns on hardware
  (protocol-questions.md) BEFORE the design is committed; later stages
  build feature by feature. Deviations from the plan are fine if
  recorded with rationale.
- `CONTEXT.md` (untracked): owner directives and hardware findings
  already verified - read before touching hardware, never rediscover
  what it records.
- `PROGRESS.md` (untracked): the loop's memory. The conversation can
  be compacted at any time; the file is the memory. Per iteration:
  what was done (commit hashes), hardware findings (per-device quirks,
  NAKs, fallbacks), open questions, and the explicitly chosen next
  step. Keep an owner-rulings section and an open-issues section; end
  with the final report (design chosen and why, fallbacks per
  receiver, verification evidence, what still needs hardware or a
  human decision).

## Loop discipline

- Each iteration: read GOAL + PROGRESS + relevant plan stage; pick an
  increment small enough to finish and verify this iteration; do it;
  verify (tests, hardware evidence); commit; update PROGRESS.md.
- make test before claiming an increment done - and check the output
  BEFORE committing, not in the same command.
- Long hardware runs go to the background with a waiter; do the next
  useful offline increment while waiting.
- Use real hardware whenever the plan calls for hardware evidence.
  Check CLAUDE.local.md for what is attached; check satpulsed is not
  running before opening a device; restore receivers after every
  session and verify the restoration.

## Branch and commit discipline

- Feature branch for the configurator; tooling/framework changes on
  their own branches landing independently (owner ruling - see
  rulings.md). One concern per commit.
- Never `git add -A`/`git add .`; `git add -u` plus new files by name.
- make, never `go build`.

## Operational gotchas (each cost real time)

- `make` before EVERY hardware run of out/<arch> binaries: a hardware
  test against a stale binary once triggered a phantom-bug hunt
  through the tool plumbing (the code was fine; the binary was old).
- Heredoc scripts do not survive nohup backgrounding (the script gets
  mangled); write a script file and run that.
- `pkill -f`/`pgrep -f` patterns match the invoking shell's own
  command line; bracket a character (`[g]pshwtest`) to avoid
  self-matching (one waiter loop never fired and one shell killed
  itself this way).
- Python edit scripts silently no-op when gofmt has changed the text
  since reading; assert every replacement applied.
- The Bash tool's working directory persists across calls; after
  `cd`-ing for a sub-build, later relative paths silently resolve
  wrong. Prefer absolute paths.
- On a saturated serial line, missing ACKs do not mean the command
  failed - distrust both directions and verify by readback or
  observation.
- Multi-worktree git: double-check `git branch --show-current` before
  merge/commit; a merge run in the wrong worktree is a silent no-op
  (or worse).

## Interacting with the owner

Bring analysis and a recommendation, not open questions (rulings.md,
"Answer your own questions"). When the owner asks a factual question
("does X work?", "does the harness check this?"), the answer may
require running an experiment - run it, then answer with evidence.
Record every directive verbatim-ish in PROGRESS.md the moment it
arrives; directives are binding across the whole session and beyond.
