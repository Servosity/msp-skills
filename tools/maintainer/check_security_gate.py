#!/usr/bin/env python3
"""check_security_gate.py - deterministic, release-blocking security gate.

The load-bearing control against a poisoned fix reaching downstream MSPs (threat T2).
Model-layer defenses (prompt doctrine, the nonce) are ~22%-breakable; this gate is
deterministic and meant to run as a REQUIRED CI check so it can't be bypassed by a
socially-engineered agent.

Always-on deterministic checks (no external tools needed):
  * Dependency policy - any module in go.mod (require OR `replace` target) not in
    tools/maintainer/dep_allowlist.json is P1; a filesystem `replace` is P1
    (anti-typosquat / anti-backdoor / anti-fork-swap). go.sum changes are surfaced.
  * Dangerous Go patterns - disabled TLS verify; shelling out / exec of a variable or
    shell; syscall.Exec/os.StartProcess/plugin.Open; cgo; //go:generate; //go:linkname;
    import "unsafe"; line directives (//line, /*line), which rewrite the positions gosec
    reports. (cobratree MCP→CLI bridge excluded; browser-open literal allowed.)
  * Install-script patterns - pipe-to-shell, eval, base64-decode-then-exec, direct shell
    invocation (cmd /c, & pwsh -Command, mshta/wscript...), non-GitHub download host in
    skills/<slug>/install.sh|.ps1 (the artifact MSPs run via curl|sh). NOTE: an installer
    is executable by nature, so this flags KNOWN-BAD remote/shell-exec forms - it is not a
    complete sandbox. CODEOWNERS review of install-script changes is the human backstop.
  * Shipped-script patterns - dangerous constructs in the .py/.sh/.ps1 a skill ships.
    Python is checked through the AST (eval/exec, os.system, pickle.load, unsafe
    yaml.load, shell=True, subprocess given a command STRING instead of a list argv),
    not a regex: skills legitimately DISCUSS these names in docstrings and references,
    and failing an honest script for describing good practice is false-RED. Shell and
    PowerShell get SCRIPT_RULES with whole-line comments stripped first, so a script
    documenting its own `curl ... | bash` install line is not flagged for saying so.
  * MCP-surface rules - seven STATE rules on a connector's vendored MCP server (cmd/*-mcp,
    internal/mcp, go.mod), from the 2026-10-22 template security pass: StreamableHTTP with
    no requireBearerAuth; mark3labs/mcp-go below v0.57.0 (no DNS-rebinding guard); an
    _execute tool with no confirm gate; POST/PUT/PATCH/DELETE endpoint tools labeled
    destructiveHint=false; GET registry entries named with an action verb and not marked
    NeedsConfirm/Mutating; a `share` command; typed endpoint tools with no read-only drop.
    They judge the whole connector, in diff mode too. A connector that failed a rule when
    it landed is listed in security_gate_baseline.json and reports P2 until its reprint
    burns it down; the list only shrinks (a slug added back is P1).

TWO LANES. A skill WITH skills/<slug>/cli/go.mod gets the Go lane (dependency policy +
gosec/govulncheck/osv-scanner/semgrep). A markdown-only skill has no Go module, so the
Go lane is skipped and the shipped-script scan is its coverage. Running the Go scanners
against a non-existent go.mod used to fail them CLOSED - 4 permanent P1s on a file that
does not exist, unfixable by any contributor - while the Python that actually ships went
completely unread. That was false-RED and false-GREEN in the same verdict.

Suppressions are BASE-OWNED, not self-grantable: a finding is suppressed only if its
(file, rule) is listed in tools/maintainer/security_suppressions.json, read from the
trusted base. No inline annotation suppresses anything by itself. The builtin rules
ignore `// #nosec`, and gosec runs with -show-ignored, so it reports every finding an
inline `#nosec` or `//gosec:disable` would have hidden (the same finding set as -nosec,
measured on all 84 connectors) and marks those findings, which is how this gate tells
them apart. An annotation-hidden finding is then accepted only by:
  * a base-owned suppression entry, like any other finding, or
  * FLEET PRECEDENT: the annotation sits on the finding's own lines or in the comment
    block directly above them, and the whole top-level declaration holding the finding
    (doc comment included, as Go's own parser delimits it: DECL_SPAN_GO, a stdlib-only
    helper embedded in this file) is byte-identical at the same path
    under cli/ in at least PRECEDENT_MIN other connectors, read from the same trusted
    base as the suppressions, each holding at least as many copies of it in that file
    as this connector does. This keeps today's behavior for unchanged press-template
    output (eleven annotated template lines, in 17-31 connectors each, inside functions
    that are identical across the fleet), so a new connector costs no suppression entry
    for them.
What this closes: an annotation added or edited in hand-written or hand-edited code,
a template line pasted or moved somewhere new, and an edit anywhere inside the
declaration that holds a precedented line, no longer hide a finding. If the helper
cannot run (no Go toolchain), there is no precedent at all (fail closed).
What it does not re-prove (the residual): a precedented declaration is trusted as text,
so what its callers pass into it, and the package-level names and imports it uses, are
not re-proven by this rule; the dangerous-pattern, dependency and reachability checks
still apply to that code.

Folded in when installed (CI installs them): gosec, govulncheck, osv-scanner, semgrep.
  * gosec runs once per build target in GOSEC_TARGETS: the runner's own OS (Linux in CI
    and release), GOOS=windows and GOOS=darwin. go/packages loads only the files one
    target builds, so before these passes no workflow ever scanned a `*_windows.go` or
    `*_darwin.go` file. Each pass fails closed on its own; a finding only an extra pass
    reports says which target.
  * govulncheck = the REACHABILITY gate. Run in DEFAULT text/symbol mode: exit 3 = a
    vulnerable symbol is CALLED from this module (P1); 0 = none called. (-format json
    ALWAYS exits 0 regardless of findings, which silently neutered the prior gate.)
  * osv-scanner = PRESENCE-only (no reachability). A bare advisory is a tracked P2; it
    only GATES (P1) when the PR introduced/bumped that module@version, the module is a
    direct first-party import (import-graph, not the `// indirect` comment), or - at
    release only - a GHSA-only high/critical advisory govulncheck's DB cannot reason about.
  * --require-scanners (CI/release): a missing/erroring required scanner is a P1, never a
    silent skip; unresolved osv->module mapping and `go list` failures also fail CLOSED.

Usage:
    check_security_gate.py --slug ninjaone --base origin/main   # gate the diff
    check_security_gate.py --slug ninjaone --base origin/main --require-scanners  # CI
    check_security_gate.py --all                                 # audit every connector
    check_security_gate.py --rebuild-allowlist                   # seed dep_allowlist.json
    check_security_gate.py --slug ninjaone --json
    check_security_gate.py --self-test                           # summary + workflows, both ways
    check_security_gate.py --mcp-surface --all --report          # MCP-surface rules only, fleet counts
    check_security_gate.py --update-baseline                     # retire burned-down baseline entries
    check_security_gate.py --seed-baseline                       # first entry for a NEW rule only
    check_security_gate.py --summarize --verdicts DIR --sha S --run-id N  # workflow summary job

Exit code: 0 = pass (no P1), 1 = block (>=1 P1), 2 = usage error.
"""
from __future__ import annotations

import argparse
import ast
import atexit
import contextlib
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
ALLOWLIST = Path(__file__).resolve().parent / "dep_allowlist.json"
SUPPRESSIONS = Path(__file__).resolve().parent / "security_suppressions.json"
BASELINE = Path(__file__).resolve().parent / "security_gate_baseline.json"

BT = "`"  # Go raw-string delimiter; included so backtick evasions are caught.

# --- dangerous Go patterns -----------------------------------------------------
# (id, severity, regex, exclude_path_substr). Calibrated to PASS the healthy fleet
# and BLOCK genuinely dangerous code (false-RED is as useless as false-GREEN).
# Legit press sites NOT flagged: OAuth browser-open (exec.Command("open"/... , url) -
# literal command) and the generated MCP→CLI bridge (internal/mcp/cobratree/).
GO_RULES = [
    ("disabled-tls-verify", "P1",
     re.compile(r"InsecureSkipVerify\s*:\s*true"), None),
    # exec of an interpreter (POSIX or Windows), or passing -c / /C - command injection.
    ("shell-interpreter", "P1",
     re.compile(r'exec\.Command(?:Context)?\([^)]*(["' + BT + r'](?:/bin/|/usr/bin/)?(?:sh|bash|zsh|dash|ash|pwsh(?:\.exe)?|powershell(?:\.exe)?|cmd(?:\.exe)?)["' + BT + r']|,\s*["' + BT + r'](?:-c|/[cC])["' + BT + r'])'),
     None),
    # exec with a NON-literal command (a variable) - the injectable shape.
    ("exec-nonliteral-cmd", "P1",
     re.compile(r'(?:exec\.Command\(\s*[A-Za-z_]|exec\.CommandContext\(\s*[A-Za-z_]\w*\s*,\s*[A-Za-z_])'),
     "/internal/mcp/cobratree/"),
    # exec with a concatenated command (first arg builds a string) - also injectable.
    ("exec-concat-cmd", "P1",
     re.compile(r'(?:exec\.Command\(\s*[^,)]*\+|exec\.CommandContext\([^,]*,\s*[^,)]*\+)'),
     "/internal/mcp/cobratree/"),
    ("syscall-exec", "P1", re.compile(r'\bsyscall\.(?:Exec|ForkExec)\b'), None),
    ("os-startprocess", "P1", re.compile(r'\bos\.StartProcess\b'), None),
    ("plugin-open", "P1", re.compile(r'\bplugin\.Open\b'), None),
    ("cgo-import", "P1", re.compile(r'(?m)^\s*(?:import\s+)?"C"\s*$'), None),
    ("go-generate", "P1", re.compile(r'//go:generate\b'), None),
    ("go-linkname", "P1", re.compile(r'//go:linkname\b'), None),
    ("unsafe-import", "P1", re.compile(r'(?m)^\s*(?:import\s+)?"unsafe"\s*$'), None),
    # A line directive (`//line f.go:N` at column 1, or `/*line f.go:N*/`) rewrites the
    # file and line every Go tool reports, gosec included. It would let new code borrow
    # the position of a suppressed file or of a precedented template line, so neither
    # suppressions nor fleet precedent could be trusted. The fleet has none, so the rule
    # refuses the text in any column, wider than Go's own column-1 rule (fail closed).
    ("line-directive", "P1", re.compile(r'//line\s|/\*line\s'), None),
    ("plaintext-http", "P2",
     re.compile(r'"http://(?!localhost|127\.0\.0\.1|0\.0\.0\.1|example\.)'), None),
]

# --- install-script patterns (the artifact downstream MSPs execute via curl|sh / iex) ---
# Case-insensitive: covers POSIX shell AND PowerShell (.ps1) RCE forms.
INSTALL_RULES = [
    ("pipe-to-shell", "P1", re.compile(r'\|\s*(?:sudo\s+)?(?:ba|z|da|a)?sh\b', re.I)),
    ("eval-in-installer", "P1", re.compile(r'(?<![\w-])eval\s', re.I)),
    ("decode-then-run", "P1", re.compile(r'\bbase64\s+(?:-d|--decode|-D)\b|\bfrombase64string\b', re.I)),
    # PowerShell EXECUTION sinks (the dangerous part). Note: Invoke-RestMethod / irm /
    # Invoke-WebRequest to a variable is a benign download (the legit installers query
    # the GitHub releases API that way) - only flag execution: IEX, DownloadString,
    # encoded commands, spawning a shell, or a download PIPED into IEX.
    ("powershell-rce", "P1",
     re.compile(r'\b(?:iex|invoke-expression)\b|\bdownloadstring\b|-e(?:nc\b|ncodedcommand\b)|start-process\b[^\n]{0,60}\b(?:powershell|pwsh|cmd)(?:\.exe)?\b|(?:irm|invoke-restmethod|iwr|invoke-webrequest)\b[^\n|]*\|\s*(?:iex|invoke-expression)', re.I)),
    # Direct shell invocation in an installer. An installer running the connector binary
    # via `& $var` is NOT a shell and won't match; a literal shell/scripting-host does.
    # Covers POSIX (`bash -c`, `/bin/sh -c`, `env sh -c`), Windows cmd with padded flags
    # (`cmd /d /s /c`), powershell with any flags before -Command, the `&` call operator,
    # and scripting hosts. NOTE: an installer is executable by nature - this catches
    # known-bad direct-exec forms; CODEOWNERS review of install-script changes is the
    # backstop for the unbounded tail.
    ("shell-invocation", "P1",
     re.compile(r'\bcmd(?:\.exe)?\b[^\n|]*\s/[ckCK]\b'
                r'|&\s*["\']?(?:powershell|pwsh|cmd|bash|sh|zsh)\b'
                r'|(?:powershell|pwsh)(?:\.exe)?\b[^\n|]*-(?:c|command|e|enc|encodedcommand)\b'
                r'|(?:^|[\s;&|(])(?:env\s+\S*\s+)?(?:/(?:usr/)?bin/)?(?:ba|z|da|a)?sh\s+-c\b'
                r'|\b(?:mshta|wscript|cscript|rundll32)\b', re.I)),
    # download from a non-GitHub host (legit installers pull from GitHub Releases).
    ("nongithub-download", "P2",
     re.compile(r'(?:curl|wget|Invoke-WebRequest|iwr|Invoke-RestMethod|irm)\b[^\n|]*https?://(?!(?:[\w.-]+\.)?(?:github\.com|githubusercontent\.com))', re.I)),
]

# Matches both indented require-block entries AND single-line `require X vY`
# (a single-line require was previously invisible to the allowlist - codex P1).
MOD_RE = re.compile(r"(?m)^(?:\s+|require\s+)([a-zA-Z0-9][^\s]*\.[^\s]+/[^\s]+)\s+v\S+")
# Same shape but ALSO captures the version (for go.mod base-vs-HEAD version-delta).
MOD_VER_RE = re.compile(r"(?m)^(?:\s+|require\s+)([a-zA-Z0-9][^\s]*\.[^\s]+/[^\s]+)\s+(v\S+)")
ARROW_RE = re.compile(r"=>\s+(\S+)")  # `replace X => TARGET [vY]`

# Scanners that provide UNIQUE coverage and so are fail-closed under --require-scanners
# (a missing/erroring one is a P1 in CI/release, not a silent skip). semgrep is excluded
# deliberately: its .semgrep.yml rules MIRROR the always-on builtin GO_RULES, so its
# absence does not reduce coverage, and its pip install is the flakiest - making it
# fail-closed would re-introduce the very false-RED this change removes.
REQUIRED_SCANNERS = ("gosec", "govulncheck", "osv-scanner")

# Control-plane rules that report the gate ITSELF failed (a scanner missing/erroring, an
# osv package that won't map). These are NEVER suppressible - a base-owned suppression
# must not be able to silence "the gate could not run" (codex). They are always P1.
CONTROL_RULES = frozenset({"scanner-missing", "scanner-error", "osv-mapping-error"})


def run(cmd, cwd=None, timeout=120, env=None):
    try:
        p = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout,
                           env=({**os.environ, **env} if env else None))
        return p.returncode, p.stdout, p.stderr
    except FileNotFoundError:
        return 127, "", "not-installed"
    except subprocess.TimeoutExpired:
        return 124, "", "timeout"


def have(tool: str) -> bool:
    return run([tool, "--version"])[0] in (0, 1)


def list_slugs() -> list[str]:
    """Every skill dir, not just the ones with a Go module.

    This used to filter on cli/go.mod, so `--all` silently skipped every
    markdown-only skill - the ones whose shipped Python nothing else reads."""
    d = REPO / "skills"
    if not d.is_dir():
        return []
    return sorted(p.name for p in d.iterdir()
                  if (p / "cli" / "go.mod").is_file() or (p / "SKILL.md").is_file())


def modules_in_gomod(gomod: Path) -> set[str]:
    return set(MOD_RE.findall(gomod.read_text(errors="replace"))) if gomod.is_file() else set()


def replace_targets(gomod: Path) -> list[str]:
    return ARROW_RE.findall(gomod.read_text(errors="replace")) if gomod.is_file() else []


