#!/usr/bin/env python3
"""Finish a conflicted rebase or merge of a branch onto main without losing a registry entry.

Every connector pull request edits the same three SOURCE registries and the same
generated files, so two PRs that never touch each other's code still conflict on
those files the moment one of them merges. The old answer was a script that took
one side wholesale for anything it did not recognise (`git checkout --theirs`),
which is how a branch silently drops main's newest entries. This tool resolves
only what it can prove, and stops on everything else.

What it resolves
----------------
  source registries   tools/maintainer/skills.json, .claude-plugin/marketplace.json,
                      assets/social/cards.yaml. A three-way merge per ENTRY (per field
                      inside a JSON entry): an entry or field only one side changed
                      takes that side; both sides' new entries are kept. If both
                      sides changed the same field differently, that is a real
                      conflict and the tool stops.
  derived files       the files build-catalog.py and build-llms.py regenerate
                      (catalog.json, README.md, docs/_data/catalog.json,
                      docs/skills/*, docs/llms*.txt, the two issue forms) plus
                      docs/_data/pending.json: main's copy is taken, then the
                      generators re-run on the merged tree.
  skill READMEs       skills/<slug>/README.md ONLY when main's side differs from
                      the merge base solely in the `.mcpb` download pin (the
                      catalog bot's re-mint): the branch's text is kept and main's
                      pin is re-applied. Any other README conflict is unknown.

Everything else is an UNKNOWN conflict: the tool prints it, changes nothing, and
exits 2. It never takes one side of a file it does not understand.

Byte-safe output
----------------
Each registry is rewritten in the format main's copy already uses, detected (not
assumed) by round-tripping main's bytes: JSON indent and ASCII escaping, the
cards.yaml block layout. New entries are placed by each file's own convention,
measured on origin/main: skills.json is sorted by slug (onboard.py sorts it), so a
new slug is inserted in sorted order (falling back to append if main is not
sorted); marketplace.json is in onboarding order, so new plugins are appended;
cards.yaml is near-sorted, so a new card goes before the first card that sorts
after it. A file whose bytes do not round-trip is refused rather than reformatted.

After resolving it re-runs the generators, re-checks (every registry parses with
no duplicate keys, round-trips, skills.json and marketplace.json name the same
slugs, the repo's own check_marketplace_sync.py passes, a second generator run is
a no-op, no conflict markers remain) and prints the final registry diff against
main, so the result is inspected, not assumed.

Usage (inside the worktree with the conflict):
    python3 tools/maintainer/resolve_registry.py            # resolve, regenerate, re-check
    python3 tools/maintainer/resolve_registry.py --dry-run  # show the plan, change nothing
    python3 tools/maintainer/resolve_registry.py --main-side theirs   # a merge of main INTO the branch
    python3 tools/maintainer/resolve_registry.py --self-test

Then `git rebase --continue` (or `git commit` for a merge). The tool never
continues, commits, aborts, resets or cleans anything itself.

Exit codes: 0 resolved (or nothing to resolve); 2 unknown or real conflict, or a
refused precondition, with nothing changed; 3 the resolution was written but the
re-check failed (abort the rebase or merge: `git rebase --abort`).

Pure stdlib.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Callable

sys.path.insert(0, str(Path(__file__).resolve().parent))
import release_matrix  # noqa: E402  (local tools/ module; owns the derived-file list)

SKILLS_JSON = "tools/maintainer/skills.json"
MARKETPLACE_JSON = ".claude-plugin/marketplace.json"
CARDS_YAML = "assets/social/cards.yaml"
REGISTRY_FILES = (SKILLS_JSON, MARKETPLACE_JSON, CARDS_YAML)

# What build-catalog.py and build-llms.py regenerate from the tree. Derived from
# release_matrix's list (the classifier CI uses) so the two cannot drift;
# pending.json is release-derived (only catalog.yml regenerates it, from the
# published-release list), so main's copy is taken but no local run rewrites it.
RELEASE_DERIVED = ("docs/_data/pending.json",)
TREE_DERIVED_EXACT = tuple(p for p in release_matrix.DERIVED_EXACT if p not in RELEASE_DERIVED)
TREE_DERIVED_PREFIX = tuple(release_matrix.DERIVED_PREFIX)
GENERATORS = ("tools/maintainer/build-catalog.py", "tools/maintainer/build-llms.py")
SYNC_CHECK = "tools/maintainer/check_marketplace_sync.py"

_SKILL_README_RE = re.compile(r"^skills/([^/]+)/README\.md$")
_PIN_RE = release_matrix._MCPB_PIN_RE  # the generator's own pin pattern
_CONFLICT_MARKER_RE = re.compile(r"^(<<<<<<<|>>>>>>>) ", re.M)


def is_tree_derived(path: str) -> bool:
    return path in TREE_DERIVED_EXACT or any(path.startswith(p) for p in TREE_DERIVED_PREFIX)


def is_derived(path: str) -> bool:
    return is_tree_derived(path) or path in RELEASE_DERIVED


class UnknownFormat(Exception):
    """A registry whose bytes this tool cannot reproduce exactly."""


class MergeConflict(Exception):
    """Both sides changed the same registry field differently."""


# --------------------------------------------------------------------------- #
# Strict JSON and format detection
# --------------------------------------------------------------------------- #

def _no_dup_pairs(pairs: list[tuple[str, object]]) -> dict:
    out: dict = {}
    for k, v in pairs:
        if k in out:
            raise UnknownFormat(f"duplicate JSON key {k!r}")
        out[k] = v
    return out


def load_json_strict(text: str) -> object:
    """json.loads that refuses duplicate keys (a textual merge can create them,
    and json.loads would silently keep only the last one)."""
    try:
        return json.loads(text, object_pairs_hook=_no_dup_pairs)
    except ValueError as exc:
        if isinstance(exc, UnknownFormat):
            raise
        raise UnknownFormat(f"not valid JSON: {exc}") from exc


def json_format(text: str) -> tuple[int, bool, bool]:
    """(indent, ensure_ascii, trailing_newline) that reproduce `text` exactly.
    Raises UnknownFormat when no combination does: the file is then not ours
    to rewrite."""
    obj = load_json_strict(text)
    for nl in (True, False):
        for indent in (2, 1, 4, 3, 8):
            for ascii_ in (True, False):
                if dump_json(obj, (indent, ascii_, nl)) == text:
                    return indent, ascii_, nl
    raise UnknownFormat("its bytes do not round-trip through json.dumps "
                        "(unrecognised indent or escaping)")


def dump_json(obj: object, fmt: tuple[int, bool, bool]) -> str:
    indent, ascii_, nl = fmt
    return json.dumps(obj, indent=indent, ensure_ascii=ascii_) + ("\n" if nl else "")


def canon(value: object) -> str:
    """Canonical bytes of a JSON value: key order ignored, every string and
    number kept exactly (no whitespace normalisation inside strings, unlike
    `git patch-id`). Distinguishes true from 1 and 1 from 1.0."""
    return json.dumps(value, sort_keys=True, ensure_ascii=True, separators=(",", ":"))


# --------------------------------------------------------------------------- #
# cards.yaml as ordered text blocks (no YAML library: stdlib only)
# --------------------------------------------------------------------------- #

_KEY_LINE_RE = re.compile(r"^([A-Za-z0-9][A-Za-z0-9._-]*):[ \t]*$")


def parse_blocks(text: str) -> tuple[str, dict[str, str]]:
    """(header, {slug: block}) for a file of top-level `slug:` blocks separated
    by one blank line, after a free-form header. Refuses anything it could not
    write back byte for byte."""
    if not text.endswith("\n"):
        raise UnknownFormat("does not end with a newline")
    lines = text[:-1].split("\n")
    i = 0
    while i < len(lines) and not _KEY_LINE_RE.match(lines[i]):
        i += 1
    header = "\n".join(lines[:i]) + ("\n" if i else "")
    blocks: dict[str, str] = {}
    cur: str | None = None
    buf: list[str] = []

    def flush() -> None:
        if cur is None:
            return
        while buf and buf[-1] == "":
            buf.pop()
        blocks[cur] = "\n".join(buf)

    for line in lines[i:]:
        m = _KEY_LINE_RE.match(line)
        if m:
            flush()
            cur = m.group(1)
            if cur in blocks:
                raise UnknownFormat(f"duplicate top-level key {cur!r}")
            buf = [line]
        elif line and not line[0].isspace():
            raise UnknownFormat(f"unexpected top-level line {line!r}")
        else:
            buf.append(line)
    flush()
    if serialize_blocks(header, blocks) != text:
        raise UnknownFormat("its block layout does not round-trip (blank-line spacing)")
    return header, blocks


def serialize_blocks(header: str, blocks: dict[str, str]) -> str:
    body = "\n\n".join(blocks.values())
    return header + body + "\n" if body else header


# --------------------------------------------------------------------------- #
# Three-way merge of parsed values
# --------------------------------------------------------------------------- #

class _Absent:
    def __repr__(self) -> str:
        return "<absent>"


ABSENT = _Absent()


def _same(a: object, b: object) -> bool:
    if a is ABSENT or b is ABSENT:
        return a is b
    return canon(a) == canon(b)


def _insert(result: list[tuple[str, object]], key: str, value: object,
            policy: str, branch_keys: list[str]) -> None:
    keys = [k for k, _ in result]
    if policy == "sorted":
        for idx, k in enumerate(keys):
            if k > key:
                result.insert(idx, (key, value))
                return
        result.append((key, value))
        return
    if policy == "after-predecessor":
        pos = branch_keys.index(key)
        for pred in reversed(branch_keys[:pos]):
            if pred in keys:
                result.insert(keys.index(pred) + 1, (key, value))
                return
        result.insert(0, (key, value))
        return
    result.append((key, value))  # "append"


def merge_value(base: object, main: object, branch: object, where: str,
                policy: Callable[[str, dict], str]) -> object:
    """Three-way merge. Returns ABSENT for a deleted value."""
    if _same(main, branch):
        return main
    if _same(base, main):
        return branch
    if _same(base, branch):
        return main
    if isinstance(main, dict) and isinstance(branch, dict) and (
            base is ABSENT or isinstance(base, dict)):
        return merge_dict({} if base is ABSENT else base, main, branch, where, policy)
    raise MergeConflict(f"{where}: both sides changed it differently")


def merge_dict(base: dict, main: dict, branch: dict, where: str,
               policy: Callable[[str, dict], str]) -> dict:
    result: list[tuple[str, object]] = []
    for k in main:
        v = merge_value(base.get(k, ABSENT), main[k], branch.get(k, ABSENT), f"{where}.{k}", policy)
        if v is not ABSENT:
            result.append((k, v))
    branch_keys = list(branch)
    how = policy(where, main)
    for k in branch_keys:
        if k in main:
            continue
        v = merge_value(base.get(k, ABSENT), ABSENT, branch[k], f"{where}.{k}", policy)
        if v is not ABSENT:
            _insert(result, k, v, how, branch_keys)
    return dict(result)


def _skills_policy(where: str, main: dict) -> str:
    if where == "$.skills":
        keys = list(main)
        return "sorted" if keys == sorted(keys) else "append"
    return "after-predecessor"


def _marketplace_policy(where: str, main: dict) -> str:
    return "append" if where == "$.plugins" else "after-predecessor"


def _plugins_to_map(obj: object) -> dict:
    """marketplace.json with its plugins list keyed by name, so it merges per
    plugin. Refuses a list it cannot key (missing or duplicate names)."""
    if not isinstance(obj, dict):
        raise UnknownFormat("marketplace.json is not an object")
    plugins = obj.get("plugins")
    if not isinstance(plugins, list):
        raise UnknownFormat("marketplace.json has no plugins list")
    keyed: dict[str, object] = {}
    for p in plugins:
        name = p.get("name") if isinstance(p, dict) else None
        if not isinstance(name, str) or name in keyed:
            raise UnknownFormat(f"marketplace.json plugin without a unique name: {name!r}")
        keyed[name] = p
    out = dict(obj)
    out["plugins"] = keyed
    return out


def _plugins_to_list(obj: dict) -> dict:
    out = dict(obj)
    out["plugins"] = list(obj["plugins"].values())
    return out


def _merge_parsed(path: str, base: str | None, main: str, branch: str) -> object:
    """The per-entry three-way merge of one registry, as a parsed value (a
    (header, blocks) pair for cards.yaml). Format-free: nothing is written."""
    if path == CARDS_YAML:
        hm, bm = parse_blocks(main)
        hb, bb = parse_blocks(branch)
        hbase, bbase = parse_blocks(base) if base is not None else ("", {})
        header = merge_value(hbase if base is not None else ABSENT, hm, hb,
                             f"{path} header", lambda w, m: "append")
        blocks = merge_dict(bbase, bm, bb, path, lambda w, m: "sorted")
        return ("" if header is ABSENT else str(header), blocks)
    om, ob = load_json_strict(main), load_json_strict(branch)
    obase = load_json_strict(base) if base is not None else ABSENT
    if path == MARKETPLACE_JSON:
        om, ob = _plugins_to_map(om), _plugins_to_map(ob)
        obase = _plugins_to_map(obase) if obase is not ABSENT else ABSENT
        return _plugins_to_list(merge_value(obase, om, ob, "$", _marketplace_policy))  # type: ignore[arg-type]
    if path == SKILLS_JSON:
        return merge_value(obase, om, ob, "$", _skills_policy)
    raise UnknownFormat(f"{path} is not a registry this tool merges")


def merge_registry_text(path: str, base: str | None, main: str | None,
                        branch: str | None) -> str | None:
    """The merged bytes of one registry file, written in MAIN's format. None
    means the merged result is a deletion. Raises UnknownFormat or
    MergeConflict; never guesses."""
    if main is None and branch is None:
        return None
    if main is None or branch is None:
        raise MergeConflict(f"{path}: deleted on one side")
    if path == CARDS_YAML:
        header, blocks = _merge_parsed(path, base, main, branch)  # type: ignore[misc]
        return serialize_blocks(header, blocks)
    fmt = json_format(main)
    return dump_json(_merge_parsed(path, base, main, branch), fmt)


def registry_entries(path: str, text: str | None) -> dict[str, str]:
    """{entry key: canonical bytes} for one registry revision; a missing file
    is no entries. Used by merge_when_green.py's change fingerprint, so an
    entry's bytes count and its position in the file does not."""
    if text is None:
        return {}
    if path == CARDS_YAML:
        return _entries_of(path, parse_blocks(text))
    return _entries_of(path, load_json_strict(text))


