# CASIC configurator hardware verification (#229)

Packet logs of one comprehensive `satpulsetool gps` configuration
invocation per attached CASIC receiver, captured 2026-06-12 with
`--packet-log`. Replay with `satpulsetool replay <file>`.

| Log | Receiver | Firmware | Invocation |
|-----|----------|----------|------------|
| `sweep-atgm332d-f8n.jsonl` | ATGM332D-AT9880-F8N-76 (dual band) | URANUS6 V6.3.2.0 | `--pps 0.1 --time-gnss GPS --nmea-out RMC,ZDA --pvt-out pos,time,tp --sats-out sat --gnss GPS,BDS` |
| `sweep-at632.jsonl` | AT362-AT6668-6T-30 (timing) | URANUS6 V6.3.0.0 | `--pps 0.1 --time-gnss GPS --nmea-out RMC --pvt-out time,tp --sats-out sat --gnss GPS,BDS` |
| `sweep-atgm332d-5n71.jsonl` | ATGM332D-5N71 | URANUS5 V5.3.0.0 | `--pps 0.1 --time-gnss GPS --nmea-out RMC --pvt-out time,off --sats-out none --gnss GPS,BDS,GLO` |

All three invocations exited 0 and reported achieved values from
verify readbacks. Every CFG request was acknowledged except:

- Both V6 units NAK enabling TIM-TP (0x02 0x00); the configurator's
  documented fallback enables TIM2-TPX (0x12 0x00) instead, which both
  ACK (visible in the logs as the final CFG-MSG exchange).
- The V5 NAKs the probe's MON-VER poll, which is itself the documented
  V5 detection signal; the two NAKs in its log are the probe NAKs.

The V5 invocation is deliberately modest: its 9600 line carries about
960 bytes/s, and requesting more output than that (e.g. NAV-PV plus
full NMEA) saturates the receiver's transmit queue to the point where
it splices packets mid-stream and acknowledgements are lost. The
configurator reports such failures honestly; the configuration itself
must fit the line budget.
