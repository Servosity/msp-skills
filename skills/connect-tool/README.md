# connect-tool - connect any tool to your agent, and prove it works

> First-party. A utility Claude Code Skill for the msp-skills catalog, by Servosity. Apache-2.0. It is not affiliated with, endorsed by, or sponsored by OpenCLI, Google, or any vendor you point it at.

Every connector you install has the same last mile: get a credential out of a vendor portal, put it somewhere safe, wire it into the tool, and prove the tool actually works. That last mile is where most setups die, usually with an agent cheerfully reporting "connected" when nothing was ever authenticated.

connect-tool does that last mile for **any** CLI, **MCP server**, or Skill, by driving the Chrome you are already logged into. You watch it happen. The secret goes to your operating system's credential store without the complete value ever passing through the agent's context. And it will not say "done" until a real authenticated read returns real data from the vendor.

**Runs on Windows and macOS.**

## What makes it different

- **Your browser, your session.** It binds the Chrome tab you already have open and logged in, through the OpenCLI browser bridge. It never spawns a fresh or headless browser, because a spawned browser carries none of your logins and every vendor portal will just show it a login wall.
- **The complete secret never enters the agent's context.** One helper process reads exactly one DOM node and writes it straight to the credential store. The agent authors the CSS selector and the destination; it never sees the value. All you get back is a receipt: `len`, `sha256[:8]`, `last4`.
- **It reconciles, it does not just install.** Already connected? It works out whether the right answer is nothing, a token refresh, a scope broadening, a re-auth, or a repair, and does only that.
- **It holds the irreversible.** Any click whose label matches post / publish / save / pay / delete / revoke is refused and surfaced to you, unless that exact verb was pre-approved for that step.
- **It proves the result.** A run is only finished when a read-only authenticated call returns live data. No receipt, no claim.
- **It learns.** Lessons ("this vendor hides the key under Configuration > Integrations") are written to a shared feedback log and injected into the next run, for every target.

## Requirements

| Dependency | Why | Windows | macOS |
|---|---|---|---|
| Windows 10+ or macOS | credential storage | Credential Manager | Keychain |
| Google Chrome | the logged-in browser it drives | <https://www.google.com/chrome/> | same |
| Node.js 20+ | to install OpenCLI | `winget install OpenJS.NodeJS.LTS` | `brew install node` |
| OpenCLI | the browser bridge | `npm install -g @jackwener/opencli` | same |
| OpenCLI Chrome extension | the other half of the bridge | one click, Chrome Web Store | same |
| `uv` | runs the helpers, and supplies its own Python | `irm https://astral.sh/uv/install.ps1 \| iex` | `brew install uv` |

Nothing else. The helpers are stdlib-only Python, so there is no `jq`, no `openssl`, no bash, and no separate Python install to manage: `uv` downloads the interpreter it needs on first run.

Check every dependency in one shot. The easy way is to ask your agent, since it already knows where the Skill landed:

> run the connect-tool dependency check

It prints one `OK` or `MISS` line per dependency with the install command for anything missing, and checks all of them rather than stopping at the first failure.

## Install

### Windows

```powershell
# 1. Dependencies
winget install OpenJS.NodeJS.LTS
powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"
npm install -g @jackwener/opencli

# 2. The OpenCLI Chrome extension (one click, no developer mode)
start https://chromewebstore.google.com/detail/opencli/ildkmabpimmkaediidaifkhjpohdnifk

# 3. The Skill itself (no Git required)
irm https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connect-tool/bootstrap.ps1 | iex
```

Or, if you prefer the plugin marketplace and have Git installed, in Claude Code:

```text
/plugin marketplace add https://github.com/servosity/msp-skills.git
/plugin install connect-tool@msp-skills
```

The `.git` suffix matters: it makes Claude Code clone over HTTPS rather than SSH, which a fresh machine has no key for.

### macOS

```bash
brew install node uv
npm install -g @jackwener/opencli
open https://chromewebstore.google.com/detail/opencli/ildkmabpimmkaediidaifkhjpohdnifk
curl -fsSL https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connect-tool/bootstrap.sh | bash
```

Then restart Claude Code so it picks up the new Skill, and ask it to run the dependency check.

If the extension shows as missing later in a session, click the reload arrow on the OpenCLI card in `chrome://extensions/`. Its background worker goes dormant when idle, and restarting the daemon alone does not wake it.

## Using it

1. Open Chrome and log in to the vendor portal you want to connect. Leave that tab focused.
2. In your agent, say what you want: *connect the halopsa CLI*, or *my NinjaOne token expired, fix it*, or *add the tickets:write scope*.
3. Answer the scope question it asks up front (target, exact scopes, credential names, what it is never allowed to click).
4. Watch it drive your own browser. Approve any hold it surfaces.
5. It ends with a receipt: a real authenticated call and the live data that came back.

Nothing is written to this repo at runtime. State, run logs, and screenshots go to `%LOCALAPPDATA%\Servosity\connect-tool\` on Windows or `~/.config/connect-tool/` on macOS, in a directory only you can read. Secrets go to the OS credential store.

## Verify the helpers on your machine

Every helper ships its own self-check, so you can confirm the security properties rather than trust them. Ask your agent to *run every connect-tool self-check*, or from the skill directory:

```
uv run scripts/grab_secret.py --selfcheck
uv run scripts/oauth_login.py --selfcheck
uv run scripts/credstore.py --selfcheck
```

The important ones: `grab_secret` asserts a captured secret never reaches stdout, that an ambiguous selector fails loudly, and that the value really did land in the credential store; `oauth_login` asserts a token-bearing URL is never surfaced and that no raw output is written to disk; `audit_log` and `state` assert no secret-shaped field can be written anywhere.

## How the secret is handled

Three lanes, tried in this order (full detail, including what this does **not** claim, in `references/security-model.md`):

- **Lane A, OAuth.** The consuming CLI runs its own loopback login and stores its own token. connect-tool navigates your browser to the consent page and drives the click. The token is never rendered, never read, and never written to disk.
- **Lane B, displayed key.** One process reads exactly one DOM node and writes it into the credential store, printing only a redacted receipt. Any match count other than exactly one is a loud failure, never a guess.
- **Lane C, you paste it.** For one-time-shown or clipboard-only secrets, it hands you a command to run in your own terminal with a hidden prompt. The value never enters a command line or the agent's context.

On Windows the credential goes to Credential Manager through a direct `CredWriteW` call, so the secret is passed as a memory buffer and never appears in any process command line. Consumers are wired through a launcher that reads the credential at start-up, so the value never lands in a config file, including MCP server configs.

## Files

- `SKILL.md` - the contract the agent follows.
- `references/security-model.md` - the three lanes, what is claimed and what is not.
- `references/windows.md` - what differs on Windows, and what is not yet verified there.
- `references/browser-and-keychain.md` - the OpenCLI command crib and credential conventions.
- `references/opencli-bootstrap.md` - installing and un-wedging the browser bridge.
- `references/state-recipes-audit.md` - state, learned recipes, reconcile table, audit events.
- `scripts/` - the helpers, each with a `--selfcheck`.
- `bootstrap.ps1` / `bootstrap.sh` - install the Skill and check dependencies, no Git required.

## No compiled binary

connect-tool ships Markdown plus stdlib Python helpers. There is no compiled CLI and no MCP server of its own to install, because it has no vendor API of its own: it connects the tools that do. Its `install.sh` / `install.ps1` / `mcp-install.md` are intentionally absent for that reason, and the catalog lists it as markdown-only on the same basis.
