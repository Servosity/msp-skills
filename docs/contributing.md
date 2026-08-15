# Contributing a new Skill or MCP server

The shorter contributor checklist lives in [CONTRIBUTING.md](../CONTRIBUTING.md) at the repo root. This page is the longer walkthrough for someone PR-ing their first skill into `msp-skills`.

## When to contribute via PR vs Build Session

- **PR a complete skill** when you already have a working CLI + MCP server for the system in question and just want to publish it under `msp-skills`.
- **Bring the system to a Build Session** when you do not yet have a working CLI / MCP and would value co-building one live. Build Sessions are Thursdays; RSVP at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions).

Both paths land you in the same repo.

## What a skill looks like inside the repo

Every skill is a directory under `skills/<vendor>/` with the following layout:

```
skills/<vendor>/
  README.md             # user-facing landing page (H1 + non-affiliation banner + install + first command)
  SKILL.md              # Claude Code skill entry point with YAML frontmatter
  AGENTS.md             # agent operating contract (when to dry-run, when to use --agent, etc.)
  guide.md              # full command reference
  manifest.json         # MCPB manifest for Claude Desktop one-click install
  mcp-descriptions.json # human-authored MCP tool descriptions (optional but recommended)
  install.sh            # macOS / Linux installer (curls binary from Releases)
  install.ps1           # Windows installer
  mcp-install.md        # MCP wire-up instructions for Claude Desktop / ChatGPT Desktop
  pain-point.md         # optional: what MSP pain this skill closes
  governance.md         # optional: permissions the skill needs and why
```

Use the existing `skills/halopsa/` and `skills/servosity/` directories as the template.

## The other shape: a markdown-only skill

Not every useful skill is a connector. A skill can be instructions plus a few
scripts, with no Go, no compiled binary, and no MCP server. `skills/connect-tool/`
is the worked example:

```
skills/<name>/
  README.md             # user-facing landing page (H1 + banner)
  SKILL.md              # entry point with the same YAML frontmatter fields
  scripts/              # optional, any language
  references/           # optional, deeper docs the agent loads on demand
```

Add `"markdown_only": true` to the skill's entry in `tools/maintainer/skills.json`.
That single flag is load-bearing: it tells the skill-contract check to require only
`SKILL.md` and `README.md`, and tells the build matrix, release queue, CLI-claims
check, release-contract check, and the connector page formula to skip the skill
entirely. Without it, checks fail asking for installers and a binary you were never
going to ship.

The security check still reads any `.py`, `.sh`, or `.ps1` you ship. It looks for
things like `shell=True`, `os.system`, `eval`, `pickle.load`, and a subprocess call
given a command string instead of a list of arguments. Writing scripts the way
`skills/connect-tool/scripts/` does - list arguments, explicit `shell=False` - passes
cleanly.

## What a maintainer finishes for you

Please do not hold a PR back over any of these. They need tooling or authority you
do not have, and a maintainer picks them up after merge:

- **Social preview images and the demo video.** Minted from an internal toolchain.
- **The `live-verified` badge.** Only a real MSP's report against a real tenant
  flips it. An author cannot self-certify, and neither can we.
- **Generated files you could not regenerate**, such as `catalog.json`. Say so in
  the PR.

## If the security check flags your PR

`security-gate` reads `security_suppressions.json` and `dep_allowlist.json` from
`main`, never from your branch. Adding an exception to your own PR therefore has no
effect - that is deliberate, so a PR cannot approve its own exception.

- **A real finding** gets fixed. Most often it is an out-of-date dependency and the
  fix is a version bump (`go get <module>@latest`).
- **A false positive** gets noted in the PR and left alone. A maintainer reviews it
  and merges the exception to `main` first; your PR then goes green on rebase.

## The non-affiliation banner

Every skill for a third-party vendor must open its `README.md` with a non-affiliation banner before any other content. Template:

