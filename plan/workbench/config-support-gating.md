# Gate Config tab controls by ConfigSupportFlags

## Goal

`gpscfg.Result` includes `ConfigSupport` (a `gpsprot.ConfigSupportFlags`
bitset from #203) describing which configuration items the probed
receiver's `Configurator` actually supports. The workbench Config tab
currently shows every control enabled unconditionally and leaves the
user to discover via apply failures or warnings that a setting is
ignored. Gate the controls so unsupported items are visibly disabled.

## Design principle: grey out, never hide

Unsupported controls are disabled in place, never hidden. The layout is
identical for every receiver; the receiver's capabilities are expressed
entirely as disabled state. Reasons:

- Every control has one location regardless of receiver model, so
  screenshots and e2e selectors do not fork per receiver.
- A greyed section is informative (this receiver has no raw output); a
  hidden one conveys nothing and can read as a bug when switching
  receivers.
- Wiring is simpler: every interior control already takes `disabled`,
  and `ConfigSubGroup` already has a `disabled` prop, so gating is one
  more boolean ANDed into existing disabled expressions. Hiding would
  need conditional JSX blocks.

Greying introduces a third reason a control can be grey, alongside "not
connected" and contextual state (e.g. survey fields grey when the mode
radio selects something else). Disambiguation is handled per control
kind:

- **Subgroups that are never otherwise greyed** (RTCM, Raw): pass
  `disabled` to the `ConfigSubGroup`, which renders the title in
  `text-text-muted`. Since these titles are never greyed for any other
  reason (not even when disconnected), a muted title is an unambiguous
  "not supported" cue. All children disable with it.
- **Text input fields** (survey accuracy, survey time, position
  accuracy): disable with `placeholder="not supported"` replacing the
  example-value placeholder. A placeholder only shows while the value
  is empty, and readback never populates an unsupported property
  (`ConfigProps` omits properties never read back), so the text stays
  visible.
- **Radios and checkboxes** (MSM4/MSM7 options, Reload reset option,
  "Report survey progress", time-mode radios): plain greying, no
  further annotation.

## Wiring

The backend produces the value at probe time but does not forward it.
`packetWorker` in `gps/app/session/session.go` builds `ReceiverEvent`
from `rslt.ReceiverInfo` and `rslt.PacketFormatsDetected` and emits it
as `gps:receiver`; `ReceiverEvent` needs a `ConfigSupport` field
populated from `rslt.ConfigSupport`. The Go type marshals as a JSON
string array via its `MarshalJSON`, so the natural frontend form is a
`Set<string>` of item names, threaded from `App` (which already tracks
the `gps:receiver` payload) into `ConfigPanel` as a prop alongside
`signalCatalog`.

The frontend keys on the JSON names, not bit positions, and ignores
unknown names. This matters because pending branches renumber bits
against each other (casic-config inserts `reload` before `port`;
septentrio-config inserts `surveyDur` mid-sequence; quectel-config
grows qualifier flags down from the top bit). The names are the stable
wire contract, so gating for not-yet-merged flags can be written now
and lights up as each branch merges.

The Config tab is only reachable after a successful probe, so
`ConfigSupport` is always populated when the panel renders -- no
zero/unknown fallback state is needed.

The apply side needs no guarding: disabled controls cannot be touched,
so unsupported props and opts are never sent.

## Gating by flag

- `speed` -- Serial speed group: grey the whole group. `ConfigGroup`
  has no `disabled` prop yet; either add the same muted-title
  treatment or grey only the interior and leave the header alone
  (open detail). The existing `baudRateApplicable` tri-state (driven
  by `ConfigProps.baudRate === 0`, i.e. is this port a UART) is
  data-driven and stays as-is, layered behind the capability gate:
  capability says "this protocol can ever configure baud", port info
  says "this specific port is UART". Septentrio is the first real
  receiver without `speed` (its live baud switch on the connected
  port cannot be verified, so it is declared unsupported).
- `survey` -- disable the "Survey-in" time-mode radio and pass
  `disabled` to the Survey-in subgroup.
- `surveyAcc` -- survey accuracy field: "not supported" placeholder.
- `surveyDur` (septentrio-config / config-support-survey-dur) --
  survey time field: "not supported" placeholder. On a Septentrio
  both accuracy and duration are absent (its survey takes no
  arguments), leaving the mode radio, "Do a new survey", and the
  progress checkbox.
- `surveyMsg` -- "Report survey progress" checkbox: grey.
- `fixedPos` -- disable the "Fixed position" time-mode radio and pass
  `disabled` to the Fixed position subgroup.
- `fixedPosAcc` -- position accuracy field: "not supported"
  placeholder.