def rebuild_allowlist() -> int:
    mods: set[str] = set()
    for slug in list_slugs():
        mods |= modules_in_gomod(REPO / "skills" / slug / "cli" / "go.mod")
    ALLOWLIST.write_text(json.dumps({
        "_comment": ("Approved Go module dependencies for msp-skills connectors. A module in "
                     "any skills/*/cli/go.mod (require OR `replace` target) NOT listed here is a "
                     "P1 in check_security_gate.py - review every addition (anti-typosquat / "
                     "anti-backdoor / anti-fork-swap). Regenerate with --rebuild-allowlist ONLY "
                     "after a human has vetted the new dependency."),
        "allowed": sorted(mods),
    }, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {ALLOWLIST} with {len(mods)} approved modules")
    return 0


def _policy_text(path: Path, ref: str | None) -> str | None:
    """Read a policy file from a trusted base ref (codex P1: in-PR policy edits must not
    take effect), falling back to the working tree when no ref is given."""
    if ref:
        rel = str(path.relative_to(REPO))
        code, out, _ = run(["git", "-C", str(REPO), "show", f"{ref}:{rel}"])
        return out if code == 0 else None
    return path.read_text(encoding="utf-8") if path.is_file() else None


def load_allowlist(ref: str | None = None) -> set[str]:
    txt = _policy_text(ALLOWLIST, ref)
    try:
        return set(json.loads(txt).get("allowed", [])) if txt else set()
    except json.JSONDecodeError:
        return set()


def load_suppressions(ref: str | None = None) -> set[tuple[str, str]]:
    txt = _policy_text(SUPPRESSIONS, ref)
    try:
        return {(e["file"], e["rule"]) for e in json.loads(txt).get("suppress", [])} if txt else set()
    except (json.JSONDecodeError, KeyError):
        return set()


def changed_files(base: str, slug: str) -> list[Path] | None:
    # -z and bytes: raw names, split only on NUL. Without -z git quotes unusual names
    # ("skills/x/\303\251.py"), and decoding as text would turn a carriage return
    # inside a name into a newline; either way that file is never scanned in diff
    # mode. --no-renames: a rename lists both ends, so a file moved out of this
    # connector still counts as a change to it.
    git = ["git", "-C", str(REPO), "-c", "core.quotePath=false", "diff", "-z", "--no-renames", "--name-only"]
    out = None
    for rng in (f"{base}...HEAD", base):
        try:
            p = subprocess.run(git + [rng], capture_output=True, timeout=120)
        except (OSError, subprocess.TimeoutExpired):
            return None
        if p.returncode == 0:
            out = p.stdout
            break
    if out is None:
        return None
    prefix = f"skills/{slug}/"
    names = [os.fsdecode(x) for x in out.split(b"\0") if x]
    return [REPO / f for f in names if f.startswith(prefix)]


def scan_go_patterns(files: list[Path], suppress: set[tuple[str, str]]) -> list[dict]:
    findings = []
    for f in files:
        if f.suffix != ".go" or f.name.endswith("_test.go") or not f.is_file():
            continue
        try:
            text = f.read_text(errors="replace")
        except OSError:
            continue
        rel = str(f.relative_to(REPO))
        for rule, sev, rx, exclude in GO_RULES:
            if exclude and exclude in f"/{rel}":
                continue
            if (rel, rule) in suppress:  # base-owned suppression (NOT self-grantable)
                continue
            m = rx.search(text)
            if m:
                lineno = text[:m.start()].count("\n") + 1
                findings.append({"tool": "builtin", "rule": rule, "severity": sev,
                                 "file": rel, "line": lineno,
                                 "evidence": text[m.start():m.start() + 80].replace("\n", "\\n")})
    return findings


def scan_install_scripts(slug: str, only: list[Path] | None, suppress: set[tuple[str, str]]) -> list[dict]:
    base = REPO / "skills" / slug
    scripts = [base / "install.sh", base / "install.ps1"]
    if only is not None:
        scripts = [s for s in scripts if s in only]
    findings = []
    for s in scripts:
        if not s.is_file():
            continue
        rel = str(s.relative_to(REPO))
        text = s.read_text(errors="replace")
        for rule, sev, rx in INSTALL_RULES:
            if (rel, rule) in suppress:
                continue
            m = rx.search(text)
            if m:
                lineno = text[:m.start()].count("\n") + 1
                findings.append({"tool": "install", "rule": rule, "severity": sev,
                                 "file": rel, "line": lineno,
                                 "evidence": text[m.start():m.start() + 80].replace("\n", "\\n")})
    return findings


def scan_python(files: list[Path], suppress: set[tuple[str, str]]) -> list[dict]:
    """Flag dangerous constructs in shipped Python, via the AST - not a regex.

    A markdown-only skill ships Python an MSP runs on their own machine, and until
    now nothing read it: scan_go_patterns filters on `.go`, and scan_install_scripts
    only opened install.sh/install.ps1.

    The AST matters. connect-tool's own grab_secret.py contains the sentence "there
    is no `shell=True` anywhere" in a docstring, and skills/*/references/*.md discuss
    these constructs by name. A regex would fail the healthiest skill in the repo for
    describing good practice - false-RED, which per doctrine is as useless as
    false-GREEN. The AST sees calls, not prose.

    Calibrated against the healthy connect-tool and grab-cookie trees: every
    subprocess call there is list-argv with an explicit shell=False, and both pass.
    """
    findings: list[dict] = []

    def add(rule: str, sev: str, rel: str, line: int, evidence: str) -> None:
        if (rel, rule) in suppress:  # base-owned suppression (NOT self-grantable)
            return
        findings.append({"tool": "script", "rule": rule, "severity": sev,
                         "file": rel, "line": line, "evidence": evidence})

    def dotted(node: ast.AST) -> str:
        """Best-effort dotted name for a call target (os.system, pickle.loads)."""
        parts: list[str] = []
        while isinstance(node, ast.Attribute):
            parts.append(node.attr)
            node = node.value
        if isinstance(node, ast.Name):
            parts.append(node.id)
        return ".".join(reversed(parts))

    for f in files:
        if f.suffix != ".py" or not f.is_file():
            continue
        rel = str(f.relative_to(REPO))
        # utf-8-sig, not utf-8: a BOM runs fine but makes ast.parse raise
        # SyntaxError, which would false-P1 a valid file (Windows editors add one).
        src = f.read_text(encoding="utf-8-sig", errors="replace")
        try:
            tree = ast.parse(src)
        except SyntaxError as e:
            # Shipped Python that does not parse cannot be reviewed - and cannot run.
            add("python-syntax-error", "P1", rel, e.lineno or 0,
                f"cannot parse shipped Python: {e.msg} (fail-closed).")
            continue

        for node in ast.walk(tree):
            if not isinstance(node, ast.Call):
                continue
            name = dotted(node.func)
            ln = getattr(node, "lineno", 0)

            if name in ("eval", "exec", "compile"):
                add("python-eval-exec", "P1", rel, ln, f"{name}() on shipped code path.")
            elif name in ("os.system", "os.popen"):
                add("python-os-system", "P1", rel, ln, f"{name}() runs a shell command string.")
            elif name in ("pickle.load", "pickle.loads"):
                add("python-pickle-load", "P1", rel, ln,
                    f"{name}() executes arbitrary code when the input is untrusted.")
            elif name == "yaml.load":
                # Loader may be a keyword OR the second positional arg. Do not just
                # count args: yaml.load(s, yaml.UnsafeLoader) has 2 and is unsafe.
                loader = next((k.value for k in node.keywords if k.arg == "Loader"), None)
                if loader is None and len(node.args) >= 2:
                    loader = node.args[1]
                leaf = dotted(loader).rsplit(".", 1)[-1] if loader is not None else ""
                if leaf not in ("SafeLoader", "CSafeLoader", "BaseLoader"):
                    add("python-yaml-unsafe", "P1", rel, ln,
                        f"yaml.load() with Loader={leaf or 'unset'} executes arbitrary tags; "
                        "use SafeLoader (or yaml.safe_load).")

            # shell=True, as an actual keyword argument (not the words in a docstring).
            for kw in node.keywords:
                if kw.arg == "shell" and isinstance(kw.value, ast.Constant) and kw.value.value is True:
                    add("python-shell-true", "P1", rel, ln,
                        f"{name or 'call'}(shell=True) - the command string is shell-interpreted.")

            # subprocess given a STRING (or a built-up string) instead of a list argv.
            if name.startswith("subprocess.") and node.args:
                first = node.args[0]
                if isinstance(first, ast.Constant) and isinstance(first.value, str):
                    add("python-subprocess-string", "P1", rel, ln,
                        f"{name}() called with a command string, not a list argv.")
                elif isinstance(first, (ast.BinOp, ast.JoinedStr)):
                    add("python-subprocess-string", "P1", rel, ln,
                        f"{name}() called with a built-up command string, not a list argv.")
    return findings


# Shell/PowerShell shipped OUTSIDE install.sh/install.ps1 (bootstrap scripts, helpers).
# Narrower than INSTALL_RULES: those are tuned for the curl|sh artifact and would
# false-RED on a legitimate bootstrap script that runs the tool it just installed.
SCRIPT_RULES = [
    ("script-pipe-to-shell", "P1", re.compile(r'\|\s*(?:sudo\s+)?(?:ba|z|da|a)?sh\b', re.I)),
    ("script-decode-then-run", "P1",
     re.compile(r'(?:\bbase64\s+(?:-d|--decode|-D)\b|\bfrombase64string\b)[^\n]{0,40}\|\s*(?:ba|z)?sh\b'
                r'|\biex\b[^\n]{0,40}\bfrombase64string\b', re.I)),
    ("script-powershell-rce", "P1",
     re.compile(r'\b(?:iex|invoke-expression)\b|\bdownloadstring\b|-e(?:nc\b|ncodedcommand\b)', re.I)),
]


def _strip_comments(text: str) -> str:
    """Blank out whole-line comments, keeping line numbering intact.

    The shell equivalent of using the AST for Python. connect-tool's bootstrap.sh
    documents its own install command in the header - `curl ... | bash` - and
    bootstrap.ps1 documents the `iex` form. Those are the instructions a user
    follows, not code the script runs, and flagging them fails an honest script for
    telling the truth about itself. Only full-line comments are stripped: a trailing
    `# ...` on a real command line is left alone, so `curl x | bash  # go` still trips.
    """
    return "\n".join("" if line.lstrip().startswith("#") else line
                     for line in text.split("\n"))


def scan_shell_scripts(slug: str, files: list[Path], suppress: set[tuple[str, str]]) -> list[dict]:
    """SCRIPT_RULES over shipped .sh/.ps1 that are not the two root installers."""
    # Skip by PATH, not by basename. scan_install_scripts reads exactly
    # skills/<slug>/install.sh and install.ps1, so a basename skip would drop
    # skills/<slug>/scripts/install.sh from BOTH scanners and ship it unscanned.
    root_installers = {REPO / "skills" / slug / "install.sh",
                       REPO / "skills" / slug / "install.ps1"}
    findings: list[dict] = []
    for f in files:
        if f.suffix not in (".sh", ".ps1") or not f.is_file():
            continue
        if f in root_installers:
            continue  # covered by scan_install_scripts with the stricter INSTALL_RULES
        rel = str(f.relative_to(REPO))
        text = _strip_comments(f.read_text(encoding="utf-8", errors="replace"))
        for rule, sev, rx in SCRIPT_RULES:
            if (rel, rule) in suppress:
                continue
            m = rx.search(text)
            if m:
                findings.append({"tool": "script", "rule": rule, "severity": sev,
                                 "file": rel, "line": text[:m.start()].count("\n") + 1,
                                 "evidence": text[m.start():m.start() + 80].replace("\n", "\\n")})
    return findings


def scan_dependencies(slug: str, allow: set[str], only: list[Path] | None) -> list[dict]:
    gomod = REPO / "skills" / slug / "cli" / "go.mod"
    gosum = REPO / "skills" / slug / "cli" / "go.sum"
    findings = []
    if only is not None:
        touched_mod = gomod in only
        touched_sum = gosum in only
        if not touched_mod and not touched_sum:
            return []
        if touched_sum and not touched_mod:
            findings.append({"tool": "dep-allowlist", "rule": "gosum-changed-review", "severity": "P2",
                             "file": f"skills/{slug}/cli/go.sum", "line": 0,
                             "evidence": "go.sum changed without go.mod - review for a swapped checksum."})
        if touched_mod:
            findings.append({"tool": "dep-allowlist", "rule": "gomod-version-review", "severity": "P2",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": "go.mod changed - review every version bump (an allowlisted module "
                                         "bumped to a malicious tag still passes the path check)."})
    # require-block modules not in the allowlist
    for mod in sorted(modules_in_gomod(gomod) - allow):
        findings.append({"tool": "dep-allowlist", "rule": "unapproved-dependency", "severity": "P1",
                         "file": f"skills/{slug}/cli/go.mod", "line": 0,
                         "evidence": f"{mod} is not in dep_allowlist.json - vet it (typosquat/backdoor?)."})
    # replace directives: filesystem target, or a module target not allowlisted
    for tgt in replace_targets(gomod):
        if tgt.startswith((".", "/", "..")):
            findings.append({"tool": "dep-allowlist", "rule": "replace-to-local-path", "severity": "P1",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": f"replace => {tgt} points at a filesystem path (untrusted code swap)."})
        elif "." in tgt.split("/")[0] and tgt not in allow:
            findings.append({"tool": "dep-allowlist", "rule": "replace-to-unapproved", "severity": "P1",
                             "file": f"skills/{slug}/cli/go.mod", "line": 0,
                             "evidence": f"replace => {tgt} swaps in a module not in dep_allowlist.json (fork/backdoor?)."})
    return findings


def _external_rel(file_path: str, slug: str) -> tuple[str, str]:
    """Normalize a scanner-reported file path to (repo_relative, slug_relative).

    External scanners (gosec especially) emit ABSOLUTE paths; some emit
    module-relative ones. Return both the repo-relative form
    (skills/<slug>/cli/...) the other scanners use, and a slug-relative form
    (cli/...). A suppression entry may then be written repo-relative (scopes to
    one connector) OR slug-relative (fleet-wide, for the byte-identical
    printing-press generated files every connector carries)."""
    if not file_path:
        return "", ""
    skill_prefix = f"skills/{slug}/"        # slug-relative is relative to the skill dir -> cli/...
    cli_prefix = f"skills/{slug}/cli/"      # reconstruct repo-relative from a module-relative path
    p = Path(file_path)
    if p.is_absolute():
        try:
            repo_rel = str(p.relative_to(REPO))
        except ValueError:
            repo_rel = file_path
    elif file_path.startswith("skills/"):
        repo_rel = file_path
    else:
        repo_rel = cli_prefix + file_path
    slug_rel = repo_rel[len(skill_prefix):] if repo_rel.startswith(skill_prefix) else repo_rel
    return repo_rel, slug_rel


def gomod_requires(text: str) -> dict[str, str]:
    """Parse go.mod require lines into {module_path: version}."""
    return {m.group(1): m.group(2) for m in MOD_VER_RE.finditer(text or "")}


def pr_introduced_modules(slug: str, base: str | None) -> set[str]:
    """Modules ADDED or version-BUMPED by this PR's go.mod diff (base -> HEAD).

    This is the non-gameable osv discriminator: it compares the actual base-vs-HEAD
    module@version set, NOT the attacker-influenceable `// indirect` comment. Empty in
    full-audit/release mode (no base)."""
    if not base:
        return set()
    rel = f"skills/{slug}/cli/go.mod"
    head_path = REPO / rel
    head = gomod_requires(head_path.read_text(errors="replace")) if head_path.is_file() else {}
    code, out, _ = run(["git", "-C", str(REPO), "show", f"{base}:{rel}"])
    base_req = gomod_requires(out) if code == 0 else {}
    return {p for p, v in head.items() if base_req.get(p) != v}


# (GOOS, GOARCH) pairs the release builds (release.yml matrix: 3 OS x 2 arch). A direct
# import behind `//go:build windows` OR `//go:build arm64` (or a `*_windows.go` /
# `*_arm64.go` filename tag) is invisible to a host-only `go list`, so a vulnerable module
# imported only on one target could be wrongly demoted (codex). Union the import graph
# across EVERY shipped target; CGO_ENABLED=0 matches the release build so directness does
# not over-count cgo-only imports absent from the shipped binaries.
BUILD_TARGETS = (("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"),
                 ("darwin", "arm64"), ("windows", "amd64"), ("windows", "arm64"))


def _go_list_direct(mod_dir: Path, goos: str, goarch: str) -> set[str] | None:
    code, out, _ = run(["go", "list", "-deps", "-json", "./..."], cwd=str(mod_dir),
                       timeout=300, env={"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "0"})
    if code != 0 or not out:
        return None
    dec, i, pkgs = json.JSONDecoder(), 0, []
    while i < len(out):
        while i < len(out) and out[i] in " \t\r\n":
            i += 1
        if i >= len(out):
            break
        try:
            obj, i = dec.raw_decode(out, i)
        except json.JSONDecodeError:
            return None
        pkgs.append(obj)
    pkg_mod = {p.get("ImportPath", ""): (p.get("Module") or {}).get("Path")
               for p in pkgs if (p.get("Module") or {}).get("Path")}
    direct: set[str] = set()
    for p in pkgs:
        if (p.get("Module") or {}).get("Main"):  # a first-party (main-module) package
            for imp in p.get("Imports", []):
                m = pkg_mod.get(imp)
                if m:
                    direct.add(m)
    return direct


def direct_import_modules(mod_dir: Path) -> set[str] | None:
    """Modules whose packages are imported by THIS module's own first-party code,
    unioned across every release GOOS.

    Computed from the import GRAPH (`go list -deps -json`), NOT the `// indirect`
    comment (codex: the comment is attacker-influenceable). Returns None if `go list`
    fails for ANY target - the caller fails CLOSED (P1 scanner-error) in CI/release
    rather than silently treating unknown directness as non-direct."""
    union: set[str] = set()
    for goos, goarch in BUILD_TARGETS:
        d = _go_list_direct(mod_dir, goos, goarch)
        if d is None:
            return None  # fail closed: a target we cannot resolve = unknown directness
        union |= d
    return union


def _longest_module_prefix(name: str, mods: set[str]) -> str | None:
    """Map an osv package path to the owning go.mod module (longest-prefix, path-aware).
    For Go + `--lockfile go.mod`, osv's package.name is already the module path; this is
    the safety net for subpackage/nested-module cases."""
    cands = [m for m in mods if name == m or name.startswith(m + "/")]
    return max(cands, key=len) if cands else None


# --- fleet precedent for annotation-hidden gosec findings ---------------------------
# A gosec finding that an inline annotation would have hidden is accepted without a
# suppression entry only when the same annotated lines already ship, unchanged, in at
# least this many OTHER connectors on the trusted base (see the module docstring).
PRECEDENT_MIN = 3
ANNOTATION_RE = re.compile(r"#nosec|//gosec:disable")


def _line_range(line) -> tuple[int, int] | None:
    """gosec reports a line as "12" or "12-14"."""
    lo, _, hi = str(line).partition("-")
    try:
        a, b = int(lo), int(hi or lo)
    except ValueError:
        return None
    return (a, b) if 0 < a <= b else None


def annotated_start(lines: list[str], lo: int, hi: int) -> int | None:
    """First line of the block that carries a finding and its annotation: the finding's
    own first line when the annotation sits on its lines, else the top of the contiguous
    `//` comment block directly above when the annotation sits there. None when neither
    holds one (gosec tied the annotation to an enclosing node): no precedent then."""
    if hi > len(lines):
        return None
    if any(ANNOTATION_RE.search(x) for x in lines[lo - 1:hi]):
        return lo
    first = lo
    while first > 1 and lines[first - 2].lstrip().startswith("//"):
        first -= 1
    if first < lo and any(ANNOTATION_RE.search(x) for x in lines[first - 1:lo - 1]):
        return first
    return None


# The compared region comes from Go's own parser, never from text heuristics: an
# indented `func`, or a `}` at column 0 inside a raw string, misleads any column-0 rule
# (Codex round 3). This stdlib-only program (go/parser, go/token; no module) reads JSON
# lines {"file": F, "line": N} and answers {"file": F, "line": N, "start": S, "end": E}:
# the first and last line of the top-level declaration that contains line N, its doc
# comment included, in raw positions (a //line directive cannot move them). 0, 0 when
# the file does not parse, or when not exactly one declaration contains the line. It lives in this file so
# the gate's self-edit guard covers it.
DECL_SPAN_GO = """package main

import (
	"bufio"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
)

type req struct {
	File string `json:"file"`
	Line int    `json:"line"`
}

type resp struct {
	File  string `json:"file"`
	Line  int    `json:"line"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

func main() {
	files := map[string]*ast.File{}
	fsets := map[string]*token.FileSet{}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	out := json.NewEncoder(os.Stdout)
	for in.Scan() {
		var r req
		if json.Unmarshal(in.Bytes(), &r) != nil {
			continue
		}
		f, seen := files[r.File]
		if !seen {
			fs := token.NewFileSet()
			parsed, err := parser.ParseFile(fs, r.File, nil, parser.ParseComments|parser.SkipObjectResolution)
			if err != nil {
				parsed = nil
			}
			files[r.File], fsets[r.File], f = parsed, fs, parsed
		}
		a := resp{File: r.File, Line: r.Line}
		if f != nil {
			hits := 0
			fs := fsets[r.File]
			line := func(p token.Pos) int { return fs.PositionFor(p, false).Line }
			for _, d := range f.Decls {
				start, end := line(d.Pos()), line(d.End())
				var doc *ast.CommentGroup
				switch x := d.(type) {
				case *ast.FuncDecl:
					doc = x.Doc
				case *ast.GenDecl:
					doc = x.Doc
				}
				if doc != nil {
					start = line(doc.Pos())
				}
				if start <= r.Line && r.Line <= end {
					a.Start, a.End = start, end
					hits++
				}
			}
			// Two declarations on one line: the line alone cannot say which holds
			// the finding, so neither is compared (no precedent).
			if hits != 1 {
				a.Start, a.End = 0, 0
			}
		}
		_ = out.Encode(a)
	}
}
"""
_DECL_SPAN = {"bin": None, "failed": False}


def decl_spans(requests: list[tuple[str, int]]) -> dict[tuple[str, int], tuple[int, int]]:
    """Ask DECL_SPAN_GO for the declaration span of each (absolute file, line). The helper
    is built once per run with the toolchain already on the machine (GOTOOLCHAIN=local: no
    toolchain download) outside any module, and any failure returns no spans, which gives
    no precedent (fail closed)."""
    if not requests or _DECL_SPAN["failed"]:
        return {}
    if _DECL_SPAN["bin"] is None:
        tmp = Path(tempfile.mkdtemp(prefix="gate-declspan-"))
        atexit.register(shutil.rmtree, tmp, True)
        (tmp / "main.go").write_text(DECL_SPAN_GO, encoding="utf-8")
        # Built for the machine it runs on: a GOOS/GOARCH set for the scan (GOOS=linux on
        # a Mac, say) would otherwise produce a helper this machine cannot execute. An
        # empty value means "the host" to the go command.
        # GOENV=off: flags persisted in the user's go env file (GOFLAGS=-toolexec=...)
        # must not reach this build either.
        env = {"GOTOOLCHAIN": "local", "GOWORK": "off", "GOENV": "off", "GOFLAGS": "", "GO111MODULE": "on",
               "CGO_ENABLED": "0", "GOOS": "", "GOARCH": "", "GOARM": "", "GOAMD64": "",
               "GOARM64": "", "GO386": ""}
        code, _, err = run(["go", "build", "-o", str(tmp / "declspan"), "main.go"],
                           cwd=str(tmp), timeout=300, env=env)
        if code != 0:
            print(f"WARNING: fleet precedent disabled, the declaration-span helper did not build: "
                  f"{(err or '').strip()[:200]}", file=sys.stderr)
            _DECL_SPAN["failed"] = True
            return {}
        _DECL_SPAN["bin"] = str(tmp / "declspan")
    data = "".join(json.dumps({"file": f, "line": n}) + "\n" for f, n in requests)
    try:
        p = subprocess.run([_DECL_SPAN["bin"]], input=data, capture_output=True, text=True, timeout=300)
    except (OSError, subprocess.TimeoutExpired) as e:
        print(f"WARNING: fleet precedent disabled, the declaration-span helper did not run: {e}",
              file=sys.stderr)
        _DECL_SPAN["failed"] = True
        return {}
    spans = {}
    for row in p.stdout.splitlines():
        try:
            r = json.loads(row)
        except json.JSONDecodeError:
            continue
        if r.get("start", 0) > 0:
            spans[(r["file"], r["line"])] = (r["start"], r["end"])
    return spans


def precedent_block(lines: list[str], lo: int, hi: int,
                    span: tuple[int, int] | None) -> list[str] | None:
    """What fleet precedent compares: the whole top-level declaration that holds the
    finding, doc comment included, as Go's parser delimits it (span). The annotation must
    sit on the finding's lines or in the comment block directly above them, inside that
    declaration. An edit anywhere in the declaration, or the line moved into another one,
    therefore loses the precedent."""
    first = annotated_start(lines, lo, hi)
    if first is None or span is None:
        return None
    start, end = span
    if not (start <= first <= lo <= hi <= end <= len(lines)):
        return None
    return lines[start - 1:end]


def count_block(lines: list[str], block: list[str]) -> int:
    n = len(block)
    return sum(1 for i in range(len(lines) - n + 1) if lines[i:i + n] == block) if n else 0


class FleetPrecedent:
    """Reads other connectors' copies of a file from the trusted base: the git ref the
    suppressions come from (--policy-base), or the working tree when none is given (a
    release or --all run, whose tree is the reviewed commit). One instance serves every
    connector of a run, so each base file is read once."""

    def __init__(self, ref: str | None, repo: Path | None = None):
        self.ref, self.repo = ref, repo or REPO
        self._paths: set[str] | None = None
        self._by_rel: dict[str, list[tuple[str, str]]] | None = None
        self._text: dict[str, str | None] = {}
        self._spans: dict[tuple[str, int], tuple[int, int] | None] = {}

    def prime(self, requests: list[tuple[str, int]]) -> None:
        """Parse every (file, line) this connector needs in one helper run."""
        todo = sorted({r for r in requests if r not in self._spans})
        got = decl_spans(todo)
        for r in todo:
            self._spans[r] = got.get(r)

    def span(self, path: str, line: int) -> tuple[int, int] | None:
        if (path, line) not in self._spans:
            self.prime([(path, line)])
        return self._spans[(path, line)]

    def _all_paths(self) -> set[str]:
        if self._paths is None:
            if self.ref:
                try:
                    p = subprocess.run(["git", "-C", str(self.repo), "ls-tree", "-r", "-z",
                                        "--name-only", self.ref, "--", "skills"],
                                       capture_output=True, timeout=120)
                    ok = p.returncode == 0
                except (OSError, subprocess.TimeoutExpired):
                    ok = False
                # An unreadable base gives no precedent at all: fail closed.
                self._paths = ({os.fsdecode(x) for x in p.stdout.split(b"\0") if x}
                               if ok else set())
            else:
                root = self.repo / "skills"
                self._paths = {str(f.relative_to(self.repo)) for f in root.glob("*/cli/**/*")
                               if f.is_file()} if root.is_dir() else set()
        return self._paths

    def _read(self, path: str) -> str | None:
        if path not in self._text:
            if self.ref:
                try:
                    p = subprocess.run(["git", "-C", str(self.repo), "show", f"{self.ref}:{path}"],
                                       capture_output=True, timeout=60)
                    self._text[path] = (p.stdout.decode("utf-8", "surrogateescape")
                                        if p.returncode == 0 else None)
                except (OSError, subprocess.TimeoutExpired):
                    self._text[path] = None
            else:
                try:
                    self._text[path] = (self.repo / path).read_bytes().decode("utf-8", "surrogateescape")
                except OSError:
                    self._text[path] = None
        return self._text[path]

    def others_with(self, slug: str, cli_rel: str, block: list[str], need: int) -> int:
        """How many OTHER connectors hold at least `need` copies of `block` in their own
        skills/<other>/cli/<cli_rel>."""
        if self._by_rel is None:
            self._by_rel = {}
            for path in sorted(self._all_paths()):
                parts = path.split("/", 3)
                if len(parts) == 4 and parts[0] == "skills" and parts[2] == "cli":
                    self._by_rel.setdefault(parts[3], []).append((parts[1], path))
        n = 0
        for other, path in self._by_rel.get(cli_rel, []):
            if other == slug:
                continue
            text = self._read(path)
            if text is not None and count_block(text.split("\n"), block) >= need:
                n += 1
        return n


def precedent_for(slug: str, repo_rel: str, line, precedent: FleetPrecedent | None) -> int:
    """Number of other connectors that carry this annotation-hidden finding's annotated
    lines unchanged (0 when there is no usable block). Accepted when >= PRECEDENT_MIN."""
    prefix = f"skills/{slug}/cli/"
    rng = _line_range(line)
    if precedent is None or rng is None or not repo_rel.startswith(prefix):
        return 0
    path = precedent.repo / repo_rel
    try:
        lines = path.read_bytes().decode("utf-8", "surrogateescape").split("\n")
    except OSError:
        return 0
    block = precedent_block(lines, *rng, precedent.span(str(path), rng[0]))
    if not block:
        return 0
    return precedent.others_with(slug, repo_rel[len(prefix):], block, count_block(lines, block))


# gosec build targets. None is the runner's own GOOS/GOARCH (Linux in CI and release,
# where this gate runs); every other entry is an extra pass with that GOOS/GOARCH and
# cgo off, as the release builds. Added 2026-10-23, when no workflow had ever scanned a
# file built only for another shipped OS: windows (its first fleet pass found four
# MEDIUM G304s in *_windows.go) and darwin (110 findings, 59 HIGH/MEDIUM, all in the
# store_adopt_darwin.go template of 17 connectors, each blocking one already carrying a
# vetted base-owned entry). darwin arm64 and amd64 report the same findings.
GOSEC_TARGETS: tuple[tuple[str, str] | None, ...] = (None, ("windows", "amd64"), ("darwin", "arm64"))


def gosec_report(code: int, out: str) -> dict | None:
    """The report of one completed gosec run, or None when it did not complete.
    Measured on gosec v2.29.0 with -quiet: a clean module exits 0 with no output; a
    finding, and a package that fails to load, exit 1 WITH a JSON report. So exit 0
    and no output is clean, exit 0 or 1 must otherwise carry an object with an
    "Issues" list, and anything else (exit 1 with no report, any other exit, other
    JSON) did not complete. Reading a missing report as "{}" made a failed run clean."""
    if code == 0 and not (out or "").strip():
        return {"Issues": []}
    if code not in (0, 1):
        return None
    try:
        report = json.loads(out)
    except (json.JSONDecodeError, TypeError):
        return None
    if not isinstance(report, dict) or not isinstance(report.get("Issues"), list):
        return None
    return report


def gosec_passes(mod: Path, slug: str, require: bool, add) -> list[dict]:
    """Every gosec issue across GOSEC_TARGETS, each once. An issue the runner's own
    pass does not report carries `_target`, every extra target that reported it
    ("GOOS=windows" or "GOOS=windows, GOOS=darwin"). A pass that does not
    complete is a scanner-error P1 under --require-scanners, named by target."""
    issues: list[dict] = []
    seen: dict[tuple, int] = {}
    for target in GOSEC_TARGETS:
        env = None if target is None else {"GOOS": target[0], "GOARCH": target[1], "CGO_ENABLED": "0"}
        label = "" if target is None else f"GOOS={target[0]}"
        code, out, _err = run(["gosec", "-quiet", "-fmt", "json", "-show-ignored", "./..."],
                              cwd=str(mod), timeout=600, env=env)
        gj = gosec_report(code, out)
        if gj is None and require:
            detail = ("timeout" if code == 124 else "not-installed" if code == 127
                      else "no report" if not (out or "").strip() else "no usable report")
            add("gosec", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                f"gosec{' (' + label + ')' if label else ''} did not complete ({detail}, exit {code}) "
                "(fail-closed).")
        for iss in (gj or {}).get("Issues", []):
            if not isinstance(iss, dict):
                continue
            key = (iss.get("rule_id"), _external_rel(str(iss.get("file", "")), slug)[0],
                   str(iss.get("line")), str(iss.get("column")))
            if key in seen:
                first = issues[seen[key]]
                if label and first.get("_target") and label not in first["_target"].split(", "):
                    first["_target"] += ", " + label
                continue
            seen[key] = len(issues)
            # The runner's own pass goes first, so a tag means its pass did not see it.
            issues.append({**iss, "_target": label} if label else iss)
    return issues


def scan_external(slug: str, suppress: set[tuple[str, str]],
                  base: str | None = None, require: bool = False,
                  precedent: FleetPrecedent | None = None,
                  accepted: list[dict] | None = None) -> list[dict]:
    findings = []
    mod = REPO / "skills" / slug / "cli"

    # Same base-owned suppression model as scan_go_patterns / scan_install_scripts
    # (the module docstring promises "a finding is suppressed only if its
    # (file, rule) is listed"); previously the external scanners ignored it, so
    # gosec findings were unsuppressable. Now normalized + filtered here.
    def add(tool: str, rule: str, sev: str, file_path: str, line, evidence: str) -> None:
        repo_rel, slug_rel = _external_rel(file_path, slug)
        # Control-plane rules ("the gate could not run") are never suppressible.
        if rule not in CONTROL_RULES and ((repo_rel, rule) in suppress or (slug_rel, rule) in suppress):
            return
        findings.append({"tool": tool, "rule": rule, "severity": sev,
                         "file": repo_rel or file_path, "line": line, "evidence": evidence})

    # Fail-closed (codex): under --require-scanners (CI/release) a missing required
    # scanner is a P1, not a silent skip. A transient install failure or an attacker
    # who can break the install must not be able to disable a gate quietly.
    gomod_path = mod / "go.mod"
    if require:
        for t in REQUIRED_SCANNERS:
            if not have(t):
                add(t, "scanner-missing", "P1", f"skills/{slug}/cli/go.mod", 0,
                    f"required scanner {t} not installed (fail-closed in CI/release).")

    if have("gosec"):
        # gosec exits 0 (clean) or 1 (issues found) on a successful run; exit code alone
        # is ambiguous (1 = issues found), so the "ran" signal is parseable JSON. Fail
        # CLOSED under --require-scanners only when gosec did NOT run: unparseable output,
        # timeout (124), or not-installed (127). We deliberately do NOT treat a non-empty
        # gosec "Golang errors" as a failure: its go/packages loader routinely emits benign
        # per-package errors (e.g. `undefined: <sym>` from command-line-arguments loads) on
        # HEALTHY connectors while still analyzing them (30+ Issues), so gating on it is a
        # fleet-wide false-RED. A genuine build break is still caught by govulncheck
        # (fail-closed) and the always-on builtin pattern scan.
        # 600s (not 300): gosec loads the whole module through go/packages; a cold load of
        # a heavy dep tree (e.g. axcient: surf + quic-go + utls + modernc/libc) on a 2-core
        # CI runner exceeds 300s and fail-closes as a scanner-error (exit 124) - an infra
        # flake, not a finding. The go-vet pre-warm step covers the cold path; this headroom
        # is the backstop so a slow runner can't false-RED a healthy connector.
        #
        # -show-ignored: inline `#nosec` / `//gosec:disable` annotations hide nothing. gosec
        # reports every finding (the same set as -nosec) and marks the ones an annotation
        # would have hidden with "nosec": true; such a finding is accepted only by a
        # base-owned suppression (in add) or by fleet precedent (below).
        #
        # One pass per build target (GOSEC_TARGETS): go/packages loads the files ONE
        # GOOS builds, so a pass on the runner's own OS never reads a file built only
        # for another shipped OS (`*_windows.go`, `//go:build windows`). Every pass
        # must complete (fail-closed per pass); findings are unioned, a finding the
        # passes share counted once.
        issues = gosec_passes(mod, slug, require, add)
        if precedent is not None:
            wanted = []
            for iss in issues:
                rng = _line_range(iss.get("line", 0))
                repo_rel, _ = _external_rel(iss.get("file", ""), slug)
                if iss.get("nosec") is True and rng and repo_rel.startswith(f"skills/{slug}/cli/"):
                    wanted.append((str(precedent.repo / repo_rel), rng[0]))
            precedent.prime(wanted)
        for iss in issues:
            sev = "P1" if iss.get("severity", "").upper() in ("HIGH", "MEDIUM") else "P2"
            details = iss.get("details", "")[:120]
            if iss.get("_target"):
                details = f"[{iss['_target']} only] " + details
            if iss.get("nosec") is True:
                repo_rel, slug_rel = _external_rel(iss.get("file", ""), slug)
                rule = iss.get("rule_id")
                if (repo_rel, rule) not in suppress and (slug_rel, rule) not in suppress:
                    others = precedent_for(slug, repo_rel, iss.get("line", 0), precedent)
                    if others >= PRECEDENT_MIN:
                        if accepted is not None:
                            accepted.append({"tool": "gosec", "rule": rule, "file": repo_rel,
                                             "line": iss.get("line", 0), "others": others})
                        continue
                details = ("an inline annotation hid this; it needs a base-owned suppression "
                           "or fleet precedent: " + details)[:160]
            add("gosec", iss.get("rule_id"), sev, iss.get("file", ""),
                iss.get("line", 0), details)
    if have("govulncheck"):
        # REACHABILITY gate. Run DEFAULT text/symbol mode and use its exit code:
        # 0 = no called vuln, 3 = a vulnerable symbol is CALLED from this module.
        # CRITICAL: -format json/sarif/openvex ALWAYS exit 0 regardless of findings
        # (documented), so the prior `-format json` + `code == 3` was dead code -
        # govulncheck never once fired a P1 (verified: a known-REACHABLE x/text vuln
        # exits 0 under -format json, 3 under text mode). Any OTHER nonzero is a
        # load/build/usage error and fails CLOSED under --require-scanners.
        # 600s (not 300): same go/packages-driven cold-load cost as gosec on heavy-dep
        # connectors; give the reachability analysis matching headroom so a slow runner
        # can't fail-close a healthy connector.
        code, out, err = run(["govulncheck", "./..."], cwd=str(mod), timeout=600)
        if code == 3:
            ev = next((ln.strip() for ln in out.splitlines() if ln.strip().startswith("Vulnerability #")),
                      "govulncheck: a vulnerability is reachable from this module's code.")
            add("govulncheck", "known-vuln", "P1", f"skills/{slug}/cli/go.mod", 0, ev[:120])
        elif code != 0 and require:
            add("govulncheck", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                f"govulncheck failed (exit {code}): {(err or out).strip()[:90]} (fail-closed).")
    if have("osv-scanner"):
        # osv-scanner is PRESENCE-only (no reachability). Gating on every advisory
        # re-blocks the whole fleet on press-pinned transitive deps a connector PR
        # cannot fix (the false-RED treadmill). Demote presence to a tracked P2;
        # keep P1 only when the PR itself introduced/bumped the module@version, when
        # the module is a DIRECT first-party import (computed from the import graph,
        # not the `// indirect` comment), or - at RELEASE only - a GHSA-only
        # high/critical advisory govulncheck's DB cannot reason about. govulncheck
        # (reachability) and dep-allowlist (identity) remain the independent P1 gates.
        osv_mods = set(gomod_requires(gomod_path.read_text(errors="replace"))) if gomod_path.is_file() else set()
        introduced = pr_introduced_modules(slug, base)
        direct = direct_import_modules(mod)  # None => go list failed (for ANY release GOOS)
        # Fail CLOSED (codex #4) the moment directness is unknowable - independent of
        # whether osv returns any vuln, so a `go list` failure can't be masked by an
        # empty osv result.
        if direct is None and require:
            add("osv-scanner", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                "`go list -deps` failed (a release GOOS did not resolve); cannot compute "
                "import-graph directness (fail-closed).")
        code_o, out, err_o = run(["osv-scanner", "--lockfile", "go.mod", "--format", "json"],
                                 cwd=str(mod), timeout=300)
        results = []
        try:
            results = json.loads(out or "{}").get("results", [])
            parsed_ok = True
        except json.JSONDecodeError:
            parsed_ok = False
        # osv-scanner: exit 0 = clean, 1 = vulnerabilities found (both are a successful
        # run we parse); ANY other code (timeout 124, missing 127, internal error) or
        # unparseable output means osv did not run -> fail CLOSED under --require-scanners
        # (codex #1: a nonzero-with-empty-stdout previously passed open).
        if (code_o not in (0, 1) or not parsed_ok) and require:
            add("osv-scanner", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                f"osv-scanner did not complete cleanly (exit {code_o}, "
                f"{'unparseable output' if not parsed_ok else (err_o or '').strip()[:60]}) (fail-closed).")
        for res in results:
            for pkg in res.get("packages", []):
                pinfo = pkg.get("package", {})
                pname, pver = pinfo.get("name", ""), pinfo.get("version", "?")
                sevmap = {vid: g.get("max_severity", "")
                          for g in pkg.get("groups", []) for vid in g.get("ids", [])}
                for v in pkg.get("vulnerabilities", []):
                    vid = v.get("id", "OSV")
                    module = pname if pname in osv_mods else _longest_module_prefix(pname, osv_mods)
                    if not module:
                        add("osv-scanner", "osv-mapping-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                            f"{vid}: osv package {pname!r} did not resolve to a go.mod module (fail-closed).")
                        continue
                    if direct is None:
                        # directness unknowable - the gate already blocked above (or is a
                        # local best-effort run); classify conservatively as not-direct so
                        # the P2 line is still recorded for visibility.
                        is_direct = False
                    else:
                        is_direct = module in direct
                    pr_new = module in introduced
                    ids = [vid] + list(v.get("aliases", []))
                    go_covered = any(str(x).startswith("GO-") for x in ids)
                    raw_sev = (sevmap.get(vid, "") or "").strip()
                    try:
                        sev_num = float(raw_sev) if raw_sev else None
                    except ValueError:
                        sev_num = None
                    # Release-only carve-out: an advisory govulncheck's DB cannot reason
                    # about (no GO- id) that is high/critical (or unknown severity) stays
                    # P1 until reviewed. Never fires in PR mode, so no treadmill.
                    ghsa_gate = (base is None) and (not go_covered) and (sev_num is None or sev_num >= 7.0)
                    gating = pr_new or is_direct or ghsa_gate
                    why = ("PR introduced/bumped this module@version" if pr_new else
                           "direct first-party import" if is_direct else
                           "release: GHSA-only, govulncheck-DB-blind, high/unknown severity" if ghsa_gate else
                           "transitive presence-only (tracked; govulncheck gates reachability)")
                    sev_tag = f"[{raw_sev}] " if raw_sev else ""
                    add("osv-scanner", vid, "P1" if gating else "P2", f"skills/{slug}/cli/go.mod", 0,
                        f"{module}@{pver} {sev_tag}- {why}; {(v.get('summary', '') or '')[:70]}")
    cfg = REPO / ".semgrep.yml"
    if have("semgrep") and cfg.is_file():
        _, out, _ = run(["semgrep", "--config", str(cfg), "--json", "--quiet", str(mod)])
        try:
            for r in json.loads(out or "{}").get("results", []):
                add("semgrep", r.get("check_id"), "P1", r.get("path", ""),
                    r.get("start", {}).get("line", 0), (r.get("extra", {}).get("message", ""))[:120])
        except json.JSONDecodeError:
            pass
    return findings


# --- MCP-surface rules (with a burn-down baseline) ---------------------------------
# Seven classes found in the 2026-10-22 template security pass, each checked by
# reading the connector's vendored MCP server source. They are STATE rules: they
# judge the connector as it stands (not just the lines a PR changed), because a
# connector that keeps the class ships it whether or not this PR touched that file.
#
# The fleet had these classes before the rules existed, so a connector already
# failing a rule is listed under that rule in security_gate_baseline.json and only
# reports (P2) until its reprint lands. Every other failure is P1. The baseline only
# SHRINKS: a listed connector that stops failing is reported as burned down
# (--update-baseline removes it), and a slug added back to a rule is P1. Rows read
# the baseline from the trusted base (--policy-base) and keep only the entries the
# PR's copy still lists, so a PR's own burn-down applies at once and its additions
# never do.
MCP_RULES = (
    ("mcp-http-no-bearer-auth",
     "StreamableHTTP server with no requireBearerAuth call: --transport http answers any "
     "caller that can reach the port"),
    ("mcp-go-no-rebinding-guard",
     "mark3labs/mcp-go below v0.57.0 has no DNS-rebinding Host guard: a web page can drive "
     "an HTTP server bound to localhost"),
    ("mcp-execute-no-confirm",
     "_execute tool with no confirm gate (a confirm parameter plus a codeOrchConfirmAccepted "
     "check): every write runs on the first call"),
    ("mcp-write-tool-nondestructive",
     "POST/PUT/PATCH/DELETE endpoint tool annotated destructiveHint=false: MCP hosts may "
     "auto-approve the write"),
    ("mcp-mutating-get-as-read",
     "GET registry entry named with an action verb and not marked NeedsConfirm/Mutating: "
     "it runs as a read, with no confirm and through the read-only switch"),
    ("mcp-share-command",
     "share subcommand present: caller-chosen --remote/--repo allow git option injection "
     "and pushing the local store anywhere"),
    ("mcp-read-only-switch-skips-typed-tools",
     "typed endpoint tools with no read-only drop (dropNonReadOnlyTools or IsMCPReadOnlyEnv "
     "in tools.go): _MCP_READ_ONLY=1 leaves their writes callable"),
)
MCP_RULE_IDS = tuple(r for r, _ in MCP_RULES)
MCP_RULE_MSG = dict(MCP_RULES)
BASELINE_READDED = "mcp-baseline-readded"
BASELINE_INVALID = "mcp-baseline-invalid"
MCP_GO_FLOOR = (0, 57, 0)
# Rule 5: the FIRST word of the path's last literal segment, split on case and
# punctuation. Whole words, not prefixes: a prefix test flags /settings (set),
# /addresses (add) and /assignments (assign), about 90 healthy reads across 21
# connectors, which is false-RED.
ACTION_VERBS = frozenset({"exec", "execute", "remove", "add", "set", "reset", "run", "invoke",
                          "send", "create", "delete", "update", "assign", "reboot", "isolate",
                          "wipe", "purge", "clear"})
WRITE_METHODS = ("POST", "PUT", "PATCH", "DELETE")
SLUG_RE = re.compile(r"[a-z0-9][a-z0-9-]*")

# Two views of a Go file, same length and offsets. `code`: comments blanked, literals
# kept. `bare`: literal CONTENTS blanked too. Structure (calls, ifs, assignments) is
# matched in `bare`, so text inside a string or a comment can never pose as code;
# literal values are read from `code` at the same offset.
_GO_LEXEME = re.compile(r'//[^\n]*|/\*.*?(?:\*/|\Z)|"(?:\\.|[^"\\\n])*"?'
                        r"|'(?:\\.|[^'\\\n])*'?|`[^`]*`?", re.S)
# A Go string literal in either form: "interpreted" or `raw`.
_STR = r'(?:"(?:\\.|[^"\\\n])*"|`[^`]*`)'
_DELIM = re.compile(r"[(){}\[\]\"'`]")


def _go_code(text: str) -> str:
    return _GO_LEXEME.sub(lambda m: re.sub(r"[^\n]", " ", m.group(0))
                          if m.group(0).startswith(("//", "/*")) else m.group(0), text)


def _go_bare(code: str) -> str:
    def blank(m: re.Match) -> str:
        t = m.group(0)
        return t[0] + re.sub(r"[^\n]", " ", t[1:-1]) + t[-1] if len(t) >= 2 else t
    return _GO_LEXEME.sub(blank, code)


def _unquote(lit: str) -> str:
    """A Go string literal's value: raw verbatim, interpreted with escapes decoded (so
    "mcp\\u002dgo" reads as mcp-go, as Go reads it)."""
    if len(lit) >= 2 and lit[0] == "`":
        return lit[1:-1]
    if len(lit) >= 2 and lit[0] == '"':
        try:
            return lit[1:-1].encode("latin-1", "backslashreplace").decode("unicode_escape")
        except (UnicodeDecodeError, UnicodeEncodeError):
            return lit[1:-1]
    return lit


def _lit_at(code: str, pos: int) -> str | None:
    """The value of the string literal starting at (or after whitespace from) pos."""
    m = re.compile(r"\s*(" + _STR + ")").match(code, pos)
    return _unquote(m.group(1)) if m else None


def _read_go(path: Path) -> tuple[str, str]:
    try:
        code = _go_code(path.read_text(encoding="utf-8", errors="replace"))
    except OSError:
        code = ""
    return code, _go_bare(code)


def _line_at(text: str, pos: int) -> int:
    return text.count("\n", 0, pos) + 1


def _skip_literal(code: str, i: int) -> int:
    """Index just past the string or rune literal that opens at code[i]."""
    q = code[i]
    if q == "`":
        j = code.find("`", i + 1)
        return len(code) if j < 0 else j + 1
    j = i + 1
    while j < len(code) and code[j] not in (q, "\n"):
        j += 2 if code[j] == "\\" else 1
    return min(j + 1, len(code))


def _close(code: str, i: int) -> int:
    """Index just past the bracket that closes the one at code[i] (literals skipped)."""
    depth, j = 0, i
    while True:
        m = _DELIM.search(code, j)
        if not m:
            return len(code)
        c, j = m.group(0), m.start()
        if c in "\"'`":
            j = _skip_literal(code, j)
            continue
        depth += 1 if c in "({[" else -1
        j += 1
        if depth == 0:
            return j


def _elements(code: str, open_idx: int):
    """(start, end) of each {...} element directly inside the composite literal at open_idx."""
    end, j = _close(code, open_idx) - 1, open_idx + 1
    while j < end:
        m = _DELIM.search(code, j, end)
        if not m:
            return
        c, k = m.group(0), m.start()
        if c in "\"'`":
            j = _skip_literal(code, k)
        elif c in "({[":
            e = _close(code, k)
            if c == "{":
                yield k, e
            j = e
        else:
            j = k + 1


def _calls(bare: str, name: str) -> list[re.Match]:
    """Call sites of `name` in real code, not its `func name(` declaration."""
    return [m for m in re.finditer(rf"\b{name}\s*\(", bare)
            if not re.search(r"\bfunc\s+$", bare[max(0, m.start() - 16):m.start()])]


def _spans(bare: str, name: str) -> list[tuple[int, int]]:
    """(open paren, end) of every call to `name`."""
    return [(m.end() - 1, _close(bare, m.end() - 1)) for m in _calls(bare, name)]


def _inside(pos: int, spans: list[tuple[int, int]]) -> bool:
    return any(a < pos < b for a, b in spans)


def _if_bodies(bare: str, cond: str) -> list[tuple[int, int]]:
    """(open brace, end) of the body of each `if <cond> {`."""
    return [(m.end() - 1, _close(bare, m.end() - 1))
            for m in re.finditer(rf"\bif\s+{cond}\s*\{{", bare)]


def _go_sources(d: Path) -> list[Path]:
    """Non-test .go files directly in d (one Go package)."""
    return sorted(p for p in d.glob("*.go") if not p.name.endswith("_test.go")) if d.is_dir() else []


def _package(d: Path) -> dict[Path, tuple[str, str]]:
    return {f: _read_go(f) for f in _go_sources(d)}


def _rel(path: Path) -> str:
    return str(path.relative_to(REPO))


def _mcp_rule_http_auth(cli: Path) -> list[tuple]:
    """The streamable server must be reachable ONLY through requireBearerAuth: each
    NewStreamableHTTPServer(...) is an argument of requireBearerAuth(...), or is assigned
    to a variable whose every later use is an argument of requireBearerAuth(...). Then no
    unauthenticated MCP handler can be served, whatever else the file does with the wrapper."""
    hits = []
    for pkg in sorted(p for p in (cli / "cmd").glob("*") if p.is_dir()):
        for f, (code, bare) in _package(pkg).items():
            auth = _spans(bare, "requireBearerAuth")
            for m in re.finditer(r"\bNewStreamableHTTPServer\s*\(", bare):
                if _inside(m.start(), auth):
                    continue
                v = re.search(r"\b([A-Za-z_]\w*)\s*:?=\s*(?:\w+\.)?$", bare[max(0, m.start() - 80):m.start()])
                end = _close(bare, m.end() - 1)
                uses = (list(re.finditer(rf"\b{v.group(1)}\b", bare[end:]))
                        if v and v.group(1) != "_" else [])
                if uses and all(_inside(end + u.start(), auth) for u in uses):
                    continue
                hits.append((_rel(f), _line_at(code, m.start()), "", "server.NewStreamableHTTPServer"))
    return hits


def _semver_below(v: str, floor: tuple[int, int, int]) -> bool:
    m = re.fullmatch(r"v(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?", v)
    if not m:
        return True  # unparseable: fail closed
    t = tuple(int(x) for x in m.groups()[:3])
    # A pre-release (or a pseudo-version) of the floor itself sorts below it.
    return t < floor or (t == floor and bool(m.group(4)))


MCP_GO = "github.com/mark3labs/mcp-go"
_GOMOD_TOKEN = re.compile(r'"(?:\\.|[^"\\])*"|`[^`]*`|=>|\S+')


def _gomod_directives(text: str):
    """(verb, tokens, line) for each require/replace entry, blocks expanded, quoted
    tokens decoded the way Go's modfile parser decodes them."""
    block = None
    for ln, line in enumerate(text.split("\n"), 1):
        s = line.strip()
        if not s:
            continue
        if block:
            if s == ")":
                block = None
            else:
                yield block, [_unquote(t) for t in _GOMOD_TOKEN.findall(s)], ln
            continue
        m = re.match(r"(require|replace|exclude|retract)\s*\(\s*$", s)
        if m:
            block = m.group(1)
            continue
        m = re.match(r"(require|replace)\s+(.*)$", s)
        if m:
            yield m.group(1), [_unquote(t) for t in _GOMOD_TOKEN.findall(m.group(2))], ln


def _mcp_rule_mcp_go(cli: Path) -> list[tuple]:
    gomod = cli / "go.mod"
    if not gomod.is_file():
        return []
    text = re.sub(r"//[^\n]*", "", gomod.read_text(encoding="utf-8", errors="replace"))
    plain = re.sub(r'"(?:\\.|[^"\\\n])*"|`[^`]*`', lambda q: _unquote(q.group(0)), text)
    if not re.search(r"(?<![\w./-])github\.com/mark3labs/mcp-go(?![\w./-])", plain):
        return []
    rel, req, reps = _rel(gomod), None, []
    for verb, toks, ln in _gomod_directives(text):
        if verb == "require" and len(toks) >= 2 and toks[0] == MCP_GO:
            req = (toks[1], ln)
        elif verb == "replace" and "=>" in toks and toks[0] == MCP_GO:
            i = toks.index("=>")
            old, new = toks[:i], toks[i + 1:]
            reps.append((old[1] if len(old) > 1 else None, new[0] if new else "",
                         new[1] if len(new) > 1 else "", ln))
    if req is None:
        return [(rel, 1, "", "mcp-go is named but no require line parses (fail-closed)")]
    ver, ln = req
    # Go applies a version-specific replace only to that version, else the wildcard one.
    rep = next((r for r in reps if r[0] == ver), None) or next((r for r in reps if r[0] is None), None)
    if rep:
        _, target, tver, rln = rep
        # A fork or a local path cannot prove the guard.
        if target != MCP_GO or _semver_below(tver, MCP_GO_FLOOR):
            return [(rel, rln, "", f"replace => {target} {tver}".strip())]
        return []
    return [(rel, ln, "", f"mcp-go {ver}")] if _semver_below(ver, MCP_GO_FLOOR) else []


def _mcp_rule_execute_confirm(cli: Path) -> list[tuple]:
    pkg = _package(cli / "internal" / "mcp")
    param = refuse = False
    for code, bare in pkg.values():
        param = param or any(_lit_at(code, m.end()) == "confirm"
                             for m in re.finditer(r"\bWithBoolean\s*\(", bare))
        # The v4.33.8 gate: `if !codeOrchConfirmAccepted(...) {` whose body returns.
        refuse = refuse or any(re.search(r"\breturn\b", bare[a:b])
                               for a, b in _if_bodies(bare, r"!\s*codeOrchConfirmAccepted\s*\([^{]*?\)"))
    if param and refuse:
        return []
    hits = []
    for f, (code, bare) in pkg.items():
        for m in re.finditer(r"\bNewTool\s*\(", bare):
            name = _lit_at(code, m.end()) or ""
            # Snake or kebab connector names: "cipp_execute", "cisco-meraki_execute".
            if re.fullmatch(r"[a-z0-9][a-z0-9_-]*_execute", name):
                hits.append((_rel(f), _line_at(code, m.start()), name, f"tool {name}"))
    return hits


def _method_at(code: str, bare: str, pos: int) -> str:
    """The HTTP method expression at pos: a literal, or http.MethodX. Anything else
    (a variable, a call) is '?', which every rule treats as a possible write."""
    lit = _lit_at(code, pos)
    if lit is not None:
        return lit.upper() or "?"
    m = re.compile(r"\s*http\.Method(\w+)\b").match(bare, pos)
    return m.group(1).upper() if m else "?"


def _typed_tools(cli: Path):
    """(file, code, offset, registration, method, path, tool) for every makeAPIHandler
    call in the internal/mcp package, any file. A call that is not an argument of an
    AddTool(...) cannot be paired with its annotations: its registration is None and the
    rules treat it as an unannotated write (fail-closed)."""
    for f, (code, bare) in _package(cli / "internal" / "mcp").items():
        adds = _spans(bare, "AddTool")
        for h in _calls(bare, "makeAPIHandler"):
            span = next(((a, b) for a, b in adds if a < h.start() < b), None)
            method = _method_at(code, bare, h.end())
            comma = re.compile(r"[^,)]*,").match(bare, h.end())
            path = (_lit_at(code, comma.end()) if comma else None) or "?"
            if span is None:
                yield f, code, h.start(), None, method, path, "?"
                continue
            reg_bare = bare[span[0]:span[1]]
            name = re.search(r"\bNewTool\s*\(", reg_bare)
            tool = (_lit_at(code, span[0] + name.end()) if name else None) or "?"
            yield f, code, span[0], reg_bare, method, path, tool


def _ann(reg_bare: str | None, call: str, value: str) -> re.Match | None:
    return re.search(rf"\b{call}\s*\(\s*{value}\s*\)", reg_bare) if reg_bare else None


def _read_only(reg_bare: str | None) -> bool:
    return bool(_ann(reg_bare, "WithReadOnlyHintAnnotation", "true"))


def _mcp_rule_write_nondestructive(cli: Path) -> list[tuple]:
    hits = []
    for f, code, off, reg, method, path, tool in _typed_tools(cli):
        if method not in WRITE_METHODS and method != "?":
            continue
        if reg is None:
            hits.append((_rel(f), _line_at(code, off), tool,
                         f"makeAPIHandler({method} {path}) outside AddTool: annotations unprovable"))
            continue
        nd = _ann(reg, "WithDestructiveHintAnnotation", "false")
        # The MCP spec reads destructiveHint only when readOnlyHint is false, so a POST
        # classified as a read (a search or report query) is not this class.
        if not nd or (method in ("POST", "?") and _read_only(reg)):
            continue
        hits.append((_rel(f), _line_at(code, off + nd.start()), tool, f"{tool} ({method} {path})"))
    return hits


# Verb-led words that are nouns when read as one word: GET /addOns lists add-ons.
_VERB_NOUNS = frozenset({"addon", "addons"})


def _segment_words(seg: str) -> list[str]:
    out: list[str] = []
    for part in re.split(r"[^A-Za-z0-9]+", seg):
        out += re.findall(r"[A-Z]+(?=[A-Z][a-z])|[A-Z]?[a-z]+|[A-Z]+|[0-9]+", part)
    return [w.lower() for w in out]


def _verb_path(path: str) -> bool:
    segs = [s for s in path.split("?", 1)[0].split("/")
            if s and not (s.startswith("{") and s.endswith("}"))]
    words = _segment_words(segs[-1]) if segs else []
    return bool(words) and words[0] in ACTION_VERBS and "".join(words[:2]) not in _VERB_NOUNS


def _mcp_rule_mutating_get(cli: Path) -> list[tuple]:
    hits = []
    for f, (code, bare) in _package(cli / "internal" / "mcp").items():
        reg = re.search(r"\bcodeOrchEndpoints\s*=\s*\[\]\s*\*?codeOrchEndpoint\s*\{", bare)
        if not reg:
            continue
        # Each element by its braces, so field order inside an entry does not matter.
        for a, b in _elements(bare, reg.end() - 1):
            entry = bare[a:b]
            mf = re.search(r"\bMethod\s*:", entry)
            pf = re.search(r"\bPath\s*:", entry)
            if not mf or not pf:
                continue
            method = _method_at(code, bare, a + mf.end())
            p = _lit_at(code, a + pf.end())
            # A method that is not provably a non-GET counts as a GET (fail-closed).
            if method not in ("GET", "?") or p is None:
                continue
            if re.search(r"\b(?:NeedsConfirm|Mutating)\s*:\s*true\b", entry):
                continue
            if _verb_path(p):
                hits.append((_rel(f), _line_at(code, a + pf.start()), p, f"GET {p}"))
    return hits


def _mcp_rule_share(cli: Path) -> list[tuple]:
    for f in sorted(cli.rglob("*.go")):
        if f.name.endswith("_test.go"):
            continue
        try:
            raw = f.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue
        # Cheap prefilter: the identifier, or a Use: literal that starts with s or an escape.
        if "newShareCmd" not in raw and not re.search(r'\bUse\s*:\s*["`](?:s|\\)', raw):
            continue
        code, bare = _read_go(f)
        m = re.search(r"\bnewShareCmd\b", bare)
        if not m:
            m = next((u for u in re.finditer(r"\bUse\s*:", bare)
                      if re.match(r"share(?:\s|$)", _lit_at(code, u.end()) or "")), None)
        if m:
            return [(_rel(f), _line_at(code, m.start()), "", code[m.start():m.start() + 40].split("\n")[0].strip())]
    return []


def _mcp_rule_read_only_drop(cli: Path) -> list[tuple]:
    """Typed write tools must be unreachable under <PREFIX>_MCP_READ_ONLY=1: each one is
    registered inside an `if !IsMCPReadOnlyEnv() {` body, or dropNonReadOnlyTools(...) is
    called inside an `if IsMCPReadOnlyEnv() {` body AFTER every write is registered (same
    file; a drop that runs first leaves the later writes callable)."""
    writes = [t for t in _typed_tools(cli) if not _read_only(t[3])]
    if not writes:
        return []
    env = r"(?:\w+\.)?IsMCPReadOnlyEnv\s*\(\s*\)"
    pkg = _package(cli / "internal" / "mcp")
    for f, (code, bare) in pkg.items():
        drops = _calls(bare, "dropNonReadOnlyTools")
        if any(_inside(d.start(), _if_bodies(bare, env))
               and all(t[0] == f and t[2] < d.start() for t in writes) for d in drops):
            return []
    open_ = []
    for t in writes:
        bare = pkg[t[0]][1]
        if not _inside(t[2], _if_bodies(bare, "!" + r"\s*" + env)):
            open_.append(t)
    if not open_:
        return []
    f, code, off, _, _, _, tool = open_[0]
    return [(_rel(f), _line_at(code, off), "", f"{len(open_)} non-read-only typed tool(s), first {tool}")]


MCP_RULE_FUNCS = {
    "mcp-http-no-bearer-auth": _mcp_rule_http_auth,
    "mcp-go-no-rebinding-guard": _mcp_rule_mcp_go,
    "mcp-execute-no-confirm": _mcp_rule_execute_confirm,
    "mcp-write-tool-nondestructive": _mcp_rule_write_nondestructive,
    "mcp-mutating-get-as-read": _mcp_rule_mutating_get,
    "mcp-share-command": _mcp_rule_share,
    "mcp-read-only-switch-skips-typed-tools": _mcp_rule_read_only_drop,
}


def mcp_surface_hits(slug: str, suppress: set[tuple[str, str]]) -> dict[str, list[tuple]]:
    """{rule: [(file, line, detail, evidence), ...]} for the rules this connector fails.

    A base-owned suppression (file, rule) silences a rule for that file; for the
    per-entry rules (write tools, verb GETs) (file, "rule:<tool or path>") silences
    one entry."""
    cli = REPO / "skills" / slug / "cli"
    if not cli.is_dir():
        return {}
    out = {}
    for rule, fn in MCP_RULE_FUNCS.items():
        hits = [h for h in fn(cli)
                if (h[0], rule) not in suppress and not (h[2] and (h[0], f"{rule}:{h[2]}") in suppress)]
        if hits:
            out[rule] = hits
    return out


def parse_baseline(text: str | None, require_all: bool) -> tuple[dict[str, set[str]] | None, list[str]]:
    """(rule -> slugs, problems). None with no problems = the file does not exist.

    require_all: every rule must have a key (an empty list once burned down), so a
    key cannot be dropped and later re-seeded with new slugs. The BASE copy is read
    without it: a rule newer than the base has no key there yet."""
    if text is None:
        return None, []
    try:
        data = json.loads(text)
    except json.JSONDecodeError as e:
        return None, [f"security_gate_baseline.json is not valid JSON ({e.msg})"]
    rules = data.get("rules") if isinstance(data, dict) else None
    if not isinstance(rules, dict):
        return None, ['security_gate_baseline.json has no "rules" object']
    out, problems = {}, []
    for r, slugs in rules.items():
        if r not in MCP_RULE_IDS:
            problems.append(f"baseline names an unknown rule {r!r}")
        elif not isinstance(slugs, list) or not all(isinstance(s, str) and SLUG_RE.fullmatch(s)
                                                    for s in slugs) or len(set(slugs)) != len(slugs):
            problems.append(f"baseline rule {r!r} is not a list of distinct valid slugs")
        else:
            out[r] = set(slugs)
    if require_all:
        problems += [f"baseline has no entry for rule {r!r} (an empty list when burned down)"
                     for r in MCP_RULE_IDS if r not in rules]
    return out, problems


def baseline_readditions(head: dict[str, set[str]], base: dict[str, set[str]] | None) -> list[tuple[str, str]]:
    """(rule, slug) the working copy lists that the base does not: a re-addition. A
    rule the base has no key for is new, and its first listing is the seed."""
    if base is None:
        return []
    return [(r, s) for r in MCP_RULE_IDS if r in base for s in sorted(head.get(r, set()) - base[r])]


def effective_baseline(policy_ref: str | None) -> tuple[dict[str, set[str]], list[str], list[tuple[str, str]]]:
    """(baseline in force, problems, re-additions). Problems fail closed (P1)."""
    head, problems = parse_baseline(_policy_text(BASELINE, None), require_all=True)
    if head is None:
        head = {}
        problems = problems or [f"{BASELINE.name} is missing; every rule needs its entry"]
    if not policy_ref:
        return {r: head.get(r, set()) for r in MCP_RULE_IDS}, problems, []
    if run(["git", "-C", str(REPO), "rev-parse", "--verify", "--quiet", f"{policy_ref}^{{commit}}"])[0] != 0:
        return {r: set() for r in MCP_RULE_IDS}, problems + [f"cannot resolve --policy-base {policy_ref!r}"], []
    base, bp = parse_baseline(_policy_text(BASELINE, policy_ref), require_all=False)
    if base is None and not bp:
        # No baseline at the base. If the base's gate already reads one, it was deleted:
        # nothing is baselined, and a file re-created here must not become the seed.
        gate = _policy_text(REPO / "tools" / "maintainer" / "check_security_gate.py", policy_ref) or ""
        if BASELINE.name in gate:
            return ({r: set() for r in MCP_RULE_IDS},
                    problems + [f"the base gate reads {BASELINE.name} but the base has none (deleted?)"], [])
        # The base predates the baseline (the change that introduces it): its seed.
        return {r: head.get(r, set()) for r in MCP_RULE_IDS}, problems, []
    base = base or {}
    return baseline_in_force(head, base), problems + bp, baseline_readditions(head, base)


def baseline_in_force(head: dict[str, set[str]], base: dict[str, set[str]]) -> dict[str, set[str]]:
    """The base's entries the working copy still lists: a PR's own burn-down applies at
    once, and a slug it adds never does. A rule with no base key yet uses its seed."""
    return {r: (head.get(r, set()) & base[r]) if r in base else head.get(r, set()) for r in MCP_RULE_IDS}


def _baseline_line(rule: str, slug: str) -> int:
    try:
        text = BASELINE.read_text(encoding="utf-8")
    except OSError:
        return 0
    k = text.find(f'"{rule}"')
    s = text.find(f'"{slug}"', k) if k >= 0 else -1
    return _line_at(text, s) if s >= 0 else 0


def scan_mcp_surface(slug: str, suppress: set[tuple[str, str]],
                     baseline: dict[str, set[str]]) -> tuple[list[dict], list[str]]:
    """(findings, burned-down rules) for one connector against the baseline in force."""
    findings: list[dict] = []
    hits = mcp_surface_hits(slug, suppress)
    for rule in MCP_RULE_IDS:
        h = hits.get(rule)
        if not h:
            continue
        listed = slug in baseline.get(rule, set())
        more = f" (+{len(h) - 1} more)" if len(h) > 1 else ""
        findings.append({"tool": "mcp-surface", "rule": rule, "severity": "P2" if listed else "P1",
                         "file": h[0][0], "line": h[0][1], "baselined": listed,
                         "count": len(h),
                         "evidence": (("baselined, burn-down pending: " if listed else "")
                                      + f"{MCP_RULE_MSG[rule]} [{h[0][3]}{more}]")})
    burned = [r for r in MCP_RULE_IDS if slug in baseline.get(r, set()) and r not in hits]
    return findings, burned


def baseline_policy_findings(problems: list[str], readded: list[tuple[str, str]]) -> list[dict]:
    """P1s for a baseline that cannot be trusted: unreadable, or grown."""
    findings: list[dict] = []
    rel = str(BASELINE.relative_to(REPO))
    for p in problems:
        findings.append({"tool": "mcp-surface", "rule": BASELINE_INVALID, "severity": "P1",
                         "file": rel, "line": 0, "evidence": f"{p} (fail-closed)."})
    for r, s in readded:
        findings.append({"tool": "mcp-surface", "rule": BASELINE_READDED, "severity": "P1",
                         "file": rel, "line": _baseline_line(r, s),
                         "evidence": f"{s} is listed under {r} but not in the base baseline: the "
                                     "baseline only shrinks, so a connector cannot be (re-)added."})
    return findings


def _fleet_failing(suppress: set[tuple[str, str]]) -> dict[str, set[str]]:
    failing: dict[str, set[str]] = {r: set() for r in MCP_RULE_IDS}
    for slug in list_slugs():
        for r in mcp_surface_hits(slug, suppress):
            failing[r].add(slug)
    return failing


def _write_baseline(rules: dict[str, set[str]]) -> None:
    BASELINE.write_text(json.dumps({
        "_comment": ("Connectors that failed an MCP-surface rule in check_security_gate.py when "
                     "the rule landed, listed per rule. A listed connector reports (P2) instead of "
                     "blocking until its reprint fixes the class; any other connector failing the "
                     "rule is P1. This list only shrinks: --update-baseline removes connectors that "
                     "no longer fail, and a slug added back is P1. Every rule keeps its key, as an "
                     "empty list once fully burned down."),
        "rules": {r: sorted(rules[r]) for r in MCP_RULE_IDS},
    }, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {BASELINE.relative_to(REPO)}: " + ", ".join(f"{r} {len(rules[r])}" for r in MCP_RULE_IDS))


def update_baseline(suppress: set[tuple[str, str]]) -> int:
    """Remove every (rule, slug) that no longer fails. Never adds anything: a missing
    file or rule key is an error (--seed-baseline is the separate, explicit seed)."""
    head, problems = parse_baseline(_policy_text(BASELINE, None), require_all=True)
    if head is None or problems:
        for p in problems or [f"{BASELINE.name} is missing"]:
            print(f"ERROR: {p} (use --seed-baseline only for a rule that has no entry yet)", file=sys.stderr)
        return 1
    failing = _fleet_failing(suppress)
    new = {r: head[r] & failing[r] for r in MCP_RULE_IDS}
    for r in MCP_RULE_IDS:
        for s in sorted(head[r] - new[r]):
            print(f"burned down: {r}: {s}")
    _write_baseline(new)
    return 0


def seed_baseline(suppress: set[tuple[str, str]]) -> int:
    """Give a rule that has NO entry yet (a new rule, or the first baseline) today's
    failures. Rules that already have an entry are left exactly as they are."""
    head, problems = parse_baseline(_policy_text(BASELINE, None), require_all=False)
    if problems:
        for p in problems:
            print(f"ERROR: {p}", file=sys.stderr)
        return 1
    head = head or {}
    missing = [r for r in MCP_RULE_IDS if r not in head]
    if not missing:
        print("every rule already has an entry; nothing to seed (--update-baseline only removes)")
        return 0
    failing = _fleet_failing(suppress)
    for r in missing:
        head[r] = failing[r]
        print(f"seeded: {r}: {len(failing[r])} connector(s)")
    _write_baseline(head)
    return 0


def mcp_report(results: list[dict], baseline: dict[str, set[str]]) -> None:
    """Fleet counts per rule, for the status doc."""
    print(f"\nMCP-surface fleet report ({len(results)} skills scanned)")
    print(f"  {'rule':<40} {'failing':>7} {'baselined':>9} {'new':>4} {'burned down':>11}")
    for r in MCP_RULE_IDS:
        fail = [x["slug"] for x in results for f in x["findings"] if f["rule"] == r]
        new = [x["slug"] for x in results for f in x["findings"] if f["rule"] == r and not f["baselined"]]
        burned = [x["slug"] for x in results if r in x.get("burned_down", [])]
        print(f"  {r:<40} {len(fail):>7} {len(baseline.get(r, set())):>9} {len(new):>4} {len(burned):>11}")
        if new:
            print(f"    new (P1): {', '.join(new)}")
        if burned:
            print(f"    burned down (run --update-baseline): {', '.join(burned)}")


# --- the security-gate workflow's summary job, and the gate's self-test ------------

def summarize_gate(needs: dict, verdict_dir: Path, sha: str, run_id: str) -> list[str]:
    """The fail-closed verdict of the security-gate workflow's summary job.

    `needs` is the workflow's toJSON(needs). Returns the list of problems; the
    summary passes only when it is empty. A connector row that was skipped,
    cancelled, timed out, failed, or never reported counts as a failure."""
    problems: list[str] = []

    def result(job: str) -> str:
        j = needs.get(job) if isinstance(needs, dict) else None
        return j.get("result", "missing") if isinstance(j, dict) else "missing"

    scope = needs.get("scope") if isinstance(needs, dict) else None
    outputs = scope.get("outputs", {}) if isinstance(scope, dict) else {}
    if result("scope") != "success":
        return [f"scope job did not succeed ({result('scope')})"]
    if result("guard") != "success":
        problems.append(f"gate-logic guard did not pass ({result('guard')})")
    gate_logic = outputs.get("gate_logic") == "true"
    st = result("selftest")
    if gate_logic and st != "success":
        problems.append(f"gate self-test did not pass ({st}) on a PR that changes gate logic")
    if not gate_logic and st not in ("success", "skipped"):
        problems.append(f"gate self-test ended {st}")
    relevant = outputs.get("relevant")
    try:
        expected = json.loads(outputs.get("slugs_json") or "[]")
    except json.JSONDecodeError:
        return problems + ["scope slugs_json is not JSON"]
    if not isinstance(expected, list) or not all(
            isinstance(s, str) and re.fullmatch(r"[a-z0-9][a-z0-9-]*", s) for s in expected) \
            or len(set(expected)) != len(expected):
        return problems + ["scope slugs_json is not a list of distinct valid slugs"]
    seen: dict[str, dict] = {}
    if verdict_dir.is_dir():
        for p in sorted(verdict_dir.rglob("verdict-*.json")):
            try:
                v = json.loads(p.read_text(encoding="utf-8"))
            except (OSError, ValueError):
                problems.append(f"unreadable verdict file {p.name}")
                continue
            s = v.get("slug") if isinstance(v, dict) else None
            if not isinstance(s, str) or p.name != f"verdict-{s}.json" or s in seen:
                problems.append(f"malformed or duplicate verdict file {p.name}")
                continue
            seen[s] = v
    if relevant == "true":
        if not expected:
            problems.append("scope reported connector changes but no slugs")
        if result("scanners") != "success":
            problems.append(f"scanner install job did not succeed ({result('scanners')})")
        if result("gate") != "success":
            problems.append(f"connector gate rows did not all succeed ({result('gate')})")
        for s in expected:
            v = seen.get(s)
            if v is None:
                problems.append(f"{s}: no verdict (row skipped, cancelled, timed out or never ran)")
            elif v.get("verdict") != "pass":
                problems.append(f"{s}: verdict {v.get('verdict')!r}")
            elif v.get("sha") != sha or str(v.get("run_id")) != str(run_id):
                problems.append(f"{s}: verdict is for another commit or run")
        for s in sorted(set(seen) - set(expected)):
            problems.append(f"{s}: verdict for a connector this change does not touch")
    elif relevant == "false":
        if expected:
            problems.append("scope reported no connector changes but listed slugs")
        if seen:
            problems.append("verdicts present although no connector changed")
        if result("gate") not in ("skipped",):
            problems.append(f"connector rows ran although none were expected ({result('gate')})")
    else:
        problems.append(f"scope did not report whether connectors changed ({relevant!r})")
    return problems


# --- self-test ---------------------------------------------------------------------

def _self_test_summary() -> int:
    failed = 0
    tmp = Path(tempfile.mkdtemp(prefix="gate-summary-"))
    try:
        sha, run_id = "abc123", "42"

        def needs(**over):
            base = {"scope": {"result": "success", "outputs": {
                        "relevant": "true", "gate_logic": "false",
                        "slugs_json": json.dumps(["alpha", "beta"])}},
                    "guard": {"result": "success"}, "selftest": {"result": "skipped"},
                    "scanners": {"result": "success"}, "gate": {"result": "success"}}
            for k, v in over.items():
                if k == "outputs":
                    base["scope"]["outputs"].update(v)
                else:
                    base[k] = v
            return base

        def verdicts(name, rows):
            d = tmp / name
            d.mkdir()
            for s, v in rows.items():
                (d / f"verdict-{s}.json").write_text(json.dumps(v), encoding="utf-8")
            return d

        ok = {"verdict": "pass", "sha": sha, "run_id": run_id}
        both = {"alpha": {"slug": "alpha", **ok}, "beta": {"slug": "beta", **ok}}
        cases = [
            ("every row ran and passed", needs(), both, True),
            ("a row never reported (skipped/cancelled/timed out)", needs(),
             {"alpha": both["alpha"]}, False),
            ("a row reported block", needs(),
             {**both, "beta": {"slug": "beta", **ok, "verdict": "block"}}, False),
            ("matrix result failure (a row timed out)", needs(gate={"result": "failure"}), both, False),
            ("matrix result cancelled", needs(gate={"result": "cancelled"}), both, False),
            ("matrix result skipped", needs(gate={"result": "skipped"}), {}, False),
            ("matrix missing from needs", {k: v for k, v in needs().items() if k != "gate"}, both, False),
            ("scanner install failed", needs(scanners={"result": "failure"}), both, False),
            ("scope failed", needs(scope={"result": "failure", "outputs": {}}), {}, False),
            ("gate-logic guard failed", needs(guard={"result": "failure"}), both, False),
            ("verdict from another run", needs(),
             {**both, "beta": {"slug": "beta", **ok, "run_id": "41"}}, False),
            ("verdict for an unexpected connector", needs(),
             {**both, "gamma": {"slug": "gamma", **ok}}, False),
            ("gate logic changed and self-test skipped", needs(outputs={"gate_logic": "true"}), both, False),
            ("gate logic changed and self-test passed",
             needs(outputs={"gate_logic": "true"}, selftest={"result": "success"}), both, True),
            ("no connector changed", needs(outputs={"relevant": "false", "slugs_json": "[]"},
                                           scanners={"result": "skipped"}, gate={"result": "skipped"}),
             {}, True),
            ("no connector changed but rows ran", needs(outputs={"relevant": "false", "slugs_json": "[]"},
                                                        gate={"result": "success"}), {}, False),
            ("relevant flag missing", needs(outputs={"relevant": ""}), both, False),
            ("connectors changed but slug list empty", needs(outputs={"slugs_json": "[]"}), {}, False),
            ("hostile slug list", needs(outputs={"slugs_json": json.dumps(["a;rm -rf /"])}), {}, False),
        ]
        for i, (name, n, rows, want) in enumerate(cases):
            probs = summarize_gate(n, verdicts(f"c{i}", rows), sha, run_id)
            got = not probs
            print(f"  [{'PASS' if got == want else 'FAIL'}] summary: {name} -> "
                  f"{'pass' if got else 'fail: ' + probs[0]}")
            failed += got != want
    finally:
        shutil.rmtree(tmp, ignore_errors=True)
    return failed


SCANNER_PINS = ("GOSEC_VERSION", "GOVULNCHECK_VERSION", "OSV_SCANNER_VERSION", "SEMGREP_VERSION",
                "SCANNER_GO", "GOSEC_SHA256", "GOVULNCHECK_SHA256", "OSV_SCANNER_SHA256")
SUMMARY_NAME = ("name: ${{ github.event_name == 'pull_request' && 'security-gate' "
                "|| 'security-gate-manual' }}")


def workflow_problems(gate_yml: str, release_yml: str) -> list[str]:
    """The PR gate and the release gate must scan with the same pinned versions
    (a divergence lets a PR pass on rules the release would fail, or the reverse),
    neither may install @latest, and only the PR summary job may report the
    required `security-gate` check."""
    problems = []
    pins = {}
    for label, text in (("security-gate.yml", gate_yml), ("release.yml", release_yml)):
        code = "\n".join(ln for ln in text.splitlines() if not ln.lstrip().startswith("#"))
        if "@latest" in code:
            problems.append(f"{label} installs a scanner @latest")
        for k in SCANNER_PINS:
            vals = re.findall(rf'(?m)^\s*{k}:\s*"([^"]+)"\s*$', text)
            if len(vals) != 1:
                problems.append(f"{label} pins {k} {len(vals)} times (want exactly 1)")
            else:
                pins.setdefault(k, {})[label] = vals[0]
    for k, by in pins.items():
        if len(set(by.values())) > 1:
            problems.append(f"{k} differs between the gates: {by}")
    if gate_yml.count(SUMMARY_NAME) != 1:
        problems.append("the summary job no longer names itself security-gate only for pull_request runs")
    gate_code = [ln for ln in gate_yml.splitlines() if not ln.lstrip().startswith("#")]
    summary = gate_yml.split(SUMMARY_NAME, 1)[-1]
    guard_at = summary.find('if [ "$GUARD" != "success" ]')
    base_at = summary.find("ref: ${{ needs.scope.outputs.base }}")
    script_at = summary.find("check_security_gate.py --summarize")
    if not (0 <= guard_at < base_at < script_at):
        problems.append("the summary must check the guard, then run the BASE copy of --summarize")
    code_text = "\n".join(gate_code)
    if re.search(r"actions/cache\b", code_text) or "cache-hit" in code_text:
        problems.append("the PR gate must not restore any cache: a PR can write its own cache "
                        "scope, and extraction can write outside the cache path")
    # setup-go caches modules and builds by default, and setup-python can cache pip:
    # both restore PR-writable state, so every setup step must keep caching off.
    steps = re.split(r"(?m)^\s*- (?=name:|uses:)", code_text)
    for st in steps:
        if re.search(r"uses:\s*actions/setup-go@", st) and not re.search(r"(?m)^\s*cache:\s*false\s*$", st):
            problems.append("a setup-go step does not set cache: false")
            break
        if re.search(r"uses:\s*actions/setup-python@", st) and re.search(r"(?m)^\s*cache(-dependency-path)?:", st):
            problems.append("a setup-python step enables a cache")
            break
    for label, text in (("security-gate.yml", gate_yml), ("release.yml", release_yml)):
        if not re.search(r'not the reviewed \$5 \(fail-closed\)"\n\s*return 1\b', text):
            problems.append(f"{label}: a scanner whose digest does not match its pin must fail the job")
    rel_gate = release_yml.split("\n  security_gate:\n", 1)[-1]
    rel_gate = re.split(r"\n  [A-Za-z0-9_-]+:\n", rel_gate, maxsplit=1)[0]
    rel_steps = rel_gate.split("\n      - ")
    setup = [st for st in rel_steps if "uses: actions/setup-go@" in st]
    if "actions/cache" in rel_gate or not setup or not all(
            re.search(r"(?m)^\s*cache:\s*false\s*$", st) for st in setup):
        problems.append("release.yml's security_gate job must keep setup-go caching off and restore no cache")
    if 'export GOTOOLCHAIN="go$SCANNER_GO" CGO_ENABLED=0 GOFLAGS=-trimpath' not in rel_gate:
        problems.append("release.yml must build the scanners with the reproducible contract")
    # -I: a PR can add tools/maintainer/json.py (or ./re.py, ./pip/) and Python would
    # import it ahead of the standard library from the script's or current directory.
    for ln in gate_code:
        if re.search(r"\bpython3\b(?!\s+-I\b)", ln) and "python-version" not in ln:
            problems.append(f"security-gate.yml runs Python without -I: {ln.strip()[:80]}")
            break
    if '"git", "diff", "-z", "--no-renames", "--name-only"' not in gate_yml:
        problems.append("the scope diff must list both ends of a rename (-z --no-renames)")
    if not re.search(r"python3 -I tools/maintainer/check_security_gate\.py", release_yml):
        problems.append("release.yml runs the gate without python3 -I")
    upload = re.search(r"uses: actions/upload-artifact@\S+\n\s+with:\n\s+name: scanners\n\s+path: ([^\n]+)", gate_yml)
    if not upload or upload.group(1).strip() != "${{ runner.temp }}/handoff":
        problems.append("the scanner artifact must be built from a fresh handoff directory of verified "
                        "binaries only, never the restored cache directory")
    for k in ("GOSEC_SHA256", "GOVULNCHECK_SHA256", "OSV_SCANNER_SHA256"):
        if not re.search(rf'(?m)^\s*{k}:\s*"[0-9a-f]{{64}}"\s*$', gate_yml):
            problems.append(f"security-gate.yml does not pin {k}")
    if re.search(r"(?m)^\s+name:\s*['\"]?security-gate['\"]?\s*$", gate_yml):
        problems.append("a job other than the summary is named security-gate")
    return problems


def _self_test_workflows() -> int:
    failed = 0
    gate_yml = (REPO / ".github/workflows/security-gate.yml").read_text(encoding="utf-8")
    rel_yml = (REPO / ".github/workflows/release.yml").read_text(encoding="utf-8")
    cases = [
        ("this repo's two workflows", gate_yml, rel_yml, True),
        ("release pins an older govulncheck", gate_yml,
         re.sub(r'GOVULNCHECK_VERSION: "[^"]+"', 'GOVULNCHECK_VERSION: "v1.7.0"', rel_yml), False),
        ("PR gate installs @latest", gate_yml.replace('"$2@$4"', '"$2@latest"'), rel_yml, False),
        ("a pin is missing", re.sub(r'(?m)^\s*SEMGREP_VERSION:.*\n', "", gate_yml), rel_yml, False),
        ("a row job takes the required name", gate_yml.replace("name: gate (${{ matrix.slug }})",
                                                               "name: security-gate"), rel_yml, False),
        ("summary reports the required name on any event", gate_yml.replace(
            SUMMARY_NAME, "name: security-gate"), rel_yml, False),
        ("summary runs the PR's copy of its logic", gate_yml.replace(
            "ref: ${{ needs.scope.outputs.base }}", "ref: ${{ github.sha }}"), rel_yml, False),
        ("summary no longer checks the guard first", gate_yml.replace(
            'if [ "$GUARD" != "success" ]', 'if [ "$GUARD" = "never" ]'), rel_yml, False),
        ("a scanner cache is restored", gate_yml.replace(
            "      - name: Build each pinned scanner cold",
            "      - uses: actions/cache/restore@v4\n        with:\n          path: x\n          key: y\n"
            "      - name: Build each pinned scanner cold"), rel_yml, False),
        ("setup-go caching left on", gate_yml.replace("go-version: ${{ env.SCANNER_GO }}\n          cache: false",
                                                      "go-version: ${{ env.SCANNER_GO }}"), rel_yml, False),
        ("setup-python caches pip", gate_yml.replace('python-version: "3.12"', 'python-version: "3.12"\n          cache: pip', 1),
         rel_yml, False),
        ("a digest mismatch is tolerated", re.sub(r'(not the reviewed \$5 \(fail-closed\)"\n\s*)return 1',
                                                 r"\1return 0", gate_yml), rel_yml, False),
        ("scanner hash pin removed", re.sub(r'(?m)^\s*GOSEC_SHA256:.*\n', "", gate_yml), rel_yml, False),
        ("a row runs the gate without -I", gate_yml.replace(
            "python3 -I tools/maintainer/check_security_gate.py \\", "python3 tools/maintainer/check_security_gate.py \\"),
         rel_yml, False),
        ("scope runs inline Python without -I", gate_yml.replace("python3 -I - <<'PY'", "python3 - <<'PY'"),
         rel_yml, False),
        ("scanner artifact uploads the restored cache directory", gate_yml.replace(
            "name: scanners\n          path: ${{ runner.temp }}/handoff", "name: scanners\n          path: ${{ runner.temp }}/scanner-cache"),
         rel_yml, False),
        ("scope diff follows renames", gate_yml.replace('"-z", "--no-renames", "--name-only"', '"-z", "--name-only"'),
         rel_yml, False),
        ("release pins a different scanner digest", gate_yml, re.sub(
            r'GOSEC_SHA256: "[0-9a-f]+"', 'GOSEC_SHA256: "' + "0" * 64 + '"', rel_yml), False),
        ("release tolerates a digest mismatch", gate_yml, re.sub(
            r'(not the reviewed \$5 \(fail-closed\)"\n\s*)return 1', r"\1return 0", rel_yml), False),
        ("release gate setup-go caching left on", gate_yml, rel_yml.replace(
            'go-version: "1.26"\n          # Cold on purpose', 'go-version: "1.26"\n          # Was cold on purpose').replace(
            "          cache: false\n      - name: Install pinned scanners", "      - name: Install pinned scanners"),
         False),
        ("release builds scanners without the reproducible contract", gate_yml, rel_yml.replace(
            'export GOTOOLCHAIN="go$SCANNER_GO" CGO_ENABLED=0 GOFLAGS=-trimpath', "true"), False),
        ("release runs the gate without -I", gate_yml, rel_yml.replace(
            "python3 -I tools/maintainer/check_security_gate.py", "python3 tools/maintainer/check_security_gate.py"), False),
    ]
    for name, g, r, want in cases:
        probs = workflow_problems(g, r)
        got = not probs
        print(f"  [{'PASS' if got == want else 'FAIL'}] workflows: {name} -> "
              f"{'pass' if got else 'fail: ' + probs[0]}")
        failed += got != want
    return failed


def _self_test_changed_files() -> int:
    """changed_files() must see a name git would quote, and both ends of a rename."""
    global REPO
    failed = 0
    tmp = Path(tempfile.mkdtemp(prefix="gate-diff-"))
    saved = REPO
    try:
        def git(*a: str) -> None:
            subprocess.run(["git", "-C", str(tmp), "-c", "user.email=t@example.com", "-c", "user.name=t",
                            *a], check=True, capture_output=True)
        git("init", "-q")
        (tmp / "skills/a").mkdir(parents=True)
        (tmp / "skills/b").mkdir(parents=True)
        (tmp / "skills/a/guard.py").write_text("\n".join(str(i) for i in range(50)), encoding="utf-8")
        (tmp / "skills/b/README.md").write_text("b\n", encoding="utf-8")
        git("add", "-A")
        git("commit", "-q", "-m", "base")
        git("mv", "skills/a/guard.py", "skills/b/guard.py")
        (tmp / "skills/b/\u00e9vil.py").write_text("import os\nos.system('id')\n", encoding="utf-8")
        (tmp / "skills/b/cr\rname.py").write_text("import os\nos.system('id')\n", encoding="utf-8")
        git("add", "-A")
        git("commit", "-q", "-m", "change")
        REPO = tmp
        a = [str(f.relative_to(tmp)) for f in (changed_files("HEAD~1", "a") or [])]
        b = [str(f.relative_to(tmp)) for f in (changed_files("HEAD~1", "b") or [])]
        flagged = [f["file"] for f in scan_python([tmp / p for p in b], set())]
        for name, ok in (("a file moved out of a connector still counts as a change to it",
                          a == ["skills/a/guard.py"]),
                         ("a name git would quote is still listed and scanned",
                          "skills/b/\u00e9vil.py" in b and "skills/b/\u00e9vil.py" in flagged),
                         ("a carriage return inside a name survives intact and is scanned",
                          "skills/b/cr\rname.py" in b and "skills/b/cr\rname.py" in flagged)):
            print(f"  [{'PASS' if ok else 'FAIL'}] diff scope: {name}")
            failed += not ok
    finally:
        REPO = saved
        shutil.rmtree(tmp, ignore_errors=True)
    return failed


# Fixture snippets for the MCP-surface self-test: the smallest shape of each class
# as the press emits it, plus the shapes that must NOT fire (the healthy template,
# near-miss names, a marker that only appears in a comment).
_MAIN_OPEN = '''package main

import "github.com/mark3labs/mcp-go/server"

func main() {
	httpSrv := server.NewStreamableHTTPServer(s)
	_ = httpSrv.Start(":7777")
}
'''
_MAIN_AUTH = '''package main

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
)

func main() {
	inner := server.NewStreamableHTTPServer(s)
	srv := &http.Server{Handler: requireBearerAuth(token, inner)}
	_ = srv.ListenAndServe()
}

func requireBearerAuth(expected string, next http.Handler) http.Handler { return next }
'''
_MAIN_AUTH_COMMENT = _MAIN_OPEN.replace("func main() {", "// requireBearerAuth(token, inner) is applied upstream.\nfunc main() {")
_MAIN_AUTH_DECL_ONLY = _MAIN_OPEN + "\nfunc requireBearerAuth(expected string, next http.Handler) http.Handler { return next }\n"
_MAIN_STDIO = 'package main\n\nfunc main() {\n\t_ = server.ServeStdio(s)\n}\n'
_MAIN_AUTH_DISCARDED = _MAIN_AUTH.replace("srv := &http.Server{Handler: requireBearerAuth(token, inner)}",
                                          "_ = requireBearerAuth(token, inner)\n\tsrv := &http.Server{Handler: inner}")
_MAIN_AUTH_VAR = _MAIN_AUTH.replace("srv := &http.Server{Handler: requireBearerAuth(token, inner)}",
                                    "h := requireBearerAuth(token, inner)\n\tsrv := &http.Server{Handler: h}")
_MAIN_AUTH_NESTED_DISCARD = _MAIN_AUTH.replace(
    "srv := &http.Server{Handler: requireBearerAuth(token, inner)}",
    "_ = fmt.Sprint(requireBearerAuth(token, inner))\n\tsrv := &http.Server{Handler: inner}")
_MAIN_AUTH_IN_STRING = _MAIN_OPEN.replace("func main() {", "var _ = `requireBearerAuth(token, httpSrv)`\n\nfunc main() {")
_MAIN_AUTH_DIRECT = _MAIN_OPEN.replace("httpSrv := server.NewStreamableHTTPServer(s)",
                                       "httpSrv := &http.Server{Handler: requireBearerAuth(token, server.NewStreamableHTTPServer(s))}")
_MAIN_AUTH_ARG = _MAIN_AUTH.replace("srv := &http.Server{Handler: requireBearerAuth(token, inner)}",
                                    "srv := newMCPHTTPServer(addr, requireBearerAuth(token, inner))")


def _fx_gomod(body: str) -> str:
    return f"module example.com/acme-cli\n\ngo 1.26\n\n{body}\n"


def _fx_execute_tool(name: str, gated: bool, check: bool = True) -> str:
    param = '\n\t\t\tmcplib.WithBoolean("confirm", mcplib.Description("Set to true to run.")),' if gated else ""
    body = ('\tif !codeOrchConfirmAccepted(args["confirm"]) {\n\t\treturn preview, nil\n\t}\n'
            if gated and check else "")
    return f'''package mcp

func RegisterCodeOrchestrationTools(s *server.MCPServer) {{
	s.AddTool(
		mcplib.NewTool("{name}",
			mcplib.WithString("endpoint_id", mcplib.Required()),{param}
		),
		handleCodeOrchExecute,
	)
}}

func handleCodeOrchExecute(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {{
{body}	return nil, nil
}}

func codeOrchConfirmAccepted(v any) bool {{
	confirmed, ok := v.(bool)
	return ok && confirmed
}}
'''


def _fx_typed_tools(tools: list[tuple[str, str, str, list[str]]], extra: str = "") -> str:
    regs = "".join(f'''	s.AddTool(
		mcplib.NewTool("{n}",
{"".join(f"			mcplib.{a},{chr(10)}" for a in ann)}		),
		makeAPIHandler("{m}", "{p}", false, false, nil, nil, nil),
	)
''' for n, m, p, ann in tools)
    return f'''package mcp

func RegisterTools(s *server.MCPServer) {{
{regs}{extra}}}

// makeAPIHandler creates a generic MCP tool handler for an API endpoint.
func makeAPIHandler(method, pathTemplate string, readOnly bool, binaryResponse bool, h map[string]string, b []mcpParamBinding, p []string) server.ToolHandlerFunc {{
	return nil
}}
'''


_RO = ["WithReadOnlyHintAnnotation(true)", "WithDestructiveHintAnnotation(false)"]
_ND = ["WithDestructiveHintAnnotation(false)"]
_D = ["WithDestructiveHintAnnotation(true)"]
_DROP = "\tif cliutil.IsMCPReadOnlyEnv() {\n\t\tdropNonReadOnlyTools(s)\n\t}\n"


def _fx_registry(entries: list[tuple[str, str, str]]) -> str:
    """entries: (method, path, extra field line or '')."""
    body = "".join(f'''	{{
		ID:     "e{i}.list",
		Method: "{m}",
		Path:   "{p}",{chr(10) + chr(9) + chr(9) + x if x else ""}
	}},
''' for i, (m, p, x) in enumerate(entries))
    return f"package mcp\n\nvar codeOrchEndpoints = []codeOrchEndpoint{{\n{body}}}\n"


def _self_test_mcp_surface() -> int:
    """Each MCP-surface rule fires on its class and stays silent on the healthy shape,
    and the baseline only shrinks."""
    global REPO, BASELINE
    failed = 0
    tmp = Path(tempfile.mkdtemp(prefix="gate-mcp-"))
    saved = (REPO, BASELINE)
    try:
        REPO = tmp
        BASELINE = tmp / "tools/maintainer/security_gate_baseline.json"
        BASELINE.parent.mkdir(parents=True)

        def put(slug: str, files: dict[str, str]) -> str:
            for rel, text in files.items():
                f = tmp / "skills" / slug / "cli" / rel
                f.parent.mkdir(parents=True, exist_ok=True)
                f.write_text(text, encoding="utf-8")
            return slug

        n = 0

        def case(rule: str, files: dict[str, str], want: bool, name: str,
                 suppress: set[tuple[str, str]] | None = None) -> None:
            nonlocal failed, n
            n += 1
            slug = put(f"c{n}", files)
            got = rule in mcp_surface_hits(slug, suppress or set())
            ok = got == want
            print(f"  [{'PASS' if ok else 'FAIL'}] mcp-surface {rule}: {name} -> {'fires' if got else 'silent'}")
            failed += not ok

        main_go = "cmd/acme-mcp/main.go"
        r1 = "mcp-http-no-bearer-auth"
        case(r1, {main_go: _MAIN_OPEN}, True, "StreamableHTTP with no token check")
        case(r1, {main_go: _MAIN_AUTH}, False, "server wrapped in requireBearerAuth (template v4.31.2+)")
        case(r1, {main_go: _MAIN_AUTH_COMMENT}, True, "requireBearerAuth only named in a comment")
        case(r1, {main_go: _MAIN_AUTH_DECL_ONLY}, True, "requireBearerAuth declared but never called")
        case(r1, {main_go: _MAIN_STDIO}, False, "stdio-only server")
        case(r1, {main_go: _MAIN_AUTH_DISCARDED}, True, "requireBearerAuth called but its result discarded")
        case(r1, {main_go: _MAIN_AUTH_VAR}, False, "wrapped handler held in a variable, then served")
        case(r1, {main_go: _MAIN_AUTH_ARG}, False,
             "wrapped handler passed as an argument (template v4.33.9 shape)")
        case(r1, {main_go: _MAIN_AUTH_NESTED_DISCARD}, True,
             "wrapper result swallowed by another call while the raw server is served")
        case(r1, {main_go: _MAIN_AUTH_IN_STRING}, True, "requireBearerAuth( only inside a raw string")
        case(r1, {main_go: _MAIN_AUTH_DIRECT}, False, "server built directly inside requireBearerAuth(...)")

        r2 = "mcp-go-no-rebinding-guard"
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go v0.47.0\n)")}, True, "v0.47.0")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go v0.57.0\n)")}, False, "v0.57.0")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go v0.58.2\n)")}, False, "v0.58.2")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go v0.57.0-rc.1\n)")}, True,
             "a pre-release of v0.57.0")
        case(r2, {"go.mod": _fx_gomod("require github.com/mark3labs/mcp-go v0.56.9 // indirect")}, True,
             "single-line require below the floor")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go v0.57.0\n)\n\n"
                                   "replace github.com/mark3labs/mcp-go => github.com/mark3labs/mcp-go v0.47.0")},
             True, "a replace pins an older version")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go v0.57.0\n)\n\n"
                                   "replace github.com/mark3labs/mcp-go => github.com/acme/mcp-go v0.60.0")},
             True, "a replace swaps in a fork")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go v0.47.0\n)\n\n"
                                   "replace github.com/mark3labs/mcp-go => github.com/mark3labs/mcp-go v0.57.1")},
             False, "a replace lifts the version to v0.57.1")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/spf13/cobra v1.9.1\n)")}, False, "no mcp-go at all")
        case(r2, {"go.mod": _fx_gomod('require "github.com/mark3labs/mcp-go" v0.47.0')}, True, "quoted module path")
        case(r2, {"go.mod": _fx_gomod('require "github.com/mark3labs/mcp-go" v0.57.0')}, False,
             "quoted module path at the floor")
        case(r2, {"go.mod": _fx_gomod('require "github.com/mark3labs/mcp\\u002dgo" v0.47.0')}, True,
             "an escaped quoted module path (Go decodes it)")
        case(r2, {"go.mod": _fx_gomod("require github.com/mark3labs/mcp-go v0.56.0\n\n"
                                   "replace github.com/mark3labs/mcp-go v0.57.0 => github.com/mark3labs/mcp-go v0.57.1")},
             True, "a version-specific replace Go ignores does not lift the floor")
        case(r2, {"go.mod": _fx_gomod("require github.com/mark3labs/mcp-go v0.57.0\n\n"
                                   "replace github.com/mark3labs/mcp-go v0.47.0 => github.com/mark3labs/mcp-go v0.47.0")},
             False, "an unused low-version replace does not lower a safe require")
        case(r2, {"go.mod": _fx_gomod("require github.com/mark3labs/mcp-go v0.47.0\n\n"
                                   "replace github.com/mark3labs/mcp-go v0.47.0 => github.com/mark3labs/mcp-go v0.57.1")},
             False, "a version-specific replace that applies lifts the floor")
        case(r2, {"go.mod": _fx_gomod("require (\n\tgithub.com/mark3labs/mcp-go-fork v0.40.0\n)\n\n"
                                   "exclude github.com/mark3labs/mcp-go v0.40.0")},
             True, "mcp-go named but no require parses (fail-closed)")

        r3, orch = "mcp-execute-no-confirm", "internal/mcp/code_orch.go"
        case(r3, {orch: _fx_execute_tool("acme_execute", gated=False)}, True, "_execute with no confirm (pre-4.33.8)")
        case(r3, {orch: _fx_execute_tool("acme_execute", gated=True)}, False,
             "confirm parameter plus codeOrchConfirmAccepted (template v4.33.8+)")
        case(r3, {orch: _fx_execute_tool("acme_execute", gated=True, check=False)}, True,
             "confirm parameter declared but never checked")
        case(r3, {orch: _fx_execute_tool("cisco-meraki_execute", gated=False)}, True, "kebab-case connector name")
        case(r3, {orch: _fx_execute_tool("computers_command-execute", gated=False)}, False,
             "a typed tool whose action is command-execute")
        case(r3, {orch: _fx_execute_tool("acme_execute", gated=True).replace(
            "if !codeOrchConfirmAccepted(args[\"confirm\"]) {", "_ = codeOrchConfirmAccepted(args[\"confirm\"])\n\tif false {")},
             True, "the confirm check is called but nothing branches on it")
        case(r3, {orch: _fx_execute_tool("acme_execute", gated=False).replace('NewTool("acme_execute"', "NewTool(`acme_execute`")},
             True, "tool name as a raw string")
        case(r3, {orch: _fx_execute_tool("acme_execute", gated=False)
                  + 'var _ = `mcplib.WithBoolean("confirm") if !codeOrchConfirmAccepted(x) { return }`\n'},
             True, "the whole gate only inside a raw string")
        case(r3, {orch: _fx_execute_tool("acme_execute", gated=True).replace("\t\treturn preview, nil\n", "")},
             True, "the refusal branch does not return")

        r4, tools = "mcp-write-tool-nondestructive", "internal/mcp/tools.go"
        case(r4, {tools: _fx_typed_tools([("computers_command-execute", "POST", "/Computers/{id}/CommandExecute", _ND)])},
             True, "POST write labeled destructiveHint=false")
        case(r4, {tools: _fx_typed_tools([("things_delete", "DELETE", "/things/{id}", _ND)])}, True,
             "DELETE labeled destructiveHint=false")
        case(r4, {tools: _fx_typed_tools([("things_create", "POST", "/things", _D)])}, False,
             "POST labeled destructiveHint=true")
        case(r4, {tools: _fx_typed_tools([("invoices_search", "POST", "/invoices/search", _RO)])}, False,
             "read-only POST (a search)")
        case(r4, {tools: _fx_typed_tools([("things_list", "GET", "/things", _RO)])}, False, "GET read")
        case(r4, {tools: _fx_typed_tools([("things_delete", "DELETE", "/things/{id}", _ND)]).replace('"DELETE"', "`DELETE`")},
             True, "method as a raw string")
        case(r4, {tools: _fx_typed_tools([("things_create", "POST", "/things", _ND)]).replace('"POST"', "http.MethodPost")},
             True, "method as http.MethodPost")
        case(r4, {tools: _fx_typed_tools([("things_create", "POST", "/things", _ND)]).replace('"POST"', "verb")},
             True, "method in a variable (cannot prove a read: fail-closed)")
        case(r4, {"internal/mcp/endpoints.go": _fx_typed_tools([("things_create", "POST", "/things", _ND)])},
             True, "registered in a file other than tools.go")
        case(r4, {tools: _fx_typed_tools([], extra='\th := makeAPIHandler("POST", "/things", false, false, nil, nil, nil)\n'
                                                 '\ts.AddTool(thingsCreateTool, h)\n')},
             True, "handler hoisted out of AddTool (annotations unprovable: fail-closed)")

        r5 = "mcp-mutating-get-as-read"
        case(r5, {orch: _fx_registry([("GET", "/RemoveStandard", "")])}, True, "GET /RemoveStandard")
        case(r5, {orch: _fx_registry([("GET", "/ExecRunBackup/{id}", "")])}, True,
             "GET /ExecRunBackup/{id} (trailing placeholder skipped)")
        case(r5, {orch: _fx_registry([("GET", "/devices/{serial}/reboot", "")])}, True, "GET .../reboot")
        case(r5, {orch: _fx_registry([("GET", "/RemoveStandard", "Mutating:     true,")])}, False,
             "marked Mutating")
        case(r5, {orch: _fx_registry([("GET", "/RemoveStandard", "NeedsConfirm: true,")])}, False,
             "marked NeedsConfirm")
        case(r5, {orch: _fx_registry([("GET", "/settings", ""), ("GET", "/addresses", ""),
                                   ("GET", "/roles/{id}/assignments", ""), ("GET", "/ListStandards", "")])},
             False, "reads whose names only START with a verb's letters")
        case(r5, {orch: _fx_registry([("POST", "/ExecRunBackup", "")])}, False, "a POST (rules 3 and 4 own writes)")
        case(r5, {orch: "package mcp\n\nvar codeOrchEndpoints = []codeOrchEndpoint{\n"
                        "\t{ID: \"ok.list\", Method: \"GET\", Path: \"/ListStandards\", Mutating: true},\n"
                        "\t{Method: \"GET\", Path: \"/RemoveStandard\", ID: \"remove\"},\n}\n"},
             True, "a later entry with ID after Method and Path (entries split by braces, not by ID)")
        case(r5, {orch: _fx_registry([("GET", "/RemoveStandard", "")]).replace('"/RemoveStandard"', "`/RemoveStandard`")},
             True, "path as a raw string")
        case(r5, {orch: _fx_registry([("GET", "/RemoveStandard", "")]).replace('Method: "GET"', "Method: http.MethodGet")},
             True, "method as http.MethodGet")
        case(r5, {orch: _fx_registry([("GET", "/RemoveStandard", "")]).replace('Method: "GET"', "Method: verb")},
             True, "method in a variable (cannot prove a non-GET: fail-closed)")
        case(r5, {orch: _fx_registry([("GET", "/v2/sites/{id}/addOns", "")])}, False, "GET .../addOns lists add-ons")
        case(r5, {orch: _fx_registry([("GET", "/RemoveStandard", "")])}, False, "per-entry base-owned suppression",
             suppress={(f"skills/c{n + 1}/cli/{orch}", f"{r5}:/RemoveStandard")})

        r6, share = "mcp-share-command", "internal/cli/share.go"
        case(r6, {share: "package cli\n\nfunc newShareCmd(flags *rootFlags) *cobra.Command {\n\treturn nil\n}\n"},
             True, "newShareCmd emitted")
        case(r6, {share: 'package cli\n\nvar c = &cobra.Command{\n\tUse: "share",\n}\n'}, True, 'Use: "share"')
        case(r6, {share: 'package cli\n\nvar a = &cobra.Command{Use: "shares <serialNumber>"}\n'
                         'var b = &cobra.Command{Use: "share-point"}\n'}, False, "shares / share-point commands")
        case(r6, {share: "package cli\n\n// newShareCmd was removed from this connector.\n"}, False,
             "newShareCmd only in a comment")
        case(r6, {share: "package cli\n\nvar c = &cobra.Command{Use: `share <remote>`}\n"}, True,
             "Use as a raw string under another constructor name")
        case(r6, {share: 'package cli\n\nvar c = &cobra.Command{Use: "\\u0073hare"}\n'}, True, "Use with an escaped literal")

        r7 = "mcp-read-only-switch-skips-typed-tools"
        writes = [("things_list", "GET", "/things", _RO), ("things_create", "POST", "/things", _D)]
        case(r7, {tools: _fx_typed_tools(writes)}, True, "typed write tool and no read-only drop")
        case(r7, {tools: _fx_typed_tools(writes, extra=_DROP)}, False, "dropNonReadOnlyTools under IsMCPReadOnlyEnv")
        case(r7, {tools: _fx_typed_tools([("things_list", "GET", "/things", _RO)])}, False,
             "only read-only typed tools: nothing to drop")
        case(r7, {tools: _fx_typed_tools([])}, False, "code-orchestration connector (makeAPIHandler declared only)")
        case(r7, {tools: _fx_typed_tools(writes, extra="\t// dropNonReadOnlyTools(s) when IsMCPReadOnlyEnv()\n")},
             True, "the drop only in a comment")
        case(r7, {tools: _fx_typed_tools(writes, extra="\t_ = cliutil.IsMCPReadOnlyEnv\n")}, True,
             "IsMCPReadOnlyEnv named but never branched on")
        guarded = _fx_typed_tools(writes).replace("\ts.AddTool(\n\t\tmcplib.NewTool(\"things_create\"",
                                                  "\tif !cliutil.IsMCPReadOnlyEnv() {\n\ts.AddTool(\n\t\tmcplib.NewTool(\"things_create\"")
        guarded = guarded.replace('makeAPIHandler("POST", "/things", false, false, nil, nil, nil),\n\t)\n',
                                  'makeAPIHandler("POST", "/things", false, false, nil, nil, nil),\n\t)\n\t}\n')
        case(r7, {tools: guarded}, False, "write registered inside if !IsMCPReadOnlyEnv() { (backup-radar shape)")
        case(r7, {tools: _fx_typed_tools(writes, extra="\tif !cliutil.IsMCPReadOnlyEnv() {\n\t\tlog(s)\n\t}\n")},
             True, "a read-only branch exists but the write is registered outside it")
        case(r7, {tools: _fx_typed_tools(writes, extra="\tif cliutil.IsMCPReadOnlyEnv() {\n\t}\n")}, True,
             "an empty read-only branch")
        case(r7, {tools: _fx_typed_tools(writes, extra="\tif false {\n\t\tdropNonReadOnlyTools(s)\n\t}\n")}, True,
             "dropNonReadOnlyTools called outside a read-only branch")
        case(r7, {tools: _fx_typed_tools(writes).replace("func RegisterTools(s *server.MCPServer) {\n",
                                                         "func RegisterTools(s *server.MCPServer) {\n" + _DROP)},
             True, "the drop runs before the writes are registered")
        case(r7, {tools: _fx_typed_tools(writes, extra="\tvar _ = `if cliutil.IsMCPReadOnlyEnv() { dropNonReadOnlyTools(s) }`\n")},
             True, "the drop only inside a raw string")
        case(r7, {tools: _fx_typed_tools([]), "internal/mcp/endpoints.go": _fx_typed_tools(writes).replace(
                  "func makeAPIHandler", "func unusedHandler"),
                  orch: "package mcp\n\nfunc gated(method string) bool {\n\treturn cliutil.IsMCPReadOnlyEnv() && method != \"GET\"\n}\n"},
             True, "typed writes outside tools.go; code-orch's return expression is not a drop")

        # --- the baseline -------------------------------------------------------------
        def check(name: str, ok: bool) -> None:
            nonlocal failed
            print(f"  [{'PASS' if ok else 'FAIL'}] baseline: {name}")
            failed += not ok

        def doc(rules: dict[str, list[str]]) -> str:
            return json.dumps({"rules": {r: rules.get(r, []) for r in MCP_RULE_IDS}})

        b, p = parse_baseline(doc({r1: ["a"]}), require_all=True)
        check("a well-formed file loads", not p and b[r1] == {"a"})
        check("a missing file reads as absent, not empty", parse_baseline(None, True) == (None, []))
        for label, text in (("not JSON", "{"), ("no rules object", "[]"),
                            ("a rule that does not exist", json.dumps({"rules": {"nope": []}})),
                            ("a slug list that is not a list", json.dumps({"rules": {r1: "a"}})),
                            ("a duplicate slug", json.dumps({"rules": {r1: ["a", "a"]}})),
                            ("a hostile slug", json.dumps({"rules": {r1: ["a;rm -rf /"]}}))):
            check(f"{label} is refused", bool(parse_baseline(text, require_all=False)[1]))
        check("a dropped rule key is refused (it could be re-seeded later)",
              bool(parse_baseline(json.dumps({"rules": {r1: []}}), require_all=True)[1]))
        base = {r1: {"a", "b"}, r2: {"a"}}
        check("a re-addition is rejected",
              baseline_readditions({r1: {"a", "b", "c"}, r2: {"a"}}, base) == [(r1, "c")])
        check("a slug burned down then added back is rejected",
              baseline_readditions({r1: {"a", "b"}, r2: {"a", "b"}}, base) == [(r2, "b")])
        check("a shrink is not a re-addition", baseline_readditions({r1: {"a"}, r2: set()}, base) == [])
        eff = baseline_in_force({r1: {"a", "c"}, r2: {"a"}, r3: {"x"}}, base)
        check("in force: the PR's burn-down applies at once, its additions never",
              eff[r1] == {"a"} and eff[r2] == {"a"})
        check("in force: a rule newer than the base uses its seed", eff[r3] == {"x"})
        pol = baseline_policy_findings(["bad"], [(r1, "c")])
        check("re-additions and an unreadable baseline are P1",
              [f["severity"] for f in pol] == ["P1", "P1"]
              and {f["rule"] for f in pol} == {BASELINE_INVALID, BASELINE_READDED})

        bad = put("bad", {main_go: _MAIN_OPEN})
        good = put("good", {main_go: _MAIN_AUTH})
        for s in (bad, good):
            (tmp / "skills" / s / "SKILL.md").write_text("x\n", encoding="utf-8")
        f_new, _ = scan_mcp_surface(bad, set(), {r1: set()})
        f_old, _ = scan_mcp_surface(bad, set(), {r1: {bad}})
        _, burned = scan_mcp_surface(good, set(), {r1: {good}})
        check("a failing connector not in the baseline is P1", [f["severity"] for f in f_new] == ["P1"])
        check("a failing baselined connector reports P2", [f["severity"] for f in f_old] == ["P2"])
        check("a baselined connector that passes is burned down", burned == [r1])

        BASELINE.write_text(doc({r1: [good]}), encoding="utf-8")
        with contextlib.redirect_stdout(io.StringIO()):
            rc = update_baseline(set())
        after, _ = parse_baseline(BASELINE.read_text(encoding="utf-8"), require_all=True)
        check("--update-baseline retires the burned-down entry and never adds a failing one",
              rc == 0 and after[r1] == set())

        # The loader end to end, through git: the base lists a, the working copy adds b.
        def git(*a: str) -> None:
            subprocess.run(["git", "-C", str(tmp), "-c", "user.email=t@example.com", "-c", "user.name=t", *a],
                           check=True, capture_output=True)
        git("init", "-q")
        BASELINE.write_text(doc({r1: ["a"]}), encoding="utf-8")
        git("add", "-A")
        git("commit", "-q", "-m", "base")
        BASELINE.write_text(doc({r1: ["a", "b"]}), encoding="utf-8")
        eff, probs, readded = effective_baseline("HEAD")
        check("the loader rejects a slug the PR adds and keeps it out of force",
              not probs and readded == [(r1, "b")] and eff[r1] == {"a"})
        check("an unresolvable --policy-base fails closed", bool(effective_baseline("no-such-ref")[1]))
        BASELINE.unlink()
        check("a missing working baseline fails closed", bool(effective_baseline(None)[1]))
        gate = tmp / "tools/maintainer/check_security_gate.py"
        gate.write_text(f"BASELINE = '{BASELINE.name}'\n", encoding="utf-8")
        git("add", "-A")
        git("commit", "-q", "-m", "baseline deleted")
        BASELINE.write_text(doc({r6: ["c1"]}), encoding="utf-8")
        eff, probs, _ = effective_baseline("HEAD")
        check("a baseline deleted from the base is not re-seeded by the next change",
              bool(probs) and eff[r6] == set())
        BASELINE.write_text(json.dumps({"rules": {r1: []}}), encoding="utf-8")
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            rc_u = update_baseline(set())
        check("--update-baseline refuses a file with a missing rule entry (it never adds)", rc_u == 1)
        with contextlib.redirect_stdout(io.StringIO()):
            rc_s = seed_baseline(set())
        seeded, _ = parse_baseline(BASELINE.read_text(encoding="utf-8"), require_all=True)
        check("--seed-baseline fills only rules with no entry and leaves the others untouched",
              rc_s == 0 and seeded is not None and set(seeded) == set(MCP_RULE_IDS)
              and seeded[r1] == set() and len(seeded[r2]) > 0)
    finally:
        REPO, BASELINE = saved
        shutil.rmtree(tmp, ignore_errors=True)
    return failed


