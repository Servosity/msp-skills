#!/usr/bin/env python3
"""Refuse to endorse a release tag at a commit whose release.yml still seals first.

Why this gate exists
--------------------
A tag push runs `.github/workflows/release.yml` **as it exists at the tagged
commit**, not as it exists on the default branch. That single fact makes a
printed `git tag <tag> <sha>` command dangerous in a way no amount of prose in
the operator's terminal can fix: paste a tag command that names a commit from
before the draft-then-seal pipeline landed and GitHub runs the OLD publish-first
workflow. Under the repository's immutable releases that means

  * the release is published before any asset exists, so it is sealed empty;
  * all six build jobs then fail with
    `HTTP 422: Cannot upload assets to an immutable release`;
  * the release object is unrepairable, and the version number is spent.

Twenty staged tags pasted at a pre-fix SHA is twenty sealed empty releases. So
the check has to be MECHANICAL and it has to run at the moment the tag command
is produced. `tools/maintainer/release_batch.sh` calls this before it stamps
anything and again before it prints a single tag line, and refuses to print tag
commands for a SHA this script rejects.

Why a content probe and not `git merge-base --is-ancestor <fix-sha> <target>`
-----------------------------------------------------------------------------
Ancestry against a hardcoded "the commit that fixed it" SHA is the obvious
mechanical check, and it was rejected for three reasons:

  1. It measures a proxy, not the property. Ancestry answers "did this history
     pass through that commit", not "does THIS commit's release.yml assemble
     into a draft and seal last". A later commit that REVERTS the pipeline is
     still a descendant, so ancestry would endorse it. This script reads the
     very bytes GitHub will execute at that SHA.
  2. A pinned SHA does not survive the merge. A squash merge (or a rebase, or a
     re-cut branch) gives main a commit that is not a descendant of the branch
     SHA anyone would have hardcoded, so the pin would false-RED every release
     until somebody edited it, and the natural fix under time pressure is to
     delete the check.
  3. A marker file rots silently. A `.release-pipeline-v2` marker copied forward
     while the workflow regresses is a green light over a broken pipeline.

How this probe cannot silently rot
----------------------------------
Two directions, both mechanical:

  * It cannot pass a regressed pipeline, because the requirements below are read
    out of the workflow text at the SHA being tagged. There is no cached verdict,
    no marker, and no second source to drift from.
  * It cannot quietly stop matching the pipeline, because `--self-test` asserts
    the repository's OWN `.github/workflows/release.yml` satisfies every
    requirement, and ci.yml runs that self-test on every pull request. Rename the
    `publish` job, drop the completeness gate, restructure the graph: CI goes red
    on the same PR that did it, and the probe and the pipeline get updated
    together or not at all. The requirements are matched structurally (the job
    that seals, the jobs it transitively needs) rather than by job NAME, so
    ordinary refactoring does not trip it.

What "contains the draft-then-seal pipeline" means, mechanically
----------------------------------------------------------------
Five requirements, all read from the workflow at the target commit:

  R1 CREATE-IS-DRAFT   every `gh release create` carries `--draft`, and there is
                       at least one. This is the requirement the old
                       publish-first workflow fails.
  R2 ONE-SEAL          exactly one job seals the release
                       (`gh release edit ... --draft=false`).
  R3 SEAL-NEEDS-UPLOADS
                       every job that runs `gh release upload` is a transitive
                       `needs:` ancestor of the sealing job, so no asset can
                       still be in flight when the release goes immutable.
  R4 GATE-BEFORE-SEAL  the sealing job runs check_release_assets.py --with-mcpb,
                       and runs it BEFORE the line that seals.
  R5 BUNDLE-IN-CONTRACT
                       one of those uploaders attaches the MCPB bundle, matched
                       on the registry-owned MCPB_ASSET variable or the .mcpb
                       extension, so the bundle is inside the sealing contract
                       rather than chasing a release that is already immutable.

Usage
-----
    python3 tools/maintainer/check_release_pipeline.py --sha <rev> [--repo <path>]
    python3 tools/maintainer/check_release_pipeline.py --file <path>
    python3 tools/maintainer/check_release_pipeline.py --self-test

Exit 0 when the commit is safe to tag, 1 when it is not (with the failing
requirement named and the consequence spelled out). Reads no network.
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

WORKFLOW = ".github/workflows/release.yml"

# The literal command fragments the requirements are expressed in. Each is text
# GitHub itself will run, so there is no second definition to drift from.
CREATE = "gh release create"
UPLOAD = "gh release upload"
SEAL = "--draft=false"
SEAL_CMD = "gh release edit"
GATE = "check_release_assets.py"
GATE_FLAG = "--with-mcpb"
# Either the registry-owned variable that carries the bundle filename, or the
# extension itself. Matching only the variable name would refuse a pipeline that
# is correct but spells the name differently - which is exactly what the first
# commit of this very branch did (it uploaded "mcpb-build/${slug}-mcp.mcpb"),
# and refusing that would have been a false-RED.
BUNDLE_MARKERS = ("MCPB_ASSET", ".mcpb")


class ProbeError(Exception):
    """The workflow could not be read at all - refuse, never assume."""


# --------------------------------------------------------------------------
# A dependency-free reader for the one thing this probe needs out of the YAML:
# the top-level `jobs:` mapping, each job's `needs:`, and each job's executable
# text with comment-only lines removed. PyYAML is deliberately not imported -
# this script has to run on a maintainer's bare system python3 during a release,
# and a missing third-party module at that moment is a worse failure than the
# one it would prevent.
# --------------------------------------------------------------------------
def _code_lines(block: str) -> list[str]:
    """The block's lines with comment-only lines dropped.

    Comments in release.yml quote the very commands this probe matches on
    ("Cannot upload assets to an immutable release", and so on), so matching raw
    text would let a comment satisfy a requirement. Only lines GitHub actually
    executes count.
    """
    return [ln for ln in block.splitlines()
            if ln.strip() and not ln.lstrip().startswith("#")]


def workflow_jobs(text: str) -> dict[str, str]:
    """{job id: job body} for the top-level `jobs:` mapping."""
    lines = text.splitlines()
    start = None
    for i, ln in enumerate(lines):
        if re.match(r"^jobs:\s*(#.*)?$", ln):
            start = i + 1
            break
    if start is None:
        return {}
    jobs: dict[str, str] = {}
    current: str | None = None
    buf: list[str] = []
    for ln in lines[start:]:
        if not ln.strip():
            if current is not None:
                buf.append(ln)
            continue
        indent = len(ln) - len(ln.lstrip())
        if indent == 0:
            break  # a new top-level key ends the jobs mapping
        m = re.match(r"^  ([A-Za-z0-9_-]+):\s*(#.*)?$", ln)
        if m and indent == 2:
            if current is not None:
                jobs[current] = "\n".join(buf)
            current, buf = m.group(1), []
            continue
        if current is not None:
            buf.append(ln)
    if current is not None:
        jobs[current] = "\n".join(buf)
    return jobs


def job_needs(body: str) -> set[str]:
    """The `needs:` of one job, in either the inline-list or scalar form."""
    out: set[str] = set()
    for ln in _code_lines(body):
        m = re.match(r"^\s{4}needs:\s*(.+?)\s*$", ln)
        if not m:
            continue
        value = m.group(1)
        if value.startswith("["):
            value = value.strip("[]")
            out.update(v.strip().strip("'\"") for v in value.split(",") if v.strip())
        else:
            out.add(value.strip().strip("'\""))
    return {n for n in out if n}


def transitive_needs(job: str, jobs: dict[str, str]) -> set[str]:
    """Every job that must finish before `job` starts."""
    seen: set[str] = set()
    stack = list(job_needs(jobs.get(job, "")))
    while stack:
        nxt = stack.pop()
        if nxt in seen or nxt not in jobs:
            continue
        seen.add(nxt)
        stack.extend(job_needs(jobs[nxt]))
    return seen


# --------------------------------------------------------------------------
# The requirements.
# --------------------------------------------------------------------------
def evaluate(text: str) -> list[tuple[str, str]]:
    """[(requirement id, why it failed)] - empty means safe to tag."""
    jobs = workflow_jobs(text)
    failures: list[tuple[str, str]] = []
    if not jobs:
        return [("PARSE", f"no top-level `jobs:` mapping found in {WORKFLOW}")]

    code = {name: _code_lines(body) for name, body in jobs.items()}

    # R1 - every release creation is a DRAFT creation.
    creates = [ln for lns in code.values() for ln in lns if CREATE in ln]
    if not creates:
        failures.append(("CREATE-IS-DRAFT", f"no `{CREATE}` anywhere in {WORKFLOW}"))
    else:
        undrafted = [ln.strip() for ln in creates if "--draft" not in ln]
        if undrafted:
            failures.append((
                "CREATE-IS-DRAFT",
                f"`{CREATE}` without --draft: {undrafted[0]!r}. This publishes the "
                "release before any asset is uploaded, and an immutable release "
                "rejects every upload after that (HTTP 422)",
            ))

    # R2 - exactly one job seals.
    sealers = [n for n, lns in code.items()
               if any(SEAL in ln and SEAL_CMD in ln for ln in lns)]
    if len(sealers) != 1:
        failures.append((
            "ONE-SEAL",
            f"expected exactly one job running `{SEAL_CMD} ... {SEAL}`, found "
            f"{len(sealers)}: {sorted(sealers) or 'none'}. Without a separate "
            "sealing job the release is public from the moment it is created",
        ))
        # R3/R4/R5 are all statements ABOUT the sealing job.
        return failures
    seal_job = sealers[0]

    # R3 - nothing uploads outside the sealing job's dependency closure.
    uploaders = {n for n, lns in code.items() if any(UPLOAD in ln for ln in lns)}
    closure = transitive_needs(seal_job, jobs) | {seal_job}
    stragglers = sorted(uploaders - closure)
    if not uploaders:
        failures.append(("SEAL-NEEDS-UPLOADS", f"no job runs `{UPLOAD}`"))
    elif stragglers:
        failures.append((
            "SEAL-NEEDS-UPLOADS",
            f"job(s) {stragglers} upload assets but are not in the `needs:` "
            f"closure of the sealing job {seal_job!r}, so the release can be "
            "sealed while their uploads are still in flight",
        ))

    # R4 - the completeness gate runs, in the sealing job, BEFORE the seal.
    seal_lines = code[seal_job]
    gate_at = next((i for i, ln in enumerate(seal_lines)
                    if GATE in ln and GATE_FLAG in ln), None)
    seal_at = next((i for i, ln in enumerate(seal_lines)
                    if SEAL in ln and SEAL_CMD in ln), None)
    if gate_at is None:
        failures.append((
            "GATE-BEFORE-SEAL",
            f"the sealing job {seal_job!r} never runs `{GATE} {GATE_FLAG}`, so it "
            "would seal whatever happens to be attached",
        ))
    elif seal_at is not None and gate_at > seal_at:
        failures.append((
            "GATE-BEFORE-SEAL",
            f"the sealing job {seal_job!r} runs `{GATE}` AFTER it seals; the "
            "verdict arrives too late to stop anything",
        ))

    # R5 - the MCPB bundle is attached before the seal, not chasing it after.
    bundlers = {n for n, lns in code.items()
                if any(UPLOAD in ln and any(b in ln for b in BUNDLE_MARKERS)
                       for ln in lns)}
    if not bundlers & closure:
        failures.append((
            "BUNDLE-IN-CONTRACT",
            "no job in the sealing job's `needs:` closure uploads the MCPB "
            f"bundle (`{UPLOAD}` naming one of {list(BUNDLE_MARKERS)}); the "
            "bundle would have to be attached after the release is immutable, "
            "which cannot succeed",
        ))

    return failures


# --------------------------------------------------------------------------
# Sources.
# --------------------------------------------------------------------------
def workflow_at(rev: str, repo: Path) -> str:
    """release.yml as it exists at `rev` - the copy GitHub would execute."""
    try:
        proc = subprocess.run(
            ["git", "-C", str(repo), "show", f"{rev}:{WORKFLOW}"],
            capture_output=True, text=True, check=False,
        )
    except OSError as exc:  # pragma: no cover - a missing git is not a scenario here
        raise ProbeError(f"could not run git: {exc}") from exc
    if proc.returncode != 0:
        raise ProbeError(
            f"could not read {WORKFLOW} at {rev!r} "
            f"({proc.stderr.strip() or 'unknown git error'}). "
            "A commit whose release workflow cannot be read must never be tagged."
        )
    return proc.stdout


# --------------------------------------------------------------------------
# Self-test: prove the probe fires on each broken shape AND stays silent on the
# repository's own release.yml. The silent direction is what pins the probe to
# the pipeline: ci.yml runs this on every pull request, so a pipeline change
# that outgrows these requirements goes red in the PR that made it.
# --------------------------------------------------------------------------
_GOOD_SKELETON = """
jobs:
  prepare:
    runs-on: ubuntu-latest
  create-release:
    needs: [prepare]
    steps:
      - run: gh release create "$TAG" --draft --notes-file notes.md
  build:
    needs: [prepare, create-release]
    steps:
      - run: gh release upload "$TAG" "$cli" --clobber
  mcpb:
    needs: [prepare, create-release, build]
    steps:
      - run: gh release upload "$TAG" "mcpb-build/${MCPB_ASSET}" --clobber
  publish:
    needs: [prepare, create-release, build, mcpb]
    steps:
      - run: |
          gh release view "$TAG" --json assets | python3 tools/maintainer/check_release_assets.py --tag "$TAG" --with-mcpb
          gh release edit "$TAG" --draft=false
