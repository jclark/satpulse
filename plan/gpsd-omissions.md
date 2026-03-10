# gpsd data not covered by gpsprot

Fields from gpsd's TPV and SKY JSON objects that have no equivalent in
`gpsprot` msg.go (including changes planned in unified-events.md).

| Category | Source | Field | Description |
|---|---|---|---|
| Jamming | TPV | `jam` | Jamming indicator (0-255) |
| Temperature | TPV | `temp` | Receiver temperature |
| Antenna | TPV | `ant` | Antenna status |
| Antenna | TPV | `antPwr` | Antenna power |
| Magnetic | TPV | `magtrack` | Magnetic course over ground |
| Magnetic | TPV | `magvar` | Magnetic declination |
| Local clock | TPV | `clockbias` | Local GNSS clock offset from UTC (ns) |
| Local clock | TPV | `clockdrift` | Clock drift rate (ns/s) |
| Accuracy | TPV | `epx` | Estimated longitude error |
| Accuracy | TPV | `epy` | Estimated latitude error |
| Accuracy | TPV | `epc` | Estimated vertical velocity error |
| DOP | SKY | `xdop` | Longitudinal DOP |
| DOP | SKY | `ydop` | Latitudinal DOP |
| Satellite health | SKY | `health` | Satellite health |
| Satellite health | SKY | `qual` | Per-satellite quality indicator |
| GLONASS | SKY | `freqid` | GLONASS frequency slot |
| Pseudorange | SKY | `pr` | Pseudorange |
| Pseudorange | SKY | `prRate` | Pseudorange rate of change |
| Pseudorange | SKY | `prRes` | Pseudorange residual |
| Moving base RTK | TPV | `relN` | North component of relative position |
| Moving base RTK | TPV | `relE` | East component of relative position |
| Moving base RTK | TPV | `relD` | Down component of relative position |
| Maritime | TPV | `depth` | Depth below keel |
| Maritime | TPV | `wanglem` | Wind angle magnetic |
| Maritime | TPV | `wangler` | Wind angle relative |
| Maritime | TPV | `wanglet` | Wind angle true |
| Maritime | TPV | `wspeedr` | Wind speed relative |
| Maritime | TPV | `wspeedt` | Wind speed true |
| Maritime | TPV | `wtemp` | Water temperature |
| Reference datum | TPV | `datum` | Reference datum name |

## gpsd object types not handled

The following gpsd JSON object types are not covered by gpsprot:

- **PPS** -- pulse-per-second timing (out of scope; satpulse handles PPS via its own PHC/PTP path)
- **IMU** -- inertial measurement unit raw data
- **ATT** -- attitude (heading/pitch/roll from IMU fusion)
- **OSC** -- oscillator discipline status