def _self_test_precedent() -> int:
    """Fleet precedent, both directions: an annotation-hidden finding is accepted only when
    its annotated lines already ship unchanged in >= PRECEDENT_MIN OTHER connectors on the
    trusted base, and an edited, copied, new or base-absent annotation is not."""
    failed = 0
    line = "\tdata, err := os.ReadFile(p) // #nosec G304 -- caller-owned path"
    edited = "\tdata, err := os.ReadFile(p) // #nosec G304 -- trust me"
    above = ["\t// #nosec G304 -- caller-owned path", "\tdata, err := os.ReadFile(p)"]
    above_edit = ["\t// #nosec G304 -- anything goes", "\tdata, err := os.ReadFile(p)"]

    def src(*body):
        return "\n".join(["package x", "", "func f(p string) {", *body, "}", ""])

    same, other = src(line), src("\tdata, err := os.ReadFile(q)")
    git = ["git", "-c", "user.name=t", "-c", "user.email=t@example.invalid",
           "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"]

    def case(name, base, work, finding_line, use_ref, want):
        nonlocal failed
        tmp = Path(tempfile.mkdtemp(prefix="gate-precedent-"))
        try:
            def put(files):
                for slug, text in files.items():
                    f = tmp / "skills" / slug / "cli" / "internal" / "x" / "x.go"
                    f.parent.mkdir(parents=True, exist_ok=True)
                    f.write_text(text, encoding="utf-8")
            put(base)
            subprocess.run(git + ["init", "-q", str(tmp)], check=True, capture_output=True)
            subprocess.run(git + ["-C", str(tmp), "add", "-A"], check=True, capture_output=True)
            subprocess.run(git + ["-C", str(tmp), "commit", "-q", "-m", "base"], check=True, capture_output=True)
            put(work)
            ref = {"head": "HEAD", "bogus": "no-such-ref"}.get(use_ref)
            fp = FleetPrecedent(ref, repo=tmp)
            others = precedent_for("a", "skills/a/cli/internal/x/x.go", finding_line, fp)
            got = others >= PRECEDENT_MIN
            print(f"  [{'PASS' if got == want else 'FAIL'}] precedent: {name} -> "
                  f"{'accepted' if got else 'not accepted'} ({others} other connector(s))")
            failed += got != want
        finally:
            shutil.rmtree(tmp, ignore_errors=True)

    four = {"a": same, "b": same, "c": same, "d": same}
    case("template line in 3 other connectors on the base", four, {}, 4, "head", True)
    case("the same, read from the working tree (release / --all)", four, {}, 4, None, True)
    case("only 2 other connectors carry it", {**four, "d": other}, {}, 4, "head", False)
    case("the connector itself does not count toward the 3", {**four, "d": other, "e": other},
         {}, 4, "head", False)
    case("annotation edited in this connector", four, {"a": src(edited)}, 4, "head", False)
    case("annotated line pasted a second time into the file", four, {"a": src(line, line)}, 4, "head", False)
    case("a copy only the PR adds elsewhere does not count (base read)", {**four, "d": other},
         {"d": same}, 4, "head", False)
    case("annotation on the comment line above, identical", {s: src(*above) for s in "abcd"},
         {}, 5, "head", True)
    case("annotation on the comment line above, edited here", {s: src(*above) for s in "abcd"},
         {"a": src(*above_edit)}, 5, "head", False)
    case("annotation not on or directly above the finding",
         {s: src("\t// #nosec G304 -- far away", "\tx := 1", "\tdata, err := os.ReadFile(p)") for s in "abcd"},
         {}, 6, "head", False)
    case("a plain comment directly above, annotation further away",
         {s: src("\t// #nosec G304 -- far away", "\tx := 1", "\t// read the file", "\tdata, err := os.ReadFile(p)")
          for s in "abcd"}, {}, 7, "head", False)
    case("template line moved into a new function in the same file",
         four, {"a": "\n".join(["package x", "", "func f(p string) {", "}", "",
                                  "func g(p string) {", line, "}", ""])}, 7, "head", False)
    case("a statement added inside the function that holds the line",
         four, {"a": src("\tp = \"/etc/shadow\"", line)}, 5, "head", False)
    case("a package-level annotated declaration, identical (no enclosing function)",
         {s: "package x\n\nconst u = \"https://example.invalid/token\" // #nosec G101 -- URL\n" for s in "abcd"},
         {}, 3, "head", True)
    tick = src("\t// trim the `", line)
    case("template function with a backtick in a comment, identical", {s: tick for s in "abcd"},
         {}, 5, "head", True)
    case("template text swallowed into a raw string opened above it",
         {s: tick for s in "abcd"},
         {"a": "\n".join(["package x", "", "func evil(p string) {", "\t_ = `", "func f(p string) {",
                           "\t// trim the `", line, "}", ""])}, 7, "head", False)
    glob = src("\t// globs like a/*/", line)
    case("template text swallowed into a /* */ comment opened above it",
         {s: glob for s in "abcd"},
         {"a": "\n".join(["package x", "", "func evil(p string) {", "\t/*", "func f(p string) {",
                           "\t// globs like a/*/", line, "}", ""])}, 7, "head", False)
    r3_base = 'package x\nimport "os"\nfunc f(p string) {\n\t_, _ = os.Open(p) // #nosec G304\n}\n'
    case("an indented top-level func with an edit above the line (Codex round 3)",
         {s: r3_base for s in "abcd"},
         {"a": 'package x\nimport "os"\n func evil(p string) {\n\tp += "/../../etc/shadow"\n'
               '\t_, _ = os.Open(p) // #nosec G304\n}\n'}, 5, "head", False)
    raw_tail = ('package x\n\nimport "os"\n\nfunc f(p string) {\n\tfor i := 0; i < 2; i++ {\n'
                '\t\t_, _ = os.Open(p) // #nosec G304\n\t\t_ = `\n}\n`\n%s\t}\n}\n')
    case("a column-0 } inside a raw string, edit after it (Codex round 3)",
         {s: raw_tail % "" for s in "abcd"},
         {"a": raw_tail % '\t\tp += "/../../etc/shadow"\n'}, 7, "head", False)
    case("the same file unchanged is still accepted", {s: raw_tail % "" for s in "abcd"},
         {}, 7, "head", True)
    two = 'package x\nimport "os"\nfunc f() {}; func g(p string) { for i:=0; i<2; i++ { _, _ = os.Open(p) // #nosec G304\n%s}\n}\n'
    case("two declarations on the finding's line, edit later in the second (Codex round 4)",
         {s: two % "" for s in "abcd"}, {"a": two % ' p += "/../../etc/shadow"\n'}, 3, "head", False)
    saved = dict(_DECL_SPAN)
    _DECL_SPAN.update(bin=None, failed=True)
    try:
        case("no Go toolchain for the parser helper fails closed", four, {}, 4, "head", False)
    finally:
        _DECL_SPAN.update(saved)
    case("unreadable base ref fails closed", four, {}, 4, "bogus", False)
    case("line range past the end of the file", four, {}, 99, "head", False)
    return failed


