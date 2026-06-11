# Bugs found by hardware testing

Clear satpulsetool bugs found while running gpshwtest against real receivers, with the evidence. Receiver limitations belong in `HW/`, and the intended semantics in `SEMANTICS.md`.

## gen 8 backend ignores antenna cable delay sets

On the LEA-M8T, `--ant-cable-delay` does not set anything: the backend polls CFG-TP5 and never writes it, then reports the receiver's existing value as achieved, with no error. Probed with 1, 123, and 32767 ns against a stored value of 50 ns - the requests bracket the stored value, so no range limit explains it. Violates the property semantics: a set must be realized or refused. Evidence: run artifacts `runs/20260610-180323/004-set-antennaCableDelay.jsonl` (outbound CFG-TP5 is a poll only).

## gen 8 backend reports a fixed position it cannot read back

Setting a fixed position as LLH on the LEA-M8T echoes the achieved position in LLH form (quantized to 1e-7 deg), but the backend stores ECEF (TMODE2) and readback reports only ECEF. The reported achieved value can never be confirmed by an independent readback, violating the readback invariant for properties. The set response should report the achieved value as stored (or the backend should store LLH when set as LLH, as the gen 9 backend preserves representation).

## gen 10 signal selection enables BDS B1I on a B1C-only receiver

On the EVK-M101 (SPG 5.10, PROTVER 34.10), enabling BDS sends CFG-VALSET with BDS_B1_ENA=1 and BDS_B1C_ENA=1; the receiver NACKs the transaction. Its own configuration reports the B1I key as readable but 0, and the M10 standard-precision parts track BDS B1C, not B1I; the same request on the F9P (B1I+B2I) is accepted. Consequence: BDS cannot be enabled through satpulsetool on this receiver, it never enters the discovered signal set, and every BDS-involving signal request is refused and unexpressible. The deduced supported-signal table for gen 10 should not include B1I. Evidence: run artifacts `runs/20260611-085153-ttyUSB1/0*-set-signals-BDS.jsonl` (the VALSET and NACK), the initial-config VALGET (B1I=0, B1C=1).

## unresolved observations

- Setting the time pulse width (--pps) realizes the whole pulse bundle including time grid = GPS. On a receiver whose as-found grid is UTC (factory state of the F9P and M10), the first probing run cannot restore the as-found running configuration - absence of timeGNSS is unrepresentable - and reports itself not-left-as-found once. Preserving the existing grid when only the width is requested would avoid the side effect.
- Backends disagree on a request that is invalid under the tool's own semantics: a denoted set with no non-augmentation signal (e.g. `--gnss GLO --band E5b` on a receiver with no GLO E5b-band signal) is refused by the ubx backend ("no suitable supported GNSS signal was enabled") but accepted by the unc backend, which disables every constellation. One of the two should be the rule; the validation arguably belongs in front of both backends.
- `--binary --pvt-out off` in one invocation leaves the messages of `--binary`'s baseline enabled, while `--pvt-out pos,vel,time,off` does disable the leap-second message: the combination of `--binary` with an explicit `off` looks order-dependent.
- One unreproduced transient on the F9P: a combined `--rtcm-out none --raw-out obs` invocation produced no raw observations afterwards, though the same invocation succeeds in isolation.