def _entries_of(path: str, obj: object) -> dict[str, str]:
    if path == CARDS_YAML:
        header, blocks = obj  # type: ignore[misc]
        out = {f"block:{k}": v for k, v in blocks.items()}
        out["header"] = header
        return out
    if path == MARKETPLACE_JSON:
        obj = _plugins_to_map(obj)
        out = {f"plugin:{k}": canon(v) for k, v in obj["plugins"].items()}
        out.update({f"top:{k}": canon(v) for k, v in obj.items() if k != "plugins"})
        return out
    if path == SKILLS_JSON:
        if not isinstance(obj, dict) or not isinstance(obj.get("skills"), dict):
            raise UnknownFormat("skills.json has no skills object")
        out = {f"skill:{k}": canon(v) for k, v in obj["skills"].items()}
        out.update({f"top:{k}": canon(v) for k, v in obj.items() if k != "skills"})
        return out
    raise UnknownFormat(f"{path} is not a registry")


def registry_slugs(keys: set[str]) -> set[str]:
    """The connector slugs a set of registry entry keys names."""
    out = set()
    for k in keys:
        kind, _, name = k.partition(":")
        if kind in ("skill", "plugin", "block") and name:
            out.add(name)
    return out


def changed_entries(path: str, old: str | None, new: str | None) -> set[str]:
    a, b = registry_entries(path, old), registry_entries(path, new)
    return {k for k in set(a) | set(b) if a.get(k) != b.get(k)}


