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
    import "unsafe". (cobratree MCP→CLI bridge excluded; browser-open literal allowed.)
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

TWO LANES. A skill WITH skills/<slug>/cli/go.mod gets the Go lane (dependency policy +
gosec/govulncheck/osv-scanner/semgrep). A markdown-only skill has no Go module, so the
Go lane is skipped and the shipped-script scan is its coverage. Running the Go scanners
against a non-existent go.mod used to fail them CLOSED - 4 permanent P1s on a file that
does not exist, unfixable by any contributor - while the Python that actually ships went
completely unread. That was false-RED and false-GREEN in the same verdict.

Suppressions are BASE-OWNED, not self-grantable: a finding is suppressed only if its
(file, rule) is listed in tools/maintainer/security_suppressions.json. Inline `// #nosec`
is NOT honored (an attacker could add it in their own diff).

Template approvals are BASE-OWNED too (tools/maintainer/template_approvals.json): a
P1 in printing-press generated code is accepted without a per-connector suppression
PR only when the gate establishes, with tools/maintainer/declgraph (go/types), that
the flagged declaration, what it depends on and what feeds it are byte-identical to
the reviewed template on every release target, with the same resolved dependencies.
Anything not established stays a P1. See the "template approvals" section below.

Folded in when installed (CI installs them): gosec, govulncheck, osv-scanner, semgrep.
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
    check_security_gate.py --self-test                           # approvals + summary, both ways
    check_security_gate.py --slug bitdefender --emit-approval spec.json  # record from a REVIEWED tree
    check_security_gate.py --summarize --verdicts DIR --sha S --run-id N  # workflow summary job

