# Maintainer tooling

These scripts are for **maintainers of this repository**, not for MSP users. They generate
the catalog, compute the release build matrix, and run the CI gates (skill contract, markdown
links, release contract, DCO sign-off, repo hygiene). If you just want to install a skill or
the statusline, you do not need anything in this folder - see the root
[README.md](../../README.md) and [`tools/statusline/`](../statusline/).

| Script | What it does |
| --- | --- |
| `verify_all.sh` | One command, one verdict: would CI pass, and is it ready for a PR? No arguments checks the full fleet; `--slug <s>` (repeatable) checks those skills; `--changed [--base origin/main]` checks what the branch changed, scoped by CI's own classifier (a shared-machinery change checks the whole fleet, as CI does). Heavy per-connector checks run for the connectors in scope, cheap fleet-wide checks for everything; a fleet failure only in other connectors is listed under `out_of_scope_failures`, never dropped. It checks the commit `git add -A` would make (git builds it in a private index; the developer's index is untouched), in a clean clone with no global git config. Writes `.verify/verdict.json` (checks, verdict, seconds, head, snapshot and base SHAs, scope). |
| `verify_connector.sh` | The per-connector half of `verify_all.sh`. CI's whole Linux build row (build, vet, cross-compile, CLI claims, hand-fix ledger, MCP gate, doctor and `go test -count=1`, on linux/amd64 with cgo on) runs in that context: natively on such a host, else in a `golang:<toolchain>` linux/amd64 container, always on a Linux disk (a private Docker volume) holding a depth-1 clone of the commit plus working-tree changes, else it fails closed. The Windows predictors (vet, test compile and link, Unix-only test lint) run with cgo off over a copy of the Windows job's sparse checkout, inside the same container in Docker mode, and fail closed when turning cgo on would change any file Go compiles. Every Go command gets an explicit environment, nothing inherited. |
| `verify_scope.py` | Scope, failure attribution and verdict for `verify_all.sh`, plus `cgo-context` (does cgo on vs off change what Go compiles), `ci-go` and `gowork` (the Go and go.work CI uses), `scanner-pins` (security-gate.yml's scanner pins) and `path-collisions` (paths some OS cannot check out, and content that points outside the repository); `--self-test` proves each. |
| `check_no_pp_leak.py` | No connector's own `-pp-` name in a user-facing file, a Go string or a hand-fix `contains` marker (a `not_contains` marker is honored). `--base` makes it a ratchet: lines already present at the base are listed, not blocking. |
| `check_unix_test_asserts.py` | Lints connector tests for assertions that only hold on Unix (POSIX errno text, mode bits, `#!` stubs, files left open, GOOS-blind literals, a `time.Now()` key on `INSERT OR IGNORE`), the classes the Windows CI job used to find only after a PR. Ledgered known Windows failures are reported, not blocking. |
| `build-catalog.py` | Regenerates `catalog.json`, the README catalog table and generated blocks, the docs site pages, the issue-form connector dropdowns, and each skill README's `.mcpb` pin. `--released-tags FILE` (catalog.yml passes the complete-published release list) keeps a pin from ever naming a tag whose release is a stranded draft or is missing an asset. |
| `release_matrix.py` | Prints the GitHub Actions build matrix (skills x os/arch). `--changed-only <base>` (merge-base; ci.yml PRs) and `--changed-since <sha>` (two-point diff; ci.yml pushes, from `github.event.before`) scope it to the skills a change-set touched, so a docs-only or catalog-bot commit builds nothing and a build-machinery edit builds the fleet. `--self-test` proves the classifier both ways. |
| `release_state.py` | Reports each skill's release state (up-to-date, version-pending, binary-pending, never-released) and writes `docs/_data/pending.json`. `--released-tags FILE` replaces "the tag exists" with "the tag names a published release". |
| `check_skill_contract.py` | Asserts every skill has the required frontmatter and files. |
| `check_release_contract.py` | Asserts install scripts and release assets agree on names. |
| `check_md_links.py` | Verifies relative Markdown links resolve. |
| `check_dco.sh` | Verifies every commit carries a DCO `Signed-off-by` line. |
| `check_mcp_gate.py` | Boots a skill's MCP server over stdio and asserts the tenant gate does not default-deny. Probes the read-only tools the gate wraps in-process; skips Cobra mirrors (the child CLI gates those) and destructive tools. Needs no credentials. |
| `check_mcp_file_flags.py` | Proves an MCP tool call cannot name a file or directory on the MCP host. The Go analyzer in `mcp_file_flags/` derives, from data flow in the connector's code (not from flag names), every flag whose value can start a filesystem path; the script then boots the real MCP server beside a recording stand-in for the CLI and asserts each such flag is absent from every mirrored tool's schema and refused by name when supplied, that an ordinary flag still reaches the CLI, and that no in-process tool hands request data to a filesystem call. `--slug`, `--all`, `--jobs N`. The analyzer's own both-directions proof is `go test ./...` in `mcp_file_flags/`. |
| `check_command_tree.py` | Compiles each connector's real `cli.RootCmd()` tree (through `go build -overlay`, so nothing is written into the connector) and fails when a destructive command (delete, cancel, purge, disable, reboot, ...) still answers to the hidden alias `create`. Also reports, without failing, command-local flags that shadow a global flag such as `--agent`. `--self-test` proves the classifier both ways. |
| `class_harness.py` | Black-box harness for the recurring connector bug CLASSES. Drives the BUILT `<slug>-cli` / `<slug>-mcp` against a local fake HTTP server, connector-agnostic (surface from the binary's `agent-context`, wiring from `manifest.json`; no per-connector Go imports). Runs three class tests (`class_secret_echo.py`, `class_auth_wire.py`, `class_write_safety.py`). `--slug X` checks one connector (the CI matrix row); `--all --report` prints per-class fleet counts; `--self-test` proves each class fires on a broken shape and is silent on a healthy one; `--seed-baseline` / `--update-baseline` / `--assert-baseline-shrinks` manage the shrink-only `class_baseline.json`. See [Class tests](#class-tests). |
| `class_baseline.json` | Connectors that FAIL a class today, per class. Same burn-down model as `security_gate_baseline.json` (#429): a listed connector reports, an unlisted one fails CI, the list only shrinks. |
| `ci_guards.sh` | Repo hygiene: no em-dashes, no personal paths, no obvious secrets. Decides on each guard's output, so a file missing from disk cannot turn a violation into a pass; `--self-test` proves it. |
| `check_press_metadata.py` | Validates `skills/<dir>/press/`, the press's own `.printing-press.json`, `tools-manifest.json` and spec that onboard.py keeps beside `cli/`: only those files, parseable, version and print (`generated_at`, `spec_checksum`) agreeing with `manifest.json`, the spec matching its checksum, nothing secret-looking or personal in the text or any decoded view of it (backslash escapes, percent-encoding, HTML entities), and a spec only with `publication.json`'s proof that its source is public (its bytes already in the repo's history outside press/, or an http(s) spec URL the provenance records that answered without credentials when it was vendored); withheld files carry their reason there. A connector without the folder is counted in the coverage line, not failed. `--base` also fails a connector the change ADDS when it was printed below `press_floor.json`. `--inspect DIR` is onboard.py's per-file verdict. `--self-test`. |
| `press_floor.json` | The lowest cli-printing-press version a new connector may be printed with (upstream's own declared minimum, with the commit it was read at), read by `check_press_metadata.py` and onboard.py. `not_press_generated` exempts a connector the press never generated (read from the base, so a PR cannot exempt itself). It rises to upstream's current floor once the paused 4.33.5 prints have landed. |
| `verify_live.py` | Flips a live-verified badge, or revokes it. A flip records the release it verified and the `cli_hash.py` hash of `cli/` at that tag (`--verified-version` when the report names it, else the newest release cut before `--reported-at`); `--bind` sets that on an existing badge. The catalog then reads "verified on vX.Y.Z" once the shipped code changes, and `check_registry_state.py` proves every recorded hash is the tag's code. |
| `gen_governance.py` | Drafts a skill's `governance.md` from its CLI surface. |
| `registry.py` | Shared loader for `skills.json` (the single source of truth for skills). |
| `skills.json` | Per-skill metadata: owner/repo, binary names, vendor, status. |
| `release_batch.sh` | Runs a release batch in a throwaway worktree, pushes the version stamp, and prints probe-gated tag commands. Never tags. |
| `merge_when_green.py` | Merges one PR only when it is green on the exact commit, on current main: per-branch lock, squash, rebase, registry-aware conflict handling, log-classified CI failures, review approvals bound to the change. See [Merging, tagging and stamping](#merging-tagging-and-stamping). `--self-test` proves it against throwaway repos. |
| `resolve_registry.py` | Finishes a conflicted rebase or merge: per-entry three-way merge of `skills.json`, `marketplace.json` and `cards.yaml`, main's copy of derived files, regenerate, re-check, print the final registry diff. Stops on any other conflict. `--self-test`. |
| `cut_tags.sh` | Cuts release tags one per push behind the pipeline probe and the CHANGELOG check; `--receipts` proves each release sealed complete and sweeps for drafts. `--self-test`. |
| `stamp_release.py` | Stamps `cli_hash_at_release` from the tagged tree once a release has sealed, and dates its CHANGELOG heading. `catalog.yml` runs it in the post-release bot commit. `--self-test`. |
| `check_release_pipeline.py` | Refuses a SHA (`--sha`) or a whole tag (`--tag T --sha S`) that must not be released. Run it before every `git tag`. |
| `burned_versions.json` | Version numbers a destroyed release already spent. Retired forever; never cut again. |
| `hooks/pre-push` | Optional hook: refuses a release-tag push the probe refuses. Install with `git config core.hooksPath tools/maintainer/hooks`. |

## Class tests

The factory's review loops converge only once a whole bug CLASS is fixed, not one instance
per round (the saas-alerts 45-round and synology 54-round secret-scrub loops). `class_harness.py`
turns each recurring class into one automatic test that runs before any review, so a reviewer
never spends a round rediscovering a member of a class a gate already covers.

It is **black box**: every connector is its own Go module, so the harness drives the BUILT
`<slug>-cli` and `<slug>-mcp` (stdio MCP) against a local fake HTTP server. It imports no
connector Go package and hard-codes no per-connector names - the command surface comes from the
binary's own `agent-context` JSON (which lists hidden groups, unlike `--help`), and the credential
and base-URL wiring from `skills/<slug>/manifest.json`. It is stdlib Python only.

Every class is proven BOTH directions. `--self-test` (run in CI's guards job) drives each class's
decision functions on a broken shape and on a healthy one; the classes also fire on real
shipped-today connectors (listed in `class_baseline.json`) and are silent on fixed ones. The PR
that added the harness records a real past commit per class whose fix the class tracks.

Ground truth over wording: the fake server records every request, so a finding is a fact about the
wire (a credential arrived at a second host, a write reached the API under read-only, a dry run
sent a request), never an inference from an error message. Each probe runs against a fresh, empty
HOME, so a token one class caches cannot mask another class's finding. A connector a class cannot
exercise gets an advisory `NOTE:` - reported, never baselined, never counted as a pass.

### One table that pairs every class with what enforces it

| Class (rule) | What enforces it | Fires on / silent on (measured) |
| --- | --- | --- |
| **Secret echo** - a credential value never reaches CLI output, errors or MCP results, in any encoding a server can reflect (URL, double-URL, HTML entities, JSON `\u`, base64/base64url, `Location`, `X-Request-Id`, truncated title), on error and success responses. | `class_secret_echo.py` (runtime): read commands and MCP read tools against a fake 403 that reflects each credential in body and headers; a 200 that reflects it in headers only (credential-free body); `--dry-run`, whose preview must mask the Authorization value. The MCP probe scans the session's raw stdout, stderr and whole result. Sentinels carry `.+/=` so each encoding is distinct. | Fires on 72 connectors (raw and URL forms masked; HTML, double-URL, JSON `\u` or base64 forms survive). Silent on the decoded-backstop connectors (cisco-duo, connectsecure, n-central, saas-alerts, watchguard) and riverside-fm (no credential). |
| **Auth on the wire** - a configured credential is actually transmitted, and none follows a redirect to another host. | `class_auth_wire.py` (runtime): `doctor` against a fake server, then against one that 307s to a SECOND loopback server (different host:port); the second may receive no configured credential, no token the fake server minted, and no Authorization header. | Fires on 5 (connectwise-automate, connectwise-manage, cove, n-central, sherweb: each forwards a credential to the second host). Silent on the rest, which strip or refuse the hop. |
| **Write safety** - every non-read MCP tool is refused under the read-only switch, and `--dry-run` sends nothing. | `class_write_safety.py` (runtime): with `<PREFIX>_MCP_READ_ONLY`, every write tool is called with real-shaped arguments (an `_execute` tool gets a genuine write endpoint id) in a CONTROL session with the switch off and again with it on; a write request reaching the fake API in the read-only session is a finding (a token mint, or a POST search agent-context marks read-only, is not a write). A write command (positional ids filled with a placeholder) run with `--dry-run` must put zero requests on the wire; any request it sends is a finding, and a clean dry run counts as proven only when the same command run for real reaches the API. **Complemented** by the static `mcp-read-only-switch-skips-typed-tools` rule in `check_security_gate.py` (#429), which reads `tools.go` for the drop pattern. | Fires on 10 OAuth client-credentials connectors whose dry run mints a token (appdirect, crowdstrike, halopsa, immybot, nerdio, ninjaone, pax8, sherweb, skykick, threatdown). Silent on connectwise-rmm, sophos-central and watchguard (same auth, `dry-run-never-mints` fix). Read-only: 9 of the 17 switched connectors proven to stop every write, 8 advisory, 0 violations. |
| MCP local-file flags | Owned by the fleet gate `check_mcp_file_flags.py` (#431), not duplicated here. | - |

**Static vs runtime read-only (measured 2026-10-23).** The static rule baselines **19** connectors,
every one of which ships **no** `_MCP_READ_ONLY` switch, so the static rule marks them to adopt the
drop pattern and the runtime class has no switch to drive. The runtime class drives the **17** that
do ship one: 9 are proven (the control session wrote, the read-only session did not), 8 are
advisory (no write tool produced an API write even with the switch off: their `_execute` tools or
typed writes need arguments the probe cannot construct), and **0** let a write through. The two
sets do not overlap: static flags the source pattern, runtime proves the shipped behavior.

### Rollout: the shrink-only baseline

`class_baseline.json` lists the connectors that FAIL a class today, per class, using the same
burn-down model as `security_gate_baseline.json` (#429):

- a **listed** connector is reported, not failed, until its fix or reprint closes the class;
- an **unlisted** connector that fails is a CI failure (fix it, do not baseline it);
- `--assert-baseline-shrinks <base>` (guards job, build-free) fails a PR whose baseline lists a
  connector the base baseline did not, so a PR cannot widen its own list; it passes on the PR that
  first introduces the file;
- `--update-baseline` removes connectors that no longer fail and never adds; `--seed-baseline`
  gives a class with no entry its first list;
- a listed connector that now passes is reported by `--all` and `--report`, and `--all` fails on
  it. The per-connector CI rows run `--slug`, which cannot see the rest of the list, so retirement
  is enforced by `--update-baseline` (run it when a fix lands) rather than by every PR.

Fleet counts at seed (84 connectors, measured 2026-10-23, a fleet-wide `--all` exits 0):

| Class | Failing (baselined) | Passing | Advisory (not exercised) |
| --- | --- | --- | --- |
| secret-echo | 72 | 6 | 6 (avanan, blumira, hubspot, mspbots, proxmox, threatlocker: reads are local or need arguments) |
| auth-wire | 5 | 79 | 0 |
| write-safety | 10 | 47 | 27 (8 read-only not proven, see above; 21 whose write commands never reached the API even without `--dry-run`, so the dry run could not be proven clean) |

Step 7 of the pipeline plan empties these lists. No connector is fixed here.

### Known gaps (open, tracked in #439)

The final review round left five findings open. Each has a concrete failing input and a one-rule
fix, and none can turn main red (every connector failing a class today is baselined). Each can let a
future connector pass a class it should fail:

1. **secret-echo decode.** The candidate-run pattern also matches `.`, so a `.`-prefixed base64
   blob is decoded misaligned and the credential is not reconstructed. Fix: base64-alphabet token
   boundaries.
2. **write-safety attribution.** The token URL points at the fake server root, so a connector's
   data `POST /` is exempted as a token exchange. Fix: a reserved token path on the fake server.
3. **write-safety attribution.** Requests sent during the read-only `tools/list` discovery fall
   outside the read-only capture window. Fix: attribute every read-only-session request.
4. **secret-echo surface parity.** The MCP success probe reuses the CLI probe's HOME, so a shared
   response cache can keep the reflected header from reaching MCP. Fix: a fresh HOME for it.
5. **baseline guard.** A base whose baseline blob exists but cannot be read (an unhydrated partial
   clone) is treated as "file absent". Not reachable in CI (full clone). Fix: prove absence with
   `git ls-tree`; any other failure fails closed.

### Known limits (accepted)

- The redirect hop changes port, not hostname. The connectors' own `CheckRedirect` compares
  host:port, which is what the 5 findings exercise; Go's built-in Authorization stripping compares
  hostnames only, so a connector relying on it alone is tested only for its custom headers.
- A credential reflected in a 200 response BODY is the server's data and is printed by design;
  the success probe reflects it in headers only.
- Advisory connectors are coverage gaps, surfaced so they are never mistaken for passes.

### Cost

Each run builds the connector's two binaries and drives a bounded set of probes (at most 4 read
commands, 8 MCP read tools, 12 MCP write tools and 4 write commands for the dry run, each probe
process capped at 20 s). Measured serially on a warm module cache, all three classes take 6-29 s
per connector (connectwise-automate 6, ninjaone 9, halopsa 11, threatdown 14, td-synnex 19,
saas-alerts 29: mirror tools shell out to the CLI, and a connector that refuses the cross-host
redirect retries with its own backoff). A whole-fleet run takes about 4 minutes with 8 workers. The classes run per changed connector in the `build` matrix row; a change
to the harness is MACHINERY (see `release_matrix.py`), so it re-runs the fleet.

## What CI builds, and when

`ci.yml` computes its build matrix with `release_matrix.py --changed-only`: a PR builds the
skills changed since its merge-base, and a push to `main` builds the skills changed since
`github.event.before` (the tip `main` had before that push). The catalog bot's
`chore: regenerate derived files [bot]` commits touch only derived files (`pending.json`,
`docs/skills/`, the `.mcpb` pin in a skill README, ...) and build nothing, except that a bot
commit which also stamps a sealed release (`stamp_release.py`) builds that one connector's
row, as the stamp PR it replaces did; a hand edit to the same README builds that skill; a change to a matrix-row checker or a build-feeding workflow
builds all 65. A Windows known-failures ledger edit builds exactly the skills whose exclusions
changed (an every-connector entry, or a missing or invalid revision, builds the fleet). A weekly full-matrix sweep (Sunday 06:00 UTC, also `workflow_dispatch`) bounds
anything scoping could miss. Before 2026-09-07 every push to `main` built the whole fleet,
which is ~210-240 runner-minutes per docs-only bot commit and, during a 21-tag release wave,
enough to starve the release builds themselves.

`catalog.yml` regenerates the release-derived files (`docs/_data/pending.json`, README
`.mcpb` pins) when the **Release** workflow completes successfully (`workflow_run`), when a
release is deleted or unpublished, and weekly - not when a tag is pushed - and it reads the
PUBLISHED release list from the API rather than `git tag`,
admitting only releases whose asset set is complete (`check_release_assets.py
--admit-published`, `.mcpb` included) - so a tag in front of a stranded draft or a partial
release reads as pending and the README keeps its last complete published download link.

## Cutting a release tag

**A tag push runs `.github/workflows/release.yml` as it exists AT THE TAGGED COMMIT**, not as
it exists on `main`. This repository has [immutable releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases)
enabled, so a release is sealed the moment it is published. Tag a commit whose workflow
publishes before it uploads and you get a permanently sealed, **empty** release plus a spent
version number that cannot be reused. That happened twice in one day to `xero-v0.1.3`, which
is why it shipped as `xero-v0.1.4`.

So never type a bare `git tag`. Cut every release tag like this:

```bash
python3 tools/maintainer/check_release_pipeline.py --tag <slug>-v<x.y.z> --sha <sha> \
  && git tag <slug>-v<x.y.z> <sha> && git push origin <slug>-v<x.y.z>
```

The `&&` is doing real work: the probe runs when you **paste**, so a command pulled out of an
old scrollback is still checked against today's facts. It refuses the tag unless **all** of:

- that commit's `release.yml` assembles the release as a draft, gates the asset set with
  `check_release_assets.py --with-mcpb`, and seals last;
- the tag is not already cut, here or on `origin`;
- the version number is not retired in [`burned_versions.json`](./burned_versions.json);
- `skills/<slug>/manifest.json` **at that commit** carries exactly the version the tag names.

On success it prints the pinned `git tag` command it just endorsed, so a passing probe is the
only place an endorsed tag command comes from. `release.py` deliberately prints no runnable
tag command at all: it runs before anything is pushed, so it has no SHA to pin to.

`release_batch.sh` does the whole choreography for a version bump and prints these lines with
the SHA filled in. It cannot help a wave whose version stamps are **already** on `main` -
`release.py` would bump past them - which is exactly when someone hand-tags, so the command
above is the one to reach for.

Belt and braces, once per clone:

```bash
git config core.hooksPath tools/maintainer/hooks
```

That installs [`hooks/pre-push`](./hooks/pre-push), which refuses any `refs/tags/<slug>-v*`
push the probe refuses, however the tag was typed. It is the second layer, not the first: an
uninstalled hook protects nobody. (`core.hooksPath` replaces `.git/hooks` wholesale; if you
keep your own hooks there, copy the file in instead.)

## Merging, tagging and stamping

The release chain is four tools. Run each from an up-to-date checkout of `main`, never from
the branch being merged or a stale shared checkout: `merge_when_green.py` and `cut_tags.sh`
refuse to run when their own code differs from `origin/main`'s, and every checker they call
(the release probe with its burned-versions ledger, the asset contract) runs from a worktree at
`origin/main` or at the tagged commit, never from the checkout they happen to live in. Every
one has `--self-test`, which
builds throwaway git repositories (and, for the merge and tag tools, a scripted fake GitHub)
and proves each refusal and each pass both ways.

```bash
# 1. Merge one PR when it is green on the exact commit, on current main.
python3 tools/maintainer/merge_when_green.py approve 412 --reviewer codex --head <sha> --evidence verdict.txt
python3 tools/maintainer/merge_when_green.py 412 --dry-run   # every step, nothing pushed or merged
python3 tools/maintainer/merge_when_green.py 412
python3 tools/maintainer/merge_when_green.py status 412      # approvals and whether each is current

# 2. Cut the tags, one per push, then prove they sealed.
tools/maintainer/cut_tags.sh --dry-run <merged-sha> tags.txt
tools/maintainer/cut_tags.sh <merged-sha> tags.txt
tools/maintainer/cut_tags.sh --receipts tags.txt
tools/maintainer/cut_tags.sh --rerun-failed tags.txt   # recovery: re-run failed runs of still-mutable drafts

# 3. Nothing: the catalog bot stamps cli_hash_at_release after each release seals.
```

### `merge_when_green.py`

In order: takes a per-branch lock (`repo_lock.py`, in the clone's shared git dir); refuses if a
worktree with the branch checked out is dirty, mid-operation, in use by another live process,
or holds unpushed commits (it never resets or cleans those; all its work happens in a private
worktree it creates and removes); squashes the branch to one commit on its merge base,
keeping every sign-off; rebases onto current `origin/main`, sending registry and derived-file
conflicts to `resolve_registry.py` and stopping on anything else; regenerates the derived files;
pushes with `--force-with-lease` pinned to the head it started from; waits for every workflow
run, check run and commit status on that exact commit, with the branch-protection contexts
present and SUCCESS; and merges with `--match-head-commit` only when all of it is green,
main has not moved since the rebase, and the required approvals are current. A refused merge
is re-validated from the top, never retried blindly.

**Trust boundary: it executes the PR branch's code with your credentials.** The branch's own
generators (`build-catalog.py`, `build-llms.py` and whatever they import) run in the private
worktree as you, with your git and `gh` credentials and access to the clone's shared git
directory, before any approval or CI, and during `--dry-run` too. It refuses forks, but a
same-repository branch is trusted code. Run it only on PRs by Servosity staff. This is a
ratified, accepted decision while every write collaborator is Servosity staff, the same trust
boundary as running `verify_all.sh` on a PR checkout, and it must be revisited before anyone
outside Servosity gets write access.

**One final gate, immediately before the merge call.** After the wait says green, everything
the merge depends on is read again in one place, and the merge call is the very next thing it
does: `main` (fetched, and this tool's own code compared with that `main`), the approvals'
change fingerprint, every check, workflow run and status on the exact head (still green and
with the same verdicts the wait saw), the PR (open, into main,
this head) and `main` once more. Any change sends the run back to validate from the top; a
same-head re-run that has turned pending or red is never merged on the strength of the earlier
green. After the merge, the head's checks are read again (up to three reads) and compared with
the gate's: any difference (or no readable listing) exits **8**, and contrary evidence seen on
any read (a row the gate never saw: changed, pending, failed, a different combined status) is
kept, so a later read that matches never erases it. Every listing is read in one paginated call
that keeps each page's `total_count`, and counts as complete only when the pages agree, no row
repeats, and the distinct rows equal that count (a bare row count passes when one run is added
and another removed between pages). GitHub's `runs?head_sha` listing sometimes answers an empty
but consistent page for a commit that has runs (1 read in 6 on 2026-10-22); before the merge the
expected workflows (`CI`, `security-gate`) then read as not started, and after it such a read is
read again rather than taken as a changed verdict. Every fetch of `main` during a run, not only
the first, re-checks the tool's freshness against it, comparing the code loaded at start (not
the files on disk now), so rules that `main` changes mid-run stop the run (exit 2: restart
from updated main) instead of validating with the old ones.

**Residual window.** The seconds between the gate's last reads and GitHub executing the merge
remain. Only server-side enforcement closes them: a merge queue, or requiring every aggregate
check in branch protection (today only `security-gate` is required). The after-merge
comparison and the parent/tree proof below detect what lands in that window. Neither the
paginated listings nor `cut_tags.sh`'s in-flight count can prove their pages came from one
snapshot when one row is deleted and another added between pages; that is accepted as part of
this window, because check runs are not deleted, a re-run surfaces as a new latest run that
reads pending, the after-merge comparison (keeping contrary evidence from every read) is the
backstop, and the release cap is a soft limit (the API failure came at 52 runs in flight; the
cap is 4).

Two cancellation residuals in `cut_tags.sh` are accepted the same way:
- **A signal inside `Popen`.** A signal that arrives in the microseconds between the bounded call's
  child being created and its handle being assigned is not passed on to that child.
- **A stopped call.** An operator SIGSTOPs a call's wrapper and then TERMs the shell. The grace
  periods (cleanup's 10 s wait, the lock holder's 30 s wait after the shell dies) then end, and the
  lock is released while that call's push lives.

The consequence of either is at most one stray tag push, one that had already passed its probe.
`cut_tags.sh --receipts` and its stranded-draft sweep catch it.

**Bounded calls.** Every git, `gh` and generator call in these tools runs in its own process
group, bounded by what is left of the current wait's deadline and at most `MSP_CALL_TIMEOUT_S`
seconds (300; `CUT_TAGS_CALL_TIMEOUT` for `cut_tags.sh`). Both must be a positive, finite number
of seconds: anything else (`inf`, `nan`, 0, a negative number, text) refuses to run, and a value
above 300 is clamped to 300. `gh`'s HTTP client has no timeout of its
own, so without this one stalled response would hold the branch lock forever. A call past its
bound is killed with its children and fails closed; `cut_tags.sh` runs its python checkers (whose
own git calls read the tagged commit) the same way. A call that fails part-way keeps every
complete row it returned, so after a merge a red row read before the failure stays evidence.
**The exit-8 latch.** Once the FIRST merge call has been made, any call timeout or interrupt at
any later point, cleanup included and across merge retries, latches exit **8**, and so does any
outcome other than a proven 0 or 7. A later complete read never clears it. Exit 8 means "go look":
after a refused first call it can be a false alarm, which is accepted. Before any merge call, a
cleanup failure (a timeout removing the private worktree) is only a warning; the branch lock is
released regardless.

**Cancellation.** SIGINT (Ctrl-C), SIGTERM and SIGHUP stop a run cleanly. In the Python tools the
signal is raised like Ctrl-C, so a blocked call's whole process group is killed and reaped, then
cleanup runs (private worktree removed, locks released). `merge_when_green.py` exits **130**
before the first merge call and **8** after it (the latch above). `cut_tags.sh` stops every bounded call still running,
waiting until its children are dead, before it releases the release-tag lock. That holds when the
signal reaches only the shell's pid, when it reaches the whole group, and when the shell dies
outright, which each call notices for itself. So an interrupted tag push never leaves a push
running after the lock is free. `resolve_registry.py` reports a resolution that is written and
staged but unverified, and says to abort the rebase or merge, when its re-check times out or is
interrupted.

**Accepted, detected risk: main moving inside the merge call.** GitHub's merge call pins the
head (`--match-head-commit`, the validated SHA) but cannot pin the base, and this repository's
branch protection is not strict (by decision: no strict mode or merge queue for now). So a
commit can still land on `main` in the second between the tool's last look at `main` and the
merge. The tool narrows that window (the final gate re-reads the PR and `main` immediately before
the call and goes round again if either moved) and never misses it: afterwards it proves the merge
commit's parent is the validated base and its tree is the green tree. If not, it names what the
intervening commits touched, waits for `main`'s own push CI on the merge commit, and exits **7**
when that CI is green or **8** when it is red or never reports. A release chain must stop on 8,
and should read the intervening commits before going on after 7. Upgrade path if 7 or 8 ever
fires in practice: turn on a merge queue (or strict "require branches to be up to date"), and
have this tool hand the validated head to the queue (`gh pr merge --auto`) instead of merging
itself; the queue then re-runs the checks on the exact base it merges onto.

A failing check is classified from its log. A **real** failure (a finding, a test or compile
failure, an error annotation) is compared with the same check on the same base commit with
the same tool versions; only when the base fails identically and main's newer tip passes with
the same tool versions is the base shown to be stale, and only then does it rebase again. A
known infrastructure **flake** is re-run at most twice, an **unknown** failure at most once.
Where a failure sits in the tree is never a reason to clear or skip it.

An approval records the head SHA, the base SHA and a byte-exact fingerprint of the change (a
pinned `git diff --binary`, not `git patch-id`, which ignores whitespace even inside string
literals). It stays current across a rebase only when the fingerprint is unchanged and main's
new commits touch nothing under the connector's directory, nothing in `release_matrix.py`'s
`MACHINERY` list, no directory the change's other files live in, no hand-written part of a
README or issue form the change also edits, and no registry entry of a slug it touches. So
that another connector merging first does not cost a re-review, the three registries are
fingerprinted entry by entry, README.md and the issue forms by their hand-written parts only
(the generator-owned regions masked with the generator's own patterns), and only the files a
generator writes whole from sources (`catalog.json`, `docs/_data/catalog.json`,
`docs/llms*.txt`, each connector's `docs/skills/<slug>.md`) are left out; the tool regenerates
and re-checks those on the commit it merges. Any other file under `docs/skills/` is an ordinary
file.

Every push it makes carries exactly one ref (`--no-follow-tags`, so a `push.followTags=true` in
someone's config cannot publish annotated tags with it), and `catalog.json` is restored from the
validated base before regenerating, because build-catalog.py keeps its `generated_at` and
prints it in the README footer.

Exit codes: 0 merged; 2 refused (lock, worktree, fork, head moved, stale copy of the tool); 3 a
conflict it will not resolve; 4 an approval is not current; 5 a check failed; 6 timed out; 7
merged in a race, main's CI then green; 8 merged (or possibly merged), but main's CI is red or
unverified after a race, the head's checks changed between the final gate and the merge, a call
timed out after the merge call, or the merge call or its proof did not complete or was interrupted
(stop the chain and check main by hand); 130 interrupted before the merge call (nothing merged).

**`repo_lock.py`: one protocol with older copies.** `release.py` and `release_batch.sh` on older
worktrees hold the same `msp-skills-locks/<name>.lock` with the original O_EXCL-and-pid copy, so
this copy keeps that as the exclusion: an atomic `O_CREAT|O_EXCL` create holding the
`{pid, host, ...}` record the old copy reads, and release unlinks the file. It also keeps an
`flock` on the open file while it holds the lock, as a liveness signal only. Facing an existing
file: a held `flock` means a live holder; otherwise the old pid rule decides, and a dead holder's
file is unlinked only while holding its `flock` and only if the path still names that inode. The
self-test runs main's original copy against this one in both directions (each refuses while the
other holds, acquires after the other releases, and reclaims after the other is killed).
Residual: the old copy reclaims a dead holder's file without an `flock` or an inode check, so an
old and a new copy reclaiming the same dead holder's file at the same moment can both end up
holding. It is the race two old copies already have with each other; it needs a dead holder and
two contenders inside the same few milliseconds, and it goes away once no old copy runs.


### `resolve_registry.py`

Run inside the conflicted worktree, then `git rebase --continue` (or `git commit` for a merge of
main into a branch, where it detects main as the merged-in side). It writes each registry in
the format main's copy uses (detected by round-tripping its bytes) and places new entries by
the file's convention: sorted in `skills.json`, appended in `marketplace.json`, sorted in
`cards.yaml`. Registries git merged cleanly are verified per entry too, because a textual merge
can produce valid JSON that names one slug twice. A fully generated file takes main's copy; a
partly generated one (README.md, the issue forms) takes one side's copy only when the other side
changed nothing outside the generator's regions, so hand-written prose is never discarded. It
replaces a conflicted file only if it is exactly what git wrote (a file edited by hand since is
refused; `git checkout -m -- <path>` restores git's version), writes nothing if a conflicted file
or the index changes while it plans, and after running the generators puts back only their own
skill-README pin re-mints, stopping on any other change. Exit 2 means nothing was changed.

### `cut_tags.sh`

Generalises the release-wave script. The tags file is resolved to an absolute path before
anything else, a missing, empty or duplicated list is refused, and a run succeeds only when the
number of tags handled equals the number listed (a relative path read nothing once and still
printed ALL-PUSHED). Per tag: skip it if it is already on the remote at this SHA, stop if it is
anywhere else, run `check_release_pipeline.py --tag --sha` from a worktree at `origin/main`
(the newest probe and burned-versions ledger) and `changelog_section.py --check` from a worktree
at the SHA, push it alone, and wait for its Release run to register. The probe is extracted from
`origin/main` afresh for every tag, after any wait, so a version retired meanwhile is refused,
and every fetch of `main` re-checks this script against it (the copy loaded at start, not the
file on disk), so a `main` that changes the cutter's rules mid-run stops it.
Before every push it waits
until fewer than `CUT_TAGS_MAX_IN_FLIGHT` (default 4) Release runs are in flight across the
repository: one tag per push is not enough by itself (52 tags pushed one at a time still put 52
runs in flight together, exhausted the Actions-token API limit, and left 9 partial drafts and 9
tags without a release). A count that cannot be read (a failed `gh` call, pages that change
while releases run) is logged as `FLIGHT-UNKNOWN` and waited out within the same
`CUT_TAGS_FLIGHT_TRIES` x `CUT_TAGS_FLIGHT_SLEEP` budget: nothing is pushed or re-run while the
count is unknown, and the run stops only when that budget runs out (one transient failure once
stopped a whole wave). The count comes from one
unfiltered listing of every Release run, counted locally: separate per-status queries miss a run
that changes status between them. It is used only when every page reports the same
`total_count` and the distinct runs read equal it (read again up to `CUT_TAGS_LIST_TRIES`,
default 3, times); one complete listing takes about 25 s today (663 runs, 7 pages). A release-tag lock (a `flock`
in the clone's shared git dir) is held from the capacity check through the push and the run's
registration, for cutting and for `--rerun-failed`, so two runs in one clone cannot both see room
under the cap; separate clones do not share it. A small holder process owns the lock (no git
hook or other child of the script can inherit it), releases it the moment the script exits for
any reason, and a re-run keeps it until its new attempt is visibly registered. Each push
carries exactly one ref (`--no-follow-tags`). `--receipts` waits for each
release to be published with its complete asset set (`check_release_assets.py --with-mcpb`, run
from a worktree at the tag's own commit, whose registry knows the connector), reports a failed
Release run as a stranded draft at once, and sweeps the repository for drafts: one whose Release
run is still going is reported as in flight, any other fails the run.

`--rerun-failed` is the recovery for such a wave. For each listed tag whose latest Release run
failed while its release is still a draft (or was never created), it re-runs the failed jobs
(`gh run rerun <id> --failed`), `CUT_TAGS_RERUN_BATCH` (default 3) at a time under the same
in-flight cap, waits for each batch, then prints the receipts. Because a re-run executes
`release.yml` as it is at the tagged commit, each candidate must pass `origin/main`'s pipeline
probe on that commit (a publish-first workflow could seal an empty immutable release), and the
failed run must be for that commit; the probe's own tag policy applies too (a burned version
is never re-released), and the run, its release and the probe are read again immediately before
each re-run. A release already published with assets missing is
immutable and is reported for a version bump; a tag with no run, or a release whose state
cannot be read (including a 403 on the lookup), is reported too. `--dry-run` lists what it would
re-run. It never deletes a release and never moves or re-pushes a tag.

### `stamp_release.py`

For a connector whose newest complete published release matches its registry and manifest
version and whose `cli_hash_at_release` is still null, it writes the hash of `skills/<dir>/cli`
at the tag (computed from git objects exactly as `cli_hash.py` computes it on a checkout) and
dates the undated CHANGELOG heading with the publish date. It never overwrites a stamp that is
set (a mismatch is printed as a WARN), reads the source directory and version from the tagged
commit itself, and declines where a `.gitattributes`, a symlink or a submodule could make
checkout bytes differ. Writes are compare-and-swap under `release.py`'s repo lock: if
`skills.json` or a CHANGELOG changed after it was read, nothing is written. `catalog.yml` runs it inside its existing
post-release commit, in the `main-autocommit` concurrency group, so a sealed release no longer
needs a separate stamp pull request.
