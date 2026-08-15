# /// script
# requires-python = ">=3.12"
# dependencies = []
# ///
"""credgrab - refresh browser-session credentials for sites with no API path.

Some vendors issue no API key and no OAuth for the surface an agent needs: the
web app authenticates with an httpOnly session cookie or an opaque token in
localStorage. That credential expires and must be re-captured by hand. credgrab
automates everything AROUND that one manual Copy-as-cURL step:

  extract (curlparse) -> store (OS credential store, credstore) ->
  wire (regenerate the tool's consumer file from the store) -> verify (live call)

The credential value never enters model-visible output: seed/wire print only a
redacted receipt (len / sha256[:8] / last4). The credential store is canonical;
consumer files (credentials.toml, .env) are DERIVED artifacts rebuilt from it
via each profile's template, so their exact format is always correct. A
mis-quoted or double-wrapped hand edit cannot recur, because the file is never
hand-written.

Commands:
  seed   --profile NAME [--curl FILE]   parse a capture -> store -> wire -> verify
  wire   --profile NAME                 rebuild the consumer file from the store
  verify --profile NAME                 run the profile's live health check
  doctor [--all | --profile NAME]       health-check; exit != 0 if any expired/expiring
  list                                  show profiles + last-seed + status
  --selfcheck [--live]                  curlparse + credstore + render tests. Offline by
                                        default; --live adds a real credential-store
                                        round trip (creates and deletes one entry, and
                                        can raise a Keychain prompt on macOS).

Profiles live in profiles/<name>.json (see README.md for the schema).
"""
from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile
import time
from pathlib import Path

import credstore
import curlparse

SCRIPT_DIR = Path(__file__).resolve().parent
# Everything anchors on the SKILL directory, never on a repo root: a Skill is
# installed wherever the host puts it, so `../..` is not a meaningful location.
# profiles/ and captures/ are siblings of scripts/, as SKILL.md describes them.
SKILL_DIR = SCRIPT_DIR.parent
PROFILES_DIR = SKILL_DIR / "profiles"
STATE_PATH = SKILL_DIR / "state.json"
VERIFY_TIMEOUT = 90

# doctor / verify exit codes. A profile's verify command signals a dead
# credential either by exit code (expired_exit) or by output text (expired_on).
OK, ERROR, EXPIRED = 0, 1, 2


# --------------------------------------------------------------------------- #
# profiles + paths
# --------------------------------------------------------------------------- #

def load_profile(name: str) -> dict:
    path = PROFILES_DIR / f"{name}.json"
    if not path.exists():
        avail = ", ".join(p.stem for p in sorted(PROFILES_DIR.glob("*.json"))) or "(none)"
        raise SystemExit(f"FAIL: no profile '{name}'. Available: {avail}")
    with open(path, "r", encoding="utf-8") as fh:
        prof = json.load(fh)
    prof.setdefault("name", name)
    return prof


def all_profiles() -> list[str]:
    return sorted(p.stem for p in PROFILES_DIR.glob("*.json"))


_HOME_VAR = re.compile(r"\$\{HOME\}|\$HOME(?![A-Za-z0-9_])")


def resolve_path(p: str, base: Path) -> Path:
    """Expand ${VARS}; resolve a relative path against `base` (absolute stays).

    $HOME is not set on Windows outside a POSIX-style shell, and expandvars
    leaves an unknown variable in place verbatim -- so `${HOME}/.config/x` would
    silently resolve to `<base>/${HOME}/.config/x` and write the consumer file
    into the repo. Substitute the platform home first. The negative lookahead
    keeps $HOMEDRIVE and $HOMEPATH intact for expandvars to handle.
    """
    p = _HOME_VAR.sub(lambda _: str(Path.home()), p) if "HOME" not in os.environ else p
    expanded = os.path.expandvars(p)
    q = Path(expanded)
    return q if q.is_absolute() else (base / q)


def capture_path(prof: dict, override: str | None) -> Path:
    if override:
        return resolve_path(override, Path.cwd())
    return resolve_path(prof["capture_file"], SKILL_DIR)


