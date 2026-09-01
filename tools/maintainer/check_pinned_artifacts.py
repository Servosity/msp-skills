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
  * tag missing, and the pin already existed ANYWHERE at BASE -> FAIL. The
    release was never cut; main is publishing a 404.

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

That gives a release commit exactly one push of grace - once, not once per push
- and turns a never-cut tag into a red main on the NEXT push, naming the slug.

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

Exit 0 = clean, 1 = violations (printed), 2 = environment error.

Run locally:  python3 tools/maintainer/check_pinned_artifacts.py
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
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


def manifest_version_at_base(
    base: str, slug: str, renames: dict[str, str]
) -> str | None:
    """The version this slug shipped at BASE, or None if it had no manifest then
    (a brand-new skill, whose first release is legitimately in flight)."""
    path = f"skills/{registry.source_dir(slug)}/manifest.json"
    p = git("show", f"{base}:{renames.get(path, path)}")
    if p.returncode != 0:
        return None
    try:
        return json.loads(p.stdout).get("version")
    except ValueError:
        return None


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


def check_offline(pins, meta, tags, base, strict, owner, repo):
    errors: list[str] = []
    notices: list[str] = []
    reminders: list[str] = []

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
        burned_tags = check_release_pipeline.load_burned()
    except check_release_pipeline.ProbeError as exc:
        burned_tags = {}
        notices.append(
            f"the retired-version ledger could not be read ({exc}); a burned "
            "version will be treated as an uncut one, which can only make the "
            "in-flight grace stricter"
        )

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
                errors.append(
                    f"{pin.where} - tag does not exist and this URL was already in the "
                    f"tree at BASE (in this file or another one): the {slug} release was "
                    f"never cut, so this URL 404s. Cut it "
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
            if slug not in base_version_cache:
                base_version_cache[slug] = manifest_version_at_base(
                    base, slug, renames
                )
            prior = base_version_cache[slug]
            prior_burned = prior is not None and f"{slug}-v{prior}" in burned_tags
            if prior is not None and f"{slug}-v{prior}" not in tags and not prior_burned:
                errors.append(
                    f"{pin.where} - new pin for an uncut tag, but the version {slug} "
                    f"carried at BASE ({prior}) was never tagged either. An in-flight "
                    f"release gets one push of grace; this is the second bump with no "
                    f"tag cut. Cut {slug}-v{prior} first, or revert the bump."
                )
                continue
            if prior_burned and slug not in burned_reported:
                # The BASE version is not an uncut release, it is a RETIRED one:
                # its tag was cut, its release sealed empty and both were deleted.
                # Telling anyone to "cut it first" is the one instruction that
                # must never be given, so say so rather than staying silent.
                # Once per slug, not once per pin.
                burned_reported.add(slug)
                notices.append(
                    f"{slug} skipped {prior}: that number is retired in "
                    f"tools/maintainer/burned_versions.json and must never be cut "
                    f"again. It does not consume this release's grace."
                )
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
    args = ap.parse_args()

    owner, repo = registry.owner_repo()
    meta = registry.skills()
    pins = collect_pins(owner, repo)
    if not pins:
        print("check_pinned_artifacts: no pinned release URLs found - nothing to check")
        return 0

    tags = local_tags()
    errors, notices, reminders = check_offline(
        pins, meta, tags, args.base, args.strict, owner, repo
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
