# gpshwtest

Read `GOAL.md` in full before working in this directory. It defines what this program is for, the semantics under test, what counts as a failure versus receiver-limitation data, the required coverage, and the ground rules for touching real GPS hardware.

`NOTES.md` records findings from hardware sessions so far (receiver behavior already measured); read it before probing, and append what you learn.

Type checking: `make typecheck` (mypy strict via uv, as in `smoketest/`).