def resolve_skill_readme(base: str | None, main: str | None, branch: str | None) -> str | None:
    """The branch's README with main's `.mcpb` pin, when main changed nothing but
    the pin since the merge base. None when that is not the situation."""
    if base is None or main is None or branch is None:
        return None
    if not release_matrix.readme_pin_only(base, main) or base == main:
        return None
    main_tags = {m.group(2): m.group(0) for m in _PIN_RE.finditer(main)}
    out = _PIN_RE.sub(lambda m: main_tags.get(m.group(2), m.group(0)), branch)
    if not release_matrix.readme_pin_only(branch, out):
        return None
    if {m.group(0) for m in _PIN_RE.finditer(out)} != set(main_tags.values()):
        return None
    return out


# --------------------------------------------------------------------------- #
# Git plumbing
# --------------------------------------------------------------------------- #

def git(repo: Path, *args: str, check: bool = True, input_bytes: bytes | None = None,
        text: bool = True) -> subprocess.CompletedProcess:
    p = subprocess.run(["git", "-C", str(repo), *args], capture_output=True,
                       input=input_bytes if not text else None, text=text)
    if check and p.returncode != 0:
        err = p.stderr if text else p.stderr.decode("utf-8", "replace")
        raise RuntimeError(f"git {' '.join(args)} failed: {err.strip()}")
    return p


def git_dir(repo: Path) -> Path:
    d = Path(git(repo, "rev-parse", "--git-dir").stdout.strip())
    return d if d.is_absolute() else (repo / d).resolve()


def show_blob(repo: Path, rev_path: str) -> str | None:
    p = subprocess.run(["git", "-C", str(repo), "show", rev_path], capture_output=True)
    if p.returncode != 0:
        return None
    return p.stdout.decode("utf-8")


def detect_operation(repo: Path) -> str | None:
    g = git_dir(repo)
    if (g / "rebase-merge").is_dir() or (g / "rebase-apply").is_dir():
        return "rebase"
    if (g / "MERGE_HEAD").is_file():
        return "merge"
    if (g / "CHERRY_PICK_HEAD").is_file():
        return "cherry-pick"
    return None


def unmerged(repo: Path) -> dict[str, dict[int, str]]:
    out: dict[str, dict[int, str]] = {}
    raw = git(repo, "ls-files", "-u", "-z").stdout
    for rec in raw.split("\0"):
        if not rec:
            continue
        meta, path = rec.split("\t", 1)
        _mode, sha, stage = meta.split()
        out.setdefault(path, {})[int(stage)] = sha
    return out


def _stage_bytes(repo: Path, stages: dict[int, str], n: int) -> bytes | None:
    sha = stages.get(n)
    if sha is None:
        return None
    p = subprocess.run(["git", "-C", str(repo), "cat-file", "blob", sha], capture_output=True)
    if p.returncode != 0:
        raise RuntimeError(f"cannot read blob {sha}")
    return p.stdout


def _stage_text(repo: Path, stages: dict[int, str], n: int) -> str | None:
    """A stage as text; a blob that is not UTF-8 raises UnknownFormat (this
    tool never resolves a binary conflict)."""
    raw = _stage_bytes(repo, stages, n)
    if raw is None:
        return None
    try:
        return raw.decode("utf-8")
    except UnicodeDecodeError as exc:
        raise UnknownFormat("not UTF-8 text") from exc


def _three_commits(repo: Path, op: str, main_side: str) -> tuple[str, str, str] | None:
    """(base, main, branch) commits of the operation in progress, for
    verifying registries git merged cleanly. None when not determinable."""
    try:
        if op == "rebase":
            rh = git(repo, "rev-parse", "--verify", "-q", "REBASE_HEAD", check=False).stdout.strip()
            if not rh:
                return None
            base = git(repo, "rev-parse", f"{rh}^").stdout.strip()
            return base, git(repo, "rev-parse", "HEAD").stdout.strip(), rh
        if op == "merge":
            mh = git(repo, "rev-parse", "MERGE_HEAD").stdout.strip()
            bases = git(repo, "merge-base", "--all", "HEAD", mh).stdout.split()
            if len(bases) != 1:
                return None
            head = git(repo, "rev-parse", "HEAD").stdout.strip()
            return (bases[0], mh, head) if main_side == "theirs" else (bases[0], head, mh)
    except RuntimeError:
        return None
    return None


def verify_registries(repo: Path, base: str, main: str, branch: str,
                      result: str | None = None, skip: set[str] | None = None) -> list[str]:
    """Problems where a registry in `result` (a commit, or the worktree when
    None) is not the per-entry three-way merge of base/main/branch. Catches a
    textual merge that is valid JSON and still wrong, e.g. one slug added at two
    different positions (a duplicate key json.loads would silently collapse)."""
    problems = []
    for path in REGISTRY_FILES:
        if skip and path in skip:
            continue
        b, m, br = (show_blob(repo, f"{c}:{path}") for c in (base, main, branch))
        if result is None:
            f = repo / path
            have = f.read_text(encoding="utf-8") if f.is_file() else None
        else:
            have = show_blob(repo, f"{result}:{path}")
        # Only one side changed it: git's result must be that side, byte for
        # byte (no parsing, so a hand-formatted registry nobody touched passes).
        if br == b or m == b:
            if have != (m if br == b else br):
                problems.append(f"{path} (only one side changed it, but the result is neither side)")
            continue
        try:
            if m is None or br is None or have is None:
                raise MergeConflict("deleted on one side")
            want = _entries_of(path, _merge_parsed(path, b, m, br))
            if want != registry_entries(path, have):
                problems.append(f"{path} (git merged it textually, but not to the per-entry result)")
        except (UnknownFormat, MergeConflict) as exc:
            problems.append(f"{path} ({exc})")
    return problems


# --------------------------------------------------------------------------- #
# Regenerate + re-check
# --------------------------------------------------------------------------- #

def _porcelain(repo: Path) -> list[tuple[str, str]]:
    raw = git(repo, "status", "--porcelain=v1", "-z", "--untracked-files=all").stdout
    out = []
    recs = raw.split("\0")
    i = 0
    while i < len(recs):
        rec = recs[i]
        i += 1
        if not rec:
            continue
        xy, path = rec[:2], rec[3:]
        if xy[0] in "RC":
            i += 1  # skip the rename source
        out.append((xy, path))
    return out


