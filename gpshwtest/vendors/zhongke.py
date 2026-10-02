"""Zhongke: the CASIC message files and the CFG-RATE reply."""

from pathlib import Path
from typing import Any

from vendor import Vendor, received


class Zhongke(Vendor):
    def msg_file(self, receiver: dict[str, Any]) -> str | None:
        """The per-model file where there is one. V5 and V6 firmware
        differ too much to share a model file, so the firmware generation
        decides between the ATGM332D files; V5 units do not reliably
        report a hardware string (the ATGM332D-5N71's is empty). The
        AT632-6T-30 reports itself as AT362. The AT372-6P has no model
        file and gets the family's shared one."""
        hw = str(receiver.get("hardware", ""))
        fw = str(receiver.get("firmware", ""))
        if fw.startswith("URANUS5"):
            return "zhongke/atgm332d-v5.toml"
        if hw.startswith("AT362-"):
            return "zhongke/at632.toml"
        if hw.startswith("ATGM332D-"):
            return "zhongke/atgm332d-v6.toml"
        if hw.startswith("AT372-"):
            return "zhongke/casic.toml"
        return None

    def fix_interval(self, log: Path) -> float | None:
        """The U2 interval in ms leading a CFG-RATE (06 04) payload; the
        rest of the payload differs between V5 and V6 but this does not."""
        for e in received(log):
            h = e.get("bin")
            if not isinstance(h, str):
                continue
            try:
                b = bytes.fromhex(h)
            except ValueError:
                continue
            if len(b) >= 10 and b[:2] == b"\xBA\xCE" and b[4:6] == b"\x06\x04":
                return int.from_bytes(b[6:8], "little") / 1000
        return None


VENDOR = Zhongke()
