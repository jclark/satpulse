# gpshwtest

Read `GOAL.md` in full before working in this directory. It defines what this program is for, the semantics under test, what counts as a failure versus receiver-limitation data, the required coverage, and the ground rules for touching real GPS hardware.

The semantics under test are documented in `SEMANTICS.md`; read it before interpreting receiver behavior. Findings from hardware sessions are split by kind: `HW/<receiver>.md` describes each receiver's limitations relative to the full model, and `BUGS.md` records clear satpulsetool bugs found. Read them before probing, and record what you learn in the right one. `IDEAS.md` holds suggested design improvements toward GOAL.md - ideas to adopt, beat, or update.

Type checking: `make typecheck` (mypy strict via uv, as in `smoketest/`).