def unstaged_outside(repo: Path, allowed: set[str]) -> list[str]:
    """Paths with unstaged or untracked changes other than `allowed`."""
    bad = []
    for xy, path in _porcelain(repo):
        if path in allowed:
            continue
        if xy == "??" or xy[1] != " ":
            bad.append(path)
    return bad


def regenerate_and_stage(repo: Path, out: Callable[[str], None] = print,
                         generators: tuple[str, ...] = GENERATORS) -> list[str]:
    """Run the repo's own generators, stage the tree-derived files they wrote,
    and put back any other file they touched (the skill-README `.mcpb` pins,
    which follow local git tags and belong to the catalog bot on main). Returns
    the derived paths that changed. Refuses if a generator leaves an untracked
    file that is not derived (nothing is deleted)."""
    before = {p for xy, p in _porcelain(repo) if xy == "??"}
    for gen in generators:
        if not (repo / gen).is_file():
            continue
        p = subprocess.run([sys.executable, gen], cwd=str(repo), capture_output=True, text=True)
        if p.returncode != 0:
            raise RuntimeError(f"{gen} failed: {(p.stdout + p.stderr).strip()[-2000:]}")
    derived, foreign, stray = [], [], []
    for xy, path in _porcelain(repo):
        if xy == "??" and path in before:
            continue
        if xy[1] == " " and xy != "??":
            continue  # staged only: not touched by the generators
        if is_tree_derived(path):
            derived.append(path)
        elif xy == "??":
            stray.append(path)
        else:
            foreign.append(path)
    if stray:
        raise RuntimeError("generators created untracked non-derived files: " + ", ".join(stray))
    if foreign:
        out("  restoring generator side effects outside the derived set: " + ", ".join(foreign))
        git(repo, "checkout", "--", *foreign)
    if derived:
        git(repo, "add", "-A", "--", *derived)
    return derived


def recheck(repo: Path, main_formats: dict[str, object] | None = None,
            generators: tuple[str, ...] = GENERATORS) -> list[str]:
    """Problems with the resolved tree; an empty list means it is sound."""
    problems: list[str] = []
    texts: dict[str, str | None] = {}
    for path in REGISTRY_FILES:
        f = repo / path
        texts[path] = f.read_text(encoding="utf-8") if f.is_file() else None
        t = texts[path]
        if t is None:
            continue
        if _CONFLICT_MARKER_RE.search(t):
            problems.append(f"{path}: conflict markers remain")
            continue
        try:
            if path == CARDS_YAML:
                parse_blocks(t)
            else:
                fmt = json_format(t)
                want = (main_formats or {}).get(path)
                if want is not None and fmt != want:
                    problems.append(f"{path}: format {fmt} differs from main's {want}")
        except UnknownFormat as exc:
            problems.append(f"{path}: {exc}")
    try:
        if texts[SKILLS_JSON] is not None and texts[MARKETPLACE_JSON] is not None:
            skills = load_json_strict(texts[SKILLS_JSON])["skills"]  # type: ignore[index]
            plugins = _plugins_to_map(load_json_strict(texts[MARKETPLACE_JSON]))["plugins"]
            if set(skills) != set(plugins):
                problems.append("skills.json and marketplace.json name different slugs: "
                                f"only in skills.json {sorted(set(skills) - set(plugins))}, "
                                f"only in marketplace.json {sorted(set(plugins) - set(skills))}")
    except (UnknownFormat, KeyError, TypeError) as exc:
        problems.append(f"registry invariant could not be read: {exc}")
    if (repo / SYNC_CHECK).is_file():
        p = subprocess.run([sys.executable, SYNC_CHECK], cwd=str(repo), capture_output=True, text=True)
        if p.returncode != 0:
            problems.append(f"{SYNC_CHECK} failed: {(p.stdout + p.stderr).strip()[-1500:]}")
    for path in (p for _xy, p in _porcelain(repo)):
        if is_derived(path) and (repo / path).is_file():
            if _CONFLICT_MARKER_RE.search((repo / path).read_text(encoding="utf-8", errors="replace")):
                problems.append(f"{path}: conflict markers remain")
    # A second generator run must change nothing: the derived files are a
    # function of the tree, not of how many times somebody ran the scripts.
    for gen in generators:
        if not (repo / gen).is_file():
            continue
        p = subprocess.run([sys.executable, gen], cwd=str(repo), capture_output=True, text=True)
        if p.returncode != 0:
            problems.append(f"{gen} failed on re-run: {(p.stdout + p.stderr).strip()[-800:]}")
    drift = [p for xy, p in _porcelain(repo) if is_tree_derived(p) and (xy == "??" or xy[1] != " ")]
    if drift:
        problems.append("a second generator run changed derived files: " + ", ".join(drift))
    foreign = [p for xy, p in _porcelain(repo)
               if not is_tree_derived(p) and xy != "??" and xy[1] != " "]
    if foreign:
        git(repo, "checkout", "--", *foreign)  # the generators' pin side effects again
    return problems


# --------------------------------------------------------------------------- #
# The resolver
# --------------------------------------------------------------------------- #

