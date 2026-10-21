#!/usr/bin/env python3
"""Gate: no destructive command answers to the name `create`.

Why this exists
---------------
cli-printing-press gives a generated endpoint command the Cobra alias `create`
when it is a POST. That is right for `tickets create-ticket`. It is wrong for a
POST that deletes, cancels, purges, disables or reboots something: on
origin/main `maxio-cli subscriptions purge-json create <id> --ack 1` purges a
subscription, and `sentinelone-cli reports create` deletes reports. An agent
that guesses the conventional verb `create` gets the destructive call instead,
and nothing in the help text it read said so. The alias is hidden from the
command list, so no reviewer sees it either.

What it checks (FAIL)
---------------------
Every command in a connector's real Cobra tree that carries the alias `create`
and whose action is destructive. The action is judged from three places:

  * the HTTP method in the command's `pp:method` annotation (DELETE);
  * the command's own name (`purge-subscription`, `bulk-delete`);
  * the last literal segment of its `pp:path` annotation
    (`/invoices/{uid}/void.json` gives `void`; `/devices/{serial}/reboot`
    gives `reboot` for `devices reboot device`). Resource and group names are
    nouns and are not read, so `quarantine export` is not a quarantine and
    `uninstall-password generate-regenerate` is not an uninstall. A command
    without `pp:path` (hand-written) is judged by its path below the
    top-level resource.

A word counts when it equals one of DESTRUCTIVE_WORDS after splitting on
punctuation and camelCase. The list is deliberately explicit (no prefix match)
so a noun such as `blocks` is not read as the verb `block`.

What it reports (never fails)
-----------------------------
Command-local flags that shadow a persistent flag inherited from an ancestor,
for example a local `--agent` string flag that silently replaces the global
`--agent` agent-mode switch on that one command. These are listed as WARN lines
and do not change the exit code.

Where the tree comes from
-------------------------
Neither source text nor the shipped `agent-context` JSON is reliable for this:

  * `agent-context` does not emit aliases at all (its command struct has no
    alias field), so it cannot see the defect.
  * The source is rewritten at runtime: `platform_client.go`
    (`attachPlatformClientCommands`) prunes aliases that collide with the
    platform `client` commands, and novel files such as auvik's
    `auvik_friendly.go` clone commands and clear or rewrite their aliases.

So this check compiles each connector's own `internal/cli` package and walks
the tree that `cli.RootCmd()` returns, which is the same tree the CLI executes
and the MCP server mirrors. The walker is a tiny Go program supplied through
`go build -overlay`, so nothing is ever written into `skills/<slug>/cli`
(the CLI hash and the working tree stay untouched).

Usage:
    python3 tools/maintainer/check_command_tree.py                 # every connector
    python3 tools/maintainer/check_command_tree.py --slug maxio     # one (repeatable)
    python3 tools/maintainer/check_command_tree.py --json out.json  # machine output
    python3 tools/maintainer/check_command_tree.py --self-test      # classifier proof

Exit 0 = no destructive `create` alias anywhere checked; 1 = at least one, or a
connector whose tree could not be read (a check that cannot look must not pass).
Pure stdlib; needs `go` on PATH.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
SKILLS = REPO / "skills"

# The alias this gate is about. Cobra matches aliases case-sensitively unless
# EnableCaseInsensitive is set, so compare lowercased to be safe either way.
CREATE_ALIAS = "create"

# Destructive or service-disrupting actions. An agent asking for `create` must
# never land on one of these. Exact words only; see the module docstring.
DESTRUCTIVE_WORDS = frozenset(
    {
        "abort",
        "archive",
        "block",
        "cancel",
        "cancellation",
        "clear",
        "deactivate",
        "deactivation",
        "decommission",
        "delete",
        "deletion",
        "deprovision",
        "destroy",
        "detach",
        "disable",
        "erase",
        "evict",
        "expire",
        "flush",
        "interrupt",
        "isolate",
        "kill",
        "lock",
        "offboard",
        "prune",
        "purge",
        "reboot",
        "removal",
        "remove",
        "reset",
        "resetall",
        "restart",
        "retire",
        "revert",
        "revoke",
        "shutdown",
        "stop",
        "suspend",
        "terminate",
        "termination",
        "truncate",
        "unassign",
        "unenroll",
        "uninstall",
        "unlink",
        "unregister",
        "void",
        "wipe",
    }
)

DESTRUCTIVE_METHODS = frozenset({"DELETE"})

# The overlay package path inside each module. It never exists on disk.
DUMP_PKG = "internal/zz_check_command_tree_dump"

DUMP_GO = r"""// Command-tree dump for tools/maintainer/check_command_tree.py.
// Supplied through `go build -overlay`; never written into the connector.
package main

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cli "__MODULE__/internal/cli"
)

