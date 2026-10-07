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

Beta and live-verified are mutually exclusive. A flip also sets the registry
"status" to "live-verified" and rewrites the "## Status" paragraph of
skills/<slug>/README.md and pain-point.md; a revoke puts both back to Beta.
--check (run by ci_guards.sh) fails on any skill where they disagree, and
--sync repairs every one.

Usage:
    python3 tools/maintainer/verify_live.py --slug halopsa --source build-session \\
        --evidence "https://circle.so/..." [--date 2026-06-04]
    python3 tools/maintainer/verify_live.py --slug cove --source github \\
        --evidence "https://github.com/Servosity/msp-skills/issues/139" \\
        --verified-by "@AvlCompCo (MSP)"   # issue_url defaults from --evidence
    python3 tools/maintainer/verify_live.py --revoke --slug halopsa
    python3 tools/maintainer/verify_live.py --status
    python3 tools/maintainer/verify_live.py --check   # exit 1 on Beta/verified drift
    python3 tools/maintainer/verify_live.py --sync    # repair that drift
    python3 tools/maintainer/verify_live.py --slug halopsa --source self \\
        --evidence "redogfood" --force

Pure stdlib.
"""

from __future__ import annotations

import datetime as dt
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
REGISTRY = registry.REGISTRY

VALID_SOURCES = ("build-session", "circle", "github", "self")

AWAITING = {"status": "awaiting", "date": None, "source": None, "evidence": None}

RSVP = "RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions)."
STATUS_PARA = re.compile(r"(^## Status\n\n)(.+?)(?=\n\n|\n*\Z)", re.M | re.S)


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


def _verified(entry: dict) -> bool:
    return _entry_state(entry).get("status") == "live-verified"


def _status_para(entry: dict, readme: bool) -> str:
    """The Status paragraph README.md (readme=True) or pain-point.md must carry."""
    if _verified(entry):
        lv = _entry_state(entry)
        receipt = f" ([receipt]({lv['issue_url']}))" if lv.get("issue_url") else ""
        para = (f"Live-verified. Confirmed by {lv.get('verified_by') or 'a real MSP'} "
                f"against their own production tenant{receipt}.")
        return para + (f" More MSPs run it live in our weekly Build Sessions. {RSVP}" if readme else "")
    vendor = entry.get("vendor") or entry["system"]
    if readme:
        return (f"Beta. Validated against the {vendor} API surface and being validated with "
                "MSPs running it live against their own production tenant in our weekly "
                f"Build Sessions. {RSVP}")
    return (f"Beta. Validated against the {vendor} API surface; the closed-loop receipt (a "
            "named MSP running it live in their production tenant at a Build Session) is "
            "tracked separately and added here as `video.md` once it exists.")


def _drift(slug: str, entry: dict) -> list[str]:
    """Every place this skill claims Beta while verified, or verified while not."""
    verified = _verified(entry)
    out = []
    if (entry.get("status") == "live-verified") != verified:
        out.append(f"skills.json status is {entry.get('status')!r}")
    for name in ("README.md", "pain-point.md"):
        p = ROOT / "skills" / entry.get("source_dir", slug) / name
        m = p.exists() and STATUS_PARA.search(p.read_text(encoding="utf-8"))
        # ponytail: an existing "Live-verified..." paragraph is kept as-is, so
        # hand-written receipt detail (acronis #325) survives a --force re-flip.
        if m and m.group(2).startswith("Live-verified") != verified:
            out.append(f"{p.relative_to(ROOT)} Status says {m.group(2)[:14]!r}")
    return out


def _sync(slug: str, entry: dict) -> None:
    """Make the registry status + Status paragraphs agree with live_verified.
    Caller writes the registry."""
    verified = _verified(entry)
    entry["status"] = "live-verified" if verified else "beta"
    for name in ("README.md", "pain-point.md"):
        p = ROOT / "skills" / entry.get("source_dir", slug) / name
        if not p.exists():
            continue
        text = p.read_text(encoding="utf-8")
        m = STATUS_PARA.search(text)
        if m and m.group(2).startswith("Live-verified") != verified:
            para = _status_para(entry, readme=name == "README.md")
            p.write_text(text[:m.start(2)] + para + text[m.end(2):], encoding="utf-8")
            print(f"  rewrote {p.relative_to(ROOT)} Status")


def cmd_check() -> int:
    skills = _read_registry().get("skills", {})
    bad = [f"{slug}: {d}" for slug, e in sorted(skills.items()) for d in _drift(slug, e)]
    if bad:
        print("verify_live --check: FAIL - Beta and live-verified are mutually exclusive:")
        for b in bad:
            print(f"  - {b}")
        print("  Fix: python3 tools/maintainer/verify_live.py --sync && "
              "python3 tools/maintainer/build-catalog.py")
        return 1
    print(f"verify_live --check: OK ({len(skills)} skills)")
    return 0


def cmd_sync() -> int:
    reg = _read_registry()
    for slug, entry in reg.get("skills", {}).items():
        if _drift(slug, entry):
            print(slug)
            _sync(slug, entry)
    _write_registry(reg)
    return 0


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
    _sync(slug, entry)
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
    _sync(slug, skills[slug])
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
        "check": False,
        "sync": False,
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
        elif a in ("--check", "--sync"):
            opts[a[2:]] = True
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
    if opts["check"]:
        return cmd_check()
    if opts["sync"]:
        return cmd_sync()

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
