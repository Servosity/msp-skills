#!/usr/bin/env python3
"""Stamp `cli_hash_at_release` for releases that have sealed, from the TAGGED tree.

A new connector merges with `"cli_hash_at_release": null` and version 0.1.0, is
tagged, and seals. Until something writes the hash of the source that release
was built from, release_state.py reports it as never-released forever. That used
to be a separate "stamp" pull request per release batch (#385, #397, #405, #411):
one more PR, one more CI round, one more merge, for a value that is fully
determined by facts already on GitHub. This script computes it, and catalog.yml
runs it inside the post-release bot commit it already makes.

What it writes, and only when every condition holds
---------------------------------------------------
For each connector whose NEWEST COMPLETE PUBLISHED release (the admitted list
catalog.yml builds with check_release_assets.py --admit-published) is
`<slug>-v<X>`, and whose registry version AND manifest.json version are both X:

  * cli_hash_at_release, when it is null: the hash of skills/<dir>/cli at that
    tag, computed exactly as cli_hash.py computes it on a checkout (same files,
    same order, same bytes). The tag hash, not main's: if main changed the CLI
    after the tag, the connector correctly reads binary-pending.
  * the CHANGELOG heading `## [X]` when it carries no date (or says
    "unreleased"): dated with the release's publish date, when --listing gives it.

What it never does
------------------
  * overwrite a stamp that is already set. A set stamp that differs from the tag
    tree is reported as a WARN line (that is the "stamp the version last" defect,
    and correcting it is a human decision);
  * stamp from a tag that is not in the admitted list (a stranded draft, a
    release missing an asset), from a tag it cannot read locally, or where a
    `.gitattributes` file, a symlink or a submodule could make the checkout bytes
    differ from the stored ones;
  * reformat skills.json: it is rewritten only if its bytes round-trip, and the
    rewrite is asserted to change nothing but the stamped fields.

Usage
    python3 tools/maintainer/stamp_release.py --released-tags released.txt [--listing releases.ndjson] [--dry-run]
    python3 tools/maintainer/stamp_release.py --self-test

--listing is the NDJSON catalog.yml fetches (one {tag_name, published_at, assets}
object per line); only published_at is read from it.

Exit 0 on success (including "nothing to stamp"); 1 when an input cannot be
read or a write would change more than intended, with nothing written.

Pure stdlib.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import cli_hash  # noqa: E402  (local tools/ module: the hash this must reproduce)
import repo_lock  # noqa: E402  (local tools/ module: writer coordination with release.py)
import resolve_registry as rr  # noqa: E402  (local tools/ module: byte-safe JSON format detection)

DEFAULT_ROOT = Path(__file__).resolve().parent.parent.parent
REGISTRY_REL = "tools/maintainer/skills.json"


class StampError(Exception):
    pass


def _git(repo: Path, *args: str) -> subprocess.CompletedProcess:
    return rr.run_bounded(["git", "-C", str(repo), *args], capture_output=True)


def tree_hash(repo: Path, rev: str, rel_dir: str) -> str | None:
    """cli_hash.compute_cli_hash of `rel_dir` as it is at `rev`, from git
    objects (no checkout). None when the checkout bytes could differ from the
    stored bytes, or the directory is absent."""
    ls = _git(repo, "ls-tree", "-r", "-z", "--full-tree", rev, "--", rel_dir + "/")
    if ls.returncode != 0:
        return None
    entries = []
    for rec in ls.stdout.split(b"\0"):
        if not rec:
            continue
        meta, path = rec.split(b"\t", 1)
        mode, kind, sha = meta.decode().split()
        if kind != "blob" or mode not in ("100644", "100755"):
            return None  # a symlink or submodule: a checkout would not read these bytes
        rel = path.decode("utf-8")[len(rel_dir) + 1:]
        entries.append((rel, sha))
    if not entries:
        return None
    # A .gitattributes on the way down could transform bytes at checkout.
    parts = rel_dir.split("/")
    for i in range(len(parts) + 1):
        attr = "/".join(parts[:i] + [".gitattributes"])
        if _git(repo, "cat-file", "-e", f"{rev}:{attr}").returncode == 0:
            return None
    if any(rel == ".gitattributes" or rel.endswith("/.gitattributes") for rel, _ in entries):
        return None
    ordered = sorted(entries, key=lambda e: e[0])
    # One bounded batch read (its own process group, killed at the cap,
    # raising rr.CallTimeout), parsed strictly: a truncated or malformed
    # answer, or a failed git, raises instead of hashing short bytes.
    p = rr.run_bounded(["git", "-C", str(repo), "cat-file", "--batch"], capture_output=True,
                       input=b"".join(sha.encode() + b"\n" for _rel, sha in ordered))
    if p.returncode != 0:
        raise StampError(f"git cat-file --batch failed ({p.returncode}): {p.stderr.decode('utf-8', 'replace')[-300:]}")
    out, pos = p.stdout, 0
    h = hashlib.sha256()
    for rel, sha in ordered:
        nl = out.find(b"\n", pos)
        header = out[pos:nl].split() if nl >= 0 else []
        if len(header) != 3 or header[0] != sha.encode() or header[1] != b"blob" or not header[2].isdigit():
            raise StampError(f"git cat-file --batch answered {out[pos:pos + 80]!r} for {sha}")
        size = int(header[2])
        data = out[nl + 1:nl + 1 + size]
        if len(data) != size or out[nl + 1 + size:nl + 2 + size] != b"\n":
            raise StampError(f"git cat-file --batch returned {len(data)} of {size} bytes for {sha}")
        pos = nl + 2 + size
        h.update(rel.encode("utf-8"))
        h.update(b"\0")
        h.update(data)
        h.update(b"\0")
    if pos != len(out):
        raise StampError("git cat-file --batch returned more than was asked for")
    return "sha256:" + h.hexdigest()


def _vtuple(v: str) -> tuple[int, ...]:
    out = []
    for chunk in v.split("."):
        m = re.match(r"\d+", chunk)
        out.append(int(m.group(0)) if m else 0)
    return tuple(out)


def newest_tag(slug: str, released: set[str]) -> tuple[str, str] | None:
    prefix = f"{slug}-v"
    tags = [t for t in released if t.startswith(prefix) and re.fullmatch(r"\d+\.\d+\.\d+", t[len(prefix):])]
    if not tags:
        return None
    best = max(tags, key=lambda t: _vtuple(t[len(prefix):]))
    return best, best[len(prefix):]


def load_released(path: Path) -> set[str]:
    try:
        tags = {ln.strip() for ln in path.read_text(encoding="utf-8").splitlines() if ln.strip()}
    except OSError as exc:
        raise StampError(f"cannot read --released-tags {path}: {exc}")
    if not tags:
        raise StampError(f"--released-tags {path} is empty; refusing to treat that as 'nothing is released'")
    return tags


def load_dates(path: Path | None) -> dict[str, str]:
    if path is None:
        return {}
    dates = {}
    for n, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        try:
            rec = json.loads(line)
        except ValueError as exc:
            raise StampError(f"--listing line {n} is not JSON: {exc}")
        tag, when = rec.get("tag_name"), rec.get("published_at")
        if isinstance(tag, str) and isinstance(when, str) and re.match(r"\d{4}-\d{2}-\d{2}", when):
            dates[tag] = when[:10]
    return dates


def _heading_re(version: str) -> re.Pattern:
    return re.compile(r"^##\s*\[" + re.escape(version) + r"\](?P<tail>[^\n]*)$", re.M)


def date_changelog(text: str, version: str, day: str) -> str | None:
    """The CHANGELOG with `## [version]` dated, or None when there is nothing
    to do (already dated, absent, ambiguous, or carrying something else)."""
    hits = list(_heading_re(version).finditer(text))
    if len(hits) != 1:
        return None
    tail = hits[0].group("tail").strip()
    if tail and not re.fullmatch(r"-?\s*\(?unreleased\)?", tail, re.I):
        return None
    m = hits[0]
    return text[:m.start()] + f"## [{version}] - {day}" + text[m.end():]


def _tag_entry(repo: Path, tag: str, slug: str) -> tuple[str, str | None, str | None] | None:
    """(source_dir, registry version, manifest version) as the TAGGED commit
    records them, or None if that commit does not register the slug as a
    releasable connector."""
    p = _git(repo, "show", f"refs/tags/{tag}:{REGISTRY_REL}")
    if p.returncode != 0:
        return None
    try:
        entry = json.loads(p.stdout.decode("utf-8")).get("skills", {}).get(slug)
    except ValueError:
        return None
    if not isinstance(entry, dict) or entry.get("markdown_only"):
        return None
    sdir = entry.get("source_dir", slug)
    m = _git(repo, "show", f"refs/tags/{tag}:skills/{sdir}/manifest.json")
    try:
        mf_version = json.loads(m.stdout.decode("utf-8")).get("version") if m.returncode == 0 else None
    except ValueError:
        mf_version = None
    return sdir, entry.get("version"), mf_version


class Plan(dict):
    """The planned changes plus the exact bytes each was computed from, so the
    write can refuse to overwrite anything that changed in between."""


def plan(repo: Path, released: set[str], dates: dict[str, str]) -> tuple[dict, dict[str, str], list[str]]:
    """(stamps {slug: hash}, changelog edits {path: (original text, new text)},
    report lines). The stamps mapping also carries `.raw`, the exact
    skills.json bytes it was computed from, which apply() compares before
    writing."""
    reg_path = repo / REGISTRY_REL
    reg_raw = reg_path.read_text(encoding="utf-8")
    reg = json.loads(reg_raw)
    skills = reg.get("skills")
    if not isinstance(skills, dict):
        raise StampError("skills.json has no skills object")
    stamps: dict[str, str] = {}
    edits: dict[str, str] = {}
    report: list[str] = []
    for slug in sorted(skills):
        entry = skills[slug]
        if not isinstance(entry, dict) or entry.get("markdown_only"):
            continue
        hit = newest_tag(slug, released)
        if hit is None:
            continue
        tag, version = hit
        sdir = entry.get("source_dir", slug)
        reg_version = entry.get("version")
        try:
            mf_version = json.loads((repo / "skills" / sdir / "manifest.json").read_text(encoding="utf-8")).get("version")
        except (OSError, ValueError):
            mf_version = None
        if reg_version != version or mf_version != version:
            if reg_version and _vtuple(str(reg_version)) < _vtuple(version):
                report.append(f"WARN {slug}: registry version {reg_version} is behind its newest release {tag}")
            continue  # a bump is pending release (or the stamps disagree): release.py owns that stamp
        if _git(repo, "rev-parse", "--verify", "-q", f"refs/tags/{tag}^{{commit}}").returncode != 0:
            report.append(f"WARN {slug}: tag {tag} is not in this clone (fetch tags); not stamped")
            continue
        # The release was built from the TAGGED tree, so where its source lives,
        # and what version it is, are read from that tree; if main has since
        # moved the connector's directory, the stamp's identity is ambiguous.
        tagged = _tag_entry(repo, tag, slug)
        if tagged is None:
            report.append(f"WARN {slug}: {tag}'s own registry has no releasable entry for {slug}; not stamped")
            continue
        tag_sdir, tag_version, tag_mf_version = tagged
        if tag_sdir != sdir:
            report.append(f"WARN {slug}: source_dir is {sdir!r} on main but {tag_sdir!r} at {tag}; not stamped")
            continue
        if tag_version != version or tag_mf_version != version:
            report.append(f"WARN {slug}: {tag} itself records version {tag_version}/{tag_mf_version}, not "
                          f"{version}; not stamped")
            continue
        th = tree_hash(repo, f"refs/tags/{tag}", f"skills/{sdir}/cli")
        if th is None:
            report.append(f"WARN {slug}: cannot reproduce the checkout hash of {tag} from git objects; not stamped")
            continue
        current = entry.get("cli_hash_at_release")
        if current is None:
            stamps[slug] = th
            report.append(f"STAMPED {slug} {tag} {th}")
        elif current != th:
            report.append(f"WARN {slug}: cli_hash_at_release {current[:19]}... differs from the {tag} tree "
                          f"{th[:19]}...; left as is (fix by hand if the stamp is wrong)")
        day = dates.get(tag)
        cl = repo / "skills" / sdir / "CHANGELOG.md"
        if day and cl.is_file():
            original = cl.read_text(encoding="utf-8")
            new = date_changelog(original, version, day)
            if new is not None:
                edits[str(cl.relative_to(repo))] = (original, new)  # type: ignore[assignment]
                report.append(f"DATED {slug} [{version}] {day}")
    stamps_plan = Plan(stamps)
    stamps_plan.raw = reg_raw  # type: ignore[attr-defined]
    return stamps_plan, edits, report


def _atomic_write(path: Path, text: str) -> None:
    """Write via a NEW, uniquely named temp file beside `path` (O_EXCL, so it
    can never be an existing file or a symlink someone left there), then
    rename it over `path`, keeping the file's mode."""
    fd, tmp = tempfile.mkstemp(prefix=f".{path.name}.", suffix=".stamp-tmp", dir=str(path.parent))
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="") as fh:
            fh.write(text)
        os.chmod(tmp, path.stat().st_mode & 0o7777)
        os.replace(tmp, path)
    except BaseException:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise


def apply(repo: Path, stamps: dict[str, str], edits: dict) -> None:
    """Write the plan, compare-and-swap: every file must still hold exactly the
    bytes the plan was computed from, or NOTHING is written (a stamp someone
    set, or CHANGELOG notes someone added, since planning are never
    overwritten). Run under repo_lock's "release" lock (see run())."""
    reg_path = repo / REGISTRY_REL
    raw = reg_path.read_text(encoding="utf-8")
    planned_raw = getattr(stamps, "raw", None)
    if stamps and planned_raw is not None and raw != planned_raw:
        raise StampError("skills.json changed after it was read; nothing written (run again)")
    for rel, (original, _new) in edits.items():
        if (repo / rel).read_text(encoding="utf-8") != original:
            raise StampError(f"{rel} changed after it was read; nothing written (run again)")
    if stamps:
        try:
            fmt = rr.json_format(raw)
        except rr.UnknownFormat as exc:
            raise StampError(f"skills.json does not round-trip ({exc}); refusing to rewrite it")
        obj = json.loads(raw)
        for slug, h in stamps.items():
            obj["skills"][slug]["cli_hash_at_release"] = h
        new = rr.dump_json(obj, fmt)
        before, after = json.loads(raw), json.loads(new)
        for slug in stamps:
            before["skills"][slug]["cli_hash_at_release"] = after["skills"][slug]["cli_hash_at_release"]
        if before != after:
            raise StampError("the rewrite would change more than the stamped fields; nothing written")
        a, b = raw.splitlines(), new.splitlines()
        touched = sum(x != y for x, y in zip(a, b))
        if len(a) != len(b) or touched != len(stamps):
            raise StampError(f"the rewrite would touch {touched} line(s) for {len(stamps)} stamp(s); nothing written")
        _atomic_write(reg_path, new)
    for rel, (_original, text) in edits.items():
        _atomic_write(repo / rel, text)


def run(repo: Path, released_file: Path, listing: Path | None, dry_run: bool) -> int:
    lock = None
    try:
        released = load_released(released_file)
        dates = load_dates(listing)
        if not dry_run:
            # The same lock release.py holds while it writes skills.json, so a
            # stamp is never planned against bytes another writer is changing.
            lock = repo_lock.acquire("release", {"tool": "stamp_release"}, root=repo)
        stamps, edits, report = plan(repo, released, dates)
        for line in report:
            print(line)
        if not stamps and not edits:
            print("stamp_release: nothing to stamp")
            return 0
        if dry_run:
            print(f"stamp_release: dry run - would stamp {len(stamps)} and date {len(edits)} heading(s)")
            return 0
        try:
            apply(repo, stamps, edits)
        except KeyboardInterrupt:
            print("::error::stamp_release: interrupted while writing: some files may already be updated; "
                  "check `git status` and restore them before running again", file=sys.stderr)
            raise
        print(f"stamp_release: stamped {len(stamps)}, dated {len(edits)} heading(s)")
        return 0
    except (StampError, OSError, ValueError, KeyError, repo_lock.LockHeld, rr.CallTimeout) as exc:
        print(f"::error::stamp_release: {exc}", file=sys.stderr)
        return 1
    finally:
        if lock is not None:
            repo_lock.release(lock)


