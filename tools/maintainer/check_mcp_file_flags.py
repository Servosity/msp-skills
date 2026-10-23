#!/usr/bin/env python3
"""Prove an MCP tool call cannot name a file or directory on the MCP host.

The trust model
---------------
Each connector ships a CLI and an MCP server. The MCP server mirrors CLI
commands as tools and forwards their flags to the companion CLI, which runs as
the account that hosts the server. The operator at the command line is
trusted; an MCP caller is not (it may be an agent following instructions it
read in a ticket, an email or a web page). So a flag whose value names a local
file or directory the CLI reads or writes must be refused when it arrives over
MCP, while the CLI keeps it.

Before this check that rule was enforced flag by flag: PR #416 (teach file
flags in five connectors), PR #423 (seven flags in five more), and a probe that
found `import --input` in five more, riverside-fm's --db and --out, and
superops' `--vars @path`. Every instance was found by reading usage text.

What this checks, per connector
-------------------------------
1. DERIVE the path flags from the code, not from names. The Go analyzer in
   tools/maintainer/mcp_file_flags/ runs a taint analysis over the connector's
   SSA form and reports every flag whose value can reach an open, read, write,
   stat, mkdir, directory walk or exec, including an "@path" value expansion.
2. Build the connector's real MCP server, with a recording stand-in for the
   companion CLI next to it, and the real command tree (through `go build
   -overlay`, so nothing is written into the connector).
3. For every MCP tool that mirrors a command, and every path flag that command
   carries (its own or inherited):
     a. the flag is ABSENT from the tool's input schema, and
     b. a tools/call that supplies it anyway gets an explicit refusal - an error
        result naming the flag - and the companion CLI never sees the value.
        A silent drop fails: the caller would get an answer computed without
        the input it asked for (crowdstrike `fleet tenants --expected` returned
        the roster without the comparison; gradient `usage push --file` said
        "--file is required").
   And one ordinary (non-path) flag on the same tool still reaches the CLI,
   so the block is not a blanket one.

Usage:
    python3 tools/maintainer/check_mcp_file_flags.py --slug riverside-fm
    python3 tools/maintainer/check_mcp_file_flags.py --all
    python3 tools/maintainer/check_mcp_file_flags.py --slug fortinet --json report.json
"""

from __future__ import annotations

import argparse
import json
import os
import re
import select
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
SKILLS = REPO / "skills"
ANALYZER = HERE / "mcp_file_flags"
DUMP_TMPL = ANALYZER / "dump" / "main.go.tmpl"
DUMP_PKG = "internal/zz_check_mcp_file_flags_dump"

# The MCP argument that carries positional arguments on mirrored tools. It is
# never a flag.
POSITIONAL_FIELD = "args"

# Connectors this check deliberately does not drive, each with the reason.
# Discovery is fail-closed: an unknown slug, a connector without a cli/ tree or
# an MCP command directory, a companion CLI name that cannot be read, or an MCP
# server that mirrors no command at all FAILS unless the slug is listed here.
# A markdown-only skill (registry `markdown_only`) has no CLI and is reported
# as not-a-connector rather than failed.
EXEMPT: dict[str, str] = {}

# Ordinary-flag probe preferences: flags whose forwarding is harmless with the
# recording stand-in and that nearly every mirrored command carries.
PREFERRED_ORDINARY = ("select", "limit", "max-pages", "since", "status", "type", "name")


def go_env() -> dict:
    env = dict(os.environ)
    env.setdefault("GOFLAGS", "-mod=readonly")
    return env


def module_path(cli_dir: Path) -> str:
    for line in (cli_dir / "go.mod").read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.split(None, 1)[1].strip()
    raise ValueError(f"no module line in {cli_dir / 'go.mod'}")


def mcp_cmd_dir(cli_dir: Path) -> str | None:
    cmd = cli_dir / "cmd"
    if not cmd.is_dir():
        return None
    for name in sorted(os.listdir(cmd)):
        if name.endswith("-mcp") and (cmd / name).is_dir():
            return name
    return None


def companion_cli_name(cli_dir: Path) -> str | None:
    """The file name the MCP server looks for next to itself."""
    src = cli_dir / "internal" / "mcp" / "cobratree" / "cli_path.go"
    if not src.is_file():
        return None
    m = re.search(r'name\s*:?=\s*"([^"]+)"', src.read_text(encoding="utf-8"))
    return m.group(1) if m else None


def build_analyzer(tmp: Path, timeout: int) -> Path:
    exe = tmp / ("mcp_file_flags.exe" if os.name == "nt" else "mcp_file_flags")
    run = subprocess.run(["go", "build", "-o", str(exe), "."], cwd=ANALYZER,
                         capture_output=True, text=True, timeout=timeout, env=go_env())
    if run.returncode != 0:
        raise RuntimeError("could not build the analyzer: " + run.stderr.strip()[-800:])
    return exe