Exit code: 0 = pass (no P1), 1 = block (>=1 P1), 2 = usage error.
"""
from __future__ import annotations

import argparse
import ast
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
    code, out, _ = run(["git", "-C", str(REPO), "diff", "--name-only", f"{base}...HEAD"])
    if code != 0:
        code, out, _ = run(["git", "-C", str(REPO), "diff", "--name-only", base])
        if code != 0:
            return None
    prefix = f"skills/{slug}/"
    return [REPO / f for f in out.splitlines() if f.startswith(prefix)]


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


def scan_external(slug: str, suppress: set[tuple[str, str]],
                  base: str | None = None, require: bool = False) -> list[dict]:
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
        code, out, err = run(["gosec", "-quiet", "-fmt", "json", "./..."], cwd=str(mod), timeout=600)
        gj = None
        try:
            gj = json.loads(out or "{}")
        except json.JSONDecodeError:
            gj = None
        if (gj is None or code in (124, 127)) and require:
            detail = "timeout" if code == 124 else "not-installed" if code == 127 else "unparseable output"
            add("gosec", "scanner-error", "P1", f"skills/{slug}/cli/go.mod", 0,
                f"gosec did not complete ({detail}, exit {code}) (fail-closed).")
        for iss in (gj or {}).get("Issues", []):
            sev = "P1" if iss.get("severity", "").upper() in ("HIGH", "MEDIUM") else "P2"
            add("gosec", iss.get("rule_id"), sev, iss.get("file", ""),
                iss.get("line", 0), iss.get("details", "")[:120])
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

# --- template approvals (decision 2 of the 2026-10-21 publish plan) ----------------
# A printing-press release renders the same generated code into every connector, and
# gosec/semgrep/builtin rules flag the same lines in each copy. Before this, every new
# connector needed its own base-policy suppression PR for code a human had already
# reviewed. A template approval applies that review automatically, but ONLY when a
# script establishes that the flagged code is the reviewed code:
#
#   * the finding matches exactly: tool, rule, module-relative file, and the
#     enclosing declaration (resolved from the finding's line, not from its text);
#   * every declaration in the approval's "slice" (the flagged declaration, what it
#     depends on, and what feeds it) is present and hashes identically. A hash
#     covers the declaration's bytes AND the resolved identity of everything it
#     references, as computed by go/types for the target the scanner saw, so a
#     shadowed builtin, a swapped import or a callee that moved all change it;
#   * the slice is closed: each slice declaration references only slice members or
#     members the approval EXCLUDES by name with a written reason; a slice type
#     gained no method outside the slice;
#   * nothing outside the slice calls or reads a slice function, method or
#     variable unless the approval lists it as an ENTRY with a written reason why
#     an outside caller cannot reach the finding (types and constants are immutable
#     and may be used freely);
#   * the module resolves the same third-party code: no `replace` directive, no
#     go.work, no vendor directory, the same go language version and godebug
#     settings, and every module the slice uses at the reviewed version with the
#     reviewed go.sum hashes;
#   * the connector declares the approval's printing-press version.
#
# The go directive's PATCH level and the `toolchain` line are deliberately not part
# of the fingerprint: they only set a toolchain floor (enforced separately by the
# GO_FLOOR guard in ci_guards.sh) and change fleet-wide on every Go security bump,
# which would void every approval without any change to the reviewed code or to the
# third-party code it resolves. The language version (go 1.N) IS fingerprinted,
# because it changes language semantics. Module versions and replace lines always
# invalidate an approval.
#
# Anything that cannot be established leaves the finding a P1, exactly as before:
# the ordinary base-policy suppression PR still applies. Approvals are read from the
# trusted --policy-base ref, never from the PR, as is the analyzer source they rely
# on. Control-plane findings (scanner-missing / scanner-error / osv-mapping-error)
# and every govulncheck, osv-scanner, dependency, install-script and shipped-script
# finding are never approvable.

APPROVALS = Path(__file__).resolve().parent / "template_approvals.json"
DECLGRAPH_DIR = Path(__file__).resolve().parent / "declgraph"
APPROVABLE_TOOLS = frozenset({"gosec", "builtin", "semgrep"})
# Unit kinds an outside declaration may not reference unless the approval names
# the unit as an entry. Types and constants are immutable; files and import blocks
# cannot be referenced.
GUARDED_KINDS = frozenset({"func", "method", "init", "var"})
RELEASE_TARGETS = tuple(f"{o}/{a}" for o, a in (("linux", "amd64"), ("linux", "arm64"),
                                                  ("darwin", "amd64"), ("darwin", "arm64"),
                                                  ("windows", "amd64"), ("windows", "arm64")))
PLACEHOLDERS = ("{MOD}", "{ENV}", "{SLUG}", "{SNAKE}")
_GO_ENV_BASE = {"GOWORK": "off", "GOFLAGS": "", "CGO_ENABLED": "0"}


def name_pairs(module_path: str, slug: str) -> list[tuple[str, str]]:
    """The connector-name substitutions declgraph applies before hashing (same order)."""
    env = slug.upper().replace("-", "_")
    snake = slug.replace("-", "_")
    pairs = [(module_path, "{MOD}"), (env, "{ENV}"), (slug, "{SLUG}")]
    if snake != slug:
        pairs.append((snake, "{SNAKE}"))
    return pairs


def normalize_name(s: str, pairs: list[tuple[str, str]]) -> str:
    for old, new in pairs:
        if old:
            s = s.replace(old, new)
    return s


def _approval_shape_ok(a) -> bool:
    """Structural validation. A malformed entry is ignored (it approves nothing)."""
    if not isinstance(a, dict):
        return False
    want = {"id": str, "press_version": str, "tool": str, "rule": str, "file": str,
            "module": str, "decls": list, "targets": list, "go_lang": str, "godebug": list,
            "modules": dict, "slice": dict, "exclude": dict, "entries": dict,
            "preconditions": list}
    for k, t in want.items():
        if not isinstance(a.get(k), t):
            return False
    if a["tool"] not in APPROVABLE_TOOLS or a["rule"] in CONTROL_RULES or not a["rule"]:
        return False
    if not a["slice"] or not a["decls"] or not a["targets"] or not a["preconditions"]:
        return False
    if not all(isinstance(x, str) and x.strip() for x in a["preconditions"]):
        return False
    if not all(isinstance(v, str) and len(v) == 64 for v in a["slice"].values()):
        return False
    if not set(a["decls"]) <= set(a["slice"]) or not set(a["entries"]) <= set(a["slice"]):
        return False
    if set(a["exclude"]) & set(a["slice"]):
        return False
    for reasons in (a["exclude"], a["entries"]):
        if not all(isinstance(v, str) and v.strip() for v in reasons.values()):
            return False
    if not set(a["targets"]) <= set(RELEASE_TARGETS):
        return False
    if not re.fullmatch(r"\d+\.\d+", a["go_lang"]):
        return False
    for m, sums in a["modules"].items():
        if "@" not in m or not isinstance(sums, dict) or set(sums) != {"zip", "mod"}:
            return False
    return True


def load_approvals(ref: str | None = None) -> list[dict]:
    txt = _policy_text(APPROVALS, ref)
    if not txt:
        return []
    try:
        data = json.loads(txt)
    except json.JSONDecodeError:
        return []
    raw = data.get("approvals", []) if isinstance(data, dict) else []
    return [a for a in raw if _approval_shape_ok(a)]


def gomod_facts(text: str) -> dict:
    """The go.mod facts an approval fingerprints, parsed the way the go command
    tokenizes directives (a paren may touch the keyword: `replace(`)."""
    facts = {"module": "", "go_lang": "", "godebug": [], "replace": False}
    block = None
    for raw in (text or "").splitlines():
        line = raw.split("//", 1)[0]
        if "=>" in line:
            facts["replace"] = True  # only replace directives use =>
        toks = line.replace("(", " ( ").replace(")", " ) ").split()
        if not toks:
            continue
        if block is not None:
            if toks[0] == ")":
                block = None
                continue
            directive, args = block, toks
        else:
            directive, args = toks[0], toks[1:]
            if args[:1] == ["("]:
                block = directive
                continue
        if directive == "replace":
            facts["replace"] = True
        elif directive == "module" and args:
            facts["module"] = args[0].strip('"')
        elif directive == "go" and args:
            m = re.match(r"(\d+)\.(\d+)", args[0])
            facts["go_lang"] = f"{m.group(1)}.{m.group(2)}" if m else "?"
        elif directive == "godebug" and args:
            facts["godebug"].append("".join(args))
    facts["godebug"].sort()
    return facts


def gosum_hashes(text: str) -> dict[str, dict[str, str]]:
    """{module@version: {"zip": h1, "mod": h1}} from go.sum text."""
    out: dict[str, dict[str, str]] = {}
    for line in (text or "").splitlines():
        parts = line.split()
        if len(parts) != 3:
            continue
        path, ver, h = parts
        kind = "zip"
        if ver.endswith("/go.mod"):
            ver, kind = ver[:-len("/go.mod")], "mod"
        out.setdefault(f"{path}@{ver}", {})[kind] = h
    return out


def module_context(mod_dir: Path, manifest: Path, repo_root: Path, slug: str) -> dict:
    gomod = mod_dir / "go.mod"
    gosum = mod_dir / "go.sum"
    ctx = {"facts": gomod_facts(gomod.read_text(errors="replace") if gomod.is_file() else ""),
           "gosum": gosum_hashes(gosum.read_text(errors="replace") if gosum.is_file() else ""),
           "vendor": (mod_dir / "vendor").exists(), "gowork": None, "press": []}
    ctx["module_shape"] = module_shape(ctx["facts"]["module"], slug)
    # The go command looks for go.work in the module directory and every parent.
    d = mod_dir.resolve()
    root = repo_root.resolve()
    while True:
        if (d / "go.work").exists():
            ctx["gowork"] = str(d / "go.work")
            break
        if d == root or d.parent == d:
            break
        d = d.parent
    try:
        m = json.loads(manifest.read_text(encoding="utf-8"))
        ctx["press"] = [m.get("printing_press_version")]
        prov = m.get("press_provenance")
        if isinstance(prov, dict) and "printing_press_version" in prov:
            ctx["press"].append(prov.get("printing_press_version"))
    except (OSError, ValueError, AttributeError):
        ctx["press"] = [None]
    return ctx


def module_shape(module_path: str, slug: str) -> str:
    """The module path with the connector's own name abstracted ("{SLUG}-pp-cli")."""
    return normalize_name(module_path, name_pairs("", slug)[1:])


