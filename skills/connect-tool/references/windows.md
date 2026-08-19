# Windows notes: what differs, and what is not yet proven

connect-tool runs on macOS and Windows from one codebase. Every helper is Python, invoked
identically on both platforms with `uv run <skill-dir>/scripts/<helper>.py`. This file
records the Windows-specific behavior and, honestly, the parts that have not been exercised
on real Windows hardware yet.

## Shell: there may not be one you expect

On native Windows, Git for Windows is **optional** for Claude Code. Without it there is no
Bash tool at all and Claude Code uses the **PowerShell** tool. That is why:

- the skill's `allowed-tools` includes `PowerShell` as well as `Bash`
- no instruction in this skill uses bash syntax (`VAR=value cmd`, `$(...)`, `$HOME`,
  `xargs`, shell loops). A `uv run <script>` line is identical in bash, PowerShell, and cmd
- helpers find their own directory with `Path(__file__)`, never by searching the filesystem

WSL is a different situation and is not the supported path: the OpenCLI daemon would run in
Linux while Chrome and its extension run in Windows, so the browser bridge would have to
cross the WSL network boundary. Run Claude Code natively on Windows.

## Where things live

| | macOS | Windows |
|---|---|---|
| state, runs, screenshots | `~/.config/connect-tool/` | `%LOCALAPPDATA%\Servosity\connect-tool\` |
| launcher shims | `~/.local/bin/<name>` | `%LOCALAPPDATA%\Servosity\connect-tool\bin\<name>.cmd` |
| credentials | Keychain | Credential Manager |
| learned lessons | `~/.claude/learning/feedback.jsonl` | same path under `%USERPROFILE%` |

Override the root with `CONNECT_TOOL_HOME`, the launcher dir with `CONNECT_TOOL_BIN`.

**Directory permissions.** `chmod` is a no-op on Windows, so `ctplatform.secure_mkdir`
breaks inheritance and grants only the current user and SYSTEM, via `icacls`. Only paths
are ever passed to `icacls`, never a secret. If it cannot lock the directory down it says
so on stderr rather than pretending.

**PATH.** The launcher directory is not on PATH by default, and adding it does not affect
an already-running Claude Code. After minting a launcher, either call it by its full path
or restart Claude Code.

## Credentials

Windows Credential Manager via `ctypes` calling `CredWriteW` / `CredReadW` / `CredDeleteW`.
Rationale, limits, and the comparison against the macOS path are in `security-model.md`.
Entries appear in the Credential Manager UI as
`Servosity/connect-tool/<service>/<account>`, so a user can see and revoke them.

## The launcher shim

`mint_wrapper.py` writes a `.cmd` file that contains **no credential logic at all**. It
calls back into `mint_wrapper.py --launch`, which reads the credential in-process and
passes it to the consumer as an environment variable. Nothing sensitive is ever written
into generated script text or handed to `cmd.exe` on a command line.

For MCP wiring, point the MCP config at an actual executable (an absolute `uv.exe` plus the
script path). A `.ps1` is not reliably executable as an MCP command, and a `.cmd` cannot be
assumed to work with shell-free process spawning.

## Verified vs unverified

Verified on macOS: all 13 helper self-checks, including a live Keychain round-trip.

**Reported failing in the field:**

1. `opencli browser <slug> bind` driving the tab you already had focused. Issue
   [#266](https://github.com/Servosity/msp-skills/issues/266) (reporter @Xenith-B, Windows 11,
   rotating a HaloPSA client secret): a new blank Chrome window opened instead of the
   signed-in tab, and the run was abandoned. **This one is not Windows-specific and not ours**
   - the bridge extension manages its own container windows by design
   (<https://github.com/jackwener/opencli/issues/2202>, observed on macOS). The mechanism, the
   reassurance to give the user, and the recovery order are in
   `references/browser-and-keychain.md`. Still open on Windows: whether `bind` works *at all*
   there once the container-window behavior is accounted for.

**Also worth knowing before you debug a Windows bridge failure:** if OpenCLI was installed as
the **OpenCLIApp desktop** app rather than via `npm install -g @jackwener/opencli`, every
`opencli` command can return **silently with no output and no error** while `opencli doctor`
reports the runtime healthy - the `opencli.cmd` stub under `WindowsApps` fails to forward to
the app-managed Node runtime (<https://github.com/jackwener/opencli/issues/2102>, open). A
silent zero-output CLI looks exactly like "the skill is broken." Check
`Get-Command opencli` and prefer the npm global install this skill documents.

**Not yet verified on Windows** (needs a real Windows box; do not claim these work):

1. A Git-less `/plugin marketplace add https://github.com/servosity/msp-skills.git`.
   Claude Code's marketplace install is described as a clone/pull; whether it works with no
   Git installed is untested. `bootstrap.ps1` exists as the no-Git path.
2. `CredWriteW` / `CredReadW` round-trip, and the entry appearing in the Credential Manager
   UI.
3. Whether the detached OAuth child survives the Claude Code tool call. Windows job objects
   can terminate descendants; `CREATE_BREAKAWAY_FROM_JOB` may be required and can be denied
   by policy. If Lane A fails this way, Lane B and Lane C are unaffected.
4. All self-checks under the PowerShell tool with no Git Bash present.
5. Behavior under Constrained Language Mode or WDAC. `ctypes` sidesteps the PowerShell
   restriction, but a policy that blocks uv's downloaded Python or Node entirely is an
   administrator conversation, not something to work around.

When one of these is exercised, move it out of this list and say what the receipt was.
