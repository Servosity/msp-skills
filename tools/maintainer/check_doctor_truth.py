#!/usr/bin/env python3
"""Gate: prove `doctor` cannot report a healthy verdict for a broken install.

Why this exists
---------------
`doctor` is the command every SKILL, guide and docs page tells an operator to
run to confirm a connector works. It used to answer "OK API: reachable" for any
host that returned any HTTP response to `GET /`, and it never probed the
credential at all on 41 of 65 connectors - it printed "present, not verified"
and stopped. Measured on the tree before this gate landed, `cipp-cli doctor`
printed an identical all-green report in three mutually exclusive states: a base
URL pointing at the vendor's web UI where every API path 404s, a correct
configuration, and a token that was rejected with 401 on every call. A verdict
that cannot tell those apart is not a health check.

The mirror defect hit correctly-configured operators. A connector whose shipped
default base_url is a placeholder dialled the placeholder host, and the DNS
failure rendered as "FAIL API: unreachable", which reads as "your install is
broken" when the real answer is "one environment variable is unset" - and the
variable was not named anywhere in the report. See issue #282.

What this checks
----------------
Build the connector's real CLI, point it at a local stub HTTP server, and run
`doctor --json` in four states. The assertion is one invariant, applied per
state, rather than a match on exact prose:

    the `credentials` verdict may be POSITIVE only when the credential
    actually worked, and the `api` verdict may be POSITIVE only when the
    connector actually reached this vendor's API.

  healthy      stub answers 200 everywhere        -> credentials MUST be positive
  expired      stub answers 401 everywhere        -> credentials MUST NOT be positive
  wrong-base   stub answers 200 at / and 404      -> neither may be positive; this is
               at every other path                   the misconfiguration that used to
                                                     render as "OK API: reachable"
  placeholder  no base URL override, so the       -> neither may be positive, and the
               connector's shipped placeholder       remedy MUST name the base-URL
               default is in force                   environment variable

Deliberately narrow, so it cannot be flaky:

* No vendor credentials. The stub accepts anything; the config file carries a
  literal `auth_header`, which is the first branch of AuthHeader() in all 65
  connectors, so the probe is exercised without any real secret.
* No network. The stub binds 127.0.0.1 on an ephemeral port.
* Prose-insensitive. Only the positive/negative polarity of a verdict is
  asserted, so rewording a message cannot break the gate, and a connector that
  grows a new verdict shape is judged on what it means.
* A connector with no argument-free GET endpoint to probe reports
  "WARN not verified ... no argument-free GET endpoint", which is negative and
  therefore honest; it is skipped for the healthy case only.

Usage:
    python3 tools/maintainer/check_doctor_truth.py --slug cipp
    python3 tools/maintainer/check_doctor_truth.py --all
    python3 tools/maintainer/check_doctor_truth.py --slug cipp --selftest
"""

from __future__ import annotations

import argparse
import http.server
import json
import os
import subprocess
import sys
import tempfile
import threading
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR

# A verdict is NEGATIVE when it opens with one of these markers or contains one
# of these phrases. Everything else counts as POSITIVE (a claim of health), so a
# new verdict shape is treated as a health claim until it says otherwise - the
# fail-safe direction, since the defect being gated is over-claiming health.
NEGATIVE_PREFIXES = ("ERROR", "WARN", "INFO", "FAIL")
NEGATIVE_PHRASES = (
    "not verified", "not configured", "unreachable", "invalid", "rejected",
    "scope-limited", "skipped", "placeholder", "not there", "error",
    # A connector whose credential store refuses to load says so with its own
    # wording; that is an honest negative, not a claim of health.
    "refused", "not loaded", "transport level only",
)


def is_positive(verdict: object) -> bool:
    """Report whether a doctor verdict claims the check passed."""
    if verdict is None:
        # A missing row is not a claim of health, but it is not honest either.
        # Callers decide; see check_state().
        return False
    s = str(verdict).strip()
    if s.startswith(NEGATIVE_PREFIXES):
        return False
    low = s.lower()
    return not any(p in low for p in NEGATIVE_PHRASES)


# The literal the fixture config carries. The stub watches for it so the gate can
# assert doctor actually SENT the credential, not merely that it classified
# status codes correctly. Without this a doctor probing an unauthenticated
# endpoint passes every state while never testing the installed credential.
PROBE_TOKEN = "check-doctor-truth"