def context_problem(a: dict, ctx: dict) -> str | None:
    facts = ctx["facts"]
    if ctx.get("module_shape") != a["module"]:
        return f"module path shape {ctx.get('module_shape')!r} != reviewed {a['module']!r}"
    if facts["replace"]:
        return "go.mod has a replace directive"
    if ctx["gowork"]:
        return f"a go.work file applies ({ctx['gowork']})"
    if ctx["vendor"]:
        return "the module has a vendor directory"
    if facts["go_lang"] != a["go_lang"]:
        return f"go language version {facts['go_lang'] or 'unset'} != reviewed {a['go_lang']}"
    if facts["godebug"] != sorted(a["godebug"]):
        return f"godebug {facts['godebug']} != reviewed {sorted(a['godebug'])}"
    if any(p != a["press_version"] for p in ctx["press"]):
        return f"declared press version {ctx['press']} != reviewed {a['press_version']}"
    for m, sums in a["modules"].items():
        got = ctx["gosum"].get(m)
        if got != sums:
            return f"go.sum for {m} does not carry the reviewed hashes"
    return None


class GraphRunner:
    """Builds the trusted declgraph once per toolchain and runs it per target."""

    def __init__(self, policy_ref: str | None, src_override: dict[str, str] | None = None):
        self.policy_ref = policy_ref
        self.src_override = src_override
        self.bins: dict[str, Path | None] = {}
        self.cache: dict[tuple[str, str], dict | None] = {}
        self.tmp: list[str] = []

    def close(self) -> None:
        for d in self.tmp:
            shutil.rmtree(d, ignore_errors=True)

    def _source(self) -> dict[str, str] | None:
        if self.src_override is not None:
            return self.src_override
        files = {}
        for name in ("go.mod", "main.go"):
            txt = _policy_text(DECLGRAPH_DIR / name, self.policy_ref)
            if txt is None:
                return None
            files[name] = txt
        return files

    def _toolchain(self, mod_dir: Path) -> str:
        code, out, _ = run(["go", "env", "GOVERSION"], cwd=str(mod_dir), env=_GO_ENV_BASE)
        return out.strip() if code == 0 else ""

    def _bin(self, toolchain: str) -> Path | None:
        if toolchain in self.bins:
            return self.bins[toolchain]
        src = self._source()
        b = None
        if src is not None and toolchain:
            d = tempfile.mkdtemp(prefix="declgraph-")
            self.tmp.append(d)
            for name, txt in src.items():
                (Path(d) / name).write_text(txt, encoding="utf-8")
            env = {**_GO_ENV_BASE, "GOTOOLCHAIN": toolchain}
            # Build for the host, whatever target the scan is for.
            hostenv = {k: v for k, v in os.environ.items() if k not in ("GOOS", "GOARCH")}
            try:
                p = subprocess.run(["go", "build", "-o", str(Path(d) / "declgraph"), "."], cwd=d,
                                   capture_output=True, text=True, timeout=600,
                                   env={**hostenv, **env})
                if p.returncode == 0:
                    b = Path(d) / "declgraph"
            except (OSError, subprocess.TimeoutExpired):
                b = None
        self.bins[toolchain] = b
        return b

    def graph(self, mod_dir: Path, slug: str, target: str) -> dict | None:
        key = (str(mod_dir), target)
        if key in self.cache:
            return self.cache[key]
        g = None
        toolchain = self._toolchain(mod_dir)
        b = self._bin(toolchain)
        if b is not None:
            goos, goarch = target.split("/")
            code, out, _ = run([str(b), "-slug", slug], cwd=str(mod_dir), timeout=900,
                               env={**_GO_ENV_BASE, "GOTOOLCHAIN": toolchain,
                                    "GOOS": goos, "GOARCH": goarch})
            if code == 0:
                try:
                    g = json.loads(out)
                except json.JSONDecodeError:
                    g = None
        self.cache[key] = g
        return g


def scanner_target(mod_dir: Path) -> str | None:
    code, out, _ = run(["go", "env", "GOOS", "GOARCH"], cwd=str(mod_dir), env=_GO_ENV_BASE)
    parts = out.split()
    return f"{parts[0]}/{parts[1]}" if code == 0 and len(parts) == 2 else None


def file_targets(mod_dir: Path, rel: str) -> set[str] | None:
    """Release targets whose build includes module-relative file `rel` (text scanners
    read every file, so their findings must hold on every target that compiles it)."""
    pkgdir = "./" + (str(Path(rel).parent) if str(Path(rel).parent) != "." else "")
    base = Path(rel).name
    hit: set[str] = set()
    for t in RELEASE_TARGETS:
        goos, goarch = t.split("/")
        code, out, _ = run(["go", "list", "-e", "-json=GoFiles", pkgdir], cwd=str(mod_dir),
                           timeout=300, env={**_GO_ENV_BASE, "GOOS": goos, "GOARCH": goarch})
        if code != 0:
            return None
        try:
            if base in (json.loads(out or "{}").get("GoFiles") or []):
                hit.add(t)
        except json.JSONDecodeError:
            return None
    return hit


