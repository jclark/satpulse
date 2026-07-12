# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

For detailed documentation of the package structure, dependencies, and layering, see @docs/internals.md.

Whenever you create a new package, add an entry describing it to the appropriate section of @docs/internals.md.

## Interaction

- Do not ask the user multiple-choice questions, and never use the `AskUserQuestion` tool (the one that renders selectable options). When you need a decision, state your recommendation in prose and let the user respond freely in their own words.
- When the user asks a question or is discussing a design, answer the question; do not edit code or docs, write plans, or fix anything until explicitly told to. Agreement to one step is not agreement to the next.
- Keep design discussions exploratory: present framings and trade-offs, not a single forced answer. The user decides; never assume which option he will pick.
- Report only deltas: what changed or what still needs fixing, never what stayed the same or is already fine.
- User messages are often dictated; resolve phonetic garbles to project terms (e.g. "Southpaw" for satpulse, "Team Mode 2" for TMODE2) before acting, and ask only if genuinely ambiguous.
- The user is a domain expert in GNSS, timing, and XML. When he questions a value or design, investigate the specific question; do not explain fundamentals.
- The user edits files between messages; re-read the current version of a file before commenting on it again.

## Technical claims

- Never state a technical claim you have not verified against the code, the vendor documentation, or a test. No speculation.
- Protocol facts come from the local vendor protocol documentation (see CLAUDE.local.md for its location), not from memory or web search.
- In protocol code, use the vendor spec's own terminology for names; never invent terms.

## Go code style

**CRITICAL: You MUST follow these rules for ALL Go code you write or modify in this repository. These rules override any default Go conventions you might know. Check each rule before generating code.**

### Consistency is paramount
- When modifying existing code, match the style of the surrounding code
- Consistency hierarchy: function > file > package > repo

### Variable/function naming
- Use short names for local variables with limited scope:
  - Loop indices: `i`, `j`, `k`
  - Common types: `s` (string), `b` (byte/buffer), `n` (number/count), `err` (error)
  - Short-lived intermediates: `v` (value), `ok` (bool from map/type assertion)
  - Abbreviate name of type
- Longer names are OK for:
  - Package-level variables, especially if exported
  - Struct fields
  - Variables with wider scope or complex meaning
- For exported function/variables, the name the user sees is the package name plus the function name

### Simplicity
- Prefer modest, targeted diffs; do not rewrite or reformat code beyond the requested change (no gofmt churn on untouched lines)
- No premature generalization: generalize when the second case arrives, not before
- Do not create a new file for a small helper; put it in the existing file where it belongs
- No backwards-compatibility shims or migration paths: this is a pre-1.0, sole-developer project
- Keep tests proportional to the change; a trivial change does not need a battery of new tests
- Programmer errors (contract violations) panic; data-dependent failures return errors

### Code density and readability
- Minimize blank lines - use them only to separate large logical sections
- Inline simple expressions instead of creating single-use variables
  - Good: `return strings.Join(processStrings(input), ",")`
  - Avoid: `processed := processStrings(input); joined := strings.Join(processed, ","); return joined`
- DO create variables to avoid repetition:
  - Bad: `process(config.Server.Host, config.Server.Port)`
  - Good: `srv := config.Server; process(srv.Host, srv.Port)`

### Control flow
- Do not use a tagless `switch {}` on non-constant conditions when an if/else (or guard-clause) chain would work just as well; reserve `switch` for dispatch on a value (especially constants/types)

### Comments
- Every exported function needs a comment starting with the function name
- NO comments inside functions unless explaining non-obvious behavior
- Comments should explain WHY, not WHAT

### Character encoding
- Use ASCII only - avoid non-ASCII characters (no fancy quotes, checkmarks, emojis, etc.)
- Exception: math symbols where truly needed (e.g., μs for microseconds)

### Function ordering
- Order code for top-to-bottom readability: readers should understand what's happening without jumping around
- Type definitions come before the functions that use them
- Main/exported functions come before their helper functions
- Example ordering:
  1. Type definitions (structs, interfaces, constants)
  2. Constructor/factory functions
  3. Main methods on those types
  4. Helper functions used by the methods
- The goal: reading from top to bottom tells a story - what the types are, what the main operations are, then how they're implemented

## Development commands

**CRITICAL: Always use `make` to build. NEVER use `go build` directly - it creates binaries in the wrong location and clutters the repository.**

