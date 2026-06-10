"""Probes for scalar configuration properties.

Each probe sets a property, reads the achieved value from the set
invocation, and confirms it with an independent readback. Disagreement
between the two, or state changed by a reported error, is a failure
(a tool-guarantee violation); everything else - refusals, quantization,
clipping - is recorded as an observation for the characterization.
"""

from dataclasses import dataclass, field
from typing import Any, Callable

from tool import Tool

Value = Any


@dataclass
class Observation:
    """One probe outcome in device-independent terms."""

    prop: str
    requested: Value
    error: str | None
    reported: Value
    readback: Value


def fmt_value(v: Value) -> str:
    """Format a model value as a satpulsetool command-line argument."""
    if isinstance(v, float):
        s = f"{v:.9f}".rstrip("0").rstrip(".")
        return s if s else "0"
    return str(v)


def fmt_nanos(v: Value) -> str:
    """Render a model value in seconds as integer nanoseconds for the CLI."""
    return str(round(float(v) * 1e9))


@dataclass
class ScalarProp:
    """A property settable by one flag and readable at one config JSON path.

    Probe values are in model units (the units of the config JSON); to_cli
    renders a model value as the flag's argument where the flag uses
    different units.
    """

    name: str
    flag: str
    values: list[Value]
    path: tuple[str, ...]
    to_cli: Callable[[Value], str] = fmt_value


PROPS = [
    ScalarProp("antennaCableDelay", "--ant-cable-delay", [1e-9, 1.23e-7, 3.2767e-5],
               ("antennaCableDelay",), to_cli=fmt_nanos),
    ScalarProp("minElevation", "--min-elev", [1, 7, 45], ("minElevation",)),
    ScalarProp("rtcmBaseID", "--rtcm-base-id", [1, 1234, 4095], ("rtcmBaseID",)),
    ScalarProp("timeGNSS", "--time-gnss", ["GAL", "BDS", "GLO", "GPS"], ("timeGNSS",)),
    ScalarProp("timePulse.width", "--pps", [0.25, 0.000123456, 0.1], ("timePulse", "width")),
]


def config_value(cfg: dict[str, Any], path: tuple[str, ...]) -> Value:
    """Extract a value from the config JSON object, None if absent."""
    v: Value = cfg
    for k in path:
        if not isinstance(v, dict):
            return None
        v = v.get(k)
    return v


@dataclass
class ProbeRun:
    """Drives probes against one receiver, collecting observations and failures."""

    tool: Tool
    observations: list[Observation] = field(default_factory=list)
    failures: list[str] = field(default_factory=list)

    def show_config(self, name: str) -> dict[str, Any]:
        """Read the full current configuration in a separate invocation."""
        inv = self.tool.gps(name, ["--show-config"])
        if inv.error is not None:
            self.failures.append(f"{name}: --show-config failed: {inv.error}")
        return inv.config()

    def probe_scalar(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Probe each value of p, then restore its initial value."""
        prev = config_value(initial, p.path)
        for v in p.values:
            inv = self.tool.gps(f"set-{p.name}", [p.flag, p.to_cli(v)])
            reported = config_value(inv.config(), p.path)
            back = config_value(self.show_config(f"readback-{p.name}"), p.path)
            self.observations.append(Observation(p.name, v, inv.error, reported, back))
            if inv.error is not None:
                if back != prev:
                    self.failures.append(
                        f"{p.name}: refusal of {v!r} changed state: {prev!r} -> {back!r}")
            else:
                if reported != back:
                    self.failures.append(
                        f"{p.name}: reported {reported!r} but readback says {back!r}")
                prev = back
        self.restore(p, initial)

    def restore(self, p: ScalarProp, initial: dict[str, Any]) -> None:
        """Set p back to its value in the initial configuration."""
        v = config_value(initial, p.path)
        if v is None:
            return
        inv = self.tool.gps(f"restore-{p.name}", [p.flag, p.to_cli(v)])
        if inv.error is not None:
            self.failures.append(f"{p.name}: restore to {v!r} failed: {inv.error}")
            return
        back = config_value(self.show_config(f"verify-restore-{p.name}"), p.path)
        if back != v:
            self.failures.append(f"{p.name}: restore to {v!r} read back as {back!r}")
