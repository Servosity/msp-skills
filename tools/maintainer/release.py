#!/usr/bin/env python3
"""The release queue: bump a skill's version everywhere, stamp its CLI hash, and
print the (never-run) git tag commands for the human to push.

For each target slug this script:
  1. Reads the current version from skills/<slug>/manifest.json.
  2. Picks the next version: for a never-released slug the existing manifest
     version IS the first release (no bump); otherwise bump per --bump.
  3. Propagates that version into every file that carries it:
       - skills/<slug>/manifest.json                 "version"
       - skills/<slug>/.claude-plugin/plugin.json    "version"
       - skills/<slug>/server.json                   "version", packages[0].version,
                                                      and the <slug>-v<version>
                                                      segment of packages[0].identifier
       - .claude-plugin/marketplace.json             matching plugins[] "version"
       - skills/<slug>/CHANGELOG.md                   a new "## [<version>]" section
                                                      (only when bumping; skipped if
                                                      the heading already exists)
       - tools/maintainer/skills.json                "version" and
                                                      "cli_hash_at_release" = current hash
  4. Prints the exact tag+push command. It NEVER runs git tag or git push.

JSON is rewritten with 2-space indent + a trailing newline to match the
repo's formatting convention. --dry-run changes nothing and prints the diff.

Pure stdlib. Run locally:
    python3 tools/maintainer/release.py --slug halopsa --dry-run
    python3 tools/maintainer/release.py --slugs halopsa,servosity --bump minor
    python3 tools/maintainer/release.py --all-pending
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import cli_hash  # noqa: E402  (local tools/ module)
import registry  # noqa: E402  (local tools/ module)
import release_state  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR
REGISTRY = registry.REGISTRY
MARKETPLACE = ROOT / ".claude-plugin" / "marketplace.json"


def _read_json(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def _write_json(path: Path, data: dict) -> None:
    """Write JSON with 2-space indent + trailing newline (repo convention)."""
    path.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")


def bump_version(version: str, kind: str) -> str:
    parts = version.split(".")
    while len(parts) < 3:
        parts.append("0")
    try:
        major, minor, patch = (int(parts[0]), int(parts[1]), int(parts[2]))
    except ValueError:
        raise SystemExit(f"release: cannot parse version '{version}'")
    if kind == "major":
        return f"{major + 1}.0.0"
    if kind == "minor":
        return f"{major}.{minor + 1}.0"
    return f"{major}.{minor}.{patch + 1}"


def changelog_section(version: str) -> str:
    """A new Keep-a-Changelog section with a placeholder bullet."""
    return (
        f"## [{version}] - unreleased\n\n"
        "### Changed\n"
        "- Describe the changes in this release.\n\n"
    )


class Planner:
    """Collect intended file edits, apply or print them based on dry_run."""

    def __init__(self, dry_run: bool) -> None:
        self.dry_run = dry_run
        self.notes: list[str] = []

    def note(self, msg: str) -> None:
        self.notes.append(msg)

    def write_json(self, path: Path, data: dict, summary: str) -> None:
        rel = path.relative_to(ROOT)
        self.note(f"  {rel}: {summary}")
        if not self.dry_run:
            _write_json(path, data)

    def write_text(self, path: Path, text: str, summary: str) -> None:
        rel = path.relative_to(ROOT)
        self.note(f"  {rel}: {summary}")
        if not self.dry_run:
            path.write_text(text, encoding="utf-8")


def propagate(slug: str, new_version: str, bumped: bool, planner: Planner) -> None:
    # manifest.json
    mf_path = SKILLS_DIR / slug / "manifest.json"
    if mf_path.exists():
        mf = _read_json(mf_path)
        if mf.get("version") != new_version:
            mf["version"] = new_version
            planner.write_json(mf_path, mf, f'version -> {new_version}')

    # .claude-plugin/plugin.json
    pj_path = SKILLS_DIR / slug / ".claude-plugin" / "plugin.json"
    if pj_path.exists():
        pj = _read_json(pj_path)
        if pj.get("version") != new_version:
            pj["version"] = new_version
            planner.write_json(pj_path, pj, f'version -> {new_version}')

    # server.json
    sj_path = SKILLS_DIR / slug / "server.json"
    if sj_path.exists():
        sj = _read_json(sj_path)
        changed = False
        if sj.get("version") != new_version:
            sj["version"] = new_version
            changed = True
        packages = sj.get("packages") or []
        if packages:
            pkg = packages[0]
            if pkg.get("version") != new_version:
                pkg["version"] = new_version
                changed = True
            ident = pkg.get("identifier", "")
            new_ident = _retag_identifier(ident, slug, new_version)
            if new_ident != ident:
                pkg["identifier"] = new_ident
                changed = True
        if changed:
            planner.write_json(sj_path, sj, f'version + packages[0] -> {new_version}')

    # root marketplace.json (matching plugins[] entry)
    if MARKETPLACE.exists():
        mp = _read_json(MARKETPLACE)
        changed = False
        for plugin in mp.get("plugins", []):
            if plugin.get("name") == slug and plugin.get("version") != new_version:
                plugin["version"] = new_version
                changed = True
        if changed:
            planner.write_json(MARKETPLACE, mp, f'plugins[{slug}].version -> {new_version}')

    # CHANGELOG.md (only when bumping, only if heading absent)
    if bumped:
        cl_path = SKILLS_DIR / slug / "CHANGELOG.md"
        if cl_path.exists():
            cl = cl_path.read_text(encoding="utf-8")
            heading = f"## [{new_version}]"
            if heading not in cl:
                new_cl = _insert_changelog(cl, new_version)
                planner.write_text(cl_path, new_cl, f'insert section [{new_version}]')

    # tools/maintainer/skills.json (version + cli_hash_at_release)
    reg = _read_json(REGISTRY)
    entry = reg.get("skills", {}).get(slug)
    if entry is not None:
        cli_dir = SKILLS_DIR / slug / "cli"
        current_hash = cli_hash.compute_cli_hash(cli_dir) if cli_dir.is_dir() else None
        changed = False
        if entry.get("version") != new_version:
            entry["version"] = new_version
            changed = True
        if entry.get("cli_hash_at_release") != current_hash:
            entry["cli_hash_at_release"] = current_hash
            changed = True
        if changed:
            planner.write_json(
                REGISTRY, reg,
                f'version -> {new_version}, cli_hash_at_release stamped'
            )


def _retag_identifier(ident: str, slug: str, version: str) -> str:
    """Rewrite the '<slug>-v<old>' segment of a download URL to the new version."""
    if not ident:
        return ident
    parts = ident.split("/")
    prefix = f"{slug}-v"
    for i, seg in enumerate(parts):
        if seg.startswith(prefix):
            parts[i] = f"{prefix}{version}"
    return "/".join(parts)


def _insert_changelog(content: str, version: str) -> str:
    """Insert a new section just before the first existing '## [' heading; if no
    such heading exists, append after the file's header prose."""
    section = changelog_section(version)
    lines = content.splitlines(keepends=True)
    for i, line in enumerate(lines):
        if line.startswith("## ["):
            return "".join(lines[:i]) + section + "".join(lines[i:])
    # No prior version section: append at end (ensure a blank line before).
    tail = content if content.endswith("\n") else content + "\n"
    if not tail.endswith("\n\n"):
        tail += "\n"
    return tail + section