# --------------------------------------------------------------------------- #
# state (timestamps + receipts only -- NEVER values)
# --------------------------------------------------------------------------- #

def read_state() -> dict:
    if not STATE_PATH.exists():
        return {"profiles": {}}
    try:
        with open(STATE_PATH, "r", encoding="utf-8") as fh:
            return json.load(fh)
    except (OSError, ValueError):
        return {"profiles": {}}


def write_state(state: dict) -> None:
    tmp = STATE_PATH.with_suffix(".json.tmp")
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(state, fh, indent=2)
    os.replace(tmp, STATE_PATH)


def record_seed(name: str, receipts: dict[str, str]) -> None:
    state = read_state()
    state.setdefault("profiles", {})[name] = {
        "last_seed": int(time.time()),
        "receipts": receipts,  # already redacted (len/sha8/last4 lines)
    }
    write_state(state)


# --------------------------------------------------------------------------- #
# extract
# --------------------------------------------------------------------------- #

def extract_values(prof: dict, headers: dict[str, str]) -> dict[str, str]:
    """Pull each profile-declared credential out of the parsed headers.

    Returns {store_as: value}. Raises SystemExit on a missing required field.
    Values are never printed here.
    """
    out: dict[str, str] = {}
    for item in prof["extract"]:
        store_as = item["store_as"]
        required = item.get("required", False)
        src = item.get("from", "header")
        value = ""
        if src == "header":
            value = headers.get(item["name"].lower(), "")
        elif src == "cookie":
            cookie = headers.get("cookie", "")
            m = re.search(item["pattern"], cookie)
            value = m.group(1) if m else ""
        else:
            raise SystemExit(f"FAIL: profile extract 'from' must be header|cookie, got {src!r}")
        if not value:
            if required:
                where = item.get("name") or item.get("pattern")
                raise SystemExit(
                    f"FAIL: required credential '{store_as}' not found in the capture "
                    f"(looked for {src}: {where}). Wrong request captured, or expired session? "
                    f"Re-capture from an authenticated request."
                )
            continue
        out[store_as] = value
    if not out:
        raise SystemExit("FAIL: no credentials extracted from the capture.")
    return out


# --------------------------------------------------------------------------- #
# wire (render the consumer file from stored values)
# --------------------------------------------------------------------------- #

def render_wire(wire: dict, values: dict[str, str]) -> str:
    """Pure render: substitute {STORE_AS} placeholders. Value braces are safe
    (literal replace, not str.format). Missing keys render as empty string."""
    def sub(template: str) -> str:
        out = template
        for key in _placeholders(template):
            out = out.replace("{" + key + "}", values.get(key, ""))
        return out

    wtype = wire["type"]
    if wtype == "template-file":
        return sub(wire["template"])
    if wtype == "env-file":
        body = ""
        if wire.get("header_comment"):
            body += wire["header_comment"].rstrip("\n") + "\n"
        for line in wire["lines"]:
            body += sub(line) + "\n"
        return body
    raise SystemExit(f"FAIL: unknown wire type {wtype!r}")


def _placeholders(template: str) -> list[str]:
    return re.findall(r"\{([A-Z0-9_]+)\}", template)


def atomic_write(path: Path, content: str, mode: int) -> None:
    """Write `content` to `path` atomically, never world-readable in between.

    The temp file carries the credential, so it is created with mkstemp -- a
    unique, O_EXCL, mode-0600 file in the destination directory. That closes two
    holes a fixed `<dest>.tmp` name leaves open: a concurrent seed/wire writing
    the same profile through the same inode, and a symlink pre-planted at the tmp
    name redirecting the credential write when the parent dir is shared-writable.
    `mode` is still applied before the rename, since mkstemp forces 0600 and the
    consumer file may legitimately want something else.
    """
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    tmp = Path(tmp_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="") as fh:
            fh.write(content)
        try:
            os.chmod(tmp, mode)
        except OSError:
            pass  # best effort on Windows
        os.replace(tmp, path)
    finally:
        try:
            tmp.unlink()
        except FileNotFoundError:
            pass  # already renamed into place