def resolve(repo: Path, main_side: str | None = None, regen: bool = True,
            dry_run: bool = False, out: Callable[[str], None] = print,
            generators: tuple[str, ...] = GENERATORS) -> int:
    repo = Path(repo)
    op = detect_operation(repo)
    if main_side is None:
        if op == "rebase":
            main_side = "ours"  # during a rebase, "ours" is the branch being rebased ONTO
        elif op == "merge":
            mh = git(repo, "rev-parse", "MERGE_HEAD").stdout.strip()
            om = git(repo, "rev-parse", "--verify", "-q", "refs/remotes/origin/main", check=False).stdout.strip()
            if om and mh == om:
                main_side = "theirs"
            else:
                out("REFUSE: a merge is in progress but MERGE_HEAD is not origin/main; "
                    "say which side is main with --main-side ours|theirs")
                return 2
        else:
            out("REFUSE: no rebase or merge in progress here (nothing to resolve)"
                if op is None else f"REFUSE: a {op} is in progress; pass --main-side explicitly")
            return 2
    main_stage, branch_stage = (2, 3) if main_side == "ours" else (3, 2)
    conflicts = unmerged(repo)
    dirty = unstaged_outside(repo, set(conflicts))
    if dirty:
        out("REFUSE: the worktree has changes outside the conflicted files "
            f"({', '.join(sorted(dirty)[:10])}); commit, stash or resolve them first")
        return 2

    plan: dict[str, tuple[str, str | bytes | None]] = {}
    unknown: list[str] = []
    main_formats: dict[str, object] = {}
    for path in sorted(conflicts):
        st = conflicts[path]
        try:
            if path in REGISTRY_FILES:
                base, main, branch = (_stage_text(repo, st, 1), _stage_text(repo, st, main_stage),
                                      _stage_text(repo, st, branch_stage))
                if main is not None and path != CARDS_YAML:
                    main_formats[path] = json_format(main)
                plan[path] = ("registry", merge_registry_text(path, base, main, branch))
            elif is_derived(path):
                plan[path] = ("main", _stage_bytes(repo, st, main_stage))
            elif _SKILL_README_RE.match(path):
                base, main, branch = (_stage_text(repo, st, 1), _stage_text(repo, st, main_stage),
                                      _stage_text(repo, st, branch_stage))
                text = resolve_skill_readme(base, main, branch)
                if text is None:
                    unknown.append(f"{path} (main changed more than the .mcpb pin)")
                else:
                    plan[path] = ("readme-pin", text)
            else:
                unknown.append(path)
        except (UnknownFormat, MergeConflict) as exc:
            unknown.append(f"{path} ({exc})")

    # Registries git merged without a conflict are still verified per entry:
    # a textual merge can be valid JSON and still wrong (a duplicate slug).
    trio = _three_commits(repo, op, main_side) if op in ("rebase", "merge") else None
    if trio is not None:
        unknown.extend(verify_registries(repo, *trio, skip=set(conflicts)))

    if unknown:
        out("UNKNOWN CONFLICT - nothing was changed. Resolve these by hand:")
        for u in unknown:
            out(f"  - {u}")
        return 2
    if not conflicts:
        out("no conflicted files")
    for path, (kind, _text) in plan.items():
        label = {"registry": "per-entry three-way merge", "main": "main's copy (derived)",
                 "readme-pin": "branch text + main's .mcpb pin"}[kind]
        out(f"  {path}: {label}")
    if dry_run:
        out("dry run: nothing written")
        return 0

    for path, (_kind, text) in plan.items():
        f = repo / path
        if text is None:
            git(repo, "rm", "-q", "--cached", "--ignore-unmatch", "--", path)
            if f.exists():
                f.unlink()
            continue
        f.parent.mkdir(parents=True, exist_ok=True)
        f.write_bytes(text if isinstance(text, bytes) else text.encode("utf-8"))
        git(repo, "add", "--", path)

    if regen:
        try:
            changed = regenerate_and_stage(repo, out, generators)
        except RuntimeError as exc:
            out(f"RE-CHECK FAILED: {exc}")
            return 3
        out(f"  regenerated derived files: {', '.join(changed) if changed else 'no change'}")
        problems = recheck(repo, main_formats, generators)
        if problems:
            out("RE-CHECK FAILED (the resolution is staged; abort the rebase or merge):")
            for pr in problems:
                out(f"  - {pr}")
            return 3
    # The side that is main, as a commit: HEAD during a rebase (the commit being
    # built on) or a merge whose "ours" is main; MERGE_HEAD when main is merged in.
    main_ref = "MERGE_HEAD" if (op == "merge" and main_side == "theirs") else "HEAD"
    diff = git(repo, "diff", "--cached", "--stat", main_ref, "--", *REGISTRY_FILES).stdout
    full = git(repo, "diff", "--cached", main_ref, "--", *REGISTRY_FILES).stdout
    out("final registry diff against main:")
    out(diff.rstrip() or "  (no registry change)")
    if full.strip():
        out(full.rstrip())
    out("RESOLVED" + ("; next: git rebase --continue" if op == "rebase" else
                      "; next: git commit" if op == "merge" else ""))
    return 0


# --------------------------------------------------------------------------- #
# --self-test: throwaway repositories, every case both directions
# --------------------------------------------------------------------------- #

FIXTURE_CATALOG = '''#!/usr/bin/env python3
"""Fixture stand-in for build-catalog.py: derived files are a pure function of
the registry, and it also re-mints a skill README pin from local tags, the side
effect the real generator has."""
import json, pathlib, subprocess
root = pathlib.Path(__file__).resolve().parents[2]
reg = json.loads((root / "tools/maintainer/skills.json").read_text())
slugs = sorted(reg["skills"])
(root / "catalog.json").write_text(json.dumps({"skills": slugs}, indent=2) + "\\n")
(root / "README.md").write_text("# fixture\\n\\n" + "".join(f"- {s}\\n" for s in slugs))
tags = subprocess.run(["git", "-C", str(root), "tag", "--list", "zz-*"], capture_output=True, text=True).stdout.split()
r = root / "skills/alpha/README.md"
if tags and r.is_file():
    t = r.read_text()
    r.write_text(t.replace("download/alpha-v0.1.0/", "download/" + sorted(tags)[-1] + "/"))
'''
FIXTURE_LLMS = '''#!/usr/bin/env python3
import json, pathlib
root = pathlib.Path(__file__).resolve().parents[2]
reg = json.loads((root / "tools/maintainer/skills.json").read_text())
(root / "docs").mkdir(exist_ok=True)
(root / "docs/llms.txt").write_text("".join(f"{s}: {reg['skills'][s]['tagline']}\\n" for s in sorted(reg["skills"])))
'''
FIXTURE_SYNC = '''#!/usr/bin/env python3
import json, pathlib, sys
root = pathlib.Path(__file__).resolve().parents[2]
a = set(json.loads((root / "tools/maintainer/skills.json").read_text())["skills"])
b = {p["name"] for p in json.loads((root / ".claude-plugin/marketplace.json").read_text())["plugins"]}
sys.exit(0 if a == b else 1)
'''
CARDS_HEADER = "# Per-vendor social-card content (fixture).\n# ASCII hyphens only.\n\n"


def fixture_env(home: Path) -> dict:
    """An environment that ignores the operator's git config (signing, hooks,
    editors) so a self-test behaves the same on every machine."""
    gc = home / "gitconfig"
    gc.write_text("[user]\n\tname = Self Test\n\temail = selftest@example.invalid\n"
                  "[init]\n\tdefaultBranch = main\n[advice]\n\tdetachedHead = false\n")
    env = dict(os.environ)
    env.update({"GIT_CONFIG_GLOBAL": str(gc), "GIT_CONFIG_NOSYSTEM": "1", "HOME": str(home),
                "GIT_EDITOR": "true", "GIT_TERMINAL_PROMPT": "0"})
    return env


def _entry(slug: str, tagline: str) -> dict:
    return {"system": f"{slug} fixture", "status": "beta", "cli_binary": f"{slug}-cli",
            "mcp_binary": f"{slug}-mcp", "tagline": tagline, "version": "0.1.0",
            "cli_hash_at_release": None}


def _plugin(slug: str) -> dict:
    return {"name": slug, "source": f"./skills/{slug}", "description": f"{slug} fixture",
            "version": "0.1.0", "tags": [slug, "msp"]}


def write_registries(root: Path, slugs: list[str], plugin_order: list[str] | None = None,
                     indent: int = 2, taglines: dict[str, str] | None = None) -> None:
    taglines = taglines or {}
    (root / "tools/maintainer").mkdir(parents=True, exist_ok=True)
    (root / ".claude-plugin").mkdir(exist_ok=True)
    (root / "assets/social").mkdir(parents=True, exist_ok=True)
    skills = {s: _entry(s, taglines.get(s, f"{s} does things")) for s in slugs}
    (root / SKILLS_JSON).write_text(json.dumps({"owner": "o", "repo": "r", "skills": skills},
                                               indent=indent, ensure_ascii=True) + "\n")
    mp = {"name": "fixture", "plugins": [_plugin(s) for s in (plugin_order or slugs)]}
    (root / MARKETPLACE_JSON).write_text(json.dumps(mp, indent=2, ensure_ascii=True) + "\n")
    blocks = {s: f'{s}:\n  vendor: "{s.title()}"\n  example_command: "What about {s}?"'
              for s in sorted(slugs)}
    (root / CARDS_YAML).write_text(serialize_blocks(CARDS_HEADER, blocks))


