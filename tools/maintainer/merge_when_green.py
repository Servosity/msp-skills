#!/usr/bin/env python3
"""Merge one pull request only when it is green on the exact commit, on current main.

    python3 tools/maintainer/merge_when_green.py 412            # do it
    python3 tools/maintainer/merge_when_green.py 412 --dry-run  # print every step, push and merge nothing
    python3 tools/maintainer/merge_when_green.py approve 412 --reviewer codex --head <sha> --evidence verdict.txt
    python3 tools/maintainer/merge_when_green.py status 412
    python3 tools/maintainer/merge_when_green.py --self-test

Run it from an up-to-date checkout of main, not from the branch being merged: it
executes its own code. (It does run the branch's own build-catalog.py and
build-llms.py, in a worktree it creates, because those are what CI's drift gate
runs on the branch; fork pull requests are refused for that reason.)

What it does, in order
----------------------
 1. Takes a per-branch ownership lock (repo_lock.py, in the clone's shared git
    dir), so two runs never drive one branch.
 2. Refuses if any worktree that has the branch checked out is dirty, mid-rebase,
    in use by another live process (a shell or agent whose working directory is
    inside it), or holds commits the remote does not have. It never resets or
    cleans those worktrees; all of its own work happens in a private worktree it
    creates and removes.
 3. Squashes the branch to ONE commit on its merge base (all four REBASE STUCK
    failures in the October factory run were fixed by hand this way), keeping
    every Signed-off-by and Co-authored-by trailer.
 4. Rebases onto current origin/main. Registry and derived-file conflicts go
    through resolve_registry.py; any other conflict stops the run (no
    --ours/--theirs, ever). Registries git merged cleanly are still verified per
    entry, and the derived files are regenerated and re-checked.
 5. Pushes with --force-with-lease pinned to the head it started from.
 6. Waits for EVERY check on that exact commit: every workflow run for the SHA,
    every check run, every commit status (CodeRabbit posts one), and the
    branch-protection required contexts, which must be present and SUCCESS
    (a skipped required check is not a pass).
 7. Classifies a failing check from its logs: real (a finding, a test or
    compile failure, an error annotation), a known infrastructure flake, or
    unknown. A real failure is compared with the same check on the same base
    commit, with the same tool versions: only when the base fails identically
    AND main's newer tip passes with the same tool versions is the base shown to
    be stale, and only then does it rebase again. Flakes are re-run at most
    twice, unknown failures at most once. Where in the tree a failure sits is
    never a reason to clear or skip it.
 8. Re-reads the PR and re-asserts every start-time precondition (open, not a
    draft, into main, same repository, same branch and head) while waiting and
    immediately before the merge call: `gh pr merge` merges into whatever base
    the PR names at that instant.
 9. Merges (squash, --match-head-commit) only when everything is green on that
    exact commit, main has not moved since the rebase (if it has, it goes round
    again, bounded), and every required review approval is CURRENT. A refused
    merge goes round again too; there is no blind retry. GitHub's merge call
    pins the head but cannot pin the base, so it checks main once more right
    before the call and then PROVES the merge commit's parent is the validated
    base and its tree the green tree; if main moved inside the call it reports
    the race and what the intervening commits touched (exit 7).

Review approvals
----------------
`approve` records (head SHA, base SHA, change fingerprint). An approval stays
current across a rebase only when both hold:
  * the fingerprint is unchanged: a byte-exact `git diff --binary` of the change
    (pinned diff options, so a reviewer's diff config cannot change it), not
    `git patch-id`, which ignores whitespace even inside string literals. What a
    rebase moves without changing what the branch did is fingerprinted by
    content instead of position: the three source registries entry by entry
    (every value byte-exact), README.md and the issue forms by their
    hand-written parts only (generator-owned regions masked with the
    generator's own patterns). Only files a generator writes WHOLE from
    sources (catalog.json, docs/_data/catalog.json, docs/llms*.txt, each
    connector's docs/skills/<slug>.md) are left out, because this tool
    regenerates and re-checks them on the commit it merges;
  * main's new commits since the approval's base touch nothing under the
    connector's directory, nothing in release_matrix.py's shared machinery list,
    no directory the branch's other files live in, no hand-written part of a
    README or issue form the branch also edits, and no registry entry of a
    slug the branch touches.
Automatic checks always re-run on the new commit regardless.

It refuses to run when its own code (and the modules it imports) differs from
origin/main's copy: a stale checkout carries a stale registry and stale rules.

Exit codes: 0 merged (or the dry run would merge); 1 internal error; 2 refused
(lock held, dirty or busy worktree, fork, head moved, stale copy of this tool);
3 conflict the tool will not resolve; 4 a required approval is not current; 5 a
check failed and was not shown to be a flake or a stale base; 6 timed out; 7
merged, but main moved inside the merge call, and main's CI on the merge commit
then passed; 8 the same race, and main's CI is red or could not be verified:
stop the chain (GitHub's merge call cannot pin the base; the race is detected,
verified and reported, not prevented: see the README).

Pure stdlib plus the `gh` CLI.
"""

from __future__ import annotations

import argparse
import difflib
import hashlib
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Callable

sys.path.insert(0, str(Path(__file__).resolve().parent))
import release_matrix  # noqa: E402  (local tools/ module: MACHINERY)
import repo_lock  # noqa: E402  (local tools/ module)
import resolve_registry as rr  # noqa: E402  (local tools/ module)

EXIT_OK, EXIT_ERROR, EXIT_REFUSED, EXIT_CONFLICT = 0, 1, 2, 3
EXIT_APPROVAL, EXIT_CHECKS, EXIT_TIMEOUT, EXIT_RACE, EXIT_RACE_RED = 4, 5, 6, 7, 8

# This tool's own code. It must be origin/main's (a stale shared checkout once
# carried an older registry and gave wrong verdicts): --allow-stale-tools only.
TOOL_FILES = ("merge_when_green.py", "resolve_registry.py", "release_matrix.py", "repo_lock.py", "registry.py",
              "build-catalog.py")  # build-catalog.py: its patterns define what the review masks hide
TOOLS_DIR = Path(__file__).resolve().parent


def _read_tool(f: str) -> str | None:
    p = TOOLS_DIR / f
    return p.read_text(encoding="utf-8", errors="surrogateescape") if p.exists() else None


# The rules THIS process runs, read once at start. Freshness compares main
# against them, not against the files on disk now: a shared checkout pulled
# mid-run would otherwise make the stale rules in memory look fresh.
LOADED_TOOLS = {f: _read_tool(f) for f in TOOL_FILES}
rr._form_options_re()  # build-catalog.py's masks, loaded together with that snapshot

FINGERPRINT_VERSION = "msp-skills-change-fingerprint/2"
FLAKE_RERUNS = 2
UNKNOWN_RERUNS = 1
BAD_CONCLUSIONS = {"failure", "timed_out", "cancelled", "action_required", "startup_failure", "stale"}


class Stop(Exception):
    def __init__(self, code: int, message: str) -> None:
        super().__init__(message)
        self.code = code


def _printable(s: str) -> str:
    """Strip control characters (a CI log can carry terminal escapes)."""
    return "".join(ch for ch in s if ch == "\t" or ch >= " ")


# --------------------------------------------------------------------------- #
# Git
# --------------------------------------------------------------------------- #

class Git:
    def __init__(self, repo: Path, env: dict | None = None) -> None:
        self.repo = Path(repo)
        self.env = env

    def run(self, *args: str, cwd: Path | None = None, check: bool = True,
            env_extra: dict | None = None) -> subprocess.CompletedProcess:
        env = dict(self.env or os.environ)
        if env_extra:
            env.update(env_extra)
        p = subprocess.run(["git", *args], cwd=str(cwd or self.repo), capture_output=True,
                           text=True, env=env)
        if check and p.returncode != 0:
            raise Stop(EXIT_ERROR, f"git {' '.join(args)} failed: {p.stderr.strip()[-1500:]}")
        return p

    def out(self, *args: str, cwd: Path | None = None) -> str:
        return self.run(*args, cwd=cwd).stdout.strip()

    def raw(self, *args: str) -> bytes:
        p = subprocess.run(["git", *args], cwd=str(self.repo), capture_output=True,
                           env=self.env or os.environ)
        if p.returncode != 0:
            raise Stop(EXIT_ERROR, f"git {' '.join(args)} failed: {p.stderr.decode('utf-8', 'replace')[-800:]}")
        return p.stdout

    def show(self, rev: str, path: str) -> str | None:
        p = subprocess.run(["git", "show", f"{rev}:{path}"], cwd=str(self.repo),
                           capture_output=True, env=self.env or os.environ)
        return p.stdout.decode("utf-8", "surrogateescape") if p.returncode == 0 else None

    def is_ancestor(self, a: str, b: str) -> bool:
        return self.run("merge-base", "--is-ancestor", a, b, check=False).returncode == 0

    def common_dir(self) -> Path:
        d = Path(self.out("rev-parse", "--git-common-dir"))
        return d if d.is_absolute() else (self.repo / d).resolve()


# --------------------------------------------------------------------------- #
# The change fingerprint and the approval rule
# --------------------------------------------------------------------------- #

# Every option that changes `git diff` output is pinned, so the fingerprint is a
# property of the two commits and not of whoever ran it.
_DIFF_CONFIG = ("-c", "core.quotePath=true", "-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false",
                "-c", "diff.relative=false", "-c", "diff.suppressBlankEmpty=false",
                "-c", "diff.ignoreSubmodules=none")
_DIFF_FLAGS = ("--binary", "--full-index", "--no-renames", "--no-color", "--no-ext-diff", "--no-textconv",
               "--diff-algorithm=myers", "--indent-heuristic", "-U3", "--inter-hunk-context=0",
               "--src-prefix=a/", "--dst-prefix=b/", "--no-relative", "--ignore-submodules=none",
               "-O/dev/null")


def changed_paths(git: Git, a: str, b: str) -> list[str]:
    """Paths changed between two commits, byte-safe: read as raw NUL-separated
    bytes (never stripped, so a leading space in a filename survives) and
    decoded with the filesystem codec (a non-UTF-8 name round-trips)."""
    raw = git.raw("diff", "--no-renames", "--name-only", "-z", a, b)
    return [os.fsdecode(p) for p in raw.split(b"\0") if p]


def _pages(git: Git, *revs: str) -> set[str]:
    out: set[str] = set()
    for r in revs:
        out |= rr.connector_pages(git.show(r, rr.SKILLS_JSON))
    return out


def fingerprint(git: Git, base: str, head: str) -> str:
    """sha256 over the change `git diff base...head`, byte-exact for every byte
    a person wrote:
      * every ordinary file: a pinned `git diff --binary` (blob ids included);
      * the three registries: entry by entry, every value byte-exact, the
        entry's position in the file ignored;
      * README.md and the issue forms: a diff of their HAND-WRITTEN parts (the
        generator-owned regions masked out with the generator's own patterns);
      * left out entirely only what a generator writes whole from sources:
        catalog.json, docs/_data/catalog.json, docs/llms*.txt and each
        registered connector's docs/skills/<slug>.md. This tool regenerates
        and re-checks those on the commit it merges, so no hand edit to them
        can land. Any other file under docs/skills/ (including the page of a
        connector the change removes or makes markdown-only, which nothing
        regenerates any more) is an ordinary file."""
    mb = git.out("merge-base", base, head)
    paths = changed_paths(git, mb, head)
    pages = _pages(git, head)  # only what the RESULTING registry regenerates
    h = hashlib.sha256()
    h.update(FINGERPRINT_VERSION.encode() + b"\n")
    literal: list[str] = []
    for p in paths:
        if p in rr.REGISTRY_FILES or p in rr.PARTIAL_DERIVED or rr.is_full_derived(p, pages):
            continue
        literal.append(p)
    for reg in rr.REGISTRY_FILES:
        if reg not in paths:
            continue
        try:
            old = rr.registry_entries(reg, git.show(mb, reg))
            new = rr.registry_entries(reg, git.show(head, reg))
        except rr.UnknownFormat:
            literal.append(reg)  # unreadable: fall back to its literal bytes
            continue
        delta = [[k, old.get(k), new.get(k)] for k in sorted(set(old) | set(new)) if old.get(k) != new.get(k)]
        h.update(b"\0registry " + reg.encode() + b"\0" + json.dumps(delta, ensure_ascii=True).encode())
    for part in rr.PARTIAL_DERIVED:
        if part not in paths:
            continue
        a, b = rr.mask_generated(part, git.show(mb, part)), rr.mask_generated(part, git.show(head, part))
        if a != b:
            diff = "".join(difflib.unified_diff((a or "").splitlines(True), (b or "").splitlines(True),
                                                f"a/{part}", f"b/{part}", n=3))
            h.update(b"\0authored " + part.encode() + b"\0" + (a is None and b"new" or b"")
                     + (b is None and b"deleted" or b"") + diff.encode("utf-8", "surrogateescape"))
    if literal:
        specs = [f":(literal){p}" for p in sorted(set(literal))]
        h.update(b"\0diff\0")
        h.update(git.raw(*_DIFF_CONFIG, "diff", *_DIFF_FLAGS, mb, head, "--", *specs))
    return "sha256:" + h.hexdigest()


@dataclass
class Footprint:
    slugs: set[str] = field(default_factory=set)
    skill_dirs: set[str] = field(default_factory=set)
    dep_prefixes: set[str] = field(default_factory=set)
    dep_files: set[str] = field(default_factory=set)
    authored_partial: set[str] = field(default_factory=set)


def footprint(git: Git, base: str, head: str) -> Footprint:
    """What the change depends on: its connectors' directories and registry
    entries, the hand-written parts of README/issue forms it edits, and the
    directory of every other file it changes."""
    mb = git.out("merge-base", base, head)
    pages = _pages(git, head)
    fp = Footprint()
    for p in changed_paths(git, mb, head):
        parts = p.split("/")
        if parts[0] == "skills" and len(parts) >= 3:
            fp.skill_dirs.add(parts[1])
            fp.slugs.add(parts[1])
        elif p in rr.REGISTRY_FILES:
            try:
                fp.slugs |= rr.registry_slugs(rr.changed_entries(p, git.show(mb, p), git.show(head, p)))
            except rr.UnknownFormat:
                fp.dep_files.add(p)
        elif p in rr.PARTIAL_DERIVED:
            if rr.mask_generated(p, git.show(mb, p)) != rr.mask_generated(p, git.show(head, p)):
                fp.authored_partial.add(p)
        elif rr.is_full_derived(p, pages) or p in rr.RELEASE_DERIVED:
            continue
        elif len(parts) > 1:
            fp.dep_prefixes.add("/".join(parts[:-1]) + "/")
        else:
            fp.dep_files.add(p)
    # A registry slug's on-disk directory (source_dir, e.g. _meta) is a
    # dependency too.
    try:
        skills = json.loads(git.show(head, rr.SKILLS_JSON) or "{}").get("skills", {})
    except ValueError:
        skills = {}
    for slug in list(fp.slugs):
        meta = skills.get(slug) if isinstance(skills, dict) else None
        fp.skill_dirs.add((meta or {}).get("source_dir") or slug)
    for slug, meta in (skills.items() if isinstance(skills, dict) else []):
        if isinstance(meta, dict) and meta.get("source_dir") in fp.skill_dirs:
            fp.slugs.add(slug)
    return fp


def main_touches(git: Git, old_base: str, new_base: str, fp: Footprint) -> list[str]:
    """Reasons main's commits old_base..new_base touch what the change depends on.
    Main's generated output (the catalog bot's commits) is not a reason: those
    files are regenerated and re-checked on the commit this tool merges."""
    if old_base == new_base:
        return []
    if not git.is_ancestor(old_base, new_base):
        return [f"main was rewritten: the approval's base {old_base[:12]} is not an ancestor of {new_base[:12]}"]
    hits = []
    names = changed_paths(git, old_base, new_base)
    pages = _pages(git, new_base)
    for p in names:
        if release_matrix.is_machinery(p):
            hits.append(f"main changed {p} (shared build machinery)")
        elif any(p.startswith(f"skills/{d}/") for d in fp.skill_dirs):
            hits.append(f"main changed {p} (this connector's directory)")
        elif p in rr.REGISTRY_FILES:
            continue  # compared per entry below
        elif p in rr.PARTIAL_DERIVED:
            if p in fp.authored_partial and (rr.mask_generated(p, git.show(old_base, p))
                                             != rr.mask_generated(p, git.show(new_base, p))):
                hits.append(f"main changed hand-written parts of {p}, which this change also edits")
        elif rr.is_full_derived(p, pages) or p in rr.RELEASE_DERIVED:
            continue
        elif p in fp.dep_files or any(p.startswith(x) for x in fp.dep_prefixes):
            hits.append(f"main changed {p} (beside a file this change edits)")
    for reg in rr.REGISTRY_FILES:
        if reg not in names:
            continue
        try:
            changed = rr.changed_entries(reg, git.show(old_base, reg), git.show(new_base, reg))
        except rr.UnknownFormat as exc:
            hits.append(f"main's {reg} cannot be compared per entry ({exc})")
            continue
        # A registry-wide field (owner, repo, the marketplace's own name, the
        # cards header) feeds every generated URL and page: always a dependency.
        for k in sorted(k for k in changed if k.startswith("top:") or k == "header"):
            hits.append(f"main changed the registry-wide {k} in {reg}")
        for s in sorted(rr.registry_slugs(changed) & fp.slugs):
            hits.append(f"main changed the {reg} entry for {s}")
    return hits


