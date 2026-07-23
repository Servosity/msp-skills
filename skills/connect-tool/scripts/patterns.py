# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""Mine the structured audit log for recurring failures → propose generic lessons.

The third job of the audit trail (improvement). Reads events.jsonl across all
runs under ~/.config/connect-tool/runs/, finds error_class values that recur for
the same target/scheme, and proposes them as connect-tool lessons. Proposals are
human-ratified: --record writes them via learning.py (scope=global), else they
just print.

  uv run patterns.py [--threshold 2]
  uv run patterns.py --record           # write proposals as global lessons
  uv run patterns.py --selfcheck
"""
from __future__ import annotations

import argparse
import collections
import json
import os
import pathlib
import sys

try:
    import learning as _learning  # type: ignore[reportMissingImports]  # sibling, on sys.path at runtime
except ImportError:  # pragma: no cover
    _learning = None


def runs_dir() -> pathlib.Path:
    return pathlib.Path(os.environ.get(
        "CONNECT_TOOL_RUNS_DIR", pathlib.Path.home() / ".config/connect-tool/runs"))


def _iter_events(root: pathlib.Path):
    if not root.exists():
        return
    for ev_file in root.glob("*/events.jsonl"):
        for line in ev_file.read_text().splitlines():
            line = line.strip()
            if line:
                try:
                    yield json.loads(line)
                except json.JSONDecodeError:
                    continue


def find_patterns(root: pathlib.Path, threshold: int = 2) -> list[dict]:
    counts: collections.Counter = collections.Counter()
    for ev in _iter_events(root):
        ec = ev.get("error_class")
        if ec and ev.get("status") in (None, "fail", "error", "retry"):
            counts[(ev.get("target", "?"), ev.get("scheme", "?"), ec)] += 1
    proposals = []
    for (target, scheme, ec), n in counts.most_common():
        if n >= threshold:
            proposals.append({
                "target": target, "scheme": scheme, "error_class": ec, "count": n,
                "lesson": f"On {scheme} targets like {target}, expect '{ec}' "
                          f"(seen {n}x) - handle it proactively next setup.",
                "tags": sorted({t for t in (scheme, ec.replace('_', '-')) if t and t != '?'}),
            })
    return proposals


def _selfcheck() -> None:
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        root = pathlib.Path(td)
        run = root / "halopsa-2026-06-29-1200"
        run.mkdir()
        evs = [
            {"target": "halopsa", "scheme": "api-key", "error_class": "console_eventual_consistency", "status": "retry"},
            {"target": "halopsa", "scheme": "api-key", "error_class": "console_eventual_consistency", "status": "retry"},
            {"target": "halopsa", "scheme": "api-key", "error_class": "selector_miss", "status": "fail"},
            {"target": "halopsa", "scheme": "api-key", "event": "verify", "status": "ok"},
        ]
        (run / "events.jsonl").write_text("\n".join(json.dumps(e) for e in evs) + "\n")
        props = find_patterns(root, threshold=2)
        assert len(props) == 1, props
        assert props[0]["error_class"] == "console_eventual_consistency", props
        assert props[0]["count"] == 2, props
    print("patterns.py selfcheck OK")


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--threshold", type=int, default=2)
    ap.add_argument("--record", action="store_true")
    ap.add_argument("--selfcheck", action="store_true")
    args = ap.parse_args(argv)

    if args.selfcheck:
        _selfcheck()
        return 0

    proposals = find_patterns(runs_dir(), args.threshold)
    if not proposals:
        print("no recurring failure patterns found")
        return 0
    for p in proposals:
        print(f"[{p['count']}x] {p['target']}/{p['scheme']}: {p['error_class']}")
        print(f"    proposed lesson: {p['lesson']}")
        if args.record:
            if _learning is None:
                print("    (cannot record: learning.py not importable)", file=sys.stderr)
            else:
                rec = _learning.record_lesson(p["lesson"], kind="anti-pattern",
                                              scope="global", tags=p["tags"])
                print(f"    recorded {rec['feedback_id']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