def make_fixture_repo(root: Path, env: dict, slugs: tuple[str, ...] = ("alpha", "delta")) -> Path:
    """A miniature msp-skills: three registries, stand-in generators, two
    connectors. Returns the work clone path; `root/origin.git` is its origin."""
    origin = root / "origin.git"
    work = root / "work"
    subprocess.run(["git", "init", "-q", "--bare", "-b", "main", str(origin)], check=True, env=env)
    subprocess.run(["git", "init", "-q", "-b", "main", str(work)], check=True, env=env)
    write_registries(work, list(slugs))
    tm = work / "tools/maintainer"
    (tm / "build-catalog.py").write_text(FIXTURE_CATALOG)
    (tm / "build-llms.py").write_text(FIXTURE_LLMS)
    (tm / "check_marketplace_sync.py").write_text(FIXTURE_SYNC)
    for s in slugs:
        d = work / "skills" / s
        (d / "cli").mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {s}\n\nline one\nline two\nline three\n")
        (d / "cli" / "main.go").write_text(f'package main\n\nconst name = "{s}"\n')
        (d / "README.md").write_text(
            f"# {s}\n\nIntro prose.\n\n[Download](https://github.com/{release_matrix._OWNER}/"
            f"{release_matrix._REPO}/releases/download/{s}-v0.1.0/{s}-mcp.mcpb) now.\n\nMore prose.\n")
    (work / "docs").mkdir(exist_ok=True)
    (work / "docs/guide.md").write_text("".join(f"guide line {i}\n" for i in range(1, 30)))
    for gen in GENERATORS:
        subprocess.run([sys.executable, gen], cwd=str(work), check=True, env=env)
    g = lambda *a: subprocess.run(["git", "-C", str(work), *a], check=True, env=env,  # noqa: E731
                                  capture_output=True)
    g("add", "-A")
    g("commit", "-q", "-s", "-m", "fixture: initial")
    g("remote", "add", "origin", str(origin))
    g("push", "-q", "origin", "main")
    g("fetch", "-q", "origin")
    return work


def add_connector(work: Path, env: dict, slug: str, message: str | None = None) -> None:
    """What an onboard does to the registries: sorted insert in skills.json,
    append in marketplace.json, sorted insert in cards.yaml; then regenerate."""
    reg = json.loads((work / SKILLS_JSON).read_text())
    reg["skills"][slug] = _entry(slug, f"{slug} does things")
    reg["skills"] = {k: reg["skills"][k] for k in sorted(reg["skills"])}
    (work / SKILLS_JSON).write_text(json.dumps(reg, indent=2, ensure_ascii=True) + "\n")
    mp = json.loads((work / MARKETPLACE_JSON).read_text())
    mp["plugins"].append(_plugin(slug))
    (work / MARKETPLACE_JSON).write_text(json.dumps(mp, indent=2, ensure_ascii=True) + "\n")
    header, blocks = parse_blocks((work / CARDS_YAML).read_text())
    blocks[slug] = f'{slug}:\n  vendor: "{slug.title()}"\n  example_command: "What about {slug}?"'
    blocks = {k: blocks[k] for k in sorted(blocks)}
    (work / CARDS_YAML).write_text(serialize_blocks(header, blocks))
    d = work / "skills" / slug
    (d / "cli").mkdir(parents=True, exist_ok=True)
    (d / "SKILL.md").write_text(f"# {slug}\n")
    (d / "cli" / "main.go").write_text(f'package main\n\nconst name = "{slug}"\n')
    for gen in GENERATORS:
        subprocess.run([sys.executable, gen], cwd=str(work), check=True, env=env)
    subprocess.run(["git", "-C", str(work), "add", "-A"], check=True, env=env)
    subprocess.run(["git", "-C", str(work), "commit", "-q", "-s", "-m",
                    message or f"skill: {slug}"], check=True, env=env)


