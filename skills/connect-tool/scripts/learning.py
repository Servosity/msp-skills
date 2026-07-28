# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""connect-tool's feedback loop: lessons learned on one target help the next one.

Everything is written to ONE shared append-only JSONL substrate,
`~/.claude/learning/feedback.jsonl` (override with $LEARNING_PRIMITIVE_STORE), so
generic auth-setup lessons (scope negotiation, token storage, nav discovery,
"never pbpaste a secret") compound across EVERY target, and scope=global lessons
are readable by any other skill that reads the same file.

  uv run learning.py guidance [--target halopsa]     # inject prior lessons before driving
  uv run learning.py record --lesson "..." [--kind correction] [--tags oauth2,halopsa] [--global]
  uv run learning.py --selfcheck

Record shape (one JSON object per line):
  schema_version, feedback_id, ts, skill, scope, kind, lesson, trigger,
  correction, transform, tags, source, applied

`feedback_id` is a stable hash of (skill, kind, lesson), so re-recording the same
lesson is idempotent and the newest line per id wins on read.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import pathlib
import sys
from datetime import datetime, timezone
from typing import Any

import ctplatform as ct

SKILL = "connect-tool"
SCHEMA_VERSION = 1
KINDS = ("rule", "correction", "preference", "anti-pattern", "voice-pattern")
DEFAULT_STORE = pathlib.Path.home() / ".claude" / "learning" / "feedback.jsonl"


def store_path() -> pathlib.Path:
    env = os.environ.get("LEARNING_PRIMITIVE_STORE")
    return pathlib.Path(env).expanduser() if env else DEFAULT_STORE


def _now_iso() -> str:
    return datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def _feedback_id(skill: str, kind: str, lesson: str) -> str:
    h = hashlib.sha1(f"{skill}|{kind}|{lesson}".encode("utf-8")).hexdigest()
    return f"fb-{h[:12]}"