"""

_GATE_LINE = ('          gh release view "$TAG" --json assets | python3 '
              'tools/maintainer/check_release_assets.py --tag "$TAG" --with-mcpb\n')
_SEAL_LINE = '          gh release edit "$TAG" --draft=false\n'


def _mutate(skeleton: str, old: str, new: str) -> str:
    assert old in skeleton, old
    return skeleton.replace(old, new)


def _self_test() -> int:
    repo_workflow = Path(__file__).resolve().parents[2] / WORKFLOW
    failures: list[str] = []

    def case(label: str, text: str, want: set[str]) -> None:
        got = {rid for rid, _why in evaluate(text)}
        ok = got == want
        print(f"  [{'ok' if ok else 'FAIL'}] {label}")
        if not ok:
            failures.append(f"{label}: wanted {sorted(want)}, got {sorted(got)}")

    # SILENT on the repository's own pipeline. This is the anti-rot pin.
    if not repo_workflow.is_file():
        failures.append(f"{WORKFLOW} not found at {repo_workflow}")
    else:
        case("this repository's release.yml is safe to tag",
             repo_workflow.read_text(encoding="utf-8"), set())

    # SILENT on the minimal correct shape, so the requirements describe a
    # pipeline rather than this one file.
    case("a minimal draft-then-seal pipeline passes", _GOOD_SKELETON, set())

    # FIRES on the exact shape that burned xero-v0.1.3: create publishes first,
    # and nothing seals afterwards because nothing has to.
    old_shape = """
