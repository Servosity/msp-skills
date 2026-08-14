#!/usr/bin/env python3
"""Regenerate every derived surface that enumerates skills, from skills.json.

This script is the single regenerator. One run refreshes:
  - catalog.json (the public machine-readable catalog)
  - docs/_data/catalog.json (the Jekyll projection the site renders from)
  - the README catalog table + skills-count badge
  - the README generated blocks (hero-live, install-featured, agent-can-do,
    footer-releases) so prose that names skills can never go stale again
  - docs/skills/<slug>.md via render_docs_page (so a live-verified flip or a
    page.json edit re-renders the site page in the same pass)

CI runs it on every PR that touches `skills/` and fails if any committed
derived file drifts from what this script would produce.

Run locally:
    python3 tools/maintainer/build-catalog.py
"""

from __future__ import annotations

import datetime as dt
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)
import render_docs_page  # noqa: E402  (local tools/ module)

ROOT = Path(__file__).resolve().parent.parent.parent
SKILLS_DIR = ROOT / "skills"
README = ROOT / "README.md"
CATALOG = ROOT / "catalog.json"
DOCS_CATALOG = ROOT / "docs" / "_data" / "catalog.json"
IT_WORKS_FORM = ROOT / ".github" / "ISSUE_TEMPLATE" / "it-works.yml"

# Owner/repo and per-skill metadata are the single source of truth in
# tools/skills.json (loaded via registry). Every install URL is built from
# these, so the catalog can never drift back to an unresolved owner token; CI
# fails the build if one is reintroduced anywhere in the tree.
OWNER, REPO = registry.owner_repo()
SKILL_META = registry.load()["skills"]


def install_url(skill: str, script: str) -> str:
    return (
        f"bash <(curl -fsSL https://raw.githubusercontent.com/{OWNER}/"
        f"{REPO}/main/skills/{skill}/{script})"
    )


def _plugin_version(skill_dir: Path) -> str:
    """Read the version from .claude-plugin/plugin.json (markdown-only fallback)."""
    pj = skill_dir / ".claude-plugin" / "plugin.json"
    if pj.exists():
        try:
            return json.loads(pj.read_text(encoding="utf-8")).get("version", "0.0.0")
        except (OSError, ValueError):
            return "0.0.0"
    return "0.0.0"


def build_entry(skill_dir: Path) -> dict:
    dir_name = skill_dir.name
    slug = registry.slug_for_dir(dir_name)

    meta = SKILL_META.get(slug)
    if meta is None:
        raise SystemExit(
            f"new skill '{slug}' (skills/{dir_name}) has no entry in "
            "tools/skills.json. Add one before the catalog can be regenerated."
        )

    markdown_only = bool(meta.get("markdown_only"))

    # markdown-only skills have no manifest.json or binaries; version comes from
    # .claude-plugin/plugin.json and the binary fields are null. Binary skills
    # keep their manifest.json as the version + description source of truth.
    if markdown_only:
        manifest = {}
        version = _plugin_version(skill_dir)
    else:
        manifest_path = skill_dir / "manifest.json"
        if not manifest_path.exists():
            raise SystemExit(f"missing manifest.json for skill {slug}")
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        version = manifest.get("version", "0.0.0")

    entry = {
        "name": slug,
        "system": meta["system"],
        "status": meta["status"],
        "skill_path": f"skills/{dir_name}",
        "cli_binary": meta.get("cli_binary"),
        "mcp_binary": meta.get("mcp_binary"),
        "version": version,
        # Which cli-printing-press version minted this skill (null for
        # markdown-only skills and any binary skill onboarded before the
        # press-provenance field was added to its manifest.json).
        "printing_press_version": printing_press_version(manifest),
        "license": manifest.get("license", "Apache-2.0"),
        "vendor": meta["vendor"],
        "vendor_trademark_owner": meta["vendor_trademark_owner"],
        "first_party": meta["first_party"],
        "install_skill_one_liner": (
            "" if markdown_only else install_url(dir_name, "install.sh")
        ),
        "install_mcp_doc": (
            "" if markdown_only else f"skills/{dir_name}/mcp-install.md"
        ),
        "description": manifest.get("description", meta.get("tagline", "")),
        "verification": _verification_state(meta, markdown_only),
    }
    if meta.get("category"):
        entry["category"] = meta["category"]
    if meta.get("tagline"):
        entry["tagline"] = meta["tagline"]
    # Only stamp the flag when true so binary skills' catalog entries stay
    # byte-identical (the CI drift gate compares the catalog exactly).
    if markdown_only:
        entry["markdown_only"] = True
    return entry


