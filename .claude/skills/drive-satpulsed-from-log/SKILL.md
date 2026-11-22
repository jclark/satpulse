---
name: drive-satpulsed-from-log
description: Drive a test satpulsed instance from a recorded packet log, with no GPS hardware, by replaying the log through a FIFO with original packet pacing. Use only when the running daemon itself is under test, such as its web interface or HTTP endpoints. To see the events a packet log produces, or to check that a decoding change still handles a capture, use `satpulsetool replay` instead, which needs no daemon.
user-invocable: true
allowed-tools: Read, Bash, Glob, Grep, Write, Edit, AskUserQuestion
---

# Drive satpulsed from a packet log

If you only need the events a packet log produces, for example to check that a decoding change still handles a capture, do not use this skill: run `satpulsetool replay --vendor <vendor> <log>`, which writes the events the log generates as JSONL to standard output, like an event log, with no daemon, FIFO, or config (see **satpulsetool(1)**). Use this skill only when the running daemon itself is under test.

Replay a recorded JSONL packet log into `satpulsed` through a FIFO, so the daemon runs its normal serial-input path with no GPS hardware attached. Useful for testing the web interface, or verifying the scan/decode/event pipeline, from a captured log.

## Setup

Use the `satpulsed-test-instance` skill to build and configure the daemon, with these specifics:

- Set `[serial] device` to a FIFO path with a sensible basename. The basename becomes the log basename (`/tmp/fifo0` produces `event.fifo0.jsonl`), so avoid a confusing name.
- Enable `[[http]]` on a non-2000 port if you want the web interface.
- The event log (`[log] event = true`) is optional. Enable it to confirm the replay pipeline is producing events; skip it if you only care about the web interface. The event log is always a good way to check the pipeline is working, but it is not always needed.

## Create the FIFO and start satpulsed

```bash
mkfifo /tmp/fifo0
```

Start `satpulsed` in the background (per the setup skill) with `device = "/tmp/fifo0"`. `satpulsed` opens the FIFO `O_RDWR`, so the order does not matter: if `pack` starts first its write will block until the daemon opens the FIFO for reading.

## Replay the log

Pick a packet log, for example `/var/log/satpulse/packet.ttyACM0.jsonl`, or a smaller test fixture. Replay it into the FIFO in the background:

```bash
out/$ARCH/satpulsetool pack --realtime \
  /var/log/satpulse/packet.ttyACM0.jsonl > /tmp/fifo0
```

- `--realtime` reproduces the original inter-packet pacing (about 1 Hz for typical logs). Without it, `pack` writes as fast as the daemon reads.
- `pack` writes only packets received from the receiver; `out:true` and non-packet metadata entries are skipped automatically.
- Use `--tag`/`--msg` to replay only some packets. See **satpulsetool-pack(1)**.

## Verify

- **Web interface:** `curl http://localhost:2001/position` should return a position that advances as the replay runs, or open the GUI at `http://localhost:2001/`.
- **Pipeline (if the event log is enabled):** the event log should grow and contain the expected event types (`time`, `posGeo`, `velGeo`, `navEpoch`, `satellites`, ...).

## Teardown

Stop the daemon per the setup skill. Because the daemon holds the FIFO `O_RDWR`, it does not exit when `pack` finishes, so you must stop it explicitly. Once the daemon is gone, `pack` exits on `SIGPIPE` (or kill it).
