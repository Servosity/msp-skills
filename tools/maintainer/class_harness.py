#!/usr/bin/env python3
"""Shared, black-box harness for the recurring connector bug CLASSES.

Why this exists
---------------
The factory's review loops converge only once a whole bug class is fixed, not
one instance per round (feedback_codex_loop_close_classes; the saas-alerts
45-round and synology 54-round secret-echo loops). The plan (Step 3) turns each
recurring class into one automatic test that runs before any review, so a
reviewer never spends a round rediscovering a member of a class a gate already
covers.

What this is
------------
A connector-agnostic harness. It drives the BUILT `<slug>-cli` and `<slug>-mcp`
binaries against a local fake HTTP server, exactly as an operator's agent would
drive them. It imports no connector Go package and hard-codes no per-connector
names: the command surface is read from the binary's own `agent-context` JSON
(which lists hidden groups, unlike --help), and the credential and base-URL
wiring is read from `skills/<slug>/manifest.json`. Everything is stdlib Python.

Three class tests live beside it and register here:
  * class_secret_echo   - a credential value never reaches CLI output, errors
                          or MCP results, in any encoding a server can reflect.
  * class_auth_wire     - a configured credential is actually transmitted, and
                          none follows a redirect to another host.
  * class_write_safety  - every non-read MCP tool is refused under the read-only
                          switch, and `--dry-run` sends nothing.

Entry point
-----------
    python3 tools/maintainer/class_harness.py --slug ninjaone
    python3 tools/maintainer/class_harness.py --all --report
    python3 tools/maintainer/class_harness.py --slug ninjaone --class secret-echo
    python3 tools/maintainer/class_harness.py --self-test
    python3 tools/maintainer/class_harness.py --all --update-baseline

Rollout without turning main red
--------------------------------
`class_baseline.json` lists the connectors that FAIL a class on main today. A
listed connector is reported, never failed (its exposure is known and awaiting a
press fix or a per-connector fix wave). CI fails only when an UNLISTED connector
fails, or when a LISTED connector now passes but is still listed (the list only
shrinks). `--update-baseline` rewrites it from a fleet run; it may only remove
entries, never add (a newly broken connector must be fixed, not baselined).

Every class proves itself BOTH directions: `--self-test` reproduces a past
broken connector shape and asserts the class fires on it, and asserts the class
is silent on a healthy shape. The report additionally names a real past broken
commit per class (see the PR).
"""

from __future__ import annotations

import argparse
import base64
import html
import json
import os
import shutil
import socket
import subprocess
import sys
import tempfile
import threading
import urllib.parse
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402

ROOT = registry.ROOT
SKILLS_DIR = registry.SKILLS_DIR
BASELINE = Path(__file__).resolve().parent / "class_baseline.json"

# Per-step timeouts. A single connector's run must stay small (CI runs it per
# matrix row); the client retries 429/5xx up to 3 times with a few seconds'
# backoff, so the fake server never answers those on a path the test waits on.
BUILD_TIMEOUT = 300
RUN_TIMEOUT = 60


# --------------------------------------------------------------------------- #
# Sentinels and the encodings a server can reflect a credential back in.
# --------------------------------------------------------------------------- #

def sentinel(var: str) -> str:
    """A unique, searchable marker for one credential variable.

    Carries transport-sensitive characters (`.+/=`) so its raw, URL, double-URL,
    HTML and JSON-escaped forms are all DIFFERENT - an alphanumeric-only value
    would make those encodings identical to the raw value and the per-encoding
    probes would be no-ops. Avoids `<`/`>`/space, which a connector's
    placeholder-credential guard would reject as a `<...>` placeholder. Long
    enough that a last-4-chars mask never equals the whole value."""
    short = "".join(ch for ch in var.lower() if ch.isalnum())[:16]
    return f"clsSENT{short}.0a1+2b3/4c5=6d7e8f"


def encodings(value: str) -> dict[str, str]:
    """Every way a server (or a proxy, or the connector's own diagnostics) can
    render one credential value. The class asserts NONE of these survives in
    output decoded back to `value` - i.e. the value is never reconstructable.

    These are the exact forms the secret-echo loops chased one at a time:
    URL and double-URL escaping, HTML entities, JSON \\u escapes, and
    base64 / base64url (the "decoded backstop" classes)."""
    b = value.encode()
    return {
        "raw": value,
        "url": urllib.parse.quote(value, safe=""),
        "url2": urllib.parse.quote(urllib.parse.quote(value, safe=""), safe=""),
        "html": html.escape(value).replace(".", "&#46;"),
        "json_u": "".join(f"\\u{ord(c):04x}" for c in value),
        "base64": base64.b64encode(b).decode(),
        "base64url": base64.urlsafe_b64encode(b).decode().rstrip("="),
    }


def leaks(text: str, value: str) -> list[str]:
    """Return the names of every encoding of `value` that appears in `text`.

    A precomputed encoding of `value` is reported by name. For everything else,
    `text` is run through the canonical decode funnel (`_decode_views`) to a
    fixed point - full JSON string unescaping, URL, HTML and base64 over EVERY
    candidate run - and a `decoded` hit is reported if the whole value appears in
    any view. A last-4-chars mask (`****abcd`) never matches, because the whole
    value is the needle."""
    hits = [name for name, enc in encodings(value).items() if enc and enc in text]
    precomputed = {"raw", "url", "url2", "html", "json_u", "base64", "base64url"}
    if not (precomputed & set(hits)):
        for view in _decode_views(text):
            if value in view:
                hits.append("decoded")
                break
    return hits


