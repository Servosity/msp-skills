# DataGate Printed CLI Agent Guide

This directory is a generated `datagate-cli` printed CLI. It was produced by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press), so treat systemic fixes as upstream Printing Press fixes first. Keep local edits narrow and document why a generated-tree patch belongs here.

## Local Operating Contract

Start by asking the generated CLI for current runtime truth:

```bash
datagate-cli doctor --json
datagate-cli agent-context --pretty
```

Use runtime discovery instead of relying on a copied command list:

```bash
datagate-cli which "<capability>" --json
datagate-cli <command> --help
```

Add `--agent` to command invocations for JSON, compact output, non-interactive defaults, no color, and confirmation-safe scripting:

```bash
datagate-cli <command> --agent
```

This build is read-only (list/get/search only), so there is no mutating command to
preview with `--dry-run` - every command is already safe to run directly.

For install, auth, examples, and longer product guidance, read `README.md` and `SKILL.md`. This file intentionally stays small so repo-local agents get invariant local guidance without duplicating the generated docs.

## Release Ledger

`CHANGELOG.md` and `.printing-press-release.json` are the public library's per-CLI release ledger. Fresh prints may carry blank skeletons, but the final `YYYY.M.N` CLI release version is assigned only after a publish PR merges in `mvanhorn/printing-press-library`. Do not hand-bump those files or edit `var version = ...` for release bookkeeping; preserve existing ledger files on reprint and let the library workflow stamp the next release.

## Local Customizations

This directory is **generated output** - a fresh print can overwrite the whole tree, so ad-hoc hand-edits don't survive on their own. One hand-fix is recorded today: `internal/config/config.go` reads `DATAGATE_CLIENT_ID` into the generic `Headers` map, because DataGate's auth needs a Bearer token AND a separate required `ClientId` header, and the spec format has no first-class slot for a second required secret header. See [handfixes.json](./handfixes.json) for the guard that fails CI if a reprint drops it.

The entry shape, and the altitude to write it at - a durable reprint-guard, not a changelog - live in the source catalog's `AGENTS.md`, which is the single source of truth; this guide intentionally doesn't duplicate them.
