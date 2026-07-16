# Native NMEA message model and ConfigSupportNativeNMEA (#370)

## Motivation

`gpsprot.ConfigOptions` controls message output on independent axes:
NMEAMsg (standard sentences RMC/GGA/GSA/GSV/VTG/GLL/ZDA), PVTMsg and
SatsMsg (abstract position/velocity/time and satellite information),
RTCMMsg, RawMsg. The axes are treated as independent because on every
receiver so far the native carrier of PVT/satellite data is a separate
binary protocol (u-blox UBX, Septentrio SBF): disabling NMEA and
enabling native PVT are orthogonal, on different wires. satpulsed
relies on this - setMsgOptions always sets NMEAMsg=None (NMEA is slow
on some receivers) plus PVTMsg for the native timing messages.

The Quectel LG290P (PQTM) is the first receiver whose native messages
ARE NMEA. PQTMPVT/PQTMEPE/... are NMEA sentences, and the only
satellite carrier is the standard GSV sentence itself. They share the
one NMEA output protocol (PQTMCFGPROT OutputProt NMEA bit): clearing
that bit silences PQTM and GSV along with the standard sentences
(hardware-verified, quectel-config stage-0 Q7). The independence
assumption fails two ways:

- NMEAMsg=None (disable standard NMEA) and PVTMsg=<timing> collide. The
  daemon's standard request would clear the NMEA protocol and starve
  the PVT messages it needs - a real bug, currently masked only because
  no daemon run against a native-NMEA receiver has happened.
- NMEAMsg naming GSV and SatsMsg both address the same physical GSV
  sentence.

This plan defines the model that keeps ConfigOptions well-defined when
that assumption fails, and the ConfigSupportNativeNMEA flag that marks
the receivers where it does.

## The two-level model

Message output is controlled at one of two levels:

- Message level: NMEAMsg names explicit standard sentences. Direct
  per-sentence control.
- Semantic level: PVTMsg/SatsMsg request abstract information; the
  backend maps it to whatever messages carry it (native PQTM, the GSV
  sentence, or binary on other receivers).

You operate at one level, not both. On a native-NMEA receiver the two
levels can resolve to the same physical message (GSV) or the same
shared wire, so mixing them is contradictory - a configuration error.

`nmea-out none` is the neutral value belonging to neither level
exclusively: it names no sentence, so it never creates a mix, and it is
the one NMEAMsg value allowed alongside the semantic level. There it
means native - emit only the native carriers the semantic axes request,
and none of the standard sentences.

## Meaning of nmea-out none

One rule, receiver-independent: `nmea-out none` = emit no NMEA except
the native carriers the semantic axes request. On u-blox those carriers
are binary, so it reduces to "no NMEA", exactly as today. On a
native-NMEA receiver it keeps the requested PQTM/GSV up while the
standard sentences go off. With no semantic axis requesting a carrier
it is plain "no NMEA". The receiver never enters into what none means;
the ConfigOptions do.

## ConfigSupportNativeNMEA flag

A new descriptor flag in gpsprot.ConfigSupportFlags: the receiver's
native PVT/satellite messages are transmitted as NMEA and share the
NMEA output protocol. Like the OnlyWithReset qualifiers it is a
descriptor, not a capability a receiver can lack, so it is excluded
from ConfigSupportFull and its bit grows down from the top; append it
to configSupportFlagNames, never reordering existing bits (the
casic-config lesson). Quectel declares it; no other current backend
does.

The flag exists for the frontends. The backend does not need it - it
knows its own carriers - so the Quectel backend realization (see
plan/quectel-config.md) can and should land first and independently.

## Frontend enforcement

Mixing message-level and semantic-level control is an error only on a
native-NMEA receiver; elsewhere the axes address different messages and
combine harmlessly, as they do today.