# Candidate-run scans for the base64 decoder: runs long enough to carry a 25+
# char credential. No sort-and-truncate (that could drop the very run carrying
# the secret); the number of runs is bounded by output length. The decoder runs
# BOTH scans and decodes the UNION of their runs, so a change can add
# reconstructions but never lose one:
#   _RUN_WIDE  the original scan: also swallows `=%.&;\` into a run, and the
#              decoder drops those bytes, so a blob with an interior `.` is
#              still reconstructed;
#   _RUN       breaks on base64-alphabet boundaries only (std and url
#              alphabets, `=` only as trailing padding), so a blob right after
#              `.`, `%`, `&`, `;`, `\` or `=` is decoded on its own instead of
#              shifted by the prefix (#439 gap 1).
# URL, HTML and JSON forms are undone by their own views in _decode_views.
_RUN_WIDE = __import__("re").compile(r"[A-Za-z0-9+/_=%.&;\\-]{12,}")
_RUN = __import__("re").compile(r"[A-Za-z0-9+/_-]{12,}={0,2}")
_JSON_ESCAPES = {"n": "\n", "t": "\t", "r": "\r", "b": "\b", "f": "\f",
                 '"': '"', "\\": "\\", "/": "/", "'": "'"}


def json_unescape(text: str) -> str:
    """Decode JSON/JS string escapes wherever they appear (not only in valid
    JSON): `\\uXXXX`, `\\xHH`, and the single-character escapes incl. `\\/` and
    `\\"`. A credential rendered with any of these is reconstructed."""
    out, i, n = [], 0, len(text)
    while i < n:
        c = text[i]
        if c == "\\" and i + 1 < n:
            nxt = text[i + 1]
            if nxt == "u" and i + 6 <= n:
                try:
                    out.append(chr(int(text[i + 2:i + 6], 16))); i += 6; continue
                except ValueError:
                    pass
            elif nxt == "x" and i + 4 <= n:
                try:
                    out.append(chr(int(text[i + 2:i + 4], 16))); i += 4; continue
                except ValueError:
                    pass
            elif nxt in _JSON_ESCAPES:
                out.append(_JSON_ESCAPES[nxt]); i += 2; continue
        out.append(c); i += 1
    return "".join(out)


def _base64_decodings(text: str) -> list[str]:
    """Decode EVERY base64/base64url-looking run in `text` (no length ranking),
    so a credential embedded in a base64 blob with a prefix or suffix is still
    reconstructed. Per-run work is bounded; the run count is bounded by the
    output length."""
    out = []
    for run in set(_RUN_WIDE.findall(text)) | set(_RUN.findall(text)):
        std = run.replace("-", "+").replace("_", "/").rstrip("=")
        if len(std) < 12:
            continue
        for cand in (std, std + "=" * (-len(std) % 4)):
            try:
                dec = base64.b64decode(cand, validate=False).decode("latin-1", "replace")
            except Exception:  # noqa: BLE001
                continue
            if dec:
                out.append(dec)
    return out


def _decode_views(text: str) -> set:
    """Every reconstruction of `text` reachable by undoing URL, HTML, JSON and
    base64 encodings, iterated to a fixed point (bounded rounds) so a nested
    encoding (base64 of a URL-escaped secret) is still undone."""
    seen = {text}
    frontier = [text]
    for _ in range(4):  # fixed point, bounded
        nxt = []
        for t in frontier:
            cands = []
            try:
                cands.append(urllib.parse.unquote(t))
                cands.append(urllib.parse.unquote(urllib.parse.unquote(t)))
            except Exception:  # noqa: BLE001
                pass
            try:
                cands.append(html.unescape(t))
            except Exception:  # noqa: BLE001
                pass
            try:
                cands.append(json_unescape(t))
            except Exception:  # noqa: BLE001
                pass
            cands.extend(_base64_decodings(t))
            for c in cands:
                if c and c not in seen:
                    seen.add(c)
                    nxt.append(c)
        if not nxt:
            break
        frontier = nxt
    return seen


# --------------------------------------------------------------------------- #
# The fake server. One handler, several modes, every request recorded.
# --------------------------------------------------------------------------- #

@dataclass
class Capture:
    """Records of what the connector sent, shared with the handler via closure."""
    requests: list = field(default_factory=list)  # (host, method, path, headers, body)
    lock: threading.Lock = field(default_factory=threading.Lock)


def _make_handler(capture: Capture, mode: str, status: int, sentinels: list[str],
                  redirect_to: str | None, body_echo: bool = True):
    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, *a):  # silence
            pass

        def handle(self):
            # A connector that closes the socket mid-response (it errored, or
            # stopped reading) must not print a stack trace from the daemon
            # thread. Swallow the expected socket errors; they are not findings.
            try:
                super().handle()
            except (ConnectionResetError, BrokenPipeError, OSError):
                pass

        def _record(self, body: bytes):
            with capture.lock:
                capture.requests.append((
                    self.headers.get("Host", ""),
                    self.command,
                    self.path,
                    {k: v for k, v in self.headers.items()},
                    body.decode("latin-1", "replace"),
                ))

        def _read_body(self) -> bytes:
            length = int(self.headers.get("Content-Length") or 0)
            return self.rfile.read(length) if length else b""

        def _token_json(self) -> bytes:
            return json.dumps({
                "access_token": "cls-fake-access-token",
                "token_type": "Bearer",
                "expires_in": 3600,
                "refresh_token": "cls-fake-refresh",
                "scope": "read",
            }).encode()

        def _echo_body(self) -> bytes:
            # A body that reflects every credential sentinel in every encoding,
            # plus a truncated form, so a connector that fails to withhold any
            # one of them surfaces it. Shaped as JSON an API error would return.
            # With body_echo False (the success probe) the body is credential
            # FREE, so printing legitimate 200 data is never a false positive;
            # the credential is reflected only in headers, which a healthy
            # connector does not print on success.
            if not body_echo:
                return b'{"data":[],"items":[],"results":[],"status":"ok"}'
            parts = []
            for s in sentinels:
                for name, enc in encodings(s).items():
                    parts.append(f'"{name}":"{enc}"')
                parts.append(f'"title":"Error for request {s[:12]}... truncated"')
            return ("{" + ",".join(parts) + "}").encode() if parts else b"{}"

        def _send(self, code: int, body: bytes, headers: dict[str, str]):
            self.send_response(code)
            for k, v in headers.items():
                self.send_header(k, v)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            try:
                self.wfile.write(body)
            except (BrokenPipeError, ConnectionResetError):
                pass

        def _dispatch(self):
            body = self._read_body()
            self._record(body)
            path_l = self.path.lower()
            # Any plausible token mint gets a token, so an OAuth connector can
            # proceed to the GET it is really being tested on.
            is_token = ("token" in path_l or "oauth" in path_l or "connect/token" in path_l
                        or b"grant_type" in body or "grant_type" in path_l)
            if is_token and self.command == "POST":
                self._send(200, self._token_json(), {"Content-Type": "application/json"})
                return
            if mode == "redirect" and redirect_to and "/redir-done" not in self.path:
                # One cross-host hop. The second host records whatever arrives.
                self._send(307, b"", {"Location": redirect_to + "/redir-done",
                                      "Content-Type": "application/json"})
                return
            # Headers that reflect credentials the way a proxy or API does.
            hdrs = {"Content-Type": "application/json"}
            for s in sentinels:
                hdrs["X-Request-Id"] = f"req-{s}"
                hdrs.setdefault("Location", f"/next?token={urllib.parse.quote(s)}")
            self._send(status, self._echo_body(), hdrs)

        do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = do_HEAD = _dispatch

    return Handler


