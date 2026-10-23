#!/usr/bin/env python3
"""Gate: every version-pinned GitHub release URL this repo publishes resolves.

The failure mode this blocks: release.py's propagate() rewrites the
`<slug>-v<version>` segment of skills/<slug>/server.json's `.mcpb` identifier
(and build-catalog.py re-mints the README release links) at version-BUMP time,
but the git tag that makes those URLs real is cut by hand, later, one push at a
time. Skip that step and main ships a download link that 404s forever. On
2026-08-25 that was 32 of 65 server.json `.mcpb` identifiers, 31 root-README
release links, and 8 skill-README `.mcpb` links - every one of them internally
self-consistent, none of them downloadable.

Why a CONSISTENCY check cannot catch this: release.py stamps the pin and the
manifest version in the same commit, so `pin == "{slug}-v{manifest.version}"`
holds for 65 of 65 slugs and catches 0 of the 32 broken pins. Only tag
EXISTENCE can see the defect.

Why a naive "the tag must exist" check cannot be used either: at the moment of
the release commit the tag does NOT exist yet (it is pushed after the commit
lands), so that check would fail every release.

The discriminator is therefore DIFF-scoped, not consistency-scoped:

  * tag exists locally                     -> OK.
  * tag missing, and this change-set INTRODUCED the pin (the exact pin string
    appears in NO tracked file at BASE) -> allowed as an in-flight release,
    provided ALL of: the tag is "{slug}-v{manifest.version}"; it sorts strictly
    above the highest tag that already exists for that slug; AND the version
    this slug carried at BASE was itself tagged, OR is recorded as RETIRED in
    tools/maintainer/burned_versions.json (a number whose release was cut,
    sealed empty by the old publish-first pipeline and withdrawn - the opposite
    of the negligence this clause is aimed at, and a number nobody may ever cut
    again). Prints the probe-gated `check_release_pipeline.py --tag ... && git
    tag ...` command as a reminder.
  * tag missing, and the pin already existed ANYWHERE at BASE -> FAIL, unless
    the release is still IN FLIGHT (next bullet). The release was never cut;
    main is publishing a 404.
  * IN FLIGHT means the release commit has landed but the wave is still
    cutting its tags. cut_tags.sh cuts one tag per push with at most four
    releases running, so a wave takes hours, and for those hours every pin of
    every connector in it is already on main with no tag. Before this clause,
    "never cut" and "being cut right now" read the same, so EVERY unrelated
    pull request was red until the wave sealed (2026-10-23: one commit bumped
    81 connectors and tools-only PRs read FAIL). A pin that existed at BASE
    and whose tag is missing is allowed, with the same reminder as a new pin,
    only when ALL of:
      - the slug's manifest carries that version both at BASE and in the tree;
      - the newest commit on BASE's FIRST-PARENT history that changed that
        manifest's version TO that version is less than PINS_INFLIGHT_HOURS
        old (default 24; 0 turns this clause off), by its committer date,
        measured against the clock of the run, and is not dated more than a
        15-minute clock skew into the future;
      - the same rules as a new pin: the version sorts above every existing
        tag for the slug and is not retired, and the version the manifest
        carried before that bump was itself tagged or retired (or there was
        no manifest before it and the slug has no tags: a brand-new skill).
    Past the window it FAILS exactly as before. The age is computed from git
    alone (the CI checkout is fetch-depth: 0), so the check stays offline.

Three ways that escape hatch was laundered before, and how each is closed:

  * A RENAME used to launder a hard failure into a green reminder. The BASE
    lookup was `git show BASE:<current path>`, which returns nothing when the
    path did not exist at BASE, so `git mv` made every pin in the file - even
    ones dead for months - read as introduced by this change-set with no content
    change at all. The pin lookup is now repo-wide (next bullet), which makes a
    rename invisible to it; `git diff --find-renames` is still used for the
    per-slug manifest-version lookup, which is inherently path-addressed.
  * A CROSS-FILE MOVE laundered a dead pin even without a rename. The BASE
    lookup was per-PATH: it asked only whether THIS file carried the URL at
    BASE. Deleting a months-dead pin from `old.md` and pasting the same URL into
    an already-existing `new.md` therefore read as newly introduced - absent
    from `new.md` at BASE - and collected an in-flight reminder instead of a
    failure. The BASE lookup is now repo-WIDE: the pin must appear in no tracked
    file at BASE to count as introduced by this change-set. A genuine in-flight
    release is unaffected, because a version bump mints a URL that exists
    nowhere at BASE.
  * The grace was UNBOUNDED. Set-membership of the pin URL is renewed by every
    version bump, so three consecutive bumps with zero tags cut all stayed
    green. Grace now additionally requires that the version this slug shipped at
    BASE has its tag cut. First uncut bump: allowed once. Second: failure,
    naming the release that was never cut. A skill with no manifest at BASE is a
    brand-new skill and gets its first release.

The in-flight window is closed against the same kinds of laundering:

  * It is decided on BASE only, never on the change-set: a pull request's own
    commits are not reachable from BASE, so they cannot refresh it; the
    manifest is found through BASE's registry, so repointing a slug's
    source_dir at another, recently bumped directory changes nothing; and only
    BASE's retired-version ledger can excuse an uncut prior (a retirement in
    either BASE's ledger or the tree's denies the window).
  * It walks FIRST-PARENT history only, so a merged branch is dated by when it
    LANDED on main (the merge commit), not by the committer date its author
    wrote on the branch commit.
  * It keys on the VERSION changing, not on the FILE changing: a recent edit to
    another field of a months-old manifest does not reopen it.
  * A manifest that appears at a commit with no manifest in its parent while
    the slug already has tags (a moved or re-created skill directory) cannot
    show which release came before it, so it gets no window.
  * A shallow checkout cannot date the bump, so it gets no window.
  Accepted residual: someone who can push to main can write a commit's date,
  or flip a version away and back. That buys at most one more window, in
  public history; writing to main is the trust boundary this gate sits behind.

That gives a release PINS_INFLIGHT_HOURS of grace, counted from the moment it
lands on main and never renewed by later pushes, and turns a never-cut tag into
a red main on the first push or pull request after that, naming the slug.

BASE is `${{ github.event.pull_request.base.sha }}` on PRs and
`${{ github.event.before }}` on pushes. Do NOT pass origin/main on a push event:
the runner's origin/main already contains the push, so every pin would look
unchanged and the release commit itself would fail. When BASE is unreachable
(or the checkout carries no tags at all) the existence check SKIPs with a
notice - the precedent is check_registry_state.py.

Also checked, offline and base-independent:
  * the pinned asset filename matches its class - a `.mcpb` pin must name
    `{mcp_binary}.mcpb`, a raw-binary pin must name an asset the release
    workflow actually produces;
  * the pinned slug equals the skills/<dir>/ that owns the file (catches a
    copy-paste pin pointing at another connector's release);
  * FRESHNESS: a pin naming an older version than the registry ships, when the
    current version's tag already exists. Advisory by default (the fix is to run
    build-catalog.py, which owns those lines); --strict promotes it to a failure.

ASSET existence is the other half, and it runs BY DEFAULT whenever the gh CLI is
present and authenticated: one `gh api repos/{owner}/{repo}/releases/tags/{TAG}`
per DISTINCT pinned tag (cached), asserting the pinned ASSET NAME is really in
that release's asset list. Tag existence alone passes datto-rmm-v0.1.2, whose
release object is green with 24 assets because its mcp-publish run was CANCELLED
and the `.mcpb` never uploaded - so a one-click download link that 404s today is
invisible to every offline check. Default-on is the wiring: verify_all.sh and
the CI job already call this script, and neither had to change.

It is fail-SOFT, never fail-flaky. If gh is missing, unauthenticated, or a
preflight `gh api repos/{owner}/{repo}` cannot reach GitHub, the asset pass
prints a NOTE and is skipped rather than reporting every tag as missing. The
same discrimination is applied per TAG: only a 404/410 from GitHub itself counts
as "no published release"; a 502, a secondary rate limit or a timeout skips that
one tag with a NOTE, because telling a maintainer to cut a release that already
exists is a false-RED, and a gate that fires on a healthy repo teaches people to
ignore it. Pass --network to require the pass (exit 2 when gh is unusable) or
--no-network to force the offline-only pass.

There is deliberately NO whole-fleet release-completeness sweep. The earlier
--require-complete-asset-set walked all 138 tags to catch auvik-v0.1.1's 16-of-25
assets, but nothing pins auvik-v0.1.1, so no published link is broken by it: it
cost 138 API calls to report something with no user-facing surface. This gate
checks what the repo actually PUBLISHES.

Exit 0 = clean, 1 = violations (printed), 2 = environment error (including a
PINS_INFLIGHT_HOURS that is not a finite number >= 0).

Run locally:  python3 tools/maintainer/check_pinned_artifacts.py
Self-test:    python3 tools/maintainer/check_pinned_artifacts.py --self-test
              (every rule above, both directions, in throwaway git repos)
"""