class _Stub(http.server.BaseHTTPRequestHandler):
    mode = "healthy"
    saw_credential = False
    saw_request = False

    def log_message(self, *a):  # silence
        pass

    def _send(self, code: int, body: bytes, ctype: str = "application/json") -> None:
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _handle(self) -> None:
        path = self.path.split("?")[0]
        type(self).saw_request = True
        # Look for the credential anywhere it can legitimately ride: a header
        # value (Authorization or a vendor-specific one), or the query string.
        if PROBE_TOKEN in self.path or any(PROBE_TOKEN in v for v in self.headers.values()):
            type(self).saw_credential = True
        if self.mode == "healthy":
            self._send(200, b'{"data":[],"items":[],"results":[]}')
        elif self.mode == "expired":
            self._send(401, b'{"error":"unauthorized"}')
        elif self.mode == "wrong-base":
            # The signature of a base URL aimed at a vendor's web UI: the root
            # serves an app shell, and nothing else exists.
            if path in ("/", ""):
                self._send(200, b"<!doctype html><html><body>app</body></html>", "text/html")
            else:
                self._send(404, b'{"error":"not found"}')

    do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = do_HEAD = _handle


def serve(mode: str):
    _Stub.mode = mode
    _Stub.saw_credential = False
    _Stub.saw_request = False
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", 0), _Stub)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, f"http://127.0.0.1:{srv.server_address[1]}"


def build_cli(slug: str, meta: dict, out_dir: str) -> str | None:
    """Build the connector's CLI. Returns the binary path, or None if it has no CLI."""
    cli_dir = registry.skill_path(slug) / "cli"
    cli_bin = meta.get("cli_binary")
    if not cli_dir.is_dir() or not cli_bin:
        return None
    main_pkg = cli_dir / "cmd" / cli_bin
    if not main_pkg.is_dir():
        # Generated trees keep the -pp- module name on the cmd dir in some skills.
        cands = [p for p in (cli_dir / "cmd").iterdir() if p.is_dir() and p.name.endswith("-cli")]
        if not cands:
            return None
        main_pkg = cands[0]
    dest = os.path.join(out_dir, cli_bin)
    r = subprocess.run(["go", "build", "-o", dest, "./" + main_pkg.relative_to(cli_dir).as_posix()],
                       cwd=cli_dir, capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"{slug}: go build failed:\n{r.stdout}\n{r.stderr}")
    return dest


def run_doctor(binary: str, slug: str, home: str, base_url: str | None) -> dict:
    """Run `doctor --json` in an isolated HOME and return the parsed report."""
    cfg_dir = Path(home) / ".config" / f"{slug}-cli"
    cfg_dir.mkdir(parents=True, exist_ok=True)
    cfg = cfg_dir / "config.toml"
    # Almost every connector parses config.toml as TOML, but at least one
    # (skykick) unmarshals the same filename as JSON. Writing the wrong dialect
    # makes config.Load fail, which looks like a doctor defect and is not one -
    # so pick the dialect the connector actually parses.
    if config_is_json(slug):
        payload = {"auth_header": "Bearer check-doctor-truth"}
        if base_url:
            payload["base_url"] = base_url
        cfg.write_text(json.dumps(payload), encoding="utf-8")
    else:
        lines = ['auth_header = "Bearer check-doctor-truth"']
        if base_url:
            lines.append(f'base_url = "{base_url}"')
        cfg.write_text("\n".join(lines) + "\n", encoding="utf-8")
    # Some connectors refuse a credential file that is group/world readable, so
    # a default-permission fixture is rejected before the probe ever runs.
    cfg.chmod(0o600)

    env = {
        "HOME": home,
        "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        "NO_COLOR": "1",
        f"{slug.upper().replace('-', '_')}_CONFIG": str(cfg),
    }
    r = subprocess.run([binary, "doctor", "--json"], env=env, capture_output=True, text=True, timeout=120)
    out = r.stdout.strip()
    start = out.find("{")
    if start < 0:
        raise RuntimeError(f"{slug}: doctor emitted no JSON.\nstdout={out}\nstderr={r.stderr[:800]}")
    report = json.loads(out[start:])
    report["__exit_code"] = r.returncode
    return report