def analyze(analyzer: Path, cli_dir: Path, timeout: int) -> dict:
    run = subprocess.run([str(analyzer), "-dir", str(cli_dir)], capture_output=True,
                         text=True, timeout=timeout, env=go_env())
    try:
        report = json.loads(run.stdout)
    except json.JSONDecodeError:
        raise RuntimeError(f"analyzer exited {run.returncode}: {run.stderr.strip()[-800:]}")
    if report.get("errors"):
        raise RuntimeError("the connector did not type-check: " + "; ".join(report["errors"][:3]))
    if not report.get("converged"):
        raise RuntimeError(f"the analysis did not converge in {report.get('rounds')} rounds")
    return report


def dump_tree(cli_dir: Path, tmp: Path, timeout: int) -> list[dict]:
    src = tmp / "dump_main.go"
    src.write_text(DUMP_TMPL.read_text(encoding="utf-8").replace(
        "__IMPORT__", module_path(cli_dir) + "/internal/cli"), encoding="utf-8")
    overlay = tmp / "overlay.json"
    overlay.write_text(json.dumps({"Replace": {str((cli_dir / DUMP_PKG / "main.go").resolve()): str(src)}}),
                       encoding="utf-8")
    exe = tmp / "treedump"
    build = subprocess.run(["go", "build", "-overlay", str(overlay), "-o", str(exe), "./" + DUMP_PKG],
                           cwd=cli_dir, capture_output=True, text=True, timeout=timeout, env=go_env())
    if build.returncode != 0:
        raise RuntimeError("could not compile the command-tree dump: " + build.stderr.strip()[-800:])
    home = tmp / "dump-home"
    home.mkdir(exist_ok=True)
    run = subprocess.run([str(exe)], capture_output=True, text=True, timeout=timeout,
                         env={"PATH": os.environ.get("PATH", ""), "HOME": str(home)})
    if run.returncode != 0:
        raise RuntimeError(f"command-tree dump exited {run.returncode}: {run.stderr.strip()[-600:]}")
    return json.loads(run.stdout)


def framework_commands(cli_dir: Path) -> set[str]:
    """The top-level command names cobratree never mirrors (classify.go)."""
    src = cli_dir / "internal" / "mcp" / "cobratree" / "classify.go"
    text = src.read_text(encoding="utf-8") if src.is_file() else ""
    m = re.search(r"var frameworkCommands = map\[string\]bool\{(.*?)\n\}", text, re.S)
    return set(re.findall(r'"([^"]+)":\s*true', m.group(1))) if m else set()


def truthy(annotations: dict | None, key: str) -> bool:
    return str((annotations or {}).get(key, "")).strip().lower() in ("true", "1", "yes")


def mirrored_nodes(tree: list[dict], framework: set[str]) -> dict[str, dict]:
    """tool name -> command, for a server whose tools carry no command-path
    metadata: the commands cobratree's RegisterAll turns into shell-out tools
    (visible, runnable, not an API endpoint, not a framework command), under
    the name toolNameForPath gives them. A later command with the same name
    replaces an earlier one, as AddTool does."""
    hidden_prefixes = set()
    out = {}
    for n in tree:
        key = " ".join(n["path"])
        parent = " ".join(n["path"][:-1])
        if parent in hidden_prefixes or n.get("hidden") or truthy(n.get("annotations"), "mcp:hidden"):
            hidden_prefixes.add(key)
            continue
        if len(n["path"]) == 1 and n["path"][0] in framework:
            hidden_prefixes.add(key)
            continue
        ann = n.get("annotations") or {}
        if not n["runnable"] or str(ann.get("pp:endpoint", "")).strip():
            continue
        out[tool_name_for_path(n["path"])] = n
    return out


def annotation_names(node: dict, key: str) -> set[str]:
    """Flag names listed in one of a command's annotations."""
    raw = str((node.get("annotations") or {}).get(key, ""))
    return {p.strip().lstrip("-") for p in re.split(r"[,;\s]+", raw) if p.strip().lstrip("-")}


def mirror_node(tool: dict, meta_paths: bool, gate_marked: bool, by_path: dict, by_tool: dict) -> dict | None:
    """The command a tool mirrors, or None for an in-process tool."""
    meta = tool.get("_meta") or {}
    if meta_paths:
        path = meta.get("pp:cli-command")
        return by_path.get(path) if isinstance(path, str) else None
    if gate_marked and meta.get("pp:tenant-gate") != "child-cli":
        return None
    return by_tool.get(tool["name"])


def tool_name_for_path(parts: list[str]) -> str:
    """cobratree's toolNameForPath (names.go), for servers that publish no
    command-path metadata on their tools."""
    out = []
    for part in parts:
        for ch in part:
            out.append(ch.lower() if ch.isalnum() else "_")
        out.append("_")
    return "_".join(p for p in "".join(out).split("_") if p)


FAKE_CLI = """#!/bin/sh
# Recording stand-in for the companion CLI: one argv element per line, then a
# separator, then an empty JSON object on stdout.
for a in "$@"; do printf '%s\\n' "$a"; done >> "{log}"
printf -- '----\\n' >> "{log}"
printf '{{}}\\n'
"""


