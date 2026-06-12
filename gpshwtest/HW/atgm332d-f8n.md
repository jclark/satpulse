# ATGM332D-AT9880-F8N-76 (URANUS6 V6.3.2.0)

Dual-band CASIC navigation receiver, UART at 115200. Characterized
2026-06-13; baseline `ATGM332D-AT9880-F8N-76-SW=URANUS6,V6.3.2.0.json`.

## Limitations relative to the model

- Signals: hardware set is GPS L1+L5, GAL E1+E5a, BDS B1I+B1C+B2a,
  GLO L1, QZSS L1+L5, NAVIC L5, SBAS L1. Requests naming only
  augmentation/regional systems (QZSS, NAVIC, SBAS alone) are refused,
  as is the empty set: at least one of GPS/GAL/BDS/GLO must be
  enabled. The chip silently clamps the requested reception list to
  its hardware (the model's silent intersection, performed by the
  silicon).
- Time-of-pulse and leap-second output (`tp`, `leap`): the firmware
  acknowledges enabling TIM-TP's fallback TIM2-TPX and TIM2-TIMEGPS
  but never emits them - pulse-time and leap information is not
  deliverable on this unit (it is on the AT632 timing variant).
- Raw output (`obs`, `nav`): same acknowledge-but-never-emit firmware
  limitation; RXM2-MEASX/RXM2-SFRBX never appear.
- RTCM output: none; rtcmBaseID does not exist.
- Fixed position: stored as ECEF in 1 cm steps with accuracy in 1 mm
  steps; an LLH request is realized by conversion to ECEF, so the LLH
  form never reads back (the position does, as ECEF).
- Time pulse width quantized to 1 us.
- Active port: the protocol cannot identify which UART the
  conversation uses (a port query answers with one entry per UART),
  so the port property does not exist.

## Notes

- Probing quiets NMEA output (RAM only) to keep saturated lines
  answerable; an invocation that does not name NMEA output leaves it
  quiet.
- The mask field of the save command is documented as reserved but is
  honoured: zero saves nothing. The load command restarts the
  receiver without acknowledging.
