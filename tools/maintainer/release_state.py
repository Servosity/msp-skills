#!/usr/bin/env python3
"""Report each skill's release state: is a new tag pending, and why?

For every slug in the registry this answers one question - "does this skill need
a release, and what kind?" - by comparing three facts:

  current_hash   the deterministic content hash of skills/<slug>/cli today
  released_hash  skills.json "cli_hash_at_release" (the hash at the last cut)
  latest_tag     the newest local git tag matching "<slug>-v*" (semver-sorted)

Classification:
  never-released   released_hash is missing/None (no release has ever been cut)
  binary-pending   current_hash != released_hash (the CLI source changed)
  version-pending  hashes match but no tag "<slug>-v<manifest version>" exists
                   (locally; with --remote also consults `git ls-remote`)
  up-to-date       hashes match and the manifest-version tag exists

A tag is necessary and not sufficient (AGENTS.md, "What counts as released"):
with immutable releases a tag can front a stranded DRAFT that no installer can
see. `--released-tags FILE` replaces "the tag exists" with "the tag names a
PUBLISHED release": FILE lists one tag per line, as catalog.yml fetches it from
`gh api repos/<owner>/<repo>/releases` (drafts excluded). With it, a tag whose
release never sealed reads as version-pending, which is the truth an operator
needs. The file is authoritative when given: `git tag` and --remote are not
consulted, and a missing or empty file is an error, never "nothing released".

This is a reporter, not a gate: it always exits 0. Feed --pending to a release
loop, or --json to another tool.

Pure stdlib. Run locally:
    python3 tools/maintainer/release_state.py
    python3 tools/maintainer/release_state.py --json
    python3 tools/maintainer/release_state.py --pending
    python3 tools/maintainer/release_state.py --remote
    python3 tools/maintainer/release_state.py --json --released-tags /tmp/released.txt
"""

from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import cli_hash  # noqa: E402  (local tools/ module)
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR

PENDING_STATES = ("never-released", "binary-pending", "version-pending")


def load_released_tags(path: str | Path) -> set[str]:
    """The published-release tag list, one tag per line (blank lines ignored).

    Fails loudly on a missing or EMPTY file: this repository has hundreds of
    published releases, so an empty list is a fetch that went wrong, and
    reading it as "nothing is released" would flip every connector to pending
    in the same derived commit that was meant to make pending.json truthful."""
    p = Path(path)
    try:
        tags = {ln.strip() for ln in p.read_text(encoding="utf-8").splitlines() if ln.strip()}
    except OSError as exc:
        raise SystemExit(f"release_state: cannot read --released-tags {p}: {exc}")
    if not tags:
        raise SystemExit(
            f"release_state: --released-tags {p} is empty; refusing to report every "
            f"connector as pending. Omit the flag to fall back to local git tags."
        )
    return tags


def manifest_version(slug: str) -> str:
    """Read the skill's version. Binary skills carry it in manifest.json;
    markdown-only skills have no manifest, so read .claude-plugin/plugin.json.
    Honors the registry source_dir mapping (slug != on-disk dir)."""
    sdir = registry.skill_path(slug)
    candidates = [sdir / "manifest.json"]
    if registry.is_markdown_only(slug):
        candidates = [sdir / ".claude-plugin" / "plugin.json"]
    for path in candidates:
        try:
            return json.loads(path.read_text(encoding="utf-8")).get("version", "0.0.0")
        except (OSError, json.JSONDecodeError):
            continue
    return "0.0.0"


def _version_tuple(v: str) -> tuple[int, ...]:
    """Parse a dotted version into a comparable int tuple; non-numeric -> 0."""
    parts = []
    for chunk in v.split("."):
        num = ""
        for ch in chunk:
            if ch.isdigit():
                num += ch
            else:
                break
        parts.append(int(num) if num else 0)
    return tuple(parts)


