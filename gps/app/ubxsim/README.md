# u-blox receiver simulator

`satpulsetool ubxsim` serves a recorded receiver personality through a pseudo-terminal.
It prints the slave path on standard output and answers configuration requests until interrupted.
`--link path` creates a symlink to the slave for the duration of the process.
The personality contains MON-VER, MON-GNSS and default-layer CFG-VALGET responses; the capture scripts are in `capture/`.
The F9P personality in `testdata/f9p/f9p-personality.ubx` can be used without hardware.

Without `--replay` or `--timing`, the simulator answers configuration traffic but emits no navigation messages.
`--replay file` supplies a recorded packet log as navigation output.
`--port uart1` is the default; `uart2`, `usb` and `spi` are also supported.
UART output is paced at the configured baud rate.

## Live timing on Linux

```sh
out/amd64/satpulsetool ubxsim --timing \
    --link /tmp/ubxsim \
    gps/app/ubxsim/testdata/f9p/f9p-personality.ubx
```

`--timing` generates a 1 Hz pulse stream and live NAV-TIMEGPS, NAV-TIMEUTC, NAV-PVT and TIM-TP messages.
The default reference is the system clock, pairing local realtime with `CLOCK_MONOTONIC_RAW`.
It assumes system realtime is already set and is not stepped during the run.
Use `--phc /dev/ptpN` to select a reference PHC instead; it must support `PTP_SYS_OFFSET_PRECISE` and supply UTC time.
For example, `--phc /dev/ptp_kvm` in a KVM guest supplies host realtime paired with guest raw time, even when guest realtime is wrong.
The reference PHC must not be the virtual PHC being synchronized.

The simulator writes future rising-edge raw-clock nanosecond deadlines to `/sys/devices/platform/virt-phc-extts/extts_schedule`.
Each write replaces the complete schedule; the simulator owns this schedule and submits the next deadline after the preceding edge has passed.
`--edge path` selects a different schedule attribute.
The caller needs read access to the reference PHC and write access to the schedule attribute.
The `virt-phc-extts` driver must timestamp the nominal deadline, rather than the eventual timer callback time.

After about one second of initial sampling, the simulator fits a linear mapping over at most five seconds of paired clock samples.
It schedules each nominal UTC second in advance and enqueues messages 100 ms after that second.
UART transmission and process scheduling can add delay to packet delivery.
NAV messages describe the preceding second; TIM-TP describes the next pulse, including GPS week rollover.
The fixed GPS-UTC offset defaults to 18 seconds and can be set with `--leap-seconds`.
Leap-second transitions are not simulated.
The message accuracy fields are synthetic 10 ns values, not measurements of the reference or mapping accuracy.

Output follows the personality's MSGOUT and OUTPROT configuration, including message-rate decimation; `satpulsed` can enable these messages during its normal startup configuration.
The pulse stream remains fixed at 1 Hz regardless of receiver time-pulse or navigation-rate settings.
NAV-PVT reports a time-only fix without a position solution.
TIM-TOS generation is tested, but the message is not emitted: it belongs to M8 time/frequency products and has no F9 CFG-MSGOUT key.
The configuration responder retains its existing behaviour.

`--timing` cannot be combined with `--replay` and is unavailable on macOS.
Reference-clock errors, discontinuities exceeding 1 ms relative to the fitted mapping, missed output seconds, and failed edge writes terminate the simulator with an error.