def config_is_json(slug: str) -> bool:
    """Does this connector unmarshal its config file as JSON rather than TOML?"""
    cfg = registry.skill_path(slug) / "cli" / "internal" / "config" / "config.go"
    try:
        text = cfg.read_text(encoding="utf-8")
    except OSError:
        return False
    return "json.Unmarshal(data, cfg)" in text and "toml.Unmarshal(data, cfg)" not in text


def has_api_probe(slug: str) -> bool:
    """Does this connector's doctor dial its base_url at all?

    aws-billing does not: it resolves credentials through the AWS SDK's default
    chain and has no base_url probe, so a stub HTTP server cannot exercise its
    credential path. Asserting a positive verdict there would be a false red -
    the gate would be demanding a claim the connector has no way to establish.
    Such a connector is still required to EMIT a credentials row, because
    silence is what let it report "auth: not required" to an operator with no
    credentials at all.
    """
    doc = registry.skill_path(slug) / "cli" / "internal" / "cli" / "doctor.go"
    try:
        return '"reachable' in doc.read_text(encoding="utf-8")
    except OSError:
        return False


def check_state(slug: str, report: dict, state: str, base_env: str,
                saw_request: bool = False, saw_credential: bool = False) -> tuple[list[str], list[str]]:
    """Apply the invariant for one state. Returns (failures, advisory notes)."""
    errs: list[str] = []
    notes: list[str] = []
    creds = report.get("credentials")
    api = report.get("api")
    cred_pos, api_pos = is_positive(creds), is_positive(api)

    if state == "healthy":
        if report.get("__exit_code") not in (0, None):
            errs.append(f"[{slug}] healthy: doctor exited {report['__exit_code']} on a working install, so it "
                        f"cannot be used in automation and reports failure when nothing is wrong")
        if saw_request and not saw_credential:
            # Reported, not failed. The check cannot mechanically separate two
            # very different causes: a connector whose doctor genuinely probes
            # unauthenticated (a real false-OK), and one whose credential shape
            # this fixture cannot supply, so the client has nothing to attach.
            # Against a real vendor API the second case answers 401 and doctor
            # reports "rejected", which is honest - it is only a permissive stub
            # that turns it into a false "valid". Failing here would be a
            # false-RED on the second case; staying silent would hide the first.
            notes.append(f"[{slug}] doctor probed the API but the probe carried no credential. Either it "
                         f"probes unauthenticated (a false OK) or this fixture cannot supply this "
                         f"connector's credential shape. Verify against a real tenant.")
        if creds is None:
            errs.append(f"[{slug}] healthy: doctor emitted no `credentials` row at all, so an operator "
                        f"is told nothing about whether the credential works")
        elif not has_api_probe(slug):
            pass  # honest negative is the only verdict available; see has_api_probe()
        elif not cred_pos and "no argument-free GET endpoint" not in str(creds):
            errs.append(f"[{slug}] healthy: credentials should verify against a working API but said: {creds!r}")
    elif state == "expired":
        if cred_pos:
            errs.append(f"[{slug}] expired: every request answered HTTP 401 and doctor still reports "
                        f"credentials as healthy: {creds!r}")
    elif state == "wrong-base":
        if cred_pos:
            errs.append(f"[{slug}] wrong-base: base_url points at a host where every API path 404s and "
                        f"doctor still reports credentials as healthy: {creds!r}")
        if api_pos:
            errs.append(f"[{slug}] wrong-base: base_url is not this vendor's API root and doctor still "
                        f"reports the API as healthy: {api!r}")
    elif state == "placeholder":
        if cred_pos:
            errs.append(f"[{slug}] placeholder: base_url is the shipped placeholder and doctor still "
                        f"reports credentials as healthy: {creds!r}")
        if api_pos:
            errs.append(f"[{slug}] placeholder: base_url is the shipped placeholder and doctor still "
                        f"reports the API as healthy: {api!r}")
        named = base_env in f"{api} {creds} {report.get('auth_hint', '')}"
        if not named:
            errs.append(f"[{slug}] placeholder: the report never names {base_env}, so a correctly "
                        f"credentialled operator is told they are broken without being told the fix")
    return errs, notes


