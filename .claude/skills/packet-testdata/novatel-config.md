# NovAtel-format capture details

This applies to receivers that output NovAtel-format logs (`NOVB`, `NOVA` and `NOVAA` packets) and are configured with low-level message files: ByNav and SinoGNSS. The logs and the `LOG` command are shared, but the rest of the configuration language differs by vendor, so read the variant file as well:

- ByNav: `bynav-config.md`
- SinoGNSS: `sinognss-config.md`

Unicore receivers also output NovAtel-format logs, but they have high-level configuration and their own command language; see `unicore-config.md`.

## Coverage: probe the manual's log table first

Every variant documents a table of logs, and receivers accept some that they never output, reject some that are documented, and output some that are not. Before planning captures, request every log in the table once and record the replies. Generate a message file with one `log <name>b once` line per log (the `A` form for ASCII-only logs), with a delay between lines so that each reply lands before the next request:

```
{ printf '[default.line]\neol = "\\r\\n"\ndelay = 1.2\n'
  for n in BESTPOS TIME RANGE ...; do printf '[[line]]\ntext = "log %sb once"\n' "$(echo $n | tr A-Z a-z)"; done
} > probe.toml
satpulsetool gps -d <device> -s <baud> -m probe.toml --packet-log probe.jsonl
```

Then sort each log into output, accepted but silent, and rejected, by walking the packet log and attributing packets to the most recent outgoing request. Probe the RTCM messages the same way (`log rtcm<n>b once`), including the ones `gps/lib/rtcmbin` decodes that the manual does not list: MSM1 to MSM7 for every system, and 1001 to 1013, 1033 and 1230. Some receivers output messages the manual omits.

The probe log is not test data; keep it with the working files.

## One format per log, or dual

A NovAtel-format log has ASCII (`A`), binary (`B`) and abbreviated ASCII (no suffix) forms. Whether a port can output the same log in two forms at once differs by receiver. Request a log in binary and then in ASCII, and check with `LOG LOGLISTA ONCE`:

- If both remain, make `-dual` captures with the ASCII and binary forms together (as for ByNav and Unicore).
- If only the second remains, the second request replaced the first. Put each form in its own capture instead (`nov-bin.jsonl`, `nov-ascii.jsonl`, `abbrev.jsonl`), and never request a log twice in one capture.

Different logs can always use different forms in the same capture.

## Triggers

- Ephemeris, raw ephemeris and almanac logs with `ONTIME N` give one satellite per period, cycling through the satellites (seen on ByNav and SinoGNSS; SinoGNSS also dumps every satellite when the log is requested). Use `ONTIME 1`, so that a 30 s capture covers every tracked satellite; `ONTIME 10` misses most of them.
- IONUTC with `ONCHANGED` is output once when requested and then only when the parameters change, so a passive capture that starts after configuration never contains it. Use `ONTIME 1` in captures.
- Whatever a `ONCE` request outputs is sent while the configuring `gps -m` session is still running, before a following passive `serial` capture opens the port. When that output belongs in the capture, run the configuration with `--packet-log <file> --capture <seconds>` instead.

## Replay and HW.toml

`vendor` in HW.toml selects the variant of the NovAtel protocol that `verify-replay.py` uses (port encoding, position type values and vendor logs). The captures are verified with the variant the daemon would use, so a wrong or missing vendor gives wrong results.

`--show-receiver` does not identify these receivers. Take the firmware string for HW.toml from the vendor's version query (see the variant file).