jobs:
  prepare:
    runs-on: ubuntu-latest
  create-release:
    needs: [prepare]
    steps:
      - run: gh release create "$TAG" --title "$TAG" --notes-file notes.md
  build:
    needs: [prepare, create-release]
    steps:
      - run: gh release upload "$TAG" "$cli" --clobber
"""
    case("the pre-fix publish-first workflow is refused",
         old_shape, {"CREATE-IS-DRAFT", "ONE-SEAL"})

    # FIRES when the draft is created but nothing ever seals it: every release
    # would strand as an invisible draft.
    case("a draft that is never sealed is refused",
         _mutate(_GOOD_SKELETON, _SEAL_LINE, ""), {"ONE-SEAL"})

    # FIRES when two jobs seal: whichever wins can seal a half-assembled set.
    case("two sealing jobs are refused",
         _GOOD_SKELETON + "  publish2:\n    needs: [build]\n    steps:\n"
         '      - run: gh release edit "$TAG" --draft=false\n',
         {"ONE-SEAL"})

    # FIRES when an uploader sits outside the sealing job's needs closure: the
    # seal could land while that job is still uploading. The bundle uploader is
    # the one moved out, so BUNDLE-IN-CONTRACT fires with it.
    detached = _mutate(_GOOD_SKELETON,
                       "  mcpb:\n    needs: [prepare, create-release, build]",
                       "  mcpb:\n    needs: [prepare]")
    detached = _mutate(detached,
                       "    needs: [prepare, create-release, build, mcpb]",
                       "    needs: [prepare, create-release, build]")
    case("an uploader outside the seal's needs closure is refused",
         detached, {"SEAL-NEEDS-UPLOADS", "BUNDLE-IN-CONTRACT"})

    # FIRES when the completeness gate is gone: the seal would publish whatever
    # happens to be attached.
    case("sealing without the completeness gate is refused",
         _mutate(_GOOD_SKELETON, _GATE_LINE, ""), {"GATE-BEFORE-SEAL"})

    # FIRES when the gate runs but AFTER the seal. A verdict that arrives too
    # late is not a gate.
    case("running the gate after the seal is refused",
         _mutate(_GOOD_SKELETON, _GATE_LINE + _SEAL_LINE, _SEAL_LINE + _GATE_LINE),
         {"GATE-BEFORE-SEAL"})

    # FIRES when the bundle is not attached before the seal: the mcp-publish
    # workflow would then have to upload it to an immutable release.
    case("a pipeline that never attaches the bundle is refused",
         _mutate(_GOOD_SKELETON,
                 '      - run: gh release upload "$TAG" "mcpb-build/${MCPB_ASSET}" --clobber',
                 '      - run: echo "bundle handled elsewhere"'),
         {"BUNDLE-IN-CONTRACT"})

    # FIRES when a comment merely QUOTES the commands. Comments in release.yml
    # quote every one of these strings; a probe that counted them would pass a
    # workflow that does none of it.
    commented = """