- `raw` -- Raw subgroup: `disabled` (muted title).
- `rtcmMSM4` / `rtcmMSM7` -- when both are absent
  (`ConfigSupportRTCMMSM == 0`), the receiver has no RTCM output
  capability at all in every current backend: pass `disabled` to the
  whole RTCM subgroup (muted title). When exactly one is supported,
  grey the other radio. "Allow MSM fallback" (`RTCMMsgLax`) stays
  ungated: it means "do the best we can" and also covers per-GNSS
  gaps (e.g. Gen 8 HPG lacking Galileo MSM), so it is meaningful
  with a single MSM version. ARP stays ungated -- no flag corresponds
  to it (`rtcmBaseID` gates DF003 configuration, not ARP output).
- `reload` (casic-config) -- "Reload" reset radio in Persistent
  operations: grey. Save and the other reset types stay ungated.
  CASIC V5 declares it while V6 does not, so both states occur.
- `signal` -- **not gated**. The flag means "supports
  finer-than-constellation selection", not "can set signals":
  constellation-level enable/disable works on every configurator
  (u-blox applies signal sets regardless of `bandsConfigSupport`,
  via VALSET keys or gen-8 CFG-GNSS; CASIC V5 collapses the request
  to a constellation set and writes CFG-NAVX `NavSystem`). On a
  receiver without the flag, a fine-grained picker selection degrades
  to constellation granularity; the verify readback reports the
  achieved set. The constellation checkboxes and "Edit signals..."
  stay enabled; see the next section for what does gate them.
- `port`, `rtcmBaseID`, `rtcmQZSS` -- no UI counterpart; ignored
  (deliberately, not an omission). `port`'s readback effect (whether
  `baudRate` comes back at all) is already absorbed by the
  `baudRateApplicable` null state.

## Constellation checkboxes: gate by SupportedGNSS

The per-constellation checkboxes are gated by
`ReceiverInfo.SupportedGNSS`, not by `ConfigSupport`. Today the
frontend fetches the catalog filtered to `supportedGNSS`
(`getAllSignals(gnss)` -> `Session.SignalCatalog`), so unsupported
constellations are *hidden* -- different receivers render different
checkbox rows, contrary to the grey-out principle. Change to:

- Always fetch the full catalog (the endpoint already returns all
  constellations when passed an empty set).
- Render the complete constellation row -- identical for every
  receiver, since the registry is device-independent.
- Grey checkboxes for constellations not in `supportedGNSS`, and
  likewise grey those constellations' signals inside the picker.
- A zero/empty `supportedGNSS` means "constellation set unknown":
  leave all checkboxes enabled. This is a permanent, legitimate state
  (unc intentionally does not populate `SupportedGNSS`), not a
  transitional gap.

## Preset buttons

The Minimum and Daemon presets set change-flags wholesale (Minimum
sets `rtcmChange`/`rawChange` unconditionally). On a receiver whose
RTCM or Raw section is disabled this would queue changes to controls
the user cannot interact with and provoke apply warnings. The presets
skip sections whose support flag is absent.

## Open decision: reset qualifiers

quectel-config adds qualifier flags (`signalOnlyWithReset`,
`modeOnlyWithReset`, `rtcmMSMOnlyWithReset`) that qualify a capability
rather than granting one: the change is stored-only and takes effect
only after save + reload reset. Quectel declares all three. Nothing
should be greyed -- the capability exists. How to surface "takes
effect after save and reload" is undecided; candidates:

- A small `text-info` note in the affected section when the qualifier
  is present.
- Extending the pending-status line in the bottom bar when a qualified
  section is touched and Save is Nothing or Reset is None.

Auto-selecting Save=Changes + Reset=Reload is rejected: it changes NVM
and restarts the receiver as a side effect of an innocuous-looking
edit.

## Receiver test matrix

Support profiles that exercise the gating (from the backends on master
and the pending config branches):

- **u-blox** varies by model: gen 8 and gen 9 outside HPG/TIM/SPGL1L5
  lack `signal` (ungated anyway); non-HPG/TIM receivers have no RTCM
  flags; `raw`, survey flags, and `rtcmBaseID` vary by firmware.
- **unc (UM980)**: full minus `surveyAcc`/`surveyMsg`/`fixedPosAcc` --
  the three placeholder/greyed singles.
- **Septentrio**: full minus `speed`/`surveyAcc`/`surveyDur`/
  `fixedPosAcc` -- first receiver with the Serial speed group greyed.
- **Allystar TAU1201**: no RTCM at all (every RTCM CFG-MSG target
  NAKs) -- RTCM subgroup muted.
- **CASIC V5**: the stress test -- no `signal`, `raw`, `surveyMsg`,
  MSM, or `port`; uniquely *has* `reload` while V6 does not.
- **Quectel**: full minus `raw`/`fixedPosAcc`, with all three reset
  qualifiers.