def approval_status(git: Git, appr: dict, base: str, head: str) -> tuple[bool, list[str]]:
    if appr.get("head") == head and appr.get("base") == base:
        return True, []
    reasons = []
    if appr.get("fingerprint_version") != FINGERPRINT_VERSION:
        reasons.append("recorded with a different fingerprint version")
    elif fingerprint(git, base, head) != appr.get("diff_hash"):
        reasons.append("the change is not byte-identical to the approved one")
    reasons += main_touches(git, appr.get("base", ""), base, footprint(git, base, head))
    return not reasons, reasons


# --------------------------------------------------------------------------- #
# CI log classification
# --------------------------------------------------------------------------- #

_TS_RE = re.compile(r"^﻿?\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z ?")
_ANSI_RE = re.compile(r"\x1b\[[0-9;]*[A-Za-z]|\^\[\[[0-9;]*[A-Za-z]")
_ECHO_MARKERS = ("\x1b[36;1m", "^[[36;1m")  # Actions echoes a step's script in cyan

NEUTRAL_PATTERNS = [re.compile(p) for p in (
    r"^##\[error\]Process completed with exit code \d+\.?$",
    r"^##\[error\]security-gate found P1 finding",       # the summary; the P1 lines decide
    r"^##\[error\]change must not merge",
    r"^##\[error\]tools/maintainer/security_suppressions\.json in a separate",
    r"^##\[(warning|notice|debug)\]",
)]
FLAKE_PATTERNS = [re.compile(p) for p in (
    r"stream error: stream ID \d+; (INTERNAL_ERROR|PROTOCOL_ERROR|REFUSED_STREAM)",
    r"\b(proxy|sum)\.golang\.org\b.*(i/o timeout|connection reset by peer|TLS handshake timeout|unexpected EOF|"
    r"\b50[234]\b|Internal Server Error|Service Unavailable|Bad Gateway)",
    r"dial tcp \S+: (i/o timeout|connect: connection (refused|reset by peer))",
    r"net/http: (TLS handshake timeout|request canceled while waiting for connection)",
    r"The runner has received a shutdown signal",
    r"The hosted runner (encountered an error|lost communication)",
    r"lost communication with the server",
    r"Could not resolve host: ",
    r"\b(API rate limit exceeded|secondary rate limit)\b",
    r"\b(ReadTimeoutError|ConnectTimeoutError)\b",
    r"\bP1 [A-Za-z0-9_.-]+:scanner-(missing|error)\b",   # a scanner that failed to install or run
    r"install failed \(non-fatal\)",
)]
REAL_PATTERNS = [re.compile(p) for p in (
    r"^\s*P1 \S+",                    # a security-gate finding (scanner faults are matched first)
    r"^--- FAIL: ",
    r"^FAIL(\s|$)",
    r"^panic: ",
    r"^\S+\.go:\d+:\d+: ",            # a compile or vet error
    r"\bFAIL(ED)?\b",                 # a maintainer gate: "check_x FAILED", "FAIL: ..."
    r"^\s*- missing DCO sign-off",
    r"^##\[error\]",                  # any other error annotation
)]
VERSION_PATTERNS = {
    "go": re.compile(r"go version (go\d+\.\d+(?:\.\d+)?)"),
    "gosec": re.compile(r"github\.com/securego/gosec/v2 (v\d+\.\d+\.\d+)"),
    "govulncheck": re.compile(r"golang\.org/x/vuln (v\d+\.\d+\.\d+)"),
    "osv-scanner": re.compile(r"github\.com/google/osv-scanner(?:/v2)? (v\d+\.\d+\.\d+)"),
    "semgrep": re.compile(r"\bsemgrep[-= ](\d+\.\d+\.\d+)\b"),
    "python": re.compile(r"Successfully set up CPython \((\d+\.\d+\.\d+)\)"),
    "runner-image": re.compile(r"actions/runner-images/blob/[^/\s]+/(\d{8}\.\d+)/"),
}


@dataclass
class Verdict:
    kind: str                     # "real" | "flake" | "unknown"
    real: list[str] = field(default_factory=list)
    flake: list[str] = field(default_factory=list)
    versions: dict[str, str] = field(default_factory=dict)


def clean_log_lines(text: str) -> list[str]:
    out = []
    for raw in text.splitlines():
        line = raw
        parts = line.split("\t", 2)
        if len(parts) == 3 and _TS_RE.match(parts[2]):
            line = parts[2]  # `gh run view --log` prefixes "job<TAB>step<TAB>"
        if any(m in line for m in _ECHO_MARKERS):
            continue  # the step's own script being echoed, not its output
        line = _TS_RE.sub("", line, count=1)
        line = _ANSI_RE.sub("", line).rstrip()
        if not line or line.startswith(("##[group]", "##[endgroup]")):
            continue
        out.append(line)
    return out


def classify_log(text: str) -> Verdict:
    v = Verdict("unknown")
    for name, pat in VERSION_PATTERNS.items():
        m = pat.search(text)
        if m:
            v.versions[name] = m.group(1)
    for line in clean_log_lines(text):
        if any(p.search(line) for p in NEUTRAL_PATTERNS):
            continue
        if any(p.search(line) for p in FLAKE_PATTERNS):
            v.flake.append(line)
        elif any(p.search(line) for p in REAL_PATTERNS):
            v.real.append(line)
    v.kind = "real" if v.real else ("flake" if v.flake else "unknown")
    return v


def signature(lines: list[str]) -> set[str]:
    """Failure lines normalised for comparison across commits: line and column
    numbers and run-specific paths drop out, the finding itself stays."""
    out = set()
    for ln in lines:
        s = re.sub(r":\d+(:\d+)?\b", ":N", ln.strip())
        s = re.sub(r"\b[0-9a-f]{12,40}\b", "<sha>", s)
        s = re.sub(r"\s+", " ", s)
        out.add(s)
    return out


SCANNERS = ("gosec", "govulncheck", "osv-scanner", "semgrep")


def relevant_tools(v: Verdict) -> set[str]:
    """The scanners a real failure's own lines name ("P1 gosec:G304 ...")."""
    return {s for s in SCANNERS if any(f"{s}:" in line for line in v.real)}


def same_versions(a: dict[str, str], b: dict[str, str], need: set[str] | frozenset = frozenset()) -> bool:
    """True only when the two runs are PROVEN to use the same tools: a
    toolchain (go or python) both name, every tool in `need` named by both, and
    no version that differs or is known on one side only. A runner image or
    any other partial match alone proves nothing."""
    if not a or not b:
        return False
    if not ({"go", "python"} & set(a) & set(b)):
        return False
    if any(k not in a or k not in b for k in need):
        return False
    return all(a.get(k) == b.get(k) for k in set(a) | set(b))


# --------------------------------------------------------------------------- #
# GitHub (gh CLI)
# --------------------------------------------------------------------------- #

class GitHub:
    def __init__(self, slug: str) -> None:
        self.slug = slug

    def _gh(self, *args: str, check: bool = True) -> subprocess.CompletedProcess:
        p = subprocess.run(["gh", *args], capture_output=True, text=True)
        if check and p.returncode != 0:
            raise Stop(EXIT_ERROR, f"gh {' '.join(args[:3])} failed: {p.stderr.strip()[-800:]}")
        return p

    def _lines(self, *args: str) -> list[dict]:
        return [json.loads(ln) for ln in self._gh(*args).stdout.splitlines() if ln.strip()]

    def pr(self, n: int) -> dict:
        return json.loads(self._gh("pr", "view", str(n), "-R", self.slug, "--json",
                                   "number,state,isDraft,headRefName,headRefOid,baseRefName,"
                                   "isCrossRepository,mergeable,title,url,mergeCommit").stdout)

    def required_contexts(self) -> list[str]:
        """The branch-protection required contexts. Fails CLOSED: an error
        (403, rate limit, 5xx) must not read as "nothing is required", which
        would accept a skipped required check. Only GitHub's explicit "not
        protected" answer means none; pass --required-context to state them
        when the token cannot read protection."""
        p = self._gh("api", f"repos/{self.slug}/branches/main/protection/required_status_checks",
                     "--jq", ".contexts", check=False)
        if p.returncode == 0:
            try:
                out = json.loads(p.stdout or "null")
            except ValueError:
                out = None
            if isinstance(out, list) and all(isinstance(c, str) for c in out):
                return out
            raise Stop(EXIT_REFUSED, f"unreadable required-checks answer {p.stdout.strip()[:120]!r}; "
                                     "pass --required-context NAME")
        if "Branch not protected" in p.stderr or "Required status checks not enabled" in p.stderr:
            return []
        raise Stop(EXIT_REFUSED, "cannot read main's required checks "
                                 f"({_printable(p.stderr.strip())[:200]}); refusing to treat that as 'none "
                                 "required'. Pass --required-context NAME (repeatable) to state them.")

    def snapshot(self, sha: str) -> dict:
        """Every workflow run, check run and commit status on `sha`: every page
        of each listing, verified against GitHub's own total_count (and, for
        statuses, the combined state), because a listing cut short at a page
        boundary can hide a failure. An incomplete listing is reported as
        `incomplete`, which _evaluate() treats as pending, never as green."""
        base_runs = f"repos/{self.slug}/actions/runs?head_sha={sha}&per_page=100"
        base_checks = f"repos/{self.slug}/commits/{sha}/check-runs?filter=latest&per_page=100"
        base_status = f"repos/{self.slug}/commits/{sha}/status?per_page=100"
        runs = self._lines("api", "--paginate", base_runs,
                           "--jq", ".workflow_runs[] | {id, name, event, status, conclusion}")
        checks = self._lines("api", "--paginate", base_checks,
                             "--jq", ".check_runs[] | {id, name, status, conclusion, app: .app.slug}")
        statuses = self._lines("api", "--paginate", base_status, "--jq", ".statuses[] | {context, state}")
        totals = {
            "workflow runs": (len(runs), self._lines("api", base_runs, "--jq", "{n: .total_count}")),
            "check runs": (len(checks), self._lines("api", base_checks, "--jq", "{n: .total_count}")),
            "statuses": (len(statuses), self._lines("api", base_status, "--jq", "{n: .total_count, state}")),
        }
        snap: dict = {"runs": runs, "checks": checks, "statuses": statuses}
        for what, (have, rows) in totals.items():
            want = rows[0].get("n") if rows else None
            if not isinstance(want, int) or want != have:
                snap["incomplete"] = f"{what}: read {have} of {want}"
        combined = (totals["statuses"][1] or [{}])[0].get("state")
        if statuses and combined not in ("success", "pending", "failure", "error"):
            snap["incomplete"] = f"combined status state {combined!r}"
        snap["combined_state"] = combined
        return snap

    def job_log(self, job_id: int) -> str:
        p = self._gh("api", "--allow-escape-sequences", f"repos/{self.slug}/actions/jobs/{job_id}/logs",
                     check=False)
        return p.stdout if p.returncode == 0 else ""

    def rerun_job(self, job_id: int) -> None:
        self._gh("api", "-X", "POST", f"repos/{self.slug}/actions/jobs/{job_id}/rerun")

    def check_on_commit(self, sha: str, name: str) -> dict | None:
        from urllib.parse import quote
        rows = self._lines("api", f"repos/{self.slug}/commits/{sha}/check-runs?check_name={quote(name)}&filter=latest",
                           "--jq", ".check_runs[] | {id, name, status, conclusion}")
        done = [r for r in rows if r.get("status") == "completed"]
        return done[0] if done else None

    def main_runs(self, sha: str) -> tuple[list[dict], bool]:
        """Push-event workflow runs for a commit on main, and whether the
        listing is complete against GitHub's total_count."""
        url = f"repos/{self.slug}/actions/runs?head_sha={sha}&event=push&per_page=100"
        runs = self._lines("api", "--paginate", url, "--jq", ".workflow_runs[] | {id, name, status, conclusion}")
        total = self._lines("api", url, "--jq", "{n: .total_count}")
        return runs, bool(total) and total[0].get("n") == len(runs)

    def catalog_busy(self) -> bool:
        rows = self._lines("api", f"repos/{self.slug}/actions/workflows/catalog.yml/runs?per_page=10",
                           "--jq", ".workflow_runs[] | {event, status}")
        return any(r["status"] != "completed" and r["event"] != "pull_request" for r in rows)

    def merge(self, n: int, sha: str) -> tuple[bool, str]:
        p = self._gh("pr", "merge", str(n), "-R", self.slug, "--squash", "--match-head-commit", sha, check=False)
        return p.returncode == 0, (p.stdout + p.stderr).strip()


def gh_slug_from_remote(git: Git, remote: str) -> str:
    url = git.out("remote", "get-url", remote)
    m = re.search(r"github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?/?$", url)
    if not m:
        raise Stop(EXIT_REFUSED, f"cannot read a GitHub owner/repo from {remote} ({url}); pass --gh-repo")
    return f"{m.group(1)}/{m.group(2)}"


# --------------------------------------------------------------------------- #
# State: approvals and re-run counters, per PR, in the clone's shared git dir
# --------------------------------------------------------------------------- #

