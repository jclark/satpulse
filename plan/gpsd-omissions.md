# gpsd data not covered by gpsprot

Fields from gpsd's TPV and SKY JSON objects that have no equivalent in
`gpsprot` msg.go (including changes planned in unified-events.md).

| Category | Source | Field | Description | Plan |
|---|---|---|---|---|
| Jamming | TPV | `jam` | Jamming indicator (0-255) | [jamming.md](jamming.md) |
| Temperature | TPV | `temp` | Receiver temperature | |
| Antenna | TPV | `ant` | Antenna status | [antenna-status.md](antenna-status.md) |
| Antenna | TPV | `antPwr` | Antenna power | [antenna-status.md](antenna-status.md) |
| Magnetic | TPV | `magtrack` | Magnetic course over ground | |
| Magnetic | TPV | `magvar` | Magnetic declination | |
| Local clock | TPV | `clockbias` | Local GNSS clock offset from UTC (ns) | [clock-bias-drift.md](clock-bias-drift.md) |
| Local clock | TPV | `clockdrift` | Clock drift rate (ns/s) | [clock-bias-drift.md](clock-bias-drift.md) |
| Accuracy | TPV | `epx` | Estimated longitude error | [lat-lon-acc.md](lat-lon-acc.md) |
| Accuracy | TPV | `epy` | Estimated latitude error | [lat-lon-acc.md](lat-lon-acc.md) |
| Accuracy | TPV | `epc` | Estimated vertical velocity error | |
| DOP | SKY | `xdop` | Longitudinal DOP | [north-east-dop.md](north-east-dop.md) |
| DOP | SKY | `ydop` | Latitudinal DOP | [north-east-dop.md](north-east-dop.md) |
| Satellite health | SKY | `health` | Satellite health | [satellite-health.md](satellite-health.md) |
| Satellite health | SKY | `qual` | Per-satellite quality indicator | |
| GLONASS | SKY | `freqid` | GLONASS frequency slot | |
| Pseudorange | SKY | `pr` | Pseudorange | |
| Pseudorange | SKY | `prRate` | Pseudorange rate of change | |
| Pseudorange | SKY | `prRes` | Pseudorange residual | |
| Moving base RTK | TPV | `relN` | North component of relative position | |
| Moving base RTK | TPV | `relE` | East component of relative position | |
| Moving base RTK | TPV | `relD` | Down component of relative position | |
| Maritime | TPV | `depth` | Depth below keel | |
| Maritime | TPV | `wanglem` | Wind angle magnetic | |
| Maritime | TPV | `wangler` | Wind angle relative | |
| Maritime | TPV | `wanglet` | Wind angle true | |
| Maritime | TPV | `wspeedr` | Wind speed relative | |
| Maritime | TPV | `wspeedt` | Wind speed true | |
| Maritime | TPV | `wtemp` | Water temperature | |
| Reference datum | TPV | `datum` | Reference datum name | |

## gpsd object types not handled

The following gpsd JSON object types are not covered by gpsprot:

- **PPS** -- pulse-per-second timing (out of scope; satpulse handles PPS via its own PHC/PTP path)
- **IMU** -- inertial measurement unit raw data
- **ATT** -- attitude (heading/pitch/roll from IMU fusion)
- **OSC** -- oscillator discipline status
