"""Quectel: the LG290P and LC29H message files and the PQTM fix-rate reply."""

import re
from pathlib import Path
from typing import Any

from vendor import Vendor, received

# Message files by hardware-name prefix. The backend's hardware name is the
# module name from PQTMVERNO, which carries a variant suffix the files do
# not (LG290P03).
MSG_FILES = {"LG290P": "quectel/lg290p.toml", "LC29H": "quectel/lc29h.toml"}


class Quectel(Vendor):
    def msg_file(self, receiver: dict[str, Any]) -> str | None:
        hw = str(receiver.get("hardware", ""))
        for prefix, mf in MSG_FILES.items():
            if hw.startswith(prefix):
                return mf
        return None

    def fix_interval(self, log: Path) -> float | None:
        for e in received(log):
            a = e.get("ascii")
            if isinstance(a, str):
                m = re.search(r"\bPQTMCFGFIXRATE,(?:(?:OK|R),)?(\d+)(?:[,*]|$)", a)
                if m:
                    return int(m.group(1)) / 1000
        return None


VENDOR = Quectel()
