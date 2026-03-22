# Message file includes

Add a file inclusion mechanism to GPS message files so that shared tag groups can be defined once and reused across multiple message files.

## Syntax

```toml
# Override gnss-gps from the included file with V6-specific command
[[casbin]]
tag = "gnss-gps"
description = "Enable GPS signals only"
class = 0x06
id = 0x0F
payload.types = "U1U1U2U4U4"
payload.values = [0, 0, 0, 0x00000005, 0x00000005]

[[include]]
src = "common/atgm332d-common.toml"
```

`[[include]]` is a new top-level array-of-tables entry. The `src` key is a path to another message file. Forward slashes in `src` are converted to the OS-specific directory separator via `filepath.FromSlash`. Relative paths are resolved relative to the directory of the including file, not the working directory.

## Semantics

### Tag override

Tags defined in the including file override tags from included files. If the including file defines tag `pps-gps`, any messages with that tag from included files are silently dropped.

### Conflict between included files

If two different included files both define the same tag, and the including file does not override it, that is an error.

### Defaults

Each file has its own `[default.*]` sections. Defaults do not propagate across file boundaries.

### Recursive includes

Included files may themselves contain `[[include]]` entries. A file may only appear once in the entire inclusion tree: if two different files both include the same file, that is an error (not just circular includes). This is simple to check and avoids duplicate messages.

### Message ordering

Messages from the including file appear first, followed by messages from included files in the order the `[[include]]` entries appear.

**Limitation:** TOML decodes `[[include]]` and message entries (e.g. `[[casbin]]`) into separate slices, so their relative position in the source file is not preserved. This means includes cannot be interleaved with messages to control ordering. By convention, `[[include]]` entries should be placed last in the file so that the source order matches the effective order.

### Error handling

It is a fatal error if any included file cannot be opened or parsed.

## Implementation

All changes are in `gps/msgfile/msgfile.go` (and additions to `msgfile_test.go`).

### Parsed stays unchanged

`Parsed` keeps its existing fields (`Default`, `Line`, `Binary`, `NMEA`, `CASBIN`, `ASBIN`, `UBX`, etc.). All existing methods (`buildTagIndices`, `TaggedMsgs`, `TagDescs`, `ValidateTags`, `ToRaw`, `filterMsgs`, `tagIndex`) stay unchanged. Callers and tests see the same API.

`Load` produces a `Parsed` whose message slices are the combined result of all files after tag ownership is resolved. From the caller's perspective, it looks the same as a single-file `Parsed`.

### New internal types

```go
type IncludeEntry struct {
    Src string `toml:"src"`
}

// fileContent is used for TOML decoding. It embeds Parsed
// and adds the Include field.
type fileContent struct {
    Parsed
    Include []IncludeEntry `toml:"include"`
}

// loadedFile tracks a parsed file during the Load tree walk.
type loadedFile struct {
    path string       // absolute path
    out  int          // DFS out-index (In is the index in the loadedFile slice)
    p    Parsed       // per-file parsed content with defaults applied
}
```

TOML decodes into `fileContent`. The embedded `Parsed` receives all existing TOML fields; `Include` receives `[[include]]` entries. After decoding, the `Parsed` part is extracted for per-file processing.

### Load changes

`Load` still takes a path and returns `*Parsed`. Internally it builds a slice of `loadedFile` via recursive tree walk, then merges into a single `Parsed`.

**Tree walk** (`loadTree`):

1. Convert path to absolute (`filepath.Abs`).
2. Check for duplicates: linear search of the `[]loadedFile` slice by path. Error if already present (catches both circular and diamond includes).
3. TOML-decode into `fileContent` (using `newDefault()` for the embedded `Parsed`).
4. Apply per-file defaults to all message types (so each message's tag is known). Do NOT validate defaults here -- validation stays in `buildTagIndices` to preserve existing error-timing behavior.
5. Append to the `[]loadedFile` slice (this file's `In` is its index).
6. For each `Include` entry, resolve path relative to this file's directory: `filepath.Join(filepath.Dir(absPath), filepath.FromSlash(entry.Src))`. Recurse.
7. After all includes return, set this file's `Out` to `len(files)`.

Files end up in preorder: each file is appended before its includes.

**DFS interval labeling.** Each file has `In` (its index) and `Out` (set after all its includes). File X is an ancestor of file Y iff `X.In <= Y.In && X.Out >= Y.Out`.

Example: A includes B1, B2. B1 includes C.

```
File  In  Out
A      0    4
B1     1    3
C      2    3
B2     3    4
```

A's interval [0,4) contains all others (ancestor of all). B1's [1,3) contains C's [2,3) but not B2's [3,4).

**Tag ownership.** After the tree walk, iterate files in preorder. Maintain `tagOwner map[string]int` (tag name to file index of first definer).

For each file, for each tag it defines (from its already-defaulted messages):
- Not in map: this file owns it. Set `tagOwner[tag] = fileIndex`.
- Already in map: check if the existing owner is an ancestor of this file (`files[owner].Out >= files[current].Out`). If yes, valid override -- the owner's messages win and this file's messages for this tag will be skipped. If no, error (two unrelated files define the same tag with no dominator).

**Merge.** Build the combined `Parsed`. Start with the root file's `Parsed` as the base (preserving its `Default` section so existing behavior is identical for single-file cases). Clear its message slices, then repopulate by iterating files in preorder, appending only messages whose tag is owned by that file:

```
for each file i:
    for each message m in file i (e.g. file.p.Line):
        if tagOwner[*m.Tag] == i:
            append m to result.Line
```

The resulting `Parsed` has combined message slices with defaults already applied, and the root file's `Default` section. All existing methods work on it unchanged:
- `buildTagIndices`: `validateDefaults` runs on the root file's defaults (same as today for single-file). `applyDefaults` is a no-op (already applied per-file). Consecutive check passes (each tag comes from one file, was consecutive there). Cross-type check works on the combined result.
- `TaggedMsgs`, `TagDescs`, `ValidateTags`, `ToRaw`, `filterMsgs`: no changes.

### Path handling

- `filepath.Abs` on the initial path and on each resolved include path.
- `filepath.FromSlash(src)` converts forward slashes to OS separator.
- `filepath.Join(filepath.Dir(currentAbsPath), converted)` resolves relative to the including file.
- Duplicate detection is a linear search of the `[]loadedFile` slice by path.

## Format documentation

Update `configs/gpsmsg/format.md` with a new "Includes" section.

## JSON schema

Update `configs/gpsmsg/gpsmsg-schema.json` to allow `[[include]]` with `src` as a required string property.

## Testing

Add tests to `gps/msgfile/msgfile_test.go`:

- Basic include: file A includes file B, tags from both are available.
- Tag override: file A defines tag `x`, included file B also defines tag `x`; only A's messages for `x` are used.
- Conflict: file A includes B and C, both define tag `y`, A does not override; error.
- Circular include: A includes B, B includes A; error.
- Diamond include: A includes B and C, both include D; error.
- Relative path resolution: included file in a subdirectory.
- Missing included file: error with the path.
- Recursive include with override: A includes B, B includes C, A overrides a tag from C.
- Included file defaults are independent.
- Self-include: A includes itself; error.
- Empty `src`: error.
- Empty-tag override: root file has untagged messages, included file also has untagged messages; root wins.
- Single file (no includes) still works identically to before.
