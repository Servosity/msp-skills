#!/usr/bin/env python3
"""The live-verification badge: flip one skill to "live-verified" or back.

A skill is "live-verified" once a real MSP has run it against a real tenant and
told us it worked - in a Build Session, on Circle, via a GitHub "it works"
report, or by our own dogfooding. That fact lives in the registry under each
skill's "live_verified" key:

    "live_verified": {
      "status": "live-verified" | "awaiting",
      "date": "YYYY-MM-DD",
      "source": "build-session" | "circle" | "github" | "self",
      "evidence": "<url or note>",
      "verified_by": "<who confirmed it, e.g. @handle (MSP)>" | null,
      "issue_url": "<clickable receipt link>" | null
    }

A skill without the key is treated as "awaiting" - tools tolerate its absence.
`verified_by` and `issue_url` are what the catalog + per-skill page actually
render (the attribution line and the "receipt ->" link); pass --verified-by /
--issue-url to set them. For a `--source github` flip whose --evidence is a
github.com URL, issue_url defaults to that URL so the receipt link is never
silently dropped (the gap that left sentinelone #116 reading "by Servosity
(maintainer)" with no receipt).

This is a DOCS-ONLY flip. It changes skills.json (which the catalog + per-skill
pages render from); it does NOT cut a release or push a tag. After a flip,
regenerate the catalog and commit.

Usage:
    python3 tools/maintainer/verify_live.py --slug halopsa --source build-session \\
        --evidence "https://circle.so/..." [--date 2026-06-04]
    python3 tools/maintainer/verify_live.py --slug cove --source github \\
        --evidence "https://github.com/Servosity/msp-skills/issues/139" \\
        --verified-by "@AvlCompCo (MSP)"   # issue_url defaults from --evidence
    python3 tools/maintainer/verify_live.py --revoke --slug halopsa
    python3 tools/maintainer/verify_live.py --status
    python3 tools/maintainer/verify_live.py --slug halopsa --source self \\
        --evidence "redogfood" --force

Pure stdlib.
"""

from __future__ import annotations

import datetime as dt
import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
REGISTRY = registry.REGISTRY

VALID_SOURCES = ("build-session", "circle", "github", "self")

AWAITING = {"status": "awaiting", "date": None, "source": None, "evidence": None}


def _read_registry() -> dict:
    """Read through registry.load(), so the identifier grammar is enforced on the
    way in. This script WRITES skills.json back; validating first means it can
    never round-trip a malformed identifier into the file the build matrix reads."""
    return registry.load()


def _write_registry(reg: dict) -> None:
    """Write skills.json preserving 2-space indent, trailing newline, and sorted
    slugs (the repo convention; matches release.py's _write_json + sorted keys)."""
    skills = reg.get("skills", {})
    reg["skills"] = {slug: skills[slug] for slug in sorted(skills)}
    REGISTRY.write_text(json.dumps(reg, indent=2) + "\n", encoding="utf-8")


def _entry_state(entry: dict) -> dict:
    """Return the live_verified sub-dict, defaulting to AWAITING when absent."""
    lv = entry.get("live_verified")
    if not isinstance(lv, dict):
        return dict(AWAITING)
    return lv


def cmd_status() -> int:
    reg = _read_registry()
    skills = {slug: reg["skills"][slug] for slug in sorted(reg.get("skills", {}))}

    headers = ("slug", "status", "date", "source", "evidence")
    rows = []
    for slug, entry in skills.items():
        # A markdown-only skill (e.g. the concierge) recommends + installs other
        # connectors; it has no tenant of its own and so cannot be live-verified.
        # Show "n/a" rather than "awaiting" so it never reads as a pending badge.
        if entry.get("markdown_only"):
            rows.append((slug, "n/a", "-", "-", "markdown-only (no tenant to verify)"))
            continue
        lv = _entry_state(entry)
        rows.append((
            slug,
            lv.get("status") or "awaiting",
            lv.get("date") or "-",
            lv.get("source") or "-",
            lv.get("evidence") or "-",
        ))

    widths = [len(h) for h in headers]
    for row in rows:
        for i, cell in enumerate(row):
            widths[i] = max(widths[i], len(str(cell)))

    def fmt(cells: tuple[str, ...]) -> str:
        return "  ".join(str(c).ljust(widths[i]) for i, c in enumerate(cells))

    print(fmt(headers))
    print(fmt(tuple("-" * w for w in widths)))
    for row in rows:
        print(fmt(row))
    return 0


def _print_followup(slug: str, source: str) -> None:
    print()
    print("Next (this command did NOT run these - it is docs-only, no release fires):")
    print("  python3 tools/maintainer/build-catalog.py")
    print(f'  git commit -am "badge: {slug} live-verified ({source})"')