def _self_test_line_directive() -> int:
    """A line directive anywhere in a connector's Go is a P1, because it can move a
    finding onto a suppressed or precedented position; an ordinary comment is not."""
    failed = 0
    rx = next(r for name, _, r, _ in GO_RULES if name == "line-directive")
    for name, text, want in [
        ("//line at column 1", "package x\n//line filelock.go:67\nfunc f() {}\n", True),
        ("/*line*/ inline", "package x\nfunc f() { /*line a.go:3*/ g() }\n", True),
        ("//line after a tab (Go ignores it; refused anyway)", "package x\n\t//line a.go:3\n", True),
        ("a comment that mentions a line", "package x\n// line 3 is fine\n//lines\n", False),
    ]:
        got = bool(rx.search(text))
        print(f"  [{'PASS' if got == want else 'FAIL'}] line directive: {name} -> {'P1' if got else 'clean'}")
        failed += got != want
    return failed


FAKE_GOSEC = r"""#!/usr/bin/env python3
# A stand-in gosec for the self-test: answers --version, logs the GOOS of every scan,
# and reports what the case script in FAKE_GOSEC_CASE says for that GOOS.
import json, os, sys
if "--version" in sys.argv:
    print("Version: fake"); sys.exit(0)
goos = os.environ.get("GOOS", "")
with open(os.environ["FAKE_GOSEC_LOG"], "a") as fh:
    fh.write((goos or "native") + " " + os.environ.get("CGO_ENABLED", "") + "\n")
case = json.loads(os.environ["FAKE_GOSEC_CASE"]).get(goos or "native", {})
if case.get("exit") is not None and "issues" not in case:
    sys.stdout.write(case.get("out", "")); sys.exit(case["exit"])  # exactly this output, no newline
mod = os.getcwd()
print(json.dumps({"Issues": [{"rule_id": r, "severity": sev, "file": os.path.join(mod, f), "line": str(ln),
                              "column": "2", "details": d, "nosec": ns}
                             for r, sev, f, ln, d, ns in case.get("issues", [])], "Golang errors": {}}))
sys.exit(1 if case.get("issues") else 0)
"""