from __future__ import annotations

import argparse
import json
import math
import os
import re
import subprocess
import sys
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)
import check_release_pipeline  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT

# The tag-time probe. Every `git tag` this script hands an operator is printed
# behind it, so the command is checked when it is PASTED rather than when it was
# printed: a reminder read out of a week-old scrollback still refuses a commit
# that would seal an empty immutable release.
PROBE_REL = "tools/maintainer/check_release_pipeline.py"


def tag_command(tag: str, sha: str) -> str:
    """The only shape of tag command this script prints: probe first, `&&` tag."""
    return (f"python3 {PROBE_REL} --tag {tag} --sha {sha} "
            f"&& git tag {tag} {sha} && git push origin {tag}")

# A literal pinned release URL. Templated ones (install.sh / verify_all.sh build
# theirs from shell variables) carry a $ or a { and are skipped: there is no pin
# to resolve until runtime.
PIN_RE = re.compile(
    r"https://github\.com/(?P<owner>[A-Za-z0-9._-]+)/(?P<repo>[A-Za-z0-9._-]+)"
    r"/releases/(?P<kind>download|tag)/(?P<rest>[^\s)\"'<>\]]+)"
)
TAG_RE = re.compile(r"^(?P<slug>[a-z0-9][a-z0-9.-]*)-v(?P<version>\d+\.\d+\.\d+)$")
ZERO_SHA = "0" * 40

# How long a release that has LANDED on main may wait for its tag before its
# pins count as never cut. See the IN FLIGHT bullet in the module docstring.
INFLIGHT_ENV = "PINS_INFLIGHT_HOURS"
INFLIGHT_DEFAULT_HOURS = 24.0
# A bump commit dated further than this past the run's clock earns no window:
# a future date would otherwise stretch the window by however far ahead it is.
# A small skew only delays the window by that much, then it opens normally.
CLOCK_SKEW_SECONDS = 15 * 60


def inflight_window_hours(environ: dict | None = None) -> float:
    """PINS_INFLIGHT_HOURS as a number of hours. Unset or blank is the default;
    anything that is not a finite number >= 0 raises ValueError (exit 2), since
    a typo must not silently become "no window" or "a window forever"."""
    raw = (os.environ if environ is None else environ).get(INFLIGHT_ENV, "")
    if not raw.strip():
        return INFLIGHT_DEFAULT_HOURS
    try:
        hours = float(raw)
    except ValueError:
        raise ValueError(f"{INFLIGHT_ENV}={raw!r} is not a number of hours") from None
    if not math.isfinite(hours) or hours < 0:
        raise ValueError(
            f"{INFLIGHT_ENV}={raw!r} must be a finite number of hours >= 0 "
            "(0 turns the in-flight window off)"
        )
    return hours


class Pin:
    """One literal pinned release URL found in one tracked file."""

    def __init__(self, path: str, url: str, kind: str, tag: str, asset: str | None):
        self.path = path
        self.url = url
        self.kind = kind
        self.tag = tag
        self.asset = asset

    @property
    def where(self) -> str:
        return f"{self.path}: {self.tag}" + (f"/{self.asset}" if self.asset else "")


def git(*args: str) -> subprocess.CompletedProcess:
    return subprocess.run(
        ["git", "-C", str(ROOT), *args], capture_output=True, text=True
    )


def version_tuple(version: str) -> tuple[int, ...]:
    return tuple(int(part) for part in version.split("."))


def parse_pins(path: str, text: str, owner: str, repo: str) -> list[Pin]:
    """Every literal pin in `text` that targets THIS repo's releases."""
    pins: list[Pin] = []
    for m in PIN_RE.finditer(text):
        url = m.group(0)
        if "$" in url or "{" in url or "}" in url:
            continue  # runtime-templated, not a pin
        if m.group("owner").lower() != owner.lower() or m.group("repo").lower() != repo.lower():
            continue  # a third-party release (gitleaks, the press library)
        rest = m.group("rest").rstrip(".,;")
        if m.group("kind") == "download":
            tag, _, asset = rest.partition("/")
            pins.append(Pin(path, url, "download", tag, asset or None))
        else:
            pins.append(Pin(path, url, "tag", rest, None))
    return pins


def collect_pins(owner: str, repo: str) -> list[Pin]:
    p = git("ls-files", "-z")
    if p.returncode != 0:
        print(f"check_pinned_artifacts: cannot list tracked files: {p.stderr.strip()}")
        sys.exit(2)
    pins: list[Pin] = []
    for rel in p.stdout.split("\0"):
        if not rel:
            continue
        f = ROOT / rel
        try:
            text = f.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue  # binary or unreadable: no pins to find
        if "/releases/" not in text:
            continue
        pins.extend(parse_pins(rel, text, owner, repo))
    return pins


def local_tags() -> set[str]:
    p = git("tag")
    return set(p.stdout.split()) if p.returncode == 0 else set()


def base_is_reachable(base: str | None) -> bool:
    if not base or base == ZERO_SHA:
        return False
    return git("rev-parse", "--verify", "--quiet", f"{base}^{{commit}}").returncode == 0


def rename_map(base: str) -> dict[str, str]:
    """current path -> the path it was renamed FROM between BASE and the tree.

    Used for the per-slug manifest-version lookup, which is path-addressed by
    nature (`skills/<dir>/manifest.json`) and so still needs to know where that
    file lived at BASE. Without it, moving a skill directory would make its
    manifest read as absent at BASE - the brand-new-skill case - and hand a
    second uncut bump the grace the unbounded-grace rule exists to deny. The
    pin lookup itself no longer needs renames: it is repo-wide.

    -z is used because a path may contain a space or a quote: with it, git emits
    STATUS NUL PATH NUL for ordinary changes and STATUS NUL OLD NUL NEW NUL for
    a rename or copy, with no shell-style quoting to unescape.
    """
    p = git("diff", "--find-renames", "--name-status", "-z", base, "--")
    if p.returncode != 0:
        return {}
    fields = p.stdout.split("\0")
    out: dict[str, str] = {}
    i = 0
    while i < len(fields):
        status = fields[i]
        if not status:
            i += 1
            continue
        if status[0] in ("R", "C") and i + 2 < len(fields):
            out[fields[i + 2]] = fields[i + 1]
            i += 3
        else:
            i += 2
    return out


class BaseUnavailable(Exception):
    """The BASE pin set could not be computed, so the in-flight grace is unsafe."""


def pins_at_base(base: str, owner: str, repo: str) -> set[str]:
    """Every pin URL the repo carried at BASE, across ALL tracked files.

    Repo-WIDE, not per-path, and that scope is the whole point. The lookup used
    to ask "did THIS file carry this URL at BASE?", which made a cross-file MOVE
    launder a dead pin: cut it out of `old.md`, paste it into an already-existing
    `new.md`, and it is absent from `new.md` at BASE, so it read as introduced by
    the change-set and collected a green in-flight reminder even though the tag
    had been missing for months. Following renames did not help - no file was
    renamed. Asking the repo-wide question instead means a pin counts as
    introduced only when it exists nowhere at BASE, which is exactly what a real
    version bump produces and exactly what a move does not.
    """
    listing = git("grep", "-I", "-l", "-F", "-e", "/releases/", base)
    if listing.returncode > 1:
        # >1 is a real git failure (bad object, unreadable tree), not "no match".
        # Returning an empty set here would make every pin in the repo look newly
        # introduced and collect the in-flight grace - the entire gate off, with
        # nothing said. Signal "unknown" so the caller withholds the grace.
        raise BaseUnavailable(
            f"git grep failed at {base!r} (exit {listing.returncode}): "
            f"{(listing.stderr or '').strip()[:200]}"
        )
    if listing.returncode == 1:
        return set()  # genuinely no pins at BASE
    urls: set[str] = set()
    for line in listing.stdout.split("\n"):
        # `git grep -l <pattern> <rev>` prints "<rev>:<path>".
        _, sep, path = line.partition(":")
        if not sep or not path:
            continue
        blob = git("show", f"{base}:{path}")
        if blob.returncode != 0:
            continue
        urls |= {pin.url for pin in parse_pins(path, blob.stdout, owner, repo)}
    return urls


