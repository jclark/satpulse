# gpshwtest

Tests satpulsetool's device-independent GPS configuration against real receivers. It probes a receiver through `satpulsetool gps --json` invocations, records every step, and derives two outputs offline: **failures** (violations of the tool's device-independent guarantees) and a **characterization** of how configuration is realized on that receiver (its limitations relative to the full model). Signal-set probing uses targeted coexistence hypotheses rather than exhaustive subset sweeps. Receiver limitations are never failures. `GOAL.md` defines the goal and the workflow; `SEMANTICS.md` defines the semantics under test.

Python 3, stdlib-only at runtime. Type checking: `make typecheck` (mypy strict via uv).

## Usage

```
python3 gpshwtest -d /dev/ttyACM0 [-s 38400]    probe a receiver
python3 gpshwtest -f /etc/satpulse.toml         connection from a satpulse config
python3 gpshwtest --analyze LOGDIR              re-analyze a recorded log directory
python3 gpshwtest --restore-from LOGDIR         restore a receiver after a crashed run
```

## Options

- `-d, --serial-device DEV` / `-s, --device-speed BPS` - the receiver's serial device and speed. Without `-s` on a UART, the speed reported by `--show-port` is used; if the receiver does not answer, candidate speeds are scanned.
- `-f, --config-file PATH` - read device and speed (and the `[phc]` table) from a satpulse TOML config instead.
- `--satpulsetool PATH` - the satpulsetool binary (default: from `PATH`).
- `--logdir DIR` - parent directory for the per-run log directory, named `gpshwtest-<timestamp>-<device>` (default: `/tmp`). Giving `--logdir` implies `--keep-logdir`.
- `--keep-logdir` - keep the log directory even on a clean exit. By default a clean run removes its log directory; a run that exits nonzero always keeps it, since the verdicts refer to its contents.
- `--baseline FILE` - compare the characterization against a checked-in one (default: no comparison). Checked-in baselines live in `baselines/<hardware>-<firmware>.json`. When the run was not `--disruptive`, the disruptive-only entries are stripped from the baseline before comparing.
- `--disruptive` - also run the probes that write NVM, reboot the receiver, or change its speed (`--save`, `--save-all`, `--reset`, `--factory-reset`, `--speed`), with recovery.
- `--setup SCRIPT` - the receiver's documented starting-state script (`setup/<receiver>.sh`), run after the factory reset on a `--disruptive` run with the device and speed as arguments, so the run starts from the same state a bring-up establishes.
- `--sudo` / `--phc IFACE:PIN[:CHAN]` - verify the time pulse electrically through a PHC pin (requires root via passwordless `sudo -n`; the pin defaults to the `[phc]` table of the config file or `/etc/satpulse.toml`). Without root or wiring, physical checks are skipped.
- `--analyze LOGDIR` - re-derive failures and the characterization from a recorded log directory, without touching hardware. Improving the analysis never requires re-running receivers.
- `--restore-from LOGDIR` - run only the restore tail derived from a crashed run's records, then verify the receiver state. For runs that died without their in-process restore (kill -9, power loss); a run that aborts normally restores the receiver itself.

## Output and exit status

The characterization JSON goes to stdout (and to `characterization.json` in the log directory); progress, failures, and the baseline diff go to stderr.

- `0` - no failures, and the characterization matches the baseline (when given).
- `1` - the characterization differs from the baseline: a behavior change to investigate and re-vet, in satpulsetool, in the program, or from a firmware change.
- `2` - failures or errors: tool-guarantee violations, a broken receiver session, or unanalyzable records. Failures dominate when both occur.

## The log directory

- `runs.jsonl` - the per-invocation record: each step's intent (in model vocabulary), argv, exit code, JSON output, and stderr.
- `NNN-<name>.jsonl` - one packet log per invocation, every packet in both directions.
- `characterization.json` - the derived characterization.

A log directory is self-contained: `--analyze` re-derives all verdicts from it offline (packet logs are interpreted with `satpulsetool replay`).

## Concurrency

One receiver per invocation. Sweeps over different receivers run in parallel from the caller; log directory names include the device so concurrent starts do not collide. satpulsetool itself locks the serial device per invocation.
