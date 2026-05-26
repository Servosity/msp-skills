# Install a Claude Code Skill (also Codex CLI, any skill-capable agent)

This page is for users of an AI agent that natively understands the Claude Code Skill format. That includes Claude Code, Codex CLI, and any other agent that loads `SKILL.md`-style files. If you use Claude Desktop or ChatGPT Desktop instead, see [install-mcp.md](./install-mcp.md).

## What gets installed

Two things end up on your machine per skill:

1. The Skill itself: `SKILL.md` plus its supporting files (`AGENTS.md`, `guide.md`, `manifest.json`). Skill-capable agents discover this directory directly.
2. A CLI binary the skill drives (for example, `halopsa-cli` or `servosity-cli`), and the MCP server alongside it.

The install scripts in each skill directory drop the binaries on your PATH. The Skill files stay in `msp-skills/skills/<vendor>/` and your agent points at that location.

## Install in two steps

### 1. Clone or download msp-skills

```bash
git clone https://github.com/Servosity/msp-skills.git
cd msp-skills
```

If you do not want to clone, the `install.sh` / `install.ps1` scripts also work via the one-liners in the root README. Cloning is the cleaner path when you plan to use multiple skills.

### 2. Run the install script for the skill you want

```bash
bash skills/halopsa/install.sh
# or
bash skills/servosity/install.sh
```

Each install script downloads the latest binaries from this repo's GitHub Releases and drops them in `~/.local/bin/` (macOS / Linux) or `%LOCALAPPDATA%\Programs\msp-skills\` (Windows). Re-run later to update.

## Tell Claude Code where the skill lives

Claude Code looks for skills in `~/.claude/skills/` by default. The simplest wire-up is a symlink:

```bash
mkdir -p ~/.claude/skills
ln -s "$(pwd)/skills/halopsa" ~/.claude/skills/halopsa
ln -s "$(pwd)/skills/servosity" ~/.claude/skills/servosity
```

On Windows (PowerShell, run as admin once for symlink privilege):

```powershell
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\.claude\skills" | Out-Null
New-Item -ItemType SymbolicLink -Path "$env:USERPROFILE\.claude\skills\halopsa" -Target "$PWD\skills\halopsa"
New-Item -ItemType SymbolicLink -Path "$env:USERPROFILE\.claude\skills\servosity" -Target "$PWD\skills\servosity"
```

Then start a new Claude Code session and ask:

```
use halopsa
```

Claude Code will load `SKILL.md`, see the trigger phrase matches, and start operating against the HaloPSA API once you have authenticated.

## Codex CLI

Codex CLI consumes the same Skill format. After running the install script and creating the symlinks above (Codex looks in `~/.codex/skills/` by default; adjust the path), Codex picks up the skill the same way.

## Auth and first commands

Per-skill auth instructions live in the skill's README. See [skills/halopsa/README.md](../skills/halopsa/README.md) and [skills/servosity/README.md](../skills/servosity/README.md).

## Troubleshooting

- `halopsa-cli: command not found` after install: install dir is not on PATH. Re-read the installer output; it prints the exact line you need to add to your shell rc file.
- Claude Code does not see the skill: the symlink target is wrong, or you started Claude Code before creating the symlink. Restart Claude Code.
- Permission denied on the install script: `chmod +x skills/<vendor>/install.sh` first.

If you are not sure which agent you have, read [which-agent.md](./which-agent.md).