def manifest_version(slug: str) -> str | None:
    mf = registry.skill_path(slug) / "manifest.json"
    try:
        return json.loads(mf.read_text(encoding="utf-8")).get("version")
    except (OSError, ValueError):
        return None


def manifest_path_at_base(slug: str, renames: dict[str, str]) -> str:
    """Where skills/<dir>/manifest.json lived at BASE (following a rename in the
    change-set, so a moved skill directory still finds its BASE manifest)."""
    path = f"skills/{registry.source_dir(slug)}/manifest.json"
    return renames.get(path, path)


def manifest_version_at_base(
    base: str, slug: str, renames: dict[str, str]
) -> str | None:
    """The version this slug shipped at BASE, or None if it had no manifest then
    (a brand-new skill, whose first release is legitimately in flight)."""
    p = git("show", f"{base}:{manifest_path_at_base(slug, renames)}")
    if p.returncode != 0:
        return None
    try:
        return json.loads(p.stdout).get("version")
    except ValueError:
        return None


def _version_at(rev: str, path: str) -> tuple[bool, str | None]:
    """(the manifest exists at rev, its version or None if unreadable)."""
    p = git("show", f"{rev}:{path}")
    if p.returncode != 0:
        return False, None
    try:
        version = json.loads(p.stdout).get("version")
    except (ValueError, AttributeError):
        return True, None
    return True, version if isinstance(version, str) else None


class Bump:
    """The commit at which a manifest's version became the pinned version."""

    def __init__(self, sha: str, when: int, prior: str | None, prior_exists: bool):
        self.sha = sha
        self.when = when            # committer date, Unix seconds
        self.prior = prior          # the version just before it, if readable
        self.prior_exists = prior_exists


def bump_commit(base: str, path: str, version: str) -> Bump | None:
    """The newest commit on BASE's first-parent history that changed `path`'s
    version TO `version`, or None when there is none to be found.

    Only BASE's history is read, so a change-set's own commits can never date
    it. Only FIRST-PARENT commits are walked, so a branch merged into main is
    dated by its merge commit (when it landed), never by the committer date its
    author wrote on the branch commit. And it looks for the VERSION changing,
    not the file: a commit that edits another field of a manifest whose version
    is unchanged is skipped, so it cannot reopen the window of an old bump.
    """
    # --no-show-signature: a log.showSignature config would print gpg lines
    # into this output, which would read as unparseable and close the window.
    log = git("log", "--first-parent", "--no-show-signature", "--format=%H %ct",
              base, "--", path)
    if log.returncode != 0:
        return None
    for line in log.stdout.splitlines():
        sha, _, when = line.partition(" ")
        if not sha or not when.strip().isdigit():
            return None  # unparseable log: no window rather than a guessed one
        _, at = _version_at(sha, path)
        if at != version:
            # Walked out of BASE's run of this version without seeing it begin.
            return None
        prior_exists, prior = _version_at(f"{sha}^1", path)
        if prior != version:
            return Bump(sha, int(when), prior, prior_exists)
    return None


REGISTRY_REL = "tools/maintainer/skills.json"
BURNED_REL = "tools/maintainer/burned_versions.json"


def source_dirs_at_base(base: str) -> dict[str, str]:
    """slug -> its skills/<dir> name, from the registry AT BASE.

    The in-flight window decides on BASE's history, so it must also find the
    manifest through BASE's layout. The tree's registry is the change-set's: a
    pull request that repointed a dead slug's source_dir at another directory
    on main, recently bumped to the same version number, would otherwise date
    the dead pin by that other directory's bump. Raises BaseUnavailable when
    the registry cannot be read at BASE, which withholds the window.
    """
    p = git("show", f"{base}:{REGISTRY_REL}")
    if p.returncode != 0:
        raise BaseUnavailable(f"{REGISTRY_REL} cannot be read at BASE")
    try:
        skills_map = json.loads(p.stdout).get("skills")
    except (ValueError, AttributeError):
        skills_map = None
    if not isinstance(skills_map, dict):
        raise BaseUnavailable(f"{REGISTRY_REL} at BASE has no 'skills' object")
    out: dict[str, str] = {}
    for slug, entry in skills_map.items():
        d = entry.get("source_dir", slug) if isinstance(entry, dict) else None
        if isinstance(d, str) and registry.SOURCE_DIR_RE.match(d):
            out[slug] = d
    return out


def burned_at_base(base: str) -> dict[str, str]:
    """The retired-version ledger AT BASE ({} when it did not exist there).

    The window reads it from BASE for the same reason it reads the registry
    there: a pull request could otherwise delete a retirement to make a pin at a
    retired version eligible, or add one to excuse an uncut prior. Raises
    ProbeError when the ledger exists at BASE but cannot be read.
    """
    if git("cat-file", "-e", f"{base}:{BURNED_REL}").returncode != 0:
        return {}
    p = git("show", f"{base}:{BURNED_REL}")
    if p.returncode != 0:
        raise check_release_pipeline.ProbeError(f"{BURNED_REL} cannot be read at BASE")
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        ledger = Path(td) / "burned_versions.json"
        ledger.write_text(p.stdout, encoding="utf-8")
        return check_release_pipeline.load_burned(ledger)


def is_shallow() -> bool:
    """A shallow checkout cuts history short, so a bump cannot be dated. Any
    answer but an explicit "false" is treated as shallow (no window)."""
    p = git("rev-parse", "--is-shallow-repository")
    return p.returncode != 0 or p.stdout.strip() != "false"


def highest_tag_version(slug: str, tags: set[str]) -> tuple[int, ...] | None:
    versions = []
    for tag in tags:
        m = TAG_RE.match(tag)
        if m and m.group("slug") == slug:
            versions.append(version_tuple(m.group("version")))
    return max(versions) if versions else None


def expected_assets(slug: str, meta: dict) -> set[str]:
    """The complete asset set a release for this slug ships: the release
    workflow's binaries, their .sha256 sidecars, and the .mcpb bundle."""
    assets: set[str] = set()
    # Names come from registry.asset_map - the same function release_matrix.py
    # feeds into the build matrix release.yml uploads from - so this gate can
    # never expect a filename the release does not actually produce.
    for per_target in registry.asset_map(meta["cli_binary"], meta["mcp_binary"]).values():
        for name in per_target.values():
            assets.add(name)
            assets.add(name + ".sha256")
    assets.add(f"{meta['mcp_binary']}.mcpb")
    return assets


def owning_slug(path: str) -> str | None:
    """The registry slug that owns skills/<dir>/... , or None outside skills/."""
    parts = Path(path).parts
    if len(parts) >= 2 and parts[0] == "skills":
        return registry.slug_for_dir(parts[1])
    return None