Build system uses GNU Make:
- `make` - Build for current architecture
- `make test` - Run all tests (`go test -v ./...`)
- `make install` - Install binaries and configs to `/usr/local/`
- `make pkg` - Build both deb and rpm packages
- `make clean` - Remove build artifacts

It builds on Linux only. On macOS, use `unix-build.sh` instead.

The web interfaces are built with npm from the `webui/` workspace. Their built
assets are checked in and embedded into the binaries, and `make` does not
rebuild them: after changing frontend sources you must regenerate and commit
the assets. See @webui/CLAUDE.md.

Testing:
- Individual package: `go test -v ./internal/packagename`
- All tests: `make test`
- Before committing a fix that changes Go code, always run the full test suite with `make test`
- Test files follow `*_test.go` convention
- Tests for `X.go` go in `X_test.go` by default; put them elsewhere only when that file would become very unwieldy

Black-box smoke tests of the real `satpulsed` binary live in `smoketest/`
(daemon-level config wiring, endpoints, logging, Ntrip, shutdown; no root or GPS
hardware). Build first with `make`, then run `make smoketest`. See
@smoketest/CLAUDE.md.

System testing on real hardware is doing using ansible in `systest/` directory.

## Code review

When asked to review code or a plan:
- Do not run tests unless explicitly requested - they have already been run.
- Do not modify any files.
- Check correctness (logic errors, off-by-one, nil risks, error handling, races) and consistency with the relevant `plan/*.md` if one is named.
- Be pragmatic and concise: flag real issues only - no nitpicks, no severity inflation.
- Deliver findings as a numbered list; when asked, walk through them one at a time.

## Git usage

- Never use `git add -A` or `git add .` - these add untracked files which may include test data or local files
- Use `git add -u` to stage modified/deleted tracked files, then add new files explicitly by name
- "Stage" means stage only. Stage, commit, and push are separate steps, each done only when explicitly requested; the user reviews staged changes before commit.
- Commit messages need an informative body explaining what the problem was and what the change does; match the style of recent `git log` entries.
- Put logically distinct changes in separate commits. Work that belongs on another branch does not go on this one.
- When multiple branches or worktrees are in play, verify the current branch before every commit; integration branches get merges only, never direct commits.
- Only create a branch when explicitly instructed to. Otherwise commit on the current branch, including the default branch.
- Prefer merge to rebase. Never rebase unless explicitly told to (the repo is checked out on multiple machines with different hardware, so rewriting shared history causes conflicts). Integrate diverged branches with `git merge`, not `git rebase` or `git pull --rebase`.
- When a commit completely resolves an issue, make `Fixes #N` (with the issue number) the last line of the commit message, so the issue closes when the commit merges.
- Never mention Claude, Claude Code, or any other AI agent or tool anywhere in a commit message, PR description, or issue - no co-authorship, attribution, "Generated with ..." line, chat/session link, emoji marker, or reference of any kind. These are public, so a private-chat link leaks it, and the history must read as the author's own work. Describe only the change itself.
- Never create a GitHub issue unless explicitly asked, even when writing a plan or notes that could become one.

## Development environment

System testing uses Ansible playbooks in `systest/`.

## Documentation style

- Headings use sentence case (capitalise only the first word and proper nouns)
- For prose the user is writing (docs, blog posts), fix typos, spelling, and grammar only; no rewrites or editorial improvements unless explicitly asked.
- Give prose corrections as exact word-level replacements with line numbers, one at a time; do not emit rewritten paragraphs.
- When editing an existing document, make minimal targeted edits that match the document's voice and style.

## Release notes

- Implementing a user-facing feature MUST include an entry in `docs/_includes/NEWS.md`, in the same change as the implementation.
- This applies to new features, behaviour changes, and upgrade notes. Bug fixes are excluded.
- Never add an entry for a bug fix, and do not add one when an existing entry already covers the change. Keep entries short.
- Add the entry under the current unreleased version heading, in the appropriate section, and reference the issue number(s) in parentheses to match the existing entries.

## Connected GPS

You can look at `/etc/satpulse.toml` if it exists to find device and speed of a connected GPS receiver.
But before using it, check that `satpulsed` is not running `ps ax | grep satpulsed`.
Use `satpulsetool gps` for operations that write to the receiver; do not send raw serial writes directly.