def printing_press_version(manifest: dict) -> str | None:
    version = manifest.get("printing_press_version")
    if version:
        return version
    meta = manifest.get("_meta")
    if not isinstance(meta, dict):
        return None
    press = meta.get("io.github.mvanhorn.cli-printing-press")
    if not isinstance(press, dict):
        return None
    return press.get("printing_press_version")


def _verification_state(meta: dict, markdown_only: bool) -> str:
    """The honest badge state: 'live-verified' only when a real MSP confirmed
    the skill against a live tenant (verify_live.py flipped it); 'awaiting'
    otherwise; 'n/a' for markdown-only skills (no tenant to verify)."""
    if markdown_only:
        return "n/a"
    lv = meta.get("live_verified") or {}
    return "live-verified" if lv.get("status") == "live-verified" else "awaiting"


def status_badge(verification: str) -> str:
    """Map a skill's VERIFICATION state to a shields.io badge URL.

    'live-verified' (a real MSP confirmed it against a live tenant; flipped
    only by verify_live.py with a date + source + evidence): green badge.
    'awaiting' (passes every mechanical gate; not yet confirmed live): amber
    badge framed as an invitation - the report is the high-leverage signal.
    'n/a' (markdown-only, no tenant to verify): neutral badge.
    """
    if verification == "live-verified":
        return "![Live-verified](https://img.shields.io/badge/Live--verified-by_a_real_MSP-2E7D32)"
    if verification == "n/a":
        return "![Meta](https://img.shields.io/badge/Meta-skill-6B7280)"
    return "![Awaiting live verification](https://img.shields.io/badge/Awaiting-live_verification-EAB308)"


def render_first_party_callout(skills: list[dict]) -> str:
    """One-line callout above the catalog table pointing at Servosity's own
    (first-party) skills. Rendered from the registry (the `first_party` flag)
    so it can never go stale as first-party skills are added or renamed. The
    ⭐ matches the per-row star in render_catalog_table()."""
    fp = [s for s in skills if s.get("first_party")]
    if not fp:
        return ""
    connectors = [
        f"the [{s['name']}](./{s['skill_path']}) "
        + ("backup & DR connector" if s["name"] == "servosity" else "connector")
        for s in fp
        if not s.get("markdown_only")
    ]
    # Markdown-only first-party skills are named by their own tagline-ish label,
    # not all lumped under "concierge" (there is more than one now).
    meta_labels = {"msp-skills-concierge": "guided concierge"}
    metas = [
        f"the [{meta_labels.get(s['name'], s['name'])}](./{s['skill_path']})"
        for s in fp
        if s.get("markdown_only")
    ]
    groups = [g for g in (_oxford(connectors), _oxford(metas)) if g]
    return f"> ⭐ **First-party, by Servosity:** {' + '.join(groups)}."


def render_catalog_table(skills: list[dict]) -> str:
    rows = [
        "| Skill | System | Status | Install |",
        "| --- | --- | --- | --- |",
    ]
    for s in skills:
        # Links resolve to the on-disk directory (skill_path), which differs from
        # the slug for a markdown-only skill (msp-skills-concierge -> skills/_meta).
        path = s["skill_path"]
        install = (
            "Marketplace"
            if s.get("markdown_only")
            else "Install"
        )
        # First-party (Servosity's own) skills get a ⭐ so they stand out in the
        # otherwise alphabetical table. Driven by the registry `first_party` flag.
        star = "⭐ " if s.get("first_party") else ""
        rows.append(
            f"| {star}[{s['name']}](./{path}) | {s['system']} | "
            f"{status_badge(s.get('verification', 'awaiting'))} | "
            f"[{install}](./{path}/README.md) |"
        )
    table = "\n".join(rows)
    callout = render_first_party_callout(skills)
    return f"{callout}\n\n{table}" if callout else table


def replace_block(content: str, marker_start: str, marker_end: str, block: str) -> str:
    pattern = re.compile(
        rf"({re.escape(marker_start)})(.*?)({re.escape(marker_end)})",
        re.DOTALL,
    )
    if not pattern.search(content):
        raise SystemExit(
            f"README is missing the {marker_start} / {marker_end} markers; "
            "cannot regenerate the generated blocks."
        )
    return pattern.sub(rf"\1\n{block}\n\3", content)


