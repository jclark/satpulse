# Daemon trusted-time management

This is not yet a worked-out implementation plan. It records the problems,
constraints, and likely solution shapes for having `satpulsed` establish and
maintain receiver trusted time, primarily for OSNMA.

The core OSNMA plan (`plan/osnma-core.md`) gives a workable manual/tool path:
build a receiver-specific `TrustedTimePacketBuilder`, then use
`satpulsetool gps` or the daemon socket to set trusted time. This plan is about
the harder daemon-owned version.

## Framing

"Initial trusted time" in the daemon should mean "get the receiver into a
trusted-time state so it can obtain its first verified OSNMA fix." It should not
mean "do all trusted-time work before `Configure` returns" or "block daemon
startup until NTP succeeds."

The likely shape is:

1. Run `gpscfg.Configure` / probe and obtain a `TrustedTimePacketBuilder`.
2. Start an asynchronous trusted-time manager after the daemon has normal port
   locking and message dispatch available.
3. Let the manager try time sources until it can set trusted time.
4. Once the receiver has OSNMA-verified fixes, decide whether receiver feedback
   can maintain trusted time without continuous network access.

This model matters for SNTP. Network queries may be slow, unavailable, or need
retry/backoff. Passing a channel into `Configure` or blocking before configure
would make the startup path more fragile than necessary.

## Configuration Shape

The likely user-facing shape is a receiver-operation flag in `[gps]` plus a
separate section describing how trusted time is obtained and checked:

```toml
[gps]
config = true
nma = "osnma"
setTrustedTime = true

[trustedTime]
# source selection, uncertainty policy, cross-checks, and refresh policy
```

`gps.config` remains about persistent receiver configuration. `gps.setTrustedTime`
is a separate operation: set the receiver's internal trusted-time clock. It
should be meaningful even when `gps.config = false`.

Open questions:

- Defaulting: should unset `gps.setTrustedTime` default from `gps.nma ==
  "osnma"`, from `gps.config && gps.nma == "osnma"`, or remain false unless
  explicitly requested? The persistent-receiver-config case argues against
  tying it too tightly to `gps.config`.
- Scope: should `[trustedTime]` be daemon-only, or should `satpulsetool gps`
  eventually be able to read the same section? The tool currently has no normal
  dependency on `satpulse.toml`.
- Naming: `[trustedTime]` should not be called assistance. TTFF assistance
  might later need a separate `[assist]` / `[assist.pull]` shape for
  time/position/ephemeris/almanac/vendor streams.

Possible `[trustedTime]` fields, not finalized:

```toml
[trustedTime]
source = "auto"              # auto | system | sntp | receiver
uncertainty = "10s"          # fallback only when source has no max error
refreshInterval = "30m"      # policy TBD
maxAge = "2h"                # policy TBD

[[trustedTime.sntp]]
server = "time.example.net"
timeout = "2s"

[trustedTime.httpsCheck]
url = "https://www.example.com/"
maxDifference = "5s"
```

These names are placeholders. The important split is that `[gps]` says whether
to perform the receiver write, while `[trustedTime]` says how to produce and
validate the time estimate.

## Time Estimate and Time Bases

The daemon should use the `gpsprot.TimeEstimate` shape from
`plan/osnma-core.md`, with optional UTC and optional TAI:

```go
type TimeEstimate struct {
    UTC time.Time
    TAI ptime.Time
    TimeOfEstimate time.Time
    Accuracy time.Duration
}
```

Unlike `satpulsetool gps`, the daemon already has a leap-second model:
`cfg.LeapSecond.leapSecond()` seeds the runtime state, and `LeapSecondMsg`
received during configuration or normal operation updates it. Trusted-time
management should plug into that existing state when it needs to convert a UTC
estimate into TAI. The open questions are policy questions about source choice,
validation, and refresh timing, not basic UTC-to-TAI conversion inside the
daemon.

## Candidate Time Sources

### System Clock / Kernel NTP

The simplest source is the local system clock, using `gps/lib/ntptime.Get`.

Useful properties:

- UTC estimate and monotonic anchor are cheap and local.
- `MaxError` gives a conservative accuracy when available.

Problems:

- The system may not be synchronized.
- A local NTP source may itself be GNSS-disciplined and geographically near the
  receiver, so it is not independent of local spoofing/jamming.

### Remote SNTP

The existing `time/lib/sntp` client can query a configured server and produce a
UTC estimate, monotonic anchor, and accuracy bound.

