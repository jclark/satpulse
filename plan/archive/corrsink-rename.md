# Rename corrsink to stream

Rename `gps/app/corrsink` to `gps/app/stream`.  This is a pure
rename with no behavioral changes.

Type renames:

- `Sink` → `Pull`
- `NewSink` → `NewPull`

Everything else stays as-is: `Source`, `TCPSource`,
`PacketWriter`, `State`, `pruningQueue`, and all internal methods.

Steps:

1. `git mv gps/app/corrsink gps/app/stream`.
2. Change `package corrsink` to `package stream` in both files.
3. Rename `Sink` → `Pull`, `NewSink` → `NewPull` in
   `stream.go` and `stream_test.go`.
4. Update all imports from `gps/app/corrsink` to `gps/app/stream`
   and update call sites (`corrsink.NewSink` → `stream.NewPull`,
   etc.).
5. Rename source files: `corrsink.go` → `pull.go`,
   `corrsink_test.go` → `pull_test.go`.
6. `make test` to verify.
