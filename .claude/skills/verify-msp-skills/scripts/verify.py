#!/usr/bin/env python3
"""verify.py - drive one msp-skills connector the way a user does, in isolation.

The feature map in ../features/ says what each check proves; this is its
harness. It builds the connector from this checkout the way release.yml does,
installs it through the connector's own install.sh, and drives the CLI
(subprocess), the MCP server over stdio (JSON-RPC) and over HTTP (plain HTTP).
Every driven process runs with HOME and the XDG dirs in a scratch directory,
the vendor replaced by a local fake on 127.0.0.1, and (macOS) inside a
sandbox-exec profile that refuses every non-loopback connection.

    python3 .claude/skills/verify-msp-skills/scripts/verify.py doctor --slug cipp
    python3 .claude/skills/verify-msp-skills/scripts/verify.py run --slug cipp
    python3 .claude/skills/verify-msp-skills/scripts/verify.py run --slug cipp --features 2,3
    python3 .claude/skills/verify-msp-skills/scripts/verify.py run --slug cipp --gates
    python3 .claude/skills/verify-msp-skills/scripts/verify.py report
    python3 .claude/skills/verify-msp-skills/scripts/verify.py cleanup --stale
    python3 .claude/skills/verify-msp-skills/scripts/verify.py self-test

One verdict per feature:
  PASS   the harness ran and the check held
  FAIL   the harness ran and the check did not hold (the observed behavior is recorded)
  ERROR  the harness broke; the verdict says nothing about the connector
  SKIP   the connector has no such surface (the reason is recorded)

`run` exits 0 when every feature is PASS or SKIP, 1 when any is FAIL and none
is ERROR, 2 when any is ERROR or isolation could not be proven, 64 on a usage
error. CI's gates (build, vet, tests, MCP gate, doctor truth) are NOT repeated
here: `--gates` calls tools/maintainer/verify_all.sh --slug for them.

Stdlib only. Reuses tools/maintainer: check_mcp_gate's JSON-RPC pieces,
class_harness's manifest env wiring, registry, and the installer release
fixture installer_fixtures/fake_github.py.
"""

from __future__ import annotations

import argparse
import base64
import datetime as dt
import fcntl
import hashlib
import hmac
import http.client
import ipaddress
import json
import os
import platform
import pwd
import queue
import re
import secrets
import shutil
import signal
import socket
import ssl
import subprocess
import sys
import tempfile
import threading
import time
import traceback
import urllib.error
import urllib.parse
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

HERE = Path(__file__).resolve()
SKILL_ROOT = HERE.parents[1]
REPO = HERE.parents[4]
MAINT = REPO / "tools" / "maintainer"
sys.path.insert(0, str(MAINT))
import registry  # noqa: E402  (repo tool: the connector registry)
import check_mcp_gate as gate  # noqa: E402  (repo tool: stdio JSON-RPC pieces)
import class_harness as wiring  # noqa: E402  (repo tool: manifest env wiring)
import kit_tls  # noqa: E402  (repo tool: a fake vendor over TLS for a kit adopter)

FAKE_GITHUB = MAINT / "installer_fixtures" / "fake_github.py"
VERIFY_ALL = MAINT / "verify_all.sh"
EVIDENCE_ROOT = REPO / ".verify-evidence"
SCRATCH_PREFIX = "verify-msp-"
FEATURES = [
    (1, "install", "install path"),
    (2, "mcp-stdio", "MCP stdio surface"),
    (3, "mcp-http", "MCP HTTP transport"),
    (4, "write-gating", "write gating"),
    (5, "read-only", "read-only switch"),
    (6, "local-data", "local data"),
]
PASS, FAIL, ERROR, SKIP = "PASS", "FAIL", "ERROR", "SKIP"
EXIT_OK, EXIT_FAIL, EXIT_HARNESS, EXIT_USAGE = 0, 1, 2, 64
SANDBOX_EXEC = "/usr/bin/sandbox-exec"
# Loopback only: the fake vendor, the release fixture and the HTTP transport
# under test all live on 127.0.0.1. Anything else (a vendor host, DNS) fails.
SANDBOX_PROFILE = """(version 1)
(allow default)
(deny network-outbound)
(allow network-outbound (remote ip "localhost:*"))
"""
PROBE_VALUE = "verify-probe"
WRITE_QUERIES = ("create", "add", "update", "delete", "remove", "set", "edit", "assign", "reset", "new")
WRITE_METHODS = ("POST", "PUT", "PATCH", "DELETE")


class HarnessError(Exception):
    """A step the harness could not run. Its feature reports ERROR, never FAIL."""


def stamp() -> str:
    return dt.datetime.now(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


# The per-run secrets (feature 3's HTTP bearer token and its wrong token).
# Ctx.secrets is this list. Wherever verify.py cuts, slices or formats a value
# for evidence, a verdict or the console, it redacts the FULL value first and
# cuts after (redact(x)[:n]): a cut secret cannot be matched any more. Values a
# check asserts on are never redacted.
RUN_SECRETS: list[str] = []


def redact(text: str, secret_list: list[str] | None = None) -> str:
    for x in RUN_SECRETS if secret_list is None else secret_list:
        if x:
            text = text.replace(x, "<redacted>")
    return text


def tail(text: str | None, n: int = 3, width: int = 240) -> str:
    lines = [ln.strip() for ln in redact(text or "").splitlines() if ln.strip()]
    return " | ".join(lines[-n:])[:width]


def write_json(path: Path, obj) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, indent=2, default=str) + "\n", encoding="utf-8")


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def parse_json_lenient(text: str):
    text = (text or "").strip()
    try:
        return json.loads(text)
    except ValueError:
        a, b = text.find("{"), text.rfind("}")
        if 0 <= a < b:
            try:
                return json.loads(text[a:b + 1])
            except ValueError:
                return None
    return None


def free_port() -> int:
    s = socket.socket()
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


def port_busy(port: int) -> bool:
    for fam, host in ((socket.AF_INET, "0.0.0.0"), (socket.AF_INET, "127.0.0.1")):
        s = socket.socket(fam, socket.SOCK_STREAM)
        try:
            s.bind((host, port))
        except OSError:
            return True
        finally:
            s.close()
    return False


def pid_alive(pid: int) -> bool:
    """True for a running process; a zombie (exited, not yet reaped) is not."""
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    try:
        r = subprocess.run(["ps", "-p", str(pid), "-o", "stat="], capture_output=True, text=True, timeout=30)
    except subprocess.TimeoutExpired:
        return True  # unknown: treat as alive, so nothing is reported stopped that was not
    return bool(r.stdout.strip()) and not r.stdout.strip().startswith("Z")


def kill_group(pid: int, proc: subprocess.Popen | None = None) -> None:
    """Stop a process this run started, and its children (every driven process
    leads its own session). Never a process found by name. With the Popen
    handle the process is also reaped."""
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            if os.getpgid(pid) == pid:
                os.killpg(pid, sig)
            else:
                os.kill(pid, sig)
        except (ProcessLookupError, PermissionError):
            pass  # gone, or (macOS) a group left holding only a zombie
        for _ in range(30):
            if (proc.poll() is not None) if proc is not None else not pid_alive(pid):
                return
            time.sleep(0.1)


def host_platform() -> tuple[str, str]:
    osname = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system(), platform.system().lower())
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(
        platform.machine().lower(), platform.machine().lower())
    return osname, arch


def git_head() -> str:
    try:
        sha = subprocess.run(["git", "-C", str(REPO), "rev-parse", "--short=12", "HEAD"],
                             capture_output=True, text=True, timeout=30).stdout.strip()
        dirty = subprocess.run(["git", "-C", str(REPO), "status", "--porcelain", "--untracked-files=no"],
                               capture_output=True, text=True, timeout=60).stdout.strip()
        return f"{sha}{' (dirty)' if dirty else ''}"
    except (OSError, subprocess.SubprocessError):
        return "unknown"


# --------------------------------------------------------------------------- #
# The fake vendor: every request on 127.0.0.1 is logged. A proxy request for
# another host (how a client honoring HTTP(S)_PROXY tries to leave the
# machine) is refused and logged as an egress attempt. For a connector that
# adopted the credential kit it serves https (kit_tls): the kit refuses to send
# a held credential over plain http, so a plain vendor would never be reached.
# Its proxy port stays plain http either way, so the egress guard is the same.
# --------------------------------------------------------------------------- #

class _QuietHTTPServer(ThreadingHTTPServer):
    def handle_error(self, request, client_address) -> None:
        # The default prints a traceback whose frames can carry request data.
        print("verify: the fake vendor's request handler raised; request data not logged", file=sys.stderr)


def dsm_params(target: str, content_type: str, body: bytes) -> dict | None:
    """The lower-cased query and form parameters of a Synology DSM WebAPI call:
    an origin-form request target under /webapi/ whose `api` parameter names a
    SYNO.* API. A body counts only when it is declared a form. None for every
    other request, including a proxy-style target that names another host."""
    path, _, query = target.partition("?")
    if not path.lower().startswith("/webapi/"):
        return None
    sources = [query]
    if content_type.split(";")[0].strip().lower() == "application/x-www-form-urlencoded":
        sources.append(body.decode("utf-8", "replace"))
    params: dict[str, str] = {}
    for raw in sources:
        for k, v in urllib.parse.parse_qsl(raw, keep_blank_values=True):
            params.setdefault(k.lower(), v.lower())
    return params if params.get("api", "").startswith("syno.") else None


def dsm_answer(params: dict) -> dict:
    """The {"success":true,"data":...} envelope a Synology DSM WebAPI answers
    with: SYNO.API.Info lists the login API, SYNO.API.Auth login mints an
    inert session, and every other call returns an empty data object."""
    if params["api"] == "syno.api.info":
        return {"success": True, "data": {"SYNO.API.Auth": {"path": "entry.cgi", "minVersion": 1, "maxVersion": 7}}}
    if params["api"] == "syno.api.auth" and params.get("method") == "login":
        return {"success": True, "data": {"sid": "verify-fake-dsm-sid", "synotoken": "verify-fake-dsm-synotoken"}}
    return {"success": True, "data": {}}


def vcenter_session_login(command: str, target: str) -> bool:
    """A vSphere Automation API login: POST /api/session (vSphere 7.0 U2 and
    later). vCenter answers 201 with the session id as a bare JSON string, and
    its clients read the id from that string; a generic {} is no session. Only
    an origin-form target counts, so a proxy-style target naming another host
    stays the egress refusal above."""
    return command == "POST" and target.partition("?")[0] == "/api/session"


NCENTRAL_AUTH_PATHS = ("/auth/authenticate", "/auth/refresh")


def ncentral_auth(command: str, target: str) -> bool:
    """N-central's two-step auth: POST <base>/auth/authenticate exchanges the
    User-API JWT, and POST <base>/auth/refresh renews, for an access token in
    N-central's own envelope; its client reads no other shape, so a generic {}
    is no token. Only an origin-form target counts, so a proxy-style target
    naming another host stays the egress refusal above."""
    path = target.partition("?")[0]
    return command == "POST" and path.startswith("/") and path.endswith(NCENTRAL_AUTH_PATHS)


def ncentral_tokens() -> dict:
    """The body of N-central's POST /api/auth/authenticate, with inert tokens."""
    return {"tokens": {"access": {"token": "verify-fake-ncentral-access", "type": "Bearer", "expirySeconds": 3600},
                       "refresh": {"token": "verify-fake-ncentral-refresh", "type": "Body", "expirySeconds": 90000}}}


class FakeVendor:
    def __init__(self, log: Path, egress_log: Path, tls: tuple[str, str] | None = None):
        self.lock = threading.Lock()
        self.requests: list[dict] = []
        self.egress: list[dict] = []
        self.log, self.egress_log = log, egress_log
        vendor = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *a):  # quiet; the JSONL log is the record
                pass

            def _reply(self, code: int, obj) -> None:
                data = b"" if self.command == "HEAD" else json.dumps(obj).encode()
                self.send_response(code)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                if data:
                    self.wfile.write(data)

            def _handle(self) -> None:
                n = int(self.headers.get("Content-Length") or 0)
                body = self.rfile.read(n) if n > 0 else b""
                if self.command == "CONNECT" or self.path.startswith(("http://", "https://")):
                    vendor.record_egress(self.command, self.path)
                    self.close_connection = True
                    return self._reply(403, {"error": "verify-msp-skills: egress refused"})
                vendor.record(self, body)
                rpc = parse_json_lenient(body.decode("utf-8", "replace")) if body else None
                if isinstance(rpc, dict) and rpc.get("jsonrpc") == "2.0" and "id" in rpc:
                    # A JSON-RPC API (e.g. bitdefender) reads with POST and wants an envelope.
                    return self._reply(200, {"jsonrpc": "2.0", "id": rpc["id"], "result": []})
                low = self.path.lower()
                # An OAuth token mint: a token-ish path, or a grant_type in the body
                # (a TOKEN_URL variable points at this server's root).
                if self.command == "POST" and ("token" in low or "oauth" in low or b"grant_type" in body):
                    return self._reply(200, {"access_token": "verify-fake-access-token",
                                             "token_type": "Bearer", "expires_in": 3600})
                dsm = dsm_params(self.path, self.headers.get("Content-Type") or "", body)
                if dsm is not None:
                    # A Synology DSM WebAPI answers every call in its own envelope,
                    # and its client refuses a host that does not (it sends no
                    # login until SYNO.API.Info lists SYNO.API.Auth).
                    return self._reply(200, dsm_answer(dsm))
                if vcenter_session_login(self.command, self.path):
                    return self._reply(201, "verify-fake-vcenter-session")
                if ncentral_auth(self.command, self.path):
                    return self._reply(200, ncentral_tokens())
                if self.command == "GET":
                    return self._reply(200, [])
                return self._reply(200, {})

            do_GET = do_POST = do_PUT = do_PATCH = do_DELETE = _handle
            do_HEAD = do_OPTIONS = do_CONNECT = _handle

        self.srv = _QuietHTTPServer(("127.0.0.1", 0), Handler)
        self.srv.daemon_threads = True
        self.url = f"http://127.0.0.1:{self.srv.server_address[1]}"
        self.proxy_url, self.proxy_srv, self.ca = self.url, None, None
        if tls is not None:
            self.proxy_srv = _QuietHTTPServer(("127.0.0.1", 0), Handler)
            self.proxy_srv.daemon_threads = True
            self.proxy_url = f"http://127.0.0.1:{self.proxy_srv.server_address[1]}"
            threading.Thread(target=self.proxy_srv.serve_forever, daemon=True).start()
            kit_tls.serve_tls(self.srv, tls)
            self.url = f"https://127.0.0.1:{self.srv.server_address[1]}"
            self.ca = tls[0]
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()

    def record(self, h: BaseHTTPRequestHandler, body: bytes) -> None:
        hdrs = {k.lower(): v for k, v in h.headers.items()}
        rec = {"t": stamp(), "method": h.command, "path": h.path,
               "header_names": sorted(hdrs),
               "auth_scheme": (hdrs.get("authorization") or "").split(" ")[0] or None,
               "body_bytes": len(body), "body_head": redact(body.decode("utf-8", "replace"))[:300]}
        with self.lock:
            rec["seq"] = len(self.requests)
            self.requests.append(rec)
            with self.log.open("a", encoding="utf-8") as f:
                f.write(json.dumps(rec) + "\n")

    def record_egress(self, method: str, target: str) -> None:
        rec = {"t": stamp(), "method": method, "target": target}
        with self.lock:
            self.egress.append(rec)
            with self.egress_log.open("a", encoding="utf-8") as f:
                f.write(json.dumps(rec) + "\n")

    def mark(self) -> int:
        with self.lock:
            return len(self.requests)

    def since(self, mark: int) -> list[dict]:
        with self.lock:
            return list(self.requests[mark:])

    def stop(self) -> None:
        for srv in (self.srv, self.proxy_srv):
            if srv is not None:
                srv.shutdown()
                srv.server_close()


