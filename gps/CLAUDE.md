# GPS package

## Hardware testing

Use the `hardware-test-gps-msgs` skill to do end-to-end testing of the GPS message pipeline (`gpsprot.*Msg`) against real GPS hardware. This runs `satpulsed` with event and packet logging to verify that binary packets are parsed, converted to `gpsprot` messages, and contain plausible data.
