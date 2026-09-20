# Pollsim configurations

Configuration files for `satpulsetool pollsim`, which runs the serial PPS
polling loop of `satpulsed` against a simulated pulse and host in virtual
time. It is for judging how the loop's tracking behaves under host stalls,
timer overshoot, idle slowdowns and pulse outages without waiting for them
to happen on hardware.

| File | Scenario |
|------|----------|
| [mac-ft232r.pollsim.toml](mac-ft232r.pollsim.toml) | Mac mini with an FT232R USB UART: 200 us queries, exact timers, idle slowdown, `preWarm` |
| [linux-uart.pollsim.toml](linux-uart.pollsim.toml) | Native UART on Linux: 10 us queries, millisecond timer truncation |
| [outage-20260920.pollsim.toml](outage-20260920.pollsim.toml) | The Mac with a burst of host stalls, as on 2026-09-20 |

Run one with

```
satpulsetool -v pollsim outage-20260920.pollsim.toml
```

`-v` shows the loop's own track status lines with simulated timestamps.
The statistics printed at the end say how many edges the time daemon would
have received, how wrong they were against the true edge, the longest gap
between them, and how many pulses were missed or rejected. `--edge-log`
writes every candidate with its true error as JSON Lines.

`satpulsetool pollsim -C` prints the default configuration with a comment
on every key.
