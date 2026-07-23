# connect-tool - connect any tool to your agent, and prove it works

> First-party. A utility Claude Code Skill for the msp-skills catalog, by Servosity. Apache-2.0. It is not affiliated with, endorsed by, or sponsored by OpenCLI, Google, or any vendor you point it at.

Every connector you install has the same last mile: get a credential out of a vendor portal, put it somewhere safe, wire it into the tool, and prove the tool actually works. That last mile is where most setups die, usually with an agent cheerfully reporting "connected" when nothing was ever authenticated.

connect-tool does that last mile for **any** CLI, **MCP server**, or Skill, by driving the Chrome you are already logged into. You watch it happen. The secret goes to the macOS Keychain without ever passing through the model's context. And it will not say "done" until a real authenticated read returns real data from the vendor.

## What makes it different

- **Your browser, your session.** It binds the Chrome tab you already have open and logged in, through the OpenCLI browser bridge. It never spawns a fresh or headless browser, because a spawned browser carries none of your logins and every vendor portal will just show it a login wall.
- **The secret never enters the model's context.** A shell helper reads exactly one DOM node and pipes it straight into the Keychain. The model authors the CSS selector and the destination; it never sees the value. All you get back is a receipt: `len`, `sha256[:8]`, `last4`.
- **It reconciles, it does not just install.** Already connected? It works out whether the right answer is nothing, a token refresh, a scope broadening, a re-auth, or a repair, and does only that.
- **It holds the irreversible.** Any click whose label matches post / publish / save / pay / delete / revoke is refused and surfaced to you, unless that exact verb was pre-approved for that step.
- **It proves the result.** A run is only finished when a read-only authenticated call returns live data. No receipt, no claim.
- **It learns.** Lessons ("this vendor hides the key under Configuration > Integrations") are written to a shared feedback log and injected into the next run, for every target.

## Requirements

connect-tool is **macOS-only**: it stores every secret in the macOS Keychain via `security`.

| Dependency | Why | Install |
|---|---|---|
| macOS | Keychain storage (`security`) | - |
| Google Chrome | the logged-in browser it drives | <https://www.google.com/chrome/> |
| Node.js | to install OpenCLI | `brew install node` |
| OpenCLI | the browser bridge (`opencli browser bind`) | `npm install -g @jackwener/opencli` |
| OpenCLI Chrome extension | the other half of the bridge | <https://github.com/jackwener/opencli/releases> |
| `uv` | runs the Python helpers | `brew install uv` (or use Python 3.12+ directly) |
| `jq` | asserts the JSON receipt in the verify step | `brew install jq` |

`openssl`, `shasum`, and `security` ship with macOS. Nothing else is needed: the Python helpers are stdlib-only and the shell helpers are plain bash.

Check every one of them in a single command:

```bash
bash ~/.claude/skills/connect-tool/scripts/preflight.sh --deps
```

It prints one `OK` or `MISS` line per dependency, with the install command for anything missing, and it checks all of them rather than stopping at the first failure.

## Install

Install the Skill:

```text
/plugin marketplace add Servosity/msp-skills
/plugin install connect-tool@msp-skills
```

Then install the two runtime pieces and reload the extension:

```bash
brew install node uv jq
npm install -g @jackwener/opencli
# download the extension from the OpenCLI releases page, then in Chrome:
#   Extensions > Developer mode > Load unpacked
bash ~/.claude/skills/connect-tool/scripts/preflight.sh --deps
```

## Using it

1. Open Chrome and log in to the vendor portal you want to connect. Leave that tab focused.
2. In your agent, say what you want, for example: *connect the halopsa CLI*, or *my NinjaOne token expired, fix it*, or *add the tickets:write scope*.
3. Answer the scope question it asks up front (target, exact scopes, Keychain names, what it is never allowed to click).
4. Watch it drive your own browser. Approve any hold it surfaces.
5. It ends with a receipt: a real authenticated call and the live data that came back.

Nothing is written to this repo at runtime. State, run logs, and screenshots go to `~/.config/connect-tool/`, and secrets go to the Keychain.

## Verify the helpers on your machine

Every helper ships its own self-check, so you can confirm the security properties rather than trust them:

```bash
cd ~/.claude/skills/connect-tool
for s in scripts/*.sh; do bash "$s" --selfcheck; done
for p in scripts/*.py; do uv run "$p" --selfcheck; done
```

The important ones: `grab_secret.sh` asserts that a captured secret never reaches stdout and that an ambiguous selector fails loudly; `oauth_login.sh` asserts that a token-bearing URL is never echoed; `audit_log.sh` and `state.py` assert that no secret-shaped field can be written to disk.

## How the secret is handled

Three lanes, tried in this order (full detail in `references/security-model.md`):

- **Lane A, OAuth.** The consuming CLI runs its own loopback login and stores its own token. connect-tool only surfaces the non-secret consent URL and drives the click. The token is never rendered and never read.
- **Lane B, displayed key.** One shell process reads exactly one DOM node, pipes it into the Keychain, and prints only a redacted receipt. Any match count other than exactly one is a loud failure, never a guess.
- **Lane C, you paste it.** For one-time-shown or clipboard-only secrets, it hands you the `security add-generic-password` line to run in your own terminal, with a hidden prompt. The value never enters argv or the agent's context.

Consumers are wired through a Keychain-reading wrapper, so the value never lands in a config file, including MCP server configs.

## Files

- `SKILL.md` - the contract the agent follows.
- `references/security-model.md` - the three lanes, residual surfaces, mitigations.
- `references/browser-and-keychain.md` - the OpenCLI command crib and Keychain conventions.
- `references/opencli-bootstrap.md` - installing and un-wedging the browser bridge.
- `references/state-recipes-audit.md` - state, learned recipes, reconcile table, audit-event schema.
- `scripts/` - the shell and Python helpers, each with a `--selfcheck`.

## Markdown-only (no binary)

connect-tool ships Markdown plus stdlib shell and Python helpers. There is no compiled CLI and no MCP server of its own to install, because it has no vendor API of its own: it connects the tools that do. Its `install.sh` / `install.ps1` / `mcp-install.md` are intentionally absent for that reason.
