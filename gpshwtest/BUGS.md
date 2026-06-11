# Bugs found by hardware testing

Clear satpulsetool bugs found while running gpshwtest against real receivers, with the evidence. Receiver limitations belong in `HW/`, and the intended semantics in `SEMANTICS.md`.

## gen 8 backend ignores antenna cable delay sets

On the LEA-M8T, `--ant-cable-delay` does not set anything: the backend polls CFG-TP5 and never writes it, then reports the receiver's existing value as achieved, with no error. Probed with 1, 123, and 32767 ns against a stored value of 50 ns - the requests bracket the stored value, so no range limit explains it. Violates the property semantics: a set must be realized or refused. Evidence: run artifacts `runs/20260610-180323/004-set-antennaCableDelay.jsonl` (outbound CFG-TP5 is a poll only).

## gen 8 backend reports a fixed position it cannot read back

Setting a fixed position as LLH on the LEA-M8T echoes the achieved position in LLH form (quantized to 1e-7 deg), but the backend stores ECEF (TMODE2) and readback reports only ECEF. The reported achieved value can never be confirmed by an independent readback, violating the readback invariant for properties. The set response should report the achieved value as stored (or the backend should store LLH when set as LLH, as the gen 9 backend preserves representation).

## unresolved observations

- Backends disagree on a request that is invalid under the tool's own semantics: a denoted set with no non-augmentation signal (e.g. `--gnss GLO --band E5b` on a receiver with no GLO E5b-band signal) is refused by the ubx backend ("no suitable supported GNSS signal was enabled") but accepted by the unc backend, which disables every constellation. One of the two should be the rule; the validation arguably belongs in front of both backends.
- `--binary --pvt-out off` in one invocation leaves the messages of `--binary`'s baseline enabled, while `--pvt-out pos,vel,time,off` does disable the leap-second message: the combination of `--binary` with an explicit `off` looks order-dependent.
- One unreproduced transient on the F9P: a combined `--rtcm-out none --raw-out obs` invocation produced no raw observations afterwards, though the same invocation succeeds in isolation.
