# Allystar message files

Select the file for the receiver family:

- `tau1201.toml` configures TAU1201 receivers. It includes the common
  commands from `asbin.toml` and uses CFG-MSG divisor 1 for 1Hz output.
- `tau13xx.toml` configures TAU13xx receivers. It includes `tau1201.toml`
  and adds RTCM output commands.
- `tau951m.toml` configures TAU951M-P2 and TAU951M-K2 receivers. It includes
  the common commands from `asbin.toml` and uses CFG-MSG divisor 5 for 1Hz
  output, including RTCM.

`asbin.toml` contains the rate-independent Allystar binary commands shared
by the model-family files. Use a model-family file rather than selecting it
directly when configuring periodic message output.