class State:
    def __init__(self, git: Git, pr: int) -> None:
        self.dir = git.common_dir() / "msp-skills-merge"
        self.path = self.dir / f"pr-{pr}.json"
        self.git = git
        self.pr = pr

    def load(self) -> dict:
        try:
            d = json.loads(self.path.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            d = {}
        d.setdefault("version", 1)
        d.setdefault("pr", self.pr)
        d.setdefault("approvals", [])
        d.setdefault("reruns", {})
        return d

    def update(self, fn: Callable[[dict], None]) -> dict:
        lock = repo_lock.acquire(f"merge-state-pr-{self.pr}", {"tool": "merge_when_green"}, root=self.git.repo)
        try:
            d = self.load()
            fn(d)
            self.dir.mkdir(parents=True, exist_ok=True)
            tmp = self.path.with_suffix(".tmp")
            tmp.write_text(json.dumps(d, indent=2) + "\n", encoding="utf-8")
            os.replace(tmp, self.path)
            return d
        finally:
            repo_lock.release(lock)


# --------------------------------------------------------------------------- #
# Worktree safety
# --------------------------------------------------------------------------- #

def _ancestry() -> set[int]:
    pids, pid = set(), os.getpid()
    while pid > 1 and pid not in pids:
        pids.add(pid)
        p = subprocess.run(["ps", "-o", "ppid=", "-p", str(pid)], capture_output=True, text=True)
        try:
            pid = int(p.stdout.strip())
        except ValueError:
            break
    return pids


def processes_in(path: Path, method: str | None = None) -> list[tuple[int, str]]:
    """Live processes (other than this one and its ancestors) whose working
    directory is inside `path`. Fails CLOSED (raises Stop) whenever that cannot
    be established: no inspector, an inspector that errors, or one whose
    listing does not even include this process (the positive control)."""
    root = str(path.resolve())
    found: list[tuple[int, str]] = []
    me = os.getpid()
    seen_self = False
    method = method or ("proc" if sys.platform.startswith("linux") and Path("/proc").is_dir() else "lsof")
    if method == "proc":
        for d in Path("/proc").iterdir():
            if not d.name.isdigit():
                continue
            try:
                cwd = os.readlink(d / "cwd")
                cmd = (d / "comm").read_text().strip()
            except OSError:
                continue
            seen_self = seen_self or int(d.name) == me
            if cwd == root or cwd.startswith(root + os.sep):
                found.append((int(d.name), cmd))
    else:
        lsof = shutil.which("lsof")
        if not lsof:
            raise Stop(EXIT_REFUSED, "cannot tell whether another process is using a worktree (no lsof); refusing")
        p = subprocess.run([lsof, "-n", "-P", "-w", "-a", "-d", "cwd", "-Fpcn"], capture_output=True, text=True)
        if p.returncode != 0:
            raise Stop(EXIT_REFUSED, f"process inspection failed (lsof exit {p.returncode}: "
                                     f"{_printable(p.stderr.strip())[:200]}); refusing to rewrite a worktree that may be in use")
        pid, cmd = None, ""
        for line in p.stdout.splitlines():
            tag, val = line[:1], line[1:]
            if tag == "p":
                pid, cmd = int(val), ""
                seen_self = seen_self or pid == me
            elif tag == "c":
                cmd = val
            elif tag == "n" and pid is not None and (val == root or val.startswith(root + os.sep)):
                found.append((pid, cmd))
    if not seen_self:
        raise Stop(EXIT_REFUSED, f"process inspection ({method}) did not list this process itself, so its view of "
                                 "who is using the worktree cannot be trusted; refusing")
    mine = _ancestry()
    return [(pid, cmd) for pid, cmd in found if pid not in mine]


def branch_worktrees(git: Git, branch: str) -> list[Path]:
    out, cur = [], None
    for line in git.out("worktree", "list", "--porcelain").splitlines():
        if line.startswith("worktree "):
            cur = Path(line[len("worktree "):])
        elif line == f"branch refs/heads/{branch}" and cur is not None:
            out.append(cur)
    return out


def worktree_problems(git: Git, wt: Path, branch: str, remote_head: str) -> list[str]:
    probs = []
    st = git.run("--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=normal", cwd=wt, check=False)
    if st.returncode != 0:
        probs.append(f"{wt}: git status failed ({st.stderr.strip()[:200]})")
    elif st.stdout.strip():
        probs.append(f"{wt} is dirty: {len(st.stdout.splitlines())} changed or untracked path(s)")
    gdir = Path(git.out("rev-parse", "--absolute-git-dir", cwd=wt))
    for marker in ("rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "index.lock"):
        if (gdir / marker).exists():
            probs.append(f"{wt} has {marker} (an operation is in progress)")
    for pid, cmd in processes_in(wt):
        probs.append(f"{wt} is in use by pid {pid} ({_printable(cmd)})")
    local = git.out("rev-parse", f"refs/heads/{branch}")
    if local != remote_head and not git.is_ancestor(local, remote_head):
        probs.append(f"{wt}: local {branch} ({local[:12]}) has commits the remote head "
                     f"({remote_head[:12]}) does not; push or drop them first")
    return probs


# --------------------------------------------------------------------------- #
# The merge
# --------------------------------------------------------------------------- #

@dataclass
class Options:
    pr: int
    dry_run: bool = False
    remote: str = "origin"
    require: tuple[str, ...] = ("codex",)
    expect_workflows: tuple[str, ...] = ("CI", "security-gate")
    timeout_s: float = 3600.0
    poll_s: float = 30.0
    settle_s: float = 60.0
    bot_wait_s: float = 900.0
    max_rebases: int = 3
    generators: tuple[str, ...] = rr.GENERATORS
    allow_stale_tools: bool = False
    required: tuple[str, ...] | None = None  # None: read branch protection


class Merger:
    def __init__(self, git: Git, gh, opts: Options, out: Callable[[str], None] = print,
                 sleep: Callable[[float], None] = time.sleep, clock: Callable[[], float] = time.monotonic) -> None:
        self.git, self.gh, self.o = git, gh, opts
        self.out, self.sleep, self.clock = out, sleep, clock
        self.wt: Path | None = None
        self.tmp: Path | None = None
        self.state = State(git, opts.pr)
        self.rebases = 0
        self.lock: Path | None = None
        self.tools = LOADED_TOOLS  # the rules this run validates with
        self.green: list[list[str]] | None = None  # the check verdicts of the last green wait

    def say(self, msg: str) -> None:
        self.out(_printable(msg))

    # -- top level -------------------------------------------------------- #
    def run(self) -> int:
        try:
            return self._run()
        except Stop as s:
            self.say(f"STOP ({s.code}): {s}")
            return s.code
        except Exception as exc:  # anything unexpected stops the run, it never merges
            self.say(f"STOP ({EXIT_ERROR}): internal error: {type(exc).__name__}: {exc}")
            return EXIT_ERROR
        finally:
            self._cleanup()

    def _remote_main(self) -> str:
        out = self.git.out("ls-remote", self.o.remote, "refs/heads/main").split()
        if not out:
            raise Stop(EXIT_ERROR, f"{self.o.remote} reports no main branch")
        return out[0]

    def _run(self) -> int:
        o = self.o
        pr = self._checked_pr()
        branch, head = pr["headRefName"], pr["headRefOid"]
        self.say(f"==> PR #{o.pr} {branch} at {head[:12]}" + (" (dry run: nothing is pushed or merged)" if o.dry_run else ""))
        try:
            self.lock = repo_lock.acquire(branch_lock_name(branch),
                                          {"tool": "merge_when_green", "pr": o.pr, "branch": branch},
                                          root=self.git.repo)
        except repo_lock.LockHeld as exc:
            raise Stop(EXIT_REFUSED, f"another run owns this branch: {exc}")
        self.say(f"==> locked branch {branch}")
        self._fetch(branch)  # also checks this tool against the main it fetched
        remote_head = self.git.out("rev-parse", f"refs/remotes/{o.remote}/{branch}")
        if remote_head != head:
            raise Stop(EXIT_REFUSED, f"the PR head moved while starting ({head[:12]} -> {remote_head[:12]}); re-run")
        self._check_branch_worktrees(branch, head, announce=True)
        required = list(self.o.required) if self.o.required is not None else self.gh.required_contexts()
        self.say(f"==> required contexts: {', '.join(required) or '(none: main is not protected)'}")
        current = head
        while True:
            self._wait_for_catalog_bot()
            self._fetch(branch)
            base = self.git.out("rev-parse", f"refs/remotes/{o.remote}/main")
            new = self._prepare(current, base, pr.get("title") or branch)
            ok, missing = self._approvals(base, new, record=False)
            if missing:
                self.say("==> approvals not current for this change: " + "; ".join(missing))
            if new != current:
                if o.dry_run:
                    self.say(f"==> [dry run] would push {new[:12]} to {branch} with --force-with-lease={current[:12]}")
                else:
                    # Everything that made the rewrite safe, checked again right
                    # before it: the PR (still open, into main, same head) and every
                    # worktree with the branch checked out (still clean, idle, pushed).
                    self._checked_pr(current, branch)
                    self._check_branch_worktrees(branch, current)
                    # Exactly this one ref: push.followTags=true in someone's config
                    # would otherwise add reachable annotated tags (unprobed releases).
                    self.git.run("-c", "push.followTags=false", "push", "--no-follow-tags",
                                 f"--force-with-lease=refs/heads/{branch}:{current}", o.remote,
                                 f"{new}:refs/heads/{branch}", cwd=self.wt)
                    self.say(f"==> pushed {new[:12]} (lease {current[:12]})")
                    current = new
            if o.dry_run:
                return self._dry_run_report(new, current, base, required, missing)
            result = self._wait_for_checks(current, base, required)
            if result == "rebase":
                continue
            verdicts = self._final_gate(current, base, required, branch)
            if verdicts is None:  # something changed since the green verdict: validate again
                self._count_rebase()
                continue
            outcome = self._merge(current, base, verdicts)  # the very next call: nothing in between
            if outcome == "retry":
                self._count_rebase()
                continue
            return outcome

    def _checked_pr(self, head: str | None = None, branch: str | None = None) -> dict:
        """The PR, re-read, with EVERY start-time precondition asserted again:
        open, not a draft, targeting main, from this repository, and (once
        known) on the same branch and head. Called at the start, while waiting
        and immediately before the merge call, because a PR can be retargeted,
        drafted or pushed to at any moment and `gh pr merge` merges into
        whatever base the PR names at that instant."""
        o = self.o
        pr = self.gh.pr(o.pr)
        if pr.get("state") != "OPEN":
            raise Stop(EXIT_REFUSED, f"PR #{o.pr} is {pr.get('state')}, not OPEN")
        if pr.get("isDraft"):
            raise Stop(EXIT_REFUSED, f"PR #{o.pr} is a draft")
        if pr.get("baseRefName") != "main":
            raise Stop(EXIT_REFUSED, f"PR #{o.pr} targets {pr.get('baseRefName')}, not main; not merging")
        if pr.get("isCrossRepository"):
            raise Stop(EXIT_REFUSED, f"PR #{o.pr} is from a fork: this tool runs the branch's generators "
                                     "and pushes to the branch, so it handles same-repository branches only")
        if branch is not None and pr.get("headRefName") != branch:
            raise Stop(EXIT_REFUSED, f"PR #{o.pr} now comes from {pr.get('headRefName')}, not {branch}")
        if head is not None and pr.get("headRefOid") != head:
            raise Stop(EXIT_REFUSED, f"the PR head moved to {str(pr.get('headRefOid'))[:12]}; not merging")
        return pr

    def _check_branch_worktrees(self, branch: str, head: str, announce: bool = False) -> None:
        for wt in branch_worktrees(self.git, branch):
            probs = worktree_problems(self.git, wt, branch, head)
            if probs:
                raise Stop(EXIT_REFUSED, "a worktree with this branch checked out is not safe to rewrite under:\n  "
                           + "\n  ".join(probs))
            if announce:
                self.say(f"==> {wt} has {branch} checked out: clean, idle and pushed (left untouched)")

    def _count_rebase(self) -> None:
        self.rebases += 1
        if self.rebases > self.o.max_rebases:
            raise Stop(EXIT_TIMEOUT, f"validated again {self.o.max_rebases} times and main or the checks are "
                                     "still moving; try later")

    def _fetch(self, branch: str) -> None:
        """Fetch main and the branch. Every fetch of main re-checks this tool
        against that main: rules that changed mid-run stop the run."""
        r = self.o.remote
        self.git.run("fetch", "-q", "--no-tags", r, f"+refs/heads/main:refs/remotes/{r}/main",
                     f"+refs/heads/{branch}:refs/remotes/{r}/{branch}")
        check_tool_freshness(self.git, r, self.o.allow_stale_tools, self.say, self.tools)

    def _wait_for_catalog_bot(self) -> None:
        deadline = self.clock() + self.o.bot_wait_s
        while self.gh.catalog_busy():
            if self.clock() >= deadline:
                self.say("==> the catalog bot is still running; continuing (its commit may move main again)")
                return
            self.say("==> waiting for the catalog bot to land its derived-files commit")
            self.sleep(self.o.poll_s)

    # -- squash + rebase + regenerate in a private worktree ---------------- #
    def _ensure_worktree(self, rev: str) -> Path:
        if self.wt is None:
            self.tmp = Path(tempfile.mkdtemp(prefix="merge-when-green-"))
            self.wt = self.tmp / "wt"
            self.git.run("worktree", "add", "-q", "--detach", str(self.wt), rev)
        else:
            # Our own worktree: resetting it is safe by construction.
            self.git.run("rebase", "--abort", cwd=self.wt, check=False)
            self.git.run("reset", "-q", "--hard", rev, cwd=self.wt)
            self.git.run("clean", "-fdq", cwd=self.wt)
        return self.wt

    def _prepare(self, head: str, base: str, title: str) -> str:
        wt = self._ensure_worktree(head)
        g = self.git
        mb = g.out("merge-base", head, base)
        commits = g.out("rev-list", f"{mb}..{head}").split()
        merges = g.out("rev-list", "--merges", f"{mb}..{head}").split()
        if not commits:
            raise Stop(EXIT_REFUSED, "the branch has no commits that are not already on main")
        tip = head
        if len(commits) > 1 or merges:
            tip = self._squash(head, mb, title, commits)
            g.run("checkout", "-q", "--detach", tip, cwd=wt)
            self.say(f"==> squashed {len(commits)} commit(s) into {tip[:12]} on merge base {mb[:12]}")
        else:
            self.say("==> one commit on its merge base already; no squash needed")
        if mb != base:
            self._rebase(wt, base)
            self.say(f"==> rebased onto origin/main {base[:12]}")
            probs = rr.verify_registries(wt, mb, base, tip, "HEAD")
            if probs:
                raise Stop(EXIT_CONFLICT, "the rebased registries are not the per-entry merge:\n  " + "\n  ".join(probs))
        else:
            self.say(f"==> already based on origin/main {base[:12]}")
        try:
            # catalog.json comes from the validated base first: the generator
            # keeps its generated_at, which reaches the README footer, and the
            # fingerprint leaves catalog.json out.
            changed = rr.regenerate_and_stage(wt, self.say, self.o.generators, trusted_rev=base)
        except RuntimeError as exc:
            raise Stop(EXIT_CONFLICT, f"regenerating derived files failed: {exc}")
        if changed:
            g.run("commit", "-q", "--amend", "--no-edit", cwd=wt)
            self.say(f"==> regenerated derived files into the commit: {', '.join(changed)}")
        probs = rr.recheck(wt, None, self.o.generators)
        if probs:
            raise Stop(EXIT_CONFLICT, "re-check failed:\n  " + "\n  ".join(probs))
        dirty = g.out("status", "--porcelain", cwd=wt)
        if dirty:
            raise Stop(EXIT_ERROR, f"the private worktree is not clean after preparing: {dirty[:300]}")
        new = g.out("rev-parse", "HEAD", cwd=wt)
        if g.out("merge-base", new, base) != base:
            raise Stop(EXIT_ERROR, "the prepared commit is not on current origin/main")
        return new

    def _squash(self, head: str, mb: str, title: str, commits: list[str]) -> str:
        g = self.git
        trailers: list[str] = []
        subjects = []
        for c in reversed(commits):
            subjects.append(g.out("log", "-1", "--format=%s", c))
            for line in g.out("log", "-1", "--format=%B", c).splitlines():
                if re.match(r"^(Signed-off-by|Co-authored-by): .+<.+>\s*$", line, re.I) and line not in trailers:
                    trailers.append(line.strip())
        name = g.out("config", "user.name")
        email = g.out("config", "user.email")
        me = f"Signed-off-by: {name} <{email}>"
        if me not in trailers:
            trailers.append(me)
        oldest = commits[-1]
        an, ae, ad = g.out("log", "-1", "--format=%an%x00%ae%x00%aI", oldest).split("\0")
        msg = (f"{title}\n\nSquashed from {len(commits)} commits by merge_when_green.py before rebasing:\n"
               + "".join(f"- {s}\n" for s in subjects) + "\n" + "\n".join(trailers) + "\n")
        tree = g.out("rev-parse", f"{head}^{{tree}}")
        env = {"GIT_AUTHOR_NAME": an, "GIT_AUTHOR_EMAIL": ae, "GIT_AUTHOR_DATE": ad}
        p = subprocess.run(["git", "commit-tree", tree, "-p", mb], input=msg, capture_output=True, text=True,
                           cwd=str(g.repo), env={**(g.env or os.environ), **env})
        if p.returncode != 0:
            raise Stop(EXIT_ERROR, f"git commit-tree failed: {p.stderr.strip()}")
        return p.stdout.strip()

    def _rebase(self, wt: Path, base: str) -> None:
        g = self.git
        env = {"GIT_EDITOR": "true"}
        p = g.run("rebase", base, cwd=wt, check=False, env_extra=env)
        if p.returncode == 0:
            return
        if not rr.unmerged(wt):
            g.run("rebase", "--abort", cwd=wt, check=False)
            raise Stop(EXIT_CONFLICT, f"the rebase stopped without a conflict: {p.stdout.strip()[-400:]} {p.stderr.strip()[-400:]}")
        self.say("==> the rebase conflicted; handing it to resolve_registry.py")
        rc = rr.resolve(wt, main_side="ours", regen=True, out=lambda s: self.say("    " + s),
                        generators=self.o.generators)
        if rc != 0:
            paths = ", ".join(sorted(rr.unmerged(wt))) or "see above"
            g.run("rebase", "--abort", cwd=wt, check=False)
            raise Stop(EXIT_CONFLICT, f"a conflict this tool will not resolve ({paths}); the rebase was aborted in "
                                      "its private worktree and nothing was pushed. Resolve it by hand on the branch.")
        c = g.run("rebase", "--continue", cwd=wt, check=False, env_extra=env)
        if c.returncode != 0:
            g.run("rebase", "--abort", cwd=wt, check=False)
            raise Stop(EXIT_CONFLICT, f"the rebase did not continue after resolving: {c.stdout.strip()[-300:]} "
                                      f"{c.stderr.strip()[-300:]}")

    # -- approvals ---------------------------------------------------------- #
    def _approvals(self, base: str, head: str, record: bool) -> tuple[bool, list[str]]:
        if not self.o.require:
            return True, []
        data = self.state.load()
        missing, carried = [], []
        for reviewer in self.o.require:
            reasons_all = []
            for appr in reversed([a for a in data["approvals"] if a.get("reviewer") == reviewer]):
                ok, reasons = approval_status(self.git, appr, base, head)
                if ok:
                    if appr.get("head") != head or appr.get("base") != base:
                        carried.append((reviewer, appr))
                    break
                reasons_all.append(f"{str(appr.get('head'))[:12]}: {', '.join(reasons)}")
            else:
                missing.append(f"{reviewer} ({'; '.join(reasons_all) if reasons_all else 'no approval recorded'})")
        if record and carried and not missing:
            fp = fingerprint(self.git, base, head)

            def add(d: dict) -> None:
                for reviewer, appr in carried:
                    d["approvals"].append({**appr, "head": head, "base": base, "diff_hash": fp,
                                           "recorded_at": _now(), "carried_from": {"head": appr["head"],
                                                                                   "base": appr["base"]}})
            self.state.update(add)
            for reviewer, appr in carried:
                self.say(f"==> {reviewer} approval of {appr['head'][:12]} carried to {head[:12]}: "
                         "the change is byte-identical and main touched nothing it depends on")
        return not missing, missing

    # -- checks ------------------------------------------------------------- #
    def _evaluate(self, snap: dict, required: list[str]) -> tuple[list[str], list[dict]]:
        pending, failed = [], []
        if snap.get("incomplete"):
            pending.append(f"the check listing is incomplete ({snap['incomplete']}); reading again")
        # GitHub's combined state is authoritative over the listing it was read
        # alongside (a context can change between the two reads). With no
        # statuses at all GitHub reports "pending", which means nothing.
        combined = snap.get("combined_state")
        if snap["statuses"] and combined == "pending":
            pending.append("the combined commit status is pending")
        elif snap["statuses"] and combined in ("failure", "error"):
            failed.append({"kind": "status", "name": "(combined commit status)", "conclusion": combined})
        names = {r.get("name") for r in snap["runs"]}
        for w in self.o.expect_workflows:
            if w not in names:
                pending.append(f"workflow {w} has not started")
        for r in snap["runs"]:
            if r.get("status") != "completed":
                pending.append(f"workflow {r.get('name')} is {r.get('status')}")
        checks = {c.get("name"): c for c in snap["checks"]}
        statuses = {s.get("context"): s for s in snap["statuses"]}
        for c in snap["checks"]:
            if c.get("status") != "completed":
                pending.append(f"check {c.get('name')} is {c.get('status')}")
            elif c.get("conclusion") in BAD_CONCLUSIONS:
                failed.append({"kind": "check", **c})
            elif c.get("name") in required and c.get("conclusion") != "success":
                failed.append({"kind": "required-not-success", **c})
        for s in snap["statuses"]:
            if s.get("state") == "pending":
                pending.append(f"status {s.get('context')} is pending")
            elif s.get("state") != "success":
                failed.append({"kind": "status", "name": s.get("context"), "conclusion": s.get("state")})
        for ctx in required:
            if ctx not in checks and ctx not in statuses:
                pending.append(f"required context {ctx} has not reported")
        failing_run_names = {r.get("name") for r in snap["runs"]
                             if r.get("status") == "completed" and r.get("conclusion") in BAD_CONCLUSIONS}
        if failing_run_names and not failed and not pending:
            for n in sorted(failing_run_names):
                failed.append({"kind": "run", "name": n, "conclusion": "failure"})
        return pending, failed

    def _wait_for_checks(self, sha: str, base: str, required: list[str]) -> str:
        deadline = self.clock() + self.o.timeout_s
        last_key, green_since = None, None
        while self.clock() < deadline:
            pr = self._checked_pr(sha)
            if pr.get("mergeable") == "CONFLICTING":
                raise Stop(EXIT_CONFLICT, "GitHub reports the PR as CONFLICTING (pull_request CI never runs then)")
            snap = self.gh.snapshot(sha)
            pending, failed = self._evaluate(snap, required)
            if failed and not pending:
                action = self._handle_failures(sha, base, failed)
                if action == "rebase":
                    return "rebase"
                last_key, green_since = None, None
                continue
            if pending or failed:
                last_key, green_since = None, None
                self.say(f"    waiting: {len(pending)} pending" + (f", {len(failed)} failed" if failed else ""))
                self.sleep(self.o.poll_s)
                continue
            key = verdicts_of(snap)
            now = self.clock()
            if key != last_key:
                last_key, green_since = key, now
            if now - green_since >= self.o.settle_s:
                self.green = key
                self.say(f"==> green on {sha[:12]}: {len(snap['checks'])} check run(s), "
                         f"{len(snap['statuses'])} status(es), {len(snap['runs'])} workflow run(s)")
                return "green"
            self.sleep(min(self.o.poll_s, max(self.o.settle_s - (now - green_since), 0.0)) or 0.0)
        raise Stop(EXIT_TIMEOUT, f"checks on {sha[:12]} did not finish within {int(self.o.timeout_s)}s")

    def _handle_failures(self, sha: str, base: str, failed: list[dict]) -> str:
        stops, rebases, reruns = [], [], []
        counts = self.state.load()["reruns"].get(sha, {})
        for f in failed:
            name = f.get("name")
            if f["kind"] != "check" or f.get("app") not in (None, "github-actions"):
                stops.append(f"{name}: {f.get('conclusion')} ({f['kind']}); no log to classify")
                continue
            v = classify_log(self.gh.job_log(f["id"]))
            n = counts.get(name, 0)
            if v.kind == "flake":
                if n < FLAKE_RERUNS:
                    reruns.append((f, f"known infrastructure flake: {v.flake[0][:160]}"))
                else:
                    stops.append(f"{name}: flake signature, but already re-run {n} time(s)")
            elif v.kind == "unknown":
                if n < UNKNOWN_RERUNS:
                    reruns.append((f, "unrecognised failure; one re-run"))
                else:
                    stops.append(f"{name}: unrecognised failure, already re-run {n} time(s)")
            else:
                action, why = self._compare_with_base(name, v, base)
                (rebases if action == "rebase" else stops).append(f"{name}: {why}")
        if stops:
            raise Stop(EXIT_CHECKS, "checks failed on " + sha[:12] + ":\n  " + "\n  ".join(stops + rebases))
        if rebases:
            self.say("==> " + "; ".join(rebases))
            self._count_rebase()
            return "rebase"
        for f, why in reruns:
            self.say(f"==> re-running {f['name']}: {why}")
            self.gh.rerun_job(f["id"])

            def bump(d: dict, name: str = f["name"]) -> None:
                d["reruns"].setdefault(sha, {})
                d["reruns"][sha][name] = d["reruns"][sha].get(name, 0) + 1
            self.state.update(bump)
        self.sleep(self.o.poll_s)
        return "rerun"

    def _compare_with_base(self, name: str, v: Verdict, base: str) -> tuple[str, str]:
        head_line = v.real[0][:200]
        b = self.gh.check_on_commit(base, name)
        if b is None:
            return "stop", f"real failure ({head_line}); no run of this check on base {base[:12]} to compare with"
        bv = classify_log(self.gh.job_log(b["id"]))
        need = relevant_tools(v)
        if not same_versions(v.versions, bv.versions, need):
            return "stop", (f"real failure ({head_line}); the base run used different or unreadable tool versions "
                            f"({v.versions} vs {bv.versions}), so the base cannot be shown stale")
        if b.get("conclusion") == "success":
            return "stop", f"real failure caused by this change ({head_line}): the same check passes on the base"
        if not (signature(v.real) <= signature(bv.real)):
            return "stop", f"real failure ({head_line}) that differs from the base's failure"
        tip = self._remote_main()
        if tip == base:
            return "stop", (f"the base fails the same way ({head_line}) and is main's tip: main is red for "
                            "this check; fix main first")
        t = self.gh.check_on_commit(tip, name)
        if t is None or t.get("conclusion") != "success":
            return "stop", f"the base fails the same way ({head_line}) and main's tip {tip[:12]} has no passing run"
        tv = classify_log(self.gh.job_log(t["id"]))
        if not same_versions(v.versions, tv.versions, need):
            return "stop", (f"main's tip passes, but with different tool versions ({tv.versions}); "
                            "the base is not shown to be stale")
        return "rebase", (f"base {base[:12]} is stale: it fails identically ({head_line}) and main's tip "
                          f"{tip[:12]} passes with the same tool versions")

    # -- merge and reports -------------------------------------------------- #
    def _final_gate(self, sha: str, base: str, required: list[str], branch: str) -> list[list[str]] | None:
        """EVERY precondition of the merge, re-read in one place, as late as
        possible: `_run` calls `_merge` straight after it. Re-reads main (and,
        through `_fetch`, this tool's freshness against that main), the PR,
        the approvals' change fingerprint, and every check on the exact head
        (complete and paginated; still green; verdicts unchanged since the
        wait said green). Returns those verdicts, which `_merge` compares with
        the head's checks once more after the merge, or None when anything
        changed, which sends the run back to validate from the top.
        Residual window: the seconds between these reads and GitHub executing
        the merge. Only server-side enforcement (a merge queue, or every
        aggregate check required) closes it; the after-merge comparison and
        the parent/tree proof detect it."""
        o = self.o
        self._fetch(branch)
        tip = self.git.out("rev-parse", f"refs/remotes/{o.remote}/main")
        if tip != base:
            self.say(f"==> main moved during the wait ({base[:12]} -> {tip[:12]}); validating again")
            return None
        ok, missing = self._approvals(base, sha, record=True)
        if not ok:
            raise Stop(EXIT_APPROVAL, "green, but a required approval is not current: " + "; ".join(missing)
                       + f". Re-review {sha[:12]} and record it with `approve {o.pr} --head {sha}`.")
        snap = self.gh.snapshot(sha)
        pending, failed = self._evaluate(snap, required)
        now = verdicts_of(snap)
        if pending or failed or now != self.green:
            what = (pending + [f"{f.get('name')} {f.get('conclusion')}" for f in failed]) or _changed(self.green, now)
            self.say(f"==> the checks on {sha[:12]} changed after the green verdict ({'; '.join(what[:5])}); "
                     "validating again")
            return None
        # Last, the cheapest reads: the PR (open, into main, this head) and main.
        pr = self._checked_pr(sha, branch)
        if pr.get("mergeable") == "CONFLICTING":
            raise Stop(EXIT_CONFLICT, "GitHub reports the PR as CONFLICTING")
        tip = self._remote_main()
        if tip != base:
            self.say(f"==> main moved to {tip[:12]} just before the merge; validating again")
            return None
        return now

    def _merge(self, sha: str, base: str, verdicts: list[list[str]]) -> int | str:
        """One merge attempt, never a blind retry, straight after _final_gate.
        GitHub's merge call can pin the head (--match-head-commit) but not the
        base, and this repository's protection is not strict, so PROVE
        afterwards that the head's checks still read as the gate saw them and
        that the merge commit sits on the validated base with the validated
        tree. A refused merge goes back to the top (re-fetch, re-validate) as
        "retry"."""
        o = self.o
        ok, text = self.gh.merge(o.pr, sha)
        pr = self.gh.pr(o.pr)
        if not ok:
            self.say(f"==> GitHub refused the merge: {_printable(text)[-300:]}")
            if pr.get("state") != "OPEN":
                raise Stop(EXIT_ERROR, f"the PR is {pr.get('state')} after a refused merge; inspect it by hand")
            if pr.get("headRefOid") != sha:
                raise Stop(EXIT_REFUSED, f"the PR head moved to {str(pr.get('headRefOid'))[:12]}; not merging")
            return "retry"
        if pr.get("state") != "MERGED":
            raise Stop(EXIT_ERROR, f"the merge did not land (state {pr.get('state')})")
        merge = ((pr.get("mergeCommit") or {}).get("oid")) or ""
        self.git.run("fetch", "-q", "--no-tags", o.remote, f"+refs/heads/main:refs/remotes/{o.remote}/main")
        if not merge or self.git.run("cat-file", "-e", f"{merge}^{{commit}}", check=False).returncode != 0:
            raise Stop(EXIT_ERROR, f"merged, but the merge commit {merge[:12] or '(none reported)'} cannot be read "
                                   "to prove it sits on the validated base; check main by hand")
        after = self._verdicts_after_merge(sha)
        if after != verdicts:
            raise Stop(EXIT_RACE_RED, f"merged #{o.pr} as {merge[:12]}, but the checks on {sha[:12]} changed between "
                                      f"the final gate and the merge ({'; '.join(_changed(verdicts, after)[:5])}): "
                                      "stop the chain; nothing may be tagged or released from main until its CI on "
                                      f"{merge[:12]} is green")
        parent = self.git.out("rev-parse", f"{merge}^1")
        same_tree = self.git.out("rev-parse", f"{merge}^{{tree}}") == self.git.out("rev-parse", f"{sha}^{{tree}}")
        if parent == base and same_tree:
            self.say(f"MERGED #{o.pr} as {merge[:12]}: on the validated base {base[:12]}, tree identical to "
                     f"the green head {sha[:12]}")
            return EXIT_OK
        between = changed_paths(self.git, base, parent) if self.git.is_ancestor(base, parent) else ["(main rewritten)"]
        race = (f"RACE: merged as {merge[:12]}, but main moved inside the merge call: its parent is "
                f"{parent[:12]}, not the validated base {base[:12]}"
                + ("" if same_tree else "; the merged tree is not the green one")
                + f". Main's commits in that window touched: {', '.join(between[:15]) or 'nothing'}.")
        self.say("==> " + race)
        # The merge landed on a base nobody validated. Verify the result the
        # only way left: main's own push CI for that merge commit, waited for
        # here so a chain cannot run on (tag, release) until it has a verdict.
        verdict, detail = self._watch_main_ci(merge)
        if verdict == "green":
            raise Stop(EXIT_RACE, race + f" Main's CI on {merge[:12]} is GREEN ({detail}); read the intervening "
                                         "commits before releasing.")
        raise Stop(EXIT_RACE_RED, race + f" Main's CI on {merge[:12]} is {verdict.upper()} ({detail}): stop the "
                                         "chain; nothing may be tagged or released from main until it is green.")

    def _verdicts_after_merge(self, sha: str) -> list[list[str]] | None:
        """The head's check verdicts read again after the merge, or None when
        no complete listing can be read (which never matches: unverified)."""
        for _ in range(3):
            try:
                snap = self.gh.snapshot(sha)
            except Exception as exc:  # unreadable after a merge is "unverified" (exit 8), never exit 1
                self.say(f"    the checks on {sha[:12]} could not be read after the merge ({str(exc)[:200]})")
                snap = {"incomplete": "lookup failed"}
            if not snap.get("incomplete"):
                return verdicts_of(snap)
            self.sleep(self.o.poll_s)
        return None

    def _watch_main_ci(self, merge: str) -> tuple[str, str]:
        """("green" | "red" | "unverified", detail) for the push-to-main CI of
        `merge`. Waits until every run for that commit has completed (and at
        least one CI run exists), within the check timeout."""
        deadline = self.clock() + self.o.timeout_s
        last_error = ""
        while self.clock() < deadline:
            try:
                runs, complete = self.gh.main_runs(merge)
            except Exception as exc:  # a lookup failure is "not verified yet", never a different exit
                last_error = str(exc)[:200]
                self.say(f"    main's CI on {merge[:12]} could not be read ({last_error}); retrying")
                self.sleep(self.o.poll_s)
                continue
            ci = [r for r in runs if r.get("name") == "CI"]
            if complete and ci and all(r.get("status") == "completed" for r in runs):
                bad = [f"{r.get('name')}={r.get('conclusion')}" for r in runs
                       if r.get("conclusion") not in ("success", "skipped", "neutral")]
                return ("red", ", ".join(bad)) if bad else ("green", f"{len(runs)} run(s) passed")
            self.say(f"    waiting for main's CI on {merge[:12]} ({len(runs)} run(s) so far)")
            self.sleep(self.o.poll_s)
        return "unverified", (f"no complete CI verdict within {int(self.o.timeout_s)}s"
                              + (f"; last lookup error: {last_error}" if last_error else ""))

    def _dry_run_report(self, new: str, current: str, base: str, required: list[str], missing: list[str]) -> int:
        if new != current:
            self.say(f"==> [dry run] would wait for every check on {new[:12]}, then merge it with "
                     f"--match-head-commit {new[:12]} if green, main is still {base[:12]}, and approvals are current")
            return EXIT_APPROVAL if missing else EXIT_OK
        pending, failed = self._evaluate(self.gh.snapshot(new), required)
        self.say(f"==> [dry run] checks on {new[:12]} now: {len(pending)} pending, {len(failed)} failed")
        for f in failed:
            self.say(f"    failed: {f.get('name')} ({f.get('conclusion')})")
        if missing:
            return EXIT_APPROVAL
        if failed:
            return EXIT_CHECKS
        self.say(f"==> [dry run] would merge #{self.o.pr} with --match-head-commit {new[:12]}"
                 + (" once the pending checks are green" if pending else ""))
        return EXIT_OK

    def _cleanup(self) -> None:
        if self.wt is not None:
            self.git.run("rebase", "--abort", cwd=self.wt, check=False)
            # Only our own worktree: never `worktree prune`, which would also drop
            # other sessions' metadata for worktrees that are briefly missing.
            self.git.run("worktree", "remove", "--force", str(self.wt), check=False)
        if self.tmp is not None:
            shutil.rmtree(self.tmp, ignore_errors=True)
        if self.lock is not None:
            repo_lock.release(self.lock)
            self.lock = None


def verdicts_of(snap: dict) -> list[list[str]]:
    """Every check run, workflow run and status on a commit as comparable
    (kind, name, status, conclusion) rows: a same-SHA re-run that is pending,
    or ends differently, changes them."""
    rows = [["check", str(c.get("name")), str(c.get("status")), str(c.get("conclusion"))] for c in snap["checks"]]
    rows += [["run", str(r.get("name")), str(r.get("status")), str(r.get("conclusion"))] for r in snap["runs"]]
    rows += [["status", str(x.get("context")), str(x.get("state")), ""] for x in snap["statuses"]]
    return sorted(rows)


def _changed(before: list[list[str]] | None, after: list[list[str]] | None) -> list[str]:
    if before is None or after is None:
        return ["no complete check listing could be read" if after is None else "no green verdict recorded"]
    b, a = {tuple(r) for r in before}, {tuple(r) for r in after}
    return ([f"was {' '.join(r)}" for r in sorted(b - a)] + [f"now {' '.join(r)}" for r in sorted(a - b)]) or ["same"]


def _now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def check_tool_freshness(git: Git, remote: str, allow: bool, out: Callable[[str], None],
                         loaded: dict[str, str | None] | None = None) -> None:
    """Refuse to run code that differs from the copy on the main just fetched
    into refs/remotes/<remote>/main. Called after EVERY fetch of main, so a
    main that changes these rules mid-run stops the run instead of being
    validated with the rules loaded at start."""
    ref = f"refs/remotes/{remote}/main"
    loaded = LOADED_TOOLS if loaded is None else loaded
    stale, absent = [], []
    for f in TOOL_FILES:
        theirs = git.show(ref, f"tools/maintainer/{f}")
        if theirs is None:
            absent.append(f)
        elif theirs != loaded.get(f):
            stale.append(f)
    if absent:
        out(f"NOTE origin/main has no copy of {', '.join(absent)} to compare this one with")
    if stale:
        msg = (f"the running {', '.join(stale)} differ(s) from origin/main's; restart from an up-to-date "
               "checkout of main (a stale copy carries a stale registry and stale rules)")
        if not allow:
            raise Stop(EXIT_REFUSED, msg)
        out(f"WARN {msg}; running anyway (--allow-stale-tools)")


def branch_lock_name(branch: str) -> str:
    """The repo_lock name that owns one branch: readable, and collision-free
    because two branch names that sanitise alike still hash apart."""
    return ("merge-branch-" + re.sub(r"[^A-Za-z0-9._-]", "_", branch)[:60] + "-"
            + hashlib.sha256(branch.encode()).hexdigest()[:8])


# --------------------------------------------------------------------------- #
# approve / status
# --------------------------------------------------------------------------- #

_VERDICT_RE = re.compile(r"^VERDICT:\s*APPROVED?\s*$")


def record_approval(git: Git, gh, pr_n: int, reviewer: str, head: str, evidence: Path | None,
                    remote: str = "origin", out: Callable[[str], None] = print,
                    allow_stale_tools: bool = False) -> int:
    pr = gh.pr(pr_n)
    branch = pr["headRefName"]
    git.run("fetch", "-q", "--no-tags", remote, f"+refs/heads/main:refs/remotes/{remote}/main",
            f"+refs/heads/{branch}:refs/remotes/{remote}/{branch}")
    check_tool_freshness(git, remote, allow_stale_tools, out)
    full = git.run("rev-parse", "--verify", "-q", f"{head}^{{commit}}", check=False).stdout.strip()
    if not full:
        out(f"REFUSE: {head} is not a commit in this clone (fetch it first)")
        return EXIT_REFUSED
    if full != pr["headRefOid"]:
        out(f"NOTE: {full[:12]} is not the PR's current head ({pr['headRefOid'][:12]}); recording it anyway, it "
            "carries only if the change is byte-identical")
    ev_sha = None
    if evidence is not None:
        text = evidence.read_text(encoding="utf-8", errors="replace")
        lines = [ln for ln in text.splitlines() if ln.strip()]
        if not lines or not _VERDICT_RE.match(lines[-1].strip()) or "[P1]" in text:
            out("REFUSE: the evidence must end with 'VERDICT: APPROVED' and carry no [P1] finding")
            return EXIT_REFUSED
        ev_sha = "sha256:" + hashlib.sha256(text.encode("utf-8")).hexdigest()
    base = git.out("merge-base", full, f"refs/remotes/{remote}/main")
    entry = {"reviewer": reviewer, "head": full, "base": base, "diff_hash": fingerprint(git, base, full),
             "fingerprint_version": FINGERPRINT_VERSION, "recorded_at": _now(), "evidence_sha256": ev_sha}
    State(git, pr_n).update(lambda d: d["approvals"].append(entry))
    out(f"recorded {reviewer} approval of #{pr_n}: head {full[:12]}, base {base[:12]}, {entry['diff_hash'][:19]}...")
    return EXIT_OK


def show_status(git: Git, gh, pr_n: int, remote: str = "origin", out: Callable[[str], None] = print) -> int:
    pr = gh.pr(pr_n)
    branch, head = pr["headRefName"], pr["headRefOid"]
    git.run("fetch", "-q", "--no-tags", remote, f"+refs/heads/main:refs/remotes/{remote}/main",
            f"+refs/heads/{branch}:refs/remotes/{remote}/{branch}")
    base = git.out("merge-base", head, f"refs/remotes/{remote}/main")
    data = State(git, pr_n).load()
    out(f"PR #{pr_n} {branch} head {head[:12]} base {base[:12]}")
    if not data["approvals"]:
        out("  no approvals recorded")
    for a in data["approvals"]:
        ok, reasons = approval_status(git, a, base, head)
        carried = " (carried)" if a.get("carried_from") else ""
        out(f"  {a['reviewer']}{carried} head {a['head'][:12]} base {a['base'][:12]}: "
            + ("CURRENT" if ok else "not current - " + "; ".join(reasons)))
    for sha, counts in data["reruns"].items():
        out(f"  re-runs on {sha[:12]}: {counts}")
    return EXIT_OK


# --------------------------------------------------------------------------- #
# --self-test
# --------------------------------------------------------------------------- #

def log_setup(go: str = "go1.26.9") -> str:
    return ("\ufeff2026-10-20T16:34:44.8767788Z Current runner version: '2.337.0'\n"
            "2026-10-20T16:34:44.8800000Z Included Software: https://github.com/actions/runner-images/blob/"
            "ubuntu24/20261012.601/images/ubuntu/Ubuntu2404-Readme.md\n"
            f"2026-10-20T16:34:57.8849654Z go version {go} linux/amd64\n"
            "2026-10-20T16:34:58.1887704Z go: downloading github.com/securego/gosec/v2 v2.29.0\n"
            "2026-10-20T16:34:58.0736750Z \x1b[36;1m  echo \"::error::security-gate found P1 finding(s)\"\x1b[0m\n")


LOG_SETUP = log_setup()
LOG_TAIL = ("2026-10-20T16:41:18.5413012Z ##[error]security-gate found P1 finding(s) - see logs. A poisoned/insecure\n"
            "2026-10-20T16:41:18.5420265Z ##[error]Process completed with exit code 1.\n")
LOG_REAL = (LOG_SETUP + "2026-10-20T16:41:18.5330926Z     P1 builtin:unsafe-import skills/bravo/cli/internal/x.go:11 - "
            "\"unsafe\"\n2026-10-20T16:41:18.5339006Z     P1 osv-scanner:scanner-missing skills/bravo/cli/go.mod:0 - "
            "required scanner osv-scanner not installed\n" + LOG_TAIL)
LOG_REAL_ELSEWHERE = (LOG_SETUP + "2026-10-20T16:41:18.5Z     P1 gosec:G304 skills/alpha/cli/helpers.go:4686 - "
                      "Potential file inclusion via variable\n" + LOG_TAIL)
LOG_FLAKE = (LOG_SETUP + "2026-10-14T06:44:54.0907282Z ##[error]../../../go/pkg/mod/github.com/googleapis/gax-go/v2@v2.24.0/"
             "apierror/apierror.go:45:2: google.golang.org/api@v0.293.0: read \"https://proxy.golang.org/google.golang.org/"
             "api/@v/v0.293.0.zip\": stream error: stream ID 1; INTERNAL_ERROR; received from peer\n"
             "2026-10-14T06:46:49.2Z gosec install failed (non-fatal)\n"
             "2026-10-14T06:46:49.2336487Z     P1 gosec:scanner-missing skills/bravo/cli/go.mod:0 - required scanner "
             "gosec not installed (fail-closed in CI/release).\n" + LOG_TAIL)
LOG_UNKNOWN = LOG_SETUP + "2026-10-20T16:41:18.5Z something odd happened\n" + LOG_TAIL
LOG_OK = LOG_SETUP + "2026-10-20T16:41:18.5Z PASS\n"


def log_ok(go: str = "go1.26.9") -> str:
    return log_setup(go) + "2026-10-20T16:41:18.5Z PASS\n"


class FakeGitHub:
    """A scripted GitHub over a local bare 'origin': PR head = the branch ref
    there, merges update its main, checks come from `responder`."""

    def __init__(self, origin: Path, env: dict, pr: int, branch: str, title: str,
                 responder: Callable[[str, str, int], tuple[str, str]], names: tuple[str, ...] = ("guards", "security-gate"),
                 on_snapshot: Callable[["FakeGitHub", str, int], None] | None = None) -> None:
        self.origin, self.env, self.n, self.branch, self.title = origin, env, pr, branch, title
        self.responder, self.names, self.on_snapshot = responder, names, on_snapshot
        self.state = "OPEN"
        self.attempts: dict[tuple[str, str], int] = {}
        self.ids: dict[int, tuple[str, str, int]] = {}
        self.base_checks: dict[tuple[str, str], tuple[str, str]] = {}
        self.reruns: list[str] = []
        self.merged: str | None = None
        self.merge_oid: str | None = None
        self.refuse_merges = 0
        self.base_ref = "main"
        self.main_ci: str | None = "success"
        self.before_merge: Callable[["FakeGitHub"], None] | None = None
        self.merge_calls = 0
        self.snapshots = 0
        self.snapshot_error_after_merge = False
        self._next = 1000

    def _git(self, *a: str) -> str:
        return subprocess.run(["git", "--git-dir", str(self.origin), *a], capture_output=True, text=True,
                              env=self.env, check=True).stdout.strip()

    def pr(self, n: int) -> dict:
        head = self._git("rev-parse", f"refs/heads/{self.branch}") if self.state == "OPEN" else self.merged
        return {"number": n, "state": self.state, "isDraft": False, "headRefName": self.branch, "headRefOid": head,
                "baseRefName": self.base_ref, "isCrossRepository": False, "mergeable": "MERGEABLE", "title": self.title,
                "mergeCommit": {"oid": self.merge_oid} if self.merge_oid else None}

    def required_contexts(self) -> list[str]:
        return ["security-gate"]

    def _check(self, sha: str, name: str) -> dict:
        attempt = self.attempts.setdefault((sha, name), 1)
        conclusion, log = self.responder(sha, name, attempt)
        self._next += 1
        self.ids[self._next] = (sha, name, attempt)
        self.logs = getattr(self, "logs", {})
        self.logs[self._next] = log
        if conclusion == "pending":  # a re-run still going
            return {"id": self._next, "name": name, "status": "in_progress", "conclusion": None, "app": "github-actions"}
        return {"id": self._next, "name": name, "status": "completed", "conclusion": conclusion, "app": "github-actions"}

    def snapshot(self, sha: str) -> dict:
        self.snapshots += 1
        if self.snapshot_error_after_merge and self.state == "MERGED":
            raise Stop(EXIT_ERROR, "gh api failed: HTTP 502: Bad Gateway")
        if self.on_snapshot:
            self.on_snapshot(self, sha, self.snapshots)
        cache = getattr(self, "cache", {})
        self.cache = cache
        for name in self.names:
            key = (sha, name, self.attempts.get((sha, name), 1))
            if key not in cache:
                cache[key] = self._check(sha, name)
        checks = [cache[(sha, nm, self.attempts[(sha, nm)])] for nm in self.names]
        by = {c["name"]: c for c in checks}
        runs = [{"name": run, "status": by[check]["status"], "event": "pull_request",
                 "conclusion": by[check]["conclusion"]} for run, check in (("CI", "guards"),
                                                                          ("security-gate", "security-gate"))]
        return {"runs": runs, "checks": checks, "statuses": [{"context": "CodeRabbit", "state": "success"}]}

    def job_log(self, job_id: int) -> str:
        return self.logs.get(job_id, "")

    def rerun_job(self, job_id: int) -> None:
        sha, name, attempt = self.ids[job_id]
        self.reruns.append(name)
        self.attempts[(sha, name)] = attempt + 1

    def check_on_commit(self, sha: str, name: str) -> dict | None:
        hit = self.base_checks.get((sha, name))
        if hit is None:
            return None
        self._next += 1
        self.logs = getattr(self, "logs", {})
        self.logs[self._next] = hit[1]
        return {"id": self._next, "name": name, "status": "completed", "conclusion": hit[0]}

    def catalog_busy(self) -> bool:
        return False

    def main_runs(self, sha: str) -> tuple[list[dict], bool]:
        """Main's push CI for a merge commit: "success", "failure", None
        (never reports) or "error" (the lookup itself fails)."""
        if self.main_ci == "error":
            raise Stop(EXIT_ERROR, "gh api failed: HTTP 503: Service Unavailable")
        if self.main_ci is None:
            return [], True
        return [{"id": 9, "name": "CI", "status": "completed", "conclusion": self.main_ci}], True

    def merge(self, n: int, sha: str) -> tuple[bool, str]:
        self.merge_calls += 1
        if self.refuse_merges > 0:
            self.refuse_merges -= 1
            return False, "Base branch was modified. Review and try the merge again."
        if self.before_merge:
            self.before_merge(self)  # e.g. a commit lands on main inside the call
        if self._git("rev-parse", f"refs/heads/{self.branch}") != sha:
            return False, "Head branch was modified. Review and try the merge again (head does not match)."
        main = self._git("rev-parse", "refs/heads/main")
        tree = self._git("rev-parse", f"{sha}^{{tree}}")
        p = subprocess.run(["git", "--git-dir", str(self.origin), "commit-tree", tree, "-p", main, "-m",
                            f"{self.title} (#{n})"], capture_output=True, text=True, env=self.env, check=True)
        self._git("update-ref", "refs/heads/main", p.stdout.strip(), main)
        self.state, self.merged, self.merge_oid = "MERGED", sha, p.stdout.strip()
        return True, ""


def _self_test() -> int:
    failures: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    print("log classification (flake vs real, from fake check logs):")
    v = classify_log(LOG_REAL)
    expect(v.kind == "real", "a security finding is real, even beside a scanner-install fault")
    expect(classify_log(LOG_FLAKE).kind == "flake", "a proxy.golang.org stream error + scanner-missing is a flake")
    expect(classify_log(LOG_UNKNOWN).kind == "unknown", "only 'exit code 1' is unknown, not real and not a flake")
    expect(classify_log(LOG_SETUP).kind == "unknown",
           "a step's echoed script (cyan '::error::' text) is not read as an error")
    expect(classify_log(LOG_REAL_ELSEWHERE).kind == "real",
           "a finding in a file outside the change is still real (location never clears a check)")
    expect(classify_log("x\tstep\t2026-10-20T16:41:18.5Z --- FAIL: TestThing (0.01s)\n").kind == "real",
           "a `gh run view --log` line with its job/step prefix is read")
    expect(v.versions.get("go") == "go1.26.9" and v.versions.get("gosec") == "v2.29.0"
           and v.versions.get("runner-image") == "20261012.601", "tool versions are read from the log")
    expect(same_versions({"go": "1"}, {"go": "1"}) and not same_versions({"go": "1"}, {"go": "2"})
           and not same_versions({}, {}) and not same_versions({"go": "1"}, {"go": "1", "gosec": "v2"}),
           "tool versions compare equal only when every one is known and identical")
    expect(not same_versions({"runner-image": "x"}, {"runner-image": "x"}),
           "a matching runner image alone is not proof of the same tools")
    expect(relevant_tools(classify_log(LOG_REAL_ELSEWHERE)) == {"gosec"}
           and not same_versions({"go": "1"}, {"go": "1"}, {"gosec"})
           and same_versions({"go": "1", "gosec": "v2"}, {"go": "1", "gosec": "v2"}, {"gosec"}),
           "the scanner a finding names must have a known, equal version on both runs")

    tmp = Path(tempfile.mkdtemp(prefix="merge-when-green-selftest-"))
    procs: list[subprocess.Popen] = []
    try:
        env = rr.fixture_env(tmp)
        for k in ("GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM", "GIT_EDITOR", "GIT_TERMINAL_PROMPT"):
            os.environ[k] = env[k]

        def sh(*a: str, cwd: Path, check: bool = True) -> str:
            return subprocess.run(list(a), cwd=str(cwd), env=env, capture_output=True, text=True,
                                  check=check).stdout.strip()

        counter = [0]

        def world(branch: str = "skill/bravo"):
            counter[0] += 1
            d = tmp / f"w{counter[0]}"
            d.mkdir()
            work = rr.make_fixture_repo(d, env)
            return d / "origin.git", work, branch

        def green(sha: str, name: str, attempt: int) -> tuple[str, str]:
            return "success", LOG_OK

        def merger(work: Path, fake: FakeGitHub, **kw) -> tuple[int, list[str]]:
            logs: list[str] = []
            # The fixture's stand-in build-catalog.py differs from the real one by
            # design, so fixture runs allow it; the freshness test passes False.
            kw.setdefault("allow_stale_tools", True)
            tools = kw.pop("tools", None)
            opts = Options(pr=fake.n, poll_s=0, settle_s=0, timeout_s=60, bot_wait_s=0, **kw)
            clk = [0.0]  # a fake clock: every sleep advances it, so timeouts end at once

            def fake_sleep(s: float, c=clk) -> None:
                c[0] += max(s, 1.0)
            m = Merger(Git(work, env), fake, opts, out=logs.append, sleep=fake_sleep, clock=lambda c=clk: c[0])
            if tools is not None:
                m.tools = tools  # the rules "loaded at start", for the mid-run freshness cases
            return m.run(), logs

        def approve(work: Path, fake: FakeGitHub, head: str, ev: str = "Looks right.\nVERDICT: APPROVED\n") -> int:
            evp = tmp / f"ev{counter[0]}.txt"
            evp.write_text(ev)
            return record_approval(Git(work, env), fake, fake.n, "codex", head, evp, out=lambda s: None,
                                   allow_stale_tools=True)

        def origin_rev(origin: Path, ref: str) -> str:
            return sh("git", "--git-dir", str(origin), "rev-parse", ref, cwd=tmp)

        def new_connector_pr(work: Path, extra_commit: bool = True) -> str:
            sh("git", "checkout", "-q", "-b", "skill/bravo", cwd=work)
            rr.add_connector(work, env, "bravo")
            if extra_commit:
                (work / "skills/bravo/SKILL.md").write_text("# bravo\n\nsecond commit\n")
                sh("git", "commit", "-q", "-s", "-am", "bravo: docs", cwd=work)
            sh("git", "push", "-q", "origin", "skill/bravo", cwd=work)
            sh("git", "checkout", "-q", "main", cwd=work)
            return sh("git", "rev-parse", "skill/bravo", cwd=work)

        def main_connector(work: Path, slug: str) -> None:
            sh("git", "checkout", "-q", "main", cwd=work)
            sh("git", "pull", "-q", "--ff-only", "origin", "main", cwd=work)
            rr.add_connector(work, env, slug)
            sh("git", "push", "-q", "origin", "main", cwd=work)

        def main_commit(work: Path, fn: Callable[[Path], None], msg: str) -> None:
            sh("git", "checkout", "-q", "main", cwd=work)
            sh("git", "pull", "-q", "--ff-only", "origin", "main", cwd=work)
            fn(work)
            sh("git", "add", "-A", cwd=work)
            sh("git", "commit", "-q", "-s", "-m", msg, cwd=work)
            sh("git", "push", "-q", "origin", "main", cwd=work)

        print("fingerprint (byte-exact, not git patch-id):")
        origin, work, _b = world()
        base = sh("git", "rev-parse", "HEAD", cwd=work)
        heads = []
        for lit in ('"a b"', '"a  b"'):
            sh("git", "checkout", "-q", "--detach", base, cwd=work)
            (work / "skills/alpha/cli/main.go").write_text(f"package main\n\nconst name = {lit}\n")
            sh("git", "commit", "-q", "-s", "-am", "lit", cwd=work)
            heads.append(sh("git", "rev-parse", "HEAD", cwd=work))
        pids = [sh("sh", "-c", f"git diff {base} {h} | git patch-id --stable", cwd=work).split()[0] for h in heads]
        g = Git(work, env)
        expect(pids[0] == pids[1], "git patch-id gives two different string literals the same id (the defect)")
        expect(fingerprint(g, base, heads[0]) != fingerprint(g, base, heads[1]),
               "the change fingerprint tells them apart")
        expect(fingerprint(g, base, heads[0]) == fingerprint(g, base, heads[0]), "and is stable for one change")
        sh("git", "checkout", "-q", "main", cwd=work)

        # T1: two-commit connector PR, registry conflict with main, approval carried, merged.
        print("known registry conflict + squash + approval carried across the rebase:")
        origin, work, branch = world()
        head = new_connector_pr(work)
        fake = FakeGitHub(origin, env, 7, branch, "skill: bravo", green)
        expect(approve(work, fake, head) == 0, "approval recorded against the original head")
        main_connector(work, "charlie")
        rc, logs = merger(work, fake)
        merged = fake.merged
        expect(rc == 0 and merged is not None, f"merged (rc={rc})")
        expect(any("squashed 2 commit(s)" in s for s in logs), "the two commits were squashed into one")
        expect(any("handing it to resolve_registry" in s for s in logs), "the registry conflict went to the resolver")
        expect(any("carried" in s for s in logs), "the approval carried: the change is byte-identical after rebase")
        main_reg = json.loads(sh("git", "--git-dir", str(origin), "show", f"main:{rr.SKILLS_JSON}", cwd=tmp))
        expect({"bravo", "charlie"} <= set(main_reg["skills"]), "main ends with both connectors registered")
        if merged:
            body = sh("git", "--git-dir", str(origin), "log", "-1", "--format=%B", merged, cwd=tmp)
            expect(body.count("Signed-off-by:") >= 1 and "bravo: docs" in body,
                   "the squash commit keeps the sign-off and lists the squashed subjects")
            expect(sh("git", "--git-dir", str(origin), "rev-list", "--count", f"{merged}^..{merged}", cwd=tmp) == "1",
                   "the pushed head is a single commit")

        # T1b: push.followTags=true and an annotated tag reachable from the push.
        origin, work, branch = world()
        new_connector_pr(work)  # two commits: squashed, so a push happens
        sh("git", "tag", "-a", "decoy-annotated", "-m", "decoy", "main", cwd=work)
        sh("git", "config", "push.followTags", "true", cwd=work)
        fake = FakeGitHub(origin, env, 30, branch, "skill: bravo", green)
        rc, logs = merger(work, fake, require=())
        tags_on_origin = sh("git", "--git-dir", str(origin), "tag", "-l", cwd=tmp)
        expect(rc == 0 and any("pushed" in s for s in logs) and "decoy-annotated" not in tags_on_origin,
               "with push.followTags=true, the branch push carries no tag")

        # T2: unknown conflict stops, pushes nothing, merges nothing.
        print("unknown conflict:")
        origin, work, branch = world()
        sh("git", "checkout", "-q", "-b", "fix/alpha", cwd=work)
        (work / "skills/alpha/SKILL.md").write_text("# alpha\n\nline one\nBRANCH\nline three\n")
        sh("git", "commit", "-q", "-s", "-am", "fix", cwd=work)
        sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
        before = origin_rev(origin, "refs/heads/fix/alpha")
        main_commit(work, lambda w: (w / "skills/alpha/SKILL.md").write_text("# alpha\n\nline one\nMAIN\nline three\n"),
                    "main edit")
        fake = FakeGitHub(origin, env, 8, "fix/alpha", "fix", green)
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_CONFLICT and fake.merged is None, f"stops with exit 3 (rc={rc})")
        expect(origin_rev(origin, "refs/heads/fix/alpha") == before, "nothing was pushed")
        expect(any("skills/alpha/SKILL.md" in s for s in logs), "the unknown path is named")

        # T3/T4/T5: a dirty worktree, a busy worktree and a held lock are refused.
        print("worktree ownership:")
        origin, work, branch = world()
        head = new_connector_pr(work, extra_commit=False)
        side = tmp / f"side{counter[0]}"
        sh("git", "worktree", "add", "-q", str(side), "skill/bravo", cwd=work)
        (side / "scratch.txt").write_text("unsaved work\n")
        fake = FakeGitHub(origin, env, 9, branch, "skill: bravo", green)
        rc, logs = merger(work, fake, require=(), dry_run=True)
        expect(rc == EXIT_REFUSED and any("dirty" in s for s in logs), "a dirty branch worktree is refused")
        expect((side / "scratch.txt").read_text() == "unsaved work\n", "and its unsaved file is untouched")
        (side / "scratch.txt").unlink()
        sleeper = subprocess.Popen(["sleep", "60"], cwd=str(side))
        procs.append(sleeper)
        time.sleep(0.3)
        rc, logs = merger(work, fake, require=(), dry_run=True)
        expect(rc == EXIT_REFUSED and any(f"pid {sleeper.pid}" in s for s in logs),
               "a worktree another live process sits in is refused")
        os.kill(sleeper.pid, signal.SIGTERM)
        sleeper.wait()
        holder = subprocess.Popen([sys.executable, "-c",
                                   "import sys, time; sys.path.insert(0, sys.argv[1]); import repo_lock, merge_when_green as m;"
                                   "repo_lock.acquire(m.branch_lock_name('skill/bravo'), root=sys.argv[2]);"
                                   "print('held', flush=True); time.sleep(60)",
                                   str(Path(__file__).resolve().parent), str(work)], stdout=subprocess.PIPE, text=True, env=env)
        procs.append(holder)
        holder.stdout.readline()
        rc, logs = merger(work, fake, require=(), dry_run=True)
        expect(rc == EXIT_REFUSED and any("another run owns this branch" in s for s in logs),
               "a branch another run has locked is refused")
        os.kill(holder.pid, signal.SIGTERM)
        holder.wait()
        rc, logs = merger(work, fake, require=())
        expect(rc == 0 and fake.merged is not None, "with the worktree clean, idle and unlocked it merges")
        expect(sh("git", "rev-parse", "HEAD", cwd=side) == head, "the side worktree was never moved")

        # T6/T7/T8: approvals across rebases.
        print("approvals across a rebase:")

        def fix_pr(work: Path) -> str:
            sh("git", "checkout", "-q", "-b", "fix/alpha", cwd=work)
            (work / "skills/alpha/SKILL.md").write_text("# alpha\n\nline one\nFIXED\nline three\n")
            sh("git", "commit", "-q", "-s", "-am", "fix alpha docs", cwd=work)
            sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
            sh("git", "checkout", "-q", "main", cwd=work)
            return sh("git", "rev-parse", "fix/alpha", cwd=work)

        for label, edit, want in (
                ("main changed only an unrelated file: the approval carries and it merges",
                 lambda w: (w / "skills/delta/SKILL.md").write_text("# delta\nchanged on main\n"), 0),
                ("main changed the connector's directory: the approval is stale (exit 4)",
                 lambda w: (w / "skills/alpha/cli/main.go").write_text('package main\n\nconst name = "x"\n'), 4),
                ("main changed shared build machinery: the approval is stale (exit 4)",
                 lambda w: (w / ".github/workflows").mkdir(parents=True, exist_ok=True)
                 or (w / ".github/workflows/ci.yml").write_text("name: CI\n"), 4)):
            origin, work, _b = world()
            head = fix_pr(work)
            fake = FakeGitHub(origin, env, 10, "fix/alpha", "fix alpha docs", green)
            approve(work, fake, head)
            main_commit(work, edit, "main moves")
            rc, logs = merger(work, fake)
            expect(rc == want and (fake.merged is not None) == (want == 0), f"{label} (rc={rc})")
        origin, work, _b = world()
        head = fix_pr(work)
        fake = FakeGitHub(origin, env, 11, "fix/alpha", "fix alpha docs", green)
        approve(work, fake, head)
        sh("git", "checkout", "-q", "fix/alpha", cwd=work)
        (work / "skills/alpha/SKILL.md").write_text("# alpha\n\nline one\nFIXED \nline three\n")
        sh("git", "commit", "-q", "-s", "-am", "one more byte", cwd=work)
        sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
        sh("git", "checkout", "-q", "main", cwd=work)
        rc, logs = merger(work, fake)
        expect(rc == EXIT_APPROVAL and any("not byte-identical" in s for s in logs),
               "a one-byte change after approval (trailing space) loses it")
        origin, work, _b = world()
        sh("git", "checkout", "-q", "-b", "fix/guide", cwd=work)
        (work / "docs/guide.md").write_text((work / "docs/guide.md").read_text().replace("guide line 20", "GUIDE 20"))
        sh("git", "commit", "-q", "-s", "-am", "guide", cwd=work)
        sh("git", "push", "-q", "origin", "fix/guide", cwd=work)
        head = sh("git", "rev-parse", "HEAD", cwd=work)
        fake = FakeGitHub(origin, env, 12, "fix/guide", "guide", green)
        approve(work, fake, head)
        main_commit(work, lambda w: (w / "docs/guide.md").write_text("new first line\n" + (w / "docs/guide.md").read_text()),
                    "main inserts a line")
        rc, logs = merger(work, fake)
        expect(rc == EXIT_APPROVAL and any("not byte-identical" in s for s in logs),
               "a rebase that shifts the change's own hunk changes the fingerprint and loses the approval")
        ev_bad = record_approval(Git(work, env), fake, 12, "codex", head, None, out=lambda s: None,
                                 allow_stale_tools=True)
        p1 = tmp / "p1.txt"
        p1.write_text("[P1] broken\nVERDICT: APPROVED\n")
        expect(ev_bad == 0 and record_approval(Git(work, env), fake, 12, "codex", head, p1,
                                               out=lambda s: None, allow_stale_tools=True) == EXIT_REFUSED,
               "evidence carrying a [P1] finding is refused as an approval")

        # T8b: content a generator does not reproduce keeps its fingerprint.
        print("approvals and generated files:")
        for label, edit in (
                ("a file added under docs/skills/ that no generator writes loses the approval",
                 lambda w: (w / "docs/skills").mkdir(exist_ok=True) or (w / "docs/skills/unreviewed.html")
                 .write_text("<script>unreviewed</script>\n")),
                ("an edit to README prose outside the generated blocks loses the approval",
                 lambda w: (w / "README.md").write_text((w / "README.md").read_text()
                                                         .replace("Outro prose.", "Outro prose, edited."))),
                ("a filename with a leading space is fingerprinted by its real name",
                 lambda w: (w / " config.json").write_text("{\"x\": 2}\n"))):
            origin, work, _b = world()
            head = fix_pr(work)
            fake = FakeGitHub(origin, env, 21, "fix/alpha", "fix alpha docs", green)
            approve(work, fake, head)
            sh("git", "checkout", "-q", "fix/alpha", cwd=work)
            edit(work)
            sh("git", "add", "-A", cwd=work)
            sh("git", "commit", "-q", "-s", "-m", "after approval", cwd=work)
            sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
            sh("git", "checkout", "-q", "main", cwd=work)
            rc, logs = merger(work, fake)
            expect(rc == EXIT_APPROVAL and any("not byte-identical" in s for s in logs), f"{label} (rc={rc})")
        origin, work, _b = world()
        g = Git(work, env)
        b0 = sh("git", "rev-parse", "HEAD", cwd=work)
        hs = []
        for content in ('{"x": 1}\n', '{"x": 2}\n'):
            sh("git", "checkout", "-q", "--detach", b0, cwd=work)
            (work / " config.json").write_text(content)
            sh("git", "add", "-A", cwd=work)
            sh("git", "commit", "-q", "-s", "-m", "space", cwd=work)
            hs.append(sh("git", "rev-parse", "HEAD", cwd=work))
        sh("git", "checkout", "-q", "main", cwd=work)
        expect(changed_paths(g, b0, hs[0]) == [" config.json"]
               and fingerprint(g, b0, hs[0]) != fingerprint(g, b0, hs[1]),
               "two contents of ' config.json' get two fingerprints (the leading space is kept)")

        # T8b1: a value a generator preserves from its own output.
        origin, work, _b = world()
        head = fix_pr(work)
        fake = FakeGitHub(origin, env, 31, "fix/alpha", "fix alpha docs", green)
        approve(work, fake, head)
        sh("git", "checkout", "-q", "fix/alpha", cwd=work)
        cat = json.loads((work / "catalog.json").read_text())
        cat["generated_at"] = "[click here](https://evil.example/x)"
        (work / "catalog.json").write_text(json.dumps(cat, indent=2) + "\n")
        sh("git", "commit", "-q", "-s", "-am", "after approval: catalog generated_at", cwd=work)
        sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
        sh("git", "checkout", "-q", "main", cwd=work)
        rc, logs = merger(work, fake)
        merged_readme = sh("git", "--git-dir", str(origin), "show", "main:README.md", cwd=tmp)
        merged_cat = sh("git", "--git-dir", str(origin), "show", "main:catalog.json", cwd=tmp)
        expect(rc == 0 and "evil.example" not in merged_readme and "evil.example" not in merged_cat,
               "a generated_at edited after approval never reaches main (catalog.json is restored from the base "
               "before regenerating)")

        # T8b1b: a symlink planted after approval is refused, never written through.
        origin, work, _b = world()
        head = fix_pr(work)
        fake = FakeGitHub(origin, env, 32, "fix/alpha", "fix alpha docs", green)
        approve(work, fake, head)
        outside = tmp / "outside-catalog-target.json"
        outside.write_text("someone's file\n")
        sh("git", "checkout", "-q", "fix/alpha", cwd=work)
        (work / "catalog.json").unlink()
        (work / "catalog.json").symlink_to(outside)
        sh("git", "add", "-A", cwd=work)
        sh("git", "commit", "-q", "-s", "-m", "after approval: catalog.json becomes a symlink", cwd=work)
        sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
        sh("git", "checkout", "-q", "main", cwd=work)
        rc, logs = merger(work, fake)
        expect(rc == EXIT_CONFLICT and outside.read_text() == "someone's file\n" and fake.merged is None,
               "a catalog.json turned into a symlink after approval is refused; its target is never written")

        # T8b2: a page nothing regenerates any more is reviewed content.
        origin, work, _b = world()
        (work / "docs/skills").mkdir(parents=True, exist_ok=True)
        (work / "docs/skills/alpha.md").write_text("# alpha page\n")
        sh("git", "add", "-A", cwd=work)
        sh("git", "commit", "-q", "-s", "-m", "alpha page", cwd=work)
        sh("git", "push", "-q", "origin", "main", cwd=work)
        sh("git", "checkout", "-q", "-b", "fix/alpha", cwd=work)
        reg = json.loads((work / rr.SKILLS_JSON).read_text())
        reg["skills"]["alpha"]["markdown_only"] = True
        (work / rr.SKILLS_JSON).write_text(json.dumps(reg, indent=2, ensure_ascii=True) + "\n")
        sh("git", "commit", "-q", "-s", "-am", "alpha becomes markdown-only", cwd=work)
        sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
        fake = FakeGitHub(origin, env, 25, "fix/alpha", "alpha md-only", green)
        approve(work, fake, sh("git", "rev-parse", "HEAD", cwd=work))
        (work / "docs/skills/alpha.md").write_text("# alpha page\n\n<script>unreviewed</script>\n")
        sh("git", "commit", "-q", "-s", "-am", "edit the orphaned page", cwd=work)
        sh("git", "push", "-q", "origin", "fix/alpha", cwd=work)
        sh("git", "checkout", "-q", "main", cwd=work)
        rc, logs = merger(work, fake)
        expect(rc == EXIT_APPROVAL and any("not byte-identical" in s for s in logs),
               "editing the page of a connector the change made markdown-only loses the approval")

        # T8b3: a registry-wide field changing on main invalidates approvals.
        origin, work, branch = world()
        head = new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 26, branch, "skill: bravo", green)
        approve(work, fake, head)

        def owner_change(w: Path) -> None:
            reg = json.loads((w / rr.SKILLS_JSON).read_text())
            reg["owner"] = "someone-else"
            (w / rr.SKILLS_JSON).write_text(json.dumps(reg, indent=2, ensure_ascii=True) + "\n")
        main_commit(work, owner_change, "registry owner changes")
        rc, logs = merger(work, fake)
        expect(rc == EXIT_APPROVAL and any("registry-wide top:owner" in s for s in logs),
               "main changing the registry's owner makes every approval stale")

        # T8b5: a listing cut short at a page boundary is never green.
        class _Stub:
            def __init__(self, rc: int, out: str) -> None:
                self.returncode, self.stdout, self.stderr = rc, out, ""

        def stub_for(statuses: list[dict], total: int, combined: str):
            def _gh(*a: str, check: bool = True) -> _Stub:
                url = next(x for x in a if x.startswith("repos/"))
                paged = "--paginate" in a
                if "/actions/runs" in url:
                    return _Stub(0, '{"id":1,"name":"CI","event":"pull_request","status":"completed","conclusion":"success"}'
                                 if paged else '{"n":1}')
                if "/check-runs" in url:
                    return _Stub(0, '{"id":2,"name":"security-gate","status":"completed","conclusion":"success",'
                                    '"app":"github-actions"}' if paged else '{"n":1}')
                return _Stub(0, "\n".join(json.dumps(s) for s in statuses) if paged
                             else json.dumps({"n": total, "state": combined}))
            return _gh
        ok_statuses = [{"context": f"ctx{i}", "state": "success"} for i in range(30)]
        verdicts = []
        for statuses, total, combined in ((ok_statuses, 30, "success"), (ok_statuses, 31, "failure"),
                                          (ok_statuses, 30, "failure"), (ok_statuses, 30, "pending")):
            g2 = GitHub("o/r")
            g2._gh = stub_for(statuses, total, combined)  # type: ignore[method-assign]
            m2 = Merger(Git(work, env), g2, Options(pr=1, expect_workflows=("CI",)), out=lambda s: None)
            pending, failed = m2._evaluate(g2.snapshot("abc"), ["security-gate"])
            verdicts.append(("green" if not pending and not failed else "pending" if pending else "failed"))
        expect(verdicts == ["green", "pending", "failed", "pending"],
               f"30 of 30 statuses is green; 30 of 31 (a failure on page two) is not; a failing combined state "
               f"fails; a pending combined state waits ({verdicts})")

        # T8b4: required checks must be readable.
        class _P:
            def __init__(self, rc: int, out: str = "", err: str = "") -> None:
                self.returncode, self.stdout, self.stderr = rc, out, err
        gh_real = GitHub("o/r")
        verdicts = []
        for rc_, out_, err_ in ((0, '["security-gate"]', ""), (1, "", "HTTP 404: Branch not protected"),
                                (1, "", "HTTP 403: API rate limit exceeded for installation"),
                                (1, "", "HTTP 502: Bad Gateway")):
            gh_real._gh = lambda *a, check=True, r=(rc_, out_, err_): _P(*r)  # type: ignore[method-assign]
            try:
                verdicts.append(gh_real.required_contexts())
            except Stop as s:
                verdicts.append(s.code)
        expect(verdicts == [["security-gate"], [], EXIT_REFUSED, EXIT_REFUSED],
               f"required checks: read, 'not protected' is none, a 403 or 5xx refuses ({verdicts})")

        # T8c: the merge call itself.
        print("the merge call (GitHub cannot pin the base):")
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 22, branch, "skill: bravo", green)
        fake.refuse_merges = 1
        rc, logs = merger(work, fake, require=())
        expect(rc == 0 and fake.merge_calls == 2 and any("refused the merge" in s for s in logs),
               "a refused merge is re-validated from the top, then merged (no blind retry)")
        expect(any("on the validated base" in s for s in logs),
               "a clean merge is proven: the merge commit's parent is the validated base, its tree the green tree")
        for main_ci, want, label in (("success", EXIT_RACE, "main's CI then passes: exit 7, reported with what "
                                                             "the window touched"),
                                     ("failure", EXIT_RACE_RED, "main's CI then fails: exit 8, the chain stops"),
                                     (None, EXIT_RACE_RED, "main's CI never reports: exit 8, unverified"),
                                     ("error", EXIT_RACE_RED, "main's CI cannot be read (HTTP 503): exit 8, "
                                                              "unverified, not exit 1")):
            origin, work, branch = world()
            new_connector_pr(work, extra_commit=False)
            fake = FakeGitHub(origin, env, 23, branch, "skill: bravo", green)
            fake.main_ci = main_ci

            def land_on_main(fk: FakeGitHub, w=work) -> None:
                main_commit(w, lambda x: (x / "skills/delta/cli/main.go").write_text("package main // raced\n"),
                            "lands inside the merge call")
            fake.before_merge = land_on_main
            rc, logs = merger(work, fake, require=())
            expect(rc == want and any("RACE" in s and "skills/delta/cli/main.go" in s for s in logs),
                   f"main moving inside the merge call is detected; {label} (rc={rc})")

        # T8c2: the PR is retargeted while its checks run.
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)

        def retarget(fk: FakeGitHub, sha: str, n: int) -> None:
            fk.base_ref = "release"
        fake = FakeGitHub(origin, env, 27, branch, "skill: bravo", green, on_snapshot=retarget)
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_REFUSED and fake.merged is None and any("targets release, not main" in s for s in logs),
               "a PR retargeted away from main while it waits is never merged")
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 28, branch, "skill: bravo", green)
        orig_approvals = Merger._approvals

        def approvals_then_retarget(self, base, head, record, fk=fake):  # retarget after the last checks
            out = orig_approvals(self, base, head, record)
            if record:
                fk.base_ref = "release"
            return out
        Merger._approvals = approvals_then_retarget  # type: ignore[method-assign]
        try:
            rc, logs = merger(work, fake, require=())
        finally:
            Merger._approvals = orig_approvals  # type: ignore[method-assign]
        expect(rc == EXIT_REFUSED and fake.merged is None and fake.merge_calls == 0,
               "a PR retargeted after the final checks is caught by the look immediately before the merge call")

        # T8c3: the PR is retargeted, or its worktree dirtied, before the push.
        for label, act in (("the PR retargeted", "retarget"), ("the branch's worktree dirtied", "dirty")):
            origin, work, branch = world()
            new_connector_pr(work)  # two commits: a squash and a push are needed
            side = tmp / f"side-{act}-{counter[0]}"
            sh("git", "worktree", "add", "-q", str(side), "skill/bravo", cwd=work)
            head_before = origin_rev(origin, "refs/heads/skill/bravo")
            fake = FakeGitHub(origin, env, 29, branch, "skill: bravo", green)
            orig_prepare = Merger._prepare

            def prepare_then_act(self, head, base, title, fk=fake, a=act, s=side, orig=orig_prepare):
                out = orig(self, head, base, title)
                if a == "retarget":
                    fk.base_ref = "release"
                else:
                    (s / "unsaved.txt").write_text("work in progress\n")
                return out
            Merger._prepare = prepare_then_act  # type: ignore[method-assign]
            try:
                rc, logs = merger(work, fake, require=())
            finally:
                Merger._prepare = orig_prepare  # type: ignore[method-assign]
            expect(rc == EXIT_REFUSED and origin_rev(origin, "refs/heads/skill/bravo") == head_before,
                   f"{label} after preparing and before the push: the branch is not rewritten")

        # T8d: ownership-aware lock release.
        print("lock ownership:")
        origin, work, _b = world()
        code = ("import sys, time, pathlib; sys.path.insert(0, sys.argv[1]); import repo_lock;"
                "l = repo_lock.acquire('own-test', root=sys.argv[2]); repo_lock.release(l);"
                "pathlib.Path(sys.argv[3]).write_text('released');"
                "[time.sleep(0.05) for _ in range(200) if not pathlib.Path(sys.argv[4]).exists()];"
                "repo_lock.release(l)")  # a second release AFTER the next owner acquired
        a_done, b_has = tmp / "a-released", tmp / "b-acquired"
        a = subprocess.Popen([sys.executable, "-c", code, str(TOOLS_DIR), str(work), str(a_done), str(b_has)], env=env)
        procs.append(a)
        for _ in range(200):
            if a_done.exists():
                break
            time.sleep(0.05)
        holder = subprocess.Popen([sys.executable, "-c",
                                   "import sys, time, pathlib; sys.path.insert(0, sys.argv[1]); import repo_lock;"
                                   "repo_lock.acquire('own-test', root=sys.argv[2]);"
                                   "pathlib.Path(sys.argv[3]).write_text('x'); time.sleep(60)",
                                   str(TOOLS_DIR), str(work), str(b_has)], env=env)
        procs.append(holder)
        a.wait(timeout=20)
        lockfile = Git(work, env).common_dir() / "msp-skills-locks" / "own-test.lock"
        held_by_b = lockfile.is_file() and json.loads(lockfile.read_text() or "{}").get("pid") == holder.pid
        third = subprocess.run([sys.executable, "-c", "import sys; sys.path.insert(0, sys.argv[1]); import repo_lock\n"
                                "try:\n    repo_lock.acquire('own-test', root=sys.argv[2]); print('acquired')\n"
                                "except repo_lock.LockHeld:\n    print('held')", str(TOOLS_DIR), str(work)],
                               capture_output=True, text=True, env=env).stdout.strip()
        expect(held_by_b and third == "held",
               "a process that released its lock cannot disturb the next owner's (a later release or its exit)")
        os.kill(holder.pid, signal.SIGKILL)
        holder.wait()
        again = subprocess.run([sys.executable, "-c", "import sys; sys.path.insert(0, sys.argv[1]); import repo_lock\n"
                                "repo_lock.acquire('own-test', root=sys.argv[2]); print('acquired')",
                                str(TOOLS_DIR), str(work)], capture_output=True, text=True, env=env).stdout.strip()
        expect(again == "acquired", "a killed holder's lock is free at once (the kernel drops it; no stale reclaim)")

        def probe_lock(name: str) -> str:
            return subprocess.run([sys.executable, "-c", "import sys; sys.path.insert(0, sys.argv[1]); import repo_lock\n"
                                   "try:\n    repo_lock.acquire(sys.argv[3], root=sys.argv[2]); print('acquired')\n"
                                   "except repo_lock.LockHeld:\n    print('held')", str(TOOLS_DIR), str(work), name],
                                  capture_output=True, text=True, env=env).stdout.strip()
        a1 = repo_lock.acquire("handle-test", root=work)
        repo_lock.release(a1)
        b1 = repo_lock.acquire("handle-test", root=work)
        repo_lock.release(a1)  # an old handle from the earlier acquisition
        expect(probe_lock("handle-test") == "held", "an old handle cannot release a later acquisition")
        if hasattr(os, "fork"):
            sys.stdout.flush()
            child = os.fork()
            if child == 0:
                repo_lock.release(b1)  # a forked child tries to release its parent's lock
                os._exit(0)
            os.waitpid(child, 0)
            expect(probe_lock("handle-test") == "held", "a forked child cannot release its parent's lock")
        repo_lock.release(b1)
        expect(probe_lock("handle-test") == "acquired", "... and the real owner's release frees it")
        holder = subprocess.Popen(["sleep", "0"])
        os.kill(holder.pid, signal.SIGTERM)
        holder.wait()

        # T8e: process inspection fails closed.
        print("process inspection:")
        fakebin = tmp / "fakebin"
        fakebin.mkdir(exist_ok=True)
        for name, body in (("lsof-fails", "#!/bin/sh\nprintf 'p%s\\nctest\\nn/\\n' \"$PPID\"\n"
                                          "echo 'lsof: permission denied' >&2\nexit 1\n"),
                           ("lsof-empty", "#!/bin/sh\nexit 0\n")):
            d = fakebin / name
            d.mkdir()
            (d / "lsof").write_text(body)
            (d / "lsof").chmod(0o755)
            old_path = os.environ["PATH"]
            os.environ["PATH"] = f"{d}{os.pathsep}{old_path}"
            try:
                processes_in(tmp, method="lsof")
                ok = False
            except Stop as s:
                ok = s.code == EXIT_REFUSED
            finally:
                os.environ["PATH"] = old_path
            expect(ok, f"an inspector that {'lists itself but exits 1' if name == 'lsof-fails' else 'lists nothing, not even itself'} "
                       "is refused, not read as 'idle'")
        try:
            processes_in(tmp)
            expect(True, "the real inspector lists this process and passes")
        except Stop:
            expect(False, "the real inspector lists this process and passes")

        # T8f: a stale copy of the tool refuses to run.
        print("tool freshness:")
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        main_commit(work, lambda w: (w / "tools/maintainer/merge_when_green.py").write_text("# a newer copy\n"),
                    "main carries a newer merge_when_green.py")
        fake = FakeGitHub(origin, env, 24, branch, "skill: bravo", green)
        rc, logs = merger(work, fake, require=(), dry_run=True, allow_stale_tools=False)
        expect(rc == EXIT_REFUSED and any("differ(s) from origin/main" in s for s in logs),
               "a copy that differs from origin/main's refuses to run")
        rc, logs = merger(work, fake, require=(), dry_run=True, allow_stale_tools=True)
        expect(rc == 0 and any("--allow-stale-tools" in s for s in logs), "... unless --allow-stale-tools, which says so")

        # T8f2: freshness compares the code LOADED at start, not the disk now:
        # a shared checkout pulled mid-run matches main on disk while this
        # process still runs the old rules.
        pulled = tmp / "pulled"
        pulled.mkdir()
        sh("git", "init", "-q", cwd=pulled)
        (pulled / "tools/maintainer").mkdir(parents=True)
        for f in TOOL_FILES:
            if _read_tool(f) is not None:
                (pulled / "tools/maintainer" / f).write_text(_read_tool(f) or "", encoding="utf-8",
                                                             errors="surrogateescape")
        sh("git", "add", "-A", cwd=pulled)
        sh("git", "commit", "-q", "-m", "main = this checkout's disk", cwd=pulled)
        sh("git", "update-ref", "refs/remotes/origin/main", "HEAD", cwd=pulled)
        verdict = []
        for loaded in (LOADED_TOOLS, {**LOADED_TOOLS, "resolve_registry.py": "# the rules loaded before the pull\n"}):
            try:
                check_tool_freshness(Git(pulled, env), "origin", False, lambda s: None, loaded)
                verdict.append("fresh")
            except Stop:
                verdict.append("stale")
        expect(verdict == ["fresh", "stale"],
               f"main equal to the disk is fresh for the code loaded from it, stale for older loaded code ({verdict})")

        # T8g: main changes the rules mid-run. The run starts fresh (its loaded
        # rules are the fixture main's own copies); main then moves during the
        # wait, and the next fetch of main checks freshness again.
        for label, change, want in (
                ("main changing resolve_registry.py during the wait stops the run at the next fetch "
                 "(no merge with the old rules)", "tools/maintainer/resolve_registry.py", EXIT_REFUSED),
                ("main moving with an unrelated commit during the wait is re-validated and merged "
                 "(freshness re-checked, still fresh)", "skills/delta/cli/main.go", 0)):
            origin, work, branch = world()
            new_connector_pr(work, extra_commit=False)
            loaded = {f: Git(work, env).show("refs/remotes/origin/main", f"tools/maintainer/{f}") for f in TOOL_FILES}

            def move_main(fk: FakeGitHub, sha: str, n: int, w=work, path=change) -> None:
                if n == 1:
                    main_commit(w, lambda x, q=path: ((x / q).parent.mkdir(parents=True, exist_ok=True),
                                                      (x / q).write_text("# newer on main\n")), f"main: {path}")
            fake = FakeGitHub(origin, env, 30, branch, "skill: bravo", green, on_snapshot=move_main)
            rc, logs = merger(work, fake, require=(), allow_stale_tools=False, tools=loaded)
            expect(rc == want and (fake.merged is None) == (want != 0)
                   and (want == 0 or any("resolve_registry.py differ(s) from origin/main" in s for s in logs)),
                   f"{label} (rc={rc})")

        # T8h: checks change on the SAME head after the wait said green (a
        # re-run). The final gate re-reads them right before the merge call;
        # the merge then compares them once more.
        print("the final gate (checks re-read immediately before the merge):")

        def rerun_at(table: dict):
            def hook(fk: FakeGitHub, sha: str, n: int) -> None:
                if n in table:
                    fk.attempts[(sha, "security-gate")] = table[n]
            return hook

        def by_attempt(table: dict) -> Callable[[str, str, int], tuple[str, str]]:
            def r(sha: str, name: str, attempt: int) -> tuple[str, str]:
                return table.get(attempt, ("success", LOG_OK)) if name == "security-gate" else ("success", LOG_OK)
            return r

        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 31, branch, "skill: bravo", by_attempt({2: ("failure", LOG_REAL)}),
                          on_snapshot=rerun_at({2: 2}))
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_CHECKS and fake.merge_calls == 0 and fake.merged is None
               and any("changed after the green verdict" in s for s in logs),
               f"a same-head re-run that turns red after the green wait is never merged (rc={rc})")
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 32, branch, "skill: bravo", by_attempt({2: ("pending", "")}),
                          on_snapshot=rerun_at({2: 2, 3: 3}))
        rc, logs = merger(work, fake, require=())
        expect(rc == 0 and fake.merge_calls == 1 and any("changed after the green verdict" in s for s in logs),
               f"one still running at the gate sends the run back to wait; green again, it merges (rc={rc})")

        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 33, branch, "skill: bravo", by_attempt({2: ("failure", LOG_REAL)}))

        def rerun_inside_merge(fk: FakeGitHub) -> None:
            fk.attempts[(fk._git("rev-parse", f"refs/heads/{fk.branch}"), "security-gate")] = 2
        fake.before_merge = rerun_inside_merge
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_RACE_RED and fake.merged is not None
               and any("changed between the final gate and the merge" in s for s in logs),
               f"checks that change inside the merge call: exit 8, the chain stops (rc={rc})")
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 34, branch, "skill: bravo", green)
        fake.snapshot_error_after_merge = True
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_RACE_RED and fake.merged is not None
               and any("no complete check listing" in s for s in logs),
               f"checks that cannot be read after the merge: exit 8, unverified, not exit 0 or 1 (rc={rc})")

        # T9: failing checks.
        print("failing checks (flake, real, unknown, stale base):")

        def scripted(table: dict) -> Callable[[str, str, int], tuple[str, str]]:
            def r(sha: str, name: str, attempt: int) -> tuple[str, str]:
                return table.get((name, attempt), ("success", LOG_OK))
            return r

        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 13, branch, "skill: bravo", scripted({("security-gate", 1): ("failure", LOG_FLAKE)}))
        rc, logs = merger(work, fake, require=())
        expect(rc == 0 and fake.reruns == ["security-gate"], f"a flake is re-run once and then merges (rc={rc})")

        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 14, branch, "skill: bravo", scripted({("security-gate", 1): ("failure", LOG_REAL)}))
        fake.base_checks[(origin_rev(origin, "refs/heads/main"), "security-gate")] = ("success", LOG_OK)
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_CHECKS and not fake.reruns and fake.merged is None
               and any("caused by this change" in s for s in logs),
               "a real failure the base does not have stops: no re-run, no merge")

        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 15, branch, "skill: bravo",
                          scripted({("guards", 1): ("failure", LOG_UNKNOWN), ("guards", 2): ("failure", LOG_UNKNOWN)}))
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_CHECKS and fake.reruns == ["guards"], "an unknown failure is re-run exactly once, then stops")

        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 16, branch, "skill: bravo",
                          scripted({("security-gate", 1): ("failure", LOG_REAL_ELSEWHERE)}))
        fake.base_checks[(origin_rev(origin, "refs/heads/main"), "security-gate")] = ("success", LOG_OK)
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_CHECKS and fake.merged is None,
               "a finding in another connector's file is never cleared by its location")

        for label, tip_go, want in (("stale base: the base fails identically and main's tip passes, so it rebases and merges",
                                     "go1.26.9", 0),
                                    ("main's tip passes only with different tool versions: not shown stale, stops",
                                     "go1.26.10", EXIT_CHECKS)):
            origin, work, branch = world()
            new_connector_pr(work, extra_commit=False)
            first_base = origin_rev(origin, "refs/heads/main")
            heads_seen: list[str] = []

            def responder(sha: str, name: str, attempt: int, hs=heads_seen) -> tuple[str, str]:
                if sha not in hs:
                    hs.append(sha)
                if name == "security-gate" and sha == hs[0]:
                    return "failure", LOG_REAL_ELSEWHERE
                return "success", LOG_OK

            def advance(fk: FakeGitHub, sha: str, n: int, w=work, go=tip_go, done=[False]) -> None:
                if not done[0]:
                    done[0] = True  # main lands a fix while the PR's checks run
                    main_commit(w, lambda x: (x / "docs/guide.md").write_text("main fixed the gate\n"), "fix main")
                    fk.base_checks[(origin_rev(fk.origin, "refs/heads/main"), "security-gate")] = ("success", log_ok(go))
            fake = FakeGitHub(origin, env, 17, branch, "skill: bravo", responder, on_snapshot=advance)
            fake.base_checks[(first_base, "security-gate")] = ("failure", LOG_REAL_ELSEWHERE)
            rc, logs = merger(work, fake, require=())
            expect(rc == want and (fake.merged is not None) == (want == 0) and not fake.reruns, f"{label} (rc={rc})")

        # T10: dry run pushes and merges nothing.
        print("dry run:")
        origin, work, branch = world()
        head = new_connector_pr(work)
        main_commit(work, lambda w: (w / "docs/guide.md").write_text("moved\n"), "main moves")
        fake = FakeGitHub(origin, env, 18, branch, "skill: bravo", green)
        rc, logs = merger(work, fake, require=(), dry_run=True)
        expect(rc == 0 and origin_rev(origin, f"refs/heads/{branch}") == head and fake.merged is None,
               "the dry run leaves the remote branch and main untouched")
        expect(any("[dry run] would push" in s for s in logs) and any("would wait for every check" in s for s in logs),
               "and prints the push and the merge it would do")
        expect(sh("git", "worktree", "list", "--porcelain", cwd=work).count("worktree ") == 1,
               "its private worktree is removed")

        # T11: a required context that was skipped is not green.
        print("required checks:")
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)
        fake = FakeGitHub(origin, env, 19, branch, "skill: bravo", scripted({("security-gate", 1): ("skipped", "")}))
        rc, logs = merger(work, fake, require=())
        expect(rc == EXIT_CHECKS and fake.merged is None, "a skipped required check is not a pass")

        # T12: the head moves while waiting.
        origin, work, branch = world()
        new_connector_pr(work, extra_commit=False)

        def push_more(fk: FakeGitHub, sha: str, n: int, w=work) -> None:
            if n == 1:
                sh("git", "fetch", "-q", "origin", cwd=w)
                sh("git", "checkout", "-q", "-B", "skill/bravo", "origin/skill/bravo", cwd=w)
                (w / "skills/bravo/SKILL.md").write_text("# bravo\nlate push\n")
                sh("git", "commit", "-q", "-s", "-am", "late", cwd=w)
                sh("git", "push", "-q", "-f", "origin", "skill/bravo", cwd=w)
                sh("git", "checkout", "-q", "main", cwd=w)
        fake = FakeGitHub(origin, env, 20, branch, "skill: bravo", green, on_snapshot=push_more)
        fake.snapshots = 0
        rc, logs = merger(work, fake, require=())
        expect(rc in (EXIT_REFUSED,) and fake.merged is None, f"a head pushed by someone else mid-wait stops it (rc={rc})")
    finally:
        for p in procs:
            if p.poll() is None:
                p.kill()
        shutil.rmtree(tmp, ignore_errors=True)

    if failures:
        print(f"\nFAIL: merge_when_green self-test - {len(failures)} case(s):")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("\nPASS: merge_when_green - every case discriminates")
    return 0