def placeholder_default(slug: str) -> bool:
    """Does this connector ship a base_url default the operator must replace?"""
    cfg = registry.skill_path(slug) / "cli" / "internal" / "config" / "config.go"
    try:
        text = cfg.read_text(encoding="utf-8")
    except OSError:
        return False
    import re
    m = re.search(r'BaseURL:\s*"([^"]*)"', text)
    if not m:
        return False
    base = m.group(1)
    if "{" in base:
        return True
    host = base.split("://")[-1].split("/")[0].lower()
    return any(k in host for k in ("your-", "your_", "yourcompany", "yourdomain", "yourmsp",
                                   "example.com", "example.net", "example.org", "changeme")) or "YOUR_" in base


def check_slug(slug: str, meta: dict, verbose: bool) -> tuple[list[str], list[str]]:
    errs: list[str] = []
    notes: list[str] = []
    base_env = f"{slug.upper().replace('-', '_')}_BASE_URL"
    with tempfile.TemporaryDirectory() as tmp:
        binary = build_cli(slug, meta, tmp)
        if binary is None:
            return [], []
        verdicts: dict[str, object] = {}
        for state in ("healthy", "expired", "wrong-base"):
            srv, url = serve(state)
            try:
                home = os.path.join(tmp, "home-" + state)
                os.makedirs(home, exist_ok=True)
                report = run_doctor(binary, slug, home, url)
                saw_request, saw_credential = _Stub.saw_request, _Stub.saw_credential
            finally:
                srv.shutdown()
            verdicts[state] = report.get("credentials")
            e, n = check_state(slug, report, state, base_env, saw_request, saw_credential)
            errs += e
            notes += n
            if verbose:
                print(f"    {slug} {state}: api={report.get('api')!r} credentials={report.get('credentials')!r}"
                      f" [sent_credential={saw_credential}]")

        # The sharpest check, and the one that needs no opinion about prose: a
        # doctor that answers the SAME thing whether the credential works, is
        # rejected, or points at the wrong host is not reporting a verdict. That
        # identical-in-every-state report is exactly the defect issue #282 named.
        probeable = ("no argument-free GET endpoint" not in str(verdicts.get("healthy") or "")
                     and has_api_probe(slug))
        if probeable:
            for broken in ("expired", "wrong-base"):
                if verdicts.get(broken) == verdicts.get("healthy"):
                    errs.append(
                        f"[{slug}] doctor reports the identical credentials verdict "
                        f"({verdicts.get('healthy')!r}) whether the credential works or the install is "
                        f"{broken}; it is not distinguishing the states")
        if placeholder_default(slug):
            home = os.path.join(tmp, "home-placeholder")
            os.makedirs(home, exist_ok=True)
            report = run_doctor(binary, slug, home, None)
            e, n = check_state(slug, report, "placeholder", base_env)
            errs += e
            notes += n
            if verbose:
                print(f"    {slug} placeholder: api={report.get('api')!r} credentials={report.get('credentials')!r}")
    return errs, notes


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--slug", help="check one skill")
    ap.add_argument("--all", action="store_true", help="check every skill (default)")
    ap.add_argument("--warn", action="store_true", help="report findings but exit 0")
    ap.add_argument("-v", "--verbose", action="store_true", help="print every verdict")
    args = ap.parse_args()

    meta = registry.skills()
    if args.slug:
        if args.slug not in meta:
            print(f"check_doctor_truth: unknown slug {args.slug!r}", file=sys.stderr)
            return 2
        slugs = [args.slug]
    else:
        slugs = sorted(meta)

    errors: list[str] = []
    all_notes: list[str] = []
    checked = 0
    for slug in slugs:
        if registry.is_markdown_only(slug):
            continue
        try:
            found, found_notes = check_slug(slug, meta[slug], args.verbose)
        except Exception as exc:  # a connector that cannot be built or run is a failure, not a skip
            errors.append(f"[{slug}] could not be checked: {exc}")
            continue
        checked += 1
        errors += found
        all_notes += found_notes

    for n in all_notes:
        print(f"check_doctor_truth: NOTE {n}")

    if errors:
        print("check_doctor_truth FAILED:")
        for e in errors:
            print(f"  - {e}")
        print("\n`doctor` is the command every guide tells an operator to run to confirm a connector")
        print("works. A verdict above claims health in a state that is provably broken, or withholds")
        print("the remedy from an operator who did everything the install asked. See issue #282.")
        return 0 if args.warn else 1

    print(f"PASS: doctor reports a verdict that tracks reality in {checked} connector(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