```
> Unofficial. Community-built Claude Code Skill and MCP server for the
> {Vendor} API. Not affiliated with, endorsed by, or sponsored by {Vendor}.
> {Vendor product names} are trademarks of {Trademark holder}.
```

For first-party skills (where the vendor itself is shipping the skill, like Servosity), use a softer disclosure that names the trademark holder without the "unofficial" wording. The Servosity README in `skills/servosity/README.md` is the example.

See [TRADEMARKS.md](../TRADEMARKS.md) for the full statement on vendor trademarks.

## SKILL.md frontmatter

Required fields:

```yaml
---
name: <vendor>
description: "..."
author: "..."
license: "Apache-2.0"
allowed-tools: "Read Bash"
vendor: "<Vendor>"
---
```

The `name` field is the slug used by skill-capable agents to load this skill. The `description` field is what shows up in agent skill listings; lead with the system name and the vocabulary an MSP would search for.

## Install scripts

Both `install.sh` and `install.ps1` should:

1. Detect OS and architecture.
2. Download BOTH the CLI binary and the MCP binary from `https://github.com/servosity/msp-skills/releases/latest/download/...`.
3. Write them to `~/.local/bin/` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills\` (Windows).
4. Mark executable and clear macOS Gatekeeper quarantine if applicable.
5. Support `DRY_RUN=1` and `MSP_SKILLS_RELEASE_BASE` env var overrides for testing.

Use `skills/halopsa/install.sh` and `skills/halopsa/install.ps1` as the template. The shape is identical across skills; just swap the binary names.

## Catalog

`catalog.json` is regenerated on every PR by `tools/maintainer/build-catalog.py`. To add a new skill to the catalog:

1. Add an entry for your skill to `tools/maintainer/skills.json`. This is the only file you hand-edit; everything else is generated.
2. Run `python3 tools/maintainer/build-catalog.py` locally to regenerate `catalog.json` and the README catalog table.
3. Commit the result. CI fails the PR if you skip this step.

## DCO sign-off

Every commit must be signed off:

```bash
git commit -s -m "Add ConnectWise skill"
```

The `-s` flag appends the `Signed-off-by:` line that asserts you have the right to submit the code under the project license. See the [Developer Certificate of Origin](https://developercertificate.org) for what you are asserting. CI rejects PRs whose commits are not signed off.

## Style

- No em-dashes anywhere. Use ` - ` (hyphen with spaces) or `:` or parentheses instead.
- Use the words MSPs search for: "Claude Code Skill", "MCP server", "Skill". Do not use insider vocabulary like "agent-native CLI" or "MSP capability pack" in user-facing copy.
- Do not name competing distribution platforms in READMEs or docs. Describe what `msp-skills` is on its own terms.

## Local verification before opening a PR

Run the same checks CI runs, rather than hand-rolled greps:

```bash
bash tools/maintainer/ci_guards.sh            # em-dashes, personal paths, secrets
python3 tools/maintainer/check_skill_contract.py   # required files + frontmatter
python3 tools/maintainer/check_vocabulary.py       # the words MSPs search for
python3 tools/maintainer/check_md_links.py         # local links resolve
python3 tools/maintainer/check_no_todos.py         # no leftover placeholders
python3 tools/maintainer/check_security_gate.py --slug <your-slug>

# Regenerate the catalog, then confirm nothing drifted
python3 tools/maintainer/build-catalog.py
git status --porcelain -- catalog.json README.md docs/_data/catalog.json docs/skills/

# Connectors only: install script dry-run
DRY_RUN=1 bash skills/<vendor>/install.sh
```

All of these should pass, and the `git status` line should print nothing.

**On Windows:** run the commands above from WSL or Git Bash (they use `bash`, `grep`, and
`python3`); use `python` if `python3` is not on your PATH. For the install dry-run in
PowerShell, use `$env:DRY_RUN=1; .\skills\<vendor>\install.ps1`. CI runs every one of these
checks on each PR regardless of your OS, so local verification is a convenience, not a
requirement.