def do_wire(prof: dict, values: dict[str, str] | None = None) -> Path:
    """Rebuild the consumer file. If `values` is None, fetch from the store."""
    wire = prof["wire"]
    if values is None:
        values = {}
        for item in prof["extract"]:
            store_as = item["store_as"]
            v = credstore.fetch(store_as, prof["name"])
            if v is None:
                if item.get("required", False):
                    raise SystemExit(
                        f"FAIL: '{store_as}' is not in the credential store yet. "
                        f"Run: credgrab seed --profile {prof['name']}"
                    )
                continue
            values[store_as] = v

    content = render_wire(wire, values)

    guard = wire.get("guard_startswith")
    if guard and not content.startswith(guard):
        # Refuse to write a mis-shaped consumer file. This is the anti-double-wrap
        # gate: the rendered file must begin exactly as the template dictates.
        raise SystemExit(
            f"FAIL: refusing to write {wire['path']} -- rendered content did not start "
            f"with the required guard. Stored value is likely malformed; re-seed."
        )

    dest = resolve_path(wire["path"], SKILL_DIR)
    mode = int(wire.get("mode", "0600"), 8) if isinstance(wire.get("mode"), str) else wire.get("mode", 0o600)
    atomic_write(dest, content, mode)
    return dest


# --------------------------------------------------------------------------- #
# verify
# --------------------------------------------------------------------------- #

def run_verify(prof: dict) -> tuple[int, str]:
    """Run the profile's live health check. Returns (code, detail_line)."""
    vr = prof.get("verify")
    if not vr:
        return ERROR, "no verify command configured for this profile"
    cmd = list(vr["cmd"])
    # A bare executable name is a PATH lookup and must stay untouched: sending
    # it through resolve_path would join it onto SKILL_DIR, so `example-cli`
    # becomes `<SKILL_DIR>/example-cli`, which does not exist, and every verify
    # fails with "binary not found". Only resolve values that are actually
    # path-like -- containing a separator, or naming a variable to expand.
    head = cmd[0]
    if any(sep in head for sep in (os.sep, "/", "\\")) or "$" in head:
        head = str(resolve_path(head, SKILL_DIR))
    cmd[0] = head
    cwd = str(resolve_path(vr["cwd"], SKILL_DIR)) if vr.get("cwd") else str(SKILL_DIR)
    try:
        r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=VERIFY_TIMEOUT)
    except FileNotFoundError:
        return ERROR, f"verify binary not found: {cmd[0]}"
    except subprocess.TimeoutExpired:
        return ERROR, "verify timed out"

    combined = ((r.stdout or "") + "\n" + (r.stderr or "")).strip()
    first = combined.splitlines()[0] if combined else f"exit {r.returncode}"

    if "expired_exit" in vr:
        # exit-code based (reliable when you control the verify command).
        if r.returncode == 0:
            return OK, first
        if r.returncode == vr["expired_exit"]:
            return EXPIRED, first
        return ERROR, first
    # substring based: check failure markers FIRST, regardless of exit code --
    # some generated CLIs print an auth error but still exit 0. Markers must be
    # specific enough not to appear in healthy output (e.g. "http 401", not "session").
    low = combined.lower()
    if any(s.lower() in low for s in vr.get("expired_on", [])):
        return EXPIRED, first
    if r.returncode == 0:
        return OK, "live authed read OK"
    return ERROR, first


VERDICT = {OK: "OK", ERROR: "ERROR", EXPIRED: "EXPIRED"}


# --------------------------------------------------------------------------- #
# commands
# --------------------------------------------------------------------------- #