# --------------------------------------------------------------------------- #
# Connectors whose vendor is not an HTTP API, declared here and never inferred
# from manifest values or a slug's text. A WinRM connector talks WS-Management
# only to the hosts its config file's [[servers]] entries name, and refuses any
# base URL, so feature 2 starts a loopback WinRM stub, writes a scratch config
# naming it (pinned to the stub's certificate), and drives the declared read
# tools. config_env selects that config file; username_var and password_var
# are the shared WinRM account, which keeps the manifest's stand-in values.
# --------------------------------------------------------------------------- #

WINRM_CONNECTORS = {
    "windows-server": {"config_env": "WINDOWS_SERVER_CONFIG", "username_var": "WINDOWS_SERVER_USERNAME",
                       "password_var": "WINDOWS_SERVER_PASSWORD", "read_tools": ("volumes_list",)},
}
WINRM_ACTION_RE = re.compile(r"<a:Action[^>]*>([^<]+)</a:Action>")
WINRM_SHELL_RE = re.compile(r'<w:Selector Name="ShellId">([^<]+)</w:Selector>')
WINRM_SCRIPT_RE = re.compile(r'<rsp:Variable Name="WSPP_SCRIPT">([^<]*)</rsp:Variable>')
WINRM_ARGS_RE = re.compile(r"<rsp:Arguments>([^<]*)</rsp:Arguments>")
WINRM_REFUSED = '{"ok":false,"error":{"kind":"refused","message":"script integrity check failed"}}'


def winrm_volumes(marker: str) -> str:
    """The stub's answer to a pinned script: one volume row in the scripts'
    {"ok":true,"data":...} envelope, carrying this run's marker."""
    row = {"drive_letter": "C", "unique_id": marker, "label": marker, "file_system": "NTFS",
           "size_bytes": 107374182400, "free_bytes": 45097156608, "percent_free": 42.0, "health_status": "Healthy"}
    return json.dumps({"ok": True, "data": [row]})


class FakeWinRM:
    """A loopback WS-Management listener over TLS, the protocol shape of the
    windows-server connector's own newKitWinRM test stub. Every request must
    carry Basic auth with the configured account, or it is answered 401 and
    nothing is served. Create opens a shell and keeps the SHA-256 of the
    WSPP_SCRIPT it carries; Command must carry the bootstrap that pins that
    hash, as a real server's bootstrap refuses a script that does not match;
    Receive answers `reply` (the run's volume row) only for a pinned script,
    else the bootstrap's refusal; Signal and Delete close. Each request is
    logged with its action, whether it was authorized and whether it served
    `reply`; never a credential."""

    def __init__(self, cert: tuple[str, str], username: str, password: str, log: Path, reply: str | None = None):
        self.marker = "verify-winrm-" + secrets.token_hex(6)
        self.reply = winrm_volumes(self.marker) if reply is None else reply
        self.basic = "Basic " + base64.b64encode(f"{username}:{password}".encode()).decode()
        self.lock = threading.RLock()  # send() records while a branch holds it
        self.requests: list[dict] = []
        self.shells: dict[str, dict] = {}
        self.log = log
        stub = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def log_message(self, *a):
                pass

            def _reply(self, code: int, inner: str | None) -> None:
                data = b"" if inner is None else (
                    '<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" '
                    'xmlns:rsp="http://schemas.microsoft.com/wbem/wsman/1/windows/shell" '
                    'xmlns:w="http://schemas.dmtf.org/wbem/wsman/1/wsman.xsd" '
                    'xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" '
                    'xmlns:x="http://schemas.xmlsoap.org/ws/2004/09/transfer"><s:Body>'
                    + inner + "</s:Body></s:Envelope>").encode()
                self.send_response(code)
                if code == 401:
                    self.send_header("WWW-Authenticate", "Basic")
                self.send_header("Content-Type", "application/soap+xml;charset=UTF-8")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                if data:
                    self.wfile.write(data)

            def do_POST(self) -> None:
                n = int(self.headers.get("Content-Length") or 0)
                body = self.rfile.read(n).decode("utf-8", "replace") if n > 0 else ""
                m = WINRM_ACTION_RE.search(body)
                action = m.group(1).rsplit("/", 1)[-1] if m else None
                authorized = hmac.compare_digest(self.headers.get("Authorization") or "", stub.basic)
                served, sent = False, False

                def send(code: int, inner: str | None) -> None:
                    # Record before any byte of the reply: a client that has read a served reply must find
                    # it in stub.since(), or a correct read could FAIL (codex).
                    nonlocal sent
                    stub.record(action, authorized, served)
                    sent = True
                    self._reply(code, inner)

                try:
                    if not authorized:
                        return send(401, None)
                    if action is None:
                        return send(400, None)
                    s = WINRM_SHELL_RE.search(body)
                    shell = s.group(1) if s else ""
                    with stub.lock:
                        if action == "Create":
                            v = WINRM_SCRIPT_RE.search(body)
                            try:
                                digest = hashlib.sha256(base64.b64decode(v.group(1), validate=True)).hexdigest().upper()
                            except (AttributeError, ValueError):
                                digest = None
                            shell = f"SHELL-{len(stub.shells) + 1}"
                            stub.shells[shell] = {"digest": digest, "pinned": False}
                            return send(200, '<x:ResourceCreated><a:ReferenceParameters><w:SelectorSet>'
                                                    f'<w:Selector Name="ShellId">{shell}</w:Selector>'
                                                    "</w:SelectorSet></a:ReferenceParameters></x:ResourceCreated>")
                        if action == "Command":
                            sh = stub.shells.get(shell) or {}
                            a = WINRM_ARGS_RE.search(body)
                            enc = re.search(r"-EncodedCommand\s+(\S+)", a.group(1)) if a else None
                            try:
                                boot = base64.b64decode(enc.group(1), validate=True).decode("utf-16-le") if enc else ""
                            except ValueError:
                                boot = ""
                            sh["pinned"] = bool(sh.get("digest")) and f"'{sh['digest']}'" in boot
                            return send(200, "<rsp:CommandResponse><rsp:CommandId>CMD-1</rsp:CommandId>"
                                                    "</rsp:CommandResponse>")
                        if action == "Receive":
                            served = (stub.shells.get(shell) or {}).get("pinned", False)
                            out = stub.reply if served else WINRM_REFUSED
                            return send(200, '<rsp:ReceiveResponse><rsp:Stream Name="stdout" CommandId="CMD-1">'
                                                    + base64.b64encode(out.encode()).decode()
                                                    + '</rsp:Stream><rsp:Stream Name="stderr" CommandId="CMD-1" End="true">'
                                                    '</rsp:Stream><rsp:CommandState CommandId="CMD-1" State="http://schemas.'
                                                    'microsoft.com/wbem/wsman/1/windows/shell/CommandState/Done">'
                                                    "<rsp:ExitCode>0</rsp:ExitCode></rsp:CommandState></rsp:ReceiveResponse>")
                        if action == "Signal":
                            return send(200, "<rsp:SignalResponse/>")
                        if action == "Delete":
                            return send(200, "")
                    return send(400, None)
                finally:
                    if not sent:
                        stub.record(action, authorized, served)

        self.srv = _QuietHTTPServer(("127.0.0.1", 0), Handler)
        self.srv.daemon_threads = True
        self.port = self.srv.server_address[1]
        kit_tls.serve_tls(self.srv, cert)
        threading.Thread(target=self.srv.serve_forever, daemon=True).start()

    def record(self, action: str | None, authorized: bool, served: bool) -> None:
        rec = {"t": stamp(), "action": action, "authorized": authorized, "served": served}
        with self.lock:
            self.requests.append(rec)
            with self.log.open("a", encoding="utf-8") as f:
                f.write(json.dumps(rec) + "\n")

    def mark(self) -> int:
        with self.lock:
            return len(self.requests)

    def since(self, mark: int) -> list[dict]:
        with self.lock:
            return list(self.requests[mark:])

    def stop(self) -> None:
        self.srv.shutdown()
        self.srv.server_close()


# --------------------------------------------------------------------------- #
# Isolation: the user's real store, config and install dir for this connector.
# --------------------------------------------------------------------------- #

def real_home() -> Path:
    return Path(pwd.getpwuid(os.getuid()).pw_dir)


def real_paths(slug: str, meta: dict) -> list[Path]:
    home = real_home()
    cli, mcp = meta.get("cli_binary") or f"{slug}-cli", meta.get("mcp_binary") or f"{slug}-mcp"
    names = sorted({cli, mcp, f"{slug}-cli", f"{slug}-pp-cli"})
    roots = [home / ".config", home / ".local" / "share", home / ".local" / "state", home / ".cache",
             home / "Library" / "Application Support", home / "Library" / "Caches"]
    for var in ("XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"):
        v = os.environ.get(var)
        if v and os.path.isabs(v):
            roots.append(Path(v))
    paths = [r / n for r in dict.fromkeys(roots) for n in names]
    return paths + [home / ".local" / "bin" / cli, home / ".local" / "bin" / mcp]


def fingerprint(paths: list[Path]) -> dict[str, dict]:
    out: dict[str, dict] = {}
    for p in paths:
        try:
            st = p.lstat()
        except FileNotFoundError:
            out[str(p)] = {"exists": False}
            continue
        except OSError as e:
            out[str(p)] = {"exists": "unknown", "error": e.__class__.__name__}
            continue
        if p.is_dir() and not p.is_symlink():
            n = size = 0
            newest = st.st_mtime_ns
            for root, dirs, files in os.walk(p):
                for name in dirs + files:
                    try:
                        s = os.lstat(os.path.join(root, name))
                    except OSError:
                        continue
                    n += 1
                    size += s.st_size
                    newest = max(newest, s.st_mtime_ns)
            out[str(p)] = {"exists": True, "kind": "dir", "entries": n, "bytes": size,
                           "newest_mtime_ns": newest}
        else:
            out[str(p)] = {"exists": True, "kind": "file", "bytes": st.st_size, "mtime_ns": st.st_mtime_ns}
    return out


# --------------------------------------------------------------------------- #
# Egress guard.
# --------------------------------------------------------------------------- #

EGRESS_PROBE = """import socket
for fam, addr in ((socket.AF_INET, ("192.0.2.1", 9)), (socket.AF_INET6, ("2001:db8::1", 9))):
    s = socket.socket(fam, socket.SOCK_STREAM)
    s.settimeout(2)
    try:
        s.connect(addr)
        print(fam.name, "connected")
    except OSError as e:
        print(fam.name, "errno", e.errno)
    finally:
        s.close()
"""


def prove_sandbox(sandbox: list[str], env: dict) -> tuple[bool, str]:
    """Under the sandbox, a connect to an off-machine address (the RFC 5737 /
    RFC 3849 documentation ranges, IPv4 and IPv6) must fail with EPERM: the
    sandbox refusing the syscall, before any packet is sent. A timeout or
    'unreachable' would only show the address is dead, not that the guard
    holds. (The profile's `localhost` also admits this host's own LAN
    addresses; nothing off the machine.)"""
    r = subprocess.run(sandbox + [sys.executable, "-I", "-c", EGRESS_PROBE], env=env,
                       capture_output=True, text=True, timeout=30)
    lines = [ln for ln in r.stdout.splitlines() if ln.strip()]
    ok = len(lines) == 2 and all(ln.endswith("errno 1") for ln in lines)
    return ok, ("sandboxed connects to 192.0.2.1 and 2001:db8::1 refused with EPERM before sending"
                if ok else f"probe under the sandbox did not get EPERM: {lines or tail(r.stderr)}")


# --------------------------------------------------------------------------- #
# One run's context.
# --------------------------------------------------------------------------- #

def source_const(cli_dir: Path, name: str) -> str | None:
    rx = re.compile(r"\b" + re.escape(name) + r'\s*=\s*"([A-Z0-9_]+)"')
    for sub in ("cmd", "internal/cli", "internal/cliutil", "internal/mcp"):
        base = cli_dir / sub
        if not base.is_dir():
            continue
        for f in sorted(base.rglob("*.go")):
            if f.name.endswith("_test.go"):
                continue
            try:
                m = rx.search(f.read_text(encoding="utf-8", errors="replace"))
            except OSError:
                continue
            if m:
                return m.group(1)
    return None


def env_names(slug: str, skill: Path, manifest: dict) -> dict[str, str]:
    """The env var names features 3, 5 and 6 need: from the connector's own
    source when it declares them, else its manifest, else derived from the
    variable prefix every other variable uses. A build without the feature
    declares no name, and the derived one is what a reprint will use."""
    keys = list(((manifest.get("server") or {}).get("mcp_config") or {}).get("env") or {})
    prefix = next((k[: -len(sfx)] for sfx in ("_MCP_READ_ONLY", "_MCP_HTTP_TOKEN", "_BASE_URL")
                   for k in keys if k.endswith(sfx)), slug.upper().replace("-", "_"))
    out: dict[str, str] = {"prefix": prefix}
    for key, const, suffix in (("read_only", "MCPReadOnlyEnvVar", "_MCP_READ_ONLY"),
                               ("http_token", "httpTokenEnvVar", "_MCP_HTTP_TOKEN"),
                               ("no_local_data", "noLocalDataEnvVar", "_NO_LOCAL_DATA")):
        found = source_const(skill / "cli", const)
        if found:
            out[key], out[key + "_from"] = found, "source"
        elif any(k.endswith(suffix) for k in keys):
            out[key], out[key + "_from"] = next(k for k in keys if k.endswith(suffix)), "manifest"
        else:
            out[key], out[key + "_from"] = prefix + suffix, "derived"
    return out


def optional_ids_unset(w, manifest: dict):
    """class_harness gives every non-secret, non-URL variable a placeholder.
    A user fills in only the required ones, and an optional identifier (an
    account or sub-tenant id) can switch a connector into another mode, so
    those stay unset here. Credentials and URLs are untouched."""
    env = ((manifest.get("server") or {}).get("mcp_config") or {}).get("env") or {}
    uc = manifest.get("user_config") or {}
    for var, ref in env.items():
        if var in w.sentinels or var not in w.env or not isinstance(ref, str) or not ref.startswith("${user_config."):
            continue
        key = ref[len("${user_config."):].rstrip("}")
        if not (uc.get(key) or {}).get("required"):
            w.env.pop(var, None)
    return w