def finding_lines(f: dict) -> tuple[int, int] | None:
    v = f.get("line")
    if isinstance(v, int) and not isinstance(v, bool) and v > 0:
        return v, v
    m = re.fullmatch(r"(\d+)(?:-(\d+))?", str(v or ""))
    if not m:
        return None
    a = int(m.group(1))
    b = int(m.group(2) or a)
    return (a, b) if 0 < a <= b else None


def unit_at(g: dict, nfile: str, lo: int, hi: int) -> str | None:
    """The innermost declaration containing lines lo..hi, or None if that is not
    exactly one declaration."""
    best: list[tuple[int, str]] = []
    for k, u in g.get("units", {}).items():
        if u.get("file") != nfile or u.get("kind") == "file":
            continue
        if u["start"] <= lo and hi <= u["end"]:
            best.append((u["end"] - u["start"], k))
    if not best:
        return None
    best.sort()
    if len(best) > 1 and best[0][0] == best[1][0]:
        return None
    return best[0][1]


def graph_problem(a: dict, g: dict | None, rel: str, lines: tuple[int, int]) -> str | None:
    if not g:
        return "the declaration graph could not be computed"
    if g.get("errors"):
        return f"declgraph: {str(g['errors'][0])[:160]}"
    nfile = g.get("files", {}).get(rel)
    if nfile != a["file"]:
        return f"file {rel} is not the reviewed {a['file']}"
    unit = unit_at(g, nfile, *lines)
    if unit not in a["decls"]:
        return f"finding sits in {unit or 'no single declaration'}, not in {a['decls']}"
    units, sl, excl = g.get("units", {}), a["slice"], a["exclude"]
    for k, h in sl.items():
        u = units.get(k)
        if u is None:
            return f"{k} is missing"
        if u.get("hash") != h:
            return f"{k} differs from the reviewed template"
    mods: set[str] = set()
    for k in sl:
        u = units[k]
        mods |= set(u.get("mods") or [])
        for r in u.get("refs") or []:
            if r not in sl and r not in excl:
                return f"{k} references {r}, outside the reviewed slice"
        if u.get("kind") == "type":
            for m in g.get("methods", {}).get(k) or []:
                if m not in sl:
                    return f"{k} has method {m}, outside the reviewed slice"
        if u.get("kind") in GUARDED_KINDS and k not in a["entries"]:
            for user in g.get("users", {}).get(k) or []:
                if user not in sl:
                    return f"{user} uses {k}, which only reviewed code may use"
    if mods != set(a["modules"]):
        return f"slice depends on modules {sorted(mods)}, reviewed {sorted(a['modules'])}"
    return None


def apply_template_approvals(slug: str, findings: list[dict], approvals: list[dict],
                             runner: GraphRunner, mod_dir: Path | None = None,
                             manifest: Path | None = None, repo_root: Path | None = None) -> None:
    """Mark P1 findings established as reviewed template code. In place; every
    finding that is not established stays exactly as it was."""
    if not approvals:
        return
    repo_root = repo_root or REPO
    mod_dir = mod_dir or (repo_root / "skills" / slug / "cli")
    manifest = manifest or (repo_root / "skills" / slug / "manifest.json")
    if not (mod_dir / "go.mod").is_file() or not re.fullmatch(r"[a-z0-9][a-z0-9-]*", slug):
        return
    prefix = f"skills/{slug}/cli/"
    facts = gomod_facts((mod_dir / "go.mod").read_text(errors="replace"))
    pairs = name_pairs(facts["module"], slug)
    ctx = None
    host: list[str | None] = []
    compiled: dict[str, set[str] | None] = {}
    for f in findings:
        if f.get("severity") != "P1" or f.get("tool") not in APPROVABLE_TOOLS \
                or f.get("rule") in CONTROL_RULES:
            continue
        file = str(f.get("file") or "")
        if not file.startswith(prefix):
            continue
        rel = file[len(prefix):]
        nfile = normalize_name(rel, pairs)
        cands = [a for a in approvals if a["tool"] == f["tool"] and a["rule"] == f["rule"]
                 and a["file"] == nfile]
        if not cands:
            continue
        lines = finding_lines(f)
        notes = []
        if ctx is None:
            ctx = module_context(mod_dir, manifest, repo_root, slug)
        if lines is None:
            notes.append("finding has no usable line")
            cands = []
        if f["tool"] == "gosec":
            if not host:
                host.append(scanner_target(mod_dir))
            targets = {host[0]} if host[0] else None
        elif cands:
            if rel not in compiled:
                compiled[rel] = file_targets(mod_dir, rel)
            targets = compiled[rel]
        else:
            targets = None
        approved = None
        for a in cands:
            why = context_problem(a, ctx)
            if why is None and not targets:
                why = "could not determine which build targets compile the file"
            if why is None and not targets <= set(a["targets"]):
                why = f"targets {sorted(targets)} not all reviewed ({a['targets']})"
            if why is None:
                # Every target the reviewed code ships on, not only the one the
                # scanner saw: a build-tag swap or a target-only file that redefines
                # or feeds a slice member must not reach any released binary.
                for t in sorted(a["targets"]):
                    why = graph_problem(a, runner.graph(mod_dir, slug, t), rel, lines)
                    if why:
                        why = f"[{t}] {why}"
                        break
            if why is None:
                approved = a
                break
            notes.append(f"{a['id']}: {why}")
        if approved:
            f["severity"] = "approved"
            f["approval"] = approved["id"]
            f["evidence"] = (f"{f.get('evidence', '')} [template approval {approved['id']}: "
                             f"identical to the reviewed press {approved['press_version']} template]")
        elif notes:
            f["approval_fallback"] = "; ".join(notes)[:600]


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