type shadow struct {
	Flag  string   `json:"flag"`
	Owner []string `json:"owner"`
}

type node struct {
	Path        []string          `json:"path"`
	Aliases     []string          `json:"aliases"`
	Hidden      bool              `json:"hidden"`
	Runnable    bool              `json:"runnable"`
	Annotations map[string]string `json:"annotations"`
	Shadows     []shadow          `json:"shadows"`
}

type inherited struct {
	flag  *pflag.Flag
	owner []string
}

func walk(c *cobra.Command, path []string, persistent map[string]inherited, out *[]node) {
	for _, sub := range c.Commands() {
		p := append(append([]string{}, path...), sub.Name())
		n := node{
			Path:        p,
			Aliases:     append([]string{}, sub.Aliases...),
			Hidden:      sub.Hidden,
			Runnable:    sub.Runnable(),
			Annotations: sub.Annotations,
		}
		seen := map[string]bool{}
		check := func(f *pflag.Flag) {
			if seen[f.Name] {
				return
			}
			seen[f.Name] = true
			// A different *pflag.Flag under an inherited name is a shadow: Cobra
			// keeps the command's own flag and never merges the ancestor's.
			if anc, ok := persistent[f.Name]; ok && anc.flag != f {
				n.Shadows = append(n.Shadows, shadow{Flag: f.Name, Owner: anc.owner})
			}
		}
		sub.Flags().VisitAll(check)
		sub.PersistentFlags().VisitAll(check)
		*out = append(*out, n)

		next := make(map[string]inherited, len(persistent))
		for k, v := range persistent {
			next[k] = v
		}
		sub.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			if _, ok := next[f.Name]; !ok {
				next[f.Name] = inherited{flag: f, owner: p}
			}
		})
		walk(sub, p, next, out)
	}
}

