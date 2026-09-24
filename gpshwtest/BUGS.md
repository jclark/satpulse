# Bugs found by hardware testing

Clear satpulsetool bugs found while running gpshwtest against real receivers, with the evidence. Receiver limitations belong in `HW/`, and the intended semantics in `SEMANTICS.md`.

## val-layer signal path ignores the receiver's concurrent major GNSS limit

The non-legacy (CFG-VALSET) signal path does not apply the protocol-reported limit on concurrent major GNSS (the `simultaneous` field of UBX-MON-GNSS), so a request enabling more majors than the receiver supports is sent as-is and fails on a receiver NAK. The legacy CFG-GNSS path handles this: it polls MON-GNSS, drops GLONASS if the request is one over the limit, and otherwise refuses with a clear error. On the EVK-M101 (SPG 5.10, PROTVER 34.10), whose limit is 3, the all-constellations request builds a VALSET enabling four majors and the receiver NAKs it with `GPTXT inv sig cfg` / `bad cfg RAM`, configuration unchanged; the legacy heuristic would have landed exactly on the limit and succeeded. Under the enabled-signals semantics the limit is protocol-reported and so belongs in the fixup intersection. Tracked as issue #313. Evidence: run artifacts `runs/20260611-130850-ttyUSB1/105-set-signals-GPS-GAL-BDS-GLO-QZSS-SBAS.jsonl` (the four-major VALSET, the NAK, and the GPTXT messages).

## unresolved observations

- Setting the time pulse width (--pps) realizes the whole pulse bundle including time grid = GPS. On a receiver whose as-found grid is UTC (factory state of the F9P and M10), the first probing run cannot restore the as-found running configuration - absence of timeGNSS is unrepresentable - and reports itself not-left-as-found once. Preserving the existing grid when only the width is requested would avoid the side effect.
- The same --pps bundle forces polarity to rising, and no flag can express falling polarity, so a receiver whose as-found polarity is falling (the TAU951M-P200 factory state) can never be restored through the CLI: every probing run on such a unit reports not-left-as-found on `timePulse.polarityRising` (restored out-of-band after runs). Either a polarity flag or a width-only --pps would remove the gap.
- Backends disagree on a request that is invalid under the tool's own semantics: a denoted set with no non-augmentation signal (e.g. `--gnss GLO --band E5b` on a receiver with no GLO E5b-band signal) is refused by the ubx backend ("no suitable supported GNSS signal was enabled") but accepted by the unc backend, which disables every constellation. One of the two should be the rule; the validation arguably belongs in front of both backends.
- One unreproduced transient on the F9P: a combined `--rtcm-out none --raw-out obs` invocation produced no raw observations afterwards, though the same invocation succeeds in isolation.