def slice_closure(g: dict, seeds: list[str], exclude: set[str]) -> set[str]:
    """Static closure from seeds: follow references, and from every type to all of its
    methods, stopping at excluded units."""
    units, seen, stack = g.get("units", {}), set(), list(seeds)
    while stack:
        k = stack.pop()
        if k in seen or k in exclude or k not in units:
            continue
        seen.add(k)
        stack += units[k].get("refs") or []
        if units[k].get("kind") == "type":
            stack += g.get("methods", {}).get(k) or []
    return seen


def build_approval(g_by_target: dict[str, dict], ctx: dict, *, ident: str, tool: str, rule: str,
                   rel: str, decls: list[str], seeds: list[str], exclude: dict[str, str],
                   entries: dict[str, str], preconditions: list[str]) -> tuple[dict | None, str]:
    """Record an approval from a REVIEWED connector tree. Returns (approval, report)."""
    targets = sorted(g_by_target)
    g0 = g_by_target[targets[0]]
    for t, g in g_by_target.items():
        if not g or g.get("errors"):
            return None, f"[{t}] declgraph failed: {(g or {}).get('errors')}"
    nfile = g0["files"].get(rel)
    if nfile is None:
        return None, f"{rel} is not a compiled file of this module on {targets[0]}"
    all_seeds = list(decls) + list(seeds)
    if any(g0["units"].get(d, {}).get("kind") == "imports" for d in decls):
        # A file-level finding: the whole file is the reviewed code.
        all_seeds += [k for k, u in g0["units"].items() if u["file"] == nfile]
    sl = slice_closure(g0, all_seeds, set(exclude))
    missing = [d for d in all_seeds if d not in sl and d not in exclude]
    if missing:
        return None, f"seeds not found: {missing}"
    for t, g in g_by_target.items():
        if slice_closure(g, all_seeds, set(exclude)) != sl:
            return None, f"[{t}] the slice differs from {targets[0]}'s"
        for k in sl:
            if g["units"][k]["hash"] != g0["units"][k]["hash"]:
                return None, f"[{t}] {k} hashes differently from {targets[0]}"
    lines = []
    for k in sorted(sl):
        u = g0["units"][k]
        if u["kind"] in GUARDED_KINDS:
            outside = sorted(set().union(*[set(g.get("users", {}).get(k) or []) for g in g_by_target.values()]) - sl)
            if outside and k not in entries:
                lines.append(f"NEEDS ENTRY  {k}  <- {outside}")
    excluded_refs = sorted({r for k in sl for r in (g0["units"][k].get("refs") or []) if r not in sl})
    unknown = [r for r in excluded_refs if r not in exclude]
    if unknown:
        return None, f"slice references units that are neither in it nor excluded: {unknown}"
    mods = sorted(set().union(*[set(g0["units"][k].get("mods") or []) for k in sl]))
    a = {"id": ident, "press_version": (ctx["press"] or [None])[0], "tool": tool, "rule": rule,
         "file": nfile, "module": ctx["module_shape"], "decls": list(decls), "targets": targets,
         "go_lang": ctx["facts"]["go_lang"], "godebug": ctx["facts"]["godebug"],
         "modules": {m: ctx["gosum"].get(m, {}) for m in mods},
         "slice": {k: g0["units"][k]["hash"] for k in sorted(sl)},
         "exclude": dict(sorted(exclude.items())), "entries": dict(sorted(entries.items())),
         "preconditions": list(preconditions)}
    if lines:
        return None, "\n".join(lines)
    if not _approval_shape_ok(a):
        return None, "recorded approval failed its own shape check"
    return a, f"slice of {len(sl)} declarations on {targets}; excluded refs {excluded_refs}"


def emit_approval(args) -> int:
    """--emit-approval: print one approval recorded from a reviewed connector."""
    slug = args.slug
    mod_dir = REPO / "skills" / slug / "cli"
    spec = json.loads(Path(args.emit_approval).read_text(encoding="utf-8"))
    runner = GraphRunner(None)
    try:
        graphs = {t: runner.graph(mod_dir, slug, t) for t in spec["targets"]}
        ctx = module_context(mod_dir, REPO / "skills" / slug / "manifest.json", REPO, slug)
        a, report = build_approval(graphs, ctx, ident=spec["id"], tool=spec["tool"], rule=spec["rule"],
                                   rel=spec["file"], decls=spec["decls"], seeds=spec.get("seeds", []),
                                   exclude=spec.get("exclude", {}), entries=spec.get("entries", {}),
                                   preconditions=spec.get("preconditions", []))
    finally:
        runner.close()
    print(report, file=sys.stderr)
    if a is None:
        return 1
    if spec.get("reviewed"):
        a["reviewed"] = spec["reviewed"]
    print(json.dumps(a, indent=1, ensure_ascii=True))
    return 0