class Server:
    """One MCP server over stdio."""

    def __init__(self, binary: Path, home: Path, timeout: int):
        env = {"PATH": os.environ.get("PATH", ""), "HOME": str(home)}
        self.proc = subprocess.Popen([str(binary)], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.DEVNULL, env=env, text=True, bufsize=1)
        self.timeout = timeout
        self.next_id = 1
        self.rpc("initialize", {"protocolVersion": "2024-11-05", "capabilities": {},
                                "clientInfo": {"name": "check_mcp_file_flags", "version": "1"}})
        self.notify("notifications/initialized")

    def notify(self, method: str) -> None:
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method}) + "\n")
        self.proc.stdin.flush()

    def rpc(self, method: str, params: dict) -> dict:
        self.next_id += 1
        rid = self.next_id
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0", "id": rid, "method": method, "params": params}) + "\n")
        self.proc.stdin.flush()
        deadline = time.time() + self.timeout
        while time.time() < deadline:
            ready, _, _ = select.select([self.proc.stdout], [], [], 1)
            if not ready:
                if self.proc.poll() is not None:
                    break
                continue
            line = self.proc.stdout.readline()
            if not line:
                break
            try:
                msg = json.loads(line)
            except json.JSONDecodeError:
                continue
            if msg.get("id") == rid:
                return msg
        raise RuntimeError(f"MCP server did not answer {method} (id {rid})")

    def close(self) -> None:
        try:
            self.proc.stdin.close()
        except OSError:
            pass
        try:
            self.proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            self.proc.kill()
            self.proc.wait()


def result_text(msg: dict) -> str:
    parts = []
    for block in (msg.get("result") or {}).get("content") or []:
        if isinstance(block, dict) and "text" in block:
            parts.append(str(block["text"]))
    err = msg.get("error")
    if isinstance(err, dict):
        parts.extend(str(err.get(k)) for k in ("message", "data") if err.get(k))
    return "\n".join(parts)


def is_error(msg: dict) -> bool:
    return isinstance(msg.get("error"), dict) or bool((msg.get("result") or {}).get("isError"))


def names_flag(text: str, flag: str) -> bool:
    return f"--{flag}" in text or f'"{flag}"' in text or f"'{flag}'" in text


def flag_matches(finding: dict, flag: dict) -> bool:
    if finding.get("name") != flag["name"]:
        return False
    if finding.get("usage") is not None and finding["usage"] != flag["usage"]:
        return False
    if finding.get("default") is not None and finding["default"] != flag["default"]:
        return False
    return True


def probe_value(flag: dict, n: int, canary_dir: Path) -> object:
    t = flag.get("type", "string")
    if t == "bool":
        return True
    if t in ("int", "int64", "int32", "uint", "uint64", "float64", "count"):
        return 7
    if t == "stringToString":
        return f"k={canary_dir}/canary-{n}"
    return f"{canary_dir}/canary-{n}"


def required_placeholders(schema: dict, skip: set[str]) -> dict:
    """Inert values for the schema's required parameters, so a probe reaches
    the argument handling it is aimed at instead of a missing-input error."""
    props = schema.get("properties") or {}
    args = {}
    for name in schema.get("required") or []:
        if name in skip or name == POSITIONAL_FIELD:
            continue
        kind = (props.get(name) or {}).get("type", "string")
        if kind in ("number", "integer"):
            args[name] = 1
        elif kind == "boolean":
            args[name] = False
        elif kind == "array":
            args[name] = ["mcp-probe"]
        elif kind == "object":
            args[name] = {}
        else:
            args[name] = "mcp-probe"
    return args


def ordinary_value(prop: dict, n: int) -> tuple[object, str]:
    kind = prop.get("type", "string")
    if kind == "boolean":
        return True, ""
    if kind in ("number", "integer"):
        return 3, "3"
    return f"mcp-ordinary-{n}", f"mcp-ordinary-{n}"