Useful properties:

- A distant server gives some independence from local GNSS conditions.
- Querying can run asynchronously after configure and retry until it succeeds.

Problems:

- Plain SNTP is unauthenticated.
- It gives UTC, not TAI; leap-second offset must come from another source.
- DNS/network failures must not block normal daemon startup.

Possible stages:

1. Single configured SNTP server.
2. Multiple servers with an agreement policy, rejecting outliers.
3. NTS or another authenticated time source. This may not be worth the
   dependency/protocol complexity if HTTPS cross-checks are good enough.

### Receiver Feedback

After OSNMA is working, the receiver may be able to feed trusted time back into
itself. This is attractive when network access is intermittent.

Possible daemon rule:

- Once NMA-verified fixes are observed and the PHC/sync controller is tracking,
  treat receiver GNSS time as a candidate trusted-time source.

Problems:

- This is a policy decision, not a cryptographic proof.
- It depends on knowing whether receiver trusted time is still valid and how
  accurate it is.
- For u-blox, `UBX-NAV-TIMETRUSTED` probably needs to be observed to make this
  robust.

## Receiver State and Observability

u-blox has native messages that are directly relevant:

- `UBX-NAV-TIMETRUSTED`: reports external trusted-time state, reference system,
  validity, propagated accuracy, and delta fields.
- `UBX-SEC-OSNMA`: reports OSNMA status in more detail than the
  `UBX-NAV-PVT.nmaFixStatus` bit.

SatPulse does not yet have a good framework for observing/logging
protocol-specific native messages without pretending they are
protocol-independent. That should probably be solved separately.

An intermediate approach is a u-blox-specific helper daemon that reads UBX from
a port exposed by existing tooling and writes trusted-time packets through the
daemon socket. This would let us learn from `NAV-TIMETRUSTED` / `SEC-OSNMA`
without committing the main daemon to the full policy.

## Refresh Policy

u-blox propagates trusted time on its TCXO with decaying accuracy. Eventually
the receiver may fall from the ADKD type 0 threshold (15 s) to the ADKD type 12
threshold (165 s), then lose OSNMA applicability altogether.

Open questions:

- Should refresh be time-based, state-based, or both?
- Can `NAV-TIMETRUSTED.propTAcc` directly drive refresh decisions?
- What should the daemon do when `deltaTimeValid` is false or delta grows?
- Should network sources always override receiver feedback, or only when
  receiver propagated accuracy is near a threshold?
- What hysteresis prevents excessive writes?

The cadence should not be "every NMA-verified epoch." Refresh should be rare
and tied to accuracy/state thresholds.

## HTTPS Cross-Check

An HTTPS `Date:` header can provide an authenticated, low-precision UTC
cross-check for any candidate source:

1. Make a HEAD request to a configured HTTPS URL.
2. Parse `Date:`.
3. Refuse to set trusted time if the candidate UTC time differs by more than a
   configured threshold.

This is much less precise than SNTP, but precision is not the point. It is an
integrity check against a local failure or spoofing scenario. A threshold around
5 s is plausibly useful: well below OSNMA's 15 s ADKD type 0 limit, but loose
enough for HTTP date granularity and RTT.

Open questions:

- Is this worth doing before multi-server SNTP?
- Should unreachable HTTPS fail closed when configured?
- Should multiple HTTPS endpoints be supported?

## Port Writes and Startup

The trusted-time manager must share the receiver output path with existing
writers:

- receiver configuration,
- proxy/socket writes,
- stream-pull assistance data,
- future sidecar/helper writes.

The manager should write only through the same `OutPortLock` discipline as the
other daemon writers. It should start after configure/probe has returned a
builder and after normal dispatcher infrastructure is ready enough to observe
receiver messages and shut down cleanly.

## Possible Phases

These are not a committed implementation order, but they seem like useful
experiments:

1. Daemon can start an async manager with a `TrustedTimePacketBuilder`, but the
   first source is only local system/kernel NTP.
2. Add SNTP retry loop using `time/lib/sntp`.
3. Add optional HTTPS cross-check.
4. Add u-blox `NAV-TIMETRUSTED` observation, probably through a native-message
   observer or helper daemon.
5. Use receiver feedback and `NAV-TIMETRUSTED` state to maintain trusted time
   without requiring continuous network access.

The core OSNMA work should not wait for this plan to be complete. The manual
`satpulsetool gps --socket` path is the practical bridge.