def _read_latest() -> dict[str, dict[str, Any]]:
    """Collapse the append-only log to the newest record per feedback_id."""
    p = store_path()
    latest: dict[str, dict[str, Any]] = {}
    if not p.exists():
        return latest
    with p.open("r", encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            fid = rec.get("feedback_id")
            if fid:
                latest[fid] = rec  # later lines win (the file is chronological)
    return latest


def _same(a: dict[str, Any], b: dict[str, Any]) -> bool:
    ignore = {"ts"}
    return {k: v for k, v in a.items() if k not in ignore} == {
        k: v for k, v in b.items() if k not in ignore
    }


def record_lesson(lesson: str, *, kind: str = "correction", scope: str = "skill",
                  tags: list[str] | None = None, trigger: str | None = None) -> dict[str, Any]:
    """Capture one auth-setup lesson. Idempotent on (skill, kind, lesson)."""
    if kind not in KINDS:
        raise SystemExit(f"unknown kind {kind!r}; expected one of {KINDS}")
    # A lesson is free text the agent wrote, and it gets injected into future
    # runs' context. So it gets the same value-shape check as the state and audit
    # logs: a lesson that carries a credential is refused, not persisted.
    if ct.looks_secret(lesson) or ct.looks_secret(" ".join(tags or [])):
        raise SystemExit("refusing to record a lesson that looks like it contains a secret")
    fid = _feedback_id(SKILL, kind, lesson)
    rec: dict[str, Any] = {
        "schema_version": SCHEMA_VERSION,
        "feedback_id": fid,
        "ts": _now_iso(),
        "skill": SKILL,
        "scope": scope,
        "kind": kind,
        "lesson": lesson,
        "trigger": trigger,
        "correction": None,
        "transform": None,
        "tags": sorted(set(tags)) if tags else [],
        "source": "user",
        "applied": False,
    }
    existing = _read_latest().get(fid)
    if existing is not None and _same(existing, rec):
        return existing
    p = store_path()
    p.parent.mkdir(parents=True, exist_ok=True)
    with p.open("a", encoding="utf-8") as fh:
        fh.write(json.dumps(rec, ensure_ascii=False) + "\n")
    return rec


def gather_guidance(target: str | None = None) -> str:
    """Prior connect-tool lessons + every global lesson, most recent first.

    We do NOT tag-filter to the target: generic lessons are the whole point, so a
    NinjaOne run still benefits from a lesson first learned on HaloPSA. The target
    is only used to label the header.
    """
    recs = [r for r in _read_latest().values()
            if r.get("skill") == SKILL or r.get("scope") == "global"]
    recs.sort(key=lambda r: r.get("ts", ""), reverse=True)
    if not recs:
        return ""
    lines = [f"Prior connect-tool auth lessons to apply ({target or 'all targets'}):"]
    for r in recs:
        tag = f" [{', '.join(r['tags'])}]" if r.get("tags") else ""
        lines.append(f"- ({r.get('kind', 'correction')}){tag} {r.get('lesson', '').strip()}")
    return "\n".join(lines)


def _selfcheck() -> None:
    import tempfile
    with tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False) as tf:
        path = tf.name
    try:
        os.environ["LEARNING_PRIMITIVE_STORE"] = path
        record_lesson("Never pbpaste a secret, read the DOM innerText.",
                      kind="anti-pattern", scope="global", tags=["security", "browser"])
        record_lesson("HaloPSA API keys live under Configuration > Integrations > API.",
                      kind="correction", tags=["halopsa", "nav-discovery"])
        g = gather_guidance("halopsa")
        assert "pbpaste" in g and "HaloPSA" in g, g
        # idempotent: the same lesson twice does not duplicate
        before = len(_read_latest())
        record_lesson("HaloPSA API keys live under Configuration > Integrations > API.",
                      kind="correction", tags=["halopsa", "nav-discovery"])
        assert len(_read_latest()) == before
        # a global lesson recorded by ANOTHER skill is still surfaced
        with open(path, "a", encoding="utf-8") as fh:
            fh.write(json.dumps({"feedback_id": "fb-other", "ts": "2026-01-01T00:00:00Z",
                                 "skill": "some-other-skill", "scope": "global",
                                 "kind": "rule", "lesson": "Verify by use.", "tags": []}) + "\n")
        assert "Verify by use." in gather_guidance()
        # ... and a skill-scoped lesson from another skill is NOT
        with open(path, "a", encoding="utf-8") as fh:
            fh.write(json.dumps({"feedback_id": "fb-priv", "ts": "2026-01-01T00:00:00Z",
                                 "skill": "some-other-skill", "scope": "skill",
                                 "kind": "rule", "lesson": "Not mine.", "tags": []}) + "\n")
        assert "Not mine." not in gather_guidance()
        # a lesson (or a tag) that carries a credential is refused, not stored.
        # Built at runtime so this file holds no credential-shaped literal.
        tok = "sk" + "_live_abcdefghijklmnop"
        before = len(_read_latest())
        cases: list[tuple[str, list[str]]] = [
            (f"the key was {tok}", ["halopsa"]),   # secret in the lesson text
            ("the key is over here", [tok]),        # secret hidden in a tag
        ]
        for lesson_text, lesson_tags in cases:
            try:
                record_lesson(lesson_text, kind="correction", tags=lesson_tags)
                raise AssertionError("a secret-bearing lesson was recorded")
            except SystemExit:
                pass
        assert len(_read_latest()) == before, "a refused lesson still got written"
    finally:
        os.unlink(path)
    print("learning.py selfcheck OK (refuses a secret-bearing lesson)")


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd")

    g = sub.add_parser("guidance")
    g.add_argument("--target")

    r = sub.add_parser("record")
    r.add_argument("--lesson", required=True)
    r.add_argument("--kind", default="correction")
    r.add_argument("--tags", default="")
    r.add_argument("--trigger")
    r.add_argument("--global", dest="is_global", action="store_true")

    ap.add_argument("--selfcheck", action="store_true")
    args = ap.parse_args(argv)

    if args.selfcheck:
        _selfcheck()
        return 0
    if args.cmd == "guidance":
        print(gather_guidance(args.target))
        return 0
    if args.cmd == "record":
        tags = [t for t in args.tags.split(",") if t]
        rec = record_lesson(args.lesson, kind=args.kind, tags=tags, trigger=args.trigger,
                            scope="global" if args.is_global else "skill")
        print(f"recorded {rec['feedback_id']} ({rec['scope']}/{rec['kind']})")
        return 0
    ap.print_help()
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