jobs:
  create-release:
    steps:
      # gh release create "$TAG" --draft
      # gh release edit "$TAG" --draft=false
      # check_release_assets.py --with-mcpb
      - run: echo nothing
"""
    case("comments quoting the commands satisfy nothing",
         commented, {"CREATE-IS-DRAFT", "ONE-SEAL"})

    # FIRES on a file that is not a workflow at all: refuse, never assume.
    case("a file with no jobs mapping is refused",
         "name: Release\non: push\n", {"PARSE"})

    if failures:
        print("check_release_pipeline self-test FAILED:", file=sys.stderr)
        for f in failures:
            print(f"  - {f}", file=sys.stderr)
        return 1
    print("check_release_pipeline self-test passed (the repo's own release.yml is "
          "endorsable; nine broken shapes are refused).")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    src = ap.add_mutually_exclusive_group()
    src.add_argument("--sha", help="commit-ish to probe, e.g. the SHA a tag would name")
    src.add_argument("--file", help="probe a workflow file on disk instead of a commit")
    ap.add_argument("--repo", default=".", help="repository to read --sha from")
    ap.add_argument("--self-test", action="store_true",
                    help="prove this gate fires and stays silent; reads no git history")
    args = ap.parse_args()

    if args.self_test:
        return _self_test()
    if not args.sha and not args.file:
        ap.error("one of --sha, --file or --self-test is required")

    try:
        if args.file:
            where = args.file
            text = Path(args.file).read_text(encoding="utf-8")
        else:
            where = f"{args.sha} ({WORKFLOW})"
            text = workflow_at(args.sha, Path(args.repo))
    except (ProbeError, OSError) as exc:
        print(f"check_release_pipeline: REFUSE - {exc}", file=sys.stderr)
        return 1

    failures = evaluate(text)
    if failures:
        print(f"check_release_pipeline: REFUSE {where}", file=sys.stderr)
        print("  This commit's release workflow does NOT assemble into a draft and",
              file=sys.stderr)
        print("  seal last. A tag push runs THAT workflow, not the one on main:",
              file=sys.stderr)
        for rid, why in failures:
            print(f"  - [{rid}] {why}", file=sys.stderr)
        print("  Under immutable releases the result is a permanently sealed, "
              "incomplete", file=sys.stderr)
        print("  release and a spent version number. Tag a commit that carries the "
              "draft-then-seal", file=sys.stderr)
        print("  pipeline instead (re-run tools/maintainer/release_batch.sh against "
              "a main that has it).", file=sys.stderr)
        return 1

    print(f"check_release_pipeline: OK {where} assembles into a draft, gates the "
          "asset set, and seals last; safe to tag.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
