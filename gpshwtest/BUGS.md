# Bugs found by hardware testing

Clear satpulsetool bugs found while running gpshwtest against real receivers, with the evidence. Receiver limitations belong in `HW/`, and the intended semantics in `SEMANTICS.md`.

## gen 8 backend ignores antenna cable delay sets

On the LEA-M8T, `--ant-cable-delay` does not set anything: the backend polls CFG-TP5 and never writes it, then reports the receiver's existing value as achieved, with no error. Probed with 1, 123, and 32767 ns against a stored value of 50 ns - the requests bracket the stored value, so no range limit explains it. Violates the property semantics: a set must be realized or refused. Evidence: run artifacts `runs/20260610-180323/004-set-antennaCableDelay.jsonl` (outbound CFG-TP5 is a poll only).

## gen 8 backend reports a fixed position it cannot read back

Setting a fixed position as LLH on the LEA-M8T echoes the achieved position in LLH form (quantized to 1e-7 deg), but the backend stores ECEF (TMODE2) and readback reports only ECEF. The reported achieved value can never be confirmed by an independent readback, violating the readback invariant for properties. The set response should report the achieved value as stored (or the backend should store LLH when set as LLH, as the gen 9 backend preserves representation).

## Unicore backend reports no port for --show-port

On the UM980, `--show-port` returns a config object with no `port` or `baudRate` fields. The backend's `ConfigProps` (`gps/internal/unc/config.go`) never sets them - `convertToProps` (`gps/internal/unc/cfgprops.go`) handles only PPS, signal group, mask, mode, and SBAS - although the receiver's CONFIG response, which the backend already consumes, carries the active port and speed (`$CONFIG,COM1,CONFIG COM1 115200*23`). The u-blox backend shows the intended behavior (`gps/internal/ubx/ubxcfg.go`: `valPort`, `SetBaudRate`). Evidence: run artifacts `runs/20260610-203914-ttyUSB0/004-show-port.jsonl`.

## Unicore raw navigation output saturates the link

`--raw-out nav` on the UM980 is realized as `GPSEPHB 1`, `BDSEPHB 1`, `GALEPHB 1`, `GLOEPHB 1`, `QZSSEPHB 1`: a full ephemeris dump of every constellation every second. That exceeds 115200 baud continuously, so the link saturates and stays saturated - the subsequent `--raw-out none` cannot get a request through ("request abandoned after timeout") and every later invocation in the session is poisoned. Broadcast ephemeris changes on a timescale of hours; the realization should be ONCHANGED (or a long period), as the u-blox raw-nav realization is effectively event-driven subframes. Evidence: run artifacts `runs/20260610-205200-ttyUSB0/073-set-rawOut-nav.jsonl` (the commands), `075-set-rawOut-none.jsonl` (the unreachable disable); recovery needed a low-level `UNLOGALL` write to the port.

## Unicore declares rtcmBaseID support but the property does not exist

The UM980 backend lists `rtcmBaseID` in `ConfigSupportFlags`, but setting `--rtcm-base-id` reports nothing achieved and readback omits the property - the defined signature of a property that does not exist on the backend. One side is wrong: either the capability flag or the property realization. Evidence: run artifacts `runs/20260610-205200-ttyUSB0/020-set-rtcmBaseID.jsonl` ff.

## gen 8 reload loads factory defaults instead of NVM

On the LEA-M8T, `--reload` sends CFG-CFG with loadMask 0x1f1f but deviceMask 0x00 (no devices), and the receiver answers by loading factory defaults rather than the saved configuration. Demonstrated by the save-granularity experiments: a CFG-NAV5 set followed by CFG-CFG saveMask=navConf deviceMask=0x03 is ACKed, yet a subsequent `--reload` reads back minElevation 5, timeGNSS GPS, pulse width 0.1, mobile mode - the u-blox defaults - and drops the UART to the default 9600. That saving itself works is proven by `--save-all` (also deviceMask 0x03) persisting a port speed change through a `--reset` (hardware reset, boots from BBR). The reload should use the same deviceMask as the saves. Note the reload's CFG-CFG also goes unacknowledged at the moment the loaded ioPort drops the link speed, so the invocation reports a transient error while the load did execute. Evidence: run artifacts `runs/20260610-225556-ttyS0/200-gran-save-minElevation.jsonl` (ACKed save), `202-gran-reload-minElevation.jsonl` (deviceMask 0x00 load), `206-gran-F-minElevation.jsonl` (defaults read back).

## unresolved observations

- `--binary --pvt-out off` in one invocation leaves the messages of `--binary`'s baseline enabled, while `--pvt-out pos,vel,time,off` does disable the leap-second message: the combination of `--binary` with an explicit `off` looks order-dependent.
- One unreproduced transient on the F9P: a combined `--rtcm-out none --raw-out obs` invocation produced no raw observations afterwards, though the same invocation succeeds in isolation.