class Ctx:
    def __init__(self, slug: str, run_dir: Path, scratch: Path):
        self.slug = slug
        self.meta = registry.skills()[slug]
        self.skill = registry.skill_path(slug)
        self.manifest = wiring.read_manifest(slug)
        self.cli_name = self.meta.get("cli_binary") or f"{slug}-cli"
        self.mcp_name = self.meta.get("mcp_binary") or f"{slug}-mcp"
        self.version = self.manifest.get("version") or self.meta.get("version") or "0.0.0"
        self.press = (self.manifest.get("printing_press_version")
                      or (self.manifest.get("press_provenance") or {}).get("printing_press_version")
                      or "unknown")
        self.run_dir, self.scratch = run_dir, scratch
        self.home = scratch / "home"
        self.build_dir = scratch / "build"
        self.bin = self.home / ".local" / "bin"
        for d in (self.build_dir, scratch / "tmp", self.bin, self.home / ".config",
                  self.home / ".local" / "share", self.home / ".local" / "state", self.home / ".cache"):
            d.mkdir(parents=True, exist_ok=True)
        self.pids: list[dict] = []
        self.sandbox: list[str] = []
        self.secrets: list[str] = RUN_SECRETS
        self.vendor: FakeVendor | None = None
        self.vendor_trust: dict[str, str] = {}  # what makes the connector trust a TLS fake vendor
        self.names = env_names(slug, self.skill, self.manifest)
        self.wiring = optional_ids_unset(wiring.classify_env(self.manifest), self.manifest)
        self.notes: list[str] = []
        self.default_tools: list[dict] | None = None
        self.save_state()

    @property
    def cli(self) -> Path:
        return self.bin / self.cli_name

    @property
    def mcp(self) -> Path:
        return self.bin / self.mcp_name

    def fdir(self, name: str) -> Path:
        d = self.run_dir / name
        d.mkdir(parents=True, exist_ok=True)
        return d

    def save_state(self, cleaned: bool = False) -> None:
        write_json(self.run_dir / "state.json",
                   {"slug": self.slug, "scratch": str(self.scratch), "owner_pid": os.getpid(),
                    "pids": self.pids, "cleaned": cleaned})

    def track(self, proc: subprocess.Popen, argv0) -> None:
        self.pids.append({"pid": proc.pid, "argv0": str(argv0)})
        self.save_state()

    def redact(self, text: str) -> str:
        return redact(text, self.secrets)

    def env(self, drop: tuple[str, ...] = (), **extra: str) -> dict[str, str]:
        """A driven process's whole environment. Nothing is inherited from the
        caller, so no real credential, config override or store path leaks in."""
        h = self.home
        e = {"PATH": os.pathsep.join([str(self.bin), "/usr/bin", "/bin", "/usr/sbin", "/sbin"]),
             "HOME": str(h), "TMPDIR": str(self.scratch / "tmp"),
             "XDG_CONFIG_HOME": str(h / ".config"), "XDG_DATA_HOME": str(h / ".local" / "share"),
             "XDG_STATE_HOME": str(h / ".local" / "state"), "XDG_CACHE_HOME": str(h / ".cache")}
        if self.vendor is not None:
            for k in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "all_proxy"):
                e[k] = self.vendor.proxy_url
            e["NO_PROXY"] = e["no_proxy"] = "127.0.0.1,localhost,::1"
            e.update({k: v for k, v in wiring.apply_urls(self.wiring, self.vendor.url).items() if v})
            e.update(self.vendor_trust)
        e.update(extra)
        for k in drop:
            e.pop(k, None)
        return e

    def wrap(self, argv) -> list[str]:
        return self.sandbox + [str(a) for a in argv]

    def run(self, name: str, argv, d: Path, env: dict | None = None, timeout: int = 60,
            sandbox: bool = True) -> dict:
        env = self.env() if env is None else env
        full = self.wrap(argv) if sandbox else [str(a) for a in argv]
        t0 = time.monotonic()
        try:
            p = subprocess.Popen(full, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                 stderr=subprocess.PIPE, env=env, cwd=str(self.scratch),
                                 start_new_session=True)
        except OSError as e:
            raise HarnessError(f"cannot start {argv[0]}: {e}") from e
        timed_out = False
        try:
            out, err = p.communicate(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            kill_group(p.pid, p)
            out, err = p.communicate()
        rec = {"name": name, "argv": [str(a) for a in argv], "sandboxed": bool(sandbox and self.sandbox),
               "env": {k: self.redact(v) for k, v in sorted(env.items())},
               "rc": p.returncode, "timed_out": timed_out, "seconds": round(time.monotonic() - t0, 2),
               "stdout": self.redact(out.decode("utf-8", "replace")),
               "stderr": self.redact(err.decode("utf-8", "replace"))}
        write_json(d / f"{name}.json", rec)
        return rec

    def stdio(self, name: str, d: Path, env: dict | None = None) -> "StdioClient":
        return StdioClient(self, name, d, self.env() if env is None else env)


# --------------------------------------------------------------------------- #
# MCP over stdio: one live session, one request at a time, so each tool call's
# vendor traffic can be attributed to it. Message shapes come from
# check_mcp_gate (HANDSHAKE, result_text, is_protocol_error, placeholder_args).
# --------------------------------------------------------------------------- #

class StdioClient:
    def __init__(self, ctx: Ctx, name: str, d: Path, env: dict):
        self.ctx = ctx
        self.transcript = (d / f"{name}.jsonl").open("w", encoding="utf-8")
        self.stderr_path = d / f"{name}.stderr.txt"
        self._stderr = self.stderr_path.open("wb")
        self.p = subprocess.Popen(ctx.wrap([ctx.mcp]), stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                  stderr=self._stderr, env=env, cwd=str(ctx.scratch), start_new_session=True)
        ctx.track(self.p, ctx.mcp)
        self.q: queue.Queue = queue.Queue()
        self.next_id = 1
        threading.Thread(target=self._reader, daemon=True).start()

    def __enter__(self) -> "StdioClient":
        return self

    def __exit__(self, *exc) -> None:
        self.close()

    def _log(self, direction: str, msg) -> None:
        self.transcript.write(json.dumps({"t": stamp(), "dir": direction, "msg": msg}) + "\n")
        self.transcript.flush()

    def _reader(self) -> None:
        for raw in self.p.stdout:
            line = raw.decode("utf-8", "replace").strip()
            if not line:
                continue
            try:
                msg = json.loads(line)
            except ValueError:
                self._log("recv-raw", line)
                continue
            self._log("recv", msg)
            self.q.put(msg)
        self.q.put(None)

    def send(self, msg: dict) -> None:
        self._log("send", msg)
        try:
            self.p.stdin.write((json.dumps(msg) + "\n").encode())
            self.p.stdin.flush()
        except (BrokenPipeError, OSError) as e:
            raise ServerGone(f"MCP server closed stdin: {e}; stderr: {self.stderr_tail()}") from e

    def stderr_tail(self) -> str:
        try:
            return tail(self.stderr_path.read_text(encoding="utf-8", errors="replace"))
        except OSError:
            return ""

    def request(self, method: str, params: dict, timeout: int = 60) -> dict:
        rid = self.next_id
        self.next_id += 1
        self.send({"jsonrpc": "2.0", "id": rid, "method": method, "params": params})
        deadline = time.monotonic() + timeout
        while True:
            left = deadline - time.monotonic()
            if left <= 0:
                raise HarnessError(f"no answer to {method} within {timeout}s")
            try:
                msg = self.q.get(timeout=left)
            except queue.Empty:
                continue
            if msg is None:
                raise ServerGone(f"MCP server exited before answering {method}; stderr: {self.stderr_tail()}")
            if msg.get("id") == rid:
                return msg

    def initialize(self) -> dict:
        msg = self.request("initialize", gate.HANDSHAKE[0]["params"])
        self.send(gate.HANDSHAKE[1])
        return msg

    def list_tools(self) -> list[dict]:
        tools: list[dict] = []
        params: dict = {}
        for _ in range(50):
            msg = self.request("tools/list", params)
            result = msg.get("result") or {}
            tools += result.get("tools") or []
            cursor = result.get("nextCursor")
            if not cursor:
                break
            params = {"cursor": cursor}
        return tools

    def call(self, name: str, args: dict, timeout: int = 60) -> dict:
        return self.request("tools/call", {"name": name, "arguments": args}, timeout)

    def close(self) -> None:
        try:
            self.p.stdin.close()
        except OSError:
            pass
        try:
            self.p.wait(timeout=5)
        except subprocess.TimeoutExpired:
            kill_group(self.p.pid, self.p)
        self.transcript.close()
        self._stderr.close()


class ServerGone(Exception):
    """The MCP server died mid-session: a fact about the connector (FAIL)."""


def is_tool_error(msg: dict) -> bool:
    return gate.is_protocol_error(msg) or bool((msg.get("result") or {}).get("isError"))


def search_endpoints(cli: StdioClient, tool: str, query: str, limit: int = 50) -> list[dict]:
    data = parse_json_lenient(gate.result_text(cli.call(tool, {"query": query, "limit": limit})))
    items = data.get("results") if isinstance(data, dict) else data
    return [i for i in items or [] if isinstance(i, dict) and (i.get("endpoint_id") or i.get("id"))]


def endpoint_params(item: dict) -> dict:
    """Placeholders a connector's own params_schema accepts: an enum's first
    value, and one item for an array that must not be empty. A connector that
    validates the body before sending would otherwise refuse the probe, and
    the confirm path would never be exercised."""
    schema = item.get("params_schema")
    params = gate.placeholder_args(schema) if isinstance(schema, dict) else {}
    props = (schema.get("properties") or {}) if isinstance(schema, dict) else {}
    for name in params:
        prop = props.get(name) if isinstance(props.get(name), dict) else {}
        if isinstance(prop.get("enum"), list) and prop["enum"]:
            params[name] = prop["enum"][0]
        elif prop.get("type") == "array" and (prop.get("minItems") or 0) >= 1:
            item_schema = prop.get("items") if isinstance(prop.get("items"), dict) else {}
            params[name] = [gate.placeholder_args({"required": ["x"], "properties": {"x": item_schema}})["x"]]
    for name in re.findall(r"\{([^}/]+)\}", item.get("path") or ""):
        params.setdefault(name, PROBE_VALUE)
    return params


def call_observed(ctx: Ctx, cli: StdioClient, tool: str, args: dict) -> dict:
    mark = ctx.vendor.mark()
    msg = cli.call(tool, args)
    reqs = ctx.vendor.since(mark)
    return {"tool": tool, "arguments": args, "is_error": is_tool_error(msg),
            "answer_head": ctx.redact(gate.result_text(msg))[:1500], "vendor_requests": reqs}


def verdict(v: str, reason: str, **extra) -> dict:
    return {"verdict": v, "reason": reason, **extra}


def combine(checks: dict[str, tuple[str, str]]) -> dict:
    vs = [v for v, _ in checks.values()]
    if FAIL in vs:
        v = FAIL
    elif ERROR in vs:
        v = ERROR
    elif vs and all(x == SKIP for x in vs):
        v = SKIP
    else:
        v = PASS
    order = sorted(checks.items(), key=lambda kv: (kv[1][0] == PASS, kv[1][0] == SKIP))
    reason = "; ".join(f"{k} {cv}: {r}" for k, (cv, r) in order)
    return verdict(v, reason, checks={k: {"verdict": cv, "reason": r} for k, (cv, r) in checks.items()})


# --------------------------------------------------------------------------- #
# Build (release.yml's flags, for this host) - the artifact install.sh installs.
# --------------------------------------------------------------------------- #

def build(ctx: Ctx) -> None:
    d = ctx.fdir("build")
    cli_cmd, mcp_cmd = registry.cmd_dirs(ctx.slug)
    module = registry.module_path(ctx.slug)
    for name, cmd, ldflags in ((ctx.cli_name, cli_cmd, f"-s -w -X {module}/internal/cli.version={ctx.version}"),
                               (ctx.mcp_name, mcp_cmd, f"-s -w -X main.version={ctx.version}")):
        argv = ["go", "build", "-trimpath", "-ldflags", ldflags, "-o", str(ctx.build_dir / name), cmd]
        r = subprocess.run(argv, cwd=str(ctx.skill / "cli"), capture_output=True, text=True, timeout=1200)
        write_json(d / f"go-build-{name}.json", {"argv": argv, "cwd": str(ctx.skill / "cli"),
                                                  "rc": r.returncode, "stdout": r.stdout, "stderr": r.stderr})
        if r.returncode != 0:
            raise HarnessError(f"go build {cmd} failed: {tail(r.stderr)}")


def place_built(ctx: Ctx) -> None:
    for name in (ctx.cli_name, ctx.mcp_name):
        shutil.copy2(ctx.build_dir / name, ctx.bin / name)


# --------------------------------------------------------------------------- #
# Feature 1: install path.
# --------------------------------------------------------------------------- #

def f1_install(ctx: Ctx) -> dict:
    d = ctx.fdir("f1-install")
    osname, arch = host_platform()
    assets = ctx.scratch / "release-assets"
    assets.mkdir(exist_ok=True)
    built = {}
    for name in (ctx.cli_name, ctx.mcp_name):
        asset = f"{name}-{osname}-{arch}"
        shutil.copy2(ctx.build_dir / name, assets / asset)
        built[name] = sha256(assets / asset)
        (assets / f"{asset}.sha256").write_text(f"{built[name]}  {asset}\n", encoding="utf-8")
    tag = f"{ctx.slug}-v{ctx.version}"
    port_file = ctx.scratch / "fake-github.port"
    gh_log = d / "fake-github.log"
    gh_log.touch()
    gh_argv = [sys.executable, "-I", str(FAKE_GITHUB), "--port-file", str(port_file), "--assets", str(assets),
               "--slug", ctx.slug, "--tag", tag, "--log", str(gh_log)]
    gh = subprocess.Popen(gh_argv, stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                          stderr=(d / "fake-github.stderr.txt").open("wb"), start_new_session=True)
    ctx.track(gh, FAKE_GITHUB)
    try:
        for _ in range(100):
            if port_file.is_file() or gh.poll() is not None:
                break
            time.sleep(0.1)
        if not port_file.is_file():
            raise HarnessError("the release fixture (fake_github.py) did not start")
        api = f"http://127.0.0.1:{port_file.read_text().strip()}"
        bash = shutil.which("bash", path="/bin:/usr/bin") or "/bin/bash"
        inst = ctx.run("install-sh", [bash, ctx.skill / "install.sh"], d, env=ctx.env(MSP_SKILLS_API_BASE=api),
                       timeout=180)
    finally:
        kill_group(gh.pid, gh)
    fixture_hits = [ln for ln in gh_log.read_text(encoding="utf-8").splitlines() if ln.strip()]
    if not fixture_hits:
        place_built(ctx)
        raise HarnessError(f"install.sh never reached the release fixture: {tail(inst['stderr'])}")
    installed = {n: (ctx.bin / n).is_file() and sha256(ctx.bin / n) == built[n] for n in built}
    if inst["rc"] != 0 or not all(installed.values()):
        place_built(ctx)
        ctx.notes.append("install.sh did not install; features 2-6 drove the built binaries directly")
        return verdict(FAIL, f"install.sh rc={inst['rc']}, installed+hash-matched={installed}: "
                             f"{tail(inst['stderr'] or inst['stdout'])}")
    sealed = "(sealed)" in inst["stdout"]
    ver = ctx.run("cli-version", [ctx.cli, "--version"], d)
    hlp = ctx.run("mcp-help", [ctx.mcp, "--help"], d)
    doc = ctx.run("cli-doctor", [ctx.cli, "doctor", "--json"], d)
    report = parse_json_lenient(doc["stdout"])
    checks = {
        "install.sh": (PASS if sealed else FAIL,
                       f"installed {ctx.cli_name} + {ctx.mcp_name} from sealed {tag} (sha256 verified)"
                       if sealed else "installed without reporting a sealed release"),
        "--version": (PASS if ver["rc"] == 0 else FAIL,
                      f"rc={ver['rc']} {tail(ver['stdout'] or ver['stderr'], 1)}"
                      + ("" if ctx.version in ver["stdout"] else f" (manifest version {ctx.version} not shown)")),
        "mcp --help": (PASS if hlp["rc"] == 0 else FAIL, f"rc={hlp['rc']}"),
        "doctor": (PASS if doc["rc"] == 0 and isinstance(report, dict) else FAIL,
                   f"rc={doc['rc']}, " + (f"{len(report)} report keys" if isinstance(report, dict)
                                          else "no JSON report")),
    }
    return combine(checks)


# --------------------------------------------------------------------------- #
# Feature 2: MCP stdio surface.
# --------------------------------------------------------------------------- #

def f2_mcp_stdio(ctx: Ctx) -> dict:
    d = ctx.fdir("f2-mcp-stdio")
    decl = WINRM_CONNECTORS.get(ctx.slug)
    if decl is None and not ctx.wiring.url_vars and not ctx.wiring.host_vars:
        return verdict(ERROR, "manifest.json names no base-URL variable, so the fake vendor cannot stand in")
    stub, env = start_winrm(ctx, decl, d) if decl else (None, None)
    try:
        with ctx.stdio("session", d, env) as cli:
            init = cli.initialize()
            info = (init.get("result") or {}).get("serverInfo") or {}
            if gate.is_protocol_error(init) or not info:
                return verdict(FAIL, f"initialize failed: {tail(gate.result_text(init))}")
            tools = cli.list_tools()
            ctx.default_tools = tools
            write_json(d / "tools.json", tools)
            if not tools:
                return verdict(FAIL, "tools/list returned no tools")
            attempts = drive_winrm(cli, tools, stub, decl) if stub else drive_read(ctx, cli, tools)
    except ServerGone as e:
        return verdict(FAIL, str(e))
    finally:
        if stub is not None:
            stub.stop()
    write_json(d / "read-attempts.json", attempts)
    head = f"initialize ok ({info.get('name')} {info.get('version')}), {len(tools)} tools"
    if stub is not None:
        return winrm_verdict(head, attempts)
    if not attempts:
        return verdict(ERROR, f"{head}; found no read tool to call")
    for a in attempts:
        if read_ok(a):
            ep, req, n = a["arguments"].get("endpoint_id"), a["vendor_requests"][-1], len(a["vendor_requests"])
            return verdict(PASS, f"{head}; {a['tool']}{f'({ep})' if ep else ''} reached the fake vendor "
                                 f"({req['method']} {redact(req['path'])[:80]}, auth={req['auth_scheme']}"
                                 f"{f', after {n - 1} earlier request(s)' if n > 1 else ''}) and answered")
    last = attempts[-1]
    reached = [a for a in attempts if a["vendor_requests"]]
    if reached:
        # The request left correctly; the fake's generic reply is what the
        # client rejected, so this run cannot judge the answer path.
        req = reached[-1]["vendor_requests"][0]
        return verdict(ERROR, f"{head}; read call reached the fake vendor ({req['method']} {redact(req['path'])[:60]}) "
                              f"but rejected its generic reply: {tail(reached[-1]['answer_head'], 1)}")
    return verdict(FAIL, f"{head}; {len(attempts)} read call(s), none reached the fake vendor: "
                         f"{tail(last['answer_head'], 1)}")


def start_winrm(ctx: Ctx, decl: dict, d: Path) -> tuple[FakeWinRM, dict]:
    """The WinRM stub and the stdio session's environment: the declared config
    variable names a scratch config (in the scratch HOME, mode 600, as the
    connector requires) with one [[servers]] entry for the stub, pinned to the
    stub's certificate by cert_sha256, the connector's own pinning option. The
    base-URL variables are left unset, as a user leaves them; the connector
    refuses any value. The account is the manifest's stand-in pair."""
    tls_dir = ctx.scratch / "winrm-tls"
    tls_dir.mkdir(parents=True, exist_ok=True)
    try:
        cert = kit_tls.make_cert(str(tls_dir))
    except RuntimeError as e:
        raise HarnessError(str(e)) from e
    env = ctx.env(drop=tuple(ctx.wiring.url_vars + ctx.wiring.host_vars))
    user, password = env.get(decl["username_var"]), env.get(decl["password_var"])
    if not user or not password:
        raise HarnessError(f"the manifest gives no stand-in value for {decl['username_var']} or {decl['password_var']}")
    stub = FakeWinRM(cert, user, password, d / "winrm-requests.jsonl")
    pin = hashlib.sha256(ssl.PEM_cert_to_DER_cert(Path(cert[0]).read_text(encoding="utf-8"))).hexdigest()
    cfg = ctx.home / ".config" / "verify-winrm-servers.toml"
    cfg.write_text(f'[[servers]]\nname = "verify-stub"\nhost = "127.0.0.1"\nport = {stub.port}\n'
                   f'auth = "basic"\ncert_sha256 = "{pin}"\n', encoding="utf-8")
    cfg.chmod(0o600)
    env[decl["config_env"]] = str(cfg)
    return stub, env


def drive_winrm(cli: StdioClient, tools: list[dict], stub: FakeWinRM, decl: dict) -> list[dict]:
    """The declared read tools, in order, until one answers with the stub's
    data. Each attempt records the WinRM requests it caused."""
    by = {t.get("name", ""): t for t in tools}
    missing = [n for n in decl["read_tools"] if n not in by]
    if missing:
        raise HarnessError(f"declared WinRM read tool(s) {missing} are not in tools/list; update WINRM_CONNECTORS")
    attempts: list[dict] = []
    for name in decl["read_tools"]:
        args = gate.placeholder_args(by[name].get("inputSchema") or {})
        mark = stub.mark()
        msg = cli.call(name, args)
        text = gate.result_text(msg)
        attempts.append({"tool": name, "arguments": args, "is_error": is_tool_error(msg),
                         "carries_marker": stub.marker in text, "answer_head": redact(text)[:1500],
                         "winrm_requests": stub.since(mark)})
        if winrm_read_ok(attempts[-1]):
            break
    return attempts


def winrm_read_ok(a: dict) -> bool:
    return not a["is_error"] and a["carries_marker"] and any(r["served"] for r in a["winrm_requests"])


def winrm_verdict(head: str, attempts: list[dict]) -> dict:
    """PASS only when a read tool answered without error, its answer carries
    the stub's marker, and the stub served that data to an authorized, pinned
    script. Anything less is the connector's (the stub answers real-shaped
    data, unlike the HTTP fake's generic reply), so FAIL."""
    for a in attempts:
        if winrm_read_ok(a):
            acts = ", ".join(dict.fromkeys(str(r["action"]) for r in a["winrm_requests"]))
            return verdict(PASS, f"{head}; {a['tool']} reached the WinRM stub ({acts}; Basic auth with the "
                                 f"configured account, pinned script) and answered with the stub's volume row")
    a = attempts[-1]
    reqs = a["winrm_requests"]
    if not reqs:
        why = "none reached the WinRM stub"
    elif not any(r["authorized"] for r in reqs):
        why = "the WinRM stub refused its credential (HTTP 401)"
    elif not any(r["served"] for r in reqs):
        why = "it reached the WinRM stub but no pinned script was served the stub's data"
    elif a["is_error"]:
        why = "it was served the stub's data but answered with an error"
    else:
        why = "it answered without the stub's data"
    return verdict(FAIL, f"{head}; {len(attempts)} WinRM read call(s): {why}: {tail(a['answer_head'], 1)}")


def is_read_tool(t: dict) -> bool:
    return (t.get("annotations") or {}).get("readOnlyHint") is True


def read_ok(a: dict) -> bool:
    return bool(a["vendor_requests"]) and not a["is_error"]


def drive_read(ctx: Ctx, cli: StdioClient, tools: list[dict]) -> list[dict]:
    """One read call that reaches the fake vendor. Order: the 4.34 endpoint
    reader `*_read` with whatever the search returns; else (older builds)
    `*_execute` with read endpoints only, a GET or an id ending .list/.get
    (JSON-RPC APIs read with POST); else the typed read-only tools. `*_get`
    is skipped: it describes an endpoint and calls nothing."""
    by = {t.get("name", ""): t for t in tools}
    search = next((n for n in by if n.endswith("_search")), None)
    reader = next((n for n in by if n.endswith("_read") and is_read_tool(by[n])), None)
    executor = next((n for n in by if n.endswith("_execute")), None)
    attempts: list[dict] = []
    if search and (reader or executor):
        items = search_endpoints(cli, search, "list")
        if not reader:
            items = [i for i in items if str(i.get("method", "")).upper() == "GET"
                     or str(i.get("endpoint_id") or i.get("id")).rsplit(".", 1)[-1] in ("list", "get")]
        items.sort(key=lambda i: (str(i.get("method", "")).upper() != "GET", "{" in (i.get("path") or "")))
        for item in items[:5]:
            args = {"endpoint_id": item.get("endpoint_id") or item.get("id"), "params": endpoint_params(item)}
            attempts.append(call_observed(ctx, cli, reader or executor, args))
            if read_ok(attempts[-1]):
                return attempts
    skip = {"sql", "context", search, reader, executor}
    for name, t in by.items():
        hints = t.get("annotations") or {}
        if (name in skip or (search and name.endswith("_get")) or not hints.get("readOnlyHint")
                or hints.get("destructiveHint")):
            continue
        attempts.append(call_observed(ctx, cli, name, gate.placeholder_args(t.get("inputSchema") or {})))
        if read_ok(attempts[-1]) or len(attempts) >= 10:
            break
    return attempts


# --------------------------------------------------------------------------- #
# Feature 3: MCP HTTP transport.
# --------------------------------------------------------------------------- #

def listening_addrs(pid: int) -> list[str]:
    if shutil.which("lsof"):
        r = subprocess.run(["lsof", "-nP", "-a", "-p", str(pid), "-iTCP", "-sTCP:LISTEN", "-Fn"],
                           capture_output=True, text=True, timeout=20)
        return [ln[1:] for ln in r.stdout.splitlines() if ln.startswith("n")]
    if shutil.which("ss"):
        r = subprocess.run(["ss", "-Hltnp"], capture_output=True, text=True, timeout=20)
        return [ln.split()[3] for ln in r.stdout.splitlines() if f"pid={pid}," in ln and len(ln.split()) > 3]
    raise HarnessError("neither lsof nor ss is available to read the listening address")


def addr_is_loopback(addr: str) -> bool:
    host = addr.rsplit(":", 1)[0].strip("[]")
    if host in ("*", "", "0.0.0.0", "::"):
        return False
    try:
        return ipaddress.ip_address(host.split("%")[0]).is_loopback
    except ValueError:
        return host == "localhost"


def http_post(url: str, payload: dict, headers: dict | None = None, timeout: int = 15):
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    req = urllib.request.Request(url, data=json.dumps(payload).encode(), method="POST",
                                 headers={"Content-Type": "application/json",
                                          "Accept": "application/json, text/event-stream", **(headers or {})})
    try:
        with opener.open(req, timeout=timeout) as r:
            return r.status, r.headers, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        return e.code, e.headers, e.read().decode("utf-8", "replace")


def rpc_body(headers, body: str) -> dict | None:
    ctype = (headers.get("Content-Type") if headers else "") or ""
    if "event-stream" in ctype:
        for line in body.splitlines():
            if line.startswith("data:"):
                try:
                    return json.loads(line[5:].strip())
                except ValueError:
                    continue
        return None
    parsed = parse_json_lenient(body)
    return parsed if isinstance(parsed, dict) else None


def f3_mcp_http(ctx: Ctx) -> dict:
    d = ctx.fdir("f3-mcp-http")
    checks = {"no-token default launch": http_default_launch(ctx, d), **http_bearer_auth(ctx, d)}
    return combine(checks)


def http_default_launch(ctx: Ctx, d: Path) -> tuple[str, str]:
    """The documented launch (`<slug>-mcp --transport http`, default address)
    with no token variable: it must refuse to start or listen on loopback only."""
    hlp = ctx.run("mcp-help", [ctx.mcp, "--help"], d)
    m = re.search(r'-addr\s+string[^\n]*\n[^\n]*\(default "([^"]*)"\)', hlp["stdout"] + hlp["stderr"])
    default_addr = m.group(1) if m else None
    port = int(default_addr.rsplit(":", 1)[1]) if default_addr and default_addr.rsplit(":", 1)[-1].isdigit() else None
    token_var = ctx.names["http_token"]
    lock_path = EVIDENCE_ROOT / ".mcp-http-default-port.lock"
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    with lock_path.open("w") as lock:
        for _ in range(600):
            try:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                break
            except BlockingIOError:
                time.sleep(0.2)
        else:
            return ERROR, "another run held the default-port lock for 120s"
        if port is not None and port_busy(port):
            return ERROR, f"default port {port} is already in use, so the default bind cannot be observed"
        err_path = d / "default-launch.stderr.txt"
        with err_path.open("wb") as err:
            p = subprocess.Popen(ctx.wrap([ctx.mcp, "--transport", "http"]), stdin=subprocess.DEVNULL,
                                 stdout=subprocess.DEVNULL, stderr=err, env=ctx.env(drop=(token_var,)),
                                 cwd=str(ctx.scratch), start_new_session=True)
            ctx.track(p, ctx.mcp)
            addrs: list[str] = []
            try:
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline and p.poll() is None:
                    addrs = listening_addrs(p.pid)
                    if addrs:
                        break
                    time.sleep(0.2)
            finally:
                rc = p.poll()
                if rc is None:
                    kill_group(p.pid, p)
        stderr = err_path.read_text(encoding="utf-8", errors="replace")
    write_json(d / "default-launch.json", {"argv": [str(ctx.mcp), "--transport", "http"],
                                           "default_addr_from_help": default_addr, "token_var_unset": token_var,
                                           "listening": addrs, "exit_code_before_kill": rc, "stderr": stderr})
    if addrs:
        wide = [a for a in addrs if not addr_is_loopback(a)]
        if wide:
            return FAIL, f"with no {token_var} it started and listens on {', '.join(wide)} (every interface)"
        return PASS, f"with no {token_var} it listens on loopback only ({', '.join(addrs)})"
    if rc is not None:
        if "address already in use" in stderr.lower():
            return ERROR, f"default port taken mid-run: {tail(stderr, 1)}"
        if token_var in stderr or "token" in stderr.lower():
            return PASS, f"with no {token_var} it refuses to start (rc={rc}): {tail(stderr, 1)}"
        return FAIL, f"exited rc={rc} without naming a token: {tail(stderr, 1)}"
    return ERROR, "still running after 10s with no listening socket observed"


def http_bearer_auth(ctx: Ctx, d: Path) -> dict[str, tuple[str, str]]:
    """With a token set and a loopback address, start the server and probe it
    (probe_bearer). A server that does not start is ERROR."""
    token_var = ctx.names["http_token"]
    token = secrets.token_hex(32)
    ctx.secrets.append(token)
    port = free_port()
    url = f"http://127.0.0.1:{port}/mcp"
    err_path = d / "auth-server.stderr.txt"
    record: dict = {"argv": [str(ctx.mcp), "--transport", "http", "--addr", f"127.0.0.1:{port}"],
                    "token_var": token_var, "url": url}
    with err_path.open("wb") as err:
        p = subprocess.Popen(ctx.wrap(record["argv"]), stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL,
                             stderr=err, env=ctx.env(**{token_var: token}), cwd=str(ctx.scratch),
                             start_new_session=True)
        ctx.track(p, ctx.mcp)
        try:
            up = False
            for _ in range(50):
                if p.poll() is not None:
                    break
                try:
                    socket.create_connection(("127.0.0.1", port), timeout=1).close()
                    up = True
                    break
                except OSError:
                    time.sleep(0.2)
            if not up:
                record["stderr"] = ctx.redact(err_path.read_text(encoding="utf-8", errors="replace"))
                write_json(d / "bearer-auth.json", record)
                return {"bearer auth": (ERROR, f"did not start with {token_var} set: {tail(record['stderr'], 1)}")}
            record["origin_allowlist_expected"] = list(origin_allowlist_expected(ctx))
            results = probe_bearer(url, token, token_var, record, ctx.secrets)
        finally:
            kill_group(p.pid, p)
    record["stderr"] = ctx.redact(err_path.read_text(encoding="utf-8", errors="replace"))
    write_json(d / "bearer-auth.json", {k: (ctx.redact(v) if isinstance(v, str) else v) for k, v in record.items()})
    return results


ORIGIN_ALLOWLIST_SINCE = (4, 34, 2)
FOREIGN_ORIGIN = "https://verify-foreign-origin.invalid"


def origin_allowlist_expected(ctx: Ctx) -> tuple[bool, str]:
    """(does this server enforce an Origin allowlist?, why). Press 4.34.2 added
    it, read from <PREFIX>_MCP_HTTP_ALLOWED_ORIGINS: the recorded press version
    decides, and a binary that carries that variable name counts too."""
    m = re.match(r"(\d+)\.(\d+)\.(\d+)", ctx.press or "")
    if m and tuple(int(x) for x in m.groups()) >= ORIGIN_ALLOWLIST_SINCE:
        return True, f"press {ctx.press}"
    try:
        if b"_MCP_HTTP_ALLOWED_ORIGINS" in ctx.mcp.read_bytes():
            return True, f"{ctx.mcp_name} reads <PREFIX>_MCP_HTTP_ALLOWED_ORIGINS"
    except OSError:
        pass
    return False, f"press {ctx.press}, and {ctx.mcp_name} does not read <PREFIX>_MCP_HTTP_ALLOWED_ORIGINS"


def probe_bearer(url: str, token: str, token_var: str, record: dict, secret_list: list[str]) -> dict[str, tuple[str, str]]:
    """Bearer auth: a request without the token and a request with a WRONG token
    are refused (401/403), and a request with the token (and no Origin header)
    gets a working MCP session. Origin allowlist: with the right token, a
    foreign Origin is refused 403. A server without an allowlist (before press
    4.34.2, record["origin_allowlist_expected"]) that answers it is SKIP, never
    FAIL. Every recorded body passes through the redaction of secret_list."""
    expected, why = (record.get("origin_allowlist_expected") or [False, "not recorded"])[:2]
    wrong = secrets.token_hex(32)
    secret_list.append(wrong)

    def red(text: str) -> str:
        return redact(text, secret_list)
    init = {"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": gate.HANDSHAKE[0]["params"]}
    st0, _, body0 = http_post(url, init)
    record["no_token"] = {"status": st0, "body_head": red(body0)[:300]}
    stw, _, bodyw = http_post(url, init, {"Authorization": f"Bearer {wrong}"})
    record["wrong_token"] = {"status": stw, "body_head": red(bodyw)[:300]}
    sto, _, bodyo = http_post(url, init, {"Authorization": f"Bearer {token}", "Origin": FOREIGN_ORIGIN})
    record["foreign_origin_with_token"] = {"origin": FOREIGN_ORIGIN, "status": sto, "body_head": red(bodyo)[:300]}
    st1, h1, body1 = http_post(url, init, {"Authorization": f"Bearer {token}"})
    msg1 = rpc_body(h1, body1)
    sid = h1.get("Mcp-Session-Id") if h1 else None
    record["with_token"] = {"status": st1, "session_id": bool(sid), "body_head": red(body1)[:300]}
    tools = []
    if st1 == 200 and msg1 and "result" in msg1:
        auth = {"Authorization": f"Bearer {token}", **({"Mcp-Session-Id": sid} if sid else {})}
        http_post(url, {"jsonrpc": "2.0", "method": "notifications/initialized"}, auth)
        st2, h2, body2 = http_post(url, {"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}}, auth)
        msg2 = rpc_body(h2, body2) or {}
        tools = (msg2.get("result") or {}).get("tools") or []
        record["tools_list"] = {"status": st2, "tools": len(tools)}
    out: dict[str, tuple[str, str]] = {}
    if st0 not in (401, 403):
        out["bearer auth"] = (FAIL, f"answered an MCP initialize with no Authorization header (HTTP {st0}); no bearer auth")
    elif stw not in (401, 403):
        out["bearer auth"] = (FAIL, f"answered an MCP initialize with a WRONG bearer token (HTTP {stw}); the token is not compared")
    elif st1 != 200 or not tools:
        out["bearer auth"] = (FAIL, f"refused no token (HTTP {st0}) and a wrong token (HTTP {stw}) but the configured "
                                    f"token with no Origin header got HTTP {st1}, {len(tools)} tools")
    else:
        out["bearer auth"] = (PASS, f"no token: HTTP {st0}; wrong token: HTTP {stw}; Bearer ${token_var}: HTTP {st1} "
                                    f"and {len(tools)} tools")
    if sto == 403:
        out["origin allowlist"] = (PASS, f"foreign Origin with the token: HTTP 403; no Origin with the token: HTTP {st1} ({why})")
    elif expected:
        out["origin allowlist"] = (FAIL, f"{why}, yet a foreign Origin ({FOREIGN_ORIGIN}) with the token got HTTP {sto}, not 403")
    else:
        out["origin allowlist"] = (SKIP, f"no Origin allowlist ({why}): a foreign Origin with the token got HTTP {sto}")
    return out


class _StubMCPHandler(BaseHTTPRequestHandler):
    """A fake `<slug>-mcp --transport http` for the self-test. `mode` picks how
    it treats the bearer token and the Origin header."""
    token = ""
    mode = "enforces-origins"

    def log_message(self, *a):  # silence
        pass

    def do_POST(self):
        n = int(self.headers.get("Content-Length") or 0)
        msg = json.loads(self.rfile.read(n) or b"{}")
        origin = self.headers.get("Origin")
        authz = self.headers.get("Authorization") or ""
        mode = type(self).mode
        if mode.startswith("echo-at-") and origin is not None:
            # A refusal body that carries the token at a chosen offset, so the
            # 300-character body head is cut inside it, at it, or just after it.
            body = ("x" * int(mode.rsplit("-", 1)[1]) + type(self).token + " end").encode()
            self.send_response(403)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        if mode == "no-origin-refused" and origin is None:
            return self._send(403, {"error": "forbidden origin"})
        if mode in ("enforces-origins", "accepts-any-bearer", "no-origin-refused") and origin is not None \
                and origin != "http://127.0.0.1":
            return self._send(403, {"error": "forbidden origin"})
        ok = authz == f"Bearer {type(self).token}" or (mode == "accepts-any-bearer" and authz.startswith("Bearer "))
        if mode != "no-auth" and not ok:
            return self._send(401, {"error": "unauthorized"})
        if "id" not in msg:
            return self._send(202, None)
        result = {"tools": [{"name": "stub_read"}]} if msg.get("method") == "tools/list" else {"serverInfo": {"name": "stub"}}
        self._send(200, {"jsonrpc": "2.0", "id": msg["id"], "result": result}, {"Mcp-Session-Id": "stub-session"})

    def _send(self, code: int, obj, headers: dict | None = None):
        body = b"" if obj is None else json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        for k, v in (headers or {}).items():
            self.send_header(k, v)
        self.end_headers()
        self.wfile.write(body)


def self_test() -> int:
    """Drive probe_bearer against stand-in servers, one per behaviour, and
    check each verdict both ways."""
    cases = [  # (mode, allowlist expected, {check: verdict})
        ("enforces-origins", True, {"bearer auth": PASS, "origin allowlist": PASS}),
        ("ignores-origins", False, {"bearer auth": PASS, "origin allowlist": SKIP}),
        ("enforces-origins", False, {"bearer auth": PASS, "origin allowlist": PASS}),
        ("accepts-any-bearer", True, {"bearer auth": FAIL}),
        ("ignores-origins", True, {"bearer auth": PASS, "origin allowlist": FAIL}),
        ("no-origin-refused", True, {"bearer auth": FAIL}),
        ("no-auth", False, {"bearer auth": FAIL}),
    ]
    bad = 0
    for mode, expected, want in cases:
        token = secrets.token_hex(32)
        _StubMCPHandler.token, _StubMCPHandler.mode = token, mode
        srv = ThreadingHTTPServer(("127.0.0.1", 0), _StubMCPHandler)
        threading.Thread(target=srv.serve_forever, daemon=True).start()
        try:
            record = {"origin_allowlist_expected": [expected, "self-test"]}
            got = probe_bearer(f"http://127.0.0.1:{srv.server_address[1]}/mcp", token, "STUB_MCP_HTTP_TOKEN",
                               record, [token])
        finally:
            srv.shutdown()
            srv.server_close()
        ok = all(got.get(k, (None, ""))[0] == v for k, v in want.items())
        leaked = token in json.dumps(record)
        if leaked:
            ok = False
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] {mode} (allowlist expected: {expected}): "
              + "; ".join(f"{k} {v[0]}" for k, v in got.items()) + (" (token leaked into the record)" if leaked else ""))
    bad += redaction_self_test()
    req = lambda m, p: {"method": m, "path": p, "header_names": [], "body_head": ""}
    for reqs, method, tmpl, want in (
        ([req("POST", "/v2/roles")], "POST", "https://{region}.iam.eset.systems/v2/roles", 1),
        ([req("POST", "/v2/roles/x")], "POST", "https://{region}.iam.eset.systems/v2/roles", 0),
        ([req("POST", "/v2/users")], "POST", "https://{region}.iam.eset.systems/v2/roles", 0),
        ([req("POST", "/v1/items/42")], "POST", "/v1/items/{id}", 1),
        ([req("GET", "/v1/items/42")], "POST", "/v1/items/{id}", 0),
    ):
        got = len(endpoint_requests(reqs, method, tmpl))
        ok = got == want
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] endpoint_requests {method} {tmpl} vs {reqs[0]['path']}: {got} (want {want})")
    bad += dsm_self_test()
    bad += vcenter_self_test()
    bad += kit_tls_self_test()
    bad += endpoint_params_self_test()
    bad += winrm_self_test()
    bad += ncentral_self_test()
    bad += write_choice_self_test()
    print("PASS: verify.py self-test" if not bad else f"FAIL: verify.py self-test ({bad} case(s))")
    return 0 if not bad else 1


