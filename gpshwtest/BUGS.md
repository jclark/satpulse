# Bugs found by hardware testing

Clear satpulsetool bugs found while running gpshwtest against real receivers, with the evidence. Receiver limitations belong in `HW/`, and the intended semantics in `SEMANTICS.md`.

## gen 8 backend reports a fixed position it cannot read back

Setting a fixed position as LLH on the LEA-M8T echoes the achieved position in LLH form (quantized to 1e-7 deg), but the backend stores ECEF (TMODE2) and readback reports only ECEF. The reported achieved value can never be confirmed by an independent readback, violating the readback invariant for properties. The set response should report the achieved value as stored (or the backend should store LLH when set as LLH, as the gen 9 backend preserves representation).

## val-layer signal path ignores the receiver's concurrent major GNSS limit

The non-legacy (CFG-VALSET) signal path does not apply the protocol-reported limit on concurrent major GNSS (the `simultaneous` field of UBX-MON-GNSS), so a request enabling more majors than the receiver supports is sent as-is and fails on a receiver NAK. The legacy CFG-GNSS path handles this: it polls MON-GNSS, drops GLONASS if the request is one over the limit, and otherwise refuses with a clear error. On the EVK-M101 (SPG 5.10, PROTVER 34.10), whose limit is 3, the all-constellations request builds a VALSET enabling four majors and the receiver NAKs it with `GPTXT inv sig cfg` / `bad cfg RAM`, configuration unchanged; the legacy heuristic would have landed exactly on the limit and succeeded. Under the enabled-signals semantics the limit is protocol-reported and so belongs in the fixup intersection. Tracked as issue #313. Evidence: run artifacts `runs/20260611-130850-ttyUSB1/105-set-signals-GPS-GAL-BDS-GLO-QZSS-SBAS.jsonl` (the four-major VALSET, the NAK, and the GPTXT messages).

## unresolved observations

- Setting the time pulse width (--pps) realizes the whole pulse bundle including time grid = GPS. On a receiver whose as-found grid is UTC (factory state of the F9P and M10), the first probing run cannot restore the as-found running configuration - absence of timeGNSS is unrepresentable - and reports itself not-left-as-found once. Preserving the existing grid when only the width is requested would avoid the side effect.
- Backends disagree on a request that is invalid under the tool's own semantics: a denoted set with no non-augmentation signal (e.g. `--gnss GLO --band E5b` on a receiver with no GLO E5b-band signal) is refused by the ubx backend ("no suitable supported GNSS signal was enabled") but accepted by the unc backend, which disables every constellation. One of the two should be the rule; the validation arguably belongs in front of both backends.
- `--binary --pvt-out off` in one invocation leaves the messages of `--binary`'s baseline enabled, while `--pvt-out pos,vel,time,off` does disable the leap-second message: the combination of `--binary` with an explicit `off` looks order-dependent.
- One unreproduced transient on the F9P: a combined `--rtcm-out none --raw-out obs` invocation produced no raw observations afterwards, though the same invocation succeeds in isolation.