# --- self-test ---------------------------------------------------------------------
# A fixture connector small enough to read: one module, stdlib only, rendered for
# two slugs. "alpha" is the reviewed template, "beta" the new connector under test.
_FIX = {
    "go.mod": "module {slug}-pp-cli\n\ngo 1.22\n",
    "cmd/{slug}-mcp/main.go": """package main

import (
	"fmt"
	"os"

	"{slug}-pp-cli/internal/store"
)

const (
	defaultAddr     = "127.0.0.1:7777"
	httpTokenEnvVar = "{ENV}_MCP_HTTP_TOKEN"
)

func main() {
	fmt.Println(defaultAddr, os.Getenv(httpTokenEnvVar), store.Open())
}
""",
    "internal/store/store.go": """package store

import (
	"sort"
	"strings"
)

// Spec-derived: differs per connector, and is reached only through bareWhere.
var resourceCols = map[string][]string{RESOURCE_COLS}

const batchSize = 64

type queryParts struct {
	q    string
	args []any
}

func bareWhere(types []string) string {
	q := make([]string, len(types))
	for i, t := range types {
		q[i] = "'" + strings.ReplaceAll(t, "'", "''") + "'"
	}
	return "resource_type IN (" + strings.Join(q, ", ") + ")"
}

func limitClause() string { return " LIMIT ?" }

func loadBatch(where, after string) queryParts {
	p := queryParts{q: "SELECT id FROM resources WHERE " + where}
	if after != "" {
		p.q += " AND id > ?"
		p.args = append(p.args, after)
	}
	p.q += limitClause()
	p.args = append(p.args, batchSize)
	return p
}

type Store struct{ name string }

func (s *Store) migrate() string {
	types := make([]string, 0, len(resourceCols))
	for t := range resourceCols {
		types = append(types, t)
	}
	sort.Strings(types)
	return loadBatch(bareWhere(types), "").q
}

func Open() string {
	s := &Store{name: "{slug}-cli"}
	return s.migrate()
}
""",
    "internal/store/extra.go": """package store

// Hand-written connector code that never touches the reviewed slice.
func Count(xs []string) int { return len(xs) }
""",
    "internal/store/raw.go": """//go:build {GOOS} && {GOARCH}

package store

import "unsafe"

func wordSize() int {
	var x uintptr
	return int(unsafe.Sizeof(x))
}

var _ = wordSize
""",
}


def _render_fixture(root: Path, slug: str, cols: str, goos: str, goarch: str) -> Path:
    mod = root / "skills" / slug / "cli"
    env = slug.upper().replace("-", "_")
    for rel, txt in _FIX.items():
        p = mod / rel.replace("{slug}", slug)
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(txt.replace("{slug}", slug).replace("{ENV}", env).replace("RESOURCE_COLS", cols)
                     .replace("{GOOS}", goos).replace("{GOARCH}", goarch), encoding="utf-8")
    (root / "skills" / slug / "manifest.json").write_text(json.dumps(
        {"printing_press_version": "9.9.9", "press_provenance": {"printing_press_version": "9.9.9"}}),
        encoding="utf-8")
    return mod


def _line_of(path: Path, needle: str) -> int:
    for i, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        if needle in line:
            return i
    raise AssertionError(f"{needle!r} not in {path}")


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


SCANNER_PINS = ("GOSEC_VERSION", "GOVULNCHECK_VERSION", "OSV_SCANNER_VERSION", "SEMGREP_VERSION")
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
    ]
    for name, g, r, want in cases:
        probs = workflow_problems(g, r)
        got = not probs
        print(f"  [{'PASS' if got == want else 'FAIL'}] workflows: {name} -> "
              f"{'pass' if got else 'fail: ' + probs[0]}")
        failed += got != want
    return failed


