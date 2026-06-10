# Bugs found by hardware testing

Clear satpulsetool bugs found while running gpshwtest against real receivers, with the evidence. Receiver limitations belong in `HW/`, and the intended semantics in `docs/gps-config-semantics.md`.

## gen 8 backend ignores antenna cable delay sets

On the LEA-M8T, `--ant-cable-delay` does not set anything: the backend polls CFG-TP5 and never writes it, then reports the receiver's existing value as achieved, with no error. Probed with 1, 123, and 32767 ns against a stored value of 50 ns - the requests bracket the stored value, so no range limit explains it. Violates the property semantics: a set must be realized or refused. Evidence: run artifacts `runs/20260610-180323/004-set-antennaCableDelay.jsonl` (outbound CFG-TP5 is a poll only).

## gen 8 backend reports a fixed position it cannot read back

Setting a fixed position as LLH on the LEA-M8T echoes the achieved position in LLH form (quantized to 1e-7 deg), but the backend stores ECEF (TMODE2) and readback reports only ECEF. The reported achieved value can never be confirmed by an independent readback, violating the readback invariant for properties. The set response should report the achieved value as stored (or the backend should store LLH when set as LLH, as the gen 9 backend preserves representation).

## errors on a saturated UART do not mean nothing changed

With raw output enabled at 9600 baud, the M8T's transmit side saturates: packets (including poll replies and ACKs) are lost or corrupted. satpulsetool's response wait (~1.2 s) then expires behind in-flight large messages and invocations fail with "no response to request" - but the configuration write may have been received and applied, with only its ACK lost. Observed both directions: a raw-output change that reported failure but applied, leaving the receiver in a state the next run did not expect. On a saturated link a reported error therefore does not imply an unchanged configuration, which breaks the refusal guarantee. The detection/probe phase fails the same way, so high-level invocations cannot even start; recovery required low-level CFG-MSG writes (`-m`) to disable the raw messages. A longer or traffic-aware response wait would help.

## detection of a fully-silenced receiver is intermittent

`--nmea-out none` on the ZED-F9P (USB) is realized by disabling the NMEA protocol on the port; with binary output not enabled, the receiver emits nothing at all. Detection of the silent receiver then fails intermittently: identical MON-VER polls answered instantly 5 s before and 4 s after went unanswered twice 1.5 s apart in between ("GPS detection failed: no output detected from GPS"). Cause undiagnosed, possibly a port-reopen/DTR timing interaction. Recovery: `--nmea`, then `--nmea-out <set>`.

## unresolved observations

- `--binary --pvt-out off` in one invocation leaves the messages of `--binary`'s baseline enabled, while `--pvt-out pos,vel,time,off` does disable the leap-second message: the combination of `--binary` with an explicit `off` looks order-dependent.
- One unreproduced transient on the F9P: a combined `--rtcm-out none --raw-out obs` invocation produced no raw observations afterwards, though the same invocation succeeds in isolation.