def check_offline(pins, meta, tags, base, strict, owner, repo,
                  now: float | None = None,
                  window_hours: float = INFLIGHT_DEFAULT_HOURS):
    errors: list[str] = []
    notices: list[str] = []
    reminders: list[str] = []
    now = time.time() if now is None else now

    head = git("rev-parse", "HEAD").stdout.strip() or "HEAD"
    base_ok = base_is_reachable(base)
    base_pins: set[str] | None = None
    base_pins_unavailable = False   # repo-wide, computed on first need
    renames = rename_map(base) if base_ok else {}
    base_version_cache: dict[str, str | None] = {}

    # Version numbers a destroyed release already spent. A BASE version that is
    # BURNED was not "a release nobody bothered to cut" - it was cut, sealed
    # empty and withdrawn - so it must not consume the one push of grace below.
    # An unreadable ledger reads as EMPTY here, which withholds the exemption
    # rather than granting it: this file may only ever make the grace narrower.
    burned_reported: set[str] = set()
    try:
        burned_tags = check_release_pipeline.load_burned(ROOT / BURNED_REL)
    except check_release_pipeline.ProbeError as exc:
        burned_tags = {}
        notices.append(
            f"the retired-version ledger could not be read ({exc}); a burned "
            "version will be treated as an uncut one, which can only make the "
            "in-flight grace stricter"
        )

    def prior_version(slug: str) -> str | None:
        if slug not in base_version_cache:
            base_version_cache[slug] = manifest_version_at_base(base, slug, renames)
        return base_version_cache[slug]

    def note_burned_prior(slug: str, prior: str) -> None:
        # The prior version is not an uncut release, it is a RETIRED one: its
        # tag was cut, its release sealed empty and both were deleted. Telling
        # anyone to "cut it first" is the one instruction that must never be
        # given, so say so rather than staying silent. Once per slug.
        if slug in burned_reported:
            return
        burned_reported.add(slug)
        notices.append(
            f"{slug} skipped {prior}: that number is retired in "
            f"tools/maintainer/burned_versions.json and must never be cut "
            f"again. It does not consume this release's grace."
        )

    # The IN FLIGHT window for a pin that was already on main at BASE (see the
    # module docstring). One answer per tag: README.md and server.json pin the
    # same release, and the answer depends only on the tag.
    inflight_cache: dict[str, tuple[str | None, str]] = {}
    shallow: list[bool] = []
    at_base_state: dict = {}

    def base_state() -> dict:
        """BASE's registry layout and retired-version ledger, read once."""
        if not at_base_state:
            try:
                at_base_state["dirs"] = source_dirs_at_base(base)
                at_base_state["burned"] = burned_at_base(base)
                at_base_state["error"] = ""
            except (BaseUnavailable, check_release_pipeline.ProbeError) as exc:
                at_base_state["error"] = str(exc)
        return at_base_state

    def inflight(slug: str, version: str) -> tuple[str | None, str]:
        """(commit to tag, why it is in flight), or (None, why it is not)."""
        tag = f"{slug}-v{version}"
        if tag not in inflight_cache:
            inflight_cache[tag] = _inflight(slug, version, tag)
        return inflight_cache[tag]

    def _inflight(slug: str, version: str, tag: str) -> tuple[str | None, str]:
        if window_hours <= 0:
            return None, f"the in-flight window is off ({INFLIGHT_ENV}=0)"
        if not shallow:
            shallow.append(is_shallow())
        if shallow[0]:
            return None, "this checkout is shallow, so the bump that made it cannot be dated"
        # Everything below is decided on BASE's state, never the change-set's:
        # BASE's registry finds the manifest, BASE's history dates the bump and
        # BASE's ledger is the only one that can excuse an uncut prior.
        state = base_state()
        if state["error"]:
            return None, f"BASE's registry or retired-version ledger is unreadable ({state['error']})"
        base_dir = state["dirs"].get(slug)
        if base_dir is None:
            return None, f"{slug} is not in the registry at BASE"
        path = f"skills/{base_dir}/manifest.json"
        mv = manifest_version(slug)
        if mv != version:
            return None, (f"skills/{registry.source_dir(slug)}/manifest.json now says "
                          f"{mv!r}, so {version} is not the release in flight")
        _, at_base = _version_at(base, path)
        if at_base != version:
            return None, (f"{path} said {at_base!r} at BASE, so {version} "
                          f"was not the release in flight there")
        if tag in state["burned"] or tag in burned_tags:
            # Retired at BASE or in the tree: either one denies.
            return None, (f"{tag} is retired in {BURNED_REL} and can never be cut")
        highest = highest_tag_version(slug, tags)
        if highest is not None and version_tuple(version) <= highest:
            return None, (f"a {slug} tag at or above it already exists "
                          f"({'.'.join(str(n) for n in highest)})")
        bump = bump_commit(base, path, version)
        if bump is None:
            return None, (f"no commit on BASE's first-parent history changed "
                          f"{path} to version {version}")
        age = now - bump.when
        hours = age / 3600
        short = bump.sha[:12]
        if age < -CLOCK_SKEW_SECONDS:
            return None, (f"{short}, the commit that bumped it, is dated "
                          f"{-hours:.1f}h in the future")
        if age >= window_hours * 3600:
            return None, (f"{short} bumped it to {version} {hours:.1f}h ago, past "
                          f"the {window_hours:g}h in-flight window ({INFLIGHT_ENV})")
        if bump.prior is None:
            if highest is not None:
                return None, (f"{short} created {path} while {slug} tags already "
                              "existed (a moved or re-created skill), so the release "
                              "before it cannot be read")
        else:
            prior_tag = f"{slug}-v{bump.prior}"
            if prior_tag in state["burned"]:
                note_burned_prior(slug, bump.prior)
            elif prior_tag not in tags:
                return None, (f"the version before that bump ({bump.prior}) was never "
                              "tagged either: a second bump with no tag cut gets no window")
        return bump.sha, (f"{short} bumped it to {version} {max(hours, 0.0):.1f}h ago, "
                          f"inside the {window_hours:g}h in-flight window")

    if not tags:
        notices.append(
            "no local tags in this checkout (shallow clone?) - tag-existence check "
            "SKIPPED; fetch tags (actions/checkout fetch-depth: 0) to enable it"
        )
    elif not base_ok:
        notices.append(
            f"BASE {base!r} unreachable - tag-existence check SKIPPED (an in-flight "
            "release cannot be told apart from a never-cut tag without it)"
        )

    for pin in pins:
        m = TAG_RE.match(pin.tag)
        if not m:
            errors.append(f"{pin.where} - pinned tag is not '<slug>-v<major.minor.patch>'")
            continue
        slug, version = m.group("slug"), m.group("version")
        if slug not in meta:
            errors.append(f"{pin.where} - pins slug '{slug}', which is not in skills.json")
            continue
        entry = meta[slug]

        # 1. The pin must belong to the skill whose directory carries it.
        owner_slug = owning_slug(pin.path)
        if owner_slug is not None and owner_slug != slug:
            errors.append(
                f"{pin.where} - lives under skills/{Path(pin.path).parts[1]}/ "
                f"(slug '{owner_slug}') but pins '{slug}'"
            )
            continue

        # 2. The asset name must match its class.
        if pin.asset:
            if pin.asset.endswith(".mcpb"):
                want = f"{entry['mcp_binary']}.mcpb"
                if pin.asset != want:
                    errors.append(f"{pin.where} - .mcpb asset must be named {want!r}")
                    continue
            elif pin.asset not in expected_assets(slug, entry):
                errors.append(
                    f"{pin.where} - asset is not one the release workflow produces "
                    f"for {slug}"
                )
                continue

        # 3. Existence, with the diff-scoped escape hatch.
        if tags and base_ok and pin.tag not in tags:
            if base_pins is None:
                try:
                    base_pins = pins_at_base(base, owner, repo)
                except BaseUnavailable as exc:
                    # The BASE pin set is unknown, so "this change-set introduced
                    # the pin" cannot be established. Withhold the grace and say
                    # why, rather than granting it to every pin in the repo.
                    notices.append(
                        f"the BASE pin set could not be computed ({exc}); the in-flight-release "
                        f"grace is WITHHELD, so a pin at an uncut tag fails until BASE is readable"
                    )
                    base_pins = set()
                    base_pins_unavailable = True
            if base_pins_unavailable or pin.url in base_pins:
                # Already on main. Still IN FLIGHT if its release commit landed
                # inside the window; never when the BASE pin set is unknown.
                why_not = ""
                if not base_pins_unavailable:
                    sha, why = inflight(slug, version)
                    if sha is not None:
                        reminders.append(
                            f"{pin.where} - in-flight release already on main ({why}); "
                            f"tag not cut yet: {tag_command(pin.tag, sha)}"
                        )
                        continue
                    why_not = f" Not in flight: {why}."
                errors.append(
                    f"{pin.where} - tag does not exist and this URL was already in the "
                    f"tree at BASE (in this file or another one): the {slug} release was "
                    f"never cut, so this URL 404s.{why_not} Cut it "
                    f"(`{tag_command(pin.tag, '<sha>')}`) or repoint "
                    "the pin at a released version."
                )
                continue
            # New or moved pin: an in-flight release. Allow, but only if it is
            # THIS skill's next version, not an arbitrary invented tag.
            mv = manifest_version(slug)
            if mv != version:
                errors.append(
                    f"{pin.where} - new pin for an uncut tag, but skills/{slug}/manifest.json "
                    f"says version {mv!r}; an in-flight release must pin "
                    f"'{slug}-v{mv}'"
                )
                continue
            highest = highest_tag_version(slug, tags)
            if highest is not None and version_tuple(version) <= highest:
                errors.append(
                    f"{pin.where} - new pin for an uncut tag whose version does not sort "
                    f"above the highest existing {slug} tag "
                    f"({'.'.join(str(n) for n in highest)})"
                )
                continue
            # Grace is for ONE in-flight release, not a renewable subscription.
            # Set-membership of the pin URL is renewed by every version bump, so
            # without this three consecutive bumps with zero tags cut would all
            # stay green. The release BEFORE this one must actually have been
            # cut. A slug with no manifest at BASE is a brand-new skill whose
            # first release is legitimately in flight.
            prior = prior_version(slug)
            prior_burned = prior is not None and f"{slug}-v{prior}" in burned_tags
            if prior is not None and f"{slug}-v{prior}" not in tags and not prior_burned:
                errors.append(
                    f"{pin.where} - new pin for an uncut tag, but the version {slug} "
                    f"carried at BASE ({prior}) was never tagged either. An in-flight "
                    f"release gets one push of grace; this is the second bump with no "
                    f"tag cut. Cut {slug}-v{prior} first, or revert the bump."
                )
                continue
            if prior_burned:
                note_burned_prior(slug, prior)
            reminders.append(
                f"{pin.where} - in-flight release, tag not cut yet. After this lands: "
                f"{tag_command(pin.tag, head)}"
            )

        # 4. Freshness: the registry has moved on and the newer tag already exists.
        reg_version = entry.get("version")
        if (
            reg_version
            and reg_version != version
            and f"{slug}-v{reg_version}" in tags
        ):
            msg = (
                f"{pin.where} - stale: {slug} ships {reg_version} and the tag "
                f"{slug}-v{reg_version} exists. Run "
                "`python3 tools/maintainer/build-catalog.py` and commit."
            )
            (errors if strict else notices).append(msg)

    return errors, notices, reminders


# A per-tag `gh api` call that failed for a reason that is NOT "this release
# does not exist" - a 502, a secondary rate limit, a DNS blip, a timeout. It is
# deliberately distinct from None: None means GitHub answered, authoritatively,
# that there is no release for the tag.
TRANSIENT = object()

# gh renders an HTTP error as e.g. `gh: Not Found (HTTP 404)` on stderr. Only a
# 404 (and 410, a release deleted out from under a pin) is an authoritative
# "no such release"; everything else - and any failure with no HTTP status at
# all, which is what a DNS failure or a timeout looks like - is transport.
RE_GH_STATUS = re.compile(r"\(HTTP (\d{3})\)")
DEFINITIVE_MISSING = {"404", "410"}


def gh_error_is_definitive(proc: subprocess.CompletedProcess) -> bool:
    """True only when GitHub itself said the release is not there."""
    m = RE_GH_STATUS.search((proc.stderr or "") + (proc.stdout or ""))
    return bool(m) and m.group(1) in DEFINITIVE_MISSING


def gh_release(
    owner: str, repo: str, tag: str | None, cache: dict, preflight: bool = False
):
    """One `gh api` call per DISTINCT tag, cached.

    Returns the release JSON, None when GitHub authoritatively answered 404/410,
    or TRANSIENT when the call failed for any other reason. That third state is
    load-bearing: every non-zero result used to read as "no published release",
    so a 502 or a secondary rate limit produced a false-RED telling a maintainer
    to cut a release that already exists. A gate that fires on a healthy repo is
    as harmful as one that misses the defect - it teaches maintainers to ignore
    it - so a transport failure now degrades to a NOTE, exactly like the
    preflight already does for the whole pass.

    With preflight=True it asks for the repo itself instead, to tell "GitHub
    unreachable" apart from "this tag has no release".
    """
    if not preflight and tag in cache:
        return cache[tag]
    path = f"repos/{owner}/{repo}" if preflight else f"repos/{owner}/{repo}/releases/tags/{tag}"
    p = _run(["gh", "api", path])
    if preflight:
        if p.returncode != 0:
            return None
        try:
            return json.loads(p.stdout)
        except json.JSONDecodeError:
            return None
    if p.returncode != 0:
        cache[tag] = None if gh_error_is_definitive(p) else TRANSIENT
    else:
        try:
            cache[tag] = json.loads(p.stdout)
        except json.JSONDecodeError:
            # A 200 whose body is not JSON is not evidence of a missing release.
            cache[tag] = TRANSIENT
    return cache[tag]


def _run(cmd: list[str]) -> subprocess.CompletedProcess:
    """subprocess.run that reports a missing executable as a non-zero result
    instead of raising - an absent `gh` is a normal, expected environment here."""
    try:
        return subprocess.run(cmd, capture_output=True, text=True)
    except OSError as exc:
        return subprocess.CompletedProcess(cmd, 127, "", str(exc))


def gh_usable() -> tuple[bool, str]:
    """Can `gh api` be expected to work here? Answered WITHOUT a network call so
    the common "no gh, no token" case costs nothing."""
    if _run(["gh", "--version"]).returncode != 0:
        return False, "the gh CLI is not on PATH"
    if os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN"):
        return True, ""
    t = _run(["gh", "auth", "token"])
    if t.returncode != 0 or not t.stdout.strip():
        return False, "the gh CLI is not authenticated (gh auth login, or set GH_TOKEN)"
    return True, ""


def check_network(pins, tags, owner, repo):
    """Assert every pinned ASSET is really attached to its release.

    Scoped to pins whose tag EXISTS locally, so this pass reports only what the
    offline pass cannot see. A pin at an uncut tag is already the offline pass's
    finding, with a better message, and repeating it here would bury the one
    thing only the network knows. (When the checkout carries no tags at all the
    offline existence check has already SKIPPED, so every pin is checked here
    instead of nothing being checked at all.)

    Returns (errors, notices). Reaching GitHub is preflighted once: if that
    fails, the whole pass degrades to a notice instead of reporting every tag as
    missing, because a network outage must never look like a broken repo.
    """
    cache: dict[str, dict | None] = {}
    if gh_release(owner, repo, None, cache, preflight=True) is None:
        return [], [
            f"gh cannot reach repos/{owner}/{repo} - pinned-ASSET existence check "
            "SKIPPED (network, token scope, or rate limit). Re-run when GitHub is "
            "reachable, or pass --network to make this a hard error."
        ]

    errors: list[str] = []
    notices: list[str] = []
    transient: set[str] = set()
    # Tag existence alone passes datto-rmm-v0.1.2: the release object is green
    # with 24 assets because its mcp-publish run was CANCELLED and the .mcpb
    # never uploaded. Only the asset LIST can see that.
    for pin in pins:
        if not TAG_RE.match(pin.tag):
            continue  # already reported by the offline pass
        if tags and pin.tag not in tags:
            continue  # an uncut tag is the offline pass's finding, not this one
        rel = gh_release(owner, repo, pin.tag, cache)
        if rel is TRANSIENT:
            transient.add(pin.tag)
            continue
        if rel is None:
            errors.append(f"{pin.where} - no published release for tag {pin.tag}")
            continue
        names = {a.get("name") for a in rel.get("assets", [])}
        if pin.asset and pin.asset not in names:
            errors.append(
                f"{pin.where} - the release exists but has no asset named "
                f"{pin.asset!r} ({len(names)} assets present). A cancelled or failed "
                "mcp-publish/release run leaves the tag green and the URL dead."
            )
    if transient:
        notices.append(
            "gh could not read " + str(len(transient)) + " release(s) for a reason "
            "GitHub did not report as 404 (transport error, 5xx, or a secondary rate "
            "limit): " + ", ".join(sorted(transient)) + ". The pinned-ASSET check is "
            "SKIPPED for those tags rather than reporting a release that exists as "
            "missing. Re-run when GitHub is reachable."
        )
    return errors, notices


# --------------------------------------------------------------------------
# Self-test: every rule in the docstring, both directions, in throwaway repos.
# --------------------------------------------------------------------------
_FIX_OWNER, _FIX_REPO = "servosity", "msp-skills"
_HOUR = 3600.0
_DAY_H = 24.0


class _Fixture:
    """A throwaway git repo laid out like this one: a registry, one skill with
    a manifest and pinned release URLs, tags, and a retired-version ledger.
    Commit dates are set explicitly, relative to a fixed `now`."""

    def __init__(self, root: Path, now: float, init: bool = True):
        self.root = root
        self.now = now
        if init:
            self.run("init", "-q", "-b", "main")
            self.registry()
            self.burned()

    def run(self, *args: str, age_h: float | None = None) -> subprocess.CompletedProcess:
        env = {k: v for k, v in os.environ.items() if not k.startswith("GIT_")}
        env.update(
            GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
            GIT_AUTHOR_NAME="gate", GIT_AUTHOR_EMAIL="gate@example.test",
            GIT_COMMITTER_NAME="gate", GIT_COMMITTER_EMAIL="gate@example.test",
        )
        if age_h is not None:
            stamp = f"@{int(self.now - age_h * _HOUR)} +0000"
            env.update(GIT_AUTHOR_DATE=stamp, GIT_COMMITTER_DATE=stamp)
        p = subprocess.run(["git", "-C", str(self.root), *args],
                           capture_output=True, text=True, env=env)
        if p.returncode != 0:
            raise RuntimeError(f"fixture git {' '.join(args)}: {p.stderr.strip()}")
        return p

    def write(self, rel: str, text: str) -> None:
        f = self.root / rel
        f.parent.mkdir(parents=True, exist_ok=True)
        f.write_text(text, encoding="utf-8")

    def registry(self, source_dir: str | None = None) -> None:
        entry = {"cli_binary": "alpha-cli", "mcp_binary": "alpha-mcp"}
        if source_dir:
            entry["source_dir"] = source_dir
        self.write("tools/maintainer/skills.json", json.dumps(
            {"owner": _FIX_OWNER, "repo": _FIX_REPO, "skills": {"alpha": entry}}) + "\n")

    def burned(self, *tags: str) -> None:
        self.write("tools/maintainer/burned_versions.json", json.dumps(
            {"burned": [{"tag": t, "why": "sealed empty, withdrawn"} for t in tags]}) + "\n")

    @staticmethod
    def url(version: str) -> str:
        return (f"https://github.com/{_FIX_OWNER}/{_FIX_REPO}/releases/download/"
                f"alpha-v{version}/alpha-mcp.mcpb")

    def release(self, version: str, d: str = "alpha", note: str = "x") -> None:
        """What release.py stamps: the manifest version and the pins together."""
        self.write(f"skills/{d}/manifest.json",
                   json.dumps({"name": "alpha", "version": version, "note": note}) + "\n")
        self.write(f"skills/{d}/server.json",
                   json.dumps({"packages": [{"identifier": self.url(version)}]}) + "\n")

    def commit(self, msg: str, age_h: float) -> str:
        self.run("add", "-A")
        self.run("commit", "-q", "--allow-empty", "-m", msg, age_h=age_h)
        return self.run("rev-parse", "HEAD").stdout.strip()

    def tag(self, name: str, rev: str = "HEAD") -> None:
        self.run("tag", name, rev)

    def history(self, bump_age_h: float) -> tuple[str, str]:
        """alpha-v0.1.1 cut long ago; 0.1.2 bumped `bump_age_h` ago and never
        cut; then an unrelated commit, which is BASE. Returns (bump, BASE)."""
        self.release("0.1.1")
        self.commit("alpha 0.1.1", 200 * _DAY_H)
        self.tag("alpha-v0.1.1")
        self.release("0.1.2")
        bump = self.commit("release alpha 0.1.2", bump_age_h)
        self.write("docs/notes.md", "unrelated\n")
        base = self.commit("docs: unrelated", 0.5)
        return bump, base

    def tools_only_change(self) -> None:
        """The pull request under test changes nothing but a tool."""
        self.write("tools/maintainer/other.py", "print('unrelated')\n")
        self.run("add", "-A")


