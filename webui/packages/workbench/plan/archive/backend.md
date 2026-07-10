# Backend-Frontend Message Architecture Plan

## Goal
Define a minimal backend architecture for the desktop app where backend responsibilities are a thin Wails adapter over `gpsprot` and related packages.

## Core Principle
The backend should be extremely simple:
- adapt package-level Go interfaces/types to Wails transport,
- stream runtime data to frontend,
- expose request/response API methods,
- avoid UI policy and avoid CLI flag semantics.

## Communication Model
There are two communication styles only:
- push streams (events),
- request/response API calls (Wails-bound methods).

## Push Streams

### 1) Packet stream
Purpose:
- packet monitor and low-level diagnostics.

Source:
- receiver IO path (raw packets before/after decode).

Payload shape (example fields):
- `ts`
- `direction` (`in`/`out`)
- `protocol`
- `raw` (text/hex/bytes encoding suitable for frontend rendering)
- `summary` (optional short decoded tag)

Notes:
- this stream is transport/protocol oriented.
- it is not semantically interpreted for UI.

### 2) Logging stream (`slog`)
Purpose:
- operational visibility, progress reporting, diagnostics.

Source:
- backend logger output.

Payload shape (structured log record):
- `time`
- `level`
- `message`
- `logger`
- `attrs` (key/value map)

Notes:
- this is structured, not flattened text.
- frontend can filter semantically by level/component/attrs.

### 3) Semantic `*Msg` stream
Purpose:
- UI-facing GNSS data at `gpsprot` semantic level.

Source:
- generated on backend by decoding/dispatching packets into `*Msg` values (`gps/gpsprot/msg.go`).

Payload shape:
- `ts`
- `kind` (semantic type)
- `msg` (DTO equivalent of the corresponding `*Msg`)

Notes:
- this stream is derived from packet stream processing.
- frontend uses this for sky/signal/time/survey views.

## API Calls (Request/Response)
These are synchronous/asynchronous Wails methods, not streams.

### Configuration API
A single `Configure` method handles all configuration operations (probe, readback, apply, save, reset).
- input: map representing `gpsprot.ConfigTarget` fields (probe, get, props, opts).
- backend maps input -> `gpsprot.ConfigTarget`.
- backend executes via `gpscfg.Configure(...)`.
- output: result summary + receiver info + readback data + structured errors/warnings.

The frontend decides what to put in the map. Different operations are just different field combinations — there is no need for separate methods.

### Geopos utility API
Examples:
- ECEF -> lat/lon/height.
- lat/lon/height -> ECEF.

Implementation:
- thin adapter over `geopos` package functions.

### Other utility/control APIs
As needed, same rule:
- small DTO boundary,
- delegate to package code,
- no UI policy in backend.

## Wails Transport Boundary

### Event emission
- backend emits the three streams via Wails event transport.
- event names and DTO schemas are versioned in code/docs.

### Method binding
- backend exposes API calls as bound methods.
- DTOs are stable and independent of frontend component structure.

## Responsibilities Split

### Backend owns
- IO, packet capture, decode/dispatch, log emission.
- package invocation (`gpsprot`, `geopos`, etc).
- DTO mapping at transport boundary.

### Frontend owns
- panel layout and interaction behavior.
- presentation decisions and filtering UX.
- local view-model/state derivation from streams.

## Non-goals
- No backend expression of CLI option model (`gpsflags`) in this architecture.
- No backend-driven UI behavior specification.
- No duplication of domain logic that already exists in `gpsprot`/`geopos`.
