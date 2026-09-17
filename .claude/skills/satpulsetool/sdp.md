# satpulsetool sdp

Manages software-defined pins (SDPs) on PTP hardware clocks (PHCs): a pin can
timestamp input pulses (external timestamping, `extts`), for example the PPS
output of a GNSS receiver, or generate periodic output pulses (`perout`).
Linux only: the subcommand does not exist in builds for other systems. Full
reference: satpulsetool-sdp(1).

One of four modes, at most one per invocation, chosen by `--show` (the
default), `-i`/`--extts`, `-o`/`--perout`, or `--disable`. The positional
argument is the network interface. It is optional with `--show` and required
otherwise. Without an interface, discovery reads sysfs and needs no
privileges; with one, the command opens the PHC device and needs root.

`--extts`, `--perout`, and `--disable` all change the pin's function: confirm
with the user before running them (see SKILL.md).

## What PHCs and pins are there?

```
satpulsetool sdp
satpulsetool sdp enp4s0
```

Without an interface, lists every interface whose PHC has SDPs; with one,
shows that interface's pins.

## Is PPS arriving on a pin?

`-i` configures the pin as an input and prints each timestamp as it arrives,
for `-t` seconds (default 2; 0 runs until interrupted). Pin and channel
default to 0; choose with `-p INDEX|NAME` and `--chan INDEX`. `-j` gives JSON
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
