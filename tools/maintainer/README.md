# Maintainer tooling

These scripts are for **maintainers of this repository**, not for MSP users. They generate
the catalog, compute the release build matrix, and run the CI gates (skill contract, markdown
links, release contract, DCO sign-off, repo hygiene). If you just want to install a skill or
the statusline, you do not need anything in this folder - see the root
[README.md](../../README.md) and [`tools/statusline/`](../statusline/).

| Script | What it does |
| --- | --- |
| `verify_all.sh` | One command, one verdict: would CI pass, and is it ready for a PR? No arguments checks the full fleet; `--slug <s>` (repeatable) checks those skills; `--changed [--base origin/main]` checks what the branch changed, scoped by CI's own classifier (a shared-machinery change checks the whole fleet, as CI does). Heavy per-connector checks run for the connectors in scope, cheap fleet-wide checks for everything; a fleet failure only in other connectors is listed under `out_of_scope_failures`, never dropped. It checks the commit `git add -A` would make (git builds it in a private index; the developer's index is untouched), in a clean clone with no global git config. Writes `.verify/verdict.json` (checks, verdict, seconds, head, snapshot and base SHAs, scope). |
| `verify_connector.sh` | The per-connector half of `verify_all.sh`. CI's whole Linux build row (build, vet, cross-compile, CLI claims, hand-fix ledger, MCP gate, doctor and `go test -count=1`, on linux/amd64 with cgo on) runs in that context: natively on such a host, else in a `golang:<toolchain>` linux/amd64 container, always on a Linux disk (a private Docker volume) holding a depth-1 clone of the commit plus working-tree changes, else it fails closed. The Windows predictors (vet, test compile and link, Unix-only test lint) run on the host with cgo off, over a copy of the Windows job's sparse checkout, and fail closed when turning cgo on would change any file Go compiles. Every Go command gets an explicit environment, nothing inherited. |
| `verify_scope.py` | Scope, failure attribution and verdict for `verify_all.sh`, plus `cgo-context` (does cgo on vs off change what Go compiles), `ci-go` and `gowork` (the Go and go.work CI uses), `windows-tree` (the Windows job's sparse checkout) and `path-collisions` (paths one case-insensitive disk folds together); `--self-test` proves each. |
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
| `check_command_tree.py` | Compiles each connector's real `cli.RootCmd()` tree (through `go build -overlay`, so nothing is written into the connector) and fails when a destructive command (delete, cancel, purge, disable, reboot, ...) still answers to the hidden alias `create`. Also reports, without failing, command-local flags that shadow a global flag such as `--agent`. `--self-test` proves the classifier both ways. |
| `ci_guards.sh` | Repo hygiene: no em-dashes, no personal paths, no obvious secrets. Decides on each guard's output, so a file missing from disk cannot turn a violation into a pass; `--self-test` proves it. |
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

**Bounded calls.** Every git, `gh` and generator call in these tools runs in its own process
group, bounded by what is left of the current wait's deadline and at most `MSP_CALL_TIMEOUT_S`
seconds (300; `CUT_TAGS_CALL_TIMEOUT` for `cut_tags.sh`). `gh`'s HTTP client has no timeout of its
own, so without this one stalled response would hold the branch lock forever. A call past its
bound is killed with its children and fails closed; `cut_tags.sh` runs its python checkers (whose
own git calls read the tagged commit) the same way. A call that fails part-way keeps every
complete row it returned, so after a merge a red row read before the failure stays evidence.
Once the merge call has been made, every outcome other than 0 (proven), 7 or 8 is **8**, because
the PR may be merged and nothing proved it, and a cleanup failure (a timeout removing the private
worktree) is a warning that never changes the exit; the branch lock is released regardless.

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
unverified after a race, the head's checks changed between the final gate and the merge, or the
merge call or its proof did not complete (stop the chain and check main by hand).

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
tags without a release), and an unreadable count stops the push. The count comes from one
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