def check_slug(slug: str, analyzer: Path, timeout: int, verbose: bool) -> dict:
    """Return a per-connector report: status pass/fail/skip, failures, counts."""
    cli_dir = SKILLS / slug / "cli"
    rep = {"slug": slug, "status": "pass", "failures": [], "notes": []}
    # Path-flag names the pre-4.30 shell-out dropped by name on EVERY command
    # (and the newer gate blocks structurally). A connector-specific path flag
    # declared via mcp:write-flags/inline on one command (vars, devices,
    # expected, agreements, file, ...) is NOT in this set, so the old shell-out
    # forwarded it verbatim on a DIFFERENT command - the cross-command leak.
    GLOBAL_BLOCKED = {"config", "db", "deliver", "input", "output", "out", "o", "home",
                      "audit-dir", "receipt-file", "notes-file", "playbook-file",
                      "playbook-notes-file", "from-csv", "client-profile", "legacy-config",
                      "legacy-db", "base-url", "token", "profile", "client", "yes", "insecure"}
    def discovery_failure(why: str) -> dict:
        if slug in EXEMPT:
            rep["status"] = "exempt"
            rep["notes"].append(f"exempt: {EXEMPT[slug]} ({why})")
        else:
            rep["status"] = "fail"
            rep["failures"].append(f"{why}; fix the connector layout or add it to EXEMPT with a reason")
        return rep

    if not (SKILLS / slug).is_dir():
        return discovery_failure(f"unknown skill `{slug}`: no skills/{slug} directory")
    if is_markdown_only(slug):
        rep["status"] = "not-a-connector"
        rep["notes"].append("markdown-only skill (registry markdown_only): no CLI or MCP server to check")
        return rep
    if not (cli_dir / "go.mod").is_file():
        return discovery_failure(f"no skills/{slug}/cli/go.mod")
    mcp_dir = mcp_cmd_dir(cli_dir)
    if mcp_dir is None:
        return discovery_failure(f"no cmd/*-mcp directory under skills/{slug}/cli")
    cli_name = companion_cli_name(cli_dir)
    if cli_name is None:
        return discovery_failure("the companion CLI name cannot be read from internal/mcp/cobratree/cli_path.go")
    tmp = Path(tempfile.mkdtemp(prefix="mcp-file-flags-"))
    server = None
    try:
        analysis = analyze(analyzer, cli_dir, timeout)
        findings = analysis.get("flags") or []
        rep["analysis"] = {k: analysis.get(k) for k in ("functions", "sources", "rounds", "seconds")}
        rep["path_flags_in_code"] = sorted({f["name"] for f in findings if f.get("name")})
        # In-process tools: an MCP request value that reaches a filesystem
        # call inside the MCP server process itself (no CLI in between).
        for f in analysis.get("mcp_requests") or []:
            sink = f["sinks"][0]
            rep["failures"].append(
                f"in-process MCP handler {f.get('func')} passes tool-call data to {sink['sink']} at {sink['pos']}"
                + (f" (via {sink['via']})" if sink.get("via") else "")
                + "; an MCP caller can make the server process touch a path of its choosing")
        for f in analysis.get("other") or []:
            sink = f["sinks"][0]
            rep["failures"].append(
                f"{f['kind']} source at {f['pos']} reaches {sink['sink']} at {sink['pos']}; "
                "it cannot be tied to one MCP tool argument, so review it and refuse it explicitly")
        # Positional arguments that can start a path (reported, not gated: the
        # gate's contract is flags; see the PR for the follow-up).
        rep["positional_paths"] = sorted({f.get("use") or f.get("func") or "?"
                                          for f in analysis.get("positional") or [] if f.get("sinks")})
        has_positional_file = any(f.get("sinks") for f in analysis.get("positional") or [])
        # Values that only ever become a component inside a directory the CLI
        # chose (a client-profile name under the clients directory, a slug
        # inside a guarded join) do not choose the file's location; they are
        # reported for review, not refused.
        rep["path_component_flags"] = sorted({f["name"] for f in analysis.get("flag_components") or [] if f.get("name")})
        for f in findings:
            if f.get("name_unknown"):
                rep["failures"].append(
                    f"a flag registered at {f['pos']} reaches {f['sinks'][0]['sink']} at {f['sinks'][0]['pos']}, "
                    "but its name is not a constant, so no MCP assertion can be made for it; give it a literal name")

        tree = dump_tree(cli_dir, tmp, timeout)
        by_path = {" ".join(n["path"]): n for n in tree}
        by_tool = mirrored_nodes(tree, framework_commands(cli_dir))
        by_use = {}
        for n in tree:
            if n.get("use"):
                by_use.setdefault(n["use"], []).append(n)

        # A finding whose usage or default changed after registration matches
        # no live flag; fall back to its name so it is never dropped.
        all_flags = [f for n in tree for f in n["flags"]]
        for f in findings:
            if f.get("name") and not any(flag_matches(f, lf) for lf in all_flags):
                if any(lf["name"] == f["name"] for lf in all_flags):
                    f["usage"] = None
                    f["default"] = None
                    rep["notes"].append(f"--{f['name']} ({f['pos']}): usage/default differ at run time; matched by name")
                else:
                    rep["notes"].append(f"--{f['name']} ({f['pos']}) reaches a path but is not on any command in the tree")

        bindir = tmp / "bin"
        bindir.mkdir()
        mcp_bin = bindir / mcp_dir
        build = subprocess.run(["go", "build", "-o", str(mcp_bin), "./cmd/" + mcp_dir], cwd=cli_dir,
                               capture_output=True, text=True, timeout=timeout, env=go_env())
        if build.returncode != 0:
            raise RuntimeError("could not build the MCP server: " + build.stderr.strip()[-600:])
        log = tmp / "argv.log"
        fake = bindir / cli_name
        fake.write_text(FAKE_CLI.format(log=str(log)), encoding="utf-8")
        fake.chmod(0o755)
        home = tmp / "home"
        home.mkdir()
        canaries = tmp / "canaries"
        canaries.mkdir()

        server = Server(mcp_bin, home, timeout=60)
        tools = server.rpc("tools/list", {}).get("result", {}).get("tools") or []
        if not tools:
            raise RuntimeError("the MCP server listed no tools")

        # How a mirror tool names its command depends on the press version:
        # 4.30.3 and later put the command path in `pp:cli-command`; 4.27 to
        # 4.30.2 mark mirrors `pp:tenant-gate: child-cli` and keep an existing
        # tool when a mirror's name collides with it; older servers carry no
        # metadata and a mirror replaces an existing tool of the same name.
        meta_paths = any(isinstance((t.get("_meta") or {}).get("pp:cli-command"), str) for t in tools)
        gate_marked = any((t.get("_meta") or {}).get("pp:tenant-gate") == "child-cli" for t in tools)
        # The mapping must account for every mirror, or a tool the script could
        # not place would go unchecked.
        listed = {t["name"] for t in tools}
        for tool in tools:
            meta = tool.get("_meta") or {}
            path = meta.get("pp:cli-command")
            if meta_paths and isinstance(path, str) and path not in by_path:
                rep["failures"].append(f"{tool['name']}: mirrors `{path}`, which is not in the command tree; cannot check it")
            if not meta_paths and gate_marked and meta.get("pp:tenant-gate") == "child-cli" and tool["name"] not in by_tool:
                rep["failures"].append(f"{tool['name']}: a mirror tool this script cannot map to a command; cannot check it")
        if not meta_paths and not gate_marked:
            # Before press 4.27 a mirror replaces any tool of the same name, so
            # every mirrored command must appear in the listing.
            for name, n_ in sorted(by_tool.items()):
                if name not in listed:
                    rep["failures"].append(f"`{' '.join(n_['path'])}`: expected an MCP mirror named {name}, none is listed; "
                                           "the walker replica in this script is out of step with the server")
        mirrored = 0
        asserted = 0
        inline_checked = 0
        ordinary_ok = 0
        exposed = set()
        n = 0
        for tool in tools:
            node = mirror_node(tool, meta_paths, gate_marked, by_path, by_tool)
            if node is None:
                continue
            mirrored += 1
            schema = tool.get("inputSchema") or {}
            props = schema.get("properties") or {}
            path_flags = [lf for lf in node["flags"] if any(flag_matches(f, lf) for f in findings)]
            path_names = {lf["name"] for lf in path_flags}
            inline_only = annotation_names(node, "mcp:inline-only-flags")
            for lf in path_flags:
                exposed.add(lf["name"])
                asserted += 1
                where = f"{tool['name']} (`{' '.join(node['path'])}`) --{lf['name']}"
                if lf["name"] in inline_only:
                    # A flag that takes inline JSON or a file stays in the
                    # schema for its inline form; every other value must be
                    # refused by name before it reaches the CLI.
                    inline_checked += 1
                    for bad in (str(canaries / f"inline-{n}"), "@" + str(canaries / f"inline-at-{n}")):
                        n += 1
                        if log.exists():
                            log.unlink()
                        call_args = required_placeholders(schema, path_names)
                        call_args[lf["name"]] = bad
                        msg = server.rpc("tools/call", {"name": tool["name"], "arguments": call_args})
                        argv = log.read_text(encoding="utf-8") if log.exists() else ""
                        if str(canaries) in argv:
                            rep["failures"].append(f"{where}: a non-inline value reached the companion CLI (argv: {argv.splitlines()[:8]})")
                        elif not is_error(msg) or not names_flag(result_text(msg), lf["name"]):
                            rep["failures"].append(f"{where}: inline-only, but the value {bad!r} was not refused by name")
                    if log.exists():
                        log.unlink()
                    call_args = required_placeholders(schema, path_names)
                    call_args[lf["name"]] = '{"mcp_probe": 1}'
                    server.rpc("tools/call", {"name": tool["name"], "arguments": call_args})
                    argv = log.read_text(encoding="utf-8").splitlines() if log.exists() else []
                    if f'--{lf["name"]}={{"mcp_probe": 1}}' not in argv:
                        rep["failures"].append(f"{where}: inline-only, but an inline JSON object did not reach the CLI (argv {argv[:8]})")
                    continue
                if lf["name"] in props:
                    rep["failures"].append(f"{where}: still in the tool's input schema; a path flag must be removed from it")
                n += 1
                value = probe_value(lf, n, canaries)
                if log.exists():
                    log.unlink()
                call_args = required_placeholders(schema, path_names)
                call_args[lf["name"]] = value
                msg = server.rpc("tools/call", {"name": tool["name"], "arguments": call_args})
                argv = log.read_text(encoding="utf-8") if log.exists() else ""
                text = result_text(msg)
                if str(canaries) in argv:
                    rep["failures"].append(f"{where}: the value reached the companion CLI (argv: {argv.splitlines()[:8]})")
                elif not is_error(msg):
                    rep["failures"].append(
                        f"{where}: supplied over MCP it was dropped silently and the command ran; "
                        "it must be refused with an error that names the flag")
                elif not names_flag(text, lf["name"]):
                    rep["failures"].append(f"{where}: refused, but the error does not name the flag: {' '.join(text.split())[:160]}")
            # One ordinary flag must still reach the CLI. Only a real flag of the
            # command qualifies; a schema property can also be a positional.
            flag_names = {lf["name"] for lf in node["flags"]}
            candidates = [k for k, v in props.items()
                          if k != POSITIONAL_FIELD and k in flag_names and k not in path_names
                          and isinstance(v, dict) and v.get("type") in ("string", "boolean", "number", "integer")]
            if not candidates:
                continue
            local = {lf["name"] for lf in node["flags"] if lf["local"]}
            candidates.sort(key=lambda k: (k not in PREFERRED_ORDINARY, k not in local,
                                           props[k].get("type") != "string", k))
            pick = candidates[0]
            n += 1
            value, expect = ordinary_value(props[pick], n)
            if log.exists():
                log.unlink()
            call_args = required_placeholders(schema, path_names | {pick})
            call_args[pick] = value
            msg = server.rpc("tools/call", {"name": tool["name"], "arguments": call_args})
            argv = log.read_text(encoding="utf-8").splitlines() if log.exists() else []
            want = f"--{pick}={expect}" if expect else f"--{pick}"
            if want in argv:
                ordinary_ok += 1
            else:
                rep["failures"].append(
                    f"{tool['name']} --{pick}: an ordinary flag did not reach the CLI "
                    f"(argv {argv[:8]}, result {' '.join(result_text(msg).split())[:120]}); the block must stay specific")
        # Same-named flags that are NOT path flags on another mirrored command:
        # a name-wide block there would cost a real MCP input.
        collisions = {}
        for tool in tools:
            node = mirror_node(tool, meta_paths, gate_marked, by_path, by_tool)
            if node is None:
                continue
            for lf in node["flags"]:
                if lf["name"] in exposed and not any(flag_matches(f, lf) for f in findings):
                    collisions.setdefault(lf["name"], []).append(" ".join(node["path"]))
        rep["same_name_non_path_flags"] = {k: sorted(v) for k, v in sorted(collisions.items())}
        rep["mirrored_tools"] = mirrored
        rep["path_flag_assertions"] = asserted
        rep["inline_only_assertions"] = inline_checked
        rep["ordinary_flag_proofs"] = ordinary_ok
        # Cross-command probe (both directions): a connector-specific path-flag
        # name, sent to a tool that does NOT declare it, must not reach the CLI.
        # The pre-4.30 shell-out forwarded any key it was handed, so on a parent
        # tool whose args field selects a path-reading subcommand this was a
        # read primitive (superops `raw` + --vars @path). Refused by name now.
        foreign = sorted({nm for nm in rep["path_flags_in_code"] if nm not in GLOBAL_BLOCKED})
        cross_checked = 0
        if foreign:
            for tool in tools:
                node = mirror_node(tool, meta_paths, gate_marked, by_path, by_tool)
                if node is None:
                    continue
                declared = {lf["name"] for lf in node["flags"]}
                for nm in foreign:
                    if nm in declared:
                        continue
                    cross_checked += 1
                    n += 1
                    if log.exists():
                        log.unlink()
                    msg = server.rpc("tools/call", {"name": tool["name"], "arguments": {nm: str(canaries / f"cross-{n}")}})
                    argv = log.read_text(encoding="utf-8") if log.exists() else ""
                    if str(canaries) in argv:
                        rep["failures"].append(
                            f"{tool['name']} (`{' '.join(node['path'])}`): the undeclared path flag --{nm} was forwarded to the CLI "
                            f"(argv {argv.splitlines()[:8]}); a key the tool does not declare must be refused, not forwarded to a command that reads it")
        rep["cross_command_assertions"] = cross_checked
        # A positional argument that reads a local file (superops `raw query
        # <@file>`): over MCP it arrives through the tool's `args` field. For
        # every mirrored tool whose command the analyzer flagged, an @path (and
        # a plain path) positional must be refused, not forwarded. A command the
        # analyzer flags but that is NOT mirrored (wordpress `media upload`, an
        # endpoint whose upload is not exposed over MCP) is unreachable and not
        # probed - recorded as such.
        # The analyzer flags a command whose POSITIONAL argument reads a local
        # file. Such a positional arrives over MCP through the tool's `args`
        # field (or a structured positional). A command is one of these when its
        # Use string shows a file positional (<file>, @file, @path, <path>);
        # superops `raw query <graphql|@file|->` is the reachable case. For a
        # mirrored one, an @path and a plain path positional must be refused;
        # one that is not mirrored (wordpress `media upload <file>`, an endpoint
        # whose upload is not exposed over MCP) is unreachable and recorded.
        pos_checked = 0
        if has_positional_file:
            file_pos_re = re.compile(r"<[^>]*(file|path)[^>]*>|@(file|path)", re.I)
            for node in tree:
                if not node.get("use") or not file_pos_re.search(node["use"]):
                    continue
                tool = next((t for t in tools
                             if mirror_node(t, meta_paths, gate_marked, by_path, by_tool) is node), None)
                if tool is None:
                    rep["notes"].append(f"positional file argument on `{node['use']}` "
                                        f"(`{' '.join(node['path'])}`) is not reachable over MCP (command not mirrored); not gated")
                    continue
                # An @path positional is the file-read convention (superops
                # resolveDocArg reads `@file`). A bare positional is the inline
                # document, not a file, so only the @path form is asserted.
                n += 1
                val = "@" + str(canaries / f"pos-{n}")
                if log.exists():
                    log.unlink()
                server.rpc("tools/call", {"name": tool["name"], "arguments": {POSITIONAL_FIELD: val}})
                argv = log.read_text(encoding="utf-8") if log.exists() else ""
                if str(canaries) in argv:
                    rep["failures"].append(
                        f"{tool['name']} (`{' '.join(node['path'])}`): an @path positional naming a local file "
                        f"was forwarded to the CLI (argv {argv.splitlines()[:6]}); it must be refused over MCP")
                pos_checked += 1
            # The twin proof: the refusal must be specific. A mirrored command
            # whose positional is NOT a file still forwards an @-leading value
            # (superops `client-360 @Acme` is a client name).
            for tool in tools:
                node = mirror_node(tool, meta_paths, gate_marked, by_path, by_tool)
                schema = tool.get("inputSchema") or {}
                if (node is None or POSITIONAL_FIELD not in (schema.get("properties") or {})
                        or file_pos_re.search(node.get("use") or "")):
                    continue
                n += 1
                val = f"@ordinary-name-{n}"
                if log.exists():
                    log.unlink()
                call_args = required_placeholders(schema, set())
                call_args[POSITIONAL_FIELD] = val
                server.rpc("tools/call", {"name": tool["name"], "arguments": call_args})
                argv = log.read_text(encoding="utf-8") if log.exists() else ""
                if val not in argv:
                    rep["failures"].append(
                        f"{tool['name']} (`{' '.join(node['path'])}`): an ordinary @-leading positional was not forwarded; "
                        f"the @path refusal must apply only to commands that read @file")
                pos_checked += 1
                break
        rep["positional_assertions"] = pos_checked
        rep["path_flags_on_mcp_tools"] = sorted(exposed)
        if mirrored and not ordinary_ok:
            rep["failures"].append("no mirrored tool proved an ordinary flag still reaches the CLI")
        if mirrored == 0:
            if slug in EXEMPT:
                rep["status"] = "exempt"
                rep["notes"].append(f"exempt: {EXEMPT[slug]} (no MCP tool mirrors a CLI command)")
            else:
                rep["failures"].append("no MCP tool could be mapped to a CLI command, so nothing was checked; "
                                       "fix the mapping or add the connector to EXEMPT with a reason")
    except (RuntimeError, OSError, subprocess.TimeoutExpired, ValueError, json.JSONDecodeError) as exc:
        rep["failures"].append(f"check could not run: {exc}")
    finally:
        if server is not None:
            server.close()
        shutil.rmtree(tmp, ignore_errors=True)
    if rep["failures"]:
        rep["status"] = "fail"
    return rep


