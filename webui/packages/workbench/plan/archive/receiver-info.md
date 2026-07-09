# Receiver panel

## Goal
Add a receiver information panel that automatically identifies the connected GPS receiver on connect and displays its identity. No manual detect button — identification runs automatically when the connection opens.

## Prerequisite
layout-shell.

## Reference documents
- [ui-panel-connection.md](ui-panel-connection.md) - connection strip (triggers auto-detect)
- [backend.md](backend.md) - configuration API calls

## Concept

### What --show-receiver does
`satpulsetool gps --show-receiver` (the default when no other operation is specified) runs a zero `ConfigTarget` with `ForceProbe = true`. This probes the receiver to identify it without reading back any configuration properties or making any changes. The result is `gpsprot.ReceiverInfo`:
- Vendor (e.g. "u-blox")
- Hardware (e.g. "ZED-F9T")
- Firmware (e.g. "TIM 2.20 PROTVER 18.00")
- Supported GNSS constellations

This is a lightweight operation (a few seconds) and is safe to run every time a connection opens.

### Separation from configuration
Reading back configuration properties (`--show-config`) is a separate operation that sets `Get` on the `ConfigTarget` to request specific property values. That belongs in config-restructure, not here.

## Steps

### 1. Backend: auto-probe on connect
After a successful `Connect()`, automatically run receiver identification in the background. This calls the single `Configure` method with a map containing only `forceProbe: true` (no `get` properties, no `props` changes).

The probe runs asynchronously because it takes a few seconds and `Connect()` should return quickly. On completion, emit a `gps:receiver` event with the receiver identity (or an error).

### 2. Frontend: receiver panel component
Rewrite `receiver-panel.tsx` to display receiver identity populated from the `gps:receiver` event:
- Vendor
- Hardware model
- Firmware version
- Supported GNSS constellations
- Packet formats detected

Panel states:
- **Disconnected**: greyed out or shows "Not connected"
- **Probing**: shows a spinner or "Identifying receiver..."
- **Identified**: shows receiver details
- **Error**: shows error message from failed probe

No detect button — the panel is purely display.

### 3. Frontend: panel placement
Place the receiver panel in the right column (or as a small info area near the connection strip). It should be visible by default but compact — this is reference information, not an interactive workflow.

## Result
When the user connects to a receiver, the app automatically identifies it within a few seconds and shows receiver identity. No manual action needed. Configuration readback is deferred to config-restructure.

## Files changed
- `desktop/app.go` (auto-probe in connect flow, `gps:receiver` event emission)
- `desktop/frontend/src/receiver-panel.tsx` (rewritten: auto-populated from event, no detect button)
- `desktop/frontend/src/app.tsx` (listen for `gps:receiver` event, pass state to receiver panel)

## Testing (Playwright)

### Without hardware
- Verify receiver panel exists and shows "Not connected" or equivalent when disconnected.
- Verify receiver panel shows "Identifying receiver..." or equivalent during connect.

### With hardware
- Connect to a receiver.
- Verify receiver panel populates with vendor, hardware, and firmware within a few seconds.
- Verify supported GNSS constellations are listed.
- Disconnect; verify panel returns to disconnected state.
- Reconnect; verify panel re-populates automatically.