def _self_test_gosec_targets() -> int:
    """The per-target gosec passes, with a stand-in gosec on PATH: a finding only the
    Windows or only the darwin pass reports blocks; with those passes removed it would
    not; a finding several passes report counts once; each pass fails closed on its
    own; a base-owned entry or the annotation rule treats an extra-target finding like
    any other."""
    global REPO
    failed = 0
    saved_repo, saved_env = REPO, {k: os.environ.get(k) for k in ("PATH", "FAKE_GOSEC_CASE", "FAKE_GOSEC_LOG")}
    tmp = Path(tempfile.mkdtemp(prefix="gate-gosec-targets-")).resolve()  # REPO is resolved too
    try:
        REPO = tmp
        mod = tmp / "skills" / "acme" / "cli"
        mod.mkdir(parents=True)
        (mod / "go.mod").write_text("module acme\n\ngo 1.26\n")
        bindir = tmp / "bin"
        bindir.mkdir()
        (bindir / "gosec").write_text(FAKE_GOSEC)
        (bindir / "gosec").chmod(0o755)
        log = tmp / "gosec.log"
        os.environ["FAKE_GOSEC_LOG"] = str(log)
        # Only the stand-in and the system tools: no real scanner can be picked up.
        os.environ["PATH"] = f"{bindir}{os.pathsep}/usr/bin{os.pathsep}/bin"
        shared = ("G104", "LOW", "shared.go", 7, "errors unhandled", False)
        win = ("G304", "MEDIUM", "open_windows.go", 13, "Potential file inclusion via variable", False)
        mac = ("G115", "HIGH", "adopt_darwin.go", 155, "integer overflow conversion int32 -> uint32", False)

        def gate(case: dict, require: bool = True, suppress: set | None = None) -> list[dict]:
            os.environ["FAKE_GOSEC_CASE"] = json.dumps(case)
            log.write_text("")
            got = scan_external("acme", suppress or set(), None, require)
            return [f for f in got if f["tool"] == "gosec"]

        def p1(found: list[dict]) -> list[dict]:
            return [f for f in found if f["severity"] == "P1"]

        cases = []
        found = gate({"native": {"issues": [shared]}, "windows": {"issues": [shared, win]}})
        cases.append(("a finding only GOOS=windows reports is a P1, labelled with its target",
                      len(p1(found)) == 1 and p1(found)[0]["file"] == "skills/acme/cli/open_windows.go"
                      and "[GOOS=windows only]" in p1(found)[0]["evidence"]))
        cases.append(("a finding both passes report is counted once", sum(f["rule"] == "G104" for f in found) == 1))
        cases.append(("the Windows and darwin passes run with their GOOS and cgo off, after the runner's own",
                      [ln.split()[:1] for ln in log.read_text().splitlines()] == [["native"], ["windows"], ["darwin"]]
                      and log.read_text().splitlines()[1:] == ["windows 0", "darwin 0"]))
        found = gate({"native": {"issues": [shared]}, "windows": {"issues": [shared]},
                      "darwin": {"issues": [shared, mac]}})
        cases.append(("a finding only GOOS=darwin reports is a P1, labelled with its target",
                      len(p1(found)) == 1 and p1(found)[0]["file"] == "skills/acme/cli/adopt_darwin.go"
                      and "[GOOS=darwin only]" in p1(found)[0]["evidence"]
                      and sum(f["rule"] == "G104" for f in found) == 1))
        both = p1(gate({"native": {"issues": []}, "windows": {"issues": [win]}, "darwin": {"issues": [win]}}))
        cases.append(("a finding both extra passes report (and the runner's does not) names both targets",
                      len(both) == 1 and "[GOOS=windows, GOOS=darwin only]" in both[0]["evidence"]))
        cases.append(("a base-owned (file, rule) entry accepts a darwin-only finding",
                      not p1(gate({"native": {"issues": []}, "windows": {"issues": []}, "darwin": {"issues": [mac]}},
                                  suppress={("skills/acme/cli/adopt_darwin.go", "G115")}))))
        timeout = p1(gate({"native": {"issues": []}, "windows": {"issues": []}, "darwin": {"exit": 124}}))
        cases.append(("a darwin pass that times out fails closed, named by its target",
                      len(timeout) == 1 and timeout[0]["rule"] == "scanner-error"
                      and "GOOS=darwin" in timeout[0]["evidence"]))
        cases.append(("the same tree with the finding gone from the Windows pass is clean",
                      not p1(gate({"native": {"issues": [shared]}, "windows": {"issues": [shared]}}))))
        saved_targets = GOSEC_TARGETS
        try:
            globals()["GOSEC_TARGETS"] = (None,)
            cases.append(("(control) without the extra passes neither finding is seen: the old blind spot",
                          not p1(gate({"native": {"issues": [shared]}, "windows": {"issues": [shared, win]},
                                       "darwin": {"issues": [mac]}}))))
        finally:
            globals()["GOSEC_TARGETS"] = saved_targets
        timeout = p1(gate({"native": {"issues": []}, "windows": {"exit": 124}}))
        cases.append(("a Windows pass that times out fails closed, named by its target",
                      len(timeout) == 1 and timeout[0]["rule"] == "scanner-error"
                      and "GOOS=windows" in timeout[0]["evidence"]))
        garbage = p1(gate({"native": {"issues": []}, "windows": {"exit": 1, "out": "not json"}}))
        cases.append(("a Windows pass with unparseable output fails closed", len(garbage) == 1
                      and garbage[0]["rule"] == "scanner-error"))
        wrong_shape = p1(gate({"native": {"issues": []}, "windows": {"exit": 0, "out": "[]"}}))
        cases.append(("a Windows pass whose JSON is not gosec's shape fails closed", len(wrong_shape) == 1
                      and wrong_shape[0]["rule"] == "scanner-error"))
        # Codex r1 P1: an exit-1 run with no report used to read as clean ("out or {}").
        # Real gosec -quiet: clean = exit 0 with no output; findings or a load error =
        # exit 1 WITH a JSON report. Anything else did not complete.
        for label, case in (("exit 1 with no report", {"exit": 1, "out": ""}),
                            ("exit 2 with no report", {"exit": 2, "out": ""}),
                            ("exit 3 with a clean-looking report", {"exit": 3, "out": '{"Issues": []}'})):
            got = p1(gate({"native": {"issues": []}, "windows": case}))
            cases.append((f"a Windows pass with {label} fails closed",
                          [f["rule"] for f in got] == ["scanner-error"]))
            got = p1(gate({"native": case, "windows": {"issues": []}}))
            cases.append((f"a native pass with {label} fails closed", [f["rule"] for f in got] == ["scanner-error"]))
        cases.append(("a clean pass (exit 0, no output, as real gosec -quiet) is clean",
                      not p1(gate({"native": {"exit": 0, "out": ""}, "windows": {"exit": 0, "out": ""}}))))
        cases.append(("a failed native pass still fails closed with the Windows pass clean",
                      [f["rule"] for f in p1(gate({"native": {"exit": 124}, "windows": {"issues": []}}))]
                      == ["scanner-error"]))
        cases.append(("outside --require-scanners a failed pass is not a P1 (local runs)",
                      not p1(gate({"native": {"issues": []}, "windows": {"exit": 124}}, require=False))))
        cases.append(("a base-owned (file, rule) entry accepts a Windows-only finding",
                      not p1(gate({"native": {"issues": []}, "windows": {"issues": [win]}},
                                  suppress={("skills/acme/cli/open_windows.go", "G304")}))))
        hidden = p1(gate({"native": {"issues": []}, "windows": {"issues": [win[:5] + (True,)]}}))
        cases.append(("an annotation-hidden Windows-only finding with no entry or precedent is a P1",
                      len(hidden) == 1 and "inline annotation hid this" in hidden[0]["evidence"]))
        for name, ok in cases:
            print(f"  [{'PASS' if ok else 'FAIL'}] gosec targets: {name}")
            failed += not ok
    finally:
        REPO = saved_repo
        for k, v in saved_env.items():
            if v is None:
                os.environ.pop(k, None)
            else:
                os.environ[k] = v
        shutil.rmtree(tmp, ignore_errors=True)
    return failed