def ncentral_self_test() -> int:
    """The fake vendor answers N-central's JWT exchange and refresh (POST
    <base>/auth/authenticate, POST <base>/auth/refresh) with N-central's token
    envelope, and every other request exactly as before."""
    tokens = ncentral_tokens()
    cases = [  # (method, request target, expected status, expected reply)
        ("POST", "/auth/authenticate", 200, tokens),
        ("POST", "/api/auth/authenticate", 200, tokens),
        ("POST", "/api/auth/refresh", 200, tokens),
        ("POST", "/api/auth/authenticate?x=1", 200, tokens),
        ("GET", "/api/auth/authenticate", 200, []),
        ("POST", "/api/auth/authenticated", 200, {}),
        ("POST", "/api/auth/validate", 200, {}),
        ("POST", "/api/devices", 200, {}),
        ("GET", "/api/devices", 200, []),
    ]
    bad = 0
    with tempfile.TemporaryDirectory(prefix="verify-ncentral-") as td:
        d = Path(td)
        vendor = FakeVendor(d / "vendor-requests.jsonl", d / "egress-attempts.jsonl")
        try:
            for method, target, want_status, want in cases:
                conn = http.client.HTTPConnection("127.0.0.1", vendor.srv.server_address[1], timeout=10)
                try:
                    conn.request(method, target, body=b"" if method == "POST" else None)
                    resp = conn.getresponse()
                    status, got = resp.status, json.loads(resp.read() or b"null")
                except (OSError, http.client.HTTPException, ValueError) as e:
                    status, got = 0, f"no reply: {type(e).__name__}"
                finally:
                    conn.close()
                ok = status == want_status and got == want
                bad += not ok
                kind = "N-central token envelope" if isinstance(got, dict) and "tokens" in got else repr(got)
                print(f"  [{'PASS' if ok else 'FAIL'}] fake vendor {method} {target}: {status} {kind}")
        finally:
            vendor.stop()
    access = (tokens.get("tokens") or {}).get("access") or {}
    ok = bool(access.get("token")) and access.get("expirySeconds", 0) > 0
    bad += not ok
    print(f"  [{'PASS' if ok else 'FAIL'}] the N-central envelope carries an access token and its expiry")
    return bad


