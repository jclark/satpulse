---
title: PPS connection setup
---

## Identify the interface

<!-- From rpi-cm4-ptp-guide/os.md lines 114-142 -->
Check that your kernel includes the necessary have ethernet PTP hardware support

```
ethtool -T eth0
```

You should see:

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
its hardware clock.

## PPS connection verification on RPi

<!-- From rpi-cm4-ptp-guide/os.md lines 195-222 -->
### PPS connection

To verify that the PPS connection is working, first configure the SYNC_OUT for input: 

```
echo 1 0 | sudo tee /sys/class/ptp/ptp0/pins/SYNC_OUT
```

Replace the `0` in `ptp0` with whatever `ethtool` said was the number.
`SYNC_OUT` here is the name of the pin to which the PPS is connected. In the `echo 1 0`, 1 means to use the pin for input and 0 means the pin should use input channel 0.


Now do:
```
echo 0 1 | sudo tee /sys/class/ptp/ptp0/extts_enable
```

This means to enable timestamping of pulses on channel 0. In the `echo 0 1`, 0 means channel 0 and 1 means to enable timestamping.


Now see if we're getting timestamps:

```
sudo cat /sys/class/ptp/ptp0/fifo
```

The `cat` command should output a line, which represents a timestamp of an input pulse and consists of 3 numbers: channel number, which is zero in this case, seconds count, nanoseconds count. Repeating the last command will give lines for successive input timestamps.

If `cat` outputs nothing, then it's not working.

## PPS verification on PC

<!-- From pc-ptp-ntp-guide/server-linux.md lines 7-57 -->
## Verify PPS

In this section, we will check that the PPS signal into the NIC is working properly.

If the NIC is `enp1s0`, then first do:

```
ethtool -T enp1s0 | grep PTP
```

This will give us the number of the PTP Hardware Clock, which will often be 0.

Make sure the interface is in an UP state.

```
ip link show enp1s0
```

If it's not up, then bring it up with

```
sudo ip link set enp1s0 up
```

Then we can do 

```
echo 1 0 | sudo tee /sys/class/ptp/ptp0/pins/SDP0
```

Replace the `0` in `ptp0` with whatever `ethtool` said was the number.
`SDP0` here is the name of the pin to which the PPS is connected. In the `echo 1 0`, 1 means to use the pin for input and 0 means the pin should use input channel 0.


Now do:
```
echo 0 1 | sudo tee /sys/class/ptp/ptp0/extts_enable
```

This means to enable timestamping of pulses on channel 0. In the `echo 0 1`, 0 means channel 0 and 1 means to enable timestamping.


Now see if we're getting timestamps:

```
sudo cat /sys/class/ptp/ptp0/fifo
```

The `cat` command should output a line, which represents a timestamp of an input pulse and consists of 3 numbers: channel number, which is zero in this case, seconds count, nanoseconds count. Repeating the last command will give lines for successive input timestamps.

If `cat` outputs nothing, then it's not working.

### Pulse width

<!-- From pc-ptp-ntp-guide/server-linux.md lines 58-65 -->
The i210 and i225 NICs have the quirk that they timestamp both the rising and falling edges of the every pulse. This means that a PPS signal will result
in two timestamps per second. Most GPS receivers default to a pulse width of 0.1s, which means that you should see consecutive timestamps separated alternately
by 0.1s and 0.9s (i.e. 100,000,000 and 900,000,000 nanoseconds).
By looking at the output of `cat` you can determine what the pulse width is.
Both chrony or ts2phc need to be configured with the pulse width for NICs that timestamp both edges.