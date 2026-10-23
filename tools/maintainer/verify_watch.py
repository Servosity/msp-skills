#!/usr/bin/env python3
"""verify_watch.py - say when the verify-msp-skills map is stale, and when the press moved.

Run weekly (and on demand) by .github/workflows/verify-watch.yml. Two outputs,
each ONE GitHub issue that is opened, updated in place, or closed:

  verify-map-stale  the feature map in .claude/skills/verify-msp-skills may be
                    behind the fleet. Reasons:
                    (a) the newest printing_press_version in skills/*/manifest.json
                        is above maintained_at_press in verify_maintained.json;
                    (b) a verify ERROR in the newest recorded result on main for
                        any connector (the verify-<slug> CI artifacts written by
                        tools/maintainer/verify_gate.py), or no recorded results
                        at all in the window, so (b) and (c) cannot be checked;
                    (c) a feature SKIPs on a connector whose press version should
                        have it (install and mcp-stdio always; every feature from
                        press 4.34.0 on);
                    (d) maintained_at is 30 or more days old.
  press-watch       mvanhorn/cli-printing-press has a release newer than the
                    fleet's newest printing_press_version. Input for the reprint
                    decision; nothing here ever reprints.

    python3 tools/maintainer/verify_watch.py --repo Servosity/msp-skills --collect --apply
    python3 tools/maintainer/verify_watch.py --repo Servosity/msp-skills --collect        # dry run
    python3 tools/maintainer/verify_watch.py --results DIR --press-release rel.json \\
        --today 2026-10-23 --issues-fixture issues.json                                   # offline
    python3 tools/maintainer/verify_watch.py --self-test

Idempotent: an issue is found by its label plus a marker comment in its body;
an open one is edited (only when its text changed), never duplicated (extra
open copies are closed as duplicates), and closed once nothing is stale. When a
source cannot be read (CI artifacts, the press API) the matching issue is left
as it is, never closed on missing evidence, and the run exits 1.

Without --apply nothing is written: the planned issue actions are printed.
Stdlib only; GitHub calls go through the `gh` CLI (GH_TOKEN in CI).
"""

from __future__ import annotations

import argparse
import datetime as dt
import io
import json
import re
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
STAMP = ROOT / "tools" / "maintainer" / "verify_maintained.json"
FEATURE_IDS = ("install", "mcp-stdio", "mcp-http", "write-gating", "read-only", "local-data")
# Every press version has an installer and a stdio MCP server; from 4.34.0 on,
# every connector has all six surfaces (features 3 to 6 flip FAIL -> PASS on
# the reprint). A SKIP inside those bounds means the map lost track of a surface.
ALWAYS_PRESENT = ("install", "mcp-stdio")
ALL_FEATURES_SINCE = "4.34.0"
STALE_DAYS = 30
WINDOW_DAYS = 14
PRESS_REPO = "mvanhorn/cli-printing-press"
MAP_LABEL, PRESS_LABEL = "verify-map-stale", "press-watch"
MAP_MARKER, PRESS_MARKER = "<!-- verify-watch:verify-map-stale -->", "<!-- verify-watch:press-watch -->"
LABELS = {MAP_LABEL: ("d93f0b", "The verify-msp-skills feature map may be behind the fleet (verify-watch.yml)"),
          PRESS_LABEL: ("0e8a16", "A newer cli-printing-press release than the fleet uses (verify-watch.yml)")}
_VER = re.compile(r"^v?(\d+)\.(\d+)\.(\d+)$")


def parse_version(text) -> tuple[int, int, int]:
    m = _VER.match(str(text or "").strip())
    if not m:
        raise ValueError(f"not a release version: {text!r}")
    return int(m.group(1)), int(m.group(2)), int(m.group(3))


def vstr(v: tuple[int, int, int]) -> str:
    return ".".join(map(str, v))


# --------------------------------------------------------------------------- #
# Inputs.
# --------------------------------------------------------------------------- #

def fleet_versions(root: Path) -> dict[str, tuple[int, int, int]]:
    """{slug: printing_press_version} for every manifest that records one.
    An unparseable version raises: the newest version must never be a guess."""
    out = {}
    for mf in sorted((root / "skills").glob("*/manifest.json")):
        v = json.loads(mf.read_text(encoding="utf-8")).get("printing_press_version")
        if v:
            out[mf.parent.name] = parse_version(v)
    return out


def load_stamp(path: Path) -> tuple[tuple[int, int, int], dt.date]:
    d = json.loads(path.read_text(encoding="utf-8"))
    return parse_version(d["maintained_at_press"]), dt.date.fromisoformat(d["maintained_at"])


VERDICTS = ("PASS", "FAIL", "ERROR", "SKIP")


def valid_record(rec) -> bool:
    """A record is evidence only if it names its slug and either carries all six
    feature verdicts or says outright that the gate errored. Anything thinner
    (an empty features object, a missing feature) is not proof of health."""
    if not isinstance(rec, dict) or not rec.get("slug") or not isinstance(rec.get("features"), dict):
        return False
    if rec.get("gate_exit") == 2 or rec.get("errors"):
        return True  # an explicit error record: it flags (b) by itself
    # Evidence of health must carry every field a decision reads: all six
    # verdicts for (b) and (c), and a readable press version for (c).
    try:
        parse_version(rec.get("press_version"))
    except ValueError:
        return False
    return all(rec["features"].get(f) in VERDICTS for f in FEATURE_IDS)


def error_record(slug: str, why: str) -> dict:
    return {"slug": slug, "press_version": None, "features": {}, "reasons": {}, "gate_exit": 2, "errors": [why]}


def record_from(gate: dict | None, verdict: dict | None) -> dict | None:
    """One connector's newest verify result, from verify_gate.py's gate.json or,
    when that is missing or incomplete, verify.py's own verdict.json. None when
    neither is usable evidence."""
    if isinstance(gate, dict) and isinstance(gate.get("features"), dict):
        rec = {"slug": gate.get("slug"), "press_version": gate.get("press_version"),
               "features": dict(gate["features"]), "reasons": dict(gate.get("reasons") or {}),
               "gate_exit": gate.get("gate_exit"), "errors": list(gate.get("errors") or [])}
        if valid_record(rec):
            return rec
    if isinstance(verdict, dict) and isinstance(verdict.get("features"), dict):
        feats = verdict["features"]
        rec = {"slug": verdict.get("slug"), "press_version": verdict.get("press_version"),
               "features": {k: (v or {}).get("verdict") if isinstance(v, dict) else None for k, v in feats.items()},
               "reasons": {k: str((v or {}).get("reason", ""))[:300] if isinstance(v, dict) else ""
                           for k, v in feats.items()},
               "gate_exit": None, "errors": [str(p) for p in verdict.get("harness_problems") or []]}
        if not all(rec["features"].get(f) in VERDICTS for f in FEATURE_IDS):
            missing = [f for f in FEATURE_IDS if rec["features"].get(f) not in VERDICTS]
            rec["errors"].append(f"verdict.json lacks a verdict for {missing}")
        if valid_record(rec):
            return rec
    return None


def load_results(results: Path) -> dict[str, dict]:
    """{slug: record}. Each <dir>/<slug>.json is one record (written by
    collect(), or by hand for a fixture)."""
    out = {}
    for p in sorted(results.glob("*.json")):
        rec = json.loads(p.read_text(encoding="utf-8"))
        slug = rec.get("slug") if isinstance(rec, dict) and rec.get("slug") else p.stem
        out[slug] = rec if valid_record(rec) else error_record(slug, f"{p.name} is not a usable verify result")
    return out


# --------------------------------------------------------------------------- #
# Decisions (pure: the self-test drives each trigger both ways).
# --------------------------------------------------------------------------- #

def map_reasons(fleet: dict, stamp_press, stamp_date, results: dict[str, dict] | None,
                today: dt.date, window_days: int = WINDOW_DAYS) -> list[str]:
    reasons = []
    if fleet:
        newest = max(fleet.values())
        if newest > stamp_press:
            at = sorted(s for s, v in fleet.items() if v == newest)
            reasons.append(f"(a) the fleet's newest printing_press_version is {vstr(newest)} "
                           f"({len(at)} connector(s): {', '.join(at[:5])}{', ...' if len(at) > 5 else ''}), "
                           f"above maintained_at_press {vstr(stamp_press)}")
    if results is not None:
        if not results:
            reasons.append(f"(b) no verify gate results were recorded on main in the last {window_days} days, "
                           f"so verify ERRORs and SKIPs cannot be checked")
        for slug in sorted(results):
            rec = results[slug]
            feats = rec.get("features") or {}
            errs = [f for f in FEATURE_IDS if feats.get(f) == "ERROR"]
            if errs or rec.get("gate_exit") == 2 or rec.get("errors"):
                why = "; ".join(f"{f}: {str((rec.get('reasons') or {}).get(f, ''))[:120]}" for f in errs)
                if not why:
                    why = "; ".join(str(e)[:120] for e in rec.get("errors") or []) or "gate exit 2"
                reasons.append(f"(b) verify ERROR on main for {slug}: {why}")
            try:
                press = parse_version(rec.get("press_version"))
            except ValueError:
                press = None
            for f in FEATURE_IDS:
                if feats.get(f) != "SKIP":
                    continue
                if f in ALWAYS_PRESENT:
                    reasons.append(f"(c) {slug} SKIPs {f}, which every press version has")
                elif press is not None and press >= parse_version(ALL_FEATURES_SINCE):
                    reasons.append(f"(c) {slug} SKIPs {f} at press {vstr(press)}, which has it "
                                   f"(every feature exists from {ALL_FEATURES_SINCE})")
    if (today - stamp_date).days >= STALE_DAYS:
        reasons.append(f"(d) maintained_at {stamp_date.isoformat()} is {STALE_DAYS} or more days old")
    return reasons


MAX_REASONS, MAX_REASON_CHARS = 40, 400  # keeps the body far under GitHub's 65536-character limit


def map_issue(reasons: list[str]) -> tuple[str, str] | None:
    if not reasons:
        return None
    shown = [f"- {r[:MAX_REASON_CHARS]}" for r in reasons[:MAX_REASONS]]
    if len(reasons) > MAX_REASONS:
        shown.append(f"- ...and {len(reasons) - MAX_REASONS} more (see the verify-watch run log)")
    counts = {k: sum(1 for r in reasons if r.startswith(f"({k})")) for k in "abcd"}
    body = "\n".join([
        MAP_MARKER,
        "The verify-msp-skills feature map (`.claude/skills/verify-msp-skills`) may be behind the fleet.",
        "",
        f"Why ({len(reasons)} reason(s): " + ", ".join(f"{k}={n}" for k, n in counts.items() if n) + "):",
        "",
        *shown,
        "",
        "What to do: run `/maintain-verification-skill` locally, on the subscription (not in CI). Then "
        "update `tools/maintainer/verify_maintained.json` (`maintained_at_press`, `maintained_at`) in the "
        "same PR.",
        "",
        "verify-watch.yml updates this issue in place and closes it on the first run where nothing is stale.",
    ]) + "\n"
    return "verify-map-stale: the verify-msp-skills map needs a maintenance pass", body


def press_issue(release: dict, fleet: dict) -> tuple[str, str] | None:
    latest = parse_version(release.get("tag_name"))
    newest = max(fleet.values()) if fleet else (0, 0, 0)
    if latest <= newest:
        return None
    at = sum(1 for v in fleet.values() if v == newest)
    # Third-party text lands in an issue body: only a link under the press's own
    # releases path is used verbatim; anything else is rebuilt from the tag.
    url = str(release.get("html_url") or "")
    if not re.fullmatch(rf"https://github\.com/{re.escape(PRESS_REPO)}/releases/tag/[A-Za-z0-9._-]+", url):
        url = f"https://github.com/{PRESS_REPO}/releases/tag/v{vstr(latest)}"
    published = str(release.get("published_at") or "")[:10]
    body = "\n".join([
        PRESS_MARKER,
        f"cli-printing-press {release.get('tag_name')} is out"
        f"{f' (published {published})' if published else ''}: {url}",
        "",
        f"The fleet's newest `printing_press_version` is {vstr(newest)} ({at} connector(s) at it, "
        f"{len(fleet)} with a recorded version).",
        "",
        "This is input for the reprint decision. Nothing reprints automatically. verify-watch.yml updates "
        "this issue when a newer release appears and closes it once the fleet has caught up.",
    ]) + "\n"
    return f"press-watch: cli-printing-press {release.get('tag_name')} is newer than the fleet ({vstr(newest)})", body


# --------------------------------------------------------------------------- #
# Issues: one per label, found by label + marker, never duplicated.
# --------------------------------------------------------------------------- #

class FakeIssues:
    """In-memory (optionally file-backed) stand-in for the GitHub issues API."""

    def __init__(self, path: Path | None = None):
        self.path = path
        self.issues = json.loads(path.read_text(encoding="utf-8")) if path and path.is_file() else []

    def save(self) -> None:
        if self.path:
            self.path.write_text(json.dumps(self.issues, indent=1) + "\n", encoding="utf-8")

    def ensure_label(self, name: str) -> None:
        pass

    def list_open(self, label: str) -> list[dict]:
        return [i for i in self.issues if i["state"] == "open" and label in i["labels"]]

    def create(self, title: str, body: str, label: str) -> int:
        n = max([i["number"] for i in self.issues] or [0]) + 1
        self.issues.append({"number": n, "title": title, "body": body, "labels": [label], "state": "open"})
        return n

    def edit(self, number: int, title: str, body: str) -> None:
        for i in self.issues:
            if i["number"] == number:
                i.update(title=title, body=body)

    def close(self, number: int, comment: str) -> None:
        for i in self.issues:
            if i["number"] == number:
                i["state"] = "closed"
                i.setdefault("comments", []).append(comment)


class GhIssues:
    def __init__(self, repo: str):
        self.repo = repo

    def _api(self, *args: str, inp: dict | None = None) -> str:
        argv = ["gh", "api", *args]
        if inp is not None:
            argv += ["--input", "-"]
        r = subprocess.run(argv, input=json.dumps(inp) if inp is not None else None,
                           capture_output=True, text=True, timeout=120)
        if r.returncode != 0:
            raise RuntimeError(f"gh api {' '.join(args[:3])} failed: {r.stderr.strip()[:300]}")
        return r.stdout

    def ensure_label(self, name: str) -> None:
        color, desc = LABELS[name]
        try:
            self._api(f"repos/{self.repo}/labels/{name}")
        except RuntimeError:
            self._api("-X", "POST", f"repos/{self.repo}/labels", inp={"name": name, "color": color,
                                                                       "description": desc})

    def list_open(self, label: str) -> list[dict]:
        # One compact JSON object per line (@json), so a body containing "][" or
        # a newline can never be mistaken for a page boundary.
        out = self._api("-X", "GET", "--paginate", f"repos/{self.repo}/issues",
                        "-f", f"labels={label}", "-f", "state=open", "-f", "per_page=100",
                        "-q", ".[] | select(.pull_request == null) | {number, title, body, state, "
                              "labels: [.labels[].name]} | @json")
        items = [json.loads(ln) for ln in out.splitlines() if ln.strip()]
        return [{**i, "body": i.get("body") or ""} for i in items]

    def create(self, title: str, body: str, label: str) -> int:
        out = self._api("-X", "POST", f"repos/{self.repo}/issues",
                        inp={"title": title, "body": body, "labels": [label]})
        return json.loads(out)["number"]

    def edit(self, number: int, title: str, body: str) -> None:
        self._api("-X", "PATCH", f"repos/{self.repo}/issues/{number}", inp={"title": title, "body": body})

    def close(self, number: int, comment: str) -> None:
        self._api("-X", "POST", f"repos/{self.repo}/issues/{number}/comments", inp={"body": comment})
        self._api("-X", "PATCH", f"repos/{self.repo}/issues/{number}",
                  inp={"state": "closed", "state_reason": "completed"})


CLOSE_NOTE = {MAP_MARKER: "Nothing is stale on this verify-watch run; closing. A new issue opens if that changes.",
              PRESS_MARKER: "The fleet has caught up with the latest cli-printing-press release; closing."}


def plan(open_issues: list[dict], marker: str, want: tuple[str, str] | None) -> list[tuple]:
    """The actions that make the open issues match `want`. Pure."""
    mine = sorted((i for i in open_issues if marker in (i.get("body") or "")), key=lambda i: i["number"])
    acts: list[tuple] = []
    if want is None:
        return [("close", i["number"], CLOSE_NOTE[marker]) for i in mine]
    title, body = want
    if not mine:
        return [("create", title, body)]
    keep = mine[0]
    if keep.get("title") != title or keep.get("body") != body:
        acts.append(("edit", keep["number"], title, body))
    for dup in mine[1:]:
        acts.append(("close", dup["number"], f"Duplicate of #{keep['number']}; closing."))
    return acts


def sync(backend, label: str, marker: str, want: tuple[str, str] | None, apply: bool) -> list[tuple]:
    acts = plan(backend.list_open(label), marker, want)
    for a in acts:
        print(f"  {label}: {a[0]} {a[1] if a[0] != 'create' else repr(a[1])}" + ("" if apply else "  (dry run)"))
        if not apply:
            continue
        if a[0] == "create":
            backend.ensure_label(label)
            n = backend.create(a[1], a[2], label)
            print(f"  {label}: opened #{n}")
        elif a[0] == "edit":
            backend.edit(a[1], a[2], a[3])
        else:
            backend.close(a[1], a[2])
    if not acts:
        print(f"  {label}: no change")
    return acts


# --------------------------------------------------------------------------- #
# Collection from main's CI runs (the verify-<slug> artifacts).
# --------------------------------------------------------------------------- #

def gh_json(*args: str):
    r = subprocess.run(["gh", "api", *args], capture_output=True, text=True, timeout=120)
    if r.returncode != 0:
        raise RuntimeError(f"gh api {' '.join(args[:3])} failed: {r.stderr.strip()[:300]}")
    return json.loads(r.stdout)


TRUSTED_EVENTS = ("push", "schedule", "workflow_dispatch")
MAX_PAGES = 20  # 2000 items; a listing longer than that is read as incomplete, never truncated


def paged(path: str, key: str, *args: str, attempts: int = 3) -> list[dict]:
    """Every item of a paginated GitHub listing, as one consistent snapshot.
    Completeness is counted in unique ids against the reported total; if the
    total changes between pages (a run landed mid-listing and shifted the
    pages) the listing starts over, up to `attempts` times. A missing or
    malformed total, a page that is not a list, an item without an id, an
    empty page before the total, too many pages, or a listing that never holds
    still each raise: a partial listing could hide a connector's newest result."""
    for _ in range(attempts):
        seen: dict = {}
        total = None
        for page in range(1, MAX_PAGES + 1):
            got = gh_json("-X", "GET", path, *args, "-f", "per_page=100", "-f", f"page={page}")
            t = got.get("total_count") if isinstance(got, dict) else None
            batch = got.get(key) if isinstance(got, dict) else None
            if not isinstance(t, int) or isinstance(t, bool) or t < 0 or not isinstance(batch, list):
                raise RuntimeError(f"{path} page {page}: no total_count or no {key} list")
            if total is not None and t != total:
                break  # the listing moved under us: start over
            total = t
            for item in batch:
                if not isinstance(item, dict) or item.get("id") is None:
                    raise RuntimeError(f"{path} page {page}: an item without an id")
                seen[item["id"]] = item
            if len(seen) >= total:
                return list(seen.values())
            if not batch:
                raise RuntimeError(f"{path} ended at {len(seen)} of {total} {key}; listing incomplete")
        else:
            raise RuntimeError(f"{path}: more than {MAX_PAGES} pages; listing incomplete")
    raise RuntimeError(f"{path} kept changing across {attempts} listings; listing incomplete")


def list_runs(repo: str, branch: str, since: dt.date) -> list[dict]:
    """Every completed ci.yml run on `branch` created since `since`."""
    return paged(f"repos/{repo}/actions/workflows/ci.yml/runs", "workflow_runs", "-f", f"branch={branch}",
                 "-f", "status=completed", "-f", f"created=>={since.isoformat()}")


def trusted_run(run: dict, repo: str, branch: str) -> bool:
    """Only main's own runs count: a pull_request run (a fork's branch may also
    be named main) never speaks for main."""
    return (run.get("event") in TRUSTED_EVENTS and run.get("head_branch") == branch
            and (run.get("head_repository") or {}).get("full_name", "").lower() == repo.lower())


def run_artifacts(repo: str, run_id) -> list[dict]:
    return paged(f"repos/{repo}/actions/runs/{run_id}/artifacts", "artifacts")


def collect(repo: str, branch: str, out: Path, today: dt.date, window_days: int = WINDOW_DAYS) -> int:
    """Write the newest verify record per connector from main's own ci.yml runs
    in the window to out/<slug>.json. Artifact zips are read in memory, member
    by basename; nothing is extracted to disk. Returns the number of records."""
    out.mkdir(parents=True, exist_ok=True)
    since = today - dt.timedelta(days=window_days)
    seen: set[str] = set()
    runs = [r for r in list_runs(repo, branch, since) if trusted_run(r, repo, branch)]
    for run in sorted(runs, key=lambda r: (r["created_at"], r["id"]), reverse=True):
        if dt.date.fromisoformat(run["created_at"][:10]) < since:
            continue
        for art in run_artifacts(repo, run["id"]):
            name = art.get("name", "")
            slug = name[len("verify-"):]
            if not name.startswith("verify-") or art.get("expired") or slug in seen:
                continue
            z = subprocess.run(["gh", "api", f"repos/{repo}/actions/artifacts/{art['id']}/zip"],
                               capture_output=True, timeout=120)
            if z.returncode != 0:
                raise RuntimeError(f"cannot download artifact {name} of run {run['id']}")
            gate = verdict = None
            try:
                with zipfile.ZipFile(io.BytesIO(z.stdout)) as zf:
                    for member in zf.namelist():
                        base = member.rsplit("/", 1)[-1]
                        if base == "gate.json" and gate is None:
                            gate = json.loads(zf.read(member))
                        elif base == "verdict.json" and verdict is None:
                            verdict = json.loads(zf.read(member))
            except (zipfile.BadZipFile, ValueError):
                gate = verdict = None
            rec = record_from(gate, verdict)
            if rec is None or rec.get("slug") != slug:
                rec = error_record(slug, f"artifact {name} of run {run['id']} holds no usable verify result")
            rec["source"] = {"run_id": run["id"], "run_url": run.get("html_url"), "event": run.get("event"),
                             "created_at": run["created_at"], "head_sha": run.get("head_sha")}
            (out / f"{slug}.json").write_text(json.dumps(rec, indent=1) + "\n", encoding="utf-8")
            seen.add(slug)
    return len(seen)


# --------------------------------------------------------------------------- #
# Main.
# --------------------------------------------------------------------------- #

def run(args) -> int:
    today = dt.date.fromisoformat(args.today) if args.today else dt.datetime.now(dt.timezone.utc).date()
    root = Path(args.skills_root) if args.skills_root else ROOT
    problems: list[str] = []
    fleet = fleet_versions(root)
    try:
        stamp_press, stamp_date = load_stamp(Path(args.stamp))
        stamp_ok = True
    except (OSError, ValueError, KeyError, TypeError) as e:
        problems.append(f"stamp {args.stamp} unreadable: {e!r}")
        stamp_ok = False

    results: dict[str, dict] | None = None
    try:
        if args.collect:
            rdir = Path(args.results) if args.results else Path(tempfile.mkdtemp(prefix="verify-watch-"))
            n = collect(args.repo, args.branch, rdir, today)
            print(f"collected {n} verify result(s) from {args.branch} CI runs in the last {WINDOW_DAYS} days")
            results = load_results(rdir)
        elif args.results:
            results = load_results(Path(args.results))
    except (RuntimeError, OSError, ValueError, zipfile.BadZipFile) as e:
        problems.append(f"verify results unreadable: {e}")
        results = None
    results_known = results is not None or not (args.collect or args.results)

    if args.issues_fixture:
        backend = FakeIssues(Path(args.issues_fixture))
    else:
        backend = GhIssues(args.repo) if args.repo else None

    print(f"fleet: {len(fleet)} connectors with a press version; newest "
          f"{vstr(max(fleet.values())) if fleet else 'none'}")
    if stamp_ok:
        reasons = map_reasons(fleet, stamp_press, stamp_date, results, today)
    else:
        reasons = [f"(d) {args.stamp} is unreadable, so when the map was maintained is unknown"]
    want = map_issue(reasons)
    print(f"verify map: {'STALE' if reasons else 'current'}")
    for r in reasons:
        print(f"  - {r}")
    if want is None and not results_known:
        print("  verify-map-stale: left as it is (verify results unreadable; never closed on missing evidence)")
    elif backend is not None:
        sync(backend, MAP_LABEL, MAP_MARKER, want, args.apply)

    try:
        release = (json.loads(Path(args.press_release).read_text(encoding="utf-8")) if args.press_release
                   else gh_json(f"repos/{PRESS_REPO}/releases/latest"))
        pwant = press_issue(release, fleet)
        print(f"press: latest {release.get('tag_name')} ({release.get('html_url')}); "
              f"{'NEWER than the fleet' if pwant else 'the fleet is current'}")
        if backend is not None:
            sync(backend, PRESS_LABEL, PRESS_MARKER, pwant, args.apply)
    except (RuntimeError, OSError, ValueError, KeyError) as e:
        problems.append(f"press release unreadable: {e}")

    if isinstance(backend, FakeIssues) and args.apply:
        backend.save()
    for p in problems:
        print(f"ERROR: {p}")
    return 1 if problems else 0


def self_test() -> int:
    bad: list[str] = []

    def expect(cond: bool, label: str) -> None:
        print(("  ok   " if cond else "  FAIL ") + label)
        if not cond:
            bad.append(label)

    v = parse_version
    today = dt.date(2026, 10, 23)
    fleet = {"cipp": v("4.24.0"), "servosity": v("4.24.0"), "zammad": v("4.33.5")}
    healthy = {"cipp": {"slug": "cipp", "press_version": "4.24.0", "gate_exit": 0, "errors": [],
                        "features": {"install": "PASS", "mcp-stdio": "PASS", "mcp-http": "FAIL",
                                     "write-gating": "FAIL", "read-only": "FAIL", "local-data": "FAIL"}}}

    def reasons(**kw):
        a = dict(fleet=fleet, stamp_press=v("4.34.0"), stamp_date=today, results=healthy, today=today)
        a.update(kw)
        return map_reasons(**a)

    print("verify-map-stale triggers (each fires on its shape, silent on the healthy fleet):")
    expect(reasons() == [], "healthy fleet, fresh stamp, no ERROR or SKIP -> nothing stale")
    r = reasons(fleet={**fleet, "cipp": v("4.35.0")})
    expect(len(r) == 1 and r[0].startswith("(a)") and "4.35.0" in r[0], "(a) newest press above the stamp -> stale")
    expect(reasons(fleet={**fleet, "cipp": v("4.34.0")}) == [], "(a) newest press EQUAL to the stamp -> silent")
    err = {"cipp": {**healthy["cipp"], "gate_exit": 2,
                    "features": {**healthy["cipp"]["features"], "mcp-http": "ERROR"},
                    "reasons": {"mcp-http": "default port 7777 is already in use"}}}
    r = reasons(results=err)
    expect(len(r) == 1 and r[0].startswith("(b)") and "7777" in r[0], "(b) a verify ERROR on main -> stale")
    r = reasons(results={})
    expect(len(r) == 1 and r[0].startswith("(b) no verify gate results"),
           "(b) no results recorded in the window -> stale (fails closed, never silently current)")
    expect(reasons(results=None) == [], "(b)/(c) not requested (results=None) -> not judged")
    old_skip = {"cipp": {**healthy["cipp"], "features": {**healthy["cipp"]["features"], "write-gating": "SKIP"}}}
    expect(reasons(results=old_skip) == [], "(c) write-gating SKIP at press 4.24.0 -> silent (may lack _execute)")
    new_skip = {"x": {"slug": "x", "press_version": "4.34.0", "gate_exit": 0, "errors": [],
                      "features": {f: "PASS" for f in FEATURE_IDS} | {"read-only": "SKIP"}}}
    r = reasons(results=new_skip)
    expect(len(r) == 1 and r[0].startswith("(c)") and "read-only" in r[0],
           "(c) read-only SKIP at press 4.34.0 -> stale")
    always = {"cipp": {**healthy["cipp"], "features": {**healthy["cipp"]["features"], "mcp-stdio": "SKIP"}}}
    r = reasons(results=always)
    expect(len(r) == 1 and r[0].startswith("(c)"), "(c) mcp-stdio SKIP at any press -> stale")
    r = reasons(stamp_date=today - dt.timedelta(days=30))
    expect(len(r) == 1 and r[0].startswith("(d)"), "(d) stamp 30 days old -> stale")
    expect(reasons(stamp_date=today - dt.timedelta(days=29)) == [], "(d) stamp 29 days old -> silent")

    print("press-watch:")
    rel = {"tag_name": "v4.34.0", "html_url": f"https://github.com/{PRESS_REPO}/releases/tag/v4.34.0",
           "published_at": "2026-10-23T14:51:39Z"}
    pw = press_issue(rel, fleet)
    expect(pw is not None and "v4.34.0" in pw[0] and rel["html_url"] in pw[1] and "4.33.5" in pw[1],
           "release newer than the fleet -> issue names the version, the link and the fleet's newest")
    odd = press_issue({**rel, "html_url": "https://evil.example/x) [click](https://evil.example"}, fleet)
    expect(odd is not None and "evil" not in odd[1] and f"{PRESS_REPO}/releases/tag/v4.34.0" in odd[1],
           "a release link outside the press's releases path -> rebuilt from the tag")
    expect(press_issue({**rel, "tag_name": "v4.33.5"}, fleet) is None, "release equal to the fleet -> no issue")
    expect(press_issue({**rel, "tag_name": "v4.30.0"}, fleet) is None, "release older than the fleet -> no issue")
    try:
        press_issue({**rel, "tag_name": "nightly"}, fleet)
        expect(False, "unparseable release tag -> raises (never read as current)")
    except ValueError:
        expect(True, "unparseable release tag -> raises (never read as current)")

    print("issue sync (idempotent, no duplicates, closes when clear):")
    fake = FakeIssues()
    want = map_issue(["(d) old"])
    sync(fake, MAP_LABEL, MAP_MARKER, want, True)
    expect(len(fake.list_open(MAP_LABEL)) == 1, "stale -> one issue opened")
    expect(sync(fake, MAP_LABEL, MAP_MARKER, want, True) == [], "same reasons again -> no action, no duplicate")
    want2 = map_issue(["(a) newer", "(d) old"])
    acts = sync(fake, MAP_LABEL, MAP_MARKER, want2, True)
    expect([a[0] for a in acts] == ["edit"] and len(fake.list_open(MAP_LABEL)) == 1
           and fake.list_open(MAP_LABEL)[0]["body"] == want2[1], "reasons changed -> edited in place")
    fake.issues.append({"number": 99, "title": "copy", "body": want2[1], "labels": [MAP_LABEL], "state": "open"})
    acts = sync(fake, MAP_LABEL, MAP_MARKER, want2, True)
    expect(acts == [("close", 99, "Duplicate of #1; closing.")] and len(fake.list_open(MAP_LABEL)) == 1,
           "a duplicate open copy -> closed, the oldest kept")
    fake.issues.append({"number": 100, "title": "unrelated", "body": "hand-written", "labels": [MAP_LABEL],
                        "state": "open"})
    acts = sync(fake, MAP_LABEL, MAP_MARKER, None, True)
    expect([a[0] for a in acts] == ["close"] and [i["number"] for i in fake.list_open(MAP_LABEL)] == [100],
           "nothing stale -> our issue closed; a hand-written issue with the label is left alone")
    expect(sync(fake, MAP_LABEL, MAP_MARKER, None, True) == [], "nothing stale again -> no action")
    dry = FakeIssues()
    sync(dry, PRESS_LABEL, PRESS_MARKER, pw, False)
    expect(dry.issues == [], "without --apply -> nothing written")

    print("records:")
    expect(record_from(None, {"slug": "a", "press_version": "4.24.0", "harness_problems": ["x"],
                              "features": {"install": {"verdict": "PASS", "reason": "r"}}})["errors"][0] == "x",
           "verdict.json fallback keeps harness problems (and names missing features)")
    expect(record_from(None, None) is None, "no gate.json and no verdict.json -> no record")

    print("collection from main's CI runs (offline, fake gh):")
    for label, want, runs, arts, zips in _collect_cases():
        got = _drive_collect(runs, arts, zips, today)
        expect(any(r.startswith("(b) verify ERROR") for r in got) is want, label)
    real_gh = globals()["gh_json"]
    for label, pages in (
            ("an empty page before the reported total -> listing raises (unreadable, never partial)",
             {1: {"total_count": 101, "workflow_runs": [_run(i) for i in range(1, 101)]},
              2: {"total_count": 101, "workflow_runs": []}}),
            ("a listing without total_count -> raises", {1: {"workflow_runs": []}}),
            ("a total_count that is not an integer -> raises", {1: {"total_count": "5", "workflow_runs": []}})):
        globals()["gh_json"] = lambda *a, _p=pages: _p[int(next(x for x in a if x.startswith("page="))[5:])]
        try:
            list_runs("Servosity/msp-skills", "main", today)
            expect(False, label)
        except RuntimeError:
            expect(True, label)
        finally:
            globals()["gh_json"] = real_gh
    calls = {"n": 0}

    def shifting(*a):
        page = int(next(x for x in a if x.startswith("page="))[5:])
        calls["n"] += 1
        if calls["n"] <= 2:  # a run completes between page 1 and page 2 of the first listing
            return ({"total_count": 200, "workflow_runs": [{"id": i} for i in range(200, 100, -1)]} if page == 1
                    else {"total_count": 201, "workflow_runs": [{"id": i} for i in range(101, 1, -1)]})
        ids = list(range(201, 0, -1))
        return {"total_count": 201, "workflow_runs": [{"id": i} for i in ids[(page - 1) * 100: page * 100]]}

    globals()["gh_json"] = shifting
    try:
        got = {r["id"] for r in list_runs("Servosity/msp-skills", "main", today)}
        expect(1 in got and len(got) == 201, "pages shifted by a run landing mid-listing -> relisted, no run lost")
    finally:
        globals()["gh_json"] = real_gh
    nover = {"slug": "cipp", "press_version": None, "gate_exit": 0, "errors": [],
             "features": {f: "PASS" for f in FEATURE_IDS} | {"read-only": "SKIP"}}
    expect(not valid_record(nover), "a record with no readable press version is not evidence of health")
    expect(valid_record({"slug": "a", "press_version": None, "features": {}, "gate_exit": 2, "errors": ["x"]}),
           "an explicit error record is valid (it flags (b) by itself)")
    many = {f"s{i:02d}": {"slug": f"s{i:02d}", "press_version": "4.24.0", "gate_exit": 2, "errors": [],
                          "features": {f: "ERROR" for f in FEATURE_IDS}, "reasons": {f: "A" * 120 for f in FEATURE_IDS}}
            for i in range(84)}
    body = map_issue(map_reasons(fleet, v("4.34.0"), today, many, today))[1]
    expect(len(body) < 60000 and "more (see the verify-watch run log)" in body,
           f"every connector ERRORing everywhere -> body bounded ({len(body)} chars < 65536)")

    print(f"\n{'PASS' if not bad else 'FAIL'}: verify_watch self-test ({len(bad)} failure(s))")
    return 0 if not bad else 1


def _zip(**members) -> bytes:
    b = io.BytesIO()
    with zipfile.ZipFile(b, "w") as z:
        for n, obj in members.items():
            z.writestr(n.replace("__", "/"), json.dumps(obj))
    return b.getvalue()


def _gate(slug: str, **verdicts: str) -> dict:
    f = {k: "PASS" for k in FEATURE_IDS}
    f.update({k.replace("_", "-"): x for k, x in verdicts.items()})
    return {"slug": slug, "press_version": "4.24.0", "features": f, "reasons": {k: k for k in f},
            "gate_exit": 2 if "ERROR" in f.values() else 0, "errors": []}


def _run(i: int, event: str = "push", repo: str = "Servosity/msp-skills",
         created: str = "2026-10-22T00:00:00Z") -> dict:
    return {"id": i, "event": event, "head_branch": "main", "head_repository": {"full_name": repo},
            "created_at": created, "html_url": f"run/{i}", "head_sha": f"sha{i}"}


def _collect_cases():
    """(label, want (b) flagged, runs newest first, {run: [artifact names]}, {(run, name): zip})."""
    err = _gate("cipp", mcp_stdio="ERROR")
    runs = [_run(i) for i in range(1, 102)]
    arts = {i: ["verify-zammad"] for i in range(1, 101)}
    arts[101] = ["verify-cipp"]
    zips = {(i, "verify-zammad"): _zip(**{"gate.json": _gate("zammad")}) for i in range(1, 101)}
    zips[(101, "verify-cipp")] = _zip(**{"gate.json": err})
    yield "an ERROR beyond the first page of runs is still read", True, runs, arts, zips
    yield ("a newer pull_request run (a fork's branch named main) cannot speak for main", True,
           [_run(3, "pull_request", "evil/fork", "2026-10-22T09:00:00Z"), _run(2)],
           {3: ["verify-cipp"], 2: ["verify-cipp"]},
           {(3, "verify-cipp"): _zip(**{"gate.json": _gate("cipp")}), (2, "verify-cipp"): _zip(**{"gate.json": err})})
    verdict = {"slug": "cipp", "press_version": "4.24.0", "harness_problems": [],
               "features": {"mcp-stdio": {"verdict": "ERROR", "reason": "harness failed"}}}
    yield ("an incomplete gate.json falls back to verdict.json's ERROR", True, [_run(1)], {1: ["verify-cipp"]},
           {(1, "verify-cipp"): _zip(**{"g__gate.json": {"slug": "cipp", "features": {}},
                                        "e__cipp__r__verdict.json": verdict})})
    yield ("an incomplete gate.json alone is not evidence of health", True, [_run(1)], {1: ["verify-cipp"]},
           {(1, "verify-cipp"): _zip(**{"gate.json": {"slug": "cipp", "features": {}}})})
    yield ("a corrupt artifact zip is not evidence of health", True, [_run(1)], {1: ["verify-cipp"]},
           {(1, "verify-cipp"): b"not a zip"})
    yield ("main's newest healthy result after an older ERROR -> not flagged", False, [_run(2), _run(1)],
           {2: ["verify-cipp"], 1: ["verify-cipp"]},
           {(2, "verify-cipp"): _zip(**{"gate.json": _gate("cipp")}), (1, "verify-cipp"): _zip(**{"gate.json": err})})


def _drive_collect(runs, arts, zips, today) -> list[str]:
    ids: dict[int, tuple] = {}

    def fake_gh(*args):
        path = next(a for a in args if a.startswith("repos/"))
        q = dict(a.split("=", 1) for a in args if "=" in a and not a.startswith("repos/"))
        per, page = int(q.get("per_page", 30)), int(q.get("page", 1))
        if path.endswith("/workflows/ci.yml/runs"):
            return {"total_count": len(runs), "workflow_runs": runs[(page - 1) * per: page * per]}
        rid = int(path.split("/runs/")[1].split("/")[0])
        listed = []
        for n in arts.get(rid, []):
            ids[len(ids) + 1] = (rid, n)
            listed.append({"id": len(ids), "name": n, "expired": False})
        return {"total_count": len(listed), "artifacts": listed[(page - 1) * per: page * per]}

    class _Done:
        def __init__(self, out: bytes):
            self.returncode, self.stdout, self.stderr = 0, out, b""

    real_run, real_gh = subprocess.run, globals()["gh_json"]

    def fake_run(argv, *a, **k):
        if argv[:2] == ["gh", "api"] and argv[2].endswith("/zip"):
            return _Done(zips[ids[int(argv[2].split("/artifacts/")[1].split("/")[0])]])
        return real_run(argv, *a, **k)

    globals()["gh_json"], subprocess.run = fake_gh, fake_run
    try:
        with tempfile.TemporaryDirectory(prefix="verify-watch-selftest-") as tmp:
            collect("Servosity/msp-skills", "main", Path(tmp), today)
            return map_reasons({"cipp": (4, 24, 0)}, (4, 34, 0), today, load_results(Path(tmp)), today)
    finally:
        globals()["gh_json"], subprocess.run = real_gh, real_run


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--self-test", action="store_true")
    ap.add_argument("--repo", help="OWNER/REPO for artifacts and issues (gh api)")
    ap.add_argument("--branch", default="main", help="whose ci.yml runs to read results from (default main)")
    ap.add_argument("--collect", action="store_true", help="download the newest verify-<slug> artifacts first")
    ap.add_argument("--results", metavar="DIR", help="one <slug>.json record per connector (collect writes here)")
    ap.add_argument("--press-release", metavar="FILE", help="release JSON instead of the GitHub API (fixtures)")
    ap.add_argument("--stamp", default=str(STAMP))
    ap.add_argument("--skills-root", metavar="DIR", help="repo root whose skills/*/manifest.json to read")
    ap.add_argument("--today", metavar="YYYY-MM-DD")
    ap.add_argument("--issues-fixture", metavar="FILE", help="a JSON file standing in for the issues API")
    ap.add_argument("--apply", action="store_true", help="write the issue changes (default: print them)")
    args = ap.parse_args(argv)
    if args.self_test:
        return self_test()
    if args.collect and not args.repo:
        ap.error("--collect needs --repo")
    return run(args)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