def self_test() -> int:
    failed = _self_test_summary() + _self_test_workflows()
    tmp = Path(tempfile.mkdtemp(prefix="gate-approvals-"))
    runner = GraphRunner(None)
    try:
        host = scanner_target(Path(tempfile.gettempdir()))
        if not host:
            print("SELF-TEST FAIL: go is not available")
            return 1
        goos, goarch = host.split("/")
        tpl_root = tmp / "template"
        tpl = _render_fixture(tpl_root, "alpha", '"alpha_items": {"parent_id"}', goos, goarch)
        g = {host: runner.graph(tpl, "alpha", host)}
        ctx = module_context(tpl, tpl_root / "skills" / "alpha" / "manifest.json", tpl_root, "alpha")
        M = "{MOD}"
        specs = [
            dict(ident="fix/g202", tool="gosec", rule="G202", rel="internal/store/store.go",
                 decls=[f"func {M}/internal/store.loadBatch"],
                 seeds=[f"method {M}/internal/store.Store.migrate", f"func {M}/internal/store.bareWhere"],
                 exclude={f"var {M}/internal/store.resourceCols": "spec data; reaches SQL only via bareWhere",
                          f"type {M}/internal/store.Store": "receiver only; migrate never reads it"},
                 entries={f"method {M}/internal/store.Store.migrate": "takes no input that reaches the query"},
                 preconditions=["bareWhere doubles every quote"]),
            dict(ident="fix/g101", tool="gosec", rule="G101", rel="cmd/alpha-mcp/main.go",
                 decls=[f"const {M}/cmd/{{SLUG}}-mcp.httpTokenEnvVar"], seeds=[], exclude={}, entries={},
                 preconditions=["the value is an environment variable name"]),
            dict(ident="fix/unsafe", tool="builtin", rule="unsafe-import", rel="internal/store/raw.go",
                 decls=[f"imports {M}/internal/store@internal/store/raw.go"], seeds=[], exclude={},
                 entries={}, preconditions=["unsafe only sizes a local word"]),
        ]
        approvals = []
        for sp in specs:
            a, report = build_approval(g, ctx, **sp)
            if a is None:
                print(f"SELF-TEST FAIL: could not record {sp['ident']}: {report}")
                return 1
            approvals.append(a)
        bad = dict(approvals[0], rule="scanner-error")
        print(f"  [{'PASS' if not _approval_shape_ok(bad) else 'FAIL'}] an approval for a "
              "control-plane rule is refused at load")
        failed += _approval_shape_ok(bad)

        def findings_for(mod: Path, slug: str, where: dict[str, str] | None = None) -> list[dict]:
            where = where or {}
            st = mod / "internal/store/store.go"
            out = [
                {"tool": "gosec", "rule": "G202", "severity": "P1",
                 "file": f"skills/{slug}/cli/internal/store/store.go",
                 "line": _line_of(st, where.get("G202", 'q: "SELECT id FROM resources WHERE " + where')),
                 "evidence": "SQL string concatenation"},
                {"tool": "gosec", "rule": "G101", "severity": "P1",
                 "file": f"skills/{slug}/cli/cmd/{slug}-mcp/main.go",
                 "line": _line_of(mod / f"cmd/{slug}-mcp/main.go", "httpTokenEnvVar ="),
                 "evidence": "Potential hardcoded credentials"},
                {"tool": "builtin", "rule": "unsafe-import", "severity": "P1",
                 "file": f"skills/{slug}/cli/internal/store/raw.go",
                 "line": _line_of(mod / "internal/store/raw.go", '"unsafe"'), "evidence": '"unsafe"'},
                {"tool": "gosec", "rule": "scanner-error", "severity": "P1",
                 "file": f"skills/{slug}/cli/go.mod", "line": 0, "evidence": "gosec did not complete"},
            ]
            return out

        def edit(mod: Path, rel: str, old: str, new: str) -> None:
            p = mod / rel
            t = p.read_text(encoding="utf-8")
            if old not in t:
                raise AssertionError(f"fixture edit anchor missing: {old!r}")
            p.write_text(t.replace(old, new, 1), encoding="utf-8")

        def case(name, mutate, want: dict[str, str], approvals_=None, where=None):
            nonlocal failed
            root = tmp / f"case{case.n}"
            case.n += 1
            mod = _render_fixture(root, "beta", '"beta_hosts": {"site_id"}, "beta_x": {"y"}', goos, goarch)
            mutate(root, mod)
            fs = findings_for(mod, "beta", where)
            apply_template_approvals("beta", fs, approvals_ or approvals, runner,
                                     mod_dir=mod, manifest=root / "skills/beta/manifest.json",
                                     repo_root=root)
            got = {f["rule"]: f["severity"] for f in fs}
            ok = all(got.get(r) == s for r, s in want.items()) and got.get("scanner-error") == "P1"
            why = next((f.get("approval_fallback") for f in fs if f.get("approval_fallback")), "")
            print(f"  [{'PASS' if ok else 'FAIL'}] {name}: {got}" + (f"  ({why[:150]})" if why else ""))
            failed += not ok
        case.n = 0
        nothing = lambda root, mod: None  # noqa: E731
        AP, P1 = "approved", "P1"
        case("unmodified template copy (other slug, other spec data)", nothing,
             {"G202": AP, "G101": AP, "unsafe-import": AP})
        case("changed flagged function", lambda r, m: edit(m, "internal/store/store.go",
             '" AND id > ?"', '" AND id > ? OR 1=1"'), {"G202": P1, "G101": AP, "unsafe-import": AP})
        case("changed callee", lambda r, m: edit(m, "internal/store/store.go",
             'return " LIMIT ?"', 'return " LIMIT ?; DROP TABLE resources"'), {"G202": P1})
        case("changed caller that feeds it", lambda r, m: edit(m, "internal/store/store.go",
             "loadBatch(bareWhere(types), \"\")", "loadBatch(bareWhere(types)+\" OR 1=1\", \"\")"),
             {"G202": P1})
        case("new outside caller feeding the flagged function", lambda r, m: edit(m, "internal/store/extra.go",
             "func Count", "func Raw(w string) string { return loadBatch(w, \"\").q }\n\nfunc Count"),
             {"G202": P1, "G101": AP})
        case("shadowed builtin in another file", lambda r, m: edit(m, "internal/store/extra.go",
             "func Count(xs []string) int { return len(xs) }",
             "func Count(xs []string) int { return 0 }\n\nfunc len(x any) int { return 1 << 20 }"),
             {"G202": P1})
        case("method added to a reviewed type", lambda r, m: edit(m, "internal/store/extra.go",
             "func Count", "func (p queryParts) String() string { return p.q }\n\nfunc Count"),
             {"G202": P1, "G101": AP})
        case("import swapped under an identical body", lambda r, m: (
             (m / "internal/strs").mkdir(), (m / "internal/strs/strs.go").write_text(
                 "package strs\n\nfunc ReplaceAll(s, o, n string) string { return s }\n\n"
                 "func Join(xs []string, sep string) string { return \"1=1\" }\n", encoding="utf-8"),
             edit(m, "internal/store/store.go", '"strings"', 'strings "beta-pp-cli/internal/strs"')),
             {"G202": P1})
        case("replace line in go.mod", lambda r, m: (m / "go.mod").write_text(
             "module beta-pp-cli\n\ngo 1.22\n\nreplace example.com/a => example.com/b v1.0.0\n",
             encoding="utf-8"), {"G202": P1, "G101": P1, "unsafe-import": P1})
        case("go.work file", lambda r, m: (r / "go.work").write_text("go 1.22\n\nuse ./skills/beta/cli\n",
             encoding="utf-8"), {"G202": P1, "G101": P1, "unsafe-import": P1})
        case("vendor directory", lambda r, m: (m / "vendor").mkdir(), {"G202": P1, "G101": P1})
        case("different press version", lambda r, m: (r / "skills/beta/manifest.json").write_text(
             json.dumps({"printing_press_version": "9.9.8"}), encoding="utf-8"),
             {"G202": P1, "G101": P1, "unsafe-import": P1})
        case("module path that is not <slug>-pp-cli", lambda r, m: edit(m, "go.mod", "module beta-pp-cli",
             "module example.com/beta-pp-cli"), {"G202": P1, "G101": P1})
        case("different go language version", lambda r, m: edit(m, "go.mod", "go 1.22", "go 1.23"),
             {"G202": P1, "G101": P1})
        case("same go language, different patch level (toolchain floor only)",
             lambda r, m: edit(m, "go.mod", "go 1.22", "go 1.22.9\n\ntoolchain go1.22.12"),
             {"G202": AP, "G101": AP})
        case("same rule in a different function", nothing, {"G202": P1, "G101": AP},
             where={"G202": "return \"resource_type IN (\" + strings.Join(q"})
        case("credential-looking value in the approved constant", lambda r, m: edit(
             m, "cmd/beta-mcp/main.go", '"BETA_MCP_HTTP_TOKEN"', '"sk_live_0123456789abcdef"'),
             {"G101": P1, "G202": AP})
        case("second unsafe use added to the approved file", lambda r, m: edit(
             m, "internal/store/raw.go", "var _ = wordSize",
             "var _ = wordSize\n\nfunc peek(p uintptr) byte { return *(*byte)(unsafe.Pointer(p)) }"),
             {"unsafe-import": P1, "G202": AP})
        case("line directive relocating a finding from new code into the approved function",
             lambda r, m: edit(m, "internal/store/store.go", "type Store struct",
                               "func evil(w string) string {\n//line store.go:%d\n\treturn \"SELECT \" + w\n}\n\n"
                               "type Store struct" % _line_of(m / "internal/store/store.go",
                                                              'q: "SELECT id FROM resources WHERE " + where')),
             {"G202": P1})
        case("normalization placeholder planted in source", lambda r, m: edit(
             m, "internal/store/store.go", "func limitClause", "// {SLUG}\nfunc limitClause"),
             {"G202": P1})
        dep = dict(approvals[0], modules={"golang.org/x/sys@v0.46.0": {"zip": "h1:a", "mod": "h1:b"}})
        case("reviewed module version absent from go.sum (dependency changed)",
             lambda r, m: (m / "go.sum").write_text(
                 "golang.org/x/sys v0.48.0 h1:a\ngolang.org/x/sys v0.48.0/go.mod h1:b\n", encoding="utf-8"),
             {"G202": P1}, approvals_=[dep])
    finally:
        runner.close()
        shutil.rmtree(tmp, ignore_errors=True)
    if failed:
        print(f"\nSELF-TEST FAIL: {failed} case(s) behaved wrong.")
        return 1
    print("\nSELF-TEST PASS: approvals apply only to unmodified template code; every listed change "
          "falls back to a P1; the summary fails on every missing or non-passing row.")
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
              require: bool = False, approvals: list[dict] | None = None,
              runner: GraphRunner | None = None) -> dict:
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
    if has_go_module(slug):
        findings += scan_dependencies(slug, allow, only)
        findings += scan_external(slug, suppress, base, require)
        if approvals and runner is not None:
            # Base-owned template approvals: established mechanically or not at all.
            apply_template_approvals(slug, findings, approvals, runner)

    p1 = [f for f in findings if f["severity"] == "P1"]
    return {"slug": slug, "base": base, "scope": "diff" if only is not None else "full",
            "lane": "go" if has_go_module(slug) else "scripts",
            "findings": findings, "p1_count": len(p1),
            "verdict": "block" if p1 else "pass",
            "tools": {t: have(t) for t in ("gosec", "govulncheck", "osv-scanner", "semgrep")}}


