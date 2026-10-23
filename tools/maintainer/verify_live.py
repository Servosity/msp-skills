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
      "issue_url": "<clickable receipt link>" | null,
      "verified_version": "X.Y.Z" | null,
      "verified_cli_hash": "sha256:<hex>" | null,
      "verified_basis": "reported" | "latest-release-at-flip" | "unknown"
    }

The last three bind the badge to the code that was verified: the release
version and the cli_hash.py hash of skills/<dir>/cli at that release's tag
(computed from git objects, exactly as stamp_release.py stamps
cli_hash_at_release). The catalog then shows "verified on vX.Y.Z" once the
shipped release carries different code, so a badge never silently covers code
no MSP ran. The basis says how the version was chosen:
  reported                the reporter named it (--verified-version)
  latest-release-at-flip  the newest release tag at the flip, skipping any
                          cut from a commit made after the report
                          (--reported-at); what a fresh install gave them
  unknown                 nothing provable (both fields null)
`--bind` sets these on an existing badge without touching its date or
evidence (the backfill path, and a report that names its version later).

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
    python3 tools/maintainer/verify_live.py --bind --slug cove --verified-version 0.1.3
    python3 tools/maintainer/verify_live.py --bind --slug cove --unknown
    python3 tools/maintainer/verify_live.py --status
    python3 tools/maintainer/verify_live.py --slug halopsa --source self \\
        --evidence "redogfood" --force

