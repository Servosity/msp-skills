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
and whose action is destructive or hazardous. In order:

  1. HTTP method DELETE in the command's `pp:method` annotation.
  2. REVIEWED_HAZARDS: endpoints read and judged destructive whose names do not
     say so (Tactical RMM's service start/stop endpoint is POST
     /services/{agent_id}/{svcname}/), keyed by slug and API path. This also
     overrides a name that says create when creating runs something
     (Meraki's `create-organization-action-batch` executes its actions).
     REVIEWED_SAFE is the opposite override: endpoints a word rule would flag
     that were read and found not destructive (Maxio's `issue-service-credit`
     adds a credit; NinjaOne's template `restore` un-archives).
  3. A DESTRUCTIVE_WORDS word in the command's action words: its own name
     (`purge-subscription`, `bulk-delete`) plus the last literal segment of its
     `pp:path` (`/invoices/{uid}/void.json` gives `void`; `/devices/{serial}/reboot`
     gives `reboot` for `devices reboot device`). Resource and group names are
     nouns and are not read, so `quarantine export` is not a quarantine and
     `uninstall-password generate-regenerate` is not an uninstall. A leaf that
     names itself a create ignores the path literal (a resource noun there),
     but still counts its own words (`create-cancel-subscription`). A command
     without `pp:path` (hand-written) is judged by its path below the
     top-level resource.
  4. A SUMMARY_VERBS verb leading the command's Short (the spec's operation
     summary): `move-rules` whose summary is "Remove Firewall Rules ...".
  5. A HAZARD_WORDS word (dispatchers such as `devices action`, remote
     execution, installs, forced updates, invoices issued, moves) in the action
     words of a leaf that neither names itself a create nor names a read
     (READ_WORDS: `get-os-patch-compliance` and `action-logs-export` pass).

Words are matched exactly after splitting on punctuation and camelCase (no
prefix match), so a noun such as `blocks` is not read as the verb `block`.

What it reports (never fails)
-----------------------------
  * Command-local flags that shadow a persistent flag inherited from an
    ancestor, for example a local `--agent` string flag that silently replaces
    the global `--agent` agent-mode switch on that one command: one summary
    line per flag (`--list-shadows` for each).
  * The work list: commands that answer to `create` without being named a
    create or judged by rules 1-5. Many are collection creates (POST /roles);
    the words cannot tell those from non-create actions, so they are listed
    (`--list-unjudged`), never failed. The durable fix is upstream: emit
    `create` only for operations that create.

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
    python3 tools/maintainer/check_command_tree.py --self-test      # classifier + walker proof
    python3 tools/maintainer/check_command_tree.py --list-unjudged  # print the work list

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
        "hold",
        "interrupt",
        "isolate",
        "kill",
        "leave",
        "lock",
        "offboard",
        "pause",
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
        "rollback",
        "rotate",
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

# Added after Codex review round 2 and a read of every remaining aliased POST:
# ending a contract or account, money moving back out, repository surgery,
# restore-over-current-data and topology changes.
DESTRUCTIVE_WORDS = DESTRUCTIVE_WORDS | frozenset(
    {
        "closure",
        "deduct",
        "finalization",
        "finalize",
        "migrate",
        "migration",
        "migrations",
        "refund",
        "refunds",
        "repair",
        "restore",
        "split",
        "unbind",
    }
)

# Not destructive by name, but a POST answering to `create` must not reach them
# either: dispatchers whose effect is chosen by a parameter (`devices action`
# reboots with --action-type=1), remote execution and installs on managed
# endpoints, forced updates, billing documents issued to customers, and moves.
# Applied only when the leaf does not name itself a create and names no read.
HAZARD_WORDS = frozenset(
    {
        "act",
        "action",
        "actions",
        "control",
        "execute",
        "force",
        "install",
        "issue",
        "move",
        "ospatch",
        "patch",
        "schedule",
    }
)

# A leaf that says it reads (`get-os-patch-compliance`, `action-logs-export`,
# `run-query`) is a read, whatever noun follows. Destructive words still count.
READ_WORDS = frozenset(
    {"download", "export", "fetch", "get", "list", "lookup", "preview", "query", "search", "view"}
)

# Summary verbs strong enough to judge on: the spec's own operation summary
# leads with one of these. Narrower than DESTRUCTIVE_WORDS because a summary
# such as "Block an item" is often a create of a block-list entry.
SUMMARY_VERBS = frozenset(
    {
        "cancel", "deactivate", "delete", "destroy", "disable", "erase", "expire", "kill",
        "purge", "reboot", "remove", "reset", "restart", "revoke", "shutdown", "stop",
        "suspend", "terminate", "uninstall", "unenroll", "unregister", "void", "wipe",
    }
)

# Endpoints judged destructive by reading the vendor's operation, whose names
# do not say so. Keyed by (slug, API path) because the path survives a reprint
# that renames the command. Each was removed in the same change that listed it.
REVIEWED_HAZARDS = {
    ("action1", "/updates/{orgId}/approvals"):
        "approves or declines update packages for deployment to endpoints",
    ("avanan", "/v1.0/action/entity"):
        "dispatches an action (quarantine, restore, ...) on a mail or file entity",
    ("cisco-meraki", "/networks/{networkId}/bind"):
        "binds a network to a template, replacing its configuration",
    ("cisco-meraki", "/organizations/{organizationId}/switch/devices/clone"):
        "overwrites the target switches' port and switch settings",
    # Named creates whose "create" executes something (Codex review round 3).
    ("cisco-meraki", "/organizations/{organizationId}/actionBatches"):
        "creates and, with confirmed=true, runs a batch of arbitrary actions, destroy included",
    ("cisco-meraki", "/devices/{serial}/sensor/commands"):
        "sends a command to a sensor (downstream power off or cycle, for example)",
    ("cisco-meraki", "/devices/{serial}/appliance/interfaces/ports/update"):
        "sets appliance port state, including enabled=false, which takes the port down",
    ("cisco-meraki", "/devices/{serial}/cellular/uplinks/bands/masks/update"):
        "sets which cellular bands the uplink may use; a narrow mask drops the uplink",
    ("cisco-meraki", "/organizations/{organizationId}/campusGateway/clusters/tunneling/batchUpdate"):
        "rewrites campus-gateway tunnel settings for many networks at once",
    ("cisco-meraki", "/networks/{networkId}/pii/requests"):
        "submits a request to delete or restrict processing of a client's personal data",
    ("immybot", "/api/v1/maintenance-sessions/{sessionId}/actions/{actionId}/rerun"):
        "re-runs a maintenance action (software install, patch or task) on the computer",
    ("immybot", "/plugins/api/v1/{providerLinkId}/{catchAll}"):
        "passes an arbitrary POST through to a provider plugin's API",
    ("pagerduty", "/incident_workflows/{id}/instances"):
        "starts an incident workflow, running its automated actions",
    ("sophos-central", "/endpoint/v1/migrations"):
        "starts a job that migrates endpoints from another tenant",
    # Codex review round 4 and the pass over the work list that followed.
    ("servosity", "/companies/{id}/agents/update-latest/"):
        "upgrades every backup agent in the company to the latest build",
    ("servosity", "/resellers/{id}/agents/update-latest/"):
        "upgrades every backup agent under the reseller to the latest build",
    ("sophos-central", "/endpoint/v1/software/packages/static/{staticPackageId}/add"):
        "adds a support-supplied software package to the tenant; Sophos calls it one-way",
    ("domotz", "/agent/{agent_id}/network/excluded-device/{device_id}"):
        "excludes a device from collector monitoring, which stops its monitoring",
    # Bulk endpoints whose body or path selects a resolve, snooze, reject or
    # delete (owner's Codex review of #416, and the re-scan of the work list).
    ("tactical-rmm", "/alerts/bulk/"):
        "bulk_action resolve or snooze on a list of alerts (Tactical RMM alerts/views.py BulkAlerts)",
    ("rootly", "/v1/alerts/{id}/resolve"):
        "resolves the alert and, with resolve_related_incidents, every incident attached to it",
    ("sophos-central", "/account-health-check/v1/snooze"):
        "snoozes account health-check findings for the policies and exclusions in the body",
    ("huntress", "/v1/incident_reports/{incident_report_id}/remediations/bulk_rejection"):
        "rejects every pending remediation on the incident report, so none is carried out",
    ("ninjaone", "/v2/devices/approval/{mode}"):
        "approves or, with mode REJECT, rejects the listed devices waiting for approval",
    ("threatdown", "/v1/accounts/{account_id}/sa/close"):
        "closes every suspicious-activity detection in detection_id_list",
    ("appdirect", "/appwise/v2/data"):
        "publishes content events; action_delete or a ResourceDeleted event removes the resource from Search",
    ("huntress", "/v1/accounts/{account_id}/incident_reports/{incident_report_id}/remediations/bulk_approval"):
        "approves every pending remediation, which Huntress then carries out on the hosts",
    ("ninjaone", "/v2/device/{id}/script/run"):
        "runs a script or built-in action on a device",
    ("tactical-rmm", "/agents/{agent_id}/meshcentral/"):
        "recovers (reinstalls) the MeshCentral agent on the endpoint",
    ("tactical-rmm", "/autotasks/{pk}/run/"):
        "runs an automated task on its agents now",
    ("tactical-rmm", "/scripts/{agent_id}/test/"):
        "runs a script on an agent",
    ("tactical-rmm", "/services/{agent_id}/{svcname}/"):
        "starts, stops or restarts a Windows service on an agent",
    ("tactical-rmm", "/software/{agent_id}/"):
        "installs software on an agent",
    ("watchguard", "/rest/firebox/management/v1/configuration/{accountId}/templates/subscribe"):
        "subscribes or unsubscribes Firebox templates, replacing device configuration",
}

# The opposite override: endpoints a word rule would flag that were read and
# found not destructive (Codex review round 6, P2). Keyed like REVIEWED_HAZARDS.
REVIEWED_SAFE = {
    ("maxio", "/subscriptions/{subscription_id}/service_credits.json"):
        "issue-service-credit adds a credit balance; nothing is refunded or finalized",
    ("maxio", "/subscription_groups/{uid}/service_credits.json"):
        "issue-subscription-group-service-credit adds a credit balance",
    ("ninjaone", "/v2/document-templates/{documentTemplateId}/restore"):
        "restore un-archives a document template, reversing an archive",
}

DESTRUCTIVE_METHODS = frozenset({"DELETE"})

# The overlay package paths inside each module. They never exist on disk.
DUMP_PKG = "internal/zz_check_command_tree_dump"
FIXTURE_PKG = "internal/zz_check_command_tree_fixture"

# Self-test fixture for the Go walker itself (see self_test).
FIXTURE_GO = r"""package zzfixture

import "github.com/spf13/cobra"

func noop(*cobra.Command, []string) error { return nil }

func RootCmd() *cobra.Command {
	root := &cobra.Command{Use: "fixture"}
	root.PersistentFlags().Bool("agent", false, "agent mode")
	root.PersistentFlags().Bool("dry-run", false, "dry run")

	// The same group registered twice; only the SECOND copy carries the alias.
	for _, aliases := range [][]string{nil, {"create"}} {
		grp := &cobra.Command{Use: "subscriptions"}
		grp.AddCommand(&cobra.Command{Use: "purge-subscription", Aliases: aliases, RunE: noop,
			Annotations: map[string]string{"pp:method": "POST", "pp:path": "/subscriptions/{id}/purge.json"}})
		root.AddCommand(grp)
	}

	tickets := &cobra.Command{Use: "tickets"}
	tickets.PersistentFlags().Bool("dry-run", false, "group-level dry run") // shadows root
	list := &cobra.Command{Use: "list", RunE: noop}
	list.Flags().String("agent", "", "agent id filter") // shadows root --agent
	get := &cobra.Command{Use: "get", RunE: noop}
	tickets.AddCommand(list, get, &cobra.Command{Use: "create-ticket", Aliases: []string{"create"}, RunE: noop,
		Annotations: map[string]string{"pp:method": "POST", "pp:path": "/tickets"}})
	root.AddCommand(tickets)
	// Merge get's inherited flags: its Flags() now holds tickets' --dry-run
	// and root's --agent. Neither is a shadow.
	_ = get.InheritedFlags()
	return root
}
"""

DUMP_GO = r"""// Command-tree dump for tools/maintainer/check_command_tree.py.
// Supplied through `go build -overlay`; never written into the connector.
package main

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	cli "__IMPORT__"
)

type shadow struct {
	Flag  string   `json:"flag"`
	Owner []string `json:"owner"`
}

type node struct {
	Path        []string          `json:"path"`
	Short       string            `json:"short"`
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
			Short:       sub.Short,
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
		// Cobra merges inherited flags nearest-parent first, so a group that
		// redeclares a persistent flag becomes the owner its descendants see.
		sub.PersistentFlags().VisitAll(func(f *pflag.Flag) {
			next[f.Name] = inherited{flag: f, owner: p}
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


def summary_verbs(short: str) -> set[str]:
    """The leading verb(s) of the command's one-line summary, singularised.

    The generator copies the spec's operation summary into Short, and a summary
    leads with the operation: "Remove Firewall Rules ..." on a command named
    `move-rules`, "Start/Stop/Restart Windows Service ..." on `control`."""
    first = (short or "").strip().split(None, 1)
    if not first:
        return set()
    out: set[str] = set()
    for w in re.split(r"[/,]", first[0].lower()):
        w = re.sub(r"[^a-z]", "", w)
        if not w:
            continue
        out.add(w)
        if w.endswith("es"):
            out.add(w[:-2])
        if w.endswith("s"):
            out.add(w[:-1])
    return out


def destructive_reason(node: dict, slug: str = "") -> str | None:
    """Why this command's action is destructive or hazardous, or None."""
    ann = node.get("annotations") or {}
    method = (ann.get("pp:method") or "").upper()
    if method in DESTRUCTIVE_METHODS:
        return f"HTTP {method}"
    reviewed = REVIEWED_HAZARDS.get((slug, ann.get("pp:path", "")))
    if reviewed:
        return "reviewed: " + reviewed
    if (slug, ann.get("pp:path", "")) in REVIEWED_SAFE:
        return None
    acts = action_words(node)
    hit = sorted(acts & DESTRUCTIVE_WORDS)
    if hit:
        return "action word " + ", ".join(repr(w) for w in hit)
    verb = sorted(summary_verbs(node.get("short", "")) & SUMMARY_VERBS)
    if verb:
        return "summary verb " + ", ".join(repr(w) for w in verb)
    path = node.get("path") or []
    leaf = words(path[-1] if path else "")
    if CREATE_ALIAS not in leaf and not (leaf & READ_WORDS):
        hazard = sorted(acts & HAZARD_WORDS)
        if hazard:
            return "dispatch/execute word " + ", ".join(repr(w) for w in hazard)
    return None


def is_named_create(node: dict) -> bool:
    path = node.get("path") or []
    return CREATE_ALIAS in words(path[-1] if path else "")


def has_create_alias(node: dict) -> bool:
    return any((a or "").lower() == CREATE_ALIAS for a in node.get("aliases") or [])


def named_creates_for(slug: str, nodes: list[dict]) -> int:
    """How many aliased commands are trusted as creates on their name alone."""
    seen = set()
    for node in nodes:
        if has_create_alias(node) and is_named_create(node) and not destructive_reason(node, slug):
            seen.add(" ".join(node["path"]))
    return len(seen)


def unjudged_for(slug: str, nodes: list[dict]) -> list[dict]:
    """Report-only work list: `create` on a command neither named a create nor
    judged destructive. The gate cannot tell a collection create (POST /roles)
    from a non-create action by its words alone, so these are listed, never
    failed. The durable fix is upstream: emit `create` only for creates."""
    out, seen = [], set()
    for node in nodes:
        if not has_create_alias(node) or is_named_create(node) or destructive_reason(node, slug):
            continue
        key = " ".join(node["path"])
        if key in seen:
            continue
        seen.add(key)
        ann = node.get("annotations") or {}
        out.append({"slug": slug, "command": key, "method": ann.get("pp:method", ""),
                    "api_path": ann.get("pp:path", ""), "short": node.get("short", "")})
    return out


def findings_for(slug: str, nodes: list[dict]) -> tuple[list[dict], list[dict]]:
    """Return (create_alias_failures, flag_shadow_reports) for one connector."""
    fails, shadows = [], []
    seen_fail: set[tuple] = set()
    seen_shadow: set[tuple] = set()
    for node in nodes:
        # Every node is judged. A generator defect can register the same group
        # twice (ninjaone `document-templates archive`), and the two copies need
        # not carry the same aliases, so only identical FINDINGS are merged.
        if has_create_alias(node):
            reason = destructive_reason(node, slug)
            ann = node.get("annotations") or {}
            key = (" ".join(node["path"]), ann.get("pp:method", ""), ann.get("pp:path", ""), reason)
            if reason and key not in seen_fail:
                seen_fail.add(key)
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
            key = (" ".join(node["path"]), sh["flag"], " ".join(sh["owner"]))
            if key in seen_shadow:
                continue
            seen_shadow.add(key)
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


def load_registry_connectors() -> dict[str, str]:
    """{slug: directory under skills/} for every registered skill that is not
    markdown-only, i.e. every skill the registry says ships a CLI."""
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    import registry  # noqa: E402  (local tools/ module)

    return {
        slug: registry.source_dir(slug)
        for slug in registry.skills()
        if not registry.is_markdown_only(slug)
    }


def enumerate_connectors(skills_dir: Path, registered: dict[str, str]) -> tuple[list[str], list[dict]]:
    """Every connector this check must read, and the ones it cannot.

    The expected set is the union of the registry's CLI-shipping skills and
    every skills/<dir>/cli/go.mod on disk, so neither a registered connector
    that lost its module nor an unregistered module can drop out silently.
    A connector is NOT filtered by whether its root.go looks right: a missing
    entry point is an error here, and a malformed one fails to compile in
    dump_tree, which is also an error. Returns (directories, errors)."""
    on_disk = {p.parent.parent.name for p in skills_dir.glob("*/cli/go.mod")}
    expected = sorted(on_disk | set(registered.values()))
    found, errors = [], []
    for d in expected:
        cli_dir = skills_dir / d / "cli"
        if not (cli_dir / "go.mod").is_file():
            errors.append({"slug": d, "error": "registered as a connector but skills/%s/cli/go.mod "
                                                "is missing" % d})
        elif not (cli_dir / "internal" / "cli" / "root.go").is_file():
            errors.append({"slug": d, "error": "no internal/cli/root.go, so the command tree has no "
                                                "entry point to read"})
        else:
            found.append(d)
    return found, errors


def connector_slugs() -> list[str]:
    """Connectors with a readable entry point (errors are reported by main)."""
    return enumerate_connectors(SKILLS, load_registry_connectors())[0]


def dump_tree(slug: str, timeout: int, fixture_go: str | None = None,
              replace_files: dict[str, str] | None = None,
              skills_dir: Path | None = None) -> tuple[list[dict] | None, str]:
    """Compile and run the walker against skills/<slug>/cli. (nodes, error).

    With `fixture_go`, the walker reads that fixture's RootCmd() instead (the
    self-test), still compiled inside the connector's module for its cobra.
    `replace_files` ({path under cli/: content}) overlays connector files, so
    the self-test can hand the walker a malformed root.go without touching
    the tree."""
    cli_dir = (skills_dir or SKILLS) / slug / "cli"
    if not (cli_dir / "internal" / "cli" / "root.go").is_file() and fixture_go is None:
        return None, "no internal/cli/root.go, so the command tree has no entry point to read"
    try:
        module = module_path(cli_dir)
    except (OSError, ValueError) as exc:
        return None, str(exc)
    tmp = Path(tempfile.mkdtemp(prefix="cmdtree-"))
    try:
        src = tmp / "main.go"
        replace = {str((cli_dir / DUMP_PKG / "main.go").resolve()): str(src)}
        import_path = module + "/internal/cli"
        if fixture_go is not None:
            # Self-test: walk a fixture tree instead of the connector's own.
            fx = tmp / "fixture.go"
            fx.write_text(fixture_go, encoding="utf-8")
            replace[str((cli_dir / FIXTURE_PKG / "fixture.go").resolve())] = str(fx)
            import_path = module + "/" + FIXTURE_PKG
        for i, (rel, content) in enumerate(sorted((replace_files or {}).items())):
            alt = tmp / f"replace-{i}.go"
            alt.write_text(content, encoding="utf-8")
            replace[str((cli_dir / rel).resolve())] = str(alt)
        src.write_text(DUMP_GO.replace("__IMPORT__", import_path), encoding="utf-8")
        overlay = tmp / "overlay.json"
        overlay.write_text(json.dumps({"Replace": replace}), encoding="utf-8")
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


def enumeration_self_test(bad: list[str]) -> str:
    """End to end: copy the smallest real connector three times into a fixture
    skills/ tree (one intact, one with no root.go, one whose root.go no longer
    defines RootCmd), register a fourth with no directory, and run the real
    check over it. Only the intact copy may pass."""
    found = connector_slugs()
    if not found:
        return "enumeration skipped (no connector to copy)"
    donor = min(found, key=lambda d: sum(1 for _ in (SKILLS / d / "cli").rglob("*")))
    tmp = Path(tempfile.mkdtemp(prefix="cmdtree-enum-"))
    try:
        skills = tmp / "skills"
        for name in ("good", "noroot", "badroot"):
            shutil.copytree(SKILLS / donor / "cli", skills / name / "cli")
        (skills / "noroot" / "cli" / "internal" / "cli" / "root.go").unlink()
        root = skills / "badroot" / "cli" / "internal" / "cli" / "root.go"
        text = root.read_text(encoding="utf-8")
        if "func RootCmd() *cobra.Command" not in text:
            bad.append(f"enumeration fixture: {donor}'s root.go has no RootCmd to break")
            return "enumeration fixture broken"
        root.write_text(text.replace("func RootCmd() *cobra.Command", "func RootCmdGone() *cobra.Command"),
                        encoding="utf-8")
        reg = tmp / "registry.json"
        reg.write_text(json.dumps({"good": "good", "ghost": "ghost"}), encoding="utf-8")
        run = subprocess.run(
            [sys.executable, str(Path(__file__).resolve()), "--skills-dir", str(skills),
             "--registry-json", str(reg), "--jobs", "2"],
            capture_output=True, text=True, timeout=1800,
        )
        out = run.stdout + run.stderr
        if run.returncode != 1:
            bad.append(f"enumeration fixture: check exited {run.returncode}, want 1:\n{out[-800:]}")
        for name, needle in (("noroot", "no internal/cli/root.go"),
                             ("badroot", "could not compile the tree walker"),
                             ("ghost", "cli/go.mod is missing")):
            if not any(f"[{name}]" in line and needle in line for line in out.splitlines()):
                bad.append(f"enumeration fixture: no FAIL line for {name} ({needle})")
        if any("[good]" in line and "FAIL" in line for line in out.splitlines()):
            bad.append("enumeration fixture: the intact copy failed, so the proof is not about the root")
        if "unreadable" not in out:
            bad.append("enumeration fixture: no unreadable connector reported")
        return f"enumeration fixture ok via {donor}"
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def self_test() -> int:
    """Both-directions proof of the classifier, no Go needed."""
    def node(path, method="POST", api="", aliases=("create",), short=""):
        ann = {"pp:method": method}
        if api:
            ann["pp:path"] = api
        return {"path": list(path), "aliases": list(aliases), "annotations": ann, "short": short}

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
        node(["networks", "firmware-upgrades", "create-network-rollback"],
             api="/networks/{networkId}/firmwareUpgrades/rollbacks"),
        node(["meeting-recordings", "leave", "meeting-recording"], api="/v1/meeting_recordings/{id}/leave"),
        node(["api-keys", "rotate", "api-key"], api="/v1/api_keys/{id}/rotate"),
        node(["subscriptions", "hold-json", "pause-subscription"], api="/subscriptions/{subscription_id}/hold.json"),
        node(["networks", "unbind", "network"], api="/networks/{networkId}/unbind"),
        node(["invoices", "refunds-json", "refund-invoice"], api="/invoices/{uid}/refunds.json"),
        # Dispatchers and remote execution (Codex round 2).
        node(["endpoint-devices", "action"], api="/rest/endpoint-security/v1/accounts/{accountId}/devices/action"),
        node(["device", "windows-service", "control"], api="/v2/device/{id}/windows-service/{serviceId}/control"),
        node(["patching", "patch-now"], api="/w/company/patch_now"),
        # The summary names the action when the name does not.
        node(["firewall-control", "move-rules", "by-category"], api="/firewall-control/{category}/move-rules",
             short="Remove Firewall Rules, defined with the ID of the rules"),
        node(["things", "rpc"], api="/things/rpc", short="Start/Stop/Restart the thing"),
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
        node(["patches", "get-os-patch-compliance"], api="/api/platform/v2/os-patching/compliance/summary"),
        node(["grid", "action-logs-export"], api="/v1/grid/action-logs/export"),
        node(["api-resources", "resources-actions-create"], api="/resources/{resource_id}/actions"),
        node(["live-discover", "run-query"], api="/live-discover/v1/queries/runs"),
        node(["roles", "post"], api="/roles", short="Creates a role"),
        node(["blocked-items", "post-by-id"], api="/endpoint/v1/settings/blocked-items", short="Block an item"),
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
    # Two registrations of one path: the copy WITH the alias must still fail.
    dup = [node(["subscriptions", "purge-json", "purge-subscription"], api="/subscriptions/{id}/purge.json", aliases=()),
           node(["subscriptions", "purge-json", "purge-subscription"], api="/subscriptions/{id}/purge.json")]
    f, _ = findings_for("selftest", dup)
    if len(f) != 1:
        bad.append(f"duplicate registration: expected 1 finding from the aliased copy, got {len(f)}")
    # A reviewed hazard fails only for its own connector.
    svc = node(["services", "services-agentid-2"], api="/services/{agent_id}/{svcname}/")
    credit = node(["subscriptions", "service-credits-json", "issue-service-credit"],
                  api="/subscriptions/{subscription_id}/service_credits.json")
    if findings_for("maxio", [credit])[0] or not findings_for("other", [credit])[0]:
        bad.append("REVIEWED_SAFE must clear a word match for its own slug only")
    batch = node(["organizations", "action-batches", "create-organization-action-batch"],
                 api="/organizations/{organizationId}/actionBatches")
    if not findings_for("cisco-meraki", [batch])[0]:
        bad.append("a reviewed hazard must override the named-create exemption")
    if not findings_for("tactical-rmm", [svc])[0] or findings_for("other", [svc])[0]:
        bad.append("REVIEWED_HAZARDS must apply to its own slug only")
    # Bulk resolve/snooze/reject/delete endpoints. Named here, not read from
    # REVIEWED_HAZARDS, so deleting an entry fails the self-test.
    for slug, path, api in [
        ("tactical-rmm", ["alerts", "alerts-bulk"], "/alerts/bulk/"),
        ("rootly", ["alerts", "resolve", "alert"], "/v1/alerts/{id}/resolve"),
        ("sophos-central", ["health-check", "post-snooze"], "/account-health-check/v1/snooze"),
        ("huntress", ["incident-reports", "remediations", "bulk-rejection-parameters"],
         "/v1/incident_reports/{incident_report_id}/remediations/bulk_rejection"),
        ("ninjaone", ["devices", "node-approval-operation"], "/v2/devices/approval/{mode}"),
        ("appdirect", ["appwise", "events"], "/appwise/v2/data"),
        ("threatdown", ["suspicious-activity", "close"], "/v1/accounts/{account_id}/sa/close"),
    ]:
        if not findings_for(slug, [node(path, api=api)])[0]:
            bad.append(f"expected FAIL for {slug} POST {api} (bulk resolve/snooze/reject/delete)")

    # Connector enumeration: a missing or malformed entry point must FAIL the
    # check, never drop the connector from the run (owner's review of #416).
    enum = "enumeration skipped (no go on PATH)"
    if shutil.which("go"):
        enum = enumeration_self_test(bad)

    # The Go walker itself, on a fixture tree, when Go and a connector exist.
    walker = "skipped (no go on PATH or no connector to borrow a module from)"
    slugs = connector_slugs()
    if shutil.which("go") and slugs:
        nodes, err = dump_tree(slugs[0], 600, fixture_go=FIXTURE_GO)
        if nodes is None:
            bad.append("walker fixture did not run: " + err)
        else:
            f, sh = findings_for("fixture", nodes)
            got_fail = sorted(x["command"] for x in f)
            got_sh = sorted((x["command"], x["flag"], x["shadows"]) for x in sh)
            want_sh = [("tickets", "dry-run", "root"), ("tickets list", "agent", "root")]
            if got_fail != ["subscriptions purge-subscription"]:
                bad.append(f"walker fixture: create-alias findings {got_fail}, want the duplicate's aliased copy only")
            if got_sh != want_sh:
                bad.append(f"walker fixture: shadows {got_sh}, want {want_sh}")
            walker = f"walker fixture ok via {slugs[0]}'s module"
    if bad:
        print("FAIL: check_command_tree self-test")
        for b in bad:
            print("  - " + b)
        return 1
    print(f"PASS: check_command_tree self-test ({len(must_fail)} must-fail, "
          f"{len(must_pass)} must-pass, 7 bulk, 1 shadow, 1 duplicate; {walker}; {enum})")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--slug", action="append", default=[], help="check one connector (repeatable)")
    ap.add_argument("--json", metavar="PATH", help="also write findings as JSON to PATH")
    ap.add_argument("--jobs", type=int, default=4, help="connectors compiled in parallel (default 4)")
    ap.add_argument("--timeout", type=int, default=600, help="per-connector timeout in seconds")
    ap.add_argument("--self-test", action="store_true", help="prove the classifier both directions")
    ap.add_argument("--list-shadows", action="store_true",
                    help="print every flag shadow instead of a per-flag summary")
    ap.add_argument("--skills-dir", help=argparse.SUPPRESS)
    ap.add_argument("--registry-json", help=argparse.SUPPRESS)
    ap.add_argument("--list-unjudged", action="store_true",
                    help="print every `create` alias the gate does not judge (report only)")
    args = ap.parse_args()

    if args.self_test:
        return self_test()
    if shutil.which("go") is None:
        print("FAIL: go is not on PATH; the command tree cannot be read.")
        return 1

    skills_dir = Path(args.skills_dir) if args.skills_dir else SKILLS
    registered = (json.loads(Path(args.registry_json).read_text(encoding="utf-8"))
                  if args.registry_json else load_registry_connectors())
    known, enum_errors = enumerate_connectors(skills_dir, registered)
    if args.slug:
        expected = set(known) | {e["slug"] for e in enum_errors}
        unknown = [s for s in args.slug if s not in expected]
        if unknown:
            print("FAIL: not a connector (no registry entry with a CLI and no skills/<dir>/cli/go.mod): "
                  + ", ".join(unknown))
            return 1
        enum_errors = [e for e in enum_errors if e["slug"] in args.slug]
        slugs = [s for s in args.slug if s in known]
    else:
        slugs = known

    results: dict[str, tuple[list[dict] | None, str]] = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=max(1, args.jobs)) as pool:
        futs = {pool.submit(dump_tree, s, args.timeout, None, None, skills_dir): s for s in slugs}
        for fut in concurrent.futures.as_completed(futs):
            results[futs[fut]] = fut.result()

    fails, shadows, unjudged = [], [], []
    errors = list(enum_errors)
    commands = named = 0
    for slug in sorted(results):
        nodes, err = results[slug]
        if nodes is None:
            errors.append({"slug": slug, "error": err})
            continue
        commands += len(nodes)
        f, sh = findings_for(slug, nodes)
        fails.extend(f)
        shadows.extend(sh)
        unjudged.extend(unjudged_for(slug, nodes))
        named += named_creates_for(slug, nodes)

    if args.list_shadows:
        for sh in shadows:
            print(f"  WARN  [{sh['slug']}] `{sh['command']}` declares its own --{sh['flag']}, "
                  f"shadowing the persistent --{sh['flag']} from {sh['shadows']} (report only)")
    else:
        by_flag: dict[str, list[dict]] = {}
        for sh in shadows:
            by_flag.setdefault(sh["flag"], []).append(sh)
        for flag in sorted(by_flag, key=lambda k: (-len(by_flag[k]), k)):
            hits = by_flag[flag]
            per_slug: dict[str, int] = {}
            for sh in hits:
                per_slug[sh["slug"]] = per_slug.get(sh["slug"], 0) + 1
            ranked = sorted(per_slug.items(), key=lambda kv: (-kv[1], kv[0]))
            where = ", ".join(f"{s} x{n}" for s, n in ranked[:6])
            if len(ranked) > 6:
                where += f", +{len(ranked) - 6} more connector(s)"
            print(f"  WARN  local --{flag} shadows a persistent --{flag} on {len(hits)} command(s): "
                  f"{where} (report only; --list-shadows for each)")
    if args.list_unjudged:
        for u in unjudged:
            print(f"  NOTE  [{u['slug']}] `{u['command']}` answers to `{CREATE_ALIAS}` "
                  f"({u['method']} {u['api_path']}): {u['short'][:90]}")
    if unjudged:
        print(f"  NOTE  {len(unjudged)} command(s) in {len({u['slug'] for u in unjudged})} connector(s) "
              f"answer to `{CREATE_ALIAS}` without being named a create or judged destructive "
              f"(report only; --list-unjudged for each)")
    if named:
        print(f"  NOTE  {named} command(s) keep `{CREATE_ALIAS}` because their own name says create; "
              f"a name is trusted, not proven (REVIEWED_HAZARDS overrides it)")
    for e in errors:
        print(f"  FAIL  [{e['slug']}] command tree unreadable: {e['error']}")
    for f in fails:
        print(f"  FAIL  [{f['slug']}] `{f['resolves_as']}` runs `{f['command']}` "
              f"({f['method']} {f['api_path']}; {f['reason']})")

    if args.json:
        Path(args.json).write_text(
            json.dumps({"create_alias_on_destructive": fails, "flag_shadows": shadows,
                        "create_alias_unjudged": unjudged, "named_creates_trusted": named,
                        "errors": errors, "connectors": len(slugs) + len(enum_errors),
                        "commands": commands},
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
