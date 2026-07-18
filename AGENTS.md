# Agent instructions

## Proactive sandbox elevation

This workspace uses automatic approval for elevation. Request elevated permissions on the first attempt whenever a command is expected or reasonably likely to need resources outside the workspace sandbox. Do not run it sandboxed first merely to see whether it fails.

Always request elevation for:

- `gh` commands and network-facing Git commands
- Git commands that modify branches, refs, the index, or commits
- Go builds, tests, generation, and project build scripts
- commands that use normal user, module, build, or tool caches
- authentication, downloads, network access, sockets, shared memory, or hardware access

Run the normal command unchanged with elevation. Do not redirect caches to `/tmp`, replace credentials, disable functionality, or otherwise create a sandbox-specific workaround.

Read-only commands confined to the workspace, such as `rg`, file inspection, `git diff`, `git status`, and `git log`, do not normally need elevation. When uncertain whether a required command needs outside access, prefer requesting elevation immediately.