# --------------------------------------------------------------------------- #
# --self-test
# --------------------------------------------------------------------------- #

def _self_test() -> int:
    failures: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            failures.append(label)

    tmp = Path(tempfile.mkdtemp(prefix="stamp-release-selftest-"))
    try:
        env = rr.fixture_env(tmp)
        import os
        for k in ("GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM"):
            os.environ[k] = env[k]
        work = rr.make_fixture_repo(tmp, env)

        def sh(*a: str) -> str:
            return subprocess.run(["git", "-C", str(work), *a], capture_output=True, text=True, check=True,
                                  env=env).stdout.strip()

        for slug in ("alpha", "delta"):
            (work / f"skills/{slug}/manifest.json").write_text(json.dumps({"version": "0.1.0"}, indent=2) + "\n")
            (work / f"skills/{slug}/CHANGELOG.md").write_text(
                f"# Changelog\n\n## [0.1.0]\n\n### Added\n- Initial {slug} release.\n")
        (work / "skills/alpha/cli/sub").mkdir()
        (work / "skills/alpha/cli/sub/x.go").write_bytes(b"package sub\r\n// crlf bytes kept\r\n")
        (work / "skills/alpha/cli/run.sh").write_text("#!/bin/sh\necho hi\n")
        (work / "skills/alpha/cli/run.sh").chmod(0o755)
        sh("add", "-A")
        sh("commit", "-q", "-s", "-m", "manifests")
        sh("tag", "alpha-v0.1.0")
        sh("tag", "delta-v0.1.0")
        tag_hash = cli_hash.compute_cli_hash(work / "skills/alpha/cli")
        (work / "skills/alpha/cli/main.go").write_text('package main\n\nconst name = "changed after the tag"\n')
        sh("commit", "-q", "-s", "-am", "main moves after the tag")
        released = tmp / "released.txt"
        listing = tmp / "listing.ndjson"
        released.write_text("alpha-v0.1.0\nother-v1.0.0\n")
        listing.write_text(json.dumps({"tag_name": "alpha-v0.1.0", "published_at": "2026-10-20T17:33:27Z",
                                       "assets": []}) + "\n")
        reg_before = (work / REGISTRY_REL).read_text()

        print("stamping a sealed first release:")
        expect(tree_hash(work, "refs/tags/alpha-v0.1.0", "skills/alpha/cli") == tag_hash,
               "the git-object hash equals cli_hash.py on a checkout (CRLF bytes and an executable included)")
        # The batch read is bounded and strict: a truncated answer raises,
        # and a git that stalls after answering is killed at the cap.
        import shutil as _sh
        real_git = _sh.which("git") or "git"
        stubdir = tmp / "gitstub"
        stubdir.mkdir()
        (stubdir / "git").write_text(
            "#!/bin/sh\n"
            'case " $* " in *" cat-file --batch "*)\n'
            '  case "$STUB_MODE" in\n'
            '    truncate) read sha; printf "%s blob 10\\nx" "$sha"; exit 0 ;;\n'
            f'    stall) "{real_git}" "$@"; sleep 30 & echo $! >"$STUB_PID"; wait $! ; exit 0 ;;\n'
            "  esac ;;\n"
            "esac\n"
            f'exec "{real_git}" "$@"\n')
        (stubdir / "git").chmod(0o755)
        old_path, old_cap = os.environ["PATH"], rr.CALL_CAP_S
        outcomes = {}
        try:
            os.environ["PATH"] = f"{stubdir}{os.pathsep}{old_path}"
            rr.CALL_CAP_S = 1.0
            for mode in ("truncate", "stall"):
                os.environ["STUB_MODE"] = mode
                t0 = time.monotonic()
                try:
                    outcomes[mode] = ("hash", tree_hash(work, "refs/tags/alpha-v0.1.0", "skills/alpha/cli"))
                except Exception as exc:  # any other exception is a wrong outcome, named, not a crash
                    outcomes[mode] = (type(exc).__name__, round(time.monotonic() - t0, 1))
        finally:
            os.environ["PATH"] = old_path
            os.environ.pop("STUB_MODE", None)
            rr.CALL_CAP_S = old_cap
        expect(outcomes.get("truncate", ("?",))[0] == "StampError",
               f"a truncated batch answer raises, never hashes short bytes ({outcomes.get('truncate')})")
        expect(outcomes.get("stall", ("?",))[0] == "CallTimeout" and outcomes["stall"][1] < 10,
               f"a git that stalls after answering is killed at the cap ({outcomes.get('stall')})")
        # Cancellation: SIGTERM to a real stamp run while its git read is
        # blocked kills that git's whole group, frees the release lock, writes
        # nothing, and exits 130.
        stub_pid = tmp / "stub.pid"
        reg_now = (work / REGISTRY_REL).read_text()
        proc = subprocess.Popen([sys.executable, str(Path(__file__).resolve()), "--repo", str(work),
                                 "--released-tags", str(released), "--listing", str(listing)],
                                env={**os.environ, "PATH": f"{stubdir}{os.pathsep}{old_path}", "STUB_MODE": "stall",
                                     "STUB_PID": str(stub_pid)}, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        for _ in range(400):
            if stub_pid.exists():
                break
            time.sleep(0.05)
        time.sleep(0.3)
        proc.terminate()
        try:
            _o, err = proc.communicate(timeout=20)
            prc = proc.returncode
        except subprocess.TimeoutExpired:
            proc.kill()
            _o, err = proc.communicate()
            prc = "hung"
        try:
            os.kill(int(stub_pid.read_text()), 0)
            stub_alive = True
        except (ProcessLookupError, ValueError, FileNotFoundError):
            stub_alive = False
        lock_file = Path(subprocess.run(["git", "-C", str(work), "rev-parse", "--git-common-dir"], capture_output=True,
                                        text=True).stdout.strip())
        lock_file = (lock_file if lock_file.is_absolute() else work / lock_file) / "msp-skills-locks" / "release.lock"
        expect(prc == 130 and not stub_alive and not lock_file.exists()
               and (work / REGISTRY_REL).read_text() == reg_now and "interrupted" in err,
               f"SIGTERM while a stamp's git read is blocked: git killed, lock freed, nothing written, exit 130 "
               f"(rc={prc}, git alive={stub_alive}, lock left={lock_file.exists()})")
        expect(run(work, released, listing, dry_run=True) == 0 and (work / REGISTRY_REL).read_text() == reg_before,
               "a dry run writes nothing")
        expect(run(work, released, listing, dry_run=False) == 0, "the stamp runs")
        reg = json.loads((work / REGISTRY_REL).read_text())["skills"]
        expect(reg["alpha"]["cli_hash_at_release"] == tag_hash, "alpha is stamped with the TAG tree's hash")
        expect(reg["alpha"]["cli_hash_at_release"] != cli_hash.compute_cli_hash(work / "skills/alpha/cli"),
               "so a CLI change after the tag still reads binary-pending")
        expect(reg["delta"]["cli_hash_at_release"] is None,
               "delta, whose tag is not in the admitted list (e.g. a stranded draft), is not stamped")
        diff = sh("diff", "--numstat", "--", REGISTRY_REL)
        expect(diff.split()[:2] == ["1", "1"], f"skills.json changes by exactly one line ({diff})")
        expect("## [0.1.0] - 2026-10-20" in (work / "skills/alpha/CHANGELOG.md").read_text(),
               "the CHANGELOG heading is dated with the publish date")
        sh("add", "-A")
        sh("commit", "-q", "-s", "-m", "stamp")
        expect(run(work, released, listing, dry_run=False) == 0 and sh("status", "--porcelain") == "",
               "a second run changes nothing (idempotent, safe to repeat on every bot run)")

        print("refusals:")
        reg_all = json.loads((work / REGISTRY_REL).read_text())
        reg_all["skills"]["alpha"]["cli_hash_at_release"] = "sha256:" + "0" * 64
        reg_all["skills"]["delta"]["version"] = "0.2.0"
        (work / REGISTRY_REL).write_text(json.dumps(reg_all, indent=2, ensure_ascii=True) + "\n")
        released.write_text("alpha-v0.1.0\ndelta-v0.1.0\n")
        before = (work / REGISTRY_REL).read_text()
        out = subprocess.run([sys.executable, __file__, "--repo", str(work), "--released-tags", str(released)],
                             capture_output=True, text=True, env=env)
        expect(out.returncode == 0 and (work / REGISTRY_REL).read_text() == before
               and "WARN alpha" in out.stdout and "STAMPED" not in out.stdout,
               "an existing stamp that differs from the tag is reported, never overwritten")
        expect("STAMPED delta" not in out.stdout
               and json.loads((work / REGISTRY_REL).read_text())["skills"]["delta"]["cli_hash_at_release"] is None,
               "a registry version ahead of the newest release is left to release.py")
        reg_all["skills"]["alpha"]["cli_hash_at_release"] = None
        bad = json.dumps(reg_all, indent=2, ensure_ascii=True).replace('  "owner"', '   "owner"', 1) + "\n"
        (work / REGISTRY_REL).write_text(bad)
        expect(run(work, released, None, dry_run=False) == 1 and (work / REGISTRY_REL).read_text() == bad,
               "a skills.json whose bytes do not round-trip is refused, not reformatted")
        empty = tmp / "empty.txt"
        empty.write_text("")
        expect(run(work, empty, None, dry_run=False) == 1, "an empty released list is an error, not 'nothing released'")
        expect(date_changelog("## [0.1.0] - 2026-01-01\n", "0.1.0", "2026-10-20") is None
               and date_changelog("## [0.1.0] - unreleased\n", "0.1.0", "2026-10-20") == "## [0.1.0] - 2026-10-20\n"
               and date_changelog("## [0.1.0] (beta)\n", "0.1.0", "2026-10-20") is None,
               "only an undated or 'unreleased' heading is dated")
        sh("checkout", "-q", "--", ".")
        reg_all = json.loads((work / REGISTRY_REL).read_text())
        reg_all["skills"]["alpha"]["cli_hash_at_release"] = None
        reg_all["skills"]["alpha"]["source_dir"] = "delta"  # main repoints alpha after the tag
        (work / REGISTRY_REL).write_text(json.dumps(reg_all, indent=2, ensure_ascii=True) + "\n")
        released.write_text("alpha-v0.1.0\n")
        out = subprocess.run([sys.executable, __file__, "--repo", str(work), "--released-tags", str(released)],
                             capture_output=True, text=True, env=env)
        expect("STAMPED" not in out.stdout and "source_dir is 'delta' on main but 'alpha'" in out.stdout,
               "a connector whose source_dir changed after its tag is not stamped from another directory")
        reg_all["skills"]["alpha"].pop("source_dir")
        (work / REGISTRY_REL).write_text(json.dumps(reg_all, indent=2, ensure_ascii=True) + "\n")
        out = subprocess.run([sys.executable, __file__, "--repo", str(work), "--released-tags", str(released),
                              "--dry-run"], capture_output=True, text=True, env=env)
        expect("STAMPED alpha" in out.stdout, "... and with the directory unchanged it is stamped")
        sh("checkout", "-q", "--", ".")
        print("writes are compare-and-swap, under release.py's lock:")
        reg_all = json.loads((work / REGISTRY_REL).read_text())
        reg_all["skills"]["alpha"]["cli_hash_at_release"] = None
        (work / REGISTRY_REL).write_text(json.dumps(reg_all, indent=2, ensure_ascii=True) + "\n")
        cl = work / "skills/alpha/CHANGELOG.md"
        cl.write_text(cl.read_text().replace("## [0.1.0] - 2026-10-20", "## [0.1.0]"))
        dates = load_dates(listing)
        p_stamps, p_edits, _r = plan(work, {"alpha-v0.1.0"}, dates)
        hand = json.loads((work / REGISTRY_REL).read_text())
        hand["skills"]["alpha"]["cli_hash_at_release"] = "sha256:" + "1" * 64  # someone fixes it by hand meanwhile
        hand_text = json.dumps(hand, indent=2, ensure_ascii=True) + "\n"
        (work / REGISTRY_REL).write_text(hand_text)
        try:
            apply(work, p_stamps, p_edits)
            ok = False
        except StampError:
            ok = True
        expect(ok and (work / REGISTRY_REL).read_text() == hand_text,
               "a stamp set by hand after planning is never overwritten")
        (work / REGISTRY_REL).write_text(json.dumps(reg_all, indent=2, ensure_ascii=True) + "\n")
        p_stamps, p_edits, _r = plan(work, {"alpha-v0.1.0"}, dates)
        notes = cl.read_text() + "\n### Fixed\n- Added while the stamp was planned.\n"
        cl.write_text(notes)
        try:
            apply(work, p_stamps, p_edits)
            ok = False
        except StampError:
            ok = True
        expect(ok and cl.read_text() == notes and "null" in (work / REGISTRY_REL).read_text(),
               "CHANGELOG notes added after planning are kept, and nothing at all is written")
        holder = subprocess.Popen([sys.executable, "-c", "import sys, time; sys.path.insert(0, sys.argv[1]); import repo_lock;"
                                   "repo_lock.acquire('release', root=sys.argv[2]); print('x', flush=True); time.sleep(60)",
                                   str(Path(__file__).resolve().parent), str(work)], stdout=subprocess.PIPE, text=True, env=env)
        holder.stdout.readline()
        rc_locked = run(work, released, listing, dry_run=False)
        holder.kill()
        holder.wait()
        expect(rc_locked == 1 and "null" in (work / REGISTRY_REL).read_text(),
               "while release.py holds its lock, the stamp writes nothing")
        bystander = work / "tools/maintainer/skills.json.stamp-tmp"
        bystander.write_text("someone's scratch file\n")
        target = tmp / "precious.txt"
        target.write_text("precious\n")
        link = work / "skills/alpha/CHANGELOG.md.stamp-tmp"
        link.symlink_to(target)
        before_mode = (work / REGISTRY_REL).stat().st_mode
        rc_ok = run(work, released, listing, dry_run=False)
        expect(rc_ok == 0 and bystander.read_text() == "someone's scratch file\n"
               and target.read_text() == "precious\n" and (work / REGISTRY_REL).stat().st_mode == before_mode
               and not list(work.glob("**/.*.stamp-tmp")),
               "the atomic write never touches an existing '.stamp-tmp' path or a symlink's target, keeps the "
               "mode, and leaves no temp file")
        bystander.unlink()
        link.unlink()
        sh("checkout", "-q", "--", ".")
        (work / ".gitattributes").write_text("*.go text eol=crlf\n")
        sh("add", ".gitattributes")
        sh("commit", "-q", "-s", "-m", "attrs")
        sh("tag", "-f", "alpha-v0.1.0")
        expect(tree_hash(work, "refs/tags/alpha-v0.1.0", "skills/alpha/cli") is None,
               "a .gitattributes that could rewrite checkout bytes makes it decline to stamp")
    finally:
        shutil.rmtree(tmp, ignore_errors=True)

    if failures:
        print(f"\nFAIL: stamp_release self-test - {len(failures)} case(s):")
        for f in failures:
            print(f"  - {f}")
        return 1
    print("\nPASS: stamp_release - every case discriminates")
    return 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--released-tags", type=Path, help="admitted complete published tags, one per line")
    ap.add_argument("--listing", type=Path, help="the release listing NDJSON (for publish dates)")
    ap.add_argument("--repo", type=Path, default=DEFAULT_ROOT, help="checkout to stamp (default: this one)")
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--self-test", action="store_true")
    args = ap.parse_args(argv)
    if args.self_test:
        return _self_test()
    if not args.released_tags:
        ap.error("--released-tags is required")
    rr.install_interrupt_handlers()  # SIGTERM/SIGHUP raise like Ctrl-C: the blocked git is killed, the lock freed
    try:
        return run(args.repo.resolve(), args.released_tags, args.listing, args.dry_run)
    except KeyboardInterrupt as exc:  # before any write, unless run() already said otherwise above
        print(f"::error::stamp_release: interrupted ({exc or 'SIGINT'})", file=sys.stderr)
        return 130


if __name__ == "__main__":
    sys.exit(main())
