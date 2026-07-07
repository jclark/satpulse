---
name: github-issue
description: Create, edit, comment on, and label GitHub issues following the project's style
user-invocable: true
allowed-tools: Bash, Glob, Grep, Read, Write, WebSearch, WebFetch
---

# Working with GitHub issues

This skill covers all work with GitHub issues: creating new issues (`gh issue create`), editing existing ones (`gh issue edit`), commenting (`gh issue comment`), and managing labels. Whenever you write or revise issue text, follow the style conventions below.

When reading an issue, use `gh issue view N --json title,body,state,labels,comments` rather than the bare command - the default template queries a deprecated GraphQL field (Projects classic) and fails on older gh versions.

## Title

- Short and descriptive, under ~80 characters
- Starts with a verb phrase ("Add ...", "Make ...", "Support ...") or a noun phrase describing the problem/need
- Sentence case (capitalize only the first word and proper nouns)
- No trailing period

Examples:
- "Add include support to message files"
- "Need better errors when no suitable time messages are being received"
- "Change TimeMsg.Accuracy from time.Duration to float64 seconds"

## Body

The body is plain prose -- no markdown headings. Structure it as:

- Opening paragraph explaining what and why (1-2 sentences). Lead with the motivation or problem, not the solution.
- Further paragraphs with details as needed: concrete examples, code snippets, config fragments in fenced code blocks with language tags.
- Bullet lists for enumerating items (key semantics, requirements, affected areas).
- Cross-references to related issues using `#NNN`.
- Pointers to plan documents or external references if they exist.
- Task checklist (optional, for larger issues): `- [ ]` / `- [x]`

Do not use markdown headings (`#`, `##`, etc.) in the body. Keep formatting light: paragraphs, bullet lists, code blocks, backticks for identifiers. Assume the reader knows the codebase. Explain *why*, not just *what*. No greeting, sign-off, or filler. Sentence case. ASCII only.

Do not hard-wrap body prose into fixed-width lines. Write each paragraph as a single long line and let GitHub wrap it for display. Hard line breaks inside a paragraph render as awkward wrapping in the GitHub UI. (This does not apply to code blocks, which are preformatted.)

## Labels

Add labels when they clearly apply. Common labels:
- `gps` - relates to GPS subsystem
- `time-sync` - relates to time sync implementation
- `RTK` - relates to RTK and RTCM

## Example

```
gh issue create --title "Add include support to message files" --label gps --body "$(cat <<'EOF'
Add a `[[include]]` mechanism to GPS message files so that shared tag groups can be defined once and reused across multiple message files.

For example, `atgm332d-v5.toml` and `atgm332d-v6.toml` share many CASIC messages that could be factored into a common included file, with each version file overriding only the tags that differ.

```toml
[[include]]
src = "common/atgm332d-common.toml"
```

Key semantics:
- Tags defined in the including file override tags from included files
- Two unrelated files defining the same tag without an override is an error

See `plan/msgfile-include.md` for detailed design.
EOF
)"
```