def self_test() -> int:
    failed = (_self_test_summary() + _self_test_workflows() + _self_test_changed_files()
              + _self_test_mcp_surface() + _self_test_precedent() + _self_test_line_directive()
              + _self_test_gosec_targets())
    if failed:
        print(f"\nSELF-TEST FAIL: {failed} case(s) behaved wrong.")
        return 1
    print("\nSELF-TEST PASS: the summary fails on every missing or non-passing row, both gates pin "
          "the same scanners with no cache, the diff scope sees quoted names and renames, each "
          "MCP-surface rule fires on its class and not on the healthy shape, the baseline "
          "only shrinks, fleet precedent accepts only annotated lines that ship unchanged "
          "on the trusted base, and gosec scans every build target in GOSEC_TARGETS.")
    return 0


def go_files(slug: str, only: list[Path] | None) -> list[Path]:
    if only is not None:
        return only
    return list((REPO / "skills" / slug / "cli").rglob("*.go"))


def script_files(slug: str, only: list[Path] | None) -> list[Path]:
    if only is not None:
        return only
    return [p for p in (REPO / "skills" / slug).rglob("*")
            if p.suffix in (".py", ".sh", ".ps1")]


def has_go_module(slug: str) -> bool:
    return (REPO / "skills" / slug / "cli" / "go.mod").is_file()


