# Preserve Acronis partner sync contracts (#325)

This patch is derived from the official Acronis Task, Account Management and
Agent API schemas, with regression fixtures in acronis_contract_test.go.

Preserve root discovery through the authenticated client identity; never pick an
arbitrary accessible tenant. Include agents and per-tenant billing resources.
Preserve compound billing identity across tenant, application, name, edition and
infrastructure. Map nested task tenant UUID/result code into rollup columns.

Both CLI and direct MCP task lists translate wire filter names, normalize sort
syntax, follow paging.cursors.after (also cursors.after), and filter exact tenant
UUIDs locally only after complete enumeration. Remote search sends text and tenant.

Serialize complete syncs and rollup readers for a database. Rebuild unfiltered
snapshots, persist incomplete status before writes, and refuse certification after
failed, denied, malformed, capped or dropped data. Never hide typed projection
failures. Old mirrors require a new complete sync. Do not infer UUIDs from names
when the Agent API returns a legacy numeric tenant ID.

Login resolves standard regional API URLs consistently and validates the datacenter
as a single DNS label before constructing either token or API URLs. Preserve
explicit custom base URL overrides. Parameter-validation 400s are not auth errors.

The active handfixes.json entry carries executable marker checks. Reapply this
intent and rerun the fixtures after a reprint; template version alone is no proof.
