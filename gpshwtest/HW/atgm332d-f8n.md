# ATGM332D-AT9880-F8N-76 (URANUS6 V6.3.2.0)

Dual-band CASIC navigation receiver, UART at 115200. Characterized
2026-06-13; baseline `ATGM332D-AT9880-F8N-76-URANUS6,V6.3.2.0.json`.

## Limitations relative to the model

- Signals: hardware set is GPS L1+L5, GAL E1+E5a, BDS B1I+B1C+B2a,
  GLO L1, QZSS L1+L5, NAVIC L5, SBAS L1. Requests naming only
  augmentation/regional systems (QZSS, NAVIC, SBAS alone) are refused,
  as is the empty set: at least one of GPS/GAL/BDS/GLO must be
  enabled. The chip silently clamps the requested reception list to
  its hardware (the model's silent intersection, performed by the
  silicon).
- Time-of-pulse, leap-second, and survey output (`tp`, `leap`,
  `survey`): the firmware acknowledges enabling TIM-TP's fallback
  TIM2-TPX, TIM2-LS, and TIM2-TIMEPOS but never emits them - this
  information is not deliverable on this unit (pulse time and survey
  are on the AT632 timing variant).
- Raw output (`obs`, `nav`): same acknowledge-but-never-emit firmware
  limitation; RXM2-MEASX/RXM2-SFRBX never appear.
- RTCM output: CFG-RTCM exists and is acknowledged, and the port
  protocol-mask RTCM bit sticks in readback, but no RTCM is emitted
  in mobile, survey, or fixed-position mode. rtcmBaseID does not
  exist in the protocol.
- Fixed position: stored as ECEF in 1 cm steps with accuracy in 1 mm
  steps; an LLH request is realized by conversion to ECEF, so the LLH
  form never reads back (the position does, as ECEF).
- Time pulse width quantized to 1 us.
- Active port: the protocol cannot identify which UART the
  conversation uses (a port query answers with one entry per UART),
  so the port property does not exist.

## Notes

- The mask field of the save command is documented as reserved but is
  honoured: zero saves nothing. The load command restarts the
  receiver without acknowledging.
