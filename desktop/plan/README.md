# Desktop app implementation plan

## Architecture documents
- [backend.md](backend.md) - Backend-frontend message architecture (event streams, API calls, responsibilities split)
- [ui-workspace-panels.md](ui-workspace-panels.md) - Top-level workspace layout and panel model

## Panel specifications
Each panel has a design section (what it does) and an implementation section (how to build it):
- [ui-panel-connection.md](ui-panel-connection.md) - Connection strip (device, speed, connect/disconnect)
- [ui-panel-configuration.md](ui-panel-configuration.md) - Configuration panel (properties, messages, operations)
- [ui-panel-logging.md](ui-panel-logging.md) - Logging panel (structured logs, filtering, progress)
- [ui-panel-packet-monitor.md](ui-panel-packet-monitor.md) - Packet monitor (raw packet diagnostics)
- [ui-panel-sky-view.md](ui-panel-sky-view.md) - Sky view (polar satellite plot)
- [ui-panel-signal-view.md](ui-panel-signal-view.md) - Signal view (CN0 bar graph)

## Implementation phases
Each phase is self-contained with goal, steps, testing plan, and files changed.

| Phase | Summary | Key dependencies |
|-------|---------|-----------------|
| [phase1.md](phase1.md) | Panel layout shell (`react-resizable-panels`) | None |
| [phase2.md](phase2.md) | Structured logging | Phase 1 |
| [phase3.md](phase3.md) | Semantic data stream + text panels (time, survey) | Phase 1 |
| [phase4.md](phase4.md) | Receiver panel (auto-identify on connect) | Phase 1 |
| [phase5a.md](phase5a.md) | Config panel restructure, readback, validation (done) | Phase 1, Phase 4 |
| [phase5b.md](phase5b.md) | Layout rework (slide-down config, collapsible panels) | Phase 5a |
| [phase5c.md](phase5c.md) | Message configuration (NMEA/RTCM/binary output control) | Phase 5a, Phase 5b |
| [phase5d.md](phase5d.md) | Live messages panel (packet stats tree + decode modal) | Phase 5b |
| [phase6a.md](phase6a.md) | Signal graph (per-constellation, per-signal vertical bars) | Phase 3, Phase 5b |
| [phase6b.md](phase6b.md) | Sky view (polar satellite plot) | Phase 6a |
| [phase6c.md](phase6c.md) | Monitor tab layout rework (compact time/survey, flex layout) | Phase 6a, Phase 6b, Phase 5d |
| [phase7.md](phase7.md) | Message file support (TOML message send, response formatting) | Phase 1 |
| [phase8.md](phase8.md) | Serial port enumeration (dropdown selector) | Phase 5b |
| [phase9.md](phase9.md) | Windows support (Win32 serial I/O, Wails build) | Phase 8 |

## Dependency graph
```
Phase 1 (layout shell)
├── Phase 2 (structured logging)
├── Phase 3 (semantic stream + text panels)
│   └── Phase 6a (signal graph)
│       └── Phase 6b (sky view)
│           └── Phase 6c (monitor layout rework) ← also depends on Phase 5d
├── Phase 4 (receiver panel)
│   └── Phase 5a (config panel restructure) ✓
│       └── Phase 5b (layout rework)
│           ├── Phase 5c (message configuration)
│           └── Phase 5d (live messages panel)
├── Phase 7 (message file support)
├── Phase 8 (serial port enumeration)
│   └── Phase 9 (Windows support)
```

## Current state
The existing app (`desktop/`) is a working Wails v2 prototype with:
- Tab-based UI (Preact + TypeScript + Vite + Tailwind CSS 4.x)
- Serial connection, packet streaming, basic configuration
- See `desktop/TODO.md` for known API gaps and refactoring needs
