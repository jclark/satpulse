# Bugs found by hardware testing

Clear satpulsetool bugs found while running gpshwtest against real receivers, with the evidence. Receiver limitations belong in `HW/`, and the intended semantics in `SEMANTICS.md`.

## gen 8 backend ignores antenna cable delay sets

On the LEA-M8T, `--ant-cable-delay` does not set anything: the backend polls CFG-TP5 and never writes it, then reports the receiver's existing value as achieved, with no error. Probed with 1, 123, and 32767 ns against a stored value of 50 ns - the requests bracket the stored value, so no range limit explains it. Violates the property semantics: a set must be realized or refused. Evidence: run artifacts `runs/20260610-180323/004-set-antennaCableDelay.jsonl` (outbound CFG-TP5 is a poll only).

## gen 8 backend reports a fixed position it cannot read back

Setting a fixed position as LLH on the LEA-M8T echoes the achieved position in LLH form (quantized to 1e-7 deg), but the backend stores ECEF (TMODE2) and readback reports only ECEF. The reported achieved value can never be confirmed by an independent readback, violating the readback invariant for properties. The set response should report the achieved value as stored (or the backend should store LLH when set as LLH, as the gen 9 backend preserves representation).

## gen 10 BDS expansion requests B1I and B1C together, which the M10 refuses

On the EVK-M101 (SPG 5.10, PROTVER 34.10), enabling BDS sends CFG-VALSET with both BDS_B1_ENA (B1I) and BDS_B1C_ENA set to 1, and the receiver NAKs the transaction with `GNTXT inv sig cfg` / `bad cfg RAM`. Both signals are individually supported - verified by direct VALSET: B1C-only is accepted, B1I-only is accepted, both-at-once is refused atomically - matching the SPG 5.10 release note: "The product works with either one of these signal types at a time." The mutual exclusion is not protocol-reported, so satpulse cannot learn it at the fixup stage; but --gnss BDS denotes all BDS signals, so its expansion is always the invalid combination and BDS is unreachable on this receiver. The deduced BDS signal set for M10-class receivers should pick one of the two (B1C matches the factory default B1C=1/B1I=0), or the pairing rule needs modeling. Evidence: run artifacts `runs/20260611-085153-ttyUSB1/0*-set-signals-BDS.jsonl` (the VALSET and NACK), and the one-at-a-time VALSET verification of 2026-06-11 (B1C-only OK, B1I-only OK, both NAK, configuration unchanged).

## unresolved observations

- Setting the time pulse width (--pps) realizes the whole pulse bundle including time grid = GPS. On a receiver whose as-found grid is UTC (factory state of the F9P and M10), the first probing run cannot restore the as-found running configuration - absence of timeGNSS is unrepresentable - and reports itself not-left-as-found once. Preserving the existing grid when only the width is requested would avoid the side effect.
- Backends disagree on a request that is invalid under the tool's own semantics: a denoted set with no non-augmentation signal (e.g. `--gnss GLO --band E5b` on a receiver with no GLO E5b-band signal) is refused by the ubx backend ("no suitable supported GNSS signal was enabled") but accepted by the unc backend, which disables every constellation. One of the two should be the rule; the validation arguably belongs in front of both backends.
- `--binary --pvt-out off` in one invocation leaves the messages of `--binary`'s baseline enabled, while `--pvt-out pos,vel,time,off` does disable the leap-second message: the combination of `--binary` with an explicit `off` looks order-dependent.
- One unreproduced transient on the F9P: a combined `--rtcm-out none --raw-out obs` invocation produced no raw observations afterwards, though the same invocation succeeds in isolation.
