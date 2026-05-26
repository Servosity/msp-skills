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
2. Download BOTH the CLI binary and the MCP binary from `https://github.com/Servosity/msp-skills/releases/latest/download/...`.
3. Write them to `~/.local/bin/` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills\` (Windows).
4. Mark executable and clear macOS Gatekeeper quarantine if applicable.
5. Support `DRY_RUN=1` and `MSP_SKILLS_RELEASE_BASE` env var overrides for testing.

Use `skills/halopsa/install.sh` and `skills/halopsa/install.ps1` as the template. The shape is identical across skills; just swap the binary names.

## Catalog

`catalog.json` is regenerated on every PR by `tools/build-catalog.py`. To add a new skill to the catalog:

1. Add an entry for your skill to the `SKILL_META` dict in `tools/build-catalog.py`. This is the only file you hand-edit; everything else is generated.
2. Run `python3 tools/build-catalog.py` locally to regenerate `catalog.json` and the README catalog table.
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

```bash
# Em-dash check (Unicode U+2014). Should return nothing.
python3 -c "import pathlib; [print(f'{p}:{i+1}') for p in pathlib.Path('.').rglob('*') if p.is_file() and '.git' not in str(p) for i, l in enumerate(p.read_text(errors='ignore').splitlines()) if chr(0x2014) in l]"

# Banned vocabulary
grep -rEn '\bagent-native CLI\b|\bMSP capability pack\b|\bcapability pack\b' .

# Personal paths and emails
grep -rn '/Users/' .
grep -rn '/home/' .

# Catalog regeneration
python3 tools/build-catalog.py
git diff catalog.json README.md  # should be empty after regen

# Install script dry-runs
DRY_RUN=1 bash skills/<vendor>/install.sh
```

All of these should pass / return nothing before you open the PR.
