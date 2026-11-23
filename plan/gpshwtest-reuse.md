# gpshwtest reuse

`gpshwtest` produces useful hardware evidence beyond its immediate
characterization report. There are two distinct reuse paths:

- a durable configuration packet corpus
- derived replay-test fixtures

These should stay separate. The corpus is raw observed receiver behavior.
Replay fixtures are a test harness artifact derived from selected evidence.

## Configuration packet corpus

The existing packet corpus in `gps/testdata/packets` is for periodic receiver
output: incoming-only traffic from ongoing receiver operation. Configuration is
a different world. Configuration traffic is stateful and bidirectional:
requests change receiver state, later responses depend on that state, and the
interesting durable fact is how a specific receiver firmware behaved through a
sequence.

The config corpus should live beside the packet corpus:

```text
gps/testdata/config/<vendor>/<model>/
  HW.toml
  gpshwtest001/
    001.jsonl
    002.jsonl
    003.jsonl
    ...
```

`HW.toml` describes the hardware and firmware for the directory, following the
same spirit as `gps/testdata/packets/<vendor>/<model>/HW.toml`.

Each `NNN.jsonl` is exactly a normal `satpulsetool gps --packet-log` file in
`gpsio.PacketLogEntry` format. There are no sidecars and no gpshwtest metadata
files in the corpus. The directory name identifies the producer/scenario class;
the numbered files are intentionally not named by topic. Tools that need to know
what is inside should search the packet logs.

The guarantee within one scenario directory is:

- `001.jsonl` starts from factory-reset receiver state.
- `002.jsonl` starts from the configuration state left by `001.jsonl`.
- In general, `NNN+1.jsonl` starts from the configuration state left by
  `NNN.jsonl`.
- The receiver may have emitted unrecorded periodic packets between files; the
  guarantee is about configuration state, not a byte-perfect continuous serial
  stream.

This makes the corpus useful when the hardware is unavailable. For example, a
receiver simulator test can start the simulator in factory-default state, feed
it the outbound packets from each numbered log in order, and compare its
responses with the inbound packets observed from real hardware while preserving
simulator state across files.

## Factory-reset gpshwtest runs

For corpus-quality evidence, a disruptive gpshwtest run should start by putting
the receiver into factory-reset state. The planned flow is:

1. Run `satpulsetool gps --factory-reset` and then `--reset` or equivalent
   recovery/resynchronization as needed for the receiver.
2. Start the full `gpshwtest --disruptive` run.
3. Compare and update the gpshwtest baselines from these new full disruptive
   runs; baseline changes are expected.
4. Export the resulting run directories into `gps/testdata/config`.

The current baselines were not captured under this rule, so they should be
regenerated after the factory-reset-first runs.

## Corpus extractor

Add a separate Python script that copies a gpshwtest run directory into a config
corpus scenario directory.

Input:

```text
gpshwtest run directory containing runs.jsonl and per-invocation packet logs
```

Output:

```text
gps/testdata/config/<vendor>/<model>/gpshwtestNNN/
  001.jsonl
  002.jsonl
  ...
```

Behavior:

- Read `runs.jsonl`.
- Walk recorded steps in order.
- For each step that references a packet log from `satpulsetool gps`, copy the
  packet log bytes unchanged to the next numbered `NNN.jsonl`.
- Skip non-packet-log records such as PHC `sdp` event captures.
- Require the destination scenario directory to be absent or empty unless a
  force option is given.
- Produce only packet-log files; do not produce metadata sidecars.
- Validate that copied files are parseable JSONL packet logs.

The extractor should not try to name files by probe type or infer semantic
topics. The ordering is the useful information.

## Replay fixture extraction

Replay fixtures are a separate derived product. They should not define the
corpus format and should not be placed in the config corpus by default.

Add a separate Python script that can build a `--test-log` format JSONL file
from selected parts of a gpshwtest run directory. This is for
`internal/gpscmd/replay_test.go` and ad hoc replay work only.

The open design problem is selection. Possible selection inputs:

- named presets such as `signals`, `mode`, `scalar`, `reload`, `output-config`,
  and `speed`
- filters over gpshwtest `runs.jsonl` intent fields
- explicit step names or sequence numbers

The replay extractor can use gpshwtest metadata because replay fixtures are
derived test artifacts, not the durable corpus. It should emit normal test-log
blocks containing `env`, packet entries, optional `receiver`, and final
`config`/`error` entries.

Selection criteria should be designed separately before implementing this
script. The initial corpus work does not depend on it.