def _self_test() -> int:
    import contextlib
    import io
    import tempfile

    g = globals()
    saved = {
        "ROOT": g["ROOT"], "pins_at_base": g["pins_at_base"],
        "registry": (registry.ROOT, registry.SKILLS_DIR, registry.REGISTRY),
    }
    # A hook (pre-push) can export GIT_DIR and friends, which would point every
    # fixture command at the real repository. Clear them for the duration.
    git_env = {k: os.environ.pop(k) for k in list(os.environ) if k.startswith("GIT_")}
    failures: list[str] = []
    passed: list[str] = []
    now = float(int(time.time()))

    def must(cond: bool, msg: str) -> None:
        if not cond:
            raise AssertionError(msg)

    def head(fx: _Fixture) -> str:
        return fx.run("rev-parse", "HEAD").stdout.strip()

    def evaluate(fx: _Fixture, base: str, window: float = INFLIGHT_DEFAULT_HOURS):
        g["ROOT"] = fx.root
        registry.ROOT = fx.root
        registry.SKILLS_DIR = fx.root / "skills"
        registry.REGISTRY = fx.root / "tools" / "maintainer" / "skills.json"
        owner, repo = registry.owner_repo()
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf):
            return check_offline(collect_pins(owner, repo), registry.skills(),
                                 local_tags(), base, False, owner, repo,
                                 now=fx.now, window_hours=window)

    def expect(label: str, build, *, fail: bool, want: str = "",
               notice: str = "", reminder_none: bool = False) -> None:
        with tempfile.TemporaryDirectory() as td:
            fx = _Fixture(Path(td), now)
            try:
                errors, notices, reminders = build(fx)
            except AssertionError as exc:
                failures.append(f"{label}: {exc}")
                return
            except Exception as exc:  # noqa: BLE001 - report, keep going
                failures.append(f"{label}: fixture raised {type(exc).__name__}: {exc}")
                return
            finally:
                g["pins_at_base"] = saved["pins_at_base"]
        body = errors if fail else reminders
        problems = []
        if fail and not errors:
            problems.append("expected FAIL, got no errors")
        if not fail and errors:
            problems.append("expected PASS, got errors")
        if want and not any(want in line for line in body):
            problems.append(f"no {'error' if fail else 'reminder'} contains {want!r}")
        if notice and not any(notice in n for n in notices):
            problems.append(f"no notice contains {notice!r}")
        if reminder_none and reminders:
            problems.append("expected no reminders")
        if problems:
            failures.append(f"{label}: {'; '.join(problems)}\n      errors={errors}\n"
                            f"      reminders={reminders}\n      notices={notices}")
        else:
            passed.append(label)

    try:
        at_base = "tag does not exist and this URL was already in the tree at BASE"

        def wave(bump_age_h: float, window: float = INFLIGHT_DEFAULT_HOURS):
            """A tools-only pull request opened while alpha 0.1.2, bumped
            `bump_age_h` ago, has no tag yet."""
            def build(fx):
                _, base = fx.history(bump_age_h)
                fx.tools_only_change()
                return evaluate(fx, base, window)
            return build

        # ---- Existing protections (behavior from before the in-flight window) ----

        def tag_cut(fx):
            fx.release("0.1.1")
            base = fx.commit("alpha 0.1.1", 200 * _DAY_H)
            fx.tag("alpha-v0.1.1")
            fx.tools_only_change()
            return evaluate(fx, base)
        expect("E1 tag cut: OK, nothing to remind", tag_cut, fail=False, reminder_none=True)

        def introduced(fx):
            fx.release("0.1.1")
            base = fx.commit("alpha 0.1.1", 200 * _DAY_H)
            fx.tag("alpha-v0.1.1")
            fx.release("0.1.2")
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("E2 a pin this change-set introduces is in flight", introduced,
               fail=False, want="in-flight release, tag not cut yet. After this lands")

        expect("E3 a pin on main whose bump is 30 days old FAILS", wave(30 * _DAY_H),
               fail=True, want=at_base)

        def renamed(fx):
            fx.write("skills/alpha/README.md", f"[download]({fx.url('0.1.2')})\n")
            _, base = fx.history(90 * _DAY_H)
            fx.run("mv", "skills/alpha/README.md", "skills/alpha/GUIDE.md")
            return evaluate(fx, base)
        expect("E4 rename laundering: a git mv of a dead pin still FAILS", renamed,
               fail=True, want=f"skills/alpha/GUIDE.md: alpha-v0.1.2/alpha-mcp.mcpb - {at_base}")

        def moved(fx):
            fx.write("skills/alpha/old.md", f"[download]({fx.url('0.1.2')})\n")
            fx.write("skills/alpha/new.md", "no pin here yet\n")
            _, base = fx.history(90 * _DAY_H)
            fx.write("skills/alpha/old.md", "moved\n")
            fx.write("skills/alpha/new.md", f"[download]({fx.url('0.1.2')})\n")
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("E5 cross-file move of a dead pin still FAILS (repo-wide BASE set)", moved,
               fail=True, want=f"skills/alpha/new.md: alpha-v0.1.2/alpha-mcp.mcpb - {at_base}")

        def second_bump(fx):
            _, base = fx.history(1.0)  # 0.1.2 in flight, inside the window
            fx.release("0.1.3")
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("E6 one uncut bump: bumping again while 0.1.2 is uncut FAILS, window or not",
               second_bump, fail=True, want="(0.1.2) was never tagged either")

        def retired_prior(fx):
            _, base = fx.history(90 * _DAY_H)
            fx.burned("alpha-v0.1.2")
            fx.release("0.1.3")
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("E7 retired versions: a burned prior does not consume the grace", retired_prior,
               fail=False, want="alpha-v0.1.3", notice="alpha skipped 0.1.2")

        def wrong_version(fx):
            fx.release("0.1.1")
            base = fx.commit("alpha 0.1.1", 200 * _DAY_H)
            fx.tag("alpha-v0.1.1")
            fx.write("skills/alpha/README.md", f"[download]({fx.url('0.1.5')})\n")
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("E8 a new pin must name the manifest's version", wrong_version,
               fail=True, want="an in-flight release must pin 'alpha-v0.1.1'")

        def base_unreadable(fx):
            _, base = fx.history(1.0)
            fx.tools_only_change()

            def boom(*_a, **_k):
                raise BaseUnavailable("simulated git grep failure")
            g["pins_at_base"] = boom
            return evaluate(fx, base)
        expect("E9 an unreadable BASE pin set withholds every grace, the window too",
               base_unreadable, fail=True, want=at_base, reminder_none=True)

        # ---- The in-flight window for a pin that is already on main ----

        def recent(fx):
            bump, base = fx.history(1.0)
            fx.tools_only_change()
            result = evaluate(fx, base)
            # (Only once it passed: a FAIL here must report as the FAIL it is.)
            must(bool(result[0]) or any(f"--sha {bump} " in r for r in result[2]),
                 f"the reminder must tag the bump commit {bump}")
            return result
        expect("N1 pin on main, no tag, bumped 1h ago: IN FLIGHT, with a reminder", recent,
               fail=False, want="inside the 24h in-flight window")

        expect("N2 the same pin bumped 30h ago: FAIL", wave(30.0), fail=True, want="30.0h ago, past the 24h in-flight window")

        def stale_touched(fx):
            fx.release("0.1.1")
            fx.commit("alpha 0.1.1", 200 * _DAY_H)
            fx.tag("alpha-v0.1.1")
            fx.release("0.1.2")
            bump = fx.commit("release alpha 0.1.2", 120 * _DAY_H)
            fx.release("0.1.2", note="a different field, same version")
            base = fx.commit("alpha: edit another manifest field", 1.0)
            fx.tools_only_change()
            result = evaluate(fx, base)
            must(any(f"{bump[:12]} bumped it to 0.1.2 2880.0h ago" in e for e in result[0]),
                 "the error must date the 120-day-old bump, not the recent edit")
            return result
        expect("N3 a months-old dead pin stays FAIL after a recent edit to another field",
               stale_touched, fail=True, want="past the 24h in-flight window")

        expect("N4 grace removed (window 0): the N1 pin FAILS", wave(1.0, window=0.0),
               fail=True, want="the in-flight window is off")

        def two_uncut(fx):
            fx.release("0.1.0")
            fx.commit("alpha 0.1.0", 200 * _DAY_H)
            fx.tag("alpha-v0.1.0")
            fx.release("0.1.1")
            fx.commit("release alpha 0.1.1 (never cut)", 10 * _DAY_H)
            fx.release("0.1.2")
            base = fx.commit("release alpha 0.1.2", 1.0)
            fx.tools_only_change()
            return evaluate(fx, base)
        expect("N5 one uncut bump inside the window: prior never tagged FAILS", two_uncut,
               fail=True, want="(0.1.1) was never tagged either")

        def two_uncut_retired(fx):
            fx.burned("alpha-v0.1.1")
            return two_uncut(fx)
        expect("N6 retired prior inside the window: IN FLIGHT, with a notice", two_uncut_retired,
               fail=False, want="inside the 24h", notice="alpha skipped 0.1.1")

        expect("N7 a bump dated 48h in the future gets no window", wave(-48.0), fail=True, want="48.0h in the future")

        expect("N7b a bump within the 15-minute clock skew still gets its window", wave(-0.1),
               fail=False, want="inside the 24h")

        def own_commits(fx):
            bump, base = fx.history(90 * _DAY_H)
            fx.release("0.1.3")
            fx.commit("pr: bump away", 0.2)
            fx.release("0.1.2")
            fx.commit("pr: bump back", 0.1)
            fx.tools_only_change()
            result = evaluate(fx, base)
            must(any(f"{bump[:12]} bumped it" in e for e in result[0]),
                 "the error must date the BASE bump, not the PR's own commits")
            return result
        expect("N8 a change-set's own commits cannot refresh the window", own_commits,
               fail=True, want="past the 24h in-flight window")

        def moved_past(fx):
            _, base = fx.history(1.0)
            fx.write("skills/alpha/manifest.json",
                     json.dumps({"name": "alpha", "version": "0.1.3"}) + "\n")
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("N9 a pin the change-set leaves behind its manifest gets no window", moved_past,
               fail=True, want="now says '0.1.3'")

        def merged(branch_age_h: float, merge_age_h: float):
            def build(fx):
                fx.release("0.1.1")
                fx.commit("alpha 0.1.1", 200 * _DAY_H)
                fx.tag("alpha-v0.1.1")
                fx.run("checkout", "-q", "-b", "feature")
                fx.release("0.1.2")
                fx.commit("release alpha 0.1.2 (branch commit)", branch_age_h)
                fx.run("checkout", "-q", "main")
                fx.write("docs/notes.md", "main moved on\n")
                fx.commit("docs: main moved on", 100 * _DAY_H)
                fx.run("merge", "-q", "--no-ff", "-m", "Merge feature", "feature",
                       age_h=merge_age_h)
                base = head(fx)
                fx.tools_only_change()
                return evaluate(fx, base)
            return build
        expect("N10 a merged branch is dated by its merge, not its own commit date",
               merged(branch_age_h=0.1, merge_age_h=30.0),
               fail=True, want="30.0h ago, past the 24h")
        expect("N10b an old branch commit merged 1h ago is in flight",
               merged(branch_age_h=100 * _DAY_H, merge_age_h=1.0),
               fail=False, want="1.0h ago, inside the 24h")

        def brand_new(fx):
            fx.write("docs/notes.md", "seed\n")
            fx.commit("seed", 200 * _DAY_H)
            fx.tag("zeta-v0.1.0")  # some other slug's tag, so the check runs
            fx.release("0.1.0")
            base = fx.commit("add alpha 0.1.0", 1.0)
            fx.tools_only_change()
            return evaluate(fx, base)
        expect("N11 a brand-new skill's first release is in flight", brand_new,
               fail=False, want="inside the 24h")

        def moved_dir(fx):
            fx.history(90 * _DAY_H)
            fx.run("mv", "skills/alpha", "skills/alpha-renamed")
            fx.registry(source_dir="alpha-renamed")
            base = fx.commit("move alpha's directory", 1.0)
            fx.tools_only_change()
            return evaluate(fx, base)
        expect("N11b moving a dead pin's skill directory does not open a window", moved_dir,
               fail=True, want="a moved or re-created skill")

        def burned_itself(fx):
            _, base = fx.history(1.0)
            fx.burned("alpha-v0.1.2")
            fx.tools_only_change()
            return evaluate(fx, base)
        expect("N12 a pin at a retired version gets no window", burned_itself,
               fail=True, want="can never be cut")

        def shallow_clone(fx):
            fx.history(1.0)
            clone = fx.root / "clone"
            fx.run("clone", "-q", "--depth", "1", f"file://{fx.root}", str(clone))
            shallow_fx = _Fixture(clone, fx.now, init=False)
            shallow_fx.tag("alpha-v0.1.1")
            return evaluate(shallow_fx, head(shallow_fx))
        expect("N13 a shallow checkout cannot date a bump: no window", shallow_clone,
               fail=True, want="this checkout is shallow")

        # ---- The window reads BASE, never the change-set (Codex round 1) ----

        def borrowed_dir(fx):
            # alpha's own bump is 30h old. Another directory on main was bumped
            # to the same number 1h ago. The PR repoints alpha's source_dir.
            fx.release("0.1.1")
            fx.commit("alpha 0.1.1", 200 * _DAY_H)
            fx.tag("alpha-v0.1.1")
            fx.release("0.1.2")
            fx.commit("release alpha 0.1.2", 30.0)
            fx.write("skills/staging/manifest.json",
                     json.dumps({"name": "staging", "version": "0.1.1"}) + "\n")
            fx.commit("add staging", 20 * _DAY_H)
            fx.write("skills/staging/manifest.json",
                     json.dumps({"name": "staging", "version": "0.1.2"}) + "\n")
            base = fx.commit("bump staging to 0.1.2", 1.0)
            fx.registry(source_dir="staging")
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("N14 a PR cannot borrow another directory's recent bump via source_dir",
               borrowed_dir, fail=True, want="30.0h ago, past the 24h")

        def unretired_by_pr(fx):
            fx.burned("alpha-v0.1.2")  # retired on main
            _, base = fx.history(1.0)
            fx.burned()                # the PR empties the ledger
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("N15 a PR cannot un-retire a pinned version by editing the ledger",
               unretired_by_pr, fail=True, want="can never be cut")

        def retired_by_pr(fx):
            fx.release("0.1.0")
            fx.commit("alpha 0.1.0", 200 * _DAY_H)
            fx.tag("alpha-v0.1.0")
            fx.release("0.1.1")
            fx.commit("release alpha 0.1.1 (never cut)", 10 * _DAY_H)
            fx.release("0.1.2")
            base = fx.commit("release alpha 0.1.2", 1.0)
            fx.burned("alpha-v0.1.1")  # the PR retires the uncut prior itself
            fx.run("add", "-A")
            return evaluate(fx, base)
        expect("N16 a PR cannot retire an uncut prior to pass the one-uncut-bump rule",
               retired_by_pr, fail=True, want="(0.1.1) was never tagged either")

        # ---- PINS_INFLIGHT_HOURS parsing ----
        for raw, want_hours in (("", 24.0), (" ", 24.0), ("6", 6.0), ("0", 0.0), ("1.5", 1.5)):
            try:
                got = inflight_window_hours({INFLIGHT_ENV: raw})
            except ValueError as exc:
                failures.append(f"P {INFLIGHT_ENV}={raw!r}: refused ({exc})")
                continue
            if got != want_hours:
                failures.append(f"P {INFLIGHT_ENV}={raw!r}: {got} != {want_hours}")
        if inflight_window_hours({}) != INFLIGHT_DEFAULT_HOURS:
            failures.append(f"P unset {INFLIGHT_ENV} must be {INFLIGHT_DEFAULT_HOURS}")
        for raw in ("abc", "-1", "nan", "inf", "24h"):
            try:
                inflight_window_hours({INFLIGHT_ENV: raw})
                failures.append(f"P {INFLIGHT_ENV}={raw!r} must be refused")
            except ValueError:
                pass
        passed.append(f"P {INFLIGHT_ENV} parsing (5 accepted, 5 refused)")

    finally:
        # Restore everything the fixtures repointed, whatever happened.
        g["ROOT"], g["pins_at_base"] = saved["ROOT"], saved["pins_at_base"]
        registry.ROOT, registry.SKILLS_DIR, registry.REGISTRY = saved["registry"]
        os.environ.update(git_env)

    for label in passed:
        print(f"  ok   {label}")
    if failures:
        print("check_pinned_artifacts self-test FAILED:")
        for f in failures:
            print(f"  FAIL {f}")
        return 1
    print(f"check_pinned_artifacts self-test passed ({len(passed)} cases)")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument(
        "--base",
        default="origin/main",
        help="commit the change-set is measured against. PRs: "
             "github.event.pull_request.base.sha. Pushes: github.event.before "
             "(NOT origin/main - it already contains the push).",
    )
    ap.add_argument(
        "--strict", action="store_true",
        help="promote stale-pin notices to failures (the fix is build-catalog.py).",
    )
    # The pinned-ASSET check runs by default when gh is usable; these only
    # override that. --network makes an unusable gh an environment error
    # (exit 2) instead of a skip; --no-network forces the offline-only pass.
    ap.add_argument(
        "--network", dest="network", action="store_true", default=None,
        help="require the pinned-ASSET check; exit 2 if gh is missing or "
             "unauthenticated instead of skipping it.",
    )
    ap.add_argument(
        "--no-network", dest="network", action="store_false",
        help="skip the pinned-ASSET check even when gh is available.",
    )
    ap.add_argument(
        "--warn", action="store_true",
        help="report findings but exit 0. For the first landing only, while main "
             "still carries the never-cut-release backlog this gate exists to "
             "surface; that backlog is cleared by a tag push, not a code change, "
             "so gating on it would block the commit that fixes it.",
    )
    ap.add_argument(
        "--self-test", action="store_true",
        help="prove every rule both directions in throwaway git repos, then exit.",
    )
    args = ap.parse_args()
    if args.self_test:
        return _self_test()

    try:
        window_hours = inflight_window_hours()
    except ValueError as exc:
        print(f"check_pinned_artifacts: {exc}")
        return 2

    owner, repo = registry.owner_repo()
    meta = registry.skills()
    pins = collect_pins(owner, repo)
    if not pins:
        print("check_pinned_artifacts: no pinned release URLs found - nothing to check")
        return 0

    tags = local_tags()
    errors, notices, reminders = check_offline(
        pins, meta, tags, args.base, args.strict, owner, repo,
        window_hours=window_hours,
    )
    if args.network is not False:
        ok, why = gh_usable()
        if ok:
            net_errors, net_notices = check_network(pins, tags, owner, repo)
            errors.extend(net_errors)
            notices.extend(net_notices)
            # --network means "I require the asset check to actually run". A
            # transient GitHub failure skips a tag with a NOTE, which is the
            # right default - but under --network a run where every tag was
            # skipped would print OK while checking nothing. Promote it.
            if args.network and net_notices:
                errors.append(
                    "--network was passed, so the pinned-ASSET check is required, but "
                    + "; ".join(net_notices)
                )
        elif args.network:
            print(f"check_pinned_artifacts: --network requires gh - {why}")
            return 2
        else:
            notices.append(
                f"pinned-ASSET existence check SKIPPED - {why}. Tag existence alone "
                "cannot see a release whose upload was cancelled (the tag is green "
                "and the asset URL 404s)."
            )

    for n in notices:
        print(f"check_pinned_artifacts: NOTE {n}")
    for r in reminders:
        print(f"check_pinned_artifacts: REMINDER {r}")

    # This is the other surface that hands an operator a `git tag <tag> <sha>` to
    # paste. A tag push runs .github/workflows/release.yml FROM THE TAGGED COMMIT,
    # so a SHA from before the draft-then-seal pipeline publishes an EMPTY
    # immutable release and spends that version number permanently. Point at the
    # mechanical check rather than trusting the reader to remember which commits
    # are safe. Printed whenever this run told anyone to cut a tag, whether that
    # came out as a reminder or as a finding.
    def tag_safety_note() -> None:
        print("check_pinned_artifacts: REMINDER a tag push runs release.yml FROM "
              "THE TAGGED COMMIT.")
        print("  Every tag command above leads with the probe for that reason: it runs "
              "when you")
        print("  PASTE, not when this printed, and it refuses a commit that predates "
              "the")
        print("  draft-then-seal pipeline, a retired version number, or a SHA that does "
              "not")
        print("  carry the version the tag names. Do not strip it. To check any other "
              "tag:")
        print(f"    python3 {PROBE_REL} --tag <tag> --sha <sha>")

    names_a_tag = bool(reminders) or any("git tag " in e for e in errors)

    if errors:
        print("check_pinned_artifacts: FAIL")
        for e in errors:
            print(f"  - {e}")
        if names_a_tag:
            tag_safety_note()
        if args.warn:
            print(f"\n  (warn mode: {len(errors)} finding(s), exit 0)")
            return 0
        return 1

    if names_a_tag:
        tag_safety_note()

    files = len({p.path for p in pins})
    print(
        f"check_pinned_artifacts: OK ({len(pins)} pinned release URLs across "
        f"{files} files resolve to a cut tag or an in-flight release)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