class _FakeSearchClient:
    """Stands in for a code-orchestration MCP server's <slug>_search: keyword
    match over a fixed catalog, as the generated handler does."""

    def __init__(self, catalog: list[dict]):
        self.catalog, self.queries = catalog, []

    def call(self, tool: str, args: dict, timeout: int = 60) -> dict:
        q = str(args.get("query") or "").lower()
        self.queries.append(q)
        words = [w for w in re.split(r"[\s_\-/{}.]+", q) if len(w) >= 3]
        hits = [e for e in self.catalog
                if any(w in re.split(r"[\s_\-/{}.]+", (e["endpoint_id"] + " " + e["path"] + " "
                                                         + e.get("summary", "")).lower()) for w in words)]
        return {"jsonrpc": "2.0", "id": 1,
                "result": {"content": [{"type": "text", "text": json.dumps({"results": hits})}]}}


def write_choice_self_test() -> int:
    """Feature 4 drives the search tool that belongs to <slug>_execute (zammad
    also lists kb_search), and finds a write endpoint whose name matches none of
    the generic write verbs (auvik's only write is POST alert.dismiss-single)
    through the connector's own tools manifest."""
    bad = 0
    for names, execute, want in (
        (["kb_search", "zammad_search", "zammad_execute"], "zammad_execute", "zammad_search"),
        (["zammad_execute", "zammad_search", "kb_search"], "zammad_execute", "zammad_search"),
        (["kb_search", "zammad_execute"], "zammad_execute", None),
        (["auvik_search", "auvik_execute"], "auvik_execute", "auvik_search"),
    ):
        got = pick_search(names, execute)
        ok = got == want
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] search tool for {execute} among {names}: {got} (want {want})")
    catalog = [
        {"endpoint_id": "alert.list", "method": "GET", "path": "/v1/alert/history/info", "summary": "List alerts"},
        {"endpoint_id": "alert.dismiss-single", "method": "POST", "path": "/v1/alert/dismiss/{id}",
         "summary": "Use the Dismiss Alert API to dismiss a specific alert"},
    ]
    with tempfile.TemporaryDirectory(prefix="verify-write-choice-") as td:
        skill = Path(td) / "auvik"
        (skill / "press").mkdir(parents=True)
        (skill / "press" / "tools-manifest.json").write_text(json.dumps({"mcp": {"orchestration": "code"}, "tools": [
            {"name": "alert_list", "method": "GET", "path": "/v1/alert/history/info"},
            {"name": "alert_dismiss-single", "method": "POST", "path": "/v1/alert/dismiss/{id}"}]}),
            encoding="utf-8")
        queries = write_queries(skill)
        cli = _FakeSearchClient(catalog)
        item = find_write_endpoint(cli, "auvik_search", queries)
        ok = (item or {}).get("endpoint_id") == "alert.dismiss-single"
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] a write named by no generic verb is found through the tools "
              f"manifest: {(item or {}).get('endpoint_id')} after {len(cli.queries)} search(es)")
        ok = find_write_endpoint(_FakeSearchClient(catalog[:1]), "auvik_search", queries) is None
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] a catalog with no write endpoint still yields none")
        ok = write_queries(Path(td) / "no-such-skill") == list(WRITE_QUERIES)
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] no tools manifest -> the generic write verbs only")
        ok = queries[:len(WRITE_QUERIES)] == list(WRITE_QUERIES) and "alert list" not in queries
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] generic verbs first; a GET tool's name is never a write query")
    return bad


def endpoint_params_self_test() -> int:
    """The write-gating probe's placeholders pass a connector that validates the
    body before sending (datto-saas's bulk seat guard): an enum takes its first
    value, an array that must not be empty gets one item, a path parameter and a
    plain string keep the probe value, and an optional property stays unset."""
    item = {"path": "/saas/{saasCustomerId}/bulkSeatChange", "params_schema": {
        "required": ["seat_type", "ids", "note"],
        "properties": {"seat_type": {"type": "string", "enum": ["User", "SharedMailbox"]},
                       "ids": {"type": "array", "minItems": 1, "items": {"type": "string"}},
                       "note": {"type": "string"}, "extra": {"type": "string", "enum": ["a"]}}}}
    got = endpoint_params(item)
    want = {"seat_type": "User", "ids": ["mcp-gate-probe"], "note": "mcp-gate-probe",
            "saasCustomerId": PROBE_VALUE}
    ok = got == want
    print(f"  [{'PASS' if ok else 'FAIL'}] write-gating placeholders a validating connector accepts: {got}")
    return 0 if ok else 1


