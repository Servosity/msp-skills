#!/usr/bin/env python3
"""The "Comparing backup vendors?" note on backup-vendor connector pages.

One source of text (backup_vendor_note.json, beside this file), two surfaces:

- docs/skills/<slug>.md, the docs-site page. render_docs_page.py calls
  docs_block() and places the note directly after the "Also from Servosity"
  callout, so build-catalog.py keeps it current like the rest of the page.
- skills/<slug>/README.md, the GitHub landing page. A marker-delimited block
  (BLOCK_START .. BLOCK_END) placed after the first-party note, else after the
  hero media block, else after the non-affiliation banner. --write inserts,
  refreshes or removes it; --check fails when any README disagrees with the
  source.

Which connectors get it is data too (applies_to in the JSON): a registry
category, plus explicit slugs for vendors that sell backup inside a connector
filed elsewhere. Servosity's own first-party connector and markdown-only skills
never do. Each link carries utm_campaign=<slug> and utm_content=<surface>, so
traffic is attributable to the page that sent it.

Usage:
    python3 tools/maintainer/backup_vendor_note.py --list
    python3 tools/maintainer/backup_vendor_note.py --write
    python3 tools/maintainer/backup_vendor_note.py --check
    python3 tools/maintainer/backup_vendor_note.py --self-test

Pure stdlib.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path
from urllib.parse import urlencode

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

SOURCE = Path(__file__).resolve().parent / "backup_vendor_note.json"
BLOCK_START = "<!-- backup-vendor-note:start -->"
BLOCK_END = "<!-- backup-vendor-note:end -->"
FIRST_PARTY_END = "<!-- first-party-note:end -->"
MEDIA_END = "<!-- media:end -->"
_BLOCK_RE = re.compile(
    r"\n*" + re.escape(BLOCK_START) + r".*?" + re.escape(BLOCK_END), re.DOTALL)


def load(path: Path = SOURCE) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def applies(slug: str, entry: dict, cfg: dict) -> bool:
    """True when this connector's pages carry the note."""
    rule = cfg.get("applies_to", {})
    if entry.get("first_party") or entry.get("markdown_only"):
        return False
    if slug in rule.get("exclude_slugs", []):
        return False
    return (entry.get("category") in rule.get("categories", [])
            or slug in rule.get("extra_slugs", []))


def link(slug: str, surface: str, cfg: dict) -> str:
    params = dict(cfg.get("utm", {}))
    params["utm_campaign"] = slug
    params["utm_content"] = surface
    return f"{cfg['url']}?{urlencode(params)}"


def note_markdown(slug: str, surface: str, cfg: dict) -> str:
    """The note as one Markdown blockquote line."""
    return (f"> **{cfg['label']}** {cfg['body']} "
            f"[{cfg['link_text']}]({link(slug, surface, cfg)})")


def docs_block(slug: str, entry: dict, cfg: dict | None = None) -> str:
    """The docs-page block, or "" when the note does not apply."""
    cfg = cfg or load()
    if not applies(slug, entry, cfg):
        return ""
    return "\n" + note_markdown(slug, "docs", cfg) + "\n"


def apply_to_readme(text: str, slug: str, want: bool, cfg: dict) -> str:
    """Return README text with the block inserted, refreshed or removed."""
    starts, ends = text.count(BLOCK_START), text.count(BLOCK_END)
    block = f"{BLOCK_START}\n{note_markdown(slug, 'readme', cfg)}\n{BLOCK_END}"
    if starts or ends:
        # A duplicate, stray or reversed marker: the boundaries are a guess, and a
        # wrong guess deletes README content, so refuse rather than repair.
        if not (starts == ends == 1 and text.index(BLOCK_START) < text.index(BLOCK_END)):
            raise ValueError(f"malformed {BLOCK_START} markers; fix them by hand")
        if want:
            return _BLOCK_RE.sub(lambda _m: "\n\n" + block, text, count=1)
        return _BLOCK_RE.sub("", text)
    if not want:
        return text
    for anchor in (FIRST_PARTY_END, MEDIA_END):
        if anchor in text:
            return text.replace(anchor, f"{anchor}\n\n{block}", 1)
    # No anchor block: place it after the banner (the first blockquote after
    # the H1), the same fallback the publish skill uses for its own notes.
    lines = text.split("\n")
    at = None
    for i, ln in enumerate(lines):
        if ln.startswith("> "):
            j = i
            while j < len(lines) and lines[j].startswith(">"):
                j += 1
            at = j
            break
    if at is None:
        at = next((i + 1 for i, ln in enumerate(lines) if ln.startswith("# ")), 0)
    lines[at:at] = ["", block]
    return "\n".join(lines)


def _readmes() -> list[tuple[str, dict, Path]]:
    out = []
    for slug, entry in registry.skills().items():
        if entry.get("markdown_only"):
            continue
        readme = registry.skill_path(slug) / "README.md"
        if readme.exists():
            out.append((slug, entry, readme))
    return out


def run(write: bool) -> int:
    cfg = load()
    drift, malformed = [], []
    for slug, entry, readme in _readmes():
        old = readme.read_text(encoding="utf-8")
        try:
            new = apply_to_readme(old, slug, applies(slug, entry, cfg), cfg)
        except ValueError as e:
            print(f"FAIL: {slug}: {e}", file=sys.stderr)
            malformed.append(slug)
            continue
        if new != old:
            drift.append(slug)
            if write:
                readme.write_text(new, encoding="utf-8")
    if write:
        print(f"backup_vendor_note: updated {len(drift)} README(s): {', '.join(drift) or '-'}")
        return 1 if malformed else 0
    if malformed:
        return 1
    if drift:
        print("FAIL: README backup-vendor note disagrees with "
              f"tools/maintainer/backup_vendor_note.json for: {', '.join(drift)}\n"
              "  fix: python3 tools/maintainer/backup_vendor_note.py --write", file=sys.stderr)
        return 1
    print("PASS: every README backup-vendor note matches its source")
    return 0