@dataclass
class Server:
    srv: ThreadingHTTPServer
    capture: Capture
    url: str

    def stop(self):
        self.srv.shutdown()
        self.srv.server_close()


def start_server(mode: str = "echo", status: int = 200, sentinels: list[str] | None = None,
                 redirect_to: str | None = None, body_echo: bool = True) -> Server:
    """Start a fake server. mode: 'echo' (reflect sentinels), 'redirect' (one
    cross-host 307 then echo). status: the HTTP status the echo answers with.
    body_echo False reflects the sentinels in HEADERS only (credential-free
    body), for the success-path probe."""
    cap = Capture()
    handler = _make_handler(cap, mode, status, sentinels or [], redirect_to, body_echo)
    srv = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return Server(srv, cap, f"http://127.0.0.1:{srv.server_address[1]}")


def fresh_home(ctx: "Ctx", tag: str) -> str:
    """A new, empty HOME under the connector's temp dir. Each class (and each
    sub-probe that could mint and CACHE a credential) must run against an empty
    home: otherwise a token a prior class cached makes a later --dry-run find a
    cached token and skip the mint, so the result would depend on class order."""
    home = os.path.join(ctx.home, "homes", tag + "-" + base64.urlsafe_b64encode(os.urandom(4)).decode().rstrip("="))
    os.makedirs(home, exist_ok=True)
    return home


def free_host() -> str:
    """A loopback host:port that resolves but is NOT the capture server, used as
    the second host for the cross-host redirect probe."""
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return f"http://127.0.0.1:{port}"


# --------------------------------------------------------------------------- #
# Connector wiring, all read from the binary and the manifest (no Go imports).
# --------------------------------------------------------------------------- #

URL_HINTS = ("_BASE_URL", "_TOKEN_URL", "_AUTHORIZATION_URL", "_URL")
HOST_HINTS = ("_HOST", "_SERVER", "_API_HOST")
CRED_HINTS = ("TOKEN", "SECRET", "PASSWORD", "API_KEY", "APIKEY", "KEY", "CLIENT_ID",
              "IKEY", "SKEY", "JWT", "ACCESS", "BEARER", "AUTH", "CREDENTIAL",
              "PRIVATE", "SUBSCRIPTION_KEY", "INTEGRATION_CODE", "PUBLIC_KEY")
# Keys that are plain identifiers, never credentials, even though a hint matches.
NOT_CRED = ("USER_AGENT", "NO_CONFIG_WRITE", "MCP_READ_ONLY", "MCP_HTTP_TOKEN",
            "CLIENT_PROFILE", "TAIL_MAX_PAGES", "CONFIG")


@dataclass
class Wiring:
    env: dict[str, str]
    sentinels: dict[str, str]   # env var -> sentinel value (credentials only)
    url_vars: list[str]
    host_vars: list[str]


def read_manifest(slug: str) -> dict:
    p = registry.skill_path(slug) / "manifest.json"
    return json.loads(p.read_text(encoding="utf-8"))


def _manifest_env_keys(manifest: dict) -> list[str]:
    env = (manifest.get("server") or {}).get("mcp_config", {}).get("env", {}) or {}
    return list(env.keys())


def _user_config_sensitive(manifest: dict) -> set[str]:
    """ENV-var names whose user_config entry is marked sensitive."""
    env = (manifest.get("server") or {}).get("mcp_config", {}).get("env", {}) or {}
    uc = manifest.get("user_config") or {}
    out = set()
    for var, ref in env.items():
        if isinstance(ref, str) and ref.startswith("${user_config."):
            key = ref[len("${user_config."):].rstrip("}")
            if (uc.get(key) or {}).get("sensitive"):
                out.add(var)
    return out