def kit_tls_self_test() -> int:
    """A credential-kit adopter against the fake vendor, both ways. The stand-in
    client refuses to send a credential over plain http and trusts only what it
    was told, as credkit.Transport does: over plain http nothing reaches the
    vendor (feature 2's false failure), over TLS with trust the request does, and
    without trust it does not. The proxy port stays plain and keeps refusing and
    logging other hosts. A connector with no kit gets a plain vendor and no new
    variable, so its verdict cannot move."""
    bad = 0

    def line(ok: bool, what: str) -> None:
        nonlocal bad
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] kit TLS: {what}")

    def kit_client(vendor: FakeVendor, trust: str | None) -> bool:
        if not vendor.url.startswith("https://"):
            return False  # the kit refuses before anything is sent
        tls = ssl.create_default_context(cafile=trust)
        handlers = (urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=tls))
        try:
            urllib.request.build_opener(*handlers).open(vendor.url + "/v1/items", timeout=10).read()
            return True
        except (OSError, http.client.HTTPException):
            return False

    with tempfile.TemporaryDirectory(prefix="verify-kit-tls-") as td:
        d = Path(td)
        cert = kit_tls.make_cert(td)
        plain = FakeVendor(d / "a.jsonl", d / "b.jsonl")
        try:
            kit_client(plain, None)
            line(not plain.requests and plain.proxy_url == plain.url and plain.url.startswith("http://"),
                 "plain http: nothing from a kit adopter reaches the vendor, and the plain vendor is one port")
        finally:
            plain.stop()
        vendor = FakeVendor(d / "c.jsonl", d / "d.jsonl", cert)
        try:
            line(kit_client(vendor, cert[0]) and len(vendor.requests) == 1,
                 "TLS with the run's certificate trusted: the request reaches the vendor")
            n = len(vendor.requests)
            other = ssl.get_default_verify_paths().cafile
            line(not kit_client(vendor, other) and len(vendor.requests) == n,
                 "TLS with the certificate not trusted: refused, and the vendor saw no request")
            conn = http.client.HTTPConnection("127.0.0.1", vendor.proxy_srv.server_address[1], timeout=10)
            try:
                conn.request("GET", "http://vendor.example.invalid/v1/items")
                refused = conn.getresponse().status == 403
            finally:
                conn.close()
            line(refused and [e["target"] for e in vendor.egress] == ["http://vendor.example.invalid/v1/items"],
                 "the proxy port is plain http, and refuses and logs another host")
        finally:
            vendor.stop()
        skill = d / "skill"
        cfg = skill / "cli" / "internal" / "config"
        cfg.mkdir(parents=True)
        line(not kit_tls.adopts_kit(skill) and kit_tls.ca_cert_var(skill) is None,
             "a connector with no kit is not an adopter")
        (cfg / "config.go").write_text('v := cliutil.EnvOverride("ACME_CA_CERT")\n', encoding="utf-8")
        (skill / "cli" / "internal" / "credkit").mkdir()
        line(kit_tls.adopts_kit(skill) and kit_tls.trust_env(skill, "c") == {"SSL_CERT_FILE": "c", "ACME_CA_CERT": "c"},
             "an adopter is told to trust the certificate through its CA variable and SSL_CERT_FILE")
        line(kit_tls.refusal(skill, "darwin") is None, "an adopter with a CA variable can run on any host")
        (cfg / "config.go").write_text("package config\n", encoding="utf-8")
        line(kit_tls.refusal(skill, "linux") is None and bool(kit_tls.refusal(skill, "darwin")),
             "an adopter with no CA variable runs on Linux, and elsewhere is a harness refusal, not a verdict")
    return bad


def winrm_exchange(stub: FakeWinRM, cafile: str, username: str, password: str, pin: bool = True) -> tuple[int, str]:
    """A stand-in for the connector's WinRM client: Create (carrying WSPP_SCRIPT),
    Command (the -EncodedCommand bootstrap, pinning the script's SHA-256 unless
    pin is False), Receive, Signal, Delete, with Basic auth over TLS trusting
    only the stub's certificate. Returns the last HTTP status and the stdout."""
    script = b"Emit-Ok @()"
    digest = hashlib.sha256(script).hexdigest().upper() if pin else "0" * 64
    boot = base64.b64encode(f"if($h -cne '{digest}'){{exit 0}}".encode("utf-16-le")).decode()
    auth = "Basic " + base64.b64encode(f"{username}:{password}".encode()).decode()
    tls = ssl.create_default_context(cafile=cafile)

    def post(action: str, shell: str, body: str) -> tuple[int, str]:
        sel = f'<w:SelectorSet><w:Selector Name="ShellId">{shell}</w:Selector></w:SelectorSet>' if shell else ""
        env = (f'<s:Envelope><s:Header><a:Action s:mustUnderstand="true">http://schemas.microsoft.com/wbem/wsman/1/'
               f"windows/shell/{action}</a:Action>{sel}</s:Header><s:Body>{body}</s:Body></s:Envelope>")
        conn = http.client.HTTPSConnection("127.0.0.1", stub.port, context=tls, timeout=10)
        try:
            conn.request("POST", "/wsman", body=env.encode(), headers={"Authorization": auth})
            resp = conn.getresponse()
            return resp.status, resp.read().decode("utf-8", "replace")
        finally:
            conn.close()

    status, body = post("Create", "", '<rsp:Shell><rsp:Environment><rsp:Variable Name="WSPP_SCRIPT">'
                                     + base64.b64encode(script).decode() + "</rsp:Variable></rsp:Environment></rsp:Shell>")
    m = WINRM_SHELL_RE.search(body)
    if status != 200 or not m:
        return status, ""
    shell = m.group(1)
    post("Command", shell, "<rsp:CommandLine><rsp:Command>powershell.exe</rsp:Command><rsp:Arguments>-NoLogo "
                           f"-NoProfile -NonInteractive -EncodedCommand {boot}</rsp:Arguments></rsp:CommandLine>")
    status, body = post("Receive", shell, "<rsp:Receive/>")
    out = re.search(r'<rsp:Stream Name="stdout"[^>]*>([^<]*)<', body)
    post("Signal", shell, "<rsp:Signal/>")
    post("Delete", shell, "")
    return status, base64.b64decode(out.group(1)).decode() if out else ""


def winrm_self_test() -> int:
    """The WinRM stub feature 2 drives a WinRM connector against, both ways,
    through winrm_verdict: a correct exchange is PASS; a wrong password or
    account is answered 401, serves nothing and is FAIL; a stub that answers
    nothing or a generic reply, and a script the bootstrap does not pin, are
    FAIL. HTTP connectors never reach this code: only a declared slug does."""
    cases = [  # (label, reply override, account, pinned script, expected verdict, expected status)
        ("correct exchange", None, ("acct", "right-password"), True, PASS, 200),
        ("wrong password refused", None, ("acct", "wrong-password"), True, FAIL, 401),
        ("wrong account refused", None, ("other", "right-password"), True, FAIL, 401),
        ("stub answers nothing", "", ("acct", "right-password"), True, FAIL, 200),
        ("stub answers a generic reply", '{"ok":true,"data":[]}', ("acct", "right-password"), True, FAIL, 200),
        ("script not pinned by the bootstrap", None, ("acct", "right-password"), False, FAIL, 200),
    ]
    bad = 0
    with tempfile.TemporaryDirectory(prefix="verify-winrm-") as td:
        cert = kit_tls.make_cert(td)
        for label, reply, (user, pw), pin, want, want_status in cases:
            stub = FakeWinRM(cert, "acct", "right-password", Path(td) / "winrm.jsonl", reply)
            try:
                status, out = winrm_exchange(stub, cert[0], user, pw, pin)
                a = {"tool": "stand-in", "arguments": {}, "is_error": status != 200 or '"ok":false' in out,
                     "carries_marker": stub.marker in out, "answer_head": out[:200], "winrm_requests": stub.since(0)}
                got = winrm_verdict("stand-in", [a])
            finally:
                stub.stop()
            ok = got["verdict"] == want and status == want_status
            if want_status == 401:
                ok = ok and not any(r["authorized"] or r["served"] for r in a["winrm_requests"])
            bad += not ok
            print(f"  [{'PASS' if ok else 'FAIL'}] WinRM stub: {label}: HTTP {status}, f2 {got['verdict']}")
        # Each request is recorded before its reply starts: the count of records when the Nth reply begins is N.
        stub = FakeWinRM(cert, "acct", "right-password", Path(td) / "winrm.jsonl")
        seen: list[int] = []
        handler = stub.srv.RequestHandlerClass
        orig = handler._reply

        def counting(h, code, inner):
            seen.append(len(stub.requests))
            return orig(h, code, inner)
        handler._reply = counting
        try:
            winrm_exchange(stub, cert[0], "acct", "right-password")
        finally:
            stub.stop()
        ok = len(seen) == 5 and seen == list(range(1, 6))
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] WinRM stub: each request recorded before its reply: records at each reply {seen}")
    return bad


def dsm_self_test() -> int:
    """The fake vendor answers a Synology DSM WebAPI call in DSM's envelope and
    every other request exactly as before ([] for GET, {} otherwise), over HTTP."""
    info = {"success": True, "data": {"SYNO.API.Auth": {"path": "entry.cgi", "minVersion": 1, "maxVersion": 7}}}
    login = {"success": True, "data": {"sid": "verify-fake-dsm-sid", "synotoken": "verify-fake-dsm-synotoken"}}
    empty = {"success": True, "data": {}}
    form, as_json = "application/x-www-form-urlencoded; charset=utf-8", "application/json"
    cases = [  # (method, request target, content type, body, expected reply)
        ("GET", "/webapi/query.cgi?api=SYNO.API.Info&method=query&version=1&query=all", None, b"", info),
        ("POST", "/webapi/auth.cgi", form, b"api=SYNO.API.Auth&version=6&method=login&account=a", login),
        ("GET", "/webapi/entry.cgi?api=SYNO.API.Auth&method=logout&version=6", None, b"", empty),
        ("GET", "/webapi/entry.cgi?api=SYNO.ActiveBackup.Inventory&method=list&version=1", None, b"", empty),
        ("GET", "/api/v1/devices", None, b"", []),
        ("POST", "/api/v1/devices", as_json, b'{"name":"x"}', {}),
        ("GET", "/webapi/entry.cgi?method=list", None, b"", []),
        ("GET", "/webapi/entry.cgi?myapi=SYNO.API.Info", None, b"", []),
        ("GET", "/v2/devices?api=SYNO.API.Info", None, b"", []),
        ("GET", "/v2/devices?next=/webapi/query.cgi&api=SYNO.API.Info", None, b"", []),
        ("GET", "/proxy/webapi/query.cgi?api=SYNO.API.Info", None, b"", []),
        ("GET", "/webapi/entry.cgi?api=SYNOLOGY.Info", None, b"", []),
        # An api= inside a body that is not a form is text, not a parameter.
        ("POST", "/webapi/entry.cgi", as_json, b'{"note":"&api=SYNO.API.Info&"}', {}),
        ("POST", "/webapi/entry.cgi", None, b"api=SYNO.API.Info", {}),
        # A proxy-style target names another host, whatever the case of its scheme.
        ("GET", "HTTP://example.invalid/webapi/query.cgi?api=SYNO.API.Info", None, b"", []),
        # A target that does not parse as a URL is still answered, never raised on.
        ("GET", "HTTP://[broken/webapi/query.cgi?api=SYNO.API.Info", None, b"", []),
        ("GET", "//[broken/webapi/query.cgi?api=SYNO.API.Info", None, b"", []),
    ]
    bad = 0
    with tempfile.TemporaryDirectory(prefix="verify-dsm-") as td:
        d = Path(td)
        vendor = FakeVendor(d / "vendor-requests.jsonl", d / "egress-attempts.jsonl")
        try:
            for method, target, ctype, body, want in cases:
                conn = http.client.HTTPConnection("127.0.0.1", vendor.srv.server_address[1], timeout=10)
                try:
                    conn.request(method, target, body=body or None, headers={"Content-Type": ctype} if ctype else {})
                    raw = conn.getresponse().read()
                    got = json.loads(raw)
                except (OSError, http.client.HTTPException, ValueError) as e:
                    got = f"no reply: {type(e).__name__}"
                finally:
                    conn.close()
                ok = got == want
                bad += not ok
                kind = "DSM envelope" if isinstance(got, dict) and "success" in got else f"generic reply {got}"
                print(f"  [{'PASS' if ok else 'FAIL'}] fake vendor {method} {target} ({ctype}): {kind}")
        finally:
            vendor.stop()
    return bad


def vcenter_self_test() -> int:
    """The fake vendor answers a vSphere Automation API login with a bare JSON
    string session id (201) and every other request exactly as before."""
    cases = [  # (method, request target, expected status, expected reply)
        ("POST", "/api/session", 201, "verify-fake-vcenter-session"),
        ("POST", "/api/session?x=1", 201, "verify-fake-vcenter-session"),
        ("GET", "/api/session", 200, []),
        ("DELETE", "/api/session", 200, {}),
        ("POST", "/api/sessions", 200, {}),
        ("POST", "/proxy/api/session", 200, {}),
        ("POST", "/rest/com/vmware/cis/session", 200, {}),
        ("GET", "/api/vcenter/vm", 200, []),
    ]
    bad = 0
    with tempfile.TemporaryDirectory(prefix="verify-vcenter-") as td:
        d = Path(td)
        vendor = FakeVendor(d / "vendor-requests.jsonl", d / "egress-attempts.jsonl")
        try:
            for method, target, want_status, want in cases:
                conn = http.client.HTTPConnection("127.0.0.1", vendor.srv.server_address[1], timeout=10)
                try:
                    conn.request(method, target, body=b"" if method == "POST" else None)
                    resp = conn.getresponse()
                    status, got = resp.status, json.loads(resp.read() or b"null")
                except (OSError, http.client.HTTPException, ValueError) as e:
                    status, got = 0, f"no reply: {type(e).__name__}"
                finally:
                    conn.close()
                ok = status == want_status and got == want
                bad += not ok
                print(f"  [{'PASS' if ok else 'FAIL'}] fake vendor {method} {target}: {status} {got!r}")
        finally:
            vendor.stop()
    return bad


def longest_token_run(text: str, token: str, k: int = 8) -> int:
    """The longest run (k or more characters) of `text` that is part of `token`; 0 if none."""
    best = 0
    for i in range(len(text) - k + 1):
        if text[i:i + k] in token:
            j = i + k
            while j < len(text) and text[i:j + 1] in token:
                j += 1
            best = max(best, j - i)
    return best


def redaction_self_test() -> int:
    """A token that a cut lands inside, at, or just after never reaches
    evidence, a verdict or the console, not even 8 of its characters: each
    value is redacted whole before it is cut. Covers feature 3's body heads
    (written as http_bearer_auth writes bearer-auth.json), the fake vendor's
    request body heads, tail()'s width, and a console line cut from a reason."""
    bad = 0
    for off in (237, 270, 292, 296, 299):
        token = secrets.token_hex(32)
        RUN_SECRETS.append(token)
        _StubMCPHandler.token, _StubMCPHandler.mode = token, f"echo-at-{off}"
        srv = ThreadingHTTPServer(("127.0.0.1", 0), _StubMCPHandler)
        threading.Thread(target=srv.serve_forever, daemon=True).start()
        with tempfile.TemporaryDirectory(prefix="verify-redact-") as td:
            d = Path(td)
            try:
                record = {"origin_allowlist_expected": [True, "self-test"]}
                probe_bearer(f"http://127.0.0.1:{srv.server_address[1]}/mcp", token, "STUB_MCP_HTTP_TOKEN",
                             record, RUN_SECRETS)
            finally:
                srv.shutdown()
                srv.server_close()
            write_json(d / "bearer-auth.json", {k: (redact(v) if isinstance(v, str) else v) for k, v in record.items()})
            vendor = FakeVendor(d / "vendor-requests.jsonl", d / "egress-attempts.jsonl")
            try:
                payload = ("z" * off + token + " end").encode()
                req = urllib.request.Request(vendor.url + "/v1/items", data=payload, method="PUT")
                urllib.request.build_opener(urllib.request.ProxyHandler({})).open(req, timeout=10).read()
            finally:
                vendor.stop()
            reason = f"server said: {tail('w' * (off - 60) + token + ' tail')}"
            write_json(d / "verdict.json", verdict(FAIL, reason))
            console = redact(reason)[:150]
            worst = max([longest_token_run(f.read_text(encoding="utf-8"), token) for f in d.iterdir()]
                        + [longest_token_run(console, token)])
        ok = worst == 0
        bad += not ok
        print(f"  [{'PASS' if ok else 'FAIL'}] a token at offset {off}, cut by a 300-character body head, a vendor "
              f"body head, tail() and a console line: longest run of it in evidence {worst} (must be 0)")
    return bad


# --------------------------------------------------------------------------- #
# Feature 4: write gating on the execute tool.
# --------------------------------------------------------------------------- #

def is_token_mint(req: dict) -> bool:
    return req["method"] == "POST" and "grant_type" in (req.get("body_head") or "")


