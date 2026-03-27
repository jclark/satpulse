The code in this directory is mainly tested using @internal/syncsim/sync_test.go. There is also a CLI for testing in @internal/syncsimcmd/syncsimcmd.go, which can be accessed as `satpulsetool syncsim`.
When changing files here, be sure to run tests there.

If you update a `toml:comment` tag or a default value in the config structs, also update the corresponding `description` or `default` in `configs/config-schema.json`.