def parse_args(argv: list[str]) -> tuple[list[str], str, bool]:
    slugs: list[str] = []
    bump = "patch"
    dry_run = False
    all_pending = False

    i = 0
    while i < len(argv):
        a = argv[i]
        if a == "--slug":
            i += 1
            slugs.append(argv[i])
        elif a == "--slugs":
            i += 1
            slugs.extend(s.strip() for s in argv[i].split(",") if s.strip())
        elif a == "--all-pending":
            all_pending = True
        elif a == "--bump":
            i += 1
            bump = argv[i]
        elif a == "--dry-run":
            dry_run = True
        else:
            raise SystemExit(f"release: unknown argument '{a}'")
        i += 1

    if bump not in ("patch", "minor", "major"):
        raise SystemExit(f"release: --bump must be patch|minor|major, got '{bump}'")

    if all_pending:
        for slug in sorted(registry.skills()):
            state = release_state.classify(slug, remote=False)["state"]
            if state in ("binary-pending", "never-released"):
                slugs.append(slug)

    # De-dup, preserve order.
    seen = set()
    ordered = []
    for s in slugs:
        if s not in seen:
            seen.add(s)
            ordered.append(s)

    if not ordered:
        raise SystemExit("release: no target slugs (use --slug, --slugs, or --all-pending)")
    return ordered, bump, dry_run


def main(argv: list[str]) -> int:
    slugs, bump, dry_run = parse_args(argv)
    known = set(registry.skills())
    tag_commands: list[str] = []

    for slug in slugs:
        if slug not in known:
            print(f"release: skipping unknown slug '{slug}'", file=sys.stderr)
            continue

        state = release_state.classify(slug, remote=False)["state"]
        current_version = release_state.manifest_version(slug)
        if state == "never-released":
            new_version = current_version
            bumped = False
        else:
            new_version = bump_version(current_version, bump)
            bumped = True

        planner = Planner(dry_run)
        propagate(slug, new_version, bumped, planner)

        header = (
            f"[{slug}] {current_version} -> {new_version} "
            f"({'first release, no bump' if not bumped else bump + ' bump'})"
        )
        print(("DRY-RUN " if dry_run else "") + header)
        if planner.notes:
            print("\n".join(planner.notes))
        else:
            print("  (no file changes needed)")

        tag = f"{slug}-v{new_version}"
        cmd = f"git tag {tag} && git push origin {tag}"
        tag_commands.append(cmd)
        print(f"  TAG: {cmd}")
        print()

    print("=" * 60)
    print("Copy-paste to tag + push (this script never runs these):")
    for cmd in tag_commands:
        print(f"  {cmd}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
