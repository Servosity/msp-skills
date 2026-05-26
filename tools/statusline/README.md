# Claude Code Statusline (canonical: Servosity/claude-code-statusline)

> Developer convenience tool, not an MSP skill. It has nothing to do with PSA,
> RMM, or backup workflows - it just makes Claude Code nicer to work in. It lives
> under `tools/` (outside `skills/`) for that reason.

Two-line status line for Claude Code that shows model, cwd, git branch, date/time, and accurate context-window usage with a progress bar.

The canonical source for this statusline is its own repository:

> **[github.com/servosity/claude-code-statusline](https://github.com/servosity/claude-code-statusline)** (MIT licensed, Python)

That repo is the source of truth. The Windows-fixing PRs are merged there. This directory does not vendor a copy; it ships install scripts that download the latest `statusline.py` from the canonical raw URL at install time, so you always get the freshest cross-platform version.

## Install

**macOS / Linux:**

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/tools/statusline/install.sh)
```

**Windows (PowerShell):**

```powershell
iwr -useb https://raw.githubusercontent.com/servosity/msp-skills/main/tools/statusline/install.ps1 | iex
```

The installer:

1. Downloads `statusline.py` from the canonical Servosity repo.
2. Writes it to `~/.claude/statusline.py` on macOS / Linux, or `%USERPROFILE%\.claude\statusline.py` on Windows.
3. Prints the JSON snippet to add to your `~/.claude/settings.json` so Claude Code starts using it.

## Why the install script doesn't vendor a local copy

Two reasons:

- The canonical repo is MIT-licensed; this repository (`msp-skills`) is Apache-2.0. Fetching at install time avoids mixing license terms in committed files.
- Windows-fixing PRs land upstream. If `msp-skills` vendored a snapshot, contributors would have to re-vendor each time, and Windows users would silently get stale code. Pulling at install time avoids that drift entirely.

## Supply-chain note

Be aware of what the installer does before you run it: it downloads and writes a
Python script (`statusline.py`) from the **`main` branch** of the canonical repo
and runs it as your Claude Code status line. That means you are trusting whatever
is on `main` at install time, not a pinned release. If you want to pin a known
version, fork the canonical repo, point the installer's
`MSP_SKILLS_RELEASE_BASE`-style source at your fork or a tagged raw URL, and
re-run. Read [`install.sh`](./install.sh) / [`install.ps1`](./install.ps1) first
if you are cautious about install-time fetches.

## Updating

The install script always pulls `main` from the canonical repo. To refresh your local copy, re-run the installer.

## Reporting bugs

Bugs in the statusline itself belong in the canonical repo: [github.com/servosity/claude-code-statusline/issues](https://github.com/servosity/claude-code-statusline/issues). Bugs in this repo's installer belong here.
