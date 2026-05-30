# GPS package

## Testing with satpulsed

The `satpulsed-test-instance` skill runs a throwaway `satpulsed` as an unprivileged user (no PHC, logs under a tmp directory) fed from a serial device or a FIFO. Two skills build on it:

- `hardware-test-gps-msgs` - end-to-end test of the GPS message pipeline (`gpsprot.*Msg`) against real GPS hardware, verifying that binary packets are parsed, converted to `gpsprot` messages, and contain plausible data.
- `drive-satpulsed-from-log` - drive `satpulsed` from a recorded packet log through a FIFO, with no GPS hardware, to exercise the pipeline or the web interface from a capture.