def _self_test() -> int:
    cfg = load()
    bad = []

    def check(name: str, ok: bool) -> None:
        print(f"  {'ok  ' if ok else 'FAIL'} {name}")
        if not ok:
            bad.append(name)

    for field in ("label", "body", "link_text", "url"):
        check(f"source has {field}", bool(cfg.get(field)))
    text = " ".join(str(cfg.get(f, "")) for f in ("label", "body", "link_text"))
    check("no em-dash or double dash in the note", "\u2014" not in text and "--" not in text)
    check("no 'only' claim in the note", not re.search(r"\bonly\b", text, re.I))
    check("body is at most 2 sentences",
          len([s for s in re.split(r"(?<=[.!?])[\"\u201d]?\s+", cfg["body"].strip()) if s]) <= 2)

    backup = {"category": "Backup/DR"}
    check("applies to a Backup/DR connector", applies("veeam", backup, cfg))
    check("skips a first-party connector",
          not applies("servosity", {"category": "Backup/DR", "first_party": True}, cfg))
    check("skips a markdown-only skill",
          not applies("x", {"category": "Backup/DR", "markdown_only": True}, cfg))
    check("skips an unrelated category", not applies("halopsa", {"category": "PSA"}, cfg))
    extra = (cfg["applies_to"].get("extra_slugs") or ["__none__"])[0]
    check("applies to an extra_slugs vendor filed elsewhere",
          extra == "__none__" or applies(extra, {"category": "RMM"}, cfg))

    url = link("veeam", "readme", cfg)
    check("link carries utm_campaign=<slug>", "utm_campaign=veeam" in url)
    check("link carries utm_source + utm_medium",
          "utm_source=msp-skills" in url and "utm_medium=connector-note" in url)
    check("docs link says utm_content=docs", "utm_content=docs" in docs_block("veeam", backup, cfg))
    check("docs block empty when not applicable", docs_block("halopsa", {"category": "PSA"}, cfg) == "")

    fp = ("# V\n\n> Unofficial. banner\n\n<!-- media:start -->\nm\n<!-- media:end -->\n\n"
          "<!-- first-party-note:start -->\n> fp\n<!-- first-party-note:end -->\n\nBody\n")
    once = apply_to_readme(fp, "veeam", True, cfg)
    check("inserts after the first-party note",
          once.index(BLOCK_START) > once.index(FIRST_PARTY_END) and once.index(BLOCK_START) < once.index("Body"))
    check("insert is idempotent", apply_to_readme(once, "veeam", True, cfg) == once)
    stale = once.replace(cfg["body"], "old wording")
    check("refreshes stale wording", apply_to_readme(stale, "veeam", True, cfg) == once)
    check("removes the block when no longer applicable",
          apply_to_readme(once, "veeam", False, cfg) == fp)
    def refuses(name: str, text: str, want: bool) -> None:
        try:
            apply_to_readme(text, "veeam", want, cfg)
            check(name, False)
        except ValueError:
            check(name, True)

    dup = once.replace(BLOCK_END, BLOCK_END + "\n\n" + BLOCK_START + "\n> stale\n" + BLOCK_END, 1)
    refuses("refuses a duplicate block", dup, True)
    refuses("refuses reversed markers", fp.replace("Body", BLOCK_END + "\nBody\n" + BLOCK_START, 1), True)
    refuses("refuses a stray marker when not applicable", fp.replace("Body", BLOCK_START + "\nBody", 1), False)
    nested = ("# X\n\n" + BLOCK_START + "\n## Authentication\nKeep this\n" + BLOCK_START
              + "\n> stale\n" + BLOCK_END + "\n")
    refuses("refuses nested markers rather than delete what they enclose", nested, True)
    refuses("refuses nested markers when not applicable", nested, False)
    media_only = "# N\n\n> Unofficial. banner\n\n<!-- media:start -->\nm\n<!-- media:end -->\n\nBody\n"
    m = apply_to_readme(media_only, "ninjaone", True, cfg)
    check("falls back to after the media block",
          m.index(MEDIA_END) < m.index(BLOCK_START) < m.index("Body"))
    bare = "# N\n\n> Unofficial. banner\n> line two\n\nBody\n"
    b = apply_to_readme(bare, "ninjaone", True, cfg)
    check("falls back to after the banner",
          b.index("line two") < b.index(BLOCK_START) < b.index("Body"))
    check("leaves a non-applicable README untouched",
          apply_to_readme(bare, "halopsa", False, cfg) == bare)

    if bad:
        print(f"FAIL: backup_vendor_note self-test ({len(bad)} case(s))", file=sys.stderr)
        return 1
    print("PASS: backup_vendor_note self-test")
    return 0


def main(argv: list[str]) -> int:
    if "--self-test" in argv:
        return _self_test()
    if "--list" in argv:
        cfg = load()
        for slug, entry in registry.skills().items():
            if applies(slug, entry, cfg):
                print(slug)
        return 0
    if "--write" in argv:
        return run(write=True)
    if "--check" in argv:
        return run(write=False)
    print(__doc__)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