def _rollback_seed(prof: dict, prior_values: dict[str, str | None],
                   dest_path: Path, prior_consumer: bytes | None) -> None:
    """Undo a seed whose verify did not pass: restore the prior credential and
    consumer file so a wrong or expired capture never destroys a known-good one."""
    for store_as, prior in prior_values.items():
        if prior is not None:
            credstore.store(store_as, prof["name"], prior)
        else:
            # There was nothing before; remove what this seed added.
            try:
                credstore.delete(store_as, prof["name"])
            except Exception:
                pass
    if prior_consumer is not None:
        dest_path.write_bytes(prior_consumer)
        try:
            os.chmod(dest_path, 0o600)
        except OSError:
            pass
    elif dest_path.exists():
        try:
            dest_path.unlink()
        except OSError:
            pass


def cmd_seed(args) -> int:
    prof = load_profile(args.profile)
    cap = capture_path(prof, args.curl)
    if not cap.exists():
        raise SystemExit(
            f"FAIL: capture file not found: {cap}\n"
            f"  Copy-as-cURL an authenticated request into that file, then re-run.\n"
            f"  (the /grab-cookie skill walks you through it)"
        )
    headers = curlparse.parse_headers_from_file(str(cap))
    values = extract_values(prof, headers)

    # Snapshot everything this seed is about to overwrite. The credential store is
    # canonical, so a failed verify must not leave it holding a bad value or the
    # consumer file wired to one. We commit only after run_verify passes.
    prior_values: dict[str, str | None] = {
        store_as: credstore.fetch(store_as, prof["name"]) for store_as in values
    }
    dest_path = resolve_path(prof["wire"]["path"], SKILL_DIR)
    prior_consumer = dest_path.read_bytes() if dest_path.exists() else None

    receipts: dict[str, str] = {}
    for store_as, value in values.items():
        credstore.store(store_as, prof["name"], value)
        receipts[store_as] = credstore.receipt(value, store_as, prof["name"])

    dest = do_wire(prof)  # rebuild from the store we just wrote

    code, detail = run_verify(prof)
    if code != OK:
        # Wrong or expired capture: roll back to the previous known state.
        _rollback_seed(prof, prior_values, dest_path, prior_consumer)
        print(f"verify: {VERDICT[code]} - {detail}")
        print(f"  rolled back: '{prof['name']}' credential and consumer file left unchanged "
              f"(the new capture did not verify)")
        return code

    # Verified good -- now it is safe to publish receipts and record the seed.
    print(f"seeded '{prof['name']}' ({len(values)} credential(s)); wired -> {dest}")
    for store_as, rc in receipts.items():
        print(f"  {store_as}: {rc}")
    record_seed(prof["name"], receipts)
    print(f"verify: {VERDICT[code]} - {detail}")
    return code


def cmd_wire(args) -> int:
    prof = load_profile(args.profile)
    dest = do_wire(prof)
    print(f"wired '{prof['name']}' from the credential store -> {dest}")
    return OK


def cmd_verify(args) -> int:
    prof = load_profile(args.profile)
    code, detail = run_verify(prof)
    print(f"{VERDICT[code]}: {prof['name']} - {detail}")
    return code


def _expiring(prof: dict, state: dict) -> bool:
    ttl = prof.get("ttl_days")
    warn = prof.get("warn_days")
    if not ttl or not warn:
        return False
    entry = state.get("profiles", {}).get(prof["name"], {})
    last = entry.get("last_seed")
    if not last:
        return False
    age_days = (time.time() - last) / 86400.0
    return age_days > (ttl - warn)


def cmd_doctor(args) -> int:
    state = read_state()
    # --all checks SEEDED profiles only. all_profiles() would include the shipped
    # example-* profiles that were never seeded and report false failures for
    # credentials that do not exist.
    if args.all or not args.profile:
        names = sorted(state.get("profiles", {}))
    else:
        names = [args.profile]
    worst = OK
    for name in names:
        prof = load_profile(name)
        code, detail = run_verify(prof)
        status = VERDICT[code]
        if code == OK and _expiring(prof, state):
            status = "EXPIRING"
            detail = "cookie nearing its ~%sd TTL; refresh soon" % prof.get("ttl_days")
            worst = max(worst, EXPIRED)
        else:
            worst = max(worst, code)
        print(f"PROFILE {name} STATUS {status} :: {detail}")
    return worst