def is_markdown_only(slug: str) -> bool:
    sys.path.insert(0, str(HERE))
    try:
        import registry  # noqa: E402  (local tools/ module)
        return slug in registry.skills() and registry.is_markdown_only(slug)
    except SystemExit:
        return False


def connector_slugs() -> list[str]:
    """Every registered non-markdown connector plus every on-disk cli/go.mod, so
    a registered connector whose cli/ tree is missing still reaches the
    fail-closed discovery checks instead of silently dropping out of --all."""
    sys.path.insert(0, str(HERE))
    import registry  # noqa: E402  (local tools/ module)
    reg = {registry.source_dir(s) for s in registry.skills() if not registry.is_markdown_only(s)}
    disk = {p.parent.parent.name for p in SKILLS.glob("*/cli/go.mod")}
    return sorted(reg | disk)


# Component-only flags (client-profile, and td-synnex's param / global-param /
# path-context, which all resolve a tenant profile) never choose a whole path:
# their value becomes one component under a directory the CLI picked. The gate
# does not refuse them, so this one check proves the connector-side name
# validation is real and uniform, instead of editing 31 connectors. The
# canonical validator is ValidateProfileName; every connector that defines it
# must define it byte-for-byte identically, and its pattern must reject a path
# separator, a parent-dir hop, an absolute path and a NUL while accepting an
# ordinary name.
import re as _re

