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

The Config tab can only be entered after a successful probe, but it
stays visible if the receiver then disconnects or re-probes, and in
those states the frontend has no `ConfigSupport`. An empty set
therefore means "unknown", and every item is treated as supported:
`!connected` already disables all controls, and greying titles or
showing "not supported" placeholders would falsely claim the receiver
lacks the capability.

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
- `reload` -- "Reload" reset radio in Persistent operations: grey.
  Save and the other reset types stay ungated. The flag and the
  gating flip both land with casic-config (the branch that
  introduces the flag also delivers the UI behaviour, so V6
  receivers never show an enabled radio that does nothing). Every
  u-blox declares it (it sits in the universal base, outside
  `ubxVariable`), as does CASIC V5; CASIC V6 does not, so both
  states occur.
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

Inside the picker, individual signals are additionally greyed by a
`signalsSupported` field on `ReceiverEvent`: the signals that exist
on the receiver, in catalog form, with absence meaning "no known
restriction". Backends do not yet report supported signals (the
eventual fix is a read-only supported-signals property on
`ConfigProps`, populated by Configurators -- ubx already computes the
set internally from key presence), so the session fakes the field: a
Configurator without `signal` support cannot select signals finer
than a whole constellation, and every backend that lacks it drives an
L1-only receiver, so when the probed flags lack `signal` the field is
the registry's L1-band catalog (e.g. a u-blox M9N greys GPS L5). The
constellation toggles skip greyed signals so they can never enter the
selected set.

## Preset buttons

The Minimum and Daemon presets set change-flags wholesale (Minimum
sets `rtcmChange`/`rawChange` unconditionally). On a receiver whose
RTCM or Raw section is disabled this would queue changes to controls
the user cannot interact with and provoke apply warnings. The presets
skip sections whose support flag is absent.

## Deferred until dependencies merge

One flag (`surveyDur`) is contributed only by a pending backend branch
and is absent from every backend on master. Once the branch lands, its
supporting receivers -- including u-blox -- declare the flag, so the
flag is present exactly when the control should be enabled. Gating on
the flag's absence *now* would grey a control that master's u-blox
(and the F9P simulator the e2e tests drive) actually supports, breaking
those tests. So the frontend gating is written but left inert until the
branch merges, as a one-line flip in `config-panel.tsx`:

- **`surveyDur`** (config-support-survey-dur): the Survey time field is
  currently gated only by survey support and the mode radio
  (`surveyTimeDisabled = surveyDisabled`). When the branch merges, change
  it to `surveyDisabled || !has(SUP.surveyDur)` and restore the "not
  supported" placeholder (`placeholder={has(SUP.surveyDur) ? '2000' :
  'not supported'}`). After merge, u-blox `tmode > 0` receivers declare
  `surveyDur`; Septentrio (survey takes no arguments) does not, so its
  field greys.

The `SUP` table in `config-panel.tsx` already carries the name, so no
other wiring is needed -- only the gating expression changes. `reload`
was originally deferred the same way, but its flip moved onto
casic-config itself: there the deferral rationale does not apply, since
that branch's u-blox already declares the flag, so its e2e tests keep
the radio enabled.

One deferred item is new frontend work rather than a gating flip:

- **Reset qualifiers** (quectel-config): `signalOnlyWithReset` and
  `modeOnlyWithReset` qualify a capability rather than granting one.
  On a receiver declaring them, the Configurator skips signal and
  positioning-mode changes entirely unless the target both saves and
  performs a reset that reloads (Save is not Nothing, Reset is Reload
  or Cold start -- the `savesAndResets` gate), so an Apply without
  them would be a silent no-op. The form must never represent such an
  impossible request. Touching a qualified section auto-selects
  Save=Changes and Reset=Reload, never downgrading a stronger existing
  choice, with a message saying what was selected and why. Conversely,
  undoing the save or reset selection while a qualified change is
  pending reverts the qualified sections to their readback values and
  clears their touched flags (other pending changes survive); the
  message makes the discarded input -- possibly typed fixed-position
  coordinates -- unsurprising. `rtcmMSMOnlyWithReset` does not fit
  this mechanism: an MSM selection without save+reset is not an
  impossible request -- the messages are enabled either way, at the
  stored MSM type if that differs from the selection, and fully as
  asked if it matches. How the UI should handle the MSM case is not
  yet worked out.

The other pending flags need no deferral: they gate controls that are
already correct on master because master's receivers declare them when
supported (the frontend keys on the stable JSON names and ignores names
no receiver emits yet). Concretely, when these branches merge nothing in
the frontend needs to change -- the gating already written lights up:

- **allystar-config**: TAU1201 declares neither MSM flag, so the RTCM
  subgroup mutes via the existing `notSupported={!rtcmSupported}` path.
  One backend fix is needed on that branch after casic-config merges:
  its `ConfigSupport` builds flags additively, so it would silently
  omit `reload` even though the TAU1201 implements it (SIMPLERST
  mode 0), greying a working radio. The fix is restructuring it
  subtractively from `ConfigSupportFull` (minus `FixedPosAcc`,
  `RTCMBaseID`, `Port`, and conditionally the MSM/QZSS flags) so
  future universal flags are picked up automatically.
- **septentrio-config**: declares no `speed`/`surveyAcc`/`fixedPosAcc`,
  which the existing `speedSupported`/`has(SUP.surveyAcc)`/
  `has(SUP.fixedPosAcc)` gates already handle (`surveyDur` excepted, see
  above).
- **quectel-config**: the capability flags need nothing; the reset
  qualifiers get the auto-select/revert mechanism above when the
  branch lands.

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
  MSM, or `port`; *has* `reload` while V6 is the one profile
  without it.
- **Quectel**: full minus `raw`/`fixedPosAcc`, with all three reset
  qualifiers.