def classify_env(manifest: dict) -> Wiring:
    """Point every URL/host variable at the capture server (filled later) and
    give every credential variable a unique sentinel. Everything else gets an
    inert placeholder so a required-var check does not abort the run."""
    keys = _manifest_env_keys(manifest)
    sensitive = _user_config_sensitive(manifest)
    url_vars, host_vars, sentinels, env = [], [], {}, {}
    for k in keys:
        up = k.upper()
        if any(up.endswith(h) for h in URL_HINTS):
            url_vars.append(k)
        elif any(up.endswith(h) for h in HOST_HINTS):
            host_vars.append(k)
        elif any(bad in up for bad in NOT_CRED):
            env[k] = ""  # inert; never a credential
        elif k in sensitive or any(h in up for h in CRED_HINTS):
            sentinels[k] = sentinel(k)
            env[k] = sentinels[k]
        else:
            # A required non-secret identifier (tenant, region, account id).
            env[k] = "cls-test"
    return Wiring(env=env, sentinels=sentinels, url_vars=url_vars, host_vars=host_vars)


# The fake server's token endpoint. A token or authorization URL variable points
# here, never at the server root, so a mint and a data write to `POST /` never
# share a path and write-safety cannot exempt that write as a mint (#439 gap 2).
# The fake server answers any POST whose path names a token with a token.
TOKEN_PATH = "/cls-harness/oauth/token"


def apply_urls(wiring: Wiring, url: str) -> dict[str, str]:
    """Return a full env dict with URL/host vars pointed at `url` (token and
    authorization URLs at its reserved TOKEN_PATH)."""
    host = url.split("://", 1)[1]
    env = dict(wiring.env)
    for k in wiring.url_vars:
        env[k] = url + TOKEN_PATH if k.upper().endswith(("_TOKEN_URL", "_AUTHORIZATION_URL")) else url
    for k in wiring.host_vars:
        env[k] = host
    return env


# --------------------------------------------------------------------------- #
# Surface discovery from agent-context.
# --------------------------------------------------------------------------- #

@dataclass
class Leaf:
    path: tuple[str, ...]
    use: str
    method: str          # GET / POST / ... / "" (novel)
    read_only: bool
    flags: list[dict]
    endpoint: str = ""   # pp:endpoint id (what an MCP _execute tool takes)
    api_path: str = ""   # pp:path template, e.g. /v2/alert/{uid}


def agent_context(cli_bin: Path, home: str, timeout: int = RUN_TIMEOUT) -> dict | None:
    env = {"PATH": os.environ.get("PATH", ""), "HOME": home}
    try:
        p = subprocess.run([str(cli_bin), "agent-context"], capture_output=True,
                           text=True, env=env, timeout=timeout)
    except (OSError, subprocess.SubprocessError):
        return None
    out = p.stdout.strip()
    start = out.find("{")
    if start < 0:
        return None
    try:
        return json.loads(out[start:])
    except json.JSONDecodeError:
        return None


def leaves(ctx: dict) -> list[Leaf]:
    """Flatten the agent-context command tree to its leaves."""
    found: list[Leaf] = []

    def walk(nodes, prefix):
        for c in nodes or []:
            path = prefix + (c["name"],)
            subs = c.get("subcommands") or []
            if subs:
                walk(subs, path)
                continue
            ann = c.get("annotations") or {}
            found.append(Leaf(
                path=path,
                use=c.get("use") or c["name"],
                method=(ann.get("pp:method") or "").upper(),
                read_only=(ann.get("mcp:read-only") == "true"),
                flags=c.get("flags") or [],
                endpoint=ann.get("pp:endpoint") or "",
                api_path=ann.get("pp:path") or "",
            ))
    walk(ctx.get("commands"), ())
    return found


def runnable_read(leaf: Leaf) -> bool:
    """A GET leaf with no required positional, safe to run argument-free."""
    if leaf.method and leaf.method != "GET":
        return False
    if "<" in leaf.use or "[" in leaf.use.replace("[flags]", ""):
        return False
    skip = {"doctor", "auth", "agent-context", "version", "completion", "help",
            "api", "mcp", "config", "profile", "sql", "analytics", "search",
            "recall", "learnings", "jobs", "playbook", "teach", "context"}
    return leaf.path[0] not in skip


# --------------------------------------------------------------------------- #
# MCP stdio driver.
# --------------------------------------------------------------------------- #

_HANDSHAKE = [
    {"jsonrpc": "2.0", "id": 1, "method": "initialize",
     "params": {"protocolVersion": "2024-11-05", "capabilities": {},
                "clientInfo": {"name": "class-harness", "version": "1"}}},
    {"jsonrpc": "2.0", "method": "notifications/initialized"},
]


def mcp_session(mcp_bin: Path, messages: list[dict], env: dict[str, str],
                timeout: int = RUN_TIMEOUT) -> tuple[list[dict], str, str]:
    """Run one stdio session. Returns (parsed responses, raw stdout, raw stderr).
    The raw streams are returned so a secret scan can see EVERYTHING the server
    emitted - a structuredContent block, a field this parser does not model, or
    anything written to stderr - not only result.content[].text."""
    payload = "".join(json.dumps(m) + "\n" for m in messages)
    try:
        p = subprocess.run([str(mcp_bin)], input=payload, capture_output=True,
                           text=True, env=env, timeout=timeout)
        stdout, stderr = p.stdout, p.stderr
    except subprocess.TimeoutExpired as e:
        stdout = e.stdout.decode() if isinstance(e.stdout, bytes) else (e.stdout or "")
        stderr = e.stderr.decode() if isinstance(e.stderr, bytes) else (e.stderr or "")
    except (OSError, subprocess.SubprocessError):
        return [], "", ""
    out = []
    for line in stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return out, stdout, stderr


def mcp_drive(mcp_bin: Path, messages: list[dict], env: dict[str, str],
              timeout: int = RUN_TIMEOUT) -> list[dict]:
    return mcp_session(mcp_bin, messages, env, timeout)[0]