# The known-correct implementation. Uniformity alone is not proof (31 identical
# `return nil` bodies would be uniform), so every connector's body and pattern
# must equal these exactly; the hostile-value test below then exercises the
# pinned pattern + '..' guard this body is known to apply.
EXPECTED_PROFILE_VALIDATOR = (
    "func ValidateProfileName(name string) error {\n"
    "\tif !profileNamePattern.MatchString(name) || strings.Contains(name, \"..\") {\n"
    "\t\treturn fmt.Errorf(\"invalid client profile name %q\", name)\n"
    "\t}\n"
    "\treturn nil\n"
    "}"
)
EXPECTED_PROFILE_PATTERN = "^[a-z0-9][a-z0-9._-]{0,62}$"

HOSTILE_COMPONENTS = ["a/b", "a\\b", "..", "../x", "/abs", "x\x00y", "a b", ""]
BENIGN_COMPONENT = "acme-corp.1"


def _profile_validator_pattern(text: str) -> str | None:
    m = _re.search(r'profileNamePattern\s*=\s*regexp\.MustCompile\(`([^`]*)`\)', text)
    return m.group(1) if m else None


def check_component_validators() -> int:
    """Prove the shared profile-name validator rejects hostile component values,
    in one place, for every connector that uses it."""
    defs = {}
    for prof in sorted(SKILLS.glob("*/cli/internal/platform/profile.go")):
        slug = prof.parents[3].name  # skills/<slug>/cli/internal/platform/profile.go
        text = prof.read_text(encoding="utf-8")
        if "func ValidateProfileName(" not in text:
            continue
        body = text[text.index("func ValidateProfileName("):]
        end = body.find("\n}\n")
        if end < 0:
            print(f"FAIL: cannot bound ValidateProfileName in {slug}")
            return 1
        defs[slug] = (body[:end + 2], _profile_validator_pattern(text))
    if not defs:
        print("FAIL: no connector defines ValidateProfileName; the component-flag safety claim is unproven.")
        return 1
    bodies = {b for b, _ in defs.values()}
    patterns = {p for _, p in defs.values()}
    failures = []
    for slug, (b, pp) in sorted(defs.items()):
        if b.rstrip() != EXPECTED_PROFILE_VALIDATOR:
            failures.append(f"{slug}: ValidateProfileName differs from the known-correct implementation")
        if pp != EXPECTED_PROFILE_PATTERN:
            failures.append(f"{slug}: profileNamePattern is {pp!r}, want {EXPECTED_PROFILE_PATTERN!r}")
    if len(bodies) != 1:
        failures.append(f"ValidateProfileName is not uniform across {len(defs)} connectors: {len(bodies)} distinct definitions")
    if len(patterns) != 1:
        byp = {}
        for slug, (_, pp) in defs.items():
            byp.setdefault(pp, []).append(slug)
        failures.append("profileNamePattern is not uniform: " + "; ".join(f"{pp!r} in {len(v)}" for pp, v in byp.items()))
    pat = next(iter(defs.values()))[1]
    if not pat:
        failures.append("could not read profileNamePattern from the validator")
    elif len(patterns) == 1:
        rx = _re.compile(pat)
        # The validator is the pattern AND a parent-dir ('..') check; model both.
        def rejected(v: str) -> bool:
            return (rx.fullmatch(v) is None) or (".." in v)
        for v in HOSTILE_COMPONENTS:
            if not rejected(v):
                failures.append(f"validator accepts hostile component value {v!r}")
        if rejected(BENIGN_COMPONENT):
            failures.append(f"validator rejects the benign name {BENIGN_COMPONENT!r} (too strict)")
    if failures:
        print("FAIL: component-flag validation:")
        for f in failures:
            print("  - " + f)
        return 1
    print(f"PASS: {len(defs)} connectors share one ValidateProfileName; pattern {pat!r} + '..' check "
          f"rejects {len(HOSTILE_COMPONENTS)} hostile component values and accepts a normal name.")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--slug", action="append", help="check one connector (repeatable)")
    parser.add_argument("--all", action="store_true", help="check every connector")
    parser.add_argument("--check-component-validators", action="store_true",
                        help="prove the shared profile-name validator rejects hostile component values")
    parser.add_argument("--timeout", type=int, default=900, help="per-step timeout in seconds")
    parser.add_argument("--json", help="write the per-connector report here")
    parser.add_argument("--jobs", type=int, default=1, help="connectors checked in parallel (local fleet runs)")
    parser.add_argument("-v", "--verbose", action="store_true")
    args = parser.parse_args()
    if getattr(args, "check_component_validators", False):
        return check_component_validators()
    if not args.slug and not args.all:
        parser.error("pass --slug <slug>, --all, or --check-component-validators")
    if shutil.which("go") is None:
        print("FAIL: go is not on PATH; the check cannot build anything.")
        return 1
    slugs = connector_slugs() if args.all else args.slug
    tmp = Path(tempfile.mkdtemp(prefix="mcp-file-flags-analyzer-"))
    try:
        analyzer = build_analyzer(tmp, args.timeout)
        reports = []
        from concurrent.futures import ThreadPoolExecutor
        with ThreadPoolExecutor(max_workers=max(1, args.jobs)) as pool:
            results = pool.map(lambda slug: check_slug(slug, analyzer, args.timeout, args.verbose), slugs)
            for r in results:
                reports.append(r)
                slug = r["slug"]
                line = (f"{r['status'].upper():4} {slug}: {r.get('mirrored_tools', 0)} mirrored tools, "
                        f"{r.get('path_flag_assertions', 0)} path-flag assertions over "
                        f"{len(r.get('path_flags_on_mcp_tools', []))} flag names, "
                        f"{r.get('ordinary_flag_proofs', 0)} ordinary-flag proofs")
                print(line, flush=True)
                for f in r["failures"][:40]:
                    print("    - " + f)
                if len(r["failures"]) > 40:
                    print(f"    ... and {len(r['failures']) - 40} more")
                if args.verbose:
                    for note in r["notes"]:
                        print("    note: " + note)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
    if args.json:
        Path(args.json).write_text(json.dumps(reports, indent=1) + "\n", encoding="utf-8")
    failed = [r["slug"] for r in reports if r["status"] == "fail"]
    checked = sum(1 for r in reports if r["status"] in ("pass", "fail"))
    exempt = [r["slug"] for r in reports if r["status"] == "exempt"]
    not_connectors = [r["slug"] for r in reports if r["status"] == "not-a-connector"]
    counts = f"{checked} checked, {len(exempt)} exempt, {len(not_connectors)} not a connector"
    if failed:
        print(f"\nFAIL: {len(failed)} connector(s) failed ({counts}): {' '.join(failed)}")
        return 1
    if checked == 0:
        print(f"\nFAIL: nothing was checked ({counts}).")
        return 1
    print(f"\nPASS: no MCP tool call can name a local file or directory ({counts}).")
    if exempt:
        print("  exempt: " + ", ".join(f"{s} ({EXEMPT.get(s, '?')})" for s in exempt))
    return 0


if __name__ == "__main__":
    sys.exit(main())