- satpulsetool: after the probe identifies the receiver, if
  ConfigSupportNativeNMEA is set and the request names explicit
  nmea-out sentences together with pvt-out/sats-out, fail with a clear
  message (control NMEA output at the message level or the semantic
  level, not both; use --nmea-out none to keep the native messages).
  This sits alongside the existing configSupport-driven option
  validation. It is where the former GSV union case becomes an error.
- satpulsed: setMsgOptions emits NMEAMsg=None plus PVTMsg/SatsMsg - the
  semantic level, with none as the native bridge - so it never mixes
  and needs no change beyond what the backend already does.
- Workbench UI: ConfigSupportNativeNMEA reaches the UI on the
  config-support-gating channel (ReceiverEvent.ConfigSupport; see
  webui/packages/workbench/plan/issues.md, config-support-gating). The
  NMEA per-sentence controls and the PVT/Sats groups become mutually
  exclusive by disabling, so the contradictory combination cannot be
  constructed and no error is ever surfaced - the interactive form of
  the CLI's error.

## Consequences

- The union / enable-wins behavior is removed. Today the Quectel
  backend lets SatsMsg override NMEAMsg for the shared GSV sentence
  (TestConfiguratorSatsMsgUnion and a sibling). Under this model that
  same request is a mixing error, so the union path and its tests go
  away, replaced by mix-does-nothing tests in the backend and the
  error in the frontend.
- No struct change to ConfigOptions: this is a semantics refinement
  plus one descriptor flag.

## gpshwtest consequences

gpshwtest (capability characterization plus a leave-as-found restore)
exercises message output group by group; the model and the flag reach
it in several ways.

- Single-level probing, now an invariant. The message-output probes
  already issue each request at one level: probe_nmea uses --nmea-out
  with explicit sentences (message level); probe_pvt and probe_sats use
  --binary (NMEAMsg=None) with --pvt-out/--sats-out (semantic level,
  none as the native bridge). None mixes, so they run unchanged - but
  this becomes a documented requirement, and any probe that combines an
  explicit --nmea-out with --pvt-out/--sats-out on a native-NMEA
  receiver would hit the new frontend error and must be avoided.

- Restoring a mixed as-found state. A native-NMEA receiver can be found
  emitting standard sentences and native PQTM messages at once (the
  LG290P default table carries GGA/GLL/GSA/GSV/RMC/VTG plus PQTMTXT).
  The two-level interface cannot express that combination in one
  request, so restore_protocol - which rebuilds the sentence set with
  --nmea/--nmea-out - cannot reconstruct the PQTM side, and the
  --nmea-out none probe case clears it. Options, to decide during
  implementation: restore through the receiver's low-level message file
  (the framework already drives one for speed restore), have
  setup/<receiver>.sh establish a non-mixed starting state so the
  leave-as-found check has a reproducible single-level target, or
  characterize the mixed-state restore as a documented receiver
  limitation. Whether the current LG290P setup (factory defaults +
  pulse mode) leaves a mixed table needs checking.

- Flag in characterization. ConfigSupportNativeNMEA is recorded like
  the other support flags, and the LG290P baseline
  (baselines/LG290P03-R02A01S.json) is regenerated to include it.
  gpshwtest may gate native-NMEA-specific expectations on it, as it
  already gates probe_signals/probe_modes on the OnlyWithReset
  qualifiers.

- Optional characterization: probe that a message-level --nmea-out set
  combined with a semantic axis returns the frontend error, as data on
  the native-NMEA behavior.

- SEMANTICS.md documents the two-level model so probe results are
  interpreted correctly.

## Phasing

1. Quectel backend realization (plan/quectel-config.md): the two-level
   ProtNMEA algorithm and do-nothing-on-mix. No flag needed; fixes the
   daemon starvation bug. Lands first.
2. ConfigSupportNativeNMEA flag in gpsprot; Quectel declares it.
3. satpulsetool mix error, gated on the flag.
4. Workbench UI mutual-exclusion gating, folded into
   config-support-gating.
5. gpshwtest: keep the probes single-level, adapt the mixed-state
   restore, record the flag in the baseline, document the model in
   SEMANTICS.md.