def mcp_list_tools(mcp_bin: Path, env: dict[str, str]) -> list[dict]:
    msgs = _HANDSHAKE + [{"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}]
    for m in mcp_drive(mcp_bin, msgs, env):
        if m.get("id") == 2:
            return (m.get("result") or {}).get("tools") or []
    return []


def mcp_result_text(message: dict) -> str:
    """All text a tool answer carries. Reads result.content[].text AND the whole
    result and error serialized, so a credential in structuredContent or any
    other field is still in the scanned text (not only the content blocks)."""
    parts = []
    result = message.get("result") or {}
    for block in result.get("content") or []:
        if isinstance(block, dict) and "text" in block:
            parts.append(str(block["text"]))
    if result:
        parts.append(json.dumps(result, default=str))
    err = message.get("error")
    if isinstance(err, dict):
        parts.append(json.dumps(err, default=str))
    return "\n".join(parts)


def mcp_is_error(message: dict) -> bool:
    if isinstance(message.get("error"), dict):
        return True
    return bool((message.get("result") or {}).get("isError"))


# --------------------------------------------------------------------------- #
# Build + context object passed to each class.
# --------------------------------------------------------------------------- #

@dataclass
class Ctx:
    slug: str
    cli_bin: Path
    mcp_bin: Path
    home: str
    surface: dict
    manifest: dict
    wiring: Wiring


def build(slug: str, tmp: str) -> tuple[Path | None, Path | None, str | None]:
    """Build the CLI and MCP binaries into one dir, named by their REAL registry
    binary names, so the MCP server finds the CLI as a same-dir sibling (its
    SiblingCLIPath looks for `<cli_binary>` next to the mcp executable). Returns
    (cli, mcp, error)."""
    cli_dir = registry.skill_path(slug) / "cli"
    if not cli_dir.is_dir():
        return None, None, None
    try:
        cli_cmd, mcp_cmd = registry.cmd_dirs(slug)
    except SystemExit as e:
        return None, None, str(e)
    meta = registry.skills().get(slug, {})
    bindir = Path(tmp) / "bin"
    bindir.mkdir(parents=True, exist_ok=True)
    cli_bin = bindir / (meta.get("cli_binary") or "cli")
    mcp_bin = bindir / (meta.get("mcp_binary") or "mcp")
    for out, cmd in ((cli_bin, cli_cmd), (mcp_bin, mcp_cmd)):
        r = subprocess.run(["go", "build", "-o", str(out), cmd], cwd=cli_dir,
                           capture_output=True, text=True, timeout=BUILD_TIMEOUT)
        if r.returncode != 0:
            return None, None, f"go build {cmd}: {r.stderr.strip()[:300]}"
    return cli_bin, mcp_bin, None


def cli_path_env(slug: str) -> str:
    """The `<SLUG>_CLI_PATH` env var name a connector's MCP server reads as a
    fallback to locate its companion CLI."""
    return slug.upper().replace("-", "_") + "_CLI_PATH"


def make_ctx(slug: str, tmp: str) -> tuple[Ctx | None, str | None]:
    cli_bin, mcp_bin, err = build(slug, tmp)
    if err:
        return None, err
    if cli_bin is None:
        return None, None  # markdown-only / no CLI
    home = os.path.join(tmp, "home")
    os.makedirs(home, exist_ok=True)
    surface = agent_context(cli_bin, home)
    if surface is None:
        return None, f"{slug}: agent-context produced no JSON"
    manifest = read_manifest(slug)
    wiring = classify_env(manifest)
    return Ctx(slug, cli_bin, mcp_bin, home, surface, manifest, wiring), None


# --------------------------------------------------------------------------- #
# Class registry + baseline + driver.
# --------------------------------------------------------------------------- #

def _load_classes():
    """Import the class modules lazily so the harness file has no import cycle."""
    import class_secret_echo
    import class_auth_wire
    import class_write_safety
    return {
        "secret-echo": class_secret_echo,
        "auth-wire": class_auth_wire,
        "write-safety": class_write_safety,
    }


BASELINE_COMMENT = (
    "Connectors that FAIL a class in class_harness.py on main today, listed per class. "
    "Same burn-down model as tools/maintainer/security_gate_baseline.json (issue #429): a "
    "listed connector is reported, not failed, until its fix or reprint closes the class; any "
    "other connector failing the class fails CI. This list only SHRINKS. --update-baseline "
    "removes connectors that no longer fail and never adds one (a newly broken connector is a "
    "P1 to fix, not to baseline); --seed-baseline gives a class with no entry its first list; a "
    "listed connector that now passes but is still listed fails CI (remove it with "
    "--update-baseline). On a PR the rows read the baseline from the trusted base via "
    "--policy-base, so a PR cannot widen its own baseline."
)
CLASS_IDS = ("secret-echo", "auth-wire", "write-safety")


def _git_show(ref: str, path: str, repo: Path = ROOT) -> str | None:
    p = subprocess.run(["git", "show", f"{ref}:{path}"], capture_output=True,
                       text=True, cwd=repo, timeout=30)
    return p.stdout if p.returncode == 0 else None


def load_baseline(policy_base: str | None = None, repo: Path = ROOT) -> dict[str, set[str]]:
    """Return {class: set(slugs)}. With policy_base, read the file as it exists at
    that git ref (the trusted base), so a PR cannot widen its own baseline. Fail
    closed: a missing or unparseable baseline cannot silently excuse failures.
    `repo` is the checkout to read (the self-test passes a throwaway one)."""
    rel = BASELINE.relative_to(ROOT).as_posix()
    if policy_base:
        text = _git_show(policy_base, rel, repo)
        if text is None:
            raise SystemExit(f"class_harness: cannot read {rel} at --policy-base {policy_base!r}")
    else:
        path = repo / rel
        if not path.exists():
            raise SystemExit(f"class_harness: baseline file missing: {path}")
        text = path.read_text(encoding="utf-8")
    try:
        data = json.loads(text)
        classes = data["classes"]
    except (json.JSONDecodeError, KeyError, TypeError) as e:
        raise SystemExit(f"class_harness: baseline is malformed ({e})")
    out: dict[str, set[str]] = {}
    for c in CLASS_IDS:
        entries = classes.get(c, [])
        if not isinstance(entries, list):
            raise SystemExit(f"class_harness: baseline class {c!r} is not a list")
        out[c] = set(entries)
    return out


def save_baseline(classes: dict[str, set[str]]) -> None:
    data = {"_comment": BASELINE_COMMENT,
            "classes": {c: sorted(classes.get(c, set())) for c in CLASS_IDS}}
    BASELINE.write_text(json.dumps(data, indent=1, ensure_ascii=True) + "\n", encoding="utf-8")


def assert_baseline_shrinks(base_ref: str, repo: Path = ROOT) -> int:
    """Build-free CI guard, mirroring the security gate's baseline-readded check
    (issue #429): the working baseline may only REMOVE connectors relative to
    the baseline at base_ref, never add. An addition means a PR tried to baseline
    a connector it broke instead of fixing it. Fail closed if either side is
    unreadable."""
    try:
        head = load_baseline(repo=repo)
    except SystemExit as e:
        print(f"FAIL: {e}")
        return 1
    # Resolve the base ref FIRST. An unreadable ref fails closed; only a VALID
    # commit at which the baseline path is absent means "the PR that introduces
    # the baseline" (a pass). Without this split, _git_show returning None for a
    # bad ref would be mistaken for the new-baseline case and pass open.
    rp = subprocess.run(["git", "rev-parse", "--verify", "--quiet", f"{base_ref}^{{commit}}"],
                        capture_output=True, text=True, cwd=repo, timeout=30)
    if rp.returncode != 0 or not rp.stdout.strip():
        print(f"FAIL: cannot resolve --assert-baseline-shrinks base ref {base_ref!r} "
              f"(fails closed; a bad base never passes the shrink guard).")
        return 1
    base_sha = rp.stdout.strip()
    rel = BASELINE.relative_to(ROOT).as_posix()
    # Absence is PROVED from the base tree, never inferred from a failed read
    # (#439 gap 5): a tree that names the file but whose blob cannot be read
    # (an unhydrated partial clone) must fail closed, not pass as "new".
    ls = subprocess.run(["git", "ls-tree", "--name-only", base_sha, "--", rel],
                        capture_output=True, text=True, cwd=repo, timeout=30)
    if ls.returncode != 0:
        print(f"FAIL: cannot list {rel} at {base_ref!r} (fails closed): {ls.stderr.strip()[:200]}")
        return 1
    if not ls.stdout.strip():
        # Valid commit, and its tree has no baseline: this is the PR that
        # introduces it. Nothing to shrink against, so it is a pass.
        print(f"PASS: class_baseline.json is new at {base_ref}; nothing to shrink against.")
        return 0
    try:
        base = load_baseline(base_sha, repo)
    except SystemExit as e:
        print(f"FAIL: {e}")
        return 1
    added = []
    for c in CLASS_IDS:
        for slug in sorted(head[c] - base[c]):
            added.append((c, slug))
    if added:
        print("FAIL: class_baseline.json added connectors vs the trusted base; the baseline "
              "only shrinks. Fix the connector instead of baselining it:")
        for c, slug in added:
            print(f"  - {c}: {slug}")
        return 1
    removed = sum(len(base[c] - head[c]) for c in CLASS_IDS)
    print(f"PASS: baseline only shrank vs {base_ref} ({removed} connector(s) retired, none added).")
    return 0


# --------------------------------------------------------------------------- #
# Self-test support: stand-in connectors and the baseline guard's own proof.
# --------------------------------------------------------------------------- #

# Shared prelude for the stand-in CLI and MCP scripts the class self-tests run
# through the real class code. Stdlib only; proxies off so loopback is direct.
FIXTURE_PRELUDE = '''import json, os, sys, urllib.request, urllib.error
BASE = os.environ.get("ACME_BASE_URL", "")
HOME = os.environ.get("HOME", "")
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))
RO = os.environ.get("ACME_MCP_READ_ONLY") == "1"
def call(method, path, body=None):
    req = urllib.request.Request(BASE + path, data=body, method=method)
    try:
        with OPENER.open(req, timeout=10) as r:
            return r.status, dict(r.headers), r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read().decode()
    except Exception as e:
        return 0, {}, str(e)
def serve(tools, on_call):
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        msg = json.loads(line)
        mid, method = msg.get("id"), msg.get("method")
        if mid is None:
            continue
        if method == "initialize":
            res = {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}},
                   "serverInfo": {"name": "fixture", "version": "1"}}
        elif method == "tools/list":
            res = {"tools": tools}
        elif method == "tools/call":
            res = on_call(msg.get("params") or {})
        else:
            res = None
        out = {"jsonrpc": "2.0", "id": mid}
        if res is None:
            out["error"] = {"code": -32601, "message": "unknown method"}
        else:
            out["result"] = res
        print(json.dumps(out), flush=True)
'''

# The stand-in connector's manifest: a base URL, a token URL, one credential and
# the read-only switch, wired the way the real manifests are.
FIXTURE_MANIFEST = {"server": {"mcp_config": {"env": {
    "ACME_BASE_URL": "${user_config.base_url}",
    "ACME_TOKEN_URL": "${user_config.token_url}",
    "ACME_API_KEY": "${user_config.api_key}",
    "ACME_MCP_READ_ONLY": "${user_config.read_only}",
}}}, "user_config": {"api_key": {"sensitive": True}}}

FIXTURE_SURFACE = {"commands": [{"name": "items", "subcommands": [
    {"name": "list", "use": "list", "annotations": {
        "pp:method": "GET", "pp:path": "/v1/items", "mcp:read-only": "true"}}]}]}


def fixture_ctx(tmp: str, cli_body: str, mcp_body: str, surface: dict | None = None) -> Ctx:
    """A Ctx whose CLI and MCP server are small Python scripts (FIXTURE_PRELUDE
    plus the given body), so a self-test drives a class's real `run()` against a
    connector shape it chooses. Nothing is built."""
    bindir = Path(tmp) / "fixture-bin"
    bindir.mkdir(parents=True, exist_ok=True)
    paths = []
    for name, body in (("acme-cli", cli_body), ("acme-mcp", mcp_body)):
        script = bindir / name
        script.write_text(f"#!{sys.executable}\n{FIXTURE_PRELUDE}\n{body}\n", encoding="utf-8")
        script.chmod(0o755)
        paths.append(script)
    home = os.path.join(tmp, "fixture-home")
    os.makedirs(home, exist_ok=True)
    manifest = json.loads(json.dumps(FIXTURE_MANIFEST))
    return Ctx("acme", paths[0], paths[1], home, surface or FIXTURE_SURFACE, manifest,
               classify_env(manifest))


def self_test_baseline_guard() -> bool:
    """--assert-baseline-shrinks, both directions, in a throwaway repository.
    Gap 5 of #439: a base whose tree HAS the baseline but whose blob cannot be
    read (an unhydrated partial clone, here a deleted loose object) must fail
    closed, never read as "the file is new at the base"."""
    import contextlib
    import io
    ok = True
    rel = BASELINE.relative_to(ROOT).as_posix()
    with tempfile.TemporaryDirectory(prefix="class-guard-selftest-") as tmp:
        repo = Path(tmp)
        env = {**os.environ, "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.invalid",
               "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.invalid",
               "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"}

        def git(*args: str) -> str:
            return subprocess.run(["git", *args], cwd=tmp, env=env, check=True,
                                  capture_output=True, text=True, timeout=30).stdout.strip()

        def write(classes: dict[str, list[str]]) -> None:
            path = repo / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(json.dumps({"classes": classes}) + "\n", encoding="utf-8")

        def guard(ref: str) -> int:
            with contextlib.redirect_stdout(io.StringIO()):
                return assert_baseline_shrinks(ref, repo)

        def expect(cond: bool, label: str) -> None:
            nonlocal ok
            if not cond:
                print(f"    baseline-guard self-test: {label}")
                ok = False

        git("init", "-q")
        (repo / "README").write_text("x\n", encoding="utf-8")
        git("add", "-A")
        git("commit", "-q", "-m", "no baseline")
        no_file = git("rev-parse", "HEAD")
        write({"secret-echo": ["a", "b"], "auth-wire": [], "write-safety": []})
        git("add", "-A")
        git("commit", "-q", "-m", "baseline")
        with_file = git("rev-parse", "HEAD")
        write({"secret-echo": ["a"], "auth-wire": [], "write-safety": []})
        expect(guard(with_file) == 0, "a baseline that only shrank must pass")
        write({"secret-echo": ["a", "b", "c"], "auth-wire": [], "write-safety": []})
        expect(guard(with_file) == 1, "a widened baseline must fail")
        expect(guard(no_file) == 0, "a base commit without the file (the introducing PR) must pass")
        expect(guard("refs/heads/no-such-base") == 1, "an unresolvable base ref must fail closed")
        # The base tree names the file but its blob is gone (unhydrated partial
        # clone): the widened baseline must fail closed, not pass as "new".
        blob = git("rev-parse", f"{with_file}:{rel}")
        (repo / ".git" / "objects" / blob[:2] / blob[2:]).unlink()
        expect(guard(with_file) == 1,
               "a base that has the baseline but whose blob cannot be read must fail closed "
               "(not read as 'new at the base')")
    return ok


def run_slug(slug: str, which: list[str], classes: dict) -> dict[str, list[str]]:
    """Run the selected classes against one connector. Returns {class: findings}."""
    results: dict[str, list[str]] = {}
    with tempfile.TemporaryDirectory(prefix=f"class-{slug}-") as tmp:
        ctx, err = make_ctx(slug, tmp)
        if ctx is None:
            if err is None:
                return {}  # markdown-only skill, nothing to test
            return {c: [f"[{slug}] could not build or inspect: {err}"] for c in which}
        for name in which:
            mod = classes[name]
            try:
                results[name] = mod.run(ctx)
            except Exception as exc:  # noqa: BLE001 - a crashing class is a failure
                results[name] = [f"[{slug}] class {name} crashed: {exc}"]
    return results


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--slug", action="append", help="check one skill (repeatable)")
    ap.add_argument("--all", action="store_true", help="check every connector")
    ap.add_argument("--class", dest="only", action="append",
                    help="run only this class (secret-echo / auth-wire / write-safety)")
    ap.add_argument("--report", action="store_true",
                    help="print per-class fleet counts; do not fail on baselined connectors")
    ap.add_argument("--policy-base", metavar="REF",
                    help="read the baseline from this git ref (the trusted base), not the "
                         "working file, so a PR cannot widen its own baseline")
    ap.add_argument("--update-baseline", action="store_true",
                    help="retire baselined connectors that no longer fail (removes only, never adds)")
    ap.add_argument("--seed-baseline", action="store_true",
                    help="give a class with no entry its first list from a fleet run (never removes)")
    ap.add_argument("--self-test", action="store_true",
                    help="prove each class fires on a broken shape and is silent on a healthy one")
    ap.add_argument("--assert-baseline-shrinks", metavar="REF", dest="shrink_base",
                    help="build-free guard: fail if the working baseline added any connector "
                         "the baseline at REF did not have (a PR cannot widen its own baseline)")
    ap.add_argument("-v", "--verbose", action="store_true")
    args = ap.parse_args(argv)

    if args.shrink_base is not None:
        return assert_baseline_shrinks(args.shrink_base)

    if shutil.which("go") is None:
        print("SKIP: go is not on PATH; cannot build any connector.")
        return 0

    classes = _load_classes()

    if args.self_test:
        ok = self_test_baseline_guard()
        print(f"  {'ok  ' if ok else 'FAIL'} baseline-guard self-test")
        for name, mod in classes.items():
            passed = mod.self_test()
            print(f"  {'ok  ' if passed else 'FAIL'} {name} self-test")
            ok = ok and passed
        print("PASS: every class fires on a broken shape and is silent on a healthy one"
              if ok else "FAIL: a class self-test did not prove both directions")
        return 0 if ok else 1

    which = args.only or list(classes.keys())
    for c in which:
        if c not in classes:
            ap.error(f"unknown class {c!r}; choose from {sorted(classes)}")

    meta = registry.skills()
    fleet_run = args.update_baseline or args.seed_baseline or args.all or args.report
    if fleet_run:
        slugs = sorted(s for s in meta if not registry.is_markdown_only(s))
    elif args.slug:
        slugs = args.slug
    else:
        ap.error("pass --slug <slug>, --all, or --report")

    baseline = load_baseline(args.policy_base)

    # Fleet/report runs parallelise so the whole-fleet pass stays minutes, not
    # an hour; a single --slug run (the CI matrix row) stays serial.
    fleet: dict[str, dict[str, list[str]]] = {}
    if len(slugs) > 1:
        from concurrent.futures import ThreadPoolExecutor
        workers = min(8, (os.cpu_count() or 4))
        with ThreadPoolExecutor(max_workers=workers) as pool:
            for slug, res in zip(slugs, pool.map(lambda s: run_slug(s, which, classes), slugs)):
                fleet[slug] = res
    else:
        for slug in slugs:
            fleet[slug] = run_slug(slug, which, classes)

    # Shape results: per class, which connectors FAIL (gating findings) and which
    # carry only advisory NOTEs. A finding beginning "NOTE:" is advisory: it is
    # a coverage or informational signal, never a CI failure and never baselined
    # (a connector a class could not exercise must not read as a clean pass).
    def is_note(line: str) -> bool:
        return line.lstrip().startswith("NOTE:")

    failing: dict[str, dict[str, list[str]]] = {c: {} for c in which}
    notes: dict[str, dict[str, list[str]]] = {c: {} for c in which}
    for slug, res in fleet.items():
        for c, findings in res.items():
            gating = [f for f in findings if not is_note(f)]
            advisory = [f for f in findings if is_note(f)]
            if gating:
                failing[c][slug] = gating
            if advisory:
                notes[c][slug] = advisory

    if args.seed_baseline:
        # First list for a class that has no entry yet. Never touches a class
        # that already has entries (that is --update-baseline's job).
        new = {c: set(baseline.get(c, set())) for c in CLASS_IDS}
        seeded = []
        for c in which:
            if baseline.get(c):
                print(f"  {c}: already has {len(baseline[c])} entries; "
                      f"--seed-baseline only seeds an empty class (use --update-baseline to shrink)")
                continue
            new[c] = set(failing[c].keys())
            seeded.append(f"{c} ({len(new[c])})")
        save_baseline(new)
        print(f"seeded {BASELINE}: {', '.join(seeded) or '(nothing; all non-empty)'}")
        return 0

    if args.update_baseline:
        # Retire-only: keep a listed connector only while it STILL fails; never
        # add a newly broken one (that must be fixed, not baselined).
        new = {c: set(baseline.get(c, set())) for c in CLASS_IDS}
        added_blocked = {}
        for c in which:
            still = set(baseline.get(c, set())) & set(failing[c].keys())
            retired = sorted(set(baseline.get(c, set())) - still)
            would_add = sorted(set(failing[c].keys()) - set(baseline.get(c, set())))
            new[c] = still
            print(f"  {c}: retired {len(retired)} burned-down, {len(still)} remain"
                  + (f"; {len(would_add)} newly failing NOT added (fix them): {would_add}"
                     if would_add else ""))
            if would_add:
                added_blocked[c] = would_add
        save_baseline(new)
        if added_blocked:
            print("ERROR: connectors fail a class but are not baselined; --update-baseline "
                  "never adds them. Fix the connector or seed a NEW class only.")
            return 1
        return 0

    # Report / gate.
    exit_code = 0
    for c in which:
        listed = set(baseline.get(c, []))
        fails = failing[c]
        unlisted = {s: f for s, f in fails.items() if s not in listed}
        fixed_still_listed = sorted(listed - set(fails.keys())
                                    - {s for s in listed if s not in fleet})
        c_notes = notes[c]
        print(f"\n== class {c}: {len(fails)} failing / {len(fleet)} checked "
              f"({len(listed)} baselined, {len(c_notes)} advisory)")
        if args.verbose or args.report:
            for s in sorted(fails):
                tag = "baselined" if s in listed else "NEW"
                print(f"   {tag:9} {s}")
                for line in fails[s]:
                    print(f"     - {line}")
            for s in sorted(c_notes):
                for line in c_notes[s]:
                    print(f"   note      {s}: {line}")
        if not args.report:
            for s in sorted(unlisted):
                exit_code = 1
                print(f"   FAIL (not baselined): {s}")
                for line in unlisted[s]:
                    print(f"     - {line}")
            # A connector that was baselined but now passes must be removed from
            # the list, so the list only shrinks and a regression cannot hide.
            only_slug_run = args.slug and not args.all
            if fixed_still_listed and not only_slug_run:
                exit_code = 1
                print(f"   FAIL: these connectors now PASS {c} but are still in the "
                      f"baseline; remove them (run --update-baseline): {fixed_still_listed}")

    if args.report:
        print("\n(report mode: baselined connectors are not failures)")
        return 0
    if exit_code == 0:
        print("\nPASS: no unlisted connector fails any class, and no listed one has silently fixed.")
    return exit_code


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