def gate_slug(slug: str, base: str | None, allow: set[str], suppress: set[tuple[str, str]],
              require: bool = False, baseline: dict[str, set[str]] | None = None,
              policy: list[dict] | None = None, precedent: FleetPrecedent | None = None) -> dict:
    only = changed_files(base, slug) if base else None
    findings = []
    findings += scan_go_patterns(go_files(slug, only), suppress)
    findings += scan_install_scripts(slug, only, suppress)
    # Always scan shipped scripts: a connector ships them too (bootstrap helpers),
    # and this is the ONLY coverage a markdown-only skill's code gets.
    findings += scan_python(script_files(slug, only), suppress)
    findings += scan_shell_scripts(slug, script_files(slug, only), suppress)

    # The Go toolchain lane, only for skills that HAVE a Go module. A markdown-only
    # skill has no skills/<slug>/cli/go.mod, and running gosec/govulncheck/osv-scanner
    # against that non-existent path made them fail-close: 4 permanent P1s on a file
    # that does not exist, unfixable by any contributor, while the Python that
    # actually ships went unread. False-RED and false-GREEN in the same verdict.
    accepted: list[dict] = []
    if has_go_module(slug):
        findings += scan_dependencies(slug, allow, only)
        findings += scan_external(slug, suppress, base, require, precedent, accepted)

    # MCP-surface rules judge the whole connector, in diff mode too (STATE rules).
    mcp, burned = scan_mcp_surface(slug, suppress, baseline or {})
    findings += mcp + list(policy or [])

    p1 = [f for f in findings if f["severity"] == "P1"]
    return {"slug": slug, "base": base, "scope": "diff" if only is not None else "full",
            "lane": "go" if has_go_module(slug) else "scripts",
            "findings": findings, "p1_count": len(p1), "precedented": accepted,
            "verdict": "block" if p1 else "pass", "burned_down": burned,
            "tools": {t: have(t) for t in ("gosec", "govulncheck", "osv-scanner", "semgrep")}}