# The "Which skill" dropdown in the it-works issue form. Anchored on the FIRST
# `options:` block (the `id: skill` dropdown precedes `id: agent` in the file)
# up to its `validations:`. We rewrite the option LIST in place - same line
# shape that already ships (`        - <slug>`) - rather than fencing with
# YAML comments inside the sequence, which the GitHub form parser is fussier
# about and which we cannot validate locally (no PyYAML in CI's catalog job).
_FORM_OPTIONS_RE = re.compile(
    r"(      options:\n)(?:        - .*\n)+?(    validations:\n)"
)


def render_it_works_form(content: str, skills: list[dict]) -> str:
    """Regenerate the it-works form's skill dropdown from the registry: every
    connector (markdown-only meta excluded), slug-sorted, plus the
    "other / not listed" escape hatch. Returns the new file content."""
    connectors = sorted(s["name"] for s in skills if not s.get("markdown_only"))
    options = [f"        - {slug}" for slug in connectors]
    options.append("        - other / not listed")
    block = "\n".join(options) + "\n"
    new_content, n = _FORM_OPTIONS_RE.subn(rf"\g<1>{block}\g<2>", content, count=1)
    if n != 1:
        raise SystemExit(
            f"build-catalog: could not find the skill dropdown options block in "
            f"{IT_WORKS_FORM.relative_to(ROOT)} (expected `      options:` followed "
            f"by `        - ...` lines and `    validations:`). The form structure "
            f"changed; update _FORM_OPTIONS_RE."
        )
    return new_content


# --------------------------------------------------------------------------- #
# README generated blocks. Each renders the volatile, skill-enumerating
# fragment between a marker pair; the hand prose around the markers stays
# agent/human-owned. This is the structural fix for the 2026-06-05 staleness
# bug: connectwise-manage + hubspot shipped, the catalog table updated, but
# the hero sentence / install examples / outcomes table all still said
# "HaloPSA and Servosity" because they were hand-maintained.
# --------------------------------------------------------------------------- #

def _short_name(meta: dict, slug: str) -> str:
    """Display name with any parenthetical qualifier stripped:
    'ConnectWise PSA (Manage)' -> 'ConnectWise PSA'."""
    name = meta.get("display_name") or meta.get("vendor", slug)
    return re.sub(r"\s*\([^)]*\)", "", name).strip()


def _connector_meta() -> list[tuple[str, dict]]:
    """(slug, meta) for every connector (markdown-only excluded), slug-sorted."""
    return [
        (slug, m) for slug, m in sorted(SKILL_META.items())
        if not m.get("markdown_only")
    ]


def _featured_meta() -> list[tuple[str, dict]]:
    """Connectors carrying a `featured` rank, lowest rank first."""
    ranked = [
        (m["featured"], slug, m)
        for slug, m in _connector_meta()
        if isinstance(m.get("featured"), int)
    ]
    return [(slug, m) for _, slug, m in sorted(ranked)]


def _oxford(names: list[str]) -> str:
    if len(names) <= 1:
        return "".join(names)
    if len(names) == 2:
        return f"{names[0]} and {names[1]}"
    return ", ".join(names[:-1]) + f", and {names[-1]}"


def render_hero_live() -> str:
    """The one 'live today' sentence inside the hero paragraph."""
    conns = _connector_meta()
    n = len(conns)
    by_rank = _featured_meta() or conns
    names = [_short_name(m, slug) for slug, m in by_rank[:4]]
    lead = "including " if n > len(names) else ""
    return (
        f"{n} connectors are live today - {lead}{_oxford(names)} - "
        "and more PSA, RMM, backup, and M365 connectors ship every week."
    )


