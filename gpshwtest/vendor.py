"""Vendor plugins: the receiver-specific knowledge gpshwtest cannot avoid.

Some probes need things high-level configuration has no vocabulary for,
such as setting the fix rate or finding which port the session is on.
They use the receiver's shipped low-level message file, and how to pick
that file and read the replies to its tags is per-vendor knowledge. It
lives in one plugin module per vendor, vendors/<name>.py, where name is
the reported vendor string lower-cased with everything but letters and
digits removed (u-blox is vendors/ublox.py). A vendor without a plugin
gets the defaults below, which know nothing; the probes that need a
plugin are then skipped.
"""

import importlib
import json
import re
from pathlib import Path
from typing import Any

from tool import Tool


class Vendor:
    """The hooks a vendor plugin overrides. A plugin module defines a
    subclass and exports an instance of it as VENDOR."""

    def msg_file(self, receiver: dict[str, Any]) -> str | None:
        """The shipped low-level message file for the receiver identified
        by a --show-receiver receiver object, as a path relative to the
        gpsmsg directory (e.g. "quectel/lg290p.toml"), or None when there
        is none."""
        return None

    def fix_interval(self, log: Path) -> float | None:
        """The fix interval in seconds reported in the packet log of a
        get-fix-rate invocation, or None when the reply is not there."""
        return None

    def active_port(self, tool: Tool, mf: Path) -> str | None:
        """Which receiver port this session is connected to, as the port
        part of the message file's speed-<baud>-<port> tags, or None when
        it cannot be determined. Any query goes through tool, so it is
        logged like every other step."""
        return None


def vendor_module_name(vendor: str) -> str:
    """The plugin module name for a reported vendor string."""
    return re.sub(r"[^a-z0-9]", "", vendor.lower())


def load_vendor(receiver: dict[str, Any]) -> Vendor:
    """The plugin for the vendor of a --show-receiver receiver object, or
    the default hooks when the vendor has no plugin."""
    name = vendor_module_name(str(receiver.get("vendor", "")))
    if not name:
        return Vendor()
    mod_name = f"vendors.{name}"
    try:
        mod = importlib.import_module(mod_name)
    except ModuleNotFoundError as e:
        if e.name != mod_name:
            raise
        return Vendor()
    v = getattr(mod, "VENDOR")
    assert isinstance(v, Vendor), f"{mod_name}.VENDOR is not a Vendor"
    return v


def received(log: Path) -> list[dict[str, Any]]:
    """The inbound entries of a packet log, empty when it cannot be read.
    For plugins parsing replies to their tags."""
    try:
        entries = [json.loads(line) for line in log.read_text().splitlines()]
    except (OSError, ValueError):
        return []
    return [e for e in entries if isinstance(e, dict) and not e.get("out")]
