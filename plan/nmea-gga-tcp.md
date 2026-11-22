# Synthesized GGA in an NMEA TCP service (#329)

## Motivation

IANA registers [`nmea-0183`](https://www.iana.org/assignments/service-names-port-numbers/service-names-port-numbers.xhtml?search=10110)
on TCP and UDP port 10110 for "NMEA-0183 Navigational Data". SatPulse already
lets a TCP proxy expose only receiver NMEA packets, but that service has no
position sentence when the receiver reports its position only through a binary
protocol.

Add an option for an NMEA TCP proxy to fill that gap with the protocol-neutral
GGA produced by `gps/nmeasyn`. This makes the proxy useful as a network
geolocation service regardless of the receiver's native output configuration.

The GGA synthesizer and selected-GGA feed are already implemented and recorded
in [archive/nmea-gga.md](archive/nmea-gga.md). This plan uses their existing
epoch-complete output. RMC and prompt GGA/RMC output are independent,
interrelated work in [nmea-rmc-synth.md](nmea-rmc-synth.md) (#429).

IANA registry entry:

- service name: `nmea-0183`
- port: 10110
- transports: TCP and UDP
- description: NMEA-0183 Navigational Data

This plan covers the TCP service only. It does not add UDP output or synthesis
to Unix socket proxies.

## Configuration

Add a `synth` option to `[[proxy.tcp]]`. It is valid only with
`protocol = "NMEA"`.

```toml
[[proxy.tcp]]
listen = ":10110"
protocol = "NMEA"
synth = true
```

The registered port is conventional, not a new default: `listen` remains
explicit, as it is for every TCP proxy.

Validation rules:

- `synth = true` requires `protocol = "NMEA"`.
- `synth = true` is rejected for other protocol-specific TCP proxies.
- The option affects only packets sent to clients of that proxy service.
- Other TCP and Unix socket proxy services continue to use the raw receiver
  packet stream.

## Output behavior

The synthesized service is still an NMEA stream, not a GGA-only stream:

- original non-GGA NMEA packets pass through immediately;
- original GGA packets pass through immediately;
- an epoch-complete synthesized GGA is inserted when the receiver has not
  already supplied GGA for the same UTC second;
- invalid receiver GGA packets do not suppress synthesized output.

The existing `GGASelector` defines receiver-versus-synthesized precedence and
same-UTC suppression. This plan does not change its latest-value output used by
Ntrip client-GGA upload.

RMC is not part of this plan. Prompt GGA output added by #429 can enhance the
service later without being required for the epoch-complete GGA service here.

## Architecture

The daemon keeps the raw receiver packet broadcast fed by `startScan`. If any
TCP NMEA proxy has `synth = true`, it also creates a selected NMEA packet stream
and a broadcast for that stream.

`gpsevent.Dispatcher` receives an optional selected-NMEA sink:

- successfully processed original NMEA packets go to the selected stream;
- original and synthesized GGA candidates pass through the existing
  `GGASelector` so receiver GGA wins for the same UTC;
- the selected synthesized GGA is inserted into the otherwise unchanged NMEA
  stream.

`proxy.Start` receives both broadcasts. A TCP service with
`protocol = "NMEA"` and `synth = true` subscribes to the selected NMEA
broadcast; every other proxy subscribes to the raw broadcast.

The selected NMEA path exists only when configured. A daemon without a
synthesized NMEA proxy does not instantiate the extra broadcast or synthesis
wiring unless another existing consumer, such as Ntrip client-GGA upload,
already needs the GGA synthesizer.

## Testing

- Configuration accepts `synth = true` for a TCP NMEA proxy.
- Configuration rejects it for TCP proxies using any other protocol.
- Original NMEA packets pass through the selected stream unchanged.
- Receiver GGA suppresses synthesized GGA for the same UTC.
- A synthesized GGA is inserted when no matching receiver GGA exists.
- Other proxy services remain on the raw packet stream.
- Add a hardware-free `smoketest/scenarios/proxy/tcp-synth` scenario that
  connects to a live TCP proxy and observes original NMEA plus synthesized GGA.