def cmd_verify(
    slug: str,
    source: str,
    evidence: str,
    date: str | None,
    force: bool,
    verified_by: str | None = None,
    issue_url: str | None = None,
) -> int:
    reg = _read_registry()
    skills = reg.get("skills", {})
    if slug not in skills:
        print(f"verify_live: unknown slug '{slug}'", file=sys.stderr)
        return 1
    if skills[slug].get("markdown_only"):
        print(
            f"verify_live: refusing '{slug}' - it is markdown-only (no tenant to "
            "verify against). markdown-only skills are not live-verifiable.",
            file=sys.stderr,
        )
        return 1
    if source not in VALID_SOURCES:
        print(
            f"verify_live: --source must be one of {', '.join(VALID_SOURCES)}, "
            f"got '{source}'",
            file=sys.stderr,
        )
        return 1
    if not evidence:
        print("verify_live: --evidence is required (a URL or short note)", file=sys.stderr)
        return 1

    entry = skills[slug]
    current = _entry_state(entry)
    if current.get("status") == "live-verified" and not force:
        print(f"verify_live: '{slug}' is already live-verified.", file=sys.stderr)
        print(f"  date:     {current.get('date') or '-'}", file=sys.stderr)
        print(f"  source:   {current.get('source') or '-'}", file=sys.stderr)
        print(f"  evidence: {current.get('evidence') or '-'}", file=sys.stderr)
        print(
            "  Run with --revoke first, or pass --force to overwrite.",
            file=sys.stderr,
        )
        return 1

    use_date = date or dt.date.today().isoformat()
    # The catalog + per-skill page render `verified_by` (who confirmed it) and
    # `issue_url` (the clickable receipt). A GitHub "it works" report IS its own
    # receipt, so default issue_url to the evidence URL when it points at a
    # github.com issue/PR and the caller did not pass one explicitly.
    use_issue_url = issue_url
    if use_issue_url is None and source == "github" and evidence.startswith(
        "https://github.com/"
    ):
        use_issue_url = evidence
    entry["live_verified"] = {
        "status": "live-verified",
        "date": use_date,
        "source": source,
        "evidence": evidence,
        "verified_by": verified_by,
        "issue_url": use_issue_url,
    }
    _write_registry(reg)

    print(f"{slug}: live-verified ({source}, {use_date})")
    print(f"  evidence:    {evidence}")
    print(f"  verified_by: {verified_by or '(default: Servosity (maintainer))'}")
    print(f"  issue_url:   {use_issue_url or '(none)'}")
    _print_followup(slug, source)
    return 0


def cmd_revoke(slug: str) -> int:
    reg = _read_registry()
    skills = reg.get("skills", {})
    if slug not in skills:
        print(f"verify_live: unknown slug '{slug}'", file=sys.stderr)
        return 1
    # Absence of the key IS "awaiting" (tools tolerate it), so revoke removes the
    # key entirely rather than writing an explicit nulled block. This makes a
    # flip -> revoke round-trip a true no-op on skills.json for a skill that had
    # never been verified, leaving the registry byte-for-byte as it started.
    skills[slug].pop("live_verified", None)
    _write_registry(reg)
    print(f"{slug}: reverted to awaiting")
    print()
    print("Next (docs-only, no release fires):")
    print("  python3 tools/maintainer/build-catalog.py")
    print(f'  git commit -am "badge: {slug} reverted to awaiting"')
    return 0


def parse_args(argv: list[str]) -> dict:
    opts: dict = {
        "slug": None,
        "source": None,
        "evidence": None,
        "date": None,
        "verified_by": None,
        "issue_url": None,
        "revoke": False,
        "status": False,
        "force": False,
    }
    i = 0
    while i < len(argv):
        a = argv[i]
        if a == "--slug":
            i += 1
            opts["slug"] = argv[i]
        elif a == "--source":
            i += 1
            opts["source"] = argv[i]
        elif a == "--evidence":
            i += 1
            opts["evidence"] = argv[i]
        elif a == "--date":
            i += 1
            opts["date"] = argv[i]
        elif a == "--verified-by":
            i += 1
            opts["verified_by"] = argv[i]
        elif a == "--issue-url":
            i += 1
            opts["issue_url"] = argv[i]
        elif a == "--revoke":
            opts["revoke"] = True
        elif a == "--status":
            opts["status"] = True
        elif a == "--force":
            opts["force"] = True
        else:
            raise SystemExit(f"verify_live: unknown argument '{a}'")
        i += 1
    return opts


def main(argv: list[str]) -> int:
    opts = parse_args(argv)

    if opts["status"]:
        return cmd_status()

    if opts["revoke"]:
        if not opts["slug"]:
            raise SystemExit("verify_live: --revoke requires --slug")
        return cmd_revoke(opts["slug"])

    if opts["slug"]:
        if not opts["source"]:
            raise SystemExit("verify_live: a flip requires --source")
        return cmd_verify(
            opts["slug"], opts["source"], opts["evidence"], opts["date"], opts["force"],
            verified_by=opts["verified_by"], issue_url=opts["issue_url"],
        )

    raise SystemExit(
        "verify_live: nothing to do. Use --status, --slug ... --source ... "
        "--evidence ..., or --revoke --slug ..."
    )


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