def endpoint_requests(reqs: list[dict], method: str, path: str) -> list[dict]:
    """Requests for this endpoint: same method, and (when the endpoint has a
    path) a request path ending in that path, placeholders matching one
    segment. A fragment (JSON-RPC '#method') is never sent, so it is dropped."""
    template = (path or "").split("#", 1)[0].split("?", 1)[0].rstrip("/")
    # An absolute template (one host per service, e.g. https://{region}.iam.x/v2/roles)
    # reaches the fake vendor as its path only.
    template = re.sub(r"^[A-Za-z][A-Za-z0-9+.-]*://[^/]*", "", template)
    rx = None
    if template and template != "?":
        rx = re.compile(re.sub(r"\\\{[^}/]+\\\}", "[^/]+", re.escape(template)) + "/?$")
    out = []
    for r in reqs:
        if r["method"] != method or is_token_mint(r):
            continue
        if rx is None or rx.search(r["path"].split("?", 1)[0]):
            out.append(r)
    return out


def pick_search(names: list[str], execute: str) -> str | None:
    """The search tool that belongs to `execute`: the same prefix
    (zammad_execute -> zammad_search). A connector can list other *_search
    tools (zammad's kb_search searches knowledge-base answers, not endpoints)."""
    want = execute[:-len("_execute")] + "_search"
    return want if want in names else None


MAX_MANIFEST_QUERIES = 200


