# GPS high-level configuration hardware tests

A stdlib-only Python harness that points `satpulsetool gps` at a live
receiver and verifies that requested configurations are applied, read
back consistently, and (in later phases) survive save/reload. Per case
it runs one invocation to set properties and a separate invocation to
read them back, so the readback is an independent probe-and-query
cycle. See `plan/gps-config-hwtest.md` (#310).

Usage (run from the repository root, after `make`):

    python3 gpshwtest -d /dev/ttyACM0 -s 38400 --all
    python3 gpshwtest -f /etc/satpulse.toml --signals --tp

Group flags select the test groups (`--signals`, `--time-gnss`, `--tp`,
`--ant-cable-delay`, `--min-elev`, `--mode`, `--rtcm-base-id`); `--all`
runs them all. Every invocation's `--packet-log`, plus `results.jsonl`
with one machine-readable entry per case, lands in the artifact
directory (`--artifact-dir`, default a temp dir that is kept on
failure or with `--keep-artifacts`). Exit code 0 means no case failed;
1 means failures; 2 means the receiver could not be probed.

The battery mutates receiver state but never writes non-volatile
memory; each group's last case leaves a sane configuration, and a
restart of satpulsed reconfigures the receiver per its own config.

Modules: `__main__.py` (CLI, group selection, summary), `runner.py`
(satpulsetool invocation, timeout, JSON and packet-log collection),
`cases.py` (test tables per group), `compare.py` (support-aware
comparators and the documented per-family tolerance rules).

Type checking: `make typecheck` (mypy strict via uv, as in
`smoketest/`).
