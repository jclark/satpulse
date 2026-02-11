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

## Implementation phases
Each phase is self-contained with goal, steps, testing plan, and files changed.

| Phase | Summary | Key dependencies |
|-------|---------|-----------------|
| [phase1.md](phase1.md) | Panel layout shell (done) | None |
| [phase2.md](phase2.md) | Structured logging (done) | Phase 1 |
| [phase3.md](phase3.md) | Semantic data stream + text panels (done) | Phase 1 |
| [phase4.md](phase4.md) | Receiver panel (done) | Phase 1 |
| [phase5a.md](phase5a.md) | Config panel restructure, readback, validation (done) | Phase 1, Phase 4 |
| [phase5b.md](phase5b.md) | Layout rework (done) | Phase 5a |
| [phase5c.md](phase5c.md) | Message configuration (done) | Phase 5a, Phase 5b |
| [phase5d.md](phase5d.md) | Live messages panel | Phase 5b |
| [phase5e.md](phase5e.md) | Time mode rework (done) | Phase 5a, Phase 5c |
| [phase5f.md](phase5f.md) | Section-based dirty tracking and discard (done) | Phase 5a |
| [phase6a.md](phase6a.md) | Signal graph | Phase 3, Phase 5b |
| [phase6b.md](phase6b.md) | Sky view | Phase 6a |
| [phase6c.md](phase6c.md) | Monitor tab layout rework | Phase 6a, Phase 6b, Phase 5d |
| [phase7.md](phase7.md) | Message file support | Phase 1 |
| [phase8.md](phase8.md) | Serial port enumeration (done) | Phase 5b |

## Dependency graph
```
Phase 1 (layout shell) -- done
├── Phase 2 (structured logging) -- done
├── Phase 3 (semantic stream + text panels) -- done
│   └── Phase 6a (signal graph)
│       └── Phase 6b (sky view)
│           └── Phase 6c (monitor layout rework) ← also depends on Phase 5d
├── Phase 4 (receiver panel) -- done
│   └── Phase 5a (config panel restructure) -- done
│       ├── Phase 5b (layout rework) -- done
│       │   ├── Phase 5c (message configuration) -- done
│       │   │   └── Phase 5e (time mode rework) -- done ← also depends on Phase 5a
│       │   └── Phase 5d (live messages panel)
│       └── Phase 5f (dirty tracking and discard) -- done
├── Phase 7 (message file support)
├── Phase 8 (serial port enumeration) -- done
```

## Current state
The existing app (`desktop/`) is a working Wails v2 prototype with:
- Tab-based UI (Preact + TypeScript + Vite + Tailwind CSS 4.x)
- Serial connection, packet streaming, basic configuration
- See [issues.md](issues.md) for known issues and improvement ideas
