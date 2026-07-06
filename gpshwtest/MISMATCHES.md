# Model mismatches

Places where the device-independent configuration model (`gps/gpsprot.ConfigTarget`/`ConfigProps`, semantics in `SEMANTICS.md`) fits a receiver's own configuration concepts awkwardly. These are candidates for model improvements, each separate cross-backend work; none is a defect in a backend, which must realize the model as it stands and show the rest as absence. Collected during the Septentrio mosaic-G5 bring-up (#341); add to this file as other bring-ups find more.

## Corrections / PPP has no property

There is no property expressing "use this corrections service in the solution". On the mosaic-G5, Galileo HAS PPP needs `setPVTMode, , +PPP` (gated on the `PPPGalileoHAS-SIS` capability) in addition to tracking E6; nothing in the model can request that, so HAS is currently unreachable through high-level configuration (an implicit enablement inferred from the E6 signal request was implemented and removed by owner ruling: no other backend infers correction usage from a signal request). Cross-backend precedent is strong: Unicore has `CONFIG PPP ENABLE E6-HAS`, u-blox has HAS and PointPerfect machinery, and BeiDou B2b / QZSS MADOCA exist on other hardware. An explicit corrections property (none / SBAS / GAL-HAS / BDS-B2b / ..., with truthful readback) is the planned fix, "in due course" per the owner.

## NavMsgAuth is binary; OSNMA has levels

`NavMsgAuth` is None/OSNMA, but Septentrio's `setGalOSNMAUsage` is two-dimensional: PVT level off/loose/strict and measurement level off/loose, where strict additionally requires an external trusted time (`exeSetTime`). The binary property cannot request strict, and the mosaic-G5 factory default is already PVT-level loose, so a default receiver truthfully reads back "OSNMA" - surprising next to receivers where authentication is off by default. When the trusted-time work lands (#105), NavMsgAuth likely wants levels (None/Loose/Strict); the Septentrio side is shaped to consume that.

## The allowed-solution-modes list has no analogue

Septentrio's Rover mode carries a list of solution modes the PVT engine may use (StandAlone, DGNSS, RTKFloat, RTKFixed, PPP). The model's `Mode` is static-vs-rover plus fixed position; it cannot express "rover without RTK" or similar. The backend preserves the receiver's list by omitting the argument. Probably fine as absence unless a second backend needs it.

## TimePulse models one pulse

The mosaic-G5 has a second, independent PPS output (`setPPS2Parameters`), and u-blox Gen 9 has TIMEPULSE2, so two backends could realize a second pulse; `gpsprot.TimePulse` models exactly one. Exposing a second pulse is a cross-backend API change (an index or a second TimePulse), demand-driven.