func main() {
	root := cli.RootCmd()
	persistent := map[string]inherited{}
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		persistent[f.Name] = inherited{flag: f, owner: []string{}}
	})
	out := []node{}
	walk(root, nil, persistent, &out)
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		os.Exit(1)
	}
}
"""

_CAMEL = re.compile(r"(?<=[a-z0-9])(?=[A-Z])")
_SPLIT = re.compile(r"[^a-z0-9]+")


def words(text: str) -> set[str]:
    """Lowercase words of a command name or path segment (camelCase aware)."""
    return {w for w in _SPLIT.split(_CAMEL.sub(" ", text or "").lower()) if w}


def last_literal_segment(api_path: str) -> str:
    segs = [s for s in (api_path or "").split("/") if s and not s.startswith("{")]
    return segs[-1] if segs else ""


def action_words(node: dict) -> set[str]:
    """The words that name what this command DOES.

    An endpoint command's action is its own name plus the last literal segment
    of its API path. Intermediate group names are not read for endpoints: the
    generator derives them from path segments that are often nouns, so
    `accounts uninstall-password generate-regenerate` (POST
    .../uninstall-password/generate) generates a password and must not read as
    `uninstall`. When the group IS the action (`devices reboot device`, POST
    /devices/{serial}/reboot) the last path literal already carries it. A
    command with no `pp:path` (a hand-written one) falls back to its whole path
    below the top-level resource.
    """
    path = node.get("path") or []
    ann = node.get("annotations") or {}
    api_path = ann.get("pp:path", "")
    out: set[str] = set()
    if api_path:
        leaf = words(path[-1] if path else "")
        out |= leaf
        # A leaf that names itself a create (`contract-child-create-entity`,
        # POST /Contracts/{parentId}/BlockHourFactors) is a create; the path
        # literal is then a resource noun (`BlockHourFactors`), not an action.
        # A destructive word in the leaf itself (`create-cancel-subscription`)
        # still counts, because `out` already holds the leaf's words.
        if CREATE_ALIAS not in leaf:
            out |= words(last_literal_segment(api_path))
    else:
        for part in (path[1:] if len(path) > 1 else path):
            out |= words(part)
    return out


def destructive_reason(node: dict) -> str | None:
    """Why this command's action is destructive, or None."""
    ann = node.get("annotations") or {}
    method = (ann.get("pp:method") or "").upper()
    if method in DESTRUCTIVE_METHODS:
        return f"HTTP {method}"
    hit = sorted(action_words(node) & DESTRUCTIVE_WORDS)
    if hit:
        return "action word " + ", ".join(repr(w) for w in hit)
    return None


def has_create_alias(node: dict) -> bool:
    return any((a or "").lower() == CREATE_ALIAS for a in node.get("aliases") or [])


def findings_for(slug: str, nodes: list[dict]) -> tuple[list[dict], list[dict]]:
    """Return (create_alias_failures, flag_shadow_reports) for one connector."""
    fails, shadows = [], []
    seen: set[str] = set()
    for node in nodes:
        # A generator defect can register the same group twice (ninjaone
        # `document-templates archive`); report each command path once.
        key = " ".join(node.get("path") or [])
        if key in seen:
            continue
        seen.add(key)
        if has_create_alias(node):
            reason = destructive_reason(node)
            if reason:
                ann = node.get("annotations") or {}
                fails.append(
                    {
                        "slug": slug,
                        "command": " ".join(node["path"]),
                        "resolves_as": " ".join(node["path"][:-1] + [CREATE_ALIAS]),
                        "method": ann.get("pp:method", ""),
                        "api_path": ann.get("pp:path", ""),
                        "reason": reason,
                    }
                )
        for sh in node.get("shadows") or []:
            shadows.append(
                {
                    "slug": slug,
                    "command": " ".join(node["path"]),
                    "flag": sh["flag"],
                    "shadows": "root" if not sh["owner"] else " ".join(sh["owner"]),
                }
            )
    return fails, shadows


def module_path(cli_dir: Path) -> str:
    for line in (cli_dir / "go.mod").read_text(encoding="utf-8").splitlines():
        if line.startswith("module "):
            return line.split(None, 1)[1].strip()
    raise ValueError(f"no module line in {cli_dir / 'go.mod'}")


def connector_slugs() -> list[str]:
    """Every skill whose vendored CLI exposes cli.RootCmd()."""
    out = []
    for d in sorted(SKILLS.iterdir()):
        root_go = d / "cli" / "internal" / "cli" / "root.go"
        if root_go.is_file() and "func RootCmd() *cobra.Command" in root_go.read_text(
            encoding="utf-8", errors="replace"
        ):
            out.append(d.name)
    return out