def mcp_surface_main(args, suppress: set[tuple[str, str]], baseline: dict[str, set[str]],
                     policy: list[dict]) -> int:
    """--mcp-surface / --report: only the MCP-surface rules, for one slug or the fleet."""
    slugs = [args.slug] if args.slug and not args.all else list_slugs()
    results = []
    for s in slugs:
        f, burned = scan_mcp_surface(s, suppress, baseline)
        results.append({"slug": s, "findings": f, "burned_down": burned,
                        "p1_count": sum(x["severity"] == "P1" for x in f)})
    blocked = [r for r in results if r["p1_count"]] or policy
    if args.json:
        print(json.dumps({"results": results, "policy": policy}, indent=2))
    else:
        for f in policy:
            print(f"P1 {f['tool']}:{f['rule']} {f['file']}:{f['line']} - {f['evidence']}")
        for r in results:
            for f in r["findings"]:
                print(f"[{r['slug']}] {f['severity']} {f['tool']}:{f['rule']} {f['file']}:{f['line']} - "
                      f"{f['evidence']}")
            for rule in r["burned_down"]:
                print(f"[{r['slug']}] BURNED DOWN mcp-surface:{rule} - no longer fails it; "
                      "--update-baseline retires the entry")
        if args.report:
            mcp_report(results, baseline)
        print(f"\nmcp-surface: {len(slugs)} skill(s), "
              f"{sum(r['p1_count'] for r in results) + len(policy)} P1 (new failures or baseline problems)")
    return 1 if blocked else 0


def main() -> int:
    ap = argparse.ArgumentParser(description="Deterministic release-blocking security gate.")
    ap.add_argument("--slug")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--base", help="git ref to diff against (e.g. origin/main); omit for full audit")
    ap.add_argument("--policy-base", help="read dep_allowlist.json + security_suppressions.json + "
                                          "security_gate_baseline.json from this trusted ref instead of "
                                          "the (PR-controlled) working tree; the baseline in force is the "
                                          "base's entries the working copy still lists")
    ap.add_argument("--rebuild-allowlist", action="store_true")
    ap.add_argument("--require-scanners", action="store_true",
                    help="fail CLOSED (P1) if a required scanner (gosec/govulncheck/osv-scanner) "
                         "is missing or errors. Set this in CI/release so a broken scanner install "
                         "cannot silently disable the gate. Omit for local dev (graceful skip).")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--mcp-surface", action="store_true",
                    help="run only the MCP-surface rules (no scanners), for --slug or the fleet")
    ap.add_argument("--report", action="store_true",
                    help="MCP-surface fleet counts per rule: failing, baselined, new, burned down")
    ap.add_argument("--update-baseline", action="store_true",
                    help="remove every connector that no longer fails its rule from "
                         "security_gate_baseline.json (never adds one)")
    ap.add_argument("--seed-baseline", action="store_true",
                    help="give a rule with NO entry yet (a new rule) today's failing connectors; "
                         "existing entries are untouched")
    ap.add_argument("--self-test", action="store_true",
                    help="prove the workflow summary, the workflow pins and the diff scope both directions")
    ap.add_argument("--summarize", action="store_true",
                    help="security-gate summary job: read toJSON(needs) from $GATE_NEEDS and the "
                         "connector verdict files from --verdicts; fail unless every expected row passed")
    ap.add_argument("--verdicts", help="directory of verdict-<slug>.json files (--summarize)")
    ap.add_argument("--sha", help="commit the verdicts must be for (--summarize)")
    ap.add_argument("--run-id", help="workflow run the verdicts must be from (--summarize)")
    args = ap.parse_args()

    if args.rebuild_allowlist:
        return rebuild_allowlist()
    if args.self_test:
        return self_test()
    if args.summarize:
        try:
            needs = json.loads(os.environ.get("GATE_NEEDS") or "{}")
        except json.JSONDecodeError:
            needs = {}
        problems = summarize_gate(needs, Path(args.verdicts or "."), args.sha or "", args.run_id or "")
        for pr in problems:
            print(f"::error::{pr}")
        print("security-gate: every expected connector row ran and passed." if not problems
              else f"security-gate: {len(problems)} problem(s); failing closed.")
        return 1 if problems else 0

    allow = load_allowlist(args.policy_base)
    suppress = load_suppressions(args.policy_base)
    if args.update_baseline:
        return update_baseline(suppress)
    if args.seed_baseline:
        return seed_baseline(suppress)
    baseline, bl_problems, readded = effective_baseline(args.policy_base)
    policy = baseline_policy_findings(bl_problems, readded)
    if args.mcp_surface or args.report:
        return mcp_surface_main(args, suppress, baseline, policy)
    if not allow:
        print("WARNING: dep_allowlist.json empty/missing - run --rebuild-allowlist (after vetting).",
              file=sys.stderr)

    slugs = list_slugs() if args.all else ([args.slug] if args.slug else [])
    if not slugs:
        ap.error("one of --slug, --all, or --rebuild-allowlist is required")

    precedent = FleetPrecedent(args.policy_base)
    # A baseline problem is a policy failure, not a connector's: one row carries it.
    results = [gate_slug(s, args.base, allow, suppress, args.require_scanners, baseline,
                         policy if i == 0 else None, precedent) for i, s in enumerate(slugs)]
    blocked = [r for r in results if r["verdict"] == "block"]

    if args.json:
        print(json.dumps(results if args.all else results[0], indent=2))
    else:
        for r in results:
            print(f"[{'BLOCK' if r['verdict'] == 'block' else 'pass'}] {r['slug']} "
                  f"({r['scope']}, {r['lane']} lane): "
                  f"{r['p1_count']} P1 / {len(r['findings'])} findings")
            for f in r["findings"]:
                print(f"    {f['severity']} {f['tool']}:{f['rule']} {f['file']}:{f['line']} - {f['evidence']}")
            for rule in r.get("burned_down", []):
                print(f"    BURNED DOWN mcp-surface:{rule} - {r['slug']} no longer fails it; "
                      "--update-baseline retires the entry")
            if r.get("precedented"):
                print(f"    info: {len(r['precedented'])} annotation-hidden gosec finding(s) accepted by "
                      f"fleet precedent (the same annotated lines in >= {PRECEDENT_MIN} other connectors)")
        missing = [t for t, ok in results[0]["tools"].items() if not ok] if results else []
        if missing:
            print(f"\nNOTE: external scanners not installed locally: {', '.join(missing)} "
                  "(CI installs them; built-in deterministic checks still ran).")

    return 1 if blocked else 0


if __name__ == "__main__":
    raise SystemExit(main())
