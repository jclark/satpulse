"""Unicore: the UM980/UM982 message files and the active-port query."""

import re
from pathlib import Path
from typing import Any

from tool import Tool
from vendor import Vendor, received

MSG_FILES = {"UM980": "unicore/um980.toml", "UM982": "unicore/um982.toml"}


class Unicore(Vendor):
    def msg_file(self, receiver: dict[str, Any]) -> str | None:
        return MSG_FILES.get(str(receiver.get("hardware", "")))

    def active_port(self, tool: Tool, mf: Path) -> str | None:
        """The port from the header of a long-format query response. The
        speed command must name the right port: the receiver happily
        reconfigures an unconnected one."""
        inv = tool.gps("query-active-port", ["-m", str(mf), "-t", "get-loglist"],
                       {"op": "session-speed", "role": "port-query"},
                       retry=False, json_out=False)
        for e in received(inv.packet_log):
            a = e.get("ascii", "")
            if isinstance(a, str) and a[:1] in "<#":
                m = re.search(r"\b(COM\d)\b", a)
                if m:
                    return m.group(1).lower()
        return None


VENDOR = Unicore()
