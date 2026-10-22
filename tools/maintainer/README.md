# Maintainer tooling

These scripts are for **maintainers of this repository**, not for MSP users. They generate
the catalog, compute the release build matrix, and run the CI gates (skill contract, markdown
links, release contract, DCO sign-off, repo hygiene). If you just want to install a skill or
the statusline, you do not need anything in this folder - see the root
[README.md](../../README.md) and [`tools/statusline/`](../statusline/).

| Script | What it does |
| --- | --- |
| `verify_all.sh` | One command, one verdict: runs every gate below. `bash tools/maintainer/verify_all.sh` |
| `build-catalog.py` | Regenerates `catalog.json`, the README catalog table and generated blocks, the docs site pages, the issue-form connector dropdowns, and each skill README's `.mcpb` pin. `--released-tags FILE` (catalog.yml passes the complete-published release list) keeps a pin from ever naming a tag whose release is a stranded draft or is missing an asset. |
| `release_matrix.py` | Prints the GitHub Actions build matrix (skills x os/arch). `--changed-only <base>` (merge-base; ci.yml PRs) and `--changed-since <sha>` (two-point diff; ci.yml pushes, from `github.event.before`) scope it to the skills a change-set touched, so a docs-only or catalog-bot commit builds nothing and a build-machinery edit builds the fleet. `--self-test` proves the classifier both ways. |
| `release_state.py` | Reports each skill's release state (up-to-date, version-pending, binary-pending, never-released) and writes `docs/_data/pending.json`. `--released-tags FILE` replaces "the tag exists" with "the tag names a published release". |
| `check_skill_contract.py` | Asserts every skill has the required frontmatter and files. |
| `check_release_contract.py` | Asserts install scripts and release assets agree on names. |
| `check_md_links.py` | Verifies relative Markdown links resolve. |
| `check_dco.sh` | Verifies every commit carries a DCO `Signed-off-by` line. |
| `check_mcp_gate.py` | Boots a skill's MCP server over stdio and asserts the tenant gate does not default-deny. Probes the read-only tools the gate wraps in-process; skips Cobra mirrors (the child CLI gates those) and destructive tools. Needs no credentials. |
| `check_command_tree.py` | Compiles each connector's real `cli.RootCmd()` tree (through `go build -overlay`, so nothing is written into the connector) and fails when a destructive command (delete, cancel, purge, disable, reboot, ...) still answers to the hidden alias `create`. Also reports, without failing, command-local flags that shadow a global flag such as `--agent`. `--self-test` proves the classifier both ways. |
| `ci_guards.sh` | Repo hygiene: no em-dashes, no personal paths, no obvious secrets. |
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

## What CI builds, and when

`ci.yml` computes its build matrix with `release_matrix.py --changed-only`: a PR builds the
skills changed since its merge-base, and a push to `main` builds the skills changed since
`github.event.before` (the tip `main` had before that push). The catalog bot's
`chore: regenerate derived files [bot]` commits touch only derived files (`pending.json`,
`docs/skills/`, the `.mcpb` pin in a skill README, ...) and build nothing, except that a bot
commit which also stamps a sealed release (`stamp_release.py`) builds that one connector's
row, as the stamp PR it replaces did; a hand edit to the same README builds that skill; a change to a matrix-row checker or a build-feeding workflow
builds all 65. A weekly full-matrix sweep (Sunday 06:00 UTC, also `workflow_dispatch`) bounds
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

GitHub's merge call pins the head but cannot pin the base, and this repository's branch
protection is not strict, so a commit can land on `main` inside the seconds between the last
check and the merge. The tool checks `main` once more immediately before the call, then
proves the merge commit's parent is the validated base and its tree is the green tree; if not,
it exits 7 naming what the intervening commits touched. That is detection, not prevention:
only strict protection or a merge queue would prevent it.

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

Exit codes: 0 merged; 2 refused (lock, worktree, fork, head moved, stale copy of the tool); 3 a
conflict it will not resolve; 4 an approval is not current; 5 a check failed; 6 timed out; 7
merged, but main moved inside the merge call.

### `resolve_registry.py`

Run inside the conflicted worktree, then `git rebase --continue` (or `git commit` for a merge of
main into a branch, where it detects main as the merged-in side). It writes each registry in
the format main's copy uses (detected by round-tripping its bytes) and places new entries by
the file's convention: sorted in `skills.json`, appended in `marketplace.json`, sorted in
`cards.yaml`. Registries git merged cleanly are verified per entry too, because a textual merge
can produce valid JSON that names one slug twice. A fully generated file takes main's copy; a
partly generated one (README.md, the issue forms) takes one side's copy only when the other side
changed nothing outside the generator's regions, so hand-written prose is never discarded. Exit
2 means nothing was changed.

### `cut_tags.sh`

Generalises the release-wave script. The tags file is resolved to an absolute path before
anything else, a missing, empty or duplicated list is refused, and a run succeeds only when the
number of tags handled equals the number listed (a relative path read nothing once and still
printed ALL-PUSHED). Per tag: skip it if it is already on the remote at this SHA, stop if it is
anywhere else, run `check_release_pipeline.py --tag --sha` from a worktree at `origin/main`
(the newest probe and burned-versions ledger) and `changelog_section.py --check` from a worktree
at the SHA, push it alone, and wait for its Release run to register. `--receipts` waits for each
release to be published with its complete asset set (`check_release_assets.py --with-mcpb`, run
from a worktree at the tag's own commit, whose registry knows the connector), reports a failed
Release run as a stranded draft at once, and sweeps the repository for drafts: one whose Release
run is still going is reported as in flight, any other fails the run.

### `stamp_release.py`

For a connector whose newest complete published release matches its registry and manifest
version and whose `cli_hash_at_release` is still null, it writes the hash of `skills/<dir>/cli`
at the tag (computed from git objects exactly as `cli_hash.py` computes it on a checkout) and
dates the undated CHANGELOG heading with the publish date. It never overwrites a stamp that is
set (a mismatch is printed as a WARN), and declines where a `.gitattributes`, a symlink or a
submodule could make checkout bytes differ. `catalog.yml` runs it inside its existing
post-release commit, in the `main-autocommit` concurrency group, so a sealed release no longer
needs a separate stamp pull request.
