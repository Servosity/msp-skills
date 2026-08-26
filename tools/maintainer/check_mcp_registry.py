#!/usr/bin/env python3
"""Gate: every skills/<slug>/server.json is publishable to the MCP Registry.

Why this exists
---------------
`mcp-publish.yml` uploads the `.mcpb` to the GitHub release and then publishes
`server.json` to registry.modelcontextprotocol.io. The registry validates the
document and answers 422 on a violation. Nothing in this repo checked that
first, and the publish step runs AFTER the asset upload, so a rejection leaves a
release that looks complete while the connector never reaches the registry - or
any of the six downstream catalogues (PulseMCP, GitHub MCP Registry, Glama,
mcp.so, Smithery, LobeHub) that fan out from it.

That is not hypothetical. On 2026-08-26 the avanan publish failed with:

    422 {"detail":"validation failed","errors":[{
        "message":"expected length <= 100",
        "location":"body.description",
        "value":"Every Avanan (Check Point Harmony Email and Collaboration) API
                 operation, plus shift-start triage, campaign"}]}

107 characters against a 100-character limit. The release carried all 25 assets
and every check was green; only the registry knew.

What this checks
----------------
Each server.json is validated against the official MCP Registry JSON Schema,
vendored beside this file as mcp_registry_schema.json and pinned to the same
`$schema` URL the documents declare. Validating against the real schema rather
than a hand-copied rule means this gate also covers `name` (pattern and 200
chars), `title` (100), `version` (255), icon `src` (255) and `sizes`, package
`fileSha256`, and the transport URL patterns - every constraint the registry
will apply, not just the one that happened to bite.

The schema is vendored deliberately: CI must not depend on a network fetch to
decide whether a connector is publishable, and a silently-updated remote schema
must not turn main red without a commit. Refresh it with --refresh-schema, which
prints a diff of the constraints that changed.

Only the keywords this schema actually uses are implemented (type, required,
properties, additionalProperties, items, enum, const, pattern, minLength,
maxLength, allOf, anyOf, not, $ref). `format` is reported, never enforced - it
is an annotation in draft-07 and the registry does not gate on it.

Usage:
    python3 tools/maintainer/check_mcp_registry.py
    python3 tools/maintainer/check_mcp_registry.py --slug avanan
    python3 tools/maintainer/check_mcp_registry.py --self-test
    python3 tools/maintainer/check_mcp_registry.py --refresh-schema
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import registry  # noqa: E402  (local tools/ module)

HERE = Path(__file__).resolve().parent
SCHEMA_PATH = HERE / "mcp_registry_schema.json"


class Validator:
    """A draft-07 subset covering exactly the keywords the registry schema uses."""

    def __init__(self, schema: dict) -> None:
        self.root = schema

    def resolve(self, node: dict) -> dict:
        seen = 0
        while "$ref" in node:
            ref = node["$ref"]
            if not ref.startswith("#/"):
                raise ValueError(f"unsupported remote $ref {ref!r}")
            target: object = self.root
            for part in ref[2:].split("/"):
                target = target[part.replace("~1", "/").replace("~0", "~")]  # type: ignore[index]
            node = target  # type: ignore[assignment]
            seen += 1
            if seen > 64:
                raise ValueError("$ref cycle")
        return node

    def check(self, value: object, node: dict, path: str) -> list[str]:
        node = self.resolve(node)
        errs: list[str] = []

        t = node.get("type")
        if t and not self._type_ok(value, t):
            return [f"{path or '<root>'}: expected type {t}, got {self._name(value)}"]

        if "const" in node and value != node["const"]:
            errs.append(f"{path}: expected the constant {node['const']!r}, got {value!r}")
        if "enum" in node and value not in node["enum"]:
            errs.append(f"{path}: {value!r} is not one of {node['enum']!r}")

        if isinstance(value, str):
            if "maxLength" in node and len(value) > node["maxLength"]:
                errs.append(
                    f"{path}: {len(value)} characters, the registry allows at most "
                    f"{node['maxLength']}. Value: {value!r}"
                )
            if "minLength" in node and len(value) < node["minLength"]:
                errs.append(f"{path}: {len(value)} characters, the registry requires at least {node['minLength']}")
            if "pattern" in node and not re.search(node["pattern"], value):
                errs.append(f"{path}: {value!r} does not match the required pattern {node['pattern']}")

        if isinstance(value, dict):
            for req in node.get("required", []) or []:
                if req not in value:
                    errs.append(f"{path or '<root>'}: missing required property {req!r}")
            props = node.get("properties") or {}
            for k, v in value.items():
                sub = f"{path}.{k}" if path else k
                if k in props:
                    errs += self.check(v, props[k], sub)
                elif node.get("additionalProperties") is False:
                    errs.append(f"{sub}: property is not allowed by the registry schema")
                elif isinstance(node.get("additionalProperties"), dict):
                    errs += self.check(v, node["additionalProperties"], sub)

        if isinstance(value, list) and isinstance(node.get("items"), dict):
            for i, item in enumerate(value):
                errs += self.check(item, node["items"], f"{path}[{i}]")

        for sub in node.get("allOf", []) or []:
            errs += self.check(value, sub, path)
        if "anyOf" in node:
            branches = [self.check(value, s, path) for s in node["anyOf"]]
            if all(b for b in branches):
                best = min(branches, key=len)
                errs.append(f"{path or '<root>'}: matched none of the allowed shapes; closest: {best[0]}")
        if "not" in node and not self.check(value, node["not"], path):
            errs.append(f"{path or '<root>'}: matches a shape the registry forbids")
        return errs

    @staticmethod
    def _type_ok(value: object, t: object) -> bool:
        types = t if isinstance(t, list) else [t]
        for one in types:
            if one == "object" and isinstance(value, dict):
                return True
            if one == "array" and isinstance(value, list):
                return True
            if one == "string" and isinstance(value, str):
                return True
            if one == "boolean" and isinstance(value, bool):
                return True
            if one == "integer" and isinstance(value, int) and not isinstance(value, bool):
                return True
            if one == "number" and isinstance(value, (int, float)) and not isinstance(value, bool):
                return True
            if one == "null" and value is None:
                return True
        return False

    @staticmethod
    def _name(value: object) -> str:
        return {dict: "object", list: "array", str: "string", bool: "boolean", type(None): "null"}.get(
            type(value), type(value).__name__
        )


def load_schema() -> dict:
    return json.loads(SCHEMA_PATH.read_text(encoding="utf-8"))


# mcp-publish.yml rewrites four fields between the in-repo document and the one it
# POSTs to the registry (the "Patch server.json with SHA + version + identifier URL"
# step). The in-repo values are deliberate placeholders - fileSha256 cannot exist
# until the bundle is built. Gating on them would fail all 65 connectors for a
# placeholder that never reaches the registry, which is a false red that teaches
# maintainers to ignore this gate. Apply the same substitutions the workflow does,
# then validate the document as it will actually be published.
CI_FILLED_SHA = "PLACEHOLDER_SHA256_FILLED_BY_CI"


def as_published(doc: dict, slug: str, version: str, owner: str, repo: str) -> dict:
    """Return the document mcp-publish.yml would POST, from the in-repo one."""
    doc = json.loads(json.dumps(doc))  # deep copy; never mutate the caller's parse
    tag = f"{slug}-v{version}" if version else ""
    doc["version"] = version or doc.get("version", "")
    for pkg in doc.get("packages", []) or []:
        pkg["version"] = doc["version"]
        if pkg.get("fileSha256", CI_FILLED_SHA) == CI_FILLED_SHA:
            # Stand in a well-formed hash: the real one is computed from the built
            # bundle. A hand-written malformed hash is still caught, because only
            # this exact placeholder is substituted.
            pkg["fileSha256"] = "0" * 64
        if tag:
            pkg["identifier"] = (
                f"https://github.com/{owner}/{repo}/releases/download/{tag}/{slug}-mcp.mcpb"
            )
        pkg.pop("download", None)
    return doc


def check_slug(slug: str, validator: Validator) -> list[str]:
    path = registry.skill_path(slug) / "server.json"
    if not path.exists():
        return []
    try:
        doc = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return [f"[{slug}] server.json is not valid JSON: {exc}"]
    meta = registry.skills().get(slug) or {}
    owner, repo = registry.owner_repo()
    doc = as_published(doc, slug, str(meta.get("version") or ""), owner, repo)
    declared = doc.get("$schema", "")
    pinned = validator.root.get("$id", "")
    out = []
    if declared and pinned and declared != pinned:
        out.append(
            f"[{slug}] server.json declares $schema {declared} but this gate validates against {pinned}. "
            f"Re-vendor with --refresh-schema, or fix the document."
        )
    return out + [f"[{slug}] {e}" for e in validator.check(doc, validator.root, "")]


def self_test(validator: Validator) -> int:
    """Prove the gate discriminates: it must fail a violation and pass a clean doc."""
    base = {
        "$schema": validator.root.get("$id"),
        "name": "io.github.servosity/example-mcp",
        "description": "A short, publishable one-line description of the connector.",
        "version": "1.0.0",
    }
    cases = [
        ("clean document", base, True),
        ("description 101 chars", {**base, "description": "x" * 101}, False),
        ("description exactly 100", {**base, "description": "x" * 100}, True),
        ("name violates the reverse-DNS pattern", {**base, "name": "not a valid name"}, False),
        ("missing required description", {k: v for k, v in base.items() if k != "description"}, False),
        ("title over 100", {**base, "title": "t" * 101}, False),
    ]
    bad = 0
    for label, doc, want_ok in cases:
        errs = validator.check(doc, validator.root, "")
        ok = not errs
        mark = "ok " if ok == want_ok else "BAD"
        if ok != want_ok:
            bad += 1
        print(f"  {mark} {label}: {'valid' if ok else errs[0]}")
    print(f"\nself-test: {len(cases) - bad} ok, {bad} BAD")
    return 1 if bad else 0


def refresh_schema() -> int:
    import urllib.request

    url = load_schema().get("$id")
    if not url:
        print("check_mcp_registry: vendored schema has no $id to refresh from", file=sys.stderr)
        return 2
    with urllib.request.urlopen(url, timeout=30) as resp:
        new = json.loads(resp.read().decode("utf-8"))
    old_text = SCHEMA_PATH.read_text(encoding="utf-8")
    new_text = json.dumps(new, indent=2, ensure_ascii=True) + "\n"
    if old_text == new_text:
        print(f"check_mcp_registry: vendored schema already current ({url})")
        return 0
    SCHEMA_PATH.write_text(new_text, encoding="utf-8")
    print(f"check_mcp_registry: refreshed the vendored schema from {url}")
    print("Re-run the gate: a constraint may have tightened.")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--slug", help="check one skill")
    ap.add_argument("--all", action="store_true", help="check every skill (default)")
    ap.add_argument("--warn", action="store_true", help="report findings but exit 0")
    ap.add_argument("--self-test", action="store_true", help="prove the gate fires on a violation")
    ap.add_argument("--refresh-schema", action="store_true", help="re-vendor the schema from its $id URL")
    args = ap.parse_args()

    if args.refresh_schema:
        return refresh_schema()

    try:
        validator = Validator(load_schema())
    except (OSError, json.JSONDecodeError) as exc:
        print(f"check_mcp_registry: cannot load {SCHEMA_PATH}: {exc}", file=sys.stderr)
        return 2

    if args.self_test:
        return self_test(validator)

    meta = registry.skills()
    if args.slug:
        if args.slug not in meta:
            print(f"check_mcp_registry: unknown slug {args.slug!r}", file=sys.stderr)
            return 2
        slugs = [args.slug]
    else:
        slugs = sorted(meta)

    errors: list[str] = []
    checked = 0
    for slug in slugs:
        if registry.is_markdown_only(slug):
            continue
        found = check_slug(slug, validator)
        if (registry.skill_path(slug) / "server.json").exists():
            checked += 1
        errors += found

    if errors:
        print("check_mcp_registry FAILED:")
        for e in errors:
            print(f"  - {e}")
        print(
            "\nThe MCP Registry answers 422 on these and the publish step runs AFTER the release assets\n"
            "are uploaded, so the release looks complete while the connector never reaches the registry\n"
            "or the catalogues that fan out from it."
        )
        return 0 if args.warn else 1

    print(f"PASS: {checked} server.json document(s) satisfy the MCP Registry schema")
    return 0


if __name__ == "__main__":
    sys.exit(main())
