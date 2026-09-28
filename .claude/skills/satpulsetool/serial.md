# satpulsetool serial

Examines serial ports. It only reads: it never transmits a byte to the device
(`gps` talks, `serial` listens), so every `serial` operation is safe on a live
receiver. The target is `-d DEV` (one port) or `-a` (every discovered port).
Full reference: satpulsetool-serial(1). Four problems it solves:

## 1. Which serial ports exist, and what is plugged into each?

With no target, lists every discovered port without opening it, one line per
port with USB vid/pid, serial number, and aliases - enough to tell which port
is which receiver. `-j` gives JSON Lines; `-i -d DEV` describes one port:

```
satpulsetool serial
satpulsetool serial -i -d /dev/ttyUSB0
```

## 2. What speed is the receiver running at?

With a target and no other options, tries reading at different speeds until
receiver output decodes, and prints the detected speed. The result is a bare
number, so it composes with `gps`:

```
satpulsetool serial -d /dev/ttyACM0
satpulsetool gps -d /dev/ttyACM0 -s $(satpulsetool serial -d /dev/ttyACM0) --show-receiver
```

`-a` instead of `-d` detects the speed on every port: use this when neither
the device nor the speed is known. Add `--packet-log FILE` to also record the
packets received during detection.

## 3. What is the receiver sending?

Passive packet capture to a JSON Lines file. Give the speed with `-s` (0 keeps
the port's current speed) and bound the capture with `-t N` seconds, since the
default with `--packet-log` is to run until interrupted:

```
satpulsetool serial -d /dev/ttyACM0 -s 38400 --packet-log capture.jsonl -t 10
```

Then decode it with `annotate` (see `decode.md`).

## 4. Is a PPS signal wired to a modem-control pin?

`-p PIN` with `cts`, `dcd`, `dsr`, or `ri` watches that pin for pulse edges
and timestamps each one. This is how to check PPS wiring or judge edge timing
quality. `-a` scans every port. `-p` alone leaves the port speed unchanged;
add `-s` and `--packet-log` to capture packets while timing edges. The default
`-t` with `-p` is 10 seconds.

```
satpulsetool serial -p cts -d /dev/ttyUSB0 -t 30
satpulsetool serial -p cts -a
satpulsetool serial -p cts -s 38400 -t 30 -d /dev/ttyUSB0 --packet-log capture.jsonl
```

Options that matter when judging timing (all require `-p`):

- `-I` inverts polarity: use it if edges trail the start of the second by the pulse width (typically 0.1 s).
- `-m poll|wait|kernel` picks how edges are detected; omitted, the best available method is used.
- `--poll-pre-warm SECONDS` (0.02 to 0.05 suggested) makes `poll` more precise on hosts that slow down when idle.
- `--max-wakeup-latency SECONDS` (Linux only) trades power for precision.
- `-j` gives one JSON object per edge with `t` (RFC 3339 UTC) and, for `poll`, `uncertainty`, `settling`, and `outlier`; ignore edges marked `settling` or `outlier` when assessing quality.