def dump_tree(slug: str, timeout: int) -> tuple[list[dict] | None, str]:
    """Compile and run the walker against skills/<slug>/cli. (nodes, error)."""
    cli_dir = SKILLS / slug / "cli"
    try:
        module = module_path(cli_dir)
    except (OSError, ValueError) as exc:
        return None, str(exc)
    tmp = Path(tempfile.mkdtemp(prefix="cmdtree-"))
    try:
        src = tmp / "main.go"
        src.write_text(DUMP_GO.replace("__MODULE__", module), encoding="utf-8")
        overlay = tmp / "overlay.json"
        target = str((cli_dir / DUMP_PKG / "main.go").resolve())
        overlay.write_text(json.dumps({"Replace": {target: str(src)}}), encoding="utf-8")
        exe = tmp / ("treedump.exe" if os.name == "nt" else "treedump")
        build = subprocess.run(
            ["go", "build", "-overlay", str(overlay), "-o", str(exe), "./" + DUMP_PKG],
            cwd=cli_dir,
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        if build.returncode != 0:
            return None, "could not compile the tree walker: " + build.stderr.strip()[-600:]
        home = tmp / "home"
        home.mkdir()
        # Building the tree must not depend on, or touch, anyone's real config.
        env = {"PATH": os.environ.get("PATH", ""), "HOME": str(home)}
        if os.name == "nt":
            env.update({"USERPROFILE": str(home), "APPDATA": str(home), "LOCALAPPDATA": str(home),
                        "SYSTEMROOT": os.environ.get("SYSTEMROOT", "")})
        run = subprocess.run([str(exe)], capture_output=True, text=True, env=env, timeout=timeout)
        if run.returncode != 0:
            return None, "tree walker exited %d: %s" % (run.returncode, run.stderr.strip()[-600:])
        nodes = json.loads(run.stdout)
        if not nodes:
            return None, "tree walker returned an empty command tree"
        return nodes, ""
    except subprocess.TimeoutExpired:
        return None, f"timed out after {timeout}s"
    except json.JSONDecodeError as exc:
        return None, f"tree walker printed invalid JSON: {exc}"
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def self_test() -> int:
    """Both-directions proof of the classifier, no Go needed."""
    def node(path, method="POST", api="", aliases=("create",)):
        ann = {"pp:method": method}
        if api:
            ann["pp:path"] = api
        return {"path": list(path), "aliases": list(aliases), "annotations": ann}

    must_fail = [
        node(["subscriptions", "purge-json", "purge-subscription"], api="/subscriptions/{subscription_id}/purge.json"),
        node(["reports", "delete"], api="/reports/delete-reports"),
        node(["devices", "reboot", "device"], api="/devices/{serial}/reboot"),
        node(["invoices", "void-invoice"], api="/invoices/{uid}/void.json"),
        node(["billing", "create-cancel-subscription"], api="/api/v1/billing/cancel-subscription"),
        node(["tasks", "create"], method="DELETE", api="/tasks/{id}"),
        node(["checks", "resetall", "checks"], api="/checks/{agent_id}/resetall/"),
        node(["registered-devices", "block-device"], api="/admin/v1/registered_devices/blocked/{compkey}"),
        node(["plugins", "enableDisablePlugin"], api="/plugins/{pluginId}/enabled"),
        # Hand-written command (no pp:path): judged by its path below the top.
        {"path": ["jobs", "purge"], "aliases": ["create"], "annotations": {}},
    ]
    must_pass = [
        node(["tickets", "create-ticket"], api="/tickets"),
        node(["quarantine", "export"], api="/v1/accounts/{account_id}/quarantine/export"),
        node(["contracts", "blocks", "contract-child-create-entity"], api="/Contracts/{parentId}/Blocks"),
        node(["backups", "unlock", "backups"], api="/backups/{id}/unlock/"),
        node(["contracts", "block-hour-factors", "contract-child-create-entity"],
             api="/Contracts/{parentId}/BlockHourFactors"),
        node(["accounts", "uninstall-password", "generate-regenerate"],
             api="/accounts/{account_id}/uninstall-password/generate"),
        node(["alert", "resolve", "alert"], api="/v2/alert/{alertUid}/resolve"),
        # Destructive, but no `create` alias: not this gate's business.
        node(["reports", "delete"], api="/reports/delete-reports", aliases=()),
        node(["reports", "delete"], api="/reports/delete-reports", aliases=("rm",)),
    ]
    bad = []
    for n in must_fail:
        f, _ = findings_for("selftest", [n])
        if not f:
            bad.append("expected FAIL, got pass: " + " ".join(n["path"]))
    for n in must_pass:
        f, _ = findings_for("selftest", [n])
        if f:
            bad.append("expected pass, got FAIL: " + " ".join(n["path"]) + " (" + f[0]["reason"] + ")")
    shadow_node = {"path": ["tickets", "list"], "aliases": [], "annotations": {},
                   "shadows": [{"flag": "agent", "owner": []}]}
    _, sh = findings_for("selftest", [shadow_node])
    if len(sh) != 1 or sh[0]["flag"] != "agent" or sh[0]["shadows"] != "root":
        bad.append("flag-shadow report did not surface a local --agent")
    if bad:
        print("FAIL: check_command_tree self-test")
        for b in bad:
            print("  - " + b)
        return 1
    print(f"PASS: check_command_tree self-test ({len(must_fail)} must-fail, "
          f"{len(must_pass)} must-pass, 1 shadow)")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--slug", action="append", default=[], help="check one connector (repeatable)")
    ap.add_argument("--json", metavar="PATH", help="also write findings as JSON to PATH")
    ap.add_argument("--jobs", type=int, default=4, help="connectors compiled in parallel (default 4)")
    ap.add_argument("--timeout", type=int, default=600, help="per-connector timeout in seconds")
    ap.add_argument("--self-test", action="store_true", help="prove the classifier both directions")
    args = ap.parse_args()

    if args.self_test:
        return self_test()
    if shutil.which("go") is None:
        print("FAIL: go is not on PATH; the command tree cannot be read.")
        return 1

    known = connector_slugs()
    slugs = args.slug or known
    unknown = [s for s in slugs if s not in known]
    if unknown:
        print("FAIL: not a connector with cli.RootCmd(): " + ", ".join(unknown))
        return 1

    results: dict[str, tuple[list[dict] | None, str]] = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=max(1, args.jobs)) as pool:
        futs = {pool.submit(dump_tree, s, args.timeout): s for s in slugs}
        for fut in concurrent.futures.as_completed(futs):
            results[futs[fut]] = fut.result()

    fails, shadows, errors = [], [], []
    commands = 0
    for slug in sorted(results):
        nodes, err = results[slug]
        if nodes is None:
            errors.append({"slug": slug, "error": err})
            continue
        commands += len(nodes)
        f, sh = findings_for(slug, nodes)
        fails.extend(f)
        shadows.extend(sh)

    for sh in shadows:
        print(f"  WARN  [{sh['slug']}] `{sh['command']}` declares its own --{sh['flag']}, "
              f"shadowing the persistent --{sh['flag']} from {sh['shadows']} (report only)")
    for e in errors:
        print(f"  FAIL  [{e['slug']}] command tree unreadable: {e['error']}")
    for f in fails:
        print(f"  FAIL  [{f['slug']}] `{f['resolves_as']}` runs `{f['command']}` "
              f"({f['method']} {f['api_path']}; {f['reason']})")

    if args.json:
        Path(args.json).write_text(
            json.dumps({"create_alias_on_destructive": fails, "flag_shadows": shadows,
                        "errors": errors, "connectors": len(slugs), "commands": commands},
                       indent=1) + "\n",
            encoding="utf-8",
        )

    bad_slugs = sorted({f["slug"] for f in fails})
    if fails or errors:
        print(f"\nFAIL: {len(fails)} destructive command(s) answer to `{CREATE_ALIAS}` in "
              f"{len(bad_slugs)} connector(s); {len(errors)} connector(s) unreadable. "
              f"Remove the alias (and record it in that connector's handfixes.json).")
        return 1
    print(f"PASS: no destructive command answers to `{CREATE_ALIAS}` across {len(slugs)} "
          f"connector(s), {commands} commands ({len(shadows)} flag shadow(s) reported).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