Pure stdlib.
"""

from __future__ import annotations

import datetime as dt
import json
import re
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)
import release_state  # noqa: E402  (local tools/ module: one version order)
import stamp_release  # noqa: E402  (local tools/ module: the hash of a tagged tree)

ROOT = registry.ROOT
REGISTRY = registry.REGISTRY

VALID_SOURCES = ("build-session", "circle", "github", "self")

AWAITING = {"status": "awaiting", "date": None, "source": None, "evidence": None}

BASES = ("reported", "latest-release-at-flip", "unknown")
VERSION_RE = re.compile(r"^\d+\.\d+\.\d+\Z")
UNKNOWN_BINDING = {"verified_version": None, "verified_cli_hash": None, "verified_basis": "unknown"}


def _git(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(["git", "-C", str(ROOT), *args], capture_output=True, text=True, timeout=60)


def _tag_exists(tag: str) -> bool:
    return _git("rev-parse", "-q", "--verify", f"refs/tags/{tag}").returncode == 0


def tag_cli_hash(slug: str, tag: str) -> str | None:
    """cli_hash.py's hash of the connector's cli/ as it is at `tag`, from git
    objects (stamp_release.tree_hash), or None when it cannot be computed."""
    rel = registry.skill_path(slug).relative_to(ROOT).as_posix() + "/cli"
    return stamp_release.tree_hash(ROOT, tag, rel)


def _parse_when(value: str) -> dt.datetime:
    try:
        when = dt.datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        raise SystemExit(f"verify_live: --reported-at {value!r} is not an ISO-8601 timestamp")
    if when.tzinfo is None:
        raise SystemExit(f"verify_live: --reported-at {value!r} needs a timezone (e.g. ...Z)")
    return when


def binding(slug: str, version: str | None = None, reported_at: str | None = None) -> dict:
    """The verified_* fields for a flip of `slug` (see the module docstring)."""
    if version is not None:
        if not VERSION_RE.match(version):
            raise SystemExit(f"verify_live: --verified-version {version!r} is not X.Y.Z")
        tag = f"{slug}-v{version}"
        if not _tag_exists(tag):
            raise SystemExit(f"verify_live: no local tag {tag} (git fetch --tags origin), so "
                             f"the code at {version} cannot be hashed")
        h = tag_cli_hash(slug, tag)
        if h is None:
            raise SystemExit(f"verify_live: the cli/ tree at {tag} cannot be hashed reproducibly")
        return {"verified_version": version, "verified_cli_hash": h, "verified_basis": "reported"}
    cutoff = _parse_when(reported_at) if reported_at else None
    prefix = f"{slug}-v"
    tags = [t for t in _git("tag", "--list", f"{prefix}*").stdout.split()
            if VERSION_RE.match(t[len(prefix):])]
    for tag in sorted(tags, key=lambda t: release_state._version_tuple(t[len(prefix):]), reverse=True):
        if cutoff is not None:
            made = _git("log", "-1", "--format=%cI", f"{tag}^{{commit}}").stdout.strip()
            if not made or _parse_when(made) > cutoff:
                continue  # cut from a commit made after the report: never what they ran
        h = tag_cli_hash(slug, tag)
        if h is None:
            break
        return {"verified_version": tag[len(prefix):], "verified_cli_hash": h,
                "verified_basis": "latest-release-at-flip"}
    print(f"verify_live: WARNING no release tag of {slug} could be bound (fetch tags?); "
          "recording the verified version as unknown", file=sys.stderr)
    return dict(UNKNOWN_BINDING)


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
    verified_version: str | None = None,
    reported_at: str | None = None,
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
    bound = binding(slug, verified_version, reported_at)
    entry["live_verified"] = {
        "status": "live-verified",
        "date": use_date,
        "source": source,
        "evidence": evidence,
        "verified_by": verified_by,
        "issue_url": use_issue_url,
        **bound,
    }
    _write_registry(reg)

    print(f"{slug}: live-verified ({source}, {use_date})")
    print(f"  evidence:    {evidence}")
    print(f"  verified_by: {verified_by or '(default: Servosity (maintainer))'}")
    print(f"  issue_url:   {use_issue_url or '(none)'}")
    print(f"  verified:    {bound['verified_version'] or 'unknown version'} ({bound['verified_basis']})")
    _print_followup(slug, source)
    return 0


def cmd_bind(slug: str, version: str | None, unknown: bool) -> int:
    """Bind an EXISTING badge to the version it verified (or record that it is
    unknown), leaving its date, source and evidence untouched."""
    if (version is None) == (not unknown):
        raise SystemExit("verify_live: --bind needs exactly one of --verified-version or --unknown")
    reg = _read_registry()
    entry = reg.get("skills", {}).get(slug)
    if entry is None:
        print(f"verify_live: unknown slug '{slug}'", file=sys.stderr)
        return 1
    lv = _entry_state(entry)
    if lv.get("status") != "live-verified":
        print(f"verify_live: '{slug}' is not live-verified; nothing to bind", file=sys.stderr)
        return 1
    lv.update(dict(UNKNOWN_BINDING) if unknown else binding(slug, version))
    entry["live_verified"] = lv
    _write_registry(reg)
    print(f"{slug}: badge bound to {lv['verified_version'] or 'an unknown version'} "
          f"({lv['verified_basis']}{', ' + lv['verified_cli_hash'][:19] + '...' if lv['verified_cli_hash'] else ''})")
    _print_followup(slug, "bind")
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
        "bind": False,
        "unknown": False,
        "verified_version": None,
        "reported_at": None,
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
        elif a == "--verified-version":
            i += 1
            opts["verified_version"] = argv[i]
        elif a == "--reported-at":
            i += 1
            opts["reported_at"] = argv[i]
        elif a == "--bind":
            opts["bind"] = True
        elif a == "--unknown":
            opts["unknown"] = True
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

    if opts["bind"]:
        if not opts["slug"]:
            raise SystemExit("verify_live: --bind requires --slug")
        return cmd_bind(opts["slug"], opts["verified_version"], opts["unknown"])

    if opts["slug"]:
        if not opts["source"]:
            raise SystemExit("verify_live: a flip requires --source")
        return cmd_verify(
            opts["slug"], opts["source"], opts["evidence"], opts["date"], opts["force"],
            verified_by=opts["verified_by"], issue_url=opts["issue_url"],
            verified_version=opts["verified_version"], reported_at=opts["reported_at"],
        )

    raise SystemExit(
        "verify_live: nothing to do. Use --status, --slug ... --source ... "
        "--evidence ..., or --revoke --slug ..."
    )


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