def cmd_list(args) -> int:
    state = read_state().get("profiles", {})
    for name in all_profiles():
        prof = load_profile(name)
        entry = state.get(name, {})
        last = entry.get("last_seed")
        when = time.strftime("%Y-%m-%d", time.localtime(last)) if last else "never"
        ttl = prof.get("ttl_days")
        ttl_s = f"ttl={ttl}d" if ttl else "ttl=probe-only"
        print(f"  {name:12} last-seed={when:11} {ttl_s}")
    return OK


def cmd_selfcheck(live: bool = False) -> int:
    curlparse._selfcheck()
    credstore._selfcheck(live=live)
    # render + guard, with no live creds
    wire_tmpl = {
        "type": "template-file",
        "template": "access_token = 'example_session={ZERORANK_SESSION}'\n",
        "guard_startswith": "access_token = 'example_session=",
    }
    rendered = render_wire(wire_tmpl, {"ZERORANK_SESSION": "s%3Aabc{}def"})  # braces in value are safe
    assert rendered == "access_token = 'example_session=s%3Aabc{}def'\n", rendered
    assert rendered.startswith(wire_tmpl["guard_startswith"])
    wire_env = {
        "type": "env-file",
        "header_comment": "# test",
        "lines": ["A={A}", "B={B}"],
    }
    assert render_wire(wire_env, {"A": "1"}) == "# test\nA=1\nB=\n", render_wire(wire_env, {"A": "1"})
    # do_wire round-trip + anti-double-wrap guard, against a temp file (no live creds)
    import tempfile
    with tempfile.TemporaryDirectory() as td:
        good_path = os.path.join(td, "credentials.toml")
        dest = do_wire(
            {"name": "x", "extract": [], "wire": {**wire_tmpl, "path": good_path}},
            values={"ZERORANK_SESSION": "s%3Agoodvalue"},
        )
        with open(dest, encoding="utf-8") as fh:
            assert fh.read() == "access_token = 'example_session=s%3Agoodvalue'\n"
        # a mis-shaped render must be refused, not written
        bad = {"type": "template-file", "template": "access_token = 'WRONG'\n",
               "guard_startswith": "access_token = 'example_session=",
               "path": os.path.join(td, "should-not-exist.toml")}
        try:
            do_wire({"name": "x", "extract": [], "wire": bad}, values={})
            raise AssertionError("guard did not block a mis-shaped render")
        except SystemExit:
            pass
        assert not os.path.exists(bad["path"]), "guard-blocked write still created the file"
    scope = "live credstore round-trip" if live else "no live creds"
    print(f"credgrab.py selfcheck OK (curlparse + credstore + render/guard, {scope})")
    return OK


# --------------------------------------------------------------------------- #
# main
# --------------------------------------------------------------------------- #

def main() -> int:
    if "--selfcheck" in sys.argv:
        return cmd_selfcheck(live="--live" in sys.argv)
    ap = argparse.ArgumentParser(prog="credgrab", description="refresh browser-session credentials")
    sub = ap.add_subparsers(dest="cmd", required=True)

    p = sub.add_parser("seed", help="parse a capture -> store -> wire -> verify")
    p.add_argument("--profile", required=True)
    p.add_argument("--curl", help="override the capture file path")
    p.set_defaults(fn=cmd_seed)

    p = sub.add_parser("wire", help="rebuild the consumer file from the store")
    p.add_argument("--profile", required=True)
    p.set_defaults(fn=cmd_wire)

    p = sub.add_parser("verify", help="run the profile's live health check")
    p.add_argument("--profile", required=True)
    p.set_defaults(fn=cmd_verify)

    p = sub.add_parser("doctor", help="health-check one or all profiles")
    p.add_argument("--profile")
    p.add_argument("--all", action="store_true")
    p.set_defaults(fn=cmd_doctor)

    p = sub.add_parser("list", help="show profiles + last-seed + status")
    p.set_defaults(fn=lambda a: cmd_list(a))

    args = ap.parse_args()
    return args.fn(args)


if __name__ == "__main__":
    sys.exit(main())
