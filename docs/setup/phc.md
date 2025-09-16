---
title: Verify PPS input on PTP Hardware Clock
---

SatPulse needs to know the name of the ethernet interface
with the pin that the PPS output of the GPS module is connected to. 

It also needs to know the number of the pin.
These pins are often called Software Defined Pins (SDPs).
They are numbered starting from 0 and also have a name.
SatPulse defaults to using pin 0.
So if there is only one pin, then there is no need to specify anything.

The ethernet controller will have a PTP Hardware Clock (PHC) device associated with it,
which has an index which is a non-negative integer.
If the index is N, then the device path is /dev/ptpN.
In Linux, the network interface has an associated PHC and
operations related to PPS input apply to the PHC rather than directly to the network interface.

On the Raspberry Pi CM4 and CM5 with Raspberry Pi OS, the network interface is `eth0` and there's only one pin,
which is named SYNC_OUT.

With Intel ethernet controllers, the name of interface can vary.
With the i210/i225/i226, there are four pins called SDP0, SDP1, SDP2 and SDP3.

## Using `satpulsetool sdp`

satpulsetool provides a subcommand [sdp]({%link man/satpulsetool-sdp.1.md%}) which is useful for identifying the interface.

* `satpulsetool sdp` with no arguments shows information about each interface that has one or more SDPs;
* `satpulsetool sdp -i --pin 0 eth0` checks for input on the specified interface and pin for 2 seconds by default; it prints out the timestamps it received as a number of seconds since the beginning of 1970; `--pin 0` is the default.

### CM4/5

On a CM5, `satpulsetool sdp` should show something like this:

```
Interface: eth0
  Status: up, carrier
  Driver: macb
  PCI: 1f00100000.ethernet
  PTP clock device: /dev/ptp0
  Pins: SYNC_OUT
  External timestamp channels: 1
  Periodic output channels: 1
```

If it doesn't show anything, then your kernel is not working.

Doing

```
sudo satpulsetool sdp -i eth0
```

should print one or two numbers, like this

```
9623.787299552
9624.787296512
```

This means it has received two timestamps, one second apart, so everything is working as expected.

### Intel

On my Intel box, which has two i225 cards in it, `satpulsetool sdp` shows this:

```
Interface: enp4s0
  Status: up, no-carrier
  Driver: igc
  PCI: 0000:04:00.0
  Vendor: Intel Corporation
  Device: Ethernet Controller I225-LM
  Revision: 03
  PTP clock device: /dev/ptp0
  Pins: SDP0, SDP1, SDP2, SDP3
  External timestamp channels: 2
  Periodic output channels: 2
Interface: enp5s0
  Status: down
  Driver: igc
  PCI: 0000:05:00.0
  Vendor: Intel Corporation
  Device: Ethernet Controller I225-LM
  Revision: 03
  PTP clock device: /dev/ptp1
  Pins: SDP0, SDP1, SDP2, SDP3
  External timestamp channels: 2
  Periodic output channels: 2
```

The one I want is the one that is up, which is `enp4s0`.
The GPS is attached to pin 1, so doing:

```
sudo satpulsetool sdp -i --pin 1 enp4s0
```

shows:

```
1757770748.482694937
1757770748.582695570
1757770749.482701362
1757770749.582701989
```

Intel NICs produce timestamps for both edges of the pulse.

If I try without `--pin 1`, it would report no timestamps received.

## Without satpulsetool

If you do not yet have satpulsetool installed, you can use other standard Linux commands to figure it out.

### Identifying the interface on a PC

The command

```
ls -l /sys/class/net/*/device/driver/module | grep 'ig[cb]'
```

will show the interfaces that have igb (for i210) or igc (for i225/i226) as their driver.

For example, on one of my machines, it shows

```
lrwxrwxrwx 1 root root 0 Sep 13 16:48 /sys/class/net/enp4s0/device/driver/module -> ../../../../module/igc
lrwxrwxrwx 1 root root 0 Sep 13 16:48 /sys/class/net/enp5s0/device/driver/module -> ../../../../module/igc
```

This tells me that I have interfaces enp4s0 and enp5s0 using the igc driver.

### Identifying the PHC for an interface

You can identify the PHC device for an interface using `ethtool -T`.

For example, on a CM4 or 5

```
ethtool -T eth0
```

should show:

```
Time stamping parameters for eth0:
Capabilities:
        hardware-transmit
        hardware-receive
        hardware-raw-clock
PTP Hardware Clock: 0
Hardware Transmit Timestamp Modes:
        off
        on
        onestep-sync
        onestep-p2p
Hardware Receive Filter Modes:
        none
        ptpv2-event
```

Note that `PTP Hardware Clock: 0` means that this interface uses `/dev/ptp0` as
its PHC device.

If you don't get this, then your kernel probably lacks the necessary support.

### Ensure interface is up

Make sure the interface is in an UP state. For example:

```
ip link show enp1s0
```

If it's not up, then bring it up with

```
sudo ip link set enp1s0 up
```

### Verify PPS input

Now you know the PHC device, you can check if PPS input is working.
Assuming the PHC device is ptp0, you can see what pins it has with

```
ls /sys/class/ptp/ptp0/pins
```

To verify that the PPS connection is working on one of the pins, for example SYNC_OUT,
first configure that pin for input: 

```
echo 1 0 | sudo tee /sys/class/ptp/ptp0/pins/SYNC_OUT
```

In the `echo 1 0`, 1 means to use the pin for input and 0 means the pin should use input channel 0.

Now do:

```
echo 0 1 | sudo tee /sys/class/ptp/ptp0/extts_enable
```

This means to enable timestamping of pulses on channel 0.
In the `echo 0 1`, 0 means channel 0 and 1 means to enable timestamping.

Now see if we're getting timestamps:

```
sudo cat /sys/class/ptp/ptp0/fifo
```

The `cat` command should output a line, which represents a timestamp of an input pulse and consists of 3 numbers: channel number, which is zero in this case, seconds count, nanoseconds count. Repeating the last command will give lines for successive input timestamps.

If `cat` outputs nothing, then it's not working.

## Pulse width on Intel

The i210 and i225/i226 NICs have the quirk that they timestamp both the rising and falling edges of every pulse.
This means that a PPS signal will result in two timestamps per second.
Most GPS receivers default to a pulse width of 0.1s
which means that you should see consecutive timestamps separated alternately by roughly 0.1s and 0.9s
(i.e. 100,000,000 and 900,000,000 nanoseconds).
By looking at the timestamps you can determine what the pulse width is.
