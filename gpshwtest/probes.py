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


@dataclass
class ModeCase:
    """One positioning-mode request: flags plus the mode fields it implies.

    Request keys use the mode JSON vocabulary, flattened (survey.* fields
    exist in the model but not in the readable mode object). Values have
    more decimals than any receiver resolution so quantization shows.
    The fixed positions are an arbitrary plausible point; receivers store
    the position without checking it against the actual location.
    """

    name: str
    args: list[str]
    request: dict[str, Value]


MODE_CASES = [
    ModeCase("survey", ["--survey", "--survey-time", "300", "--survey-acc", "2.345"],
             {"static": True, "survey.minDuration": 300, "survey.accLimit": 2.345}),
    ModeCase("fixed-llh",
             ["--fixed-pos-llh", "13.7318284567,100.6447407891,12.34567",
              "--fixed-pos-acc", "0.12345"],
             {"static": True, "fixedPosLLH[0]": 13.7318284567,
              "fixedPosLLH[1]": 100.6447407891, "height": 12.34567,
              "fixedPosAcc": 0.12345}),
    ModeCase("fixed-ecef",
             ["--fixed-pos-ecef", "-1132881.12345,6092270.56789,1504542.90123",
              "--fixed-pos-acc", "1.23456"],
             {"static": True, "fixedPosECEF[0]": -1132881.12345,
              "fixedPosECEF[1]": 6092270.56789, "fixedPosECEF[2]": 1504542.90123,
              "fixedPosAcc": 1.23456}),
    ModeCase("mobile", ["--mobile"], {"static": False}),
]


def config_value(cfg: dict[str, Any], path: tuple[str, ...]) -> Value:
    """Extract a value from the config JSON object, None if absent."""
    v: Value = cfg
    for k in path:
        if not isinstance(v, dict):
            return None
        v = v.get(k)
    return v


def flat_value(obj: Value, key: str) -> Value:
    """Extract a value by flattened key like "fixedPosLLH[0]" or "survey.minDuration"."""
    cur = obj
    for part in key.split("."):
        name, _, idx = part.partition("[")
        if not isinstance(cur, dict):
            return None
        cur = cur.get(name)
        if idx:
            i = int(idx.rstrip("]"))
            if not isinstance(cur, list) or i >= len(cur):
                return None
            cur = cur[i]
    return cur


def mode_args(mode: dict[str, Any]) -> list[str]:
    """Build the flags that reproduce a mode readback. Survey parameters are
    not readable, so a surveyed mode is restored with default survey settings."""
    if not mode.get("static"):
        return ["--mobile"]
    args: list[str]
    if "fixedPosECEF" in mode:
        x, y, z = mode["fixedPosECEF"]
        args = ["--fixed-pos-ecef", f"{x},{y},{z}"]
    elif "fixedPosLLH" in mode:
        lat, lon = mode["fixedPosLLH"]
        args = ["--fixed-pos-llh", f"{lat},{lon},{mode.get('height', 0)}"]
    else:
        args = ["--survey"]
    if "fixedPosAcc" in mode:
        args += ["--fixed-pos-acc", fmt_value(mode["fixedPosAcc"])]
    return args


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

    def probe_modes(self, initial: dict[str, Any]) -> None:
        """Probe each positioning-mode case, then restore the initial mode."""
        prev = config_value(initial, ("mode",))
        for case in MODE_CASES:
            inv = self.tool.gps(f"set-mode-{case.name}", case.args)
            reported = config_value(inv.config(), ("mode",))
            back = config_value(self.show_config(f"readback-mode-{case.name}"), ("mode",))
            if inv.error is not None:
                for k, v in case.request.items():
                    self.observations.append(Observation(f"mode.{k}", v, inv.error, None, None))
                if back != prev:
                    self.failures.append(
                        f"mode {case.name}: refusal changed state: {prev!r} -> {back!r}")
                continue
            if reported != back:
                self.failures.append(
                    f"mode {case.name}: reported {reported!r} but readback says {back!r}")
            for k, v in case.request.items():
                self.observations.append(Observation(
                    f"mode.{k}", v, None, flat_value(reported, k), flat_value(back, k)))
            prev = back
        self.restore_mode(initial)

    def restore_mode(self, initial: dict[str, Any]) -> None:
        """Set the positioning mode back to its initial readback."""
        mode = config_value(initial, ("mode",))
        if not isinstance(mode, dict):
            return
        inv = self.tool.gps("restore-mode", mode_args(mode))
        if inv.error is not None:
            self.failures.append(f"mode: restore to {mode!r} failed: {inv.error}")
            return
        back = config_value(self.show_config("verify-restore-mode"), ("mode",))
        if back != mode:
            self.failures.append(f"mode: restore to {mode!r} read back as {back!r}")

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
