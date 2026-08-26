# Contributing to msp-skills

**The most valuable thing you can send us is not code. It is a receipt.**

Most connectors here were built without access to the vendor's system. They pass
every mechanical gate we can run - they build, their installers resolve, and a
check reads every documented command back against the binary's real command
surface - but a gate cannot tell us what a live tenant does. Only you can. So the
top rung of this ladder is one sentence from an MSP who ran a connector against
their own tenant and watched it work.

Everything below is ordered by how much it helps, not by how hard it is.

## Pick your door

| Rung | You want to... | Do this | Need to code? |
| --- | --- | --- | --- |
| 1 | Tell us a connector worked against your real tenant | [Fill in the "it works" form](https://github.com/servosity/msp-skills/issues/new?template=it-works.yml) | No |
| 2 | Tell us one misbehaved against your real tenant | [Fill in the bug-report form](https://github.com/servosity/msp-skills/issues/new?template=bug-report.yml) | No |
| 3 | Fix a command, a doc, or a live-API quirk | Open a small pull request | A little |
| 4 | Add a connector or a Skill you (or your AI) built | Open a pull request | Yes |

Two more doors, for a system that is not here yet:

| You want to... | Do this | Need to code? |
| --- | --- | --- |
| Ask for a tool we do not cover | [The request form](https://github.com/servosity/msp-skills/issues/new?template=skill-request.yml) - see [the 90-second walkthrough](docs/requesting-a-skill.md) | No |
| Build one with us, live | Join a free Build Session at [compoundingteams.com/build-sessions](https://compoundingteams.com/build-sessions) and bring your vendor | No - we drive |

## Rung 1: send an "it works" receipt

A connector ships marked **awaiting live verification**. That is not a warning
label and it is not a defect - it means the only thing still missing is an MSP.
The connector is ready to run today; nobody has yet told us what happened when
they ran it.

Your report is what changes that. Every **Live-verified** badge records where its
receipt came from. Some are first-party: Servosity ran that connector against our
own production tenant and said so, and the badge carries that as its source. The
rest came from MSPs outside this project. A first-party receipt is a real
receipt, but it is the one kind we can always hand ourselves - which is exactly
why an outside MSP's receipt is the most valuable thing anyone sends us. It is
the one thing we cannot give ourselves.

1. Run any connector against your real tenant. One read command is enough.
2. [Fill in the "it works" form](https://github.com/servosity/msp-skills/issues/new?template=it-works.yml).
   It asks which skill, what you ran, and what came back.
3. The **Live-verified** badge flips on that connector's page, the receipts wall,
   and the catalog - and you get the public credit for it.

That is roughly 60 seconds of your time in exchange for every future MSP knowing
the thing actually works. See the current state of every connector on
[the receipts wall](https://msp-skills.compoundingteams.com/verified/).

Prefer not to use GitHub? Email hello@servosity.com with the connector name and
your MSP. Plain text is perfect.

## Rung 2: report a bug with real-tenant evidence

A bug you hit against a live tenant is worth more than a bug we imagine. It tells
us something the vendor's API does that no spec or gate revealed.

[The bug-report form](https://github.com/servosity/msp-skills/issues/new?template=bug-report.yml)
asks for what matters:

- The skill name and the exact command you ran.
- The exact error or the wrong output (redact tenant names, IDs, and keys).
- Your OS and your agent: Claude Code, Codex, Claude Desktop, or ChatGPT Desktop.

A real-tenant bug report is treated as a first-class contribution, and fixing one
usually flips that connector to live-verified in the same pass.

Found a **security** problem? Email security@servosity.com instead of filing it
publicly.

## Rung 3: hand-fixes and docs pull requests

Small, surgical, and welcome. A corrected example, a clearer sentence, a flag
that was documented wrong: open the pull request.

One rule matters more than the rest. Connectors under `skills/<slug>/cli` are
**generated**, and regenerating one can silently undo a hand-fix: the build and
the tests stay green while the live-API behavior your fix encoded quietly
disappears.

So if you hand-edit a file marked `DO NOT EDIT`, record it in that connector's
`skills/<slug>/handfixes.json`. A check then fails if a future regeneration drops
it. Prefer a small targeted fix over regenerating the whole connector. Full
rationale and workflow: [docs/reprint-survival.md](docs/reprint-survival.md).

## Rung 4: add a connector or a Skill

### What you do not have to get right

You are not expected to produce a finished, polished skill. A maintainer finishes
these for you after your pull request is merged, so please do not hold your work
back over them:

- **Social preview images and the demo video.** These are minted from an internal
  toolchain you do not have. Leave them out.
- **The `live-verified` badge.** Do not set it on your own pull request. It is
  flipped from a report of a real run against a real tenant. The badge records
  the receipt's source - first-party, or a report from outside this project - and
  a link to the evidence, and it names the verifier when the report includes one.
- **Anything the automated checks generate**, like `catalog.json`. If you cannot
  regenerate it, say so in the pull request and we will.
- **A perfect first commit.** We would rather see the working thing and iterate
  with you in review.

If a check fails for a reason that is not about your code, that is a bug on our
side and we want to hear about it. Two contributors hit exactly that in August
2026 and both were right.

### The two shapes a skill can take

Pick whichever describes what you built. Both are first-class.

**1. A connector** - a Go CLI plus an MCP server for a vendor's API, usually
generated by cli-printing-press. Look at `skills/halopsa/` for the full layout:

```
skills/<vendor>/
  SKILL.md              # entry point, with the frontmatter fields listed below
  README.md             # landing page: title, then the non-affiliation banner
  AGENTS.md             # how an agent should operate the tool
  guide.md              # full command reference
  cli/                  # the Go source
  manifest.json         # one-click install for Claude Desktop
  install.sh            # macOS and Linux installer
  install.ps1           # Windows installer
  mcp-install.md        # how to wire the MCP server up
  governance.md         # which commands are read, write, or destructive
  pain-point.md         # the MSP pain this closes
```

Both installers detect OS and architecture, download the CLI and MCP binaries
from the repo's Releases, write them to `~/.local/bin` (or
`%LOCALAPPDATA%\Programs\msp-skills\` on Windows), clear macOS Gatekeeper
quarantine, and honor `DRY_RUN=1`. Copy `skills/halopsa/install.sh` and
`skills/halopsa/install.ps1`, then replace every HaloPSA-specific string in them.
There are more than the binary names:

- **The skill slug.** `SKILL="halopsa"` in `install.sh`, `$Skill = "halopsa"` in
  `install.ps1`. This is the one that bites: it is the release-tag prefix the
  script searches for, so a copy that misses it resolves HaloPSA's release
  instead of yours.
- **The binary names.** `CLI_BIN` and `MCP_BIN` in `install.sh`; `$CliBin` and
  `$McpBin` in `install.ps1`, both ending in `.exe`.
- **The documentation URLs printed at the end.** Both scripts hard-code
  `skills/halopsa#readme` and `skills/halopsa/mcp-install.md` in their closing
  "Next:" block, and `install.ps1` also prints `halopsa-cli --version` as a
  literal rather than through `$CliBin`.
- **The header comments**, which name the skill and its `halopsa-v*` tag pattern.

**2. A markdown-only skill** - instructions, and optionally some scripts. No Go,
no compiled binary, no MCP server. `skills/connect-tool/` is the example:

```
skills/<name>/
  SKILL.md              # entry point, same frontmatter fields
  README.md             # landing page: title, then the banner
  scripts/              # optional, any language
  references/           # optional, deeper docs the agent loads on demand
```

A markdown-only skill needs `"markdown_only": true` on its entry in
`tools/maintainer/skills.json`. That one flag tells every automated check to stop
expecting installers, a binary, and a release - none of which you have. If you
forget it, the checks will fail in confusing ways, so it is worth a second look.

### Registering your skill

`tools/maintainer/skills.json` is the only file you hand-edit; everything else is
generated from it. Add your entry, then run
`python3 tools/maintainer/build-catalog.py` and commit the result.

### What the automated checks require

These run on your pull request. Each one exists because something went wrong once.

- **Sign-off on every commit** - `git commit -s`. This adds one line saying the
  code is yours to contribute under our license. If you forget, the check tells
  you the single command that fixes it.
- **`SKILL.md` frontmatter** with `name`, `description`, `allowed-tools`, `author`,
  `license`, and `vendor`. Frontmatter is the block between `---` lines at the top
  of the file.
- **A banner at the top of your `README.md`**, right after the title (template
  below).
- **No em-dashes** anywhere. Use ` - `, a colon, or parentheses.
- **No personal paths, emails, or API keys** in any committed file.
- **The words MSPs search for.** Say "Claude Code Skill", "Skill", "MCP server",
  "MCP". Avoid insider jargon in anything a customer reads.
- **No comparisons to other distribution platforms.** Describe what this is on its
  own terms.

Run them all locally with one command before you push:

```bash
bash tools/maintainer/verify_all.sh
```

### If the security check flags your pull request

`security-gate` reads its policy from `main`, not from your branch. That means
**you cannot approve your own exception** - adding one to your branch has no
effect, which is deliberate and not something you did wrong.

So when it flags something:

- **If it is a real finding**, fix it. Most often it is an out-of-date dependency,
  and the fix is a version bump (`go get <module>@latest`).
- **If you believe it is a false positive**, say so in the pull request and leave
  it. A maintainer reviews it and, if you are right, merges the exception to
  `main` first. Your branch then goes green on rebase.

Either way, do not let it block you from opening the pull request. Tell us what
you are seeing.

## License and sign-off

This project is [Apache 2.0](./LICENSE). By contributing you agree your work is
licensed the same way.

We use the [Developer Certificate of Origin](https://developercertificate.org)
rather than a contributor agreement, which means there is no paperwork to sign.
You assert the right to contribute by signing off each commit:

```
git commit -s -m "your message"
```

If you already made commits without it:

```
git rebase --signoff origin/main
git push --force-with-lease
```

## Using a vendor's name

When your Skill or MCP server talks to a third-party vendor's API:

- Refer to the vendor **descriptively** ("the HaloPSA API", "the ConnectWise REST
  endpoint"). Nothing that implies they endorsed, sponsored, or partnered on it.
- No vendor logos. Text only.
- No "Official", "Certified", or "Partner" in the name, description, or copy.
- Put the non-affiliation banner in your `README.md` as a blockquote directly
  under the `#` title. The check (`tools/maintainer/check_skill_contract.py`)
  looks for the first `>` line within the first four non-empty lines after the
  H1, so nothing long may sit between the title and the banner:

```
> Unofficial. Community-built Claude Code Skill and MCP server for the
> {Vendor} API. Not affiliated with, endorsed by, or sponsored by {Vendor}.
> {Vendor product names} are trademarks of {Trademark holder}.
```

  For a first-party skill (the vendor itself shipping it, like Servosity), use a
  softer disclosure that names the trademark holder without the "unofficial"
  wording. `skills/servosity/README.md` is the example.
- "Servosity" and "Compounding Teams" are trademarks of Servosity Inc. Please do
  not use them in your own skill name or branding.

Full statement: [TRADEMARKS.md](./TRADEMARKS.md).

## Still not sure?

Open an issue anyway. A question that turns out to be a misunderstanding still
tells us the docs were unclear, and that is useful.

Thank you for helping make MSP work less painful and more compounding.
