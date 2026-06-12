# Syncsim configurations

This directory contains configuration files for `satpulsetool syncsim`, which simulates a PHC being disciplined to GPS PPS using the `phcsync` controller.
The simulator can be used for tuning controller parameters (such as the PI servo gains `track.kp` and `track.ki` in the `[sync]` table) for specific hardware combinations without waiting hours or days for real hardware to settle.

## Files

The top-level files are complete, runnable simulation configurations:

| File | Hardware |
|------|----------|
| [cm4-f9t.syncsim.toml](cm4-f9t.syncsim.toml) | Raspberry Pi Compute Module 4 with u-blox ZED-F9T |
| [i225-m8t.syncsim.toml](i225-m8t.syncsim.toml) | Intel i225-T1 PCIe NIC with u-blox LEA-M8T |
| [timehat-f9t.syncsim.toml](timehat-f9t.syncsim.toml) | Time Appliances TimeHAT (TCXO) with u-blox ZED-F9T |

Each combines a PHC oscillator noise model, a GPS PPS error model, and `[sync]` controller parameters tuned for that hardware.

The [phc/](phc/) and [gps/](gps/) subdirectories contain the underlying noise parameters in isolation, derived from real hardware measurements. They are reference fragments; you can use them as starting points when composing a config for a hardware combination not represented above.

[syncsim-schema.json](syncsim-schema.json) is the JSON Schema for syncsim configurations. Editors that understand the `#:schema` comment (e.g. VS Code with Even Better TOML) will use it for completion and validation.

## Running a simulation

Run a single simulation from this directory:

```
make timehat-f9t.result.toml
```

Or run all configured simulations in parallel:

```
make -j
```

Each run produces four output files:

| Suffix | Contents |
|--------|----------|
| `.result.toml` | Final summary statistics in TOML format |
| `.syncsim.log` | Periodic stats and mode transitions (slog text) |
| `.ts.jsonl` | PHC timestamps in JSONL |
| `.clock.log` | Per-second clock offsets (space-separated text) |

The Makefile passes `-v` so the simulator emits INFO-level logging, and `--stats 600` so it writes a summary line every 600 simulated seconds.

## Reading the results

The most useful fields in `.result.toml` for comparing configurations are:

- **`absOffMax`** — the worst-case offset between the disciplined PHC and true time, in nanoseconds.
- **`trackingADev`** — the Allan deviation of the offset, a measure of frequency stability. 

## Tuning your own hardware

To explore different controller parameters for a hardware combination, copy one of the top-level configs and edit the `[sync]` block. For systematic sweeps, generate one config per parameter combination and use `make -j` to run them in parallel.