def main() -> int:
    ap = argparse.ArgumentParser(description="Deterministic release-blocking security gate.")
    ap.add_argument("--slug")
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--base", help="git ref to diff against (e.g. origin/main); omit for full audit")
    ap.add_argument("--policy-base", help="read dep_allowlist.json + security_suppressions.json from "
                                          "this trusted ref instead of the (PR-controlled) working tree")
    ap.add_argument("--rebuild-allowlist", action="store_true")
    ap.add_argument("--require-scanners", action="store_true",
                    help="fail CLOSED (P1) if a required scanner (gosec/govulncheck/osv-scanner) "
                         "is missing or errors. Set this in CI/release so a broken scanner install "
                         "cannot silently disable the gate. Omit for local dev (graceful skip).")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--self-test", action="store_true",
                    help="prove template approvals and the workflow summary both directions, then exit")
    ap.add_argument("--emit-approval", metavar="SPEC.json",
                    help="record one template approval from the REVIEWED --slug tree and print it")
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
    if args.emit_approval:
        if not args.slug:
            ap.error("--emit-approval needs --slug")
        return emit_approval(args)

    allow = load_allowlist(args.policy_base)
    suppress = load_suppressions(args.policy_base)
    approvals = load_approvals(args.policy_base)
    runner = GraphRunner(args.policy_base)
    if not allow:
        print("WARNING: dep_allowlist.json empty/missing - run --rebuild-allowlist (after vetting).",
              file=sys.stderr)

    slugs = list_slugs() if args.all else ([args.slug] if args.slug else [])
    if not slugs:
        ap.error("one of --slug, --all, or --rebuild-allowlist is required")

    try:
        results = [gate_slug(s, args.base, allow, suppress, args.require_scanners, approvals, runner)
                   for s in slugs]
    finally:
        runner.close()
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
                if f.get("approval_fallback"):
                    print(f"        no template approval: {f['approval_fallback']}")
        missing = [t for t, ok in results[0]["tools"].items() if not ok] if results else []
        if missing:
            print(f"\nNOTE: external scanners not installed locally: {', '.join(missing)} "
                  "(CI installs them; built-in deterministic checks still ran).")

    return 1 if blocked else 0


if __name__ == "__main__":
    raise SystemExit(main())