# --------------------------------------------------------------------------- #
# CLI
# --------------------------------------------------------------------------- #

def main(argv: list[str] | None = None) -> int:
    argv = sys.argv[1:] if argv is None else argv
    if "--self-test" in argv:
        return _self_test()
    sub = argv[0] if argv and argv[0] in ("approve", "status") else None
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter,
                                 prog="merge_when_green.py" + (f" {sub}" if sub else ""))
    ap.add_argument("pr", type=int)
    ap.add_argument("--repo", default=".", help="the clone to work in (default: the current checkout)")
    ap.add_argument("--remote", default="origin")
    ap.add_argument("--gh-repo", help="owner/repo on GitHub (default: read from the remote URL)")
    if sub == "approve":
        ap.add_argument("--reviewer", required=True, help="who approved, e.g. codex or a GitHub login")
        ap.add_argument("--head", required=True, help="the exact commit that was reviewed")
        ap.add_argument("--evidence", type=Path, help="the review text; must end with 'VERDICT: APPROVED', no [P1]")
        ap.add_argument("--allow-stale-tools", action="store_true",
                        help="run even though this tool's code differs from origin/main's (not recommended)")
    elif sub is None:
        ap.add_argument("--dry-run", action="store_true", help="print every step; push and merge nothing")
        ap.add_argument("--require-approval", action="append", metavar="NAME",
                        help="reviewer whose current approval is required (repeatable; default: codex)")
        ap.add_argument("--no-approval-required", action="store_true", help="merge on green checks alone")
        ap.add_argument("--expect-workflow", action="append", metavar="NAME",
                        help="workflow that must run on the head (default: CI and security-gate)")
        ap.add_argument("--timeout-min", type=float, default=60.0)
        ap.add_argument("--poll-sec", type=float, default=30.0)
        ap.add_argument("--settle-sec", type=float, default=60.0)
        ap.add_argument("--max-rebases", type=int, default=3)
        ap.add_argument("--required-context", action="append", metavar="NAME",
                        help="required check name(s); default: read main's branch protection (fails closed)")
        ap.add_argument("--allow-stale-tools", action="store_true",
                        help="run even though this tool's code differs from origin/main's (not recommended)")
    args = ap.parse_args(argv[1:] if sub else argv)
    repo = Path(subprocess.run(["git", "-C", args.repo, "rev-parse", "--show-toplevel"], capture_output=True,
                               text=True, check=True).stdout.strip())
    git = Git(repo)
    try:
        gh = GitHub(args.gh_repo or gh_slug_from_remote(git, args.remote))
        if sub == "approve":
            return record_approval(git, gh, args.pr, args.reviewer, args.head, args.evidence, args.remote,
                                   allow_stale_tools=args.allow_stale_tools)
        if sub == "status":
            return show_status(git, gh, args.pr, args.remote)
    except Stop as s:
        print(f"STOP ({s.code}): {s}")
        return s.code
    require: tuple[str, ...] = () if args.no_approval_required else tuple(args.require_approval or ("codex",))
    opts = Options(pr=args.pr, dry_run=args.dry_run, remote=args.remote, require=require,
                   expect_workflows=tuple(args.expect_workflow or ("CI", "security-gate")),
                   timeout_s=args.timeout_min * 60, poll_s=args.poll_sec, settle_s=args.settle_sec,
                   max_rebases=args.max_rebases, allow_stale_tools=args.allow_stale_tools,
                   required=tuple(args.required_context) if args.required_context else None)
    return Merger(git, gh, opts).run()


if __name__ == "__main__":
    sys.exit(main())