def write_queries(skill: Path) -> list[str]:
    """The generic write verbs, then the name of every state-changing tool in
    the connector's press/tools-manifest.json (an API whose only write is, say,
    POST alert_dismiss-single matches none of the verbs). Search still finds
    the endpoint, as an agent would; the manifest only supplies the words."""
    out = list(WRITE_QUERIES)
    try:
        tm = json.loads((skill / "press" / "tools-manifest.json").read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return out
    tools = tm.get("tools") if isinstance(tm, dict) else None
    for t in tools if isinstance(tools, list) else []:
        if not isinstance(t, dict) or str(t.get("method", "")).upper() not in WRITE_METHODS:
            continue
        q = re.sub(r"[_\-./{}]+", " ", str(t.get("name") or "")).strip()
        if q and q not in out:
            out.append(q)
        if len(out) >= len(WRITE_QUERIES) + MAX_MANIFEST_QUERIES:
            break
    return out


def find_write_endpoint(cli, search: str, queries: list[str]) -> dict | None:
    """The first POST, PUT, PATCH or DELETE result for the first query that
    has one, preferring a path with no placeholders."""
    for q in queries:
        writes = [i for i in search_endpoints(cli, search, q)
                  if str(i.get("method", "")).upper() in WRITE_METHODS]
        if writes:
            writes.sort(key=lambda i: ("{" in (i.get("path") or ""),
                                       WRITE_METHODS.index(str(i["method"]).upper())))
            return writes[0]
    return None


def looks_like_preview(text: str) -> bool:
    data = parse_json_lenient(text)
    if isinstance(data, dict) and (data.get("confirmation_required") is True or data.get("preview") is True):
        return True
    return "confirm" in (text or "").lower()


def f4_write_gating(ctx: Ctx) -> dict:
    d = ctx.fdir("f4-write-gating")
    try:
        with ctx.stdio("session", d) as cli:
            cli.initialize()
            names = [t.get("name", "") for t in cli.list_tools()]
            execute = next((n for n in names if n.endswith("_execute")), None)
            if not execute:
                return verdict(SKIP, "tools/list has no *_execute tool")
            search = pick_search(names, execute)
            if not search:
                return verdict(ERROR, f"{execute} is listed but there is no "
                                      f"{execute[:-len('_execute')]}_search tool to find a write endpoint")
            item = find_write_endpoint(cli, search, write_queries(ctx.skill))
            write_json(d / "chosen-endpoint.json", item)
            if not item:
                return verdict(ERROR, f"{search} found no POST/PUT/PATCH/DELETE endpoint to drive")
            method, path = str(item["method"]).upper(), item.get("path") or "?"
            args = {"endpoint_id": item.get("endpoint_id") or item.get("id"), "params": endpoint_params(item)}
            first = call_observed(ctx, cli, execute, args)
            write_json(d / "call-without-confirm.json", first)
            sent = endpoint_requests(first["vendor_requests"], method, path)
            if sent:
                return verdict(FAIL, f"{execute} sent {method} {path} to the vendor at once, with no confirm "
                                     "and no preview")
            if not looks_like_preview(first["answer_head"]):
                return verdict(ERROR, f"{execute} neither sent nor previewed {method} {path}: "
                                      f"{tail(first['answer_head'], 1)}")
            second = call_observed(ctx, cli, execute, {**args, "confirm": True})
            write_json(d / "call-with-confirm.json", second)
    except ServerGone as e:
        return verdict(FAIL, str(e))
    if not endpoint_requests(second["vendor_requests"], method, path):
        return verdict(FAIL, f"{execute} previewed {method} {path}, but confirm=true did not send it either: "
                             f"{tail(second['answer_head'], 1)}")
    return verdict(PASS, f"{execute} previewed {method} {path} without confirm (nothing sent); confirm=true sent it")


# --------------------------------------------------------------------------- #
# Feature 5: the read-only switch.
# --------------------------------------------------------------------------- #

def f5_read_only(ctx: Ctx) -> dict:
    d = ctx.fdir("f5-read-only")
    var, src = ctx.names["read_only"], ctx.names["read_only_from"]
    try:
        with ctx.stdio("default", d) as cli:
            cli.initialize()
            default = cli.list_tools()
        with ctx.stdio("read-only", d, env=ctx.env(**{var: "1"})) as cli:
            cli.initialize()
            ro = cli.list_tools()
    except ServerGone as e:
        return verdict(FAIL, str(e))
    write_json(d / "tools-default.json", default)
    write_json(d / f"tools-{var}.json", ro)

    writes = [t["name"] for t in default if not is_read_tool(t)]
    still = [t["name"] for t in ro if not is_read_tool(t)]
    if not ro:
        return verdict(FAIL, f"{var}=1 lists no tools at all")
    if still:
        return verdict(FAIL, f"{var}=1 (name from {src}) still lists {len(still)} write tool(s): "
                             f"{', '.join(still[:4])}{' ...' if len(still) > 4 else ''}")
    return verdict(PASS, f"{var}=1 drops {len(writes)} write tool(s)"
                         f"{' (' + ', '.join(writes[:3]) + ')' if writes else ''}; {len(ro)} read-only tools remain")


# --------------------------------------------------------------------------- #
# Feature 6: local data.
# --------------------------------------------------------------------------- #

def under(path: str, root: Path) -> bool:
    try:
        Path(path).resolve().relative_to(root.resolve())
        return True
    except (ValueError, OSError):
        return False


def store_files(home: Path) -> set[str]:
    return {str(p) for p in home.rglob("*.db") if p.is_file()}


def store_location(rep, home: Path) -> tuple[str | None, str | None]:
    """(store dir, the doctor field that named it). The 4.34 shape is
    paths.data.dir; older builds name the database file (e.g. cache.db_path).
    A store path outside the scratch HOME returns (None, field)."""
    if isinstance(rep, dict) and isinstance(rep.get("paths"), dict):
        data = rep["paths"].get("data")
        if isinstance(data, dict) and isinstance(data.get("dir"), str):
            return (data["dir"] if under(data["dir"], home) else None), "paths.data.dir"
    found: list[tuple[str, str]] = []

    def walk(o, key: str) -> None:
        if isinstance(o, dict):
            for k, v in o.items():
                walk(v, f"{key}.{k}" if key else str(k))
        elif isinstance(o, list):
            for i, v in enumerate(o):
                walk(v, f"{key}[{i}]")
        elif isinstance(o, str) and o.startswith("/") and o.endswith(".db"):
            found.append((key, o))

    walk(rep, "")
    for key, path in found:
        return (str(Path(path).parent) if under(path, home) else None), key
    return None, None


def f6_local_data(ctx: Ctx) -> dict:
    d = ctx.fdir("f6-local-data")
    var = ctx.names["no_local_data"]
    checks: dict[str, tuple[str, str]] = {}

    doc = ctx.run("doctor", [ctx.cli, "doctor", "--json"], d)
    rep = parse_json_lenient(doc["stdout"])
    data_dir, where = store_location(rep, ctx.home)
    if where and data_dir is None:
        checks["doctor"] = (ERROR, f"doctor names a store outside the scratch HOME ({where})")
    elif data_dir:
        rel = Path(data_dir).resolve().relative_to(ctx.home.resolve())
        extra = (f"; local_data: {redact(str(rep['local_data']))[:50]}" if isinstance(rep, dict) and "local_data" in rep
                 else "; no local_data field")
        checks["doctor"] = (PASS, f"reports the store at ~/{rel} ({where}){extra}")
    else:
        checks["doctor"] = (FAIL, f"doctor --json (rc={doc['rc']}) names no local store path")

    probe = ctx.run("purge-help", [ctx.cli, "local-data", "purge", "--help"], d)
    if probe["rc"] != 0:
        checks["purge"] = (FAIL, f"no `local-data purge` command ({tail(probe['stderr'], 1)})")
    else:
        target = Path(data_dir) if data_dir else ctx.home / ".local" / "share" / ctx.cli_name
        target.mkdir(parents=True, exist_ok=True)
        seed = target / "data.db"
        seed.write_bytes(b"verify-msp-skills scaffolding: a stand-in store file, not a database\n")
        pv = ctx.run("purge-preview", [ctx.cli, "local-data", "purge", "--json"], d)
        kept = seed.exists()
        ap = ctx.run("purge-apply", [ctx.cli, "local-data", "purge", "--yes", "--json"], d)
        gone = not seed.exists()
        seed.unlink(missing_ok=True)
        listed = str(seed) in pv["stdout"] or str(seed.resolve()) in pv["stdout"]
        if pv["rc"] == 0 and listed and kept and ap["rc"] == 0 and gone:
            checks["purge"] = (PASS, "preview listed the store and deleted nothing; --yes deleted it")
        else:
            checks["purge"] = (FAIL, f"preview rc={pv['rc']} listed={listed} kept={kept}; "
                                     f"--yes rc={ap['rc']} deleted={gone}")

    sh = ctx.run("sync-help", [ctx.cli, "sync", "--help"], d)
    if sh["rc"] != 0:
        checks["sync"] = (SKIP, "no sync command")
    else:
        before = store_files(ctx.home)
        mark = ctx.vendor.mark()
        r = ctx.run("sync-no-local-data", [ctx.cli, "sync", "--json"], d, env=ctx.env(**{var: "1"}), timeout=90)
        reqs = ctx.vendor.since(mark)
        new = sorted(store_files(ctx.home) - before)
        named = var in (r["stdout"] + r["stderr"])
        if r["rc"] not in (0, None) and named and not reqs and not new:
            checks["sync"] = (PASS, f"{var}=1 refused sync (rc={r['rc']}) before any request")
        else:
            checks["sync"] = (FAIL, f"sync ran under {var}=1: rc={r['rc']}{' (timed out)' if r['timed_out'] else ''}, "
                                    f"{len(reqs)} vendor request(s), {len(new)} store file(s) written")
    return combine(checks)


FEATURE_FUNCS = {"install": f1_install, "mcp-stdio": f2_mcp_stdio, "mcp-http": f3_mcp_http,
                 "write-gating": f4_write_gating, "read-only": f5_read_only, "local-data": f6_local_data}


# --------------------------------------------------------------------------- #
# run / doctor / report / cleanup.
# --------------------------------------------------------------------------- #

def start_vendor(ctx: Ctx, run_dir: Path) -> FakeVendor:
    """The fake vendor for this connector: plain http with nothing added, unless
    it adopted the credential kit, when it serves https and is told to trust it."""
    tls = None
    if kit_tls.adopts_kit(ctx.skill):
        why = kit_tls.refusal(ctx.skill)
        if why:
            raise HarnessError(why)
        tls_dir = ctx.scratch / "vendor-tls"
        tls_dir.mkdir(parents=True, exist_ok=True)
        try:
            tls = kit_tls.make_cert(str(tls_dir))
        except RuntimeError as e:
            raise HarnessError(str(e)) from e
        ctx.vendor_trust = kit_tls.trust_env(ctx.skill, tls[0])
    return FakeVendor(run_dir / "vendor-requests.jsonl", run_dir / "egress-attempts.jsonl", tls)


def setup_egress_guard(ctx: Ctx, mode: str) -> dict:
    if mode == "sandbox":
        if not os.access(SANDBOX_EXEC, os.X_OK):
            raise HarnessError("the egress guard needs sandbox-exec (macOS); on Linux pass --egress-guard proxy-env")
        profile = ctx.scratch / "egress.sb"
        profile.write_text(SANDBOX_PROFILE, encoding="utf-8")
        ctx.sandbox = [SANDBOX_EXEC, "-f", str(profile)]
        ok, detail = prove_sandbox(ctx.sandbox, ctx.env())
        if ok:
            ca_args = ["--cacert", ctx.vendor.ca] if ctx.vendor.ca else []
            r = subprocess.run(ctx.wrap(["/usr/bin/curl", "-s", "-m", "5", "-o", "/dev/null", "-w", "%{http_code}",
                                         *ca_args, ctx.vendor.url + "/__verify_loopback_probe"]),
                               env=ctx.env(), capture_output=True, text=True, timeout=20)
            if r.stdout.strip() != "200":
                ok, detail = False, f"the sandbox also blocks loopback (curl rc={r.returncode}); nothing can be driven"
        return {"guard": "sandbox-exec", "proved": ok, "detail": detail}
    return {"guard": "proxy-env", "proved": True,
            "detail": "HTTP(S)_PROXY points at the fake vendor, which refuses and logs other hosts; "
                      "a client that ignores proxy settings is not contained"}


def run_slug(slug: str, features: list[int], args) -> int:
    meta = registry.skills().get(slug)
    if meta is None or registry.is_markdown_only(slug) or not (registry.skill_path(slug) / "cli").is_dir():
        print(f"verify: {slug!r} is not a connector with a cli/ tree", file=sys.stderr)
        return EXIT_USAGE
    root = Path(args.evidence_root).resolve()
    run_dir = root / slug / f"{dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%SZ')}-{os.getpid()}"
    run_dir.mkdir(parents=True, exist_ok=True)
    scratch = Path(tempfile.mkdtemp(prefix=f"{SCRATCH_PREFIX}{slug}-"))
    ctx = Ctx(slug, run_dir, scratch)
    osname, arch = host_platform()
    info = {"slug": slug, "connector_version": ctx.version, "press_version": ctx.press, "head": git_head(),
            "host": f"{osname}/{arch}", "started_at": stamp(), "scratch": str(scratch),
            "run_dir": str(run_dir), "env_names": ctx.names, "features_requested": features}
    paths = real_paths(slug, meta)
    before = fingerprint(paths)
    write_json(run_dir / "isolation-before.json", before)
    results: dict[str, dict] = {}
    problems: list[str] = []
    gates = None
    try:
        if args.gates:
            gates = run_gates(slug, run_dir)
        ctx.vendor = start_vendor(ctx, run_dir)
        info["fake_vendor"] = ctx.vendor.url
        guard = setup_egress_guard(ctx, args.egress_guard)
        info["egress_guard"] = guard
        if not guard["proved"]:
            raise HarnessError(f"egress guard not proven: {guard['detail']}")
        build(ctx)
        if 1 not in features:
            place_built(ctx)
            ctx.notes.append("feature 1 not requested: drove the built binaries without install.sh")
        for num, fid, _title in FEATURES:
            if num not in features:
                continue
            try:
                res = FEATURE_FUNCS[fid](ctx)
            except HarnessError as e:
                res = verdict(ERROR, str(e))
            except Exception as e:  # noqa: BLE001 - any harness crash is ERROR, with its traceback kept
                (ctx.fdir(f"f{num}-{fid}") / "traceback.txt").write_text(traceback.format_exc(), encoding="utf-8")
                res = verdict(ERROR, f"harness exception: {e!r}")
            if not (ctx.cli.is_file() and ctx.mcp.is_file()):
                place_built(ctx)
                if num == 1:
                    ctx.notes.append("install.sh did not install; features 2-6 drove the built binaries directly")
            results[fid] = res
            print(f"  {num} {fid:<13} {res['verdict']:<5} {redact(res['reason'])[:150]}", flush=True)
    except HarnessError as e:
        problems.append(str(e))
    except Exception as e:  # noqa: BLE001 - a crashed harness still writes its verdict
        (run_dir / "traceback.txt").write_text(traceback.format_exc(), encoding="utf-8")
        problems.append(f"harness exception: {e!r}")
    finally:
        for num, fid, _t in FEATURES:
            if num in features and fid not in results:
                results[fid] = verdict(ERROR, f"not run: {problems[-1] if problems else 'harness stopped'}")
        listing = sorted(str(p.relative_to(ctx.home)) for p in ctx.home.rglob("*"))
        (run_dir / "scratch-home-tree.txt").write_text("\n".join(listing) + "\n", encoding="utf-8")
        if ctx.vendor is not None:
            info["vendor_requests"] = len(ctx.vendor.requests)
            info["egress_attempts"] = ctx.vendor.egress
            ctx.vendor.stop()
        cleanup_ctx(ctx, keep=args.keep_scratch)
    after = fingerprint(paths)
    write_json(run_dir / "isolation-after.json", after)
    changed = [p for p in before if before[p] != after.get(p)]
    info["isolation"] = {"real_paths_checked": len(paths), "changed": changed,
                         "existing_before": sum(1 for v in before.values() if v.get("exists") is True)}
    if changed:
        problems.append(f"isolation: {len(changed)} real path(s) changed during the run: {changed[:3]}")
    info.update({"finished_at": stamp(), "harness_problems": problems, "notes": ctx.notes, "gates": gates,
                 "scratch_removed": not scratch.exists(), "features": results})
    write_json(run_dir / "verdict.json", info)
    code = exit_code(results, problems)
    summary = summary_text(info, code)
    (run_dir / "SUMMARY.txt").write_text(summary, encoding="utf-8")
    survived = (run_dir / "verdict.json").is_file() and (run_dir / "SUMMARY.txt").is_file()
    print(summary, end="")
    print(f"evidence: {run_dir} ({'kept after cleanup' if survived else 'MISSING'})")
    return code if survived else EXIT_HARNESS


def exit_code(results: dict[str, dict], problems: list[str]) -> int:
    vs = [r["verdict"] for r in results.values()]
    if problems or ERROR in vs:
        return EXIT_HARNESS
    return EXIT_FAIL if FAIL in vs else EXIT_OK


def summary_text(info: dict, code: int) -> str:
    iso = info["isolation"]
    guard = info.get("egress_guard") or {}
    lines = [f"verify-msp-skills  {info['slug']}  {info['started_at']}",
             f"connector {info['connector_version']}  press {info['press_version']}  head {info['head']}  "
             f"host {info['host']}  egress guard {guard.get('guard', 'none')}"
             f"{' (proved)' if guard.get('proved') else ' (NOT proved)'}",
             f"isolation: {iso['real_paths_checked']} real paths checked "
             f"({iso['existing_before']} existed), {len(iso['changed'])} changed; "
             f"scratch {'removed' if info['scratch_removed'] else 'kept'}; "
             f"egress attempts refused: {len(info.get('egress_attempts') or [])}"]
    if info.get("gates"):
        lines.append(f"gates (verify_all.sh --slug): {info['gates']['verdict']}  {info['gates']['detail']}")
    lines.append("")
    for num, fid, _t in FEATURES:
        r = info["features"].get(fid)
        if r:
            lines.append(f"{num} {fid:<13} {r['verdict']:<5} {redact(r['reason'])[:220]}")
    for p in info["harness_problems"]:
        lines.append(f"HARNESS: {p}")
    for n in info["notes"]:
        lines.append(f"note: {n}")
    meaning = {EXIT_OK: "every check held", EXIT_FAIL: "harness ran, at least one check failed",
               EXIT_HARNESS: "harness broke (ERROR) or isolation not proven; see HARNESS lines"}[code]
    lines.append(f"exit {code}: {meaning}")
    return "\n".join(lines) + "\n"


def run_gates(slug: str, run_dir: Path) -> dict:
    """CI's gates for this connector come from verify_all.sh, not from here."""
    out = run_dir / "gates-verify_all.json"
    argv = ["bash", str(VERIFY_ALL), "--slug", slug, "--verdict", str(out)]
    r = subprocess.run(argv, cwd=str(REPO), capture_output=True, text=True, timeout=7200)
    (run_dir / "gates-verify_all.log").write_text(r.stdout + r.stderr, encoding="utf-8")
    return {"argv": argv, "rc": r.returncode, "verdict": "PASS" if r.returncode == 0 else "FAIL",
            "detail": f"rc={r.returncode}; verdict file {out.name}"}


def cleanup_ctx(ctx: Ctx, keep: bool) -> None:
    for ent in ctx.pids:
        if pid_alive(ent["pid"]) and cmd_matches(ent["pid"], ent["argv0"]):
            kill_group(ent["pid"])
    if not keep:
        shutil.rmtree(ctx.scratch, ignore_errors=True)
    ctx.save_state(cleaned=not keep)


def cmd_matches(pid: int, argv0: str) -> bool:
    try:
        r = subprocess.run(["ps", "-p", str(pid), "-o", "command="], capture_output=True, text=True, timeout=30)
    except subprocess.TimeoutExpired:
        return False  # cannot confirm it is ours: never signal it
    return Path(argv0).name in r.stdout


def cleanup_run(run_dir: Path) -> list[str]:
    """Stop what a run started and remove its scratch. Evidence is never touched."""
    state_file = run_dir / "state.json"
    if not state_file.is_file():
        return []
    state = json.loads(state_file.read_text(encoding="utf-8"))
    msgs = []
    for ent in state.get("pids", []):
        if pid_alive(ent["pid"]) and cmd_matches(ent["pid"], ent["argv0"]):
            kill_group(ent["pid"])
            msgs.append(f"stopped pid {ent['pid']} ({Path(ent['argv0']).name})")
    scratch = Path(state.get("scratch", ""))
    if scratch.name.startswith(SCRATCH_PREFIX) and scratch.is_dir():
        shutil.rmtree(scratch, ignore_errors=True)
        msgs.append(f"removed scratch {scratch}")
    state["cleaned"] = True
    write_json(state_file, state)
    return msgs


def leftover_runs(root: Path) -> list[Path]:
    out = []
    for state_file in sorted(root.glob("*/*/state.json")):
        try:
            state = json.loads(state_file.read_text(encoding="utf-8"))
        except (OSError, ValueError):
            continue
        owner = state.get("owner_pid")
        if owner and owner != os.getpid() and pid_alive(owner) and cmd_matches(owner, HERE.name):
            continue  # that run is still in progress; it cleans up after itself
        alive = any(pid_alive(e["pid"]) and cmd_matches(e["pid"], e["argv0"]) for e in state.get("pids", []))
        if alive or Path(state.get("scratch", "/nonexistent")).is_dir():
            out.append(state_file.parent)
    return out


def cmd_doctor(args) -> int:
    ok = True

    def line(good: bool, label: str, detail: str) -> None:
        nonlocal ok
        ok = ok and good
        print(f"[{'ok' if good else 'FAIL':<4}] {label}: {detail}")

    go = shutil.which("go")
    try:
        gov = subprocess.run([go, "version"], capture_output=True, text=True, timeout=60).stdout.strip() if go else ""
    except subprocess.TimeoutExpired:
        gov = ""
    line(bool(go), "go", gov or "not on PATH (needed to build the connector)")
    slug = args.slug
    meta = registry.skills().get(slug)
    skill = registry.skill_path(slug) if meta else None
    good = bool(meta) and not registry.is_markdown_only(slug) and (skill / "cli" / "cmd").is_dir()
    line(good, "connector", f"skills/{skill.name}/cli/cmd: {sorted(p.name for p in (skill / 'cli' / 'cmd').iterdir())}"
         if good else f"{slug!r} is not a registered connector with a cli/ tree")
    if not good:
        return EXIT_FAIL
    man = wiring.read_manifest(slug)
    press = man.get("printing_press_version") or (man.get("press_provenance") or {}).get("printing_press_version")
    line(True, "manifest", f"version {man.get('version')}, printing_press_version {press or 'unknown'}")
    names = env_names(slug, skill, man)
    line(True, "env names", ", ".join(f"{k}={names[k]} ({names[k + '_from']})"
                                      for k in ("read_only", "http_token", "no_local_data")))
    line((skill / "install.sh").is_file() and FAKE_GITHUB.is_file(), "installer",
         f"skills/{skill.name}/install.sh + {FAKE_GITHUB.relative_to(REPO)}")
    if args.egress_guard == "sandbox":
        if os.access(SANDBOX_EXEC, os.X_OK):
            with tempfile.TemporaryDirectory(prefix=SCRATCH_PREFIX) as tmp:
                prof = Path(tmp) / "egress.sb"
                prof.write_text(SANDBOX_PROFILE, encoding="utf-8")
                good, detail = prove_sandbox([SANDBOX_EXEC, "-f", str(prof)],
                                             {"PATH": "/usr/bin:/bin", "HOME": tmp})
            line(good, "egress guard", f"sandbox-exec: {detail}")
        else:
            line(False, "egress guard", "sandbox-exec missing; on Linux pass --egress-guard proxy-env (weaker)")
    else:
        line(True, "egress guard", "proxy-env only: a client that ignores HTTP(S)_PROXY is not contained")
    reader = shutil.which("lsof") or shutil.which("ss")
    line(bool(reader), "socket reader", reader or "neither lsof nor ss (feature 3 needs one)")
    main_go = next(iter(sorted((skill / "cli" / "cmd").glob("*-mcp/main.go"))), None)
    m = re.search(r'defaultHTTPAddr\s*=\s*"([^"]*)"', main_go.read_text(encoding="utf-8")) if main_go else None
    if m and m.group(1).rsplit(":", 1)[-1].isdigit():
        port = int(m.group(1).rsplit(":", 1)[1])
        line(not port_busy(port), "default HTTP port", f"{m.group(1)} {'busy' if port_busy(port) else 'free'}")
    else:
        line(True, "default HTTP port", "not declared in source; feature 3 reads it from --help")
    root = Path(args.evidence_root).resolve()
    try:
        root.mkdir(parents=True, exist_ok=True)
        probe = root / ".write-probe"
        probe.write_text("x")
        probe.unlink()
        writable = True
    except OSError:
        writable = False
    try:
        ignored = subprocess.run(["git", "-C", str(REPO), "check-ignore", "-q", str(root / "x" / "verdict.json")],
                                 timeout=60).returncode == 0
    except subprocess.TimeoutExpired:
        ignored = False
    line(writable and (ignored or not under(str(root), REPO)), "evidence root",
         f"{root} ({'writable' if writable else 'NOT writable'}, "
         f"{'git-ignored' if ignored else 'outside the repo' if not under(str(root), REPO) else 'NOT git-ignored'})")
    left = leftover_runs(root)
    line(not left, "leftover runs", "none" if not left else f"{len(left)}: run `cleanup --stale`")
    print("verdict: worth driving" if ok else "verdict: fix the FAIL lines first")
    return EXIT_OK if ok else EXIT_FAIL


def cmd_report(args) -> int:
    root = Path(args.evidence_root).resolve()
    rows = []
    for slug_dir in sorted(p for p in root.iterdir() if p.is_dir()) if root.is_dir() else []:
        runs = sorted(p for p in slug_dir.iterdir() if (p / "verdict.json").is_file())
        if runs:
            rows.append(json.loads((runs[-1] / "verdict.json").read_text(encoding="utf-8")))
    if args.json:
        print(json.dumps([{"slug": r["slug"], "press_version": r["press_version"], "head": r["head"],
                           "run_dir": r["run_dir"], "harness_problems": r["harness_problems"],
                           "features": {k: v["verdict"] for k, v in r["features"].items()}} for r in rows], indent=2))
        return EXIT_OK
    head = f"{'slug':<24} {'press':<8} " + " ".join(f"{n}-{f}"[:13].ljust(13) for n, f, _ in FEATURES)
    print(head)
    for r in rows:
        cells = " ".join((r["features"].get(f) or {}).get("verdict", "-").ljust(13) for _, f, _ in FEATURES)
        flag = "  HARNESS" if r["harness_problems"] else ""
        print(f"{r['slug']:<24} {r['press_version']:<8} {cells}{flag}")
    return EXIT_OK


def cmd_cleanup(args) -> int:
    root = Path(args.evidence_root).resolve()
    targets = [Path(args.run).resolve()] if args.run else leftover_runs(root)
    for run_dir in targets:
        msgs = cleanup_run(run_dir)
        print(f"{run_dir}: {'; '.join(msgs) if msgs else 'nothing to stop or remove'}")
    if not targets:
        print("nothing to clean")
    return EXIT_OK


def parse_features(text: str) -> list[int]:
    out = []
    for part in (p.strip() for p in text.split(",") if p.strip()):
        match = [n for n, fid, _ in FEATURES if part in (str(n), fid)]
        if not match:
            raise SystemExit(f"verify: unknown feature {part!r} (use 1-6 or {', '.join(f for _, f, _ in FEATURES)})")
        out += match
    return sorted(set(out))


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--evidence-root", default=str(EVIDENCE_ROOT), help="default: <repo>/.verify-evidence")
    sub = ap.add_subparsers(dest="cmd", required=True)
    d = sub.add_parser("doctor", help="read-only: is this host and connector worth driving?")
    d.add_argument("--slug", required=True)
    d.add_argument("--egress-guard", choices=("sandbox", "proxy-env"),
                   default="sandbox" if platform.system() == "Darwin" else "proxy-env")
    r = sub.add_parser("run", help="build, install, drive, capture evidence, clean up")
    r.add_argument("--slug", action="append", default=[], help="repeatable")
    r.add_argument("--all", action="store_true", help="every connector with a cli/ tree, one after another")
    r.add_argument("--features", default="1,2,3,4,5,6", help="e.g. 2,3 or mcp-stdio,mcp-http")
    r.add_argument("--keep-scratch", action="store_true", help="keep the scratch HOME and binaries for debugging")
    r.add_argument("--gates", action="store_true", help="also run tools/maintainer/verify_all.sh --slug (CI gates)")
    r.add_argument("--egress-guard", choices=("sandbox", "proxy-env"),
                   default="sandbox" if platform.system() == "Darwin" else "proxy-env")
    rp = sub.add_parser("report", help="latest verdict per slug, one row each")
    rp.add_argument("--json", action="store_true")
    sub.add_parser("self-test", help="feature 3's HTTP probe, the kit's TLS vendor and the WinRM stub against stand-ins, both ways")
    c = sub.add_parser("cleanup", help="stop what a run started and remove its scratch (never evidence)")
    g = c.add_mutually_exclusive_group(required=True)
    g.add_argument("--run", help="one run's evidence folder")
    g.add_argument("--stale", action="store_true", help="every run that left a process or scratch dir behind")
    args = ap.parse_args()
    if args.cmd == "self-test":
        return self_test()
    if args.cmd == "doctor":
        return cmd_doctor(args)
    if args.cmd == "report":
        return cmd_report(args)
    if args.cmd == "cleanup":
        return cmd_cleanup(args)
    features = parse_features(args.features)
    if args.all:
        slugs = [s for s in registry.skills() if not registry.is_markdown_only(s)
                 and (registry.skill_path(s) / "cli").is_dir()]
    else:
        slugs = args.slug
    if not slugs:
        ap.error("run needs --slug S (repeatable) or --all")
    worst = EXIT_OK
    for slug in slugs:
        print(f"==> {slug}", flush=True)
        worst = max(worst, run_slug(slug, features, args))
    return worst


if __name__ == "__main__":
    sys.exit(main())
