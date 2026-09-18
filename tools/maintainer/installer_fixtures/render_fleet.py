#!/usr/bin/env python3
"""render_fleet.py - keep every skills/<slug>/install.{sh,ps1} identical modulo slug.

The 67 installers are one script rendered per slug from the generator
templates (msp-skills-publish/assets/templates/install.{sh,ps1}.tmpl, placeholders
{{SLUG}}, {{CLI_BIN}}, {{MCP_BIN}}). This tool has two modes:

  --check                 Prove uniformity: render the REFERENCE installer
                          (skills/<ref>/install.*, default hudu) for every other
                          slug and fail if any shipped installer differs. Runs in
                          CI (installer-fixtures.yml) so a one-off hand edit to a
                          single installer cannot land silently.
  --from-template DIR     Render DIR/install.sh.tmpl and DIR/install.ps1.tmpl for
                          every slug in skills.json and write them in place. Used
                          once per template change; --check then proves the result.

Slug -> binary names come from tools/maintainer/skills.json (cli_binary /
mcp_binary), the same registry check_release_contract.py cross-checks against.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import registry  # noqa: E402

KINDS = ("install.sh", "install.ps1")


def render(template: str, slug: str, cli_bin: str, mcp_bin: str) -> str:
    return (template.replace("{{SLUG}}", slug)
            .replace("{{CLI_BIN}}", cli_bin)
            .replace("{{MCP_BIN}}", mcp_bin))


def to_template(text: str, slug: str, cli_bin: str, mcp_bin: str) -> str:
    """Inverse of render for a reference installer: binary names first (they
    contain the slug), then the bare slug as a whole token."""
    text = text.replace(cli_bin, "{{CLI_BIN}}").replace(mcp_bin, "{{MCP_BIN}}")
    # A slug may be followed by "-v" (the tag prefix) but never by a letter or
    # digit, and never preceded by one or by a hyphen.
    return re.sub(rf"(?<![A-Za-z0-9-]){re.escape(slug)}(?![A-Za-z0-9])", "{{SLUG}}", text)


def targets() -> list[tuple[str, str, str]]:
    out = []
    for slug, m in sorted(registry.skills().items()):
        if registry.is_markdown_only(slug):
            continue
        out.append((slug, m["cli_binary"], m["mcp_binary"]))
    return out


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--check", action="store_true")
    ap.add_argument("--from-template", metavar="DIR")
    ap.add_argument("--ref", default="hudu", help="reference slug for --check")
    a = ap.parse_args()
    if not a.check and not a.from_template:
        ap.error("one of --check or --from-template is required")

    tmpls: dict[str, str] = {}
    if a.from_template:
        for kind in KINDS:
            tmpls[kind] = (Path(a.from_template) / f"{kind}.tmpl").read_text(encoding="utf-8")
    else:
        ref = dict((s, (c, m)) for s, c, m in targets()).get(a.ref)
        if not ref:
            print(f"reference slug {a.ref!r} is not a binary skill", file=sys.stderr)
            return 2
        for kind in KINDS:
            p = registry.skill_path(a.ref) / kind
            tmpls[kind] = to_template(p.read_text(encoding="utf-8"), a.ref, *ref)

    bad = 0
    n = 0
    for slug, cli_bin, mcp_bin in targets():
        for kind in KINDS:
            p = registry.skill_path(slug) / kind
            want = render(tmpls[kind], slug, cli_bin, mcp_bin)
            n += 1
            if a.from_template:
                p.write_text(want, encoding="utf-8")
            elif p.read_text(encoding="utf-8") != want:
                bad += 1
                print(f"DIFFERS: {p.relative_to(registry.ROOT)} is not the reference installer rendered for {slug}")
    if a.from_template:
        print(f"rendered {n} installer(s) from {a.from_template}")
        return 0
    if bad:
        print(f"installer uniformity FAILED: {bad} file(s) differ from skills/{a.ref} modulo slug")
        return 1
    print(f"installer uniformity OK: {n} file(s) identical to skills/{a.ref} modulo slug")
    return 0


if __name__ == "__main__":
    sys.exit(main())
