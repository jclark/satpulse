"""Extract a gpshwtest run into the durable configuration packet corpus.

The output directory contains only numbered copies of satpulsetool gps
packet logs. gpshwtest metadata is used only to choose the logs and preserve
their order; it is not copied into the corpus.
"""

import argparse
import json
import shutil
import sys
from collections.abc import Iterable
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass
class Step:
    seq: int
    name: str
    intent: dict[str, Any]
    log: Path | None
    timeout: float | None


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("run_dir", type=Path,
                    help="gpshwtest run directory containing runs.jsonl")
    ap.add_argument("dest", type=Path,
                    help="destination corpus scenario directory")
    ap.add_argument("--force", action="store_true",
                    help="replace an existing non-empty destination")
    args = ap.parse_args()
    try:
        logs = packet_logs(args.run_dir)
        write_corpus(logs, args.dest, args.force)
    except CorpusError as e:
        print(f"extract-corpus: {e}", file=sys.stderr)
        return 2
    print(f"copied {len(logs)} packet logs to {args.dest}", file=sys.stderr)
    return 0


class CorpusError(Exception):
    """A run directory cannot be converted into a corpus scenario."""


def packet_logs(run_dir: Path) -> list[Path]:
    steps = list(read_steps(run_dir))
    logs: list[Path] = []
    for s in steps:
        if s.timeout is not None:
            raise CorpusError(f"{s.name}: timed out; refusing incomplete run")
        if s.log is None or s.intent.get("role") == "setup":
            continue
        logs.append(s.log)
    if not logs:
        raise CorpusError(f"{run_dir}: no packet logs found")
    return logs


def read_steps(run_dir: Path) -> Iterable[Step]:
    runs = run_dir / "runs.jsonl"
    try:
        lines = runs.read_text(encoding="utf-8").splitlines()
    except OSError as e:
        raise CorpusError(f"{runs}: {e}") from e
    for i, line in enumerate(lines, 1):
        if not line:
            continue
        try:
            v = json.loads(line)
        except ValueError as e:
            raise CorpusError(f"{runs}:{i}: invalid JSON: {e}") from e
        if not isinstance(v, dict):
            raise CorpusError(f"{runs}:{i}: expected JSON object")
        intent = v.get("intent")
        log = v.get("log")
        yield Step(seq=int(v.get("seq", 0)),
                   name=str(v.get("name", f"line-{i}")),
                   intent=intent if isinstance(intent, dict) else {},
                   log=run_dir / log if isinstance(log, str) else None,
                   timeout=float(v["timeout"]) if "timeout" in v else None)


def write_corpus(logs: list[Path], dest: Path, force: bool) -> None:
    for p in logs:
        validate_packet_log(p)
    parent = dest.parent
    parent.mkdir(parents=True, exist_ok=True)
    tmp = parent / f".{dest.name}.tmp"
    if tmp.exists():
        if not tmp.is_dir():
            raise CorpusError(f"{tmp}: exists and is not a directory")
        if not force:
            raise CorpusError(f"{tmp}: temporary directory already exists")
        shutil.rmtree(tmp)
    if dest.exists():
        if not dest.is_dir():
            raise CorpusError(f"{dest}: exists and is not a directory")
        if any(dest.iterdir()):
            if not force:
                raise CorpusError(f"{dest}: destination is not empty")
            shutil.rmtree(dest)
    tmp.mkdir()
    width = max(3, len(str(len(logs))))
    try:
        for i, src in enumerate(logs, 1):
            shutil.copyfile(src, tmp / f"{i:0{width}d}.jsonl")
        if dest.exists():
            dest.rmdir()
        tmp.rename(dest)
    except Exception:
        shutil.rmtree(tmp, ignore_errors=True)
        raise


def validate_packet_log(path: Path) -> None:
    try:
        with path.open("rb") as f:
            for i, line in enumerate(f, 1):
                if not line.strip():
                    continue
                try:
                    v = json.loads(line)
                except ValueError as e:
                    raise CorpusError(f"{path}:{i}: invalid JSON: {e}") from e
                if not isinstance(v, dict):
                    raise CorpusError(f"{path}:{i}: expected JSON object")
    except OSError as e:
        raise CorpusError(f"{path}: {e}") from e


if __name__ == "__main__":
    sys.exit(main())