def _git(args: list[str]) -> str:
    """Run a git command from ROOT; return stdout, or '' on any failure."""
    try:
        out = subprocess.run(
            ["git", *args],
            cwd=str(ROOT),
            capture_output=True,
            text=True,
            timeout=15,
        )
    except (OSError, subprocess.SubprocessError):
        return ""
    return out.stdout if out.returncode == 0 else ""


def latest_tag(slug: str, released: set[str] | None = None) -> tuple[str | None, str | None]:
    """Return (tag, version) for the highest-semver tag '<slug>-v*' - among the
    PUBLISHED releases when `released` is given, else among local git tags.

    Sorts by parsed version tuple in pure Python (not `sort -V`)."""
    prefix = f"{slug}-v"
    if released is not None:
        tags = [t for t in released if t.startswith(prefix)]
    else:
        raw = _git(["tag", "--list", f"{prefix}*"])
        tags = [t.strip() for t in raw.splitlines() if t.strip().startswith(prefix)]
    if not tags:
        return None, None
    best = max(tags, key=lambda t: _version_tuple(t[len(prefix):]))
    return best, best[len(prefix):]


def tag_exists_local(tag: str) -> bool:
    raw = _git(["tag", "--list", tag])
    return any(line.strip() == tag for line in raw.splitlines())


def tag_exists_remote(tag: str) -> bool:
    raw = _git(["ls-remote", "--tags", "origin", tag])
    return bool(raw.strip())


def classify(slug: str, remote: bool, released: set[str] | None = None) -> dict:
    """`released`, when given, is the published-release tag set (see
    load_released_tags) and is authoritative: it replaces both the local tag
    lookup and the --remote fallback."""
    # markdown-only skills have no cli/ to hash and never cut a binary release;
    # report a terminal "markdown-only" state (never pending).
    if registry.is_markdown_only(slug):
        return {
            "state": "markdown-only",
            "version": manifest_version(slug),
            "latest_tag": None,
            "current_hash": None,
            "released_hash": None,
        }

    cli_dir = registry.skill_path(slug) / "cli"
    current = cli_hash.compute_cli_hash(cli_dir) if cli_dir.is_dir() else None
    entry = registry.skills().get(slug, {})
    released_hash = entry.get("cli_hash_at_release")
    version = manifest_version(slug)
    lt, _lt_ver = latest_tag(slug, released)

    version_tag = f"{slug}-v{version}"
    if released_hash is None:
        state = "never-released"
    elif current != released_hash:
        state = "binary-pending"
    else:
        if released is not None:
            have_tag = version_tag in released
        else:
            have_tag = tag_exists_local(version_tag)
            if not have_tag and remote:
                have_tag = tag_exists_remote(version_tag)
        state = "up-to-date" if have_tag else "version-pending"

    return {
        "state": state,
        "version": version,
        "latest_tag": lt,
        "current_hash": current,
        "released_hash": released_hash,
    }


def main(argv: list[str]) -> int:
    as_json = "--json" in argv
    pending_only = "--pending" in argv
    remote = "--remote" in argv
    released: set[str] | None = None
    if "--released-tags" in argv:
        i = argv.index("--released-tags")
        if i + 1 >= len(argv):
            raise SystemExit("release_state: --released-tags needs a FILE")
        released = load_released_tags(argv[i + 1])

    slugs = sorted(registry.skills())
    results = {slug: classify(slug, remote, released) for slug in slugs}

    if pending_only:
        for slug in slugs:
            if results[slug]["state"] in PENDING_STATES:
                print(slug)
        return 0

    if as_json:
        print(json.dumps(results, indent=2))
        return 0

    # Aligned table.
    headers = ("slug", "state", "version", "latest tag", "hash-match")
    rows = []
    for slug in slugs:
        r = results[slug]
        match = "n/a" if r["released_hash"] is None else (
            "yes" if r["current_hash"] == r["released_hash"] else "no"
        )
        rows.append((
            slug,
            r["state"],
            r["version"],
            r["latest_tag"] or "-",
            match,
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


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
