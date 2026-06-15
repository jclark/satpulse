# gpshwtest systest

Integrate `gpshwtest` with the existing `systest/` workflow without turning the
playbooks into long-running blocking jobs.

The existing systest style is phase-oriented: `start.yml` starts SatPulse,
`check.yml` inspects a running service, and `stop.yml` stops it. The gpshwtest
integration should follow that style with separate start, status, and verify
playbooks.

## Assumptions

- `gpshwtest` is deployed as a zipapp.
- The playbooks use the existing inventory variables for `instance`,
  `config_file`, `interface`, `pin`, `channel`, and related device facts.
- One optional inventory variable is added:

```yaml
baseline: ""
```

When set, `baseline` is relative to `gpshwtest/baselines/` and omits the
`.json` extension, for example:

```yaml
baseline: UM980-Build17548
```

No gpshwtest playbook restarts `satpulse@{{ instance }}`. The operator runs the
existing `start.yml` explicitly when SatPulse should be started again.

## Remote layout

Use a fixed directory on each target, scoped by instance:

```text
/var/tmp/satpulse-gpshwtest/{{ instance }}/
  gpshwtest.pyz
  active.json
  runs/
    <timestamp>/
      stdout.log
      stderr.log
      rc
      logdir/
```

The fixed layout makes status and verify independent of the Ansible process
that started the run. Only one active run is allowed per target instance.

## gpshwtest-start.yml

Start a gpshwtest run asynchronously and return quickly.

Tasks:

1. Ensure the remote layout exists.
2. Fail if `active.json` indicates a run is already active.
3. Copy the current `gpshwtest.pyz` to the target.
4. Stop `satpulse@{{ instance }}`.
5. Start a remote wrapper asynchronously.
6. Record active run state under `active.json`.

The wrapper should:

- create a timestamped run directory
- run `gpshwtest.pyz` with `-f {{ config_file }}`
- use a log directory inside the timestamped run directory
- write stdout, stderr, and exit status
- mark the run complete
- not restart SatPulse

The initial implementation can run the normal verification-oriented
`gpshwtest` command. The factory-reset-first disruptive command needed for the
configuration corpus can be added when that gpshwtest behavior is implemented.

## gpshwtest-status.yml

Report progress without changing system state.

Tasks:

1. Read `active.json` if present.
2. Check whether the recorded process is still running.
3. If running, print the current run directory, number of completed
   `runs.jsonl` records, and the most recent step name if available.
4. If complete, print the run directory and exit status.
5. If no run is active or complete, report that directly.

This playbook should not verify baselines, fetch artifacts, stop services, or
start services.

## gpshwtest-verify.yml

Verify a completed run on the target.

Tasks:

1. Require that the latest run is complete.
2. If `baseline` is set, copy
   `gpshwtest/baselines/{{ baseline }}.json` from the controller to the remote
   run area.
3. Run `gpshwtest.pyz --analyze <logdir>` on the target.
4. If a baseline was copied, pass it with `--baseline` and fail on mismatch or
   analysis failure.
5. If no baseline is set, run analysis without comparison and print one concise
   line naming the generated characterization path.

Example no-baseline message:

```text
No baseline for serpa.lan ttyUSB0; generated <remote>/characterization.json
```

`gpshwtest-verify.yml` should not fetch the run directory. Manual retrieval is
left to the operator when a new baseline or corpus artifact is wanted.

## Typical workflow

```sh
./t gpshwtest-start
./t gpshwtest-status
./t gpshwtest-status
./t gpshwtest-verify
./t start
```

For a new receiver with no baseline:

1. Leave `baseline` empty or unset.
2. Run start/status/verify.
3. Inspect the generated characterization manually on the target.
4. Copy it into `gpshwtest/baselines/`.
5. Add the extensionless baseline name to `systest/inventory.yml`.

## Later work

- Add a zipapp build target.
- Add baseline entries for known receivers.
- Decide whether gpshwtest-start should always use the factory-reset-first
  disruptive mode once the current baselines have been regenerated.
- Add a separate corpus extraction workflow that consumes completed disruptive
  gpshwtest run directories and writes `gps/testdata/config`.
