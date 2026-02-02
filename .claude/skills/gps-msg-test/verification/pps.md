# Verification: PPS Configuration

Applies to: `pps`, `pps-off`, `get-pps`

## Observable Effect Verification

**PPS output cannot be verified programmatically.** It requires physical measurement equipment:
- Oscilloscope
- Frequency counter
- Logic analyzer
- Another device with PPS input (e.g., PHC card with external timestamp)

The user must manually verify:
- PPS signal is present on the correct pin
- Pulse width matches configuration
- Polarity (rising/falling edge) matches configuration
- PPS only occurs when receiver has fix (if configured that way)

## Common PPS Parameters

- **Pulse width:** Typically 100ms (100000 microseconds) or 1ms
- **Polarity:** Rising edge (high pulse) or falling edge (low pulse)
- **Lock mode:** Output always, or only when receiver has fix
- **Time alignment:** GPS time, UTC, or specific constellation
