# Surviving a reprint: how connector fixes outlive cli-printing-press

Every connector under `skills/<slug>/cli` is **generated** by
[cli-printing-press](https://github.com/mvanhorn/cli-printing-press) and then
carries connector-specific edits: live-API quirks, synthesized identifiers,
corrected examples, and bug-fix back-ports. The press is upstream and refreshes
on its own cadence; we pull new binaries and **reprint** connectors whenever it
improves.

A reprint regenerates those `DO NOT EDIT` files. The danger is that it can
**silently clobber** a hand-fix - and `go build` / `go test` stay green while
the fix is gone, because nothing tests the live-API behavior the fix encoded.
This happened with axcient: a full reprint reverted the Python-style `id_`
primary-key fallback, reintroducing `all_items_failed_id_extraction` (0 rows)
for appliances/vaults. Only an adversarial review caught it.

This doc is how we keep msp-skills and the press **independent** - fix
connectors at any time, refresh the press at any time - while features,
bug-fixes, and live-API lessons survive.

## Why regen-merge isn't enough

`cli-printing-press regen-merge` preserves whole hand-authored files (`NOVEL`)
and new top-level declarations (`TEMPLATED-WITH-ADDITIONS`). It does **not**
preserve **inline body edits** to generated files - adding `"id_"` to an
existing slice literal, or a `Hand-wired:` block inside an existing function,
is classified `TEMPLATED-BODY-DRIFT` and left for a human to merge by hand. A
blind "overwrite every `DO NOT EDIT` file" reprint drops them. The decl-set
comparison is structural; it can't see that a one-line change inside a function
is load-bearing.

## The three layers (use them in this order)

### 1. Encode it in the spec (best - survives by construction)

If the press can express the behavior as a spec extension, that's the most
reprint-proof home: it becomes *input* to generation, so any press version
regenerates it correctly. The `id_` case is the textbook example - the
generated code itself prints *"Annotate the spec with `x-resource-id` to fix"*.
Prefer `x-resource-id`, `x-pp-resource`, sync-hints, `x-auth-*` over a hand-edit
whenever the press supports it. (For true independence the annotated spec should
travel with the connector; today the spec lives in the press library, so
spec-encoding also means a press change or a vendored spec.)

When the press *can't* express it (e.g. synthesizing an id for a resource that
has none), or you need the fix shipped now, fall through to layer 2.

### 2. Record it in the hand-fix ledger (enforced safety net)

Each connector may carry **`skills/<slug>/handfixes.json`** recording every
edit to a generated file with a grep-able marker. `check_handfixes.py` asserts
every marker is still present (and any banned anti-pattern still absent) and
**fails CI** if a reprint dropped one - turning a silent clobber into a loud,
blocking failure. It runs in the per-skill CI `build` job and in
`verify_all.sh`. Skills without a ledger are a no-op.

Add an entry whenever you hand-edit a generated file:

```json
{
  "slug": "<slug>",
  "doc": "docs/reprint-survival.md",
  "handfixes": [
    {
      "id": "short-kebab-id",
      "summary": "one line",
      "why": "the live-API truth / rationale a future maintainer needs",
      "status": "active",
      "spec_encode_followup": "optional: the x-... annotation this should become",
      "asserts": [
        {"file": "cli/internal/store/store.go", "contains": "\"id_\"", "min_count": 2},
        {"file": "cli/internal/mcp/tools.go", "not_contains": "fmt.Sprintf(\"%v\", v), 1)"}
      ]
    }
  ]
}
```

- `file` is relative to `skills/<slug>/`.
- `contains` + `min_count` (default 1): the marker that must be present.
- `not_contains`: an anti-pattern that must stay gone (e.g. the buggy line a fix removed).
- `status`: `active` = connector-only, a reprint would clobber it; `upstreamed`
  = also fixed in the press, so a reprint from a fixed press regenerates it -
  but the back-port must still be present today. Both are asserted present.

Pick markers that are **specific** (a distinctive substring of the fix), not
generic. When you intentionally change a fix, update its ledger entry in the
same PR.

### 3. Capture the lesson (this doc + press retros)

Process lessons live here. Things the **press itself** should fix generically
(e.g. handle Python-`id_` keys, or have regen-merge detect body-level slice
edits) go upstream as a `/printing-press-retro` issue, so the whole fleet
benefits and we stop carrying the hand-fix.

Unique / transcendent novel commands (e.g. axcient's `health`, `client-rollup`)
are already mostly safe - they're whole hand-authored files regen-merge keeps,
recorded in `.printing-press.json`. A ledger entry asserting "the novel command
still resolves" is a cheap backstop.

## When fixing a connector

1. **Prefer surgical over a full reprint** when the goal is a targeted fix.
   Revert the cli tree to `main`, apply only the fix. A full reprint absorbs all
   upstream improvements but risks clobbering hand-fixes and can't be live-verified
   without credentials.
2. If you *do* full-reprint, **3-way merge every `BODY-DRIFT` file** (preserve
   the hand-fix, apply the template delta) - never blind-overwrite - and run
   `check_handfixes.py` before merging.
3. **Run `python3 tools/maintainer/check_handfixes.py` locally** (it's in
   `verify_all.sh`) and add/refresh ledger entries for any new hand-edit.
4. Link the originating GitHub issue to the ledger entry. Issues are for triage;
   the **ledger** is the source of truth a reprint is checked against.

## Why not "GitHub issues as the source of truth"?

Issues are where bugs arrive and get discussed, but they're the wrong durable
home for reprint-survival: not co-located with the code, no mechanical
enforcement, they close and get buried, and a reprint shouldn't have to parse
prose from a closed issue. The ledger is versioned with the connector and
*checked in CI*; issues link to it.