def _install_prompt(slug: str, meta: dict) -> str:
    """The paste-into-your-agent install prompt for one connector. Same shape
    as the prompt each skill README carries, parameterized from the registry."""
    display = meta.get("display_name") or meta.get("vendor", slug)
    dir_name = meta.get("source_dir", slug)
    cli = meta.get("cli_binary", f"{slug}-cli")
    auth = meta.get("install_auth_hint", f"Then run `{cli} --help`")
    return (
        f"> Install the {display} Skill and MCP server from {OWNER.capitalize()}/{REPO} "
        "in this agent workspace. If this workspace uses a POSIX shell (macOS, Linux, "
        "WSL, or Bash), run `bash <(curl -fsSL https://raw.githubusercontent.com/"
        f"{OWNER.capitalize()}/{REPO}/main/skills/{dir_name}/install.sh)`. If it uses "
        "Windows PowerShell, run `iwr -useb https://raw.githubusercontent.com/"
        f"{OWNER.capitalize()}/{REPO}/main/skills/{dir_name}/install.ps1 | iex`. "
        f"{auth} and run `{cli} --help` to explore."
    )


def render_install_featured() -> str:
    """Path A of 'Install in 60 seconds': the agent-decides prompt first, then
    the top-2 featured connectors' prompts, then the swap-the-slug line."""
    featured = _featured_meta()[:2]
    parts = [
        "**Not sure which connector to pick? Let your agent decide.** "
        "Paste this into **Claude Code**, **Codex CLI**, or **Claude Cowork**:",
        "",
        f"> Read https://github.com/{OWNER.capitalize()}/{REPO} and, using everything "
        "you know about me and how my MSP works, recommend which connectors I should "
        "install - then install the ones I approve.",
        "",
        "Prefer a guided version? The [concierge](./skills/_meta) does the same from "
        f"inside Claude Code: `/plugin marketplace add {OWNER.capitalize()}/{REPO}`, then "
        "`/plugin install msp-skills-concierge@msp-skills`.",
        "",
    ]
    for i, (slug, meta) in enumerate(featured):
        display = meta.get("display_name") or meta.get("vendor", slug)
        lead = (
            f"**Or install a specific connector.** The **{display}** prompt:"
            if i == 0
            else f"And the **{display}** prompt:"
        )
        parts += [lead, "", _install_prompt(slug, meta), ""]
    parts.append(
        "The same prompt works for **every connector in the table above** - swap the "
        "skill name and slug. If your agent can't run shell, use Path B below."
    )
    return "\n".join(parts)


def render_agent_can_do() -> str:
    """The outcomes table: real rows pulled from each connector's page.json
    (the same prose its docs page renders from), so every connector is
    represented and every command is claims-gate-verified."""
    conns = _connector_meta()
    per_skill = 2 if len(conns) <= 6 else 1
    rows = [
        f"Outcomes, not hype - drawn from each of the {len(conns)} connectors' "
        "skill pages:",
        "",
        "| Outcome | Skill | Command |",
        "| --- | --- | --- |",
    ]
    for slug, meta in conns:
        page_path = registry.skill_path(slug) / "page.json"
        if not page_path.exists():
            raise SystemExit(
                f"skills/{meta.get('source_dir', slug)}/page.json missing - "
                "every connector needs one for the agent-can-do block."
            )
        outcomes = json.loads(page_path.read_text(encoding="utf-8")).get("outcomes", [])
        if not outcomes:
            raise SystemExit(f"page.json for '{slug}' has no outcomes.")
        for o in outcomes[:per_skill]:
            rows.append(f"| {o['question']} | {slug} | `{o['command']}` |")
    return "\n".join(rows)


def render_footer_releases(generated_at: str) -> str:
    """The footer line: per-connector latest release links from the registry
    versions (each version has a `<slug>-v<version>` tag by release contract)."""
    links = []
    for slug, meta in _connector_meta():
        version = meta.get("version")
        if not version:
            continue
        tag = f"{slug}-v{version}"
        links.append(
            f"[{tag}](https://github.com/{OWNER}/{REPO}/releases/tag/{tag})"
        )
    return f"_Last updated: {generated_at}. Latest releases: {' · '.join(links)}._"


