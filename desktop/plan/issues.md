# Desktop GUI issues

## PROBE-READBACK: Combine probe and config readback into one operation

On connect, two separate `gpscfg.Configure` calls happen in sequence: the initial probe (to detect the receiver and get `ReceiverInfo`) and then the config readback (to get current property values for the Config tab). If the user is already on the Config tab when they reconnect, both could be combined into a single `Configure` call by adding `Get: readProps` to the initial probe target. This would halve the round-trip time for the reconnect case.

Currently the probe is initiated by the Go backend (`packetWorker`) and the readback is initiated by the frontend (`doReadback` via `ReadConfig`). Combining them would require the backend to know whether properties should be read during the probe, or a way to piggyback the readback request onto the probe.

## SPEED-CHANGE: Add configuration support for receiver serial speed change

The CLI supports `--speed` to configure the GPS receiver's serial port speed (as opposed to `--device-speed` which sets the host serial port speed). The desktop GUI needs equivalent support so users can change the receiver's baud rate from the Config tab.

This is more involved than a simple property write because after changing the receiver's serial speed, the host serial port must also be reopened at the new speed to maintain communication. The backend would need to coordinate the speed change on the receiver with reopening the serial connection at the matching baud rate.
