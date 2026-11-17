# satpulsetool sdp

Manages software-defined pins (SDPs) on PTP hardware clocks (PHCs): a pin can
timestamp input pulses (external timestamping, `extts`), for example the PPS
output of a GNSS receiver, or generate periodic output pulses (`perout`).
Linux only: the subcommand does not exist in builds for other systems. Full
reference: satpulsetool-sdp(1).

At most one mode may be specified: `--show-ifaces`, `--show-pins`,
`-i`/`--extts`, `-o`/`--perout`, or `--disable`. The positional
argument selects the PHC: a network interface, or, if it contains a `/`, the
PHC's device path (for a PHC with no interface, such as `/dev/ptp1`). It is
optional with `--show-ifaces`, where it must be an interface, and required
otherwise. In `--show-ifaces` mode, discovery needs no privileges; other
modes open the PHC device and need root.

`--extts`, `--perout`, and `--disable` all change the pin's function: confirm
with the user before running them (see SKILL.md).

## What PHCs and pins are there?

```
satpulsetool sdp
satpulsetool sdp enp4s0
```

Without an argument, lists every interface whose PHC has pins or channels; a PHC
with no interface is not listed. With an interface, shows its PHC's pins, or the
interface record if the PHC has no pins; with a device path, shows the pins, and
fails if the PHC has none, since showing interfaces needs an interface.

## Is PPS arriving on a pin?

`-i` configures the pin as an input and prints each timestamp as it arrives,
for `-t` seconds (default 2; 0 runs until interrupted). The channel defaults
to 0, and the pin defaults to 0 when required; choose with `-p INDEX|NAME`
and `--chan INDEX`. `-j` gives JSON
Lines. Timestamps buffered before the read started are dropped unless
`--show-stale` is given.

```
satpulsetool sdp -i eth0
satpulsetool sdp -i -j -t 30 -p 1 eth0
```

Exit status 2 means no timestamps were received.

## Generate a pulse on a pin

`-o` configures the pin as periodic output aligned to the PHC. `--period`
defaults to 1 second (a PPS signal); `--period 0` stops the output. `-w`
sets the pulse width where the driver allows it; Intel igb and igc do not,
and always use half the period.

```
satpulsetool sdp -o eth0
satpulsetool sdp -o -p 1 --period 1e-3 eth0
satpulsetool sdp -o --period 0 eth0
```

## Disable a pin

```
satpulsetool sdp --disable -p 1 eth0
```
