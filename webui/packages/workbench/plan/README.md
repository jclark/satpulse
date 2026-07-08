# Desktop app implementation plan

## Planning -- to do
Needs implementation design before work can begin:
- [live-messages.md](live-messages.md) - Relocate to Packets tab (current plan targets Monitor tab; see [ui-monitor-tab.md](ui-monitor-tab.md))
- [ui-monitor-tab.md](ui-monitor-tab.md) - Monitor tab layout wiring (arranging nav-summary, map, scatter, sky view, signals, survey, PVT into the tab)

## Implementation plans -- to do

| Plan | Summary | Dependencies |
|------|---------|-------------|
| [signal-strength.md](signal-strength.md) | Signal graph | semantic-stream, layout-rework |
| [nav-summary.md](nav-summary.md) | Navigation summary panel | position/velocity messages, NavEpochMsg |
| [position-scatter.md](position-scatter.md) | Position scatter panel | position/velocity messages, NavEpochMsg |
| [msgfile-response.md](msgfile-response.md) | Message file response handling | msgfile-send |
| [shared-webui.md](shared-webui.md) | Shared Preact component library | semantic-tokens |
| [vrs.md](vrs.md) | Send position as NMEA to a VRS caster | vrs PR stack (#331-#333) landed and merged |

## Dependency graph
```
signal-strength
msgfile-send (done)
└── msgfile-response
nav-summary ← depends on position/velocity messages, NavEpochMsg
position-scatter ← depends on position/velocity messages, NavEpochMsg
semantic-tokens (done)
└── shared-webui
```

## Issues
See [issues.md](issues.md) for known issues and improvement ideas.

## Archive
Completed plans, architecture docs, and panel specs are in [archive/](archive/).