def build_docs_catalog(skills: list[dict]) -> dict:
    """The Jekyll projection (docs/_data/catalog.json). The site renders its
    'What's in the box' table and install examples from this via Liquid, so
    the homepage can never again list fewer skills than the repo ships.
    No generated_at: content-only, so regeneration is a true no-op."""
    connectors = [
        {
            "slug": s["name"],
            "display_name": SKILL_META[s["name"]].get("display_name", s["name"]),
            "tagline": s.get("tagline", s.get("description", "")),
            "category": s.get("category", ""),
            "verification": s.get("verification", "awaiting"),
            # Named verification: who confirmed it against a production tenant,
            # and the public receipt (it-works issue) when one exists. Null
            # until verify_live.py records them - the site renders the names.
            "verified_by": (
                (SKILL_META[s["name"]].get("live_verified") or {}).get("verified_by")
            ),
            "issue_url": (
                (SKILL_META[s["name"]].get("live_verified") or {}).get("issue_url")
            ),
            "url": f"/skills/{s['name']}/",
        }
        for s in skills
        if not s.get("markdown_only")
    ]
    featured = [
        {
            "slug": slug,
            "display_name": meta.get("display_name") or meta.get("vendor", slug),
            "cli_binary": meta.get("cli_binary", f"{slug}-cli"),
            "skill_dir": meta.get("source_dir", slug),
            "install_auth_hint": meta.get(
                "install_auth_hint",
                f"Then run `{meta.get('cli_binary', slug + '-cli')} --help`",
            ),
        }
        for slug, meta in _featured_meta()
    ]
    return {"count": len(connectors), "connectors": connectors, "featured": featured}


def main() -> int:
    skill_dirs = sorted(p for p in SKILLS_DIR.iterdir() if p.is_dir())
    skills = [build_entry(d) for d in skill_dirs]

    # Preserve the existing generated_at when the substantive content is
    # unchanged, so re-running build-catalog.py is a TRUE no-op. The CI drift
    # gate (catalog.yml) compares the regenerated file byte-for-byte; a live
    # `dt.date.today()` stamp made every PR fail on any day other than the one
    # the catalog was last committed. Bump the date only when skills change.
    generated_at = dt.date.today().isoformat()
    if CATALOG.exists():
        try:
            prev = json.loads(CATALOG.read_text(encoding="utf-8"))
            if prev.get("schema_version") == 1 and prev.get("skills") == skills:
                generated_at = prev.get("generated_at", generated_at)
        except Exception:
            pass

    catalog = {
        "schema_version": 1,
        "generated_at": generated_at,
        "skills": skills,
    }
    CATALOG.write_text(json.dumps(catalog, indent=2) + "\n", encoding="utf-8")

    DOCS_CATALOG.parent.mkdir(parents=True, exist_ok=True)
    DOCS_CATALOG.write_text(json.dumps(build_docs_catalog(skills), indent=2) + "\n", encoding="utf-8")

    readme = README.read_text(encoding="utf-8")
    new_readme = replace_block(
        readme,
        "<!-- catalog:start -->",
        "<!-- catalog:end -->",
        render_catalog_table(skills),
    )
    for marker, block in (
        ("hero-live", render_hero_live()),
        ("install-featured", render_install_featured()),
        ("agent-can-do", render_agent_can_do()),
        ("footer-releases", render_footer_releases(generated_at)),
    ):
        new_readme = replace_block(
            new_readme, f"<!-- {marker}:start -->", f"<!-- {marker}:end -->", block
        )
    # The skills-count shield lives OUTSIDE the marker region; regenerate it
    # here too so the count can never go stale again (it sat at 2 while the
    # catalog grew). Count = connector skills (markdown-only meta excluded).
    connector_count = sum(1 for s in skills if not s.get("markdown_only"))
    new_readme = re.sub(
        r"img\.shields\.io/badge/skills-\d+-green",
        f"img.shields.io/badge/skills-{connector_count}-green",
        new_readme,
    )
    README.write_text(new_readme, encoding="utf-8")

    # Regenerate the it-works issue form's skill dropdown so a reporter can pick
    # the exact connector (not be forced into "other"); the catalog.yml drift
    # gate keeps it in lockstep with the registry. Skipped gracefully if the
    # form is absent (e.g. a partial checkout).
    if IT_WORKS_FORM.exists():
        form = IT_WORKS_FORM.read_text(encoding="utf-8")
        IT_WORKS_FORM.write_text(render_it_works_form(form, skills), encoding="utf-8")

    # Re-render every connector's docs page in the same pass, so a registry
    # change (live-verified flip, display rename) or a page.json edit shows up
    # on the site without a separate onboard.py run.
    for slug, _meta in _connector_meta():
        render_docs_page.render_page(slug)

    print(f"Regenerated catalog.json + docs/_data/catalog.json ({len(skills)} "
          f"skills, {connector_count} connectors), README generated blocks + "
          f"count badge, and {connector_count} docs skill pages.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
