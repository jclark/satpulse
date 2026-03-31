# Captures using high-level configuration

This applies to receivers that support high-level configuration via satpulsetool (currently u-blox and Unicore). These receivers support `--pvt-out`, `--sats-out`, `--binary`, `--nmea-out`, `--survey`, `--time-gnss`, `--gnss`, `--speed`, `--reload`, and `--show-receiver`.

## Getting receiver info

Run `satpulsetool gps -d <device> -s <baud> --show-receiver` to get the receiver model, firmware, and protocol version. Use this information for the HW.toml file. The firmware field in HW.toml should match the full firmware string from `--show-receiver` (e.g., "HPG 1.51 PROTVER 27.50").

## Reload between captures

`--reload` restores the receiver to its NVM-saved state. This is essential between captures to prevent configuration leakage. After `--reload` on USB, the device may briefly disappear -- add `sleep 2` before the next command.

## High-level config captures

Use `--binary` to switch to binary output protocol (disables NMEA). Use `--pvt-out <flags>,off` to enable specific messages and disable unneeded ones. `--binary` cannot be combined with `--nmea-out` in the same invocation.

### Key flags

- `--pvt-out daemon` -- the set satpulsed uses (includes tp, after, tai, leap, survey, qual, epoch, off)
- `--pvt-out daemon,pos` -- daemon + position (satpulsed enables this when track log or HTTP is configured)
- `--sats-out sat,sig` -- satellite and signal info (bandwidth-sensitive, may need higher baud)
- `--pvt-out tp,after,tai,off` -- minimal time-only set
- `--pvt-out pos,ecef,time,epoch,off` -- ECEF position + time
- `--survey --survey-time 60 --survey-acc 50` -- short survey for testing (generous accuracy so it stays in progress during capture)

### Baud rate considerations

At low baud rates (e.g., 9600), satellite messages may not fit. The daemon set without satellites is safe at 9600. Satellite messages typically need 38400+.

To temporarily change baud: `satpulsetool gps -d <device> -s <baud> --speed 9600` (RAM only, no `--save`). To restore: `--reload`.

### Combining with low-level message files

`-m` (message file) cannot be combined with high-level config in a single invocation. But you can run high-level config first (without `--capture`), then a second invocation with `-m` to add extra messages. The `-m` invocation does not probe or reset the receiver.

This two-step approach is used for per-constellation captures: high-level config sets `--time-gnss` and `--gnss`, then message file tags enable additional time messages.

## Cross-protocol satellite capture

Capture NMEA GSV/GSA alongside native satellite messages for cross-protocol validation. This does not use `--binary`, so both NMEA and native satellite messages appear in the same capture.

```
--pvt-out tp,after,tai,epoch,off --sats-out sat,sig --nmea-out RMC,GSA,GSV
```

## Cold start

To save configuration to NVM: `satpulsetool gps -d <device> -s <baud> --save`. To cold start: `satpulsetool gps -d <device> -s <baud> --reset`. To factory reset afterwards: `satpulsetool gps -d <device> -s <baud> --factory-reset`. On USB, add `sleep 2` after reset or factory reset as the device may briefly disconnect.

## Per-constellation time captures

If the receiver supports `--time-gnss`, capture one trace per constellation to test TimTP with different GNSS references. Each capture:

1. Reload
2. High-level config: `--binary --pvt-out tp,after,tai,leap,epoch,off --time-gnss <gnss> --gnss gps,<gnss>`
3. Message file: enable additional time message variants via tags
4. Capture

For GPS-only: `--time-gnss gps --gnss gps` (no secondary constellation needed).