def _self_test() -> int:
    failures: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    print("format detection and merge primitives:")
    for indent in (1, 2, 4):
        t = json.dumps({"a": {"b": "\u00e9 x"}}, indent=indent, ensure_ascii=True) + "\n"
        expect(json_format(t) == (indent, True, True), f"indent={indent} ASCII-escaped JSON round-trips")
    t = json.dumps({"a": "\u00e9"}, indent=2, ensure_ascii=False) + "\n"
    expect(json_format(t) == (2, False, True), "non-ASCII JSON keeps its raw characters")
    try:
        json_format('{"a": 1,  "b": 2}\n')
        expect(False, "hand-spaced JSON is refused, not reformatted")
    except UnknownFormat:
        expect(True, "hand-spaced JSON is refused, not reformatted")
    try:
        load_json_strict('{"a": 1, "a": 2}')
        expect(False, "a duplicate key (a textual merge artefact) is refused")
    except UnknownFormat:
        expect(True, "a duplicate key (a textual merge artefact) is refused")
    expect(canon({"x": "a  b"}) != canon({"x": "a b"}),
           "whitespace inside a string changes the canonical bytes (unlike git patch-id)")
    expect(canon(True) != canon(1) and canon(1) != canon(1.0), "true, 1 and 1.0 stay distinct")
    try:
        parse_blocks("a:\n  x: 1\n\n\nb:\n  y: 2\n")
        expect(False, "a cards.yaml with double blank lines is refused")
    except UnknownFormat:
        expect(True, "a cards.yaml with double blank lines is refused")
    try:
        merge_value({"v": 1}, {"v": 2}, {"v": 3}, "$", _skills_policy)
        expect(False, "both sides changing one field differently is a conflict")
    except MergeConflict:
        expect(True, "both sides changing one field differently is a conflict")
    expect(merge_value({"v": 1, "w": 1}, {"v": 2, "w": 1}, {"v": 1, "w": 3}, "$", _skills_policy)
           == {"v": 2, "w": 3}, "different fields of one entry merge field by field")

    tmp = Path(tempfile.mkdtemp(prefix="resolve-registry-selftest-"))
    try:
        env = fixture_env(tmp)
        os.environ.update({k: env[k] for k in ("GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "GIT_EDITOR")})

        def run(*a: str, cwd: Path, check: bool = True) -> subprocess.CompletedProcess:
            return subprocess.run(list(a), cwd=str(cwd), env=env, capture_output=True, text=True, check=check)

        def scenario(name: str) -> Path:
            d = tmp / name
            d.mkdir()
            return make_fixture_repo(d, env)

        quiet: Callable[[str], None] = lambda s: None  # noqa: E731

        # 1. Known conflict: main and the branch each add a connector.
        print("known registry conflict (both sides onboard a connector):")
        w = scenario("known")
        run("git", "checkout", "-q", "-b", "skill/bravo", cwd=w)
        add_connector(w, env, "bravo")
        run("git", "checkout", "-q", "main", cwd=w)
        add_connector(w, env, "charlie")
        add_connector(w, env, "zulu")
        run("git", "checkout", "-q", "skill/bravo", cwd=w)
        rb = run("git", "rebase", "main", cwd=w, check=False)
        expect(rb.returncode != 0 and bool(unmerged(w)), "the rebase stops on registry conflicts")
        # A local tag makes the stand-in generator re-mint a skill README pin,
        # as the real one does from `git tag`: that side effect must not land.
        run("git", "tag", "zz-alpha-v9", cwd=w)
        logs: list[str] = []
        rc = resolve(w, out=logs.append)
        expect(rc == 0, f"resolver resolves it (rc={rc})")
        expect(run("git", "status", "--porcelain", "--", "skills/alpha/README.md", cwd=w).stdout == "",
               "a generator's skill-README pin side effect is put back, not staged")
        reg = json.loads((w / SKILLS_JSON).read_text())
        expect(list(reg["skills"]) == ["alpha", "bravo", "charlie", "delta", "zulu"],
               "skills.json keeps both sides' entries, in sorted order")
        mp = [p["name"] for p in json.loads((w / MARKETPLACE_JSON).read_text())["plugins"]]
        expect(mp == ["alpha", "delta", "charlie", "zulu", "bravo"],
               "marketplace.json keeps main's order and appends the branch's plugin")
        _h, cards = parse_blocks((w / CARDS_YAML).read_text())
        expect(list(cards) == ["alpha", "bravo", "charlie", "delta", "zulu"],
               "cards.yaml keeps both cards, the new one in sorted position")
        expect(json_format((w / SKILLS_JSON).read_text()) == (2, True, True),
               "skills.json is written in main's exact format")
        expect(json.loads((w / "catalog.json").read_text())["skills"]
               == ["alpha", "bravo", "charlie", "delta", "zulu"],
               "derived files are regenerated from the merged registries")
        expect(any("final registry diff" in s for s in logs) and any("+    \"bravo\"" in s for s in logs),
               "the final registry diff is printed and shows the branch's entry")
        cont = run("git", "rebase", "--continue", cwd=w, check=False)
        expect(cont.returncode == 0, "the rebase continues cleanly afterwards")

        # 2. Unknown conflict: both sides edit the same SKILL.md line.
        print("unknown conflict (hand-written file):")
        w = scenario("unknown")
        run("git", "checkout", "-q", "-b", "fix/alpha", cwd=w)
        (w / "skills/alpha/SKILL.md").write_text("# alpha\n\nline one\nBRANCH\nline three\n")
        run("git", "commit", "-q", "-s", "-am", "fix", cwd=w)
        run("git", "checkout", "-q", "main", cwd=w)
        (w / "skills/alpha/SKILL.md").write_text("# alpha\n\nline one\nMAIN\nline three\n")
        add_connector(w, env, "charlie")
        run("git", "commit", "-q", "-s", "-am", "main edit", cwd=w, check=False)
        run("git", "checkout", "-q", "fix/alpha", cwd=w)
        run("git", "rebase", "main", cwd=w, check=False)
        before = (w / "skills/alpha/SKILL.md").read_text()
        idx_before = run("git", "ls-files", "-u", cwd=w).stdout
        logs = []
        rc = resolve(w, out=logs.append)
        expect(rc == 2, f"an unknown conflict stops the resolver (rc={rc})")
        expect((w / "skills/alpha/SKILL.md").read_text() == before
               and run("git", "ls-files", "-u", cwd=w).stdout == idx_before,
               "nothing was changed: the conflict and the index are exactly as git left them")
        expect(any("skills/alpha/SKILL.md" in s for s in logs), "the unknown path is named")

        # 3. Real registry conflict: both sides change one field differently.
        print("same field changed differently on both sides:")
        w = scenario("field")
        run("git", "checkout", "-q", "-b", "fix/tagline", cwd=w)
        write_registries(w, ["alpha", "delta"], taglines={"alpha": "branch tagline"})
        run("git", "commit", "-q", "-s", "-am", "branch tagline", cwd=w)
        run("git", "checkout", "-q", "main", cwd=w)
        write_registries(w, ["alpha", "delta"], taglines={"alpha": "main tagline"})
        run("git", "commit", "-q", "-s", "-am", "main tagline", cwd=w)
        run("git", "checkout", "-q", "fix/tagline", cwd=w)
        run("git", "rebase", "main", cwd=w, check=False)
        before = (w / SKILLS_JSON).read_text()
        rc = resolve(w, out=quiet)
        expect(rc == 2 and (w / SKILLS_JSON).read_text() == before,
               "a same-field conflict is refused and nothing is written")
        run("git", "rebase", "--abort", cwd=w)
        # ... and the healthy direction: different fields of the same entry.
        run("git", "checkout", "-q", "-b", "fix/version", "main~1", cwd=w)
        reg = json.loads((w / SKILLS_JSON).read_text())
        reg["skills"]["alpha"]["version"] = "0.2.0"
        (w / SKILLS_JSON).write_text(json.dumps(reg, indent=2, ensure_ascii=True) + "\n")
        run("git", "commit", "-q", "-s", "-am", "bump", cwd=w)
        rb = run("git", "rebase", "main", cwd=w, check=False)
        rc = 0 if rb.returncode == 0 and not unmerged(w) else resolve(w, out=quiet)
        reg = json.loads((w / SKILLS_JSON).read_text())["skills"]["alpha"]
        expect(rc == 0 and reg["version"] == "0.2.0" and reg["tagline"] == "main tagline",
               "different fields of the same entry keep both sides' changes")
        run("git", "rebase", "--abort", cwd=w, check=False)

        # 4. Format conventions: an indent-4 registry stays indent 4; an unsorted
        #    skills map takes appends instead of sorted inserts.
        print("format and ordering conventions are read from main, not assumed:")
        w = scenario("format")
        write_registries(w, ["delta", "alpha"], indent=4)
        for gen in GENERATORS:
            run(sys.executable, gen, cwd=w)
        run("git", "commit", "-q", "-s", "-am", "unsorted, indent 4", cwd=w)
        run("git", "checkout", "-q", "-b", "skill/bravo", cwd=w)
        reg = json.loads((w / SKILLS_JSON).read_text())
        reg["skills"]["bravo"] = _entry("bravo", "b")
        (w / SKILLS_JSON).write_text(json.dumps(reg, indent=4, ensure_ascii=True) + "\n")
        mp = json.loads((w / MARKETPLACE_JSON).read_text())
        mp["plugins"].append(_plugin("bravo"))
        (w / MARKETPLACE_JSON).write_text(json.dumps(mp, indent=2, ensure_ascii=True) + "\n")
        run("git", "commit", "-q", "-s", "-am", "bravo", cwd=w)
        run("git", "checkout", "-q", "main", cwd=w)
        reg = json.loads((w / SKILLS_JSON).read_text())
        reg["skills"]["charlie"] = _entry("charlie", "c")
        (w / SKILLS_JSON).write_text(json.dumps(reg, indent=4, ensure_ascii=True) + "\n")
        mp = json.loads((w / MARKETPLACE_JSON).read_text())
        mp["plugins"].append(_plugin("charlie"))
        (w / MARKETPLACE_JSON).write_text(json.dumps(mp, indent=2, ensure_ascii=True) + "\n")
        run("git", "commit", "-q", "-s", "-am", "charlie", cwd=w)
        run("git", "checkout", "-q", "skill/bravo", cwd=w)
        run("git", "rebase", "main", cwd=w, check=False)
        rc = resolve(w, out=quiet)
        t = (w / SKILLS_JSON).read_text()
        expect(rc == 0 and json_format(t) == (4, True, True), "an indent-4 skills.json stays indent 4")
        expect(list(json.loads(t)["skills"]) == ["delta", "alpha", "charlie", "bravo"],
               "an unsorted skills map gets the new slug appended, not sorted in")
        run("git", "rebase", "--abort", cwd=w, check=False)

        # 5. Skill README: main re-minted only the pin; the branch edited prose.
        print("skill README conflicts:")
        for case, main_edit, want_rc in (
                ("pin-only", lambda s: s.replace("alpha-v0.1.0", "alpha-v0.1.1"), 0),
                ("prose", lambda s: s.replace("Intro prose.", "Main intro."), 2)):
            w = scenario(f"readme-{case}")
            readme = w / "skills/alpha/README.md"
            orig = readme.read_text()
            run("git", "checkout", "-q", "-b", "fix/readme", cwd=w)
            readme.write_text(orig.replace("Intro prose.", "Branch intro.").replace(") now.", ") today."))
            run("git", "commit", "-q", "-s", "-am", "branch prose", cwd=w)
            run("git", "checkout", "-q", "main", cwd=w)
            readme.write_text(main_edit(orig))
            run("git", "commit", "-q", "-s", "-am", "main", cwd=w)
            run("git", "checkout", "-q", "fix/readme", cwd=w)
            run("git", "rebase", "main", cwd=w, check=False)
            conflicted = "skills/alpha/README.md" in unmerged(w)
            rc = resolve(w, out=quiet, regen=False)
            if case == "pin-only":
                got = readme.read_text()
                expect(conflicted and rc == 0 and "Branch intro." in got and "today." in got
                       and "alpha-v0.1.1" in got and "alpha-v0.1.0" not in got,
                       "a pin-only change on main keeps the branch's prose and main's pin")
            else:
                expect(conflicted and rc == want_rc, "a prose change on main is an unknown conflict")
            run("git", "rebase", "--abort", cwd=w, check=False)

        # 6. A merge of main INTO the branch (main is "theirs").
        print("merge of main into the branch:")
        w = scenario("merge")
        run("git", "checkout", "-q", "-b", "skill/bravo", cwd=w)
        add_connector(w, env, "bravo")
        run("git", "checkout", "-q", "main", cwd=w)
        add_connector(w, env, "charlie")
        run("git", "push", "-q", "origin", "main", cwd=w)
        run("git", "fetch", "-q", "origin", cwd=w)
        run("git", "checkout", "-q", "skill/bravo", cwd=w)
        run("git", "merge", "--no-edit", "origin/main", cwd=w, check=False)
        rc = resolve(w, out=quiet)
        mp = [p["name"] for p in json.loads((w / MARKETPLACE_JSON).read_text())["plugins"]]
        expect(rc == 0 and mp == ["alpha", "delta", "charlie", "bravo"],
               "main is detected as the merged-in side and its order is kept")

        # 6b. A CLEAN textual merge that is still wrong: both sides add the same
        #     slug at different positions, so git merges without a conflict and
        #     the JSON carries the key twice.
        print("a conflict-free textual merge is still verified per entry:")
        w = scenario("dupkey")
        run("git", "checkout", "-q", "-b", "skill/bravo", cwd=w)
        reg = json.loads((w / SKILLS_JSON).read_text())
        reg["skills"]["bravo"] = _entry("bravo", "branch bravo")   # appended at the end
        (w / SKILLS_JSON).write_text(json.dumps(reg, indent=2, ensure_ascii=True) + "\n")
        run("git", "commit", "-q", "-s", "-am", "bravo (branch)", cwd=w)
        run("git", "checkout", "-q", "main", cwd=w)
        reg = json.loads((w / SKILLS_JSON).read_text())
        reg["skills"]["bravo"] = _entry("bravo", "main bravo")
        reg["skills"] = {k: reg["skills"][k] for k in sorted(reg["skills"])}  # sorted in
        (w / SKILLS_JSON).write_text(json.dumps(reg, indent=2, ensure_ascii=True) + "\n")
        run("git", "commit", "-q", "-s", "-am", "bravo (main)", cwd=w)
        main_sha = run("git", "rev-parse", "main", cwd=w).stdout.strip()
        run("git", "checkout", "-q", "skill/bravo", cwd=w)
        base_sha = run("git", "merge-base", "main", "HEAD", cwd=w).stdout.strip()
        branch_sha = run("git", "rev-parse", "HEAD", cwd=w).stdout.strip()
        rb = run("git", "rebase", "main", cwd=w, check=False)
        probs = verify_registries(w, base_sha, main_sha, branch_sha, "HEAD")
        expect(rb.returncode == 0 and any(SKILLS_JSON in p for p in probs),
               "git rebased it cleanly, and the per-entry check still flags skills.json")
        expect(verify_registries(w, base_sha, main_sha, main_sha, main_sha) == [],
               "... while a registry that IS the per-entry result passes")
        # A registry only main changed, in a hand-formatted (non round-tripping)
        # layout: verified byte for byte, not refused for its format.
        run("git", "rebase", "--abort", cwd=w, check=False)
        run("git", "checkout", "-q", "main", cwd=w)
        raw = (w / SKILLS_JSON).read_text().replace('"tags"', '"tags"')
        (w / SKILLS_JSON).write_text(raw.replace('"owner": "o",', '"owner":   "o",'))
        run("git", "commit", "-q", "-s", "-am", "hand-formatted", cwd=w)
        hand = run("git", "rev-parse", "HEAD", cwd=w).stdout.strip()
        expect(verify_registries(w, main_sha, hand, main_sha, hand) == [],
               "a hand-formatted registry only one side changed is compared byte for byte, not refused")
        expect(verify_registries(w, main_sha, hand, main_sha, main_sha) != [],
               "... and a result that is neither side is still caught")

        # 7. A worktree with unrelated unstaged edits is refused.
        print("preconditions:")
        w = scenario("dirty")
        run("git", "checkout", "-q", "-b", "skill/bravo", cwd=w)
        add_connector(w, env, "bravo")
        run("git", "checkout", "-q", "main", cwd=w)
        add_connector(w, env, "charlie")
        run("git", "checkout", "-q", "skill/bravo", cwd=w)
        run("git", "rebase", "main", cwd=w, check=False)
        (w / "docs/guide.md").write_text("an unrelated edit\n")
        rc = resolve(w, out=quiet)
        expect(rc == 2, "unstaged changes outside the conflict are refused")
        run("git", "checkout", "--", "docs/guide.md", cwd=w)
        expect(resolve(w, out=quiet) == 0, "... and the same conflict resolves once they are gone")
        expect(resolve(scenario("idle"), out=quiet) == 2, "no rebase or merge in progress is refused")

        # 8. regenerate_and_stage on its own (merge_when_green calls it after a
        #    conflict-free rebase): the pin it re-mints from a local tag is put
        #    back, and an unchanged tree stages nothing.
        print("regenerating after a clean rebase:")
        w = scenario("regen")
        run("git", "tag", "zz-alpha-v9", cwd=w)
        changed = regenerate_and_stage(w, quiet)
        expect(changed == [] and run("git", "status", "--porcelain", cwd=w).stdout == "",
               "a generator's skill-README pin side effect is put back and nothing is staged")
        write_registries(w, ["alpha", "delta", "echo"])
        run("git", "add", "-A", cwd=w)
        changed = regenerate_and_stage(w, quiet)
        expect("catalog.json" in changed and "README.md" in changed,
               "a registry change stages the derived files it changes")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

    if failures:
        print(f"\nFAIL: resolve_registry self-test - {len(failures)} case(s):")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("\nPASS: resolve_registry - every case discriminates")
    return 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", default=".", help="worktree with the conflict (default: current directory)")
    ap.add_argument("--main-side", choices=("ours", "theirs"),
                    help="which side of the conflict is main (auto: ours in a rebase, "
                         "theirs in a merge of origin/main)")
    ap.add_argument("--no-regen", action="store_true", help="resolve only; skip the generators and re-check")
    ap.add_argument("--dry-run", action="store_true", help="print the plan, write nothing")
    ap.add_argument("--self-test", action="store_true", help="prove every case both directions")
    args = ap.parse_args(argv)
    if args.self_test:
        return _self_test()
    repo = Path(git(Path(args.repo), "rev-parse", "--show-toplevel").stdout.strip())
    return resolve(repo, args.main_side, regen=not args.no_regen, dry_run=args.dry_run)


if __name__ == "__main__":
    sys.exit(main())
