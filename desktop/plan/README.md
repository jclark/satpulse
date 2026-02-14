# Desktop app implementation plan

## Architecture documents
- [backend.md](backend.md) - Backend-frontend message architecture (event streams, API calls, responsibilities split)
- [ui-workspace-panels.md](ui-workspace-panels.md) - Top-level workspace layout and panel model
- [message-semantics.md](message-semantics.md) - Message configuration flag semantics (what flags mean, how they map to ConfigOptions)

## Panel specifications
Each panel has a design section (what it does) and an implementation section (how to build it):
- [ui-panel-connection.md](ui-panel-connection.md) - Connection strip (device, speed, connect/disconnect)
- [ui-panel-configuration.md](ui-panel-configuration.md) - Configuration panel (properties, messages, operations)
- [ui-panel-logging.md](ui-panel-logging.md) - Logging panel (structured logs, filtering, progress)
- [ui-panel-packet-monitor.md](ui-panel-packet-monitor.md) - Packet monitor (raw packet diagnostics)
- [ui-panel-sky-view.md](ui-panel-sky-view.md) - Sky view (polar satellite plot)
- [ui-panel-signal-view.md](ui-panel-signal-view.md) - Signal view (CN0 bar graph)
- [ui-panel-message-file.md](ui-panel-message-file.md) - Message file panel (load, tag select, send, responses)

## Implementation plans -- to do

| Plan | Summary | Dependencies |
|------|---------|-------------|
| [live-messages.md](live-messages.md) | Live messages panel | layout-rework |
| [signal-strength.md](signal-strength.md) | Signal graph | semantic-stream, layout-rework |
| [sky-view.md](sky-view.md) | Sky view | signal-strength |
| [monitor-layout.md](monitor-layout.md) | Monitor tab layout rework | signal-strength, sky-view, live-messages |
| [msgfile-response.md](msgfile-response.md) | Message file response handling | msgfile-send |

## Implementation plans -- done

| Plan | Summary | Dependencies |
|------|---------|-------------|
| [layout-shell.md](layout-shell.md) | Panel layout shell | None |
| [structured-logging.md](structured-logging.md) | Structured logging | layout-shell |
| [semantic-stream.md](semantic-stream.md) | Semantic data stream + text panels | layout-shell |
| [receiver-info.md](receiver-info.md) | Receiver panel | layout-shell |
| [config-restructure.md](config-restructure.md) | Config panel restructure, readback, validation | layout-shell, receiver-info |
| [layout-rework.md](layout-rework.md) | Layout rework | config-restructure |
| [message-config.md](message-config.md) | Message configuration | config-restructure, layout-rework |
| [time-mode.md](time-mode.md) | Time mode rework | config-restructure, message-config |
| [dirty-tracking.md](dirty-tracking.md) | Section-based dirty tracking and discard | config-restructure |
| [port-enumeration.md](port-enumeration.md) | Serial port enumeration | layout-rework |
| [msgfile-send.md](msgfile-send.md) | Message file send | layout-rework, msgfile MsgCount |
| [pvt-msgs-panel.md](pvt-msgs-panel.md) | PVT messages panel | layout-rework |

## Dependency graph
```
layout-shell -- done
├── structured-logging -- done
├── semantic-stream -- done
│   └── signal-strength
│       └── sky-view
│           └── monitor-layout ← also depends on live-messages
├── receiver-info -- done
│   └── config-restructure -- done
│       ├── layout-rework -- done
│       │   ├── message-config -- done
│       │   │   └── time-mode -- done ← also depends on config-restructure
│       │   └── live-messages
│       │   └── pvt-msgs-panel -- done
│       └── dirty-tracking -- done
├── msgfile-send -- done ← depends on layout-rework
│   └── msgfile-response
├── port-enumeration -- done
```

## Current state
The existing app (`desktop/`) is a working Wails v2 prototype with:
- Tab-based UI (Preact + TypeScript + Vite + Tailwind CSS 4.x)
- Serial connection, packet streaming, basic configuration
- See [issues.md](issues.md) for known issues and improvement ideas
