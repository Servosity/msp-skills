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
| `check_no_pp_leak.py` | No connector's own `-pp-` name in a user-facing file, a Go string or a hand-fix `contains` marker (a `not_contains` marker is honored). Reads `cli/NOTICE` too. `--base` makes it a ratchet: lines already present at the base are listed, not blocking; CI runs the full audit (`--all`, no base) since the fleet went clean. |
| `check_unix_test_asserts.py` | Lints connector tests for assertions that only hold on Unix (POSIX errno text, mode bits, `#!` stubs, files left open, GOOS-blind literals, a `time.Now()` key on `INSERT OR IGNORE`), the classes the Windows CI job used to find only after a PR. Ledgered known Windows failures are reported, not blocking. |
| `build-catalog.py` | Regenerates `catalog.json`, the README catalog table and generated blocks, the docs site pages, the issue-form connector dropdowns, and each skill README's `.mcpb` pin. `--released-tags FILE` (catalog.yml passes the complete-published release list) keeps a pin from ever naming a tag whose release is a stranded draft or is missing an asset. |
| `backup_vendor_note.py` | The "Comparing backup vendors?" note on backup-vendor connector pages. Text and the list of connectors it applies to live in `backup_vendor_note.json` (one source); `render_docs_page.py` puts it on the docs page, `--write` puts it in a marker block in each README, `--check` fails on drift, `--list` names the connectors. `--self-test`. |
| `release_matrix.py` | Prints the GitHub Actions build matrix (skills x os/arch). `--changed-only <base>` (merge-base; ci.yml PRs) and `--changed-since <sha>` (two-point diff; ci.yml pushes, from `github.event.before`) scope it to the skills a change-set touched, so a docs-only or catalog-bot commit builds nothing and a build-machinery edit builds the fleet. `--self-test` proves the classifier both ways. |
| `release_state.py` | Reports each skill's release state (up-to-date, version-pending, binary-pending, never-released) and writes `docs/_data/pending.json`. `--released-tags FILE` replaces "the tag exists" with "the tag names a published release". |
| `check_skill_contract.py` | Asserts every skill has the required frontmatter and files. |
| `check_release_contract.py` | Asserts install scripts and release assets agree on names. |
| `check_md_links.py` | Verifies relative Markdown links resolve. |
| `check_dco.sh` | Verifies every commit carries a DCO `Signed-off-by` line. |
| `check_mcp_gate.py` | Boots a skill's MCP server over stdio and asserts the tenant gate does not default-deny. Probes the read-only tools the gate wraps in-process; skips Cobra mirrors (the child CLI gates those) and destructive tools. Needs no credentials. |
| `check_mcp_file_flags.py` | Proves an MCP tool call cannot name a file or directory on the MCP host. The Go analyzer in `mcp_file_flags/` derives, from data flow in the connector's code (not from flag names), every flag whose value can start a filesystem path; the script then boots the real MCP server beside a recording stand-in for the CLI and asserts each such flag is absent from every mirrored tool's schema and refused by name when supplied, that an ordinary flag still reaches the CLI, and that no in-process tool hands request data to a filesystem call. A name resolved inside an `os.Root` (press 4.34.0's opt-in MCP upload root) counts as naming a path only where a caller-chosen value reached the root's own location; `os.OpenRoot` of such a value, and a plain `os.Open` of the name, stay sinks. `--slug`, `--all`, `--jobs N`. The analyzer's own both-directions proof is `go test ./...` in `mcp_file_flags/`. |
| `credkit_verify/` | The shared credential kit's wiring checker (Go, `go/types`). For a connector that carries `cli/internal/credkit/`, it verifies the hand-written wiring and names every site it cannot classify; a connector without the kit is skipped. Run `go -C tools/maintainer/credkit_verify run . --slug <slug>`. See "The kit's wiring checker" below. |
| `check_command_tree.py` | Compiles each connector's real `cli.RootCmd()` tree (through `go build -overlay`, so nothing is written into the connector) and fails when a destructive command (delete, cancel, purge, disable, reboot, ...) still answers to the hidden alias `create`. Also fails a command-local flag that takes a value and shadows a persistent bare switch such as `--agent` or `--json` (the documented bare switch would swallow the next argument), and reports, without failing, every other local flag that shadows a global one. `--self-test` proves the classifier both ways. |
| `class_harness.py` | Black-box harness for the recurring connector bug CLASSES. Drives the BUILT `<slug>-cli` / `<slug>-mcp` against a local fake HTTP server, connector-agnostic (surface from the binary's `agent-context`, wiring from `manifest.json`; no per-connector Go imports). Runs three class tests (`class_secret_corpus.py`, the credential leak test list; `class_auth_wire.py`; `class_write_safety.py`). `--slug X` checks one connector (the CI matrix row); `--all --report` prints per-class fleet counts; `--self-test` proves each class fires on a broken shape and is silent on a healthy one, and that the baseline guard fails closed; `--seed-baseline` / `--update-baseline` / `--assert-baseline-shrinks` manage the shrink-only `class_baseline.json`. See [Class tests](#class-tests). |
| `kit_tls.py` | How a local fake vendor is served to a connector that adopted the credential kit: https with a certificate made for the run, trusted through `<PREFIX>_CA_CERT` and `SSL_CERT_FILE`. Used by `check_doctor_truth.py`, `class_harness.py` and the verify skill. A connector without the kit stays on plain http with no new variable. |
| `verify_gate.py` | CI gate over `.claude/skills/verify-msp-skills`: `--slug X` runs its `verify.py run` (Linux: `--egress-guard proxy-env`) and compares the six feature verdicts with `verify_baseline.json`. An unlisted FAIL blocks; a listed feature that now passes blocks until its entry is removed; an ERROR blocks as a harness error, not a connector defect. `--policy-base REF` reads the baseline from the trusted base. `--assert-baseline-shrinks REF` (guards job), `--seed-from DIR` (first seed from CI verdict artifacts), `--self-test`. See [Verify gate](#verify-gate). |
| `verify_baseline.json` | `connectors`: the verify features each connector FAILS on main today (3 to 6 by design before press 4.34.0). `known_errors`: the harness ERRORs the fleet had when it was seeded (reported, not gating). Both shrink-only, like `class_baseline.json`; a reprint PR removes its own entries. |
| `class_secret_corpus.py` | The credential leak test list's engine: runs every line of `secret_corpus.jsonl` against a built connector and judges it by [`docs/credential-leak-contract.md`](../../docs/credential-leak-contract.md). `--slug X` / `--all --jobs 4` run it, `--self-test` proves it both ways, `--check-baseline --base REF` is the build-free guard. See [The credential leak test list](#the-credential-leak-test-list). |
| `secret_corpus.jsonl` | The credential leak test list: one case per line, each a way a review found a held credential leaking. |
| `corpus_baseline.json` | The leak cases each connector fails on main today, per connector and per case. Shrink-only; a connector on the shared credential kit lists nothing. |
| `class_baseline.json` | Connectors that FAIL a class today, per class. Same burn-down model as `security_gate_baseline.json` (#429): a listed connector reports, an unlisted one fails CI, the list only shrinks. |
| `ci_guards.sh` | Repo hygiene: no em-dashes, no personal paths, no obvious secrets, a current `.coderabbit.yaml`, no press title on a skill page, no null press version in a manifest. Decides on each guard's output, so a file missing from disk cannot turn a violation into a pass; `--self-test` proves it. |
| `check_press_metadata.py` | Validates `skills/<dir>/press/`, the press's own `.printing-press.json`, `tools-manifest.json` and spec that onboard.py keeps beside `cli/`: only those files, parseable, version and print (`generated_at`, `spec_checksum`) agreeing with `manifest.json`, the spec matching its checksum, nothing secret-looking or personal in the text or any decoded view of it (backslash escapes, percent-encoding, HTML entities), and a vendor spec only with `publication.json`'s proof that its exact bytes are already in this repository's history outside press/ (no network check exists; every other spec is withheld as "spec source not public", and publishing one from another source is a later, deliberate change). Withheld files carry one of a fixed set of reasons there, never text from a file, tool or server. A connector without the folder is counted in the coverage line, not failed. `--base` also fails a connector the change ADDS when it was printed below `press_floor.json`. `--inspect DIR` is onboard.py's per-file verdict. `--self-test`. |
| `fresh_render_diff.py` | Review aid for a connector PR (publish plan step 5.4). Re-renders the connector with its own recorded press version (fails closed when no binary reports exactly that version, and prints the `go install` that gets it) from its own spec (found by `spec_checksum` in `--spec`, `skills/<dir>/press/` or the local press folders; fails closed when none matches), puts the render through the vendoring, strip-pp-cli and copyright transforms the pipeline applies, and compares byte for byte. What remains is the review set: hand-written files, hand edits inside generated files, deleted template files, `go.mod`/`go.sum`, the spec, symlinks. A spec with an external `$ref` is refused, and in `--pr` mode a spec that came inside the PR is rendered only inside msp-sandbox. `--spec-invariant` is the weaker fallback when the spec is not available: it clears only files every reference render (built-in specs plus the local real specs at that version) produces identically; `--calibrate X.Y.Z` measures that fallback leave-one-out. Not a CI gate (CI has no press binary or specs). `--self-test` runs anywhere, with a stand-in press. |
| `press_floor.json` | The lowest cli-printing-press version a new connector may be printed with (upstream's own declared minimum, with the commit it was read at), read by `check_press_metadata.py` and onboard.py. `not_press_generated` exempts a connector the press never generated (read from the base, so a PR cannot exempt itself). It rises to upstream's current floor once the paused 4.33.5 prints have landed. |
| `verify_live.py` | Flips a live-verified badge, or revokes it. A flip records the release it verified and the `cli_hash.py` hash of `cli/` at that tag (`--verified-version` when the report names it, else the newest release cut before `--reported-at`); `--bind` sets that on an existing badge. The catalog then reads "verified on vX.Y.Z" once the shipped code changes, and `check_registry_state.py` proves every recorded hash is the tag's code. A flip or revoke also moves the registry `status` and the README / pain-point Status paragraphs, because Beta and live-verified are mutually exclusive: `--check` (CI) fails on any disagreement and `--sync` repairs it. |
| `verify_watch.py` | Run weekly by `verify-watch.yml`. Keeps one `verify-map-stale` issue open while the verify-msp-skills map is behind the fleet (a manifest press version above `maintained_at_press`, a verify ERROR or an unexpected SKIP in main's newest `verify-<slug>` CI artifacts, or a stamp 30+ days old), and one `press-watch` issue while `mvanhorn/cli-printing-press` has a newer release than the fleet (input for the reprint decision; never reprints). Never duplicates, closes when clear, never closes on unreadable evidence. Dry run without `--apply`; `--self-test`. |
| `verify_maintained.json` | When the verify-msp-skills feature map was last maintained (`maintained_at`) and the press version it covers (`maintained_at_press`). Update both in the PR that runs `/maintain-verification-skill`. |
| `gen_governance.py` | Drafts a skill's `governance.md` from its CLI surface. |
| `registry.py` | Shared loader for `skills.json` (the single source of truth for skills). |
| `skills.json` | Per-skill metadata: owner/repo, binary names, vendor, status. |
| `release_batch.sh` | Runs a release batch in a throwaway worktree, pushes the version stamp, and prints probe-gated tag commands. Never tags. |
| `merge_when_green.py` | Merges one PR only when it is green on the exact commit, on current main: per-branch lock, squash, rebase, registry-aware conflict handling, log-classified CI failures, review approvals bound to the change, and a record of whether CodeRabbit reviewed that exact commit (a skip or rate-limit notice is not a review; `--coderabbit require` makes one mandatory). See [Merging, tagging and stamping](#merging-tagging-and-stamping). `--self-test` proves it against throwaway repos. |
| `gen_coderabbit_config.py` | Writes `.coderabbit.yaml`: CodeRabbit `path_filters` that exclude untouched press template output (Go files with the press's DO-NOT-EDIT header that `handfixes.json` does not name) and nothing else, so every hand-fixed generated file, spec and hand-written file stays in review. Every pattern is checked against every file that must stay in review before it is written. `--check` (run by `ci_guards.sh`) fails on a stale file; `--stats`; `--self-test`. |
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
| **Credential leaks** - a held credential never reaches output, a file or another host, in any form the contract names. | `class_secret_corpus.py` (runtime) runs every line of `secret_corpus.jsonl`: each line seeds one credential the connector declares, has a fake vendor carry it in one form and shape, drives one path (reads, MCP tools, doctor, flag and protocol errors, feedback, teach, `--deliver`, redirects), and fails if it survives in stdout, stderr, MCP output, a file under the fresh HOME, or a request to a second host. Judged by `docs/credential-leak-contract.md`. It replaced the secret-echo class, whose every check is a row here. | See [The credential leak test list](#the-credential-leak-test-list). |
| **Auth on the wire** - a configured credential is actually transmitted, and none follows a redirect to another host. | `class_auth_wire.py` (runtime): `doctor` against a fake server, then against one that 307s to a SECOND loopback server (different host:port); the second may receive no configured credential, no token the fake server minted, and no Authorization header. A doctor that sends nothing is reported as a NOTE (not judged), never read as a pass. | Fires on 5 (connectwise-automate, connectwise-manage, cove, n-central, sherweb: each forwards a credential to the second host). Silent on the rest, which strip or refuse the hop. |
| **Write safety** - every non-read MCP tool is refused under the read-only switch, and `--dry-run` sends nothing. | `class_write_safety.py` (runtime): with `<PREFIX>_MCP_READ_ONLY`, every write tool is called with real-shaped arguments (an `_execute` tool gets a genuine write endpoint id) in a CONTROL session with the switch off and again with it on; a write request reaching the fake API in the read-only session is a finding (a token mint, or a POST search agent-context marks read-only, is not a write). A write command (positional ids filled with a placeholder) run with `--dry-run` must put zero requests on the wire; any request it sends is a finding, and a clean dry run counts as proven only when the same command run for real reaches the API. **Complemented** by the static `mcp-read-only-switch-skips-typed-tools` rule in `check_security_gate.py` (#429), which reads `tools.go` for the drop pattern. | Fires on 10 OAuth client-credentials connectors whose dry run mints a token (appdirect, crowdstrike, halopsa, immybot, nerdio, ninjaone, pax8, sherweb, skykick, threatdown). Silent on connectwise-rmm, sophos-central and watchguard (same auth, `dry-run-never-mints` fix). Read-only: 9 of the 17 switched connectors proven to stop every write, 8 advisory, 0 violations. |
| MCP local-file flags | Owned by the fleet gate `check_mcp_file_flags.py` (#431), not duplicated here. | - |

**Static vs runtime read-only (measured 2026-10-23).** The static rule baselines **19** connectors,
every one of which ships **no** `_MCP_READ_ONLY` switch, so the static rule marks them to adopt the
drop pattern and the runtime class has no switch to drive. The runtime class drives the **17** that
do ship one: 9 are proven (the control session wrote, the read-only session did not), 8 are
advisory (no write tool produced an API write even with the switch off: their `_execute` tools or
typed writes need arguments the probe cannot construct), and **0** let a write through. The two
sets do not overlap: static flags the source pattern, runtime proves the shipped behavior.

### The credential leak test list

`secret_corpus.jsonl` is a list of every way a review has found a connector leaking a credential it
holds. Each line is one case: where the credential comes from (an environment variable the
connector declares secret, a config file field, a token the connector mints, a vendor field), the
form it travels in (raw, base64, base32, ROT13, a chain such as `rot13>base64`), the shape of the
vendor response that carries it (an error body, a success row, a header, gzip, a value split across
fields) and the path that runs the program (a read, an MCP tool, `doctor`, a bad flag, an unknown MCP
tool, `feedback`, `teach`, `--deliver`, a redirect). `class_secret_corpus.py` runs every line against
the built connector with no connector-specific code. The rules are the owner-ruled
[credential leak contract](../../docs/credential-leak-contract.md); this list is how CI checks them.

A case fails when the shared kit's matcher withholds any unit the run produced: stdout, stderr, the
MCP server's output, any file the run leaves under its fresh HOME, or a request that reached a
second fake host (any method, every value of a repeated header, every trailer of a chunked body).
The list has no matcher or decoder of its own (contract section 8): `class_secret_corpus.py` builds
the kit's `credmatch` command (`tools/maintainer/credkit/cmd/credmatch`, a temporary copy of the
kit package plus that command) once per run into a temporary directory, deletes it when the run
ends, and hands it the held set and each unit. It answers clean, leak (with the contract clause:
2(a) whole, 2(b) a piece of 8 or more, 2(c) a request to a host that is no held value's origin) or
exhausted. Every reading section 3 names (each decoder at every offset, recursively, charsets,
compressed streams, joined JSON values, integer spellings) is the kit's. A leak and an exhausted
unit both fail the case. A file or directory the run leaves that the test cannot read, a request
the fake cannot parse, and a request body in a transfer coding it cannot undo (every
`Transfer-Encoding` value is read, in order; `chunked` must come last; `gzip` and `deflate` are
undone) fail the case. A file that two runs of one case wrote (the 6.5 split across runs) is judged
as each run's own writes. The test knows what it injected only to name an advisory line's own
exception; a new form needs an encoder, and a reading section 3 lacks is a kit change.

The credentials it holds come only from what the connector declares: each variable whose
`user_config` entry in `manifest.json` is `sensitive`, plus each one `server.json` marks
`isSecret`, plus each one the shared kit's `SecretEnv` names. A variable's name means nothing.
Every declared variable a run sets is held in that run, whole and in pieces, and only while the
program is given it: the token-error line, which unsets static tokens so the client mints, seeds the
declared variable the token request carries, or does not apply. No piece is exempt, so a declared
URL or host variable gets an address of its own: the fake's port by the name `localhost` (and a
URL a path of its own), where every unheld variable spells it `127.0.0.1`, so no unheld value the
harness gives the program shares an 8-character piece with a held one. The held set of a run is
the kit's own (contract section 8; `credmatch`'s acquire request runs the kit's startup and
transport code), once for each process as it ran (its own argv and environment exactly as given,
and an MCP server's input, read for declared MCP arguments) and once for each CLI child an MCP
server started (recorded by the wrapper at paths written into it, never in a variable the server
could drop): what `Start` holds from the process's environment, argv and the config file it was
given (read at its own path with the bytes the run was given, then restored), the declared
`BaseURL` (its variable and default) and the bytes it read on stdin when a declared stdin flag was given
(`auth_header`, every `[headers]` value, the base URL's user and password, a file that does not
read the one canonical way held whole), then what the kit's transport holds from every request the
fakes received (auth headers whole and their parts, Basic auth's user and password, cookie values,
declared headers and fields) the process sent and every response it received, as sent (every header line, `Server`
and `Date` included; no body for a HEAD, 204 or 304 response): `Set-Cookie` values, declared
response fields in headers and bodies. Identifiers the connector declares, each with its reason,
are exempt as in the kit, and a value the list holds on its own is exempt where the kit exempts
its source, the source spelled exactly as the kit compares it. Each process is judged on its own
held set (holdings last until the process exits): its stdout and stderr, the SQLite statements it
ran (by pid), the requests it sent to the second host, and the files it changed (by a snapshot after
each process); an MCP server's with the CLI children it started, and a file no process can be named
for on every process's. A
value that arrives in a response is held only when it was delivered; such values, and the user of
a base URL the harness writes, carry no fixed prefix, so one never delivered cannot be guessed.
Every token the fake vendor hands out is held for the rest of that run, and so is every object key
beneath a declared response field (a vendor that stores a credential as a key; the kit holds the
values there, not the keys), except a value that occurs as written in the kit's public text
(`true`, `false`, `null`, a status code, a protocol word): the kit's own rule records it instead
(the result's `recorded` count) and does not hold it. A mint that never happened leaves only that
token unheld. Most cases inject one credential,
the first declared one the connector sends on the wire and already withholds when a vendor echoes it
(else the first on the wire); `each-env-var` injects every declared variable on its own, a declared
URL variable included. A vendor response field is held only when the connector declares it: the
`ResponseFields` (and `SecretEnv`, `Identifiers` with their reasons, and every other list) of the
one literal of the shared kit's own `Declarations` type, read from every package the connector's
two binaries link that reaches the kit, directly or through another package (a nested module's
included), under the build's own settings once for each release target (declarations that differ
between targets fail the connector's run). The type may be named only as that literal's
type, and the value used only as `credkit.Start`'s argument, a literal or a variable of the kit's
own type (the documented wiring: a literal in a wiring package, `credkit.Start(wiring.D)` in each
`main`), and `Start` is only ever called directly; its address taken, a copy kept in a field, a
field set or read elsewhere, an element of another literal, a call's result, a generic type
argument, an unnamed struct of the same fields, `Start` as a function value (reflection), an
identifier with no reason, a `BaseURL` that is not a literal, a `Files` callback (or any field the
list cannot read as data; `Origins` is not read, since the list treats every host but the fake's as
foreign) or a literal that does not type-check fails the connector's run.

Stdout and stderr are captured byte for byte, and in the writes the kit's pump checks one by one:
the bytes between two pauses of the pipe (5 ms here, 10 ms in the kit, so the list sees every
boundary the kit would). A stream is judged whole and, through the kit's own stream check, write
by write with the stream's context, so a short credential that ends one write is found even when
the next write puts a digit after it. Bytes written before a child the program started keeps the
pipe open are still judged.

Every SQLite statement a run executes is judged as the kit's unit, its text and bound values in
binding order, as it ran: the list builds each connector with `modernc.org/sqlite` replaced (`go
build -modfile`) by a mirror whose statement layer writes each statement through the kit's own
`sqlUnit` (a transaction's begin, commit and rollback, and the DSN the driver opens, too) to
`corpus-sql.trace` beside the executable, outside HOME: the place is found once at start from the
executable's own path, so no variable a process could clear decides it, and each process writes a
start record with its pid, so a process of a binary that links the driver and wrote none fails the
case. The build is the release build (`CGO_ENABLED=0`). A build that links another SQLite package,
replaces any `modernc.org` module, reaches the driver's C layer (`modernc.org/sqlite/<sub>`) from
outside the trusted modules (the driver, `modernc.org/libc`, `mathutil`, `memory`, by module, not by
a path's prefix) other than for its `SQLITE_` constants (read by type, so no spelling of the import
hides a use), carries assembly or a `go:linkname` that names SQLite, or whose driver the patch does
not fit cannot be judged and fails the run.

Every file write a run makes is a unit too (contract section 2: a file written as a stream of
writes). The same build patches the toolchain's `os.File` `Write`, `WriteAt` and `ReadFrom` (in a
hard-linked mirror of the connector's toolchain, since `go build -overlay` refuses files under the
module cache) to record each write to a regular file, whatever its name (by `fstat`, not by
spelling), with the file's device and inode and its name in the same trace before the write is
made; extending a file (`Truncate`, `os.Truncate`, a write past its end) records the zeros it
materializes. A write the trace
cannot record, or a write into the trace's own file, is not made: the process stops with exit 97
(the statement trace does the same). Each process's writes to each file (by device and inode, so
every alias of one file is one stream) are judged in order, whole and write by write, on that
process's held set, so a write later deleted or overwritten is still
judged; a stream the kit cannot read whole fails. A file can be written below `os.File` only
through a system call, so the trace build refuses any use outside the standard library and the
trusted modules (`golang.org/x/sys`, `x/net` and `x/crypto` added; never the connector's own
module) of a `syscall`, `x/sys/unix`, `x/sys/windows` or `modernc.org/libc` function that writes,
truncates, maps memory, splices or makes a raw system call (a raw call is allowed only with a named
read-only trap: `SYS_PROC_INFO`, `SYS_FGETATTRLIST`, `SYS_GETATTRLIST`), assembly that contains a
system-call instruction or a raw encoding (`WORD`, `BYTE` and the like) anywhere, and a
`go:linkname` naming a system call or a write.
The reader type-checks the connector with `go/types` against the packages `go list -export`
reports, under the settings the connector build uses (no inherited `GOFLAGS`, `GOOS` or `GOARCH`;
`CGO_ENABLED=0`), once for each release target, so a comment, a string, a `_test.go` file, `testdata` or a file the build excludes declares nothing,
an alias of the kit's type counts (a positional literal by field order), and a named string constant
counts by its value. A conversion to the kit's type from another type, a field of it
set outside a literal, an element that is not a constant string, a second literal and code that does
not type-check stop the connector's run instead of holding nothing. With no field declared, the two
vendor-field lines do not apply.

The six cases in section 6 of the contract (the MCP request id echoed back, scientific notation of
a non-digit credential, an undeclared numeric vendor field, a declared identifier, a split across
two runs, a piece shorter than 8 characters) are **advisory**: reported, never failing. Each such
line names its contract item in an `advisory` field, and the list does not load unless exactly
those six are advisory. An advisory line excuses only its own exception, in its own shape: the
request id only as the top-level `id` of a response frame (never a nested `id`), scientific
notation never for an all-digit credential, a numeric field never when the connector declares it.
A held credential that leaks any other way during it still fails. Every other line gates.

**The learning loop.** When a review finds a credential leak the list missed, the fix to the gate
is one line, usually in minutes:

1. Write the case as a line: `{"id":"succ-rot47","family":"fold","form":"rot47","refs":["acme r2 #12"]}`.
   Reuse the fields an existing line uses; `refs` names the finding.
2. If the line uses a form the engine does not know, add its encoder to `ENC` in
   `class_secret_corpus.py` (a reading the kit's matcher lacks is a kit change). A new shape or
   drive is a few lines in `shape_response` or `Engine.drive`. Land any engine change on its own
   first: the engine is the kit's matcher, the `credmatch` command and this harness, and a change
   that adds a baseline entry may touch nothing but `secret_corpus.jsonl` and
   `corpus_baseline.json`, because an added line anywhere the engine imports or CI runs it can
   rebind an existing case.
3. Run `python3 tools/maintainer/class_secret_corpus.py --self-test`. The deliberately leaky
   stand-in must fail the new case and the clean one must pass it; the self-test says which way it
   missed.
4. Run the case on every connector: `python3 tools/maintainer/class_secret_corpus.py --all --jobs 4 --only succ-rot47`.
   A brand-new case may land together with the connectors it already fails on: add those
   `(connector, case)` entries to `corpus_baseline.json` in the same change. That is the only way
   an entry is ever added.

From then on every connector runs the new line in CI, so the finding never has to be found by a
review again.

**The baseline only shrinks.** `corpus_baseline.json` lists, per connector, the gated cases it
fails on main today. In the CI row a listed failure is reported as a known failure; any other gated
failure fails the row and names the case, its form, shape, source and drive, and every place the
credential surfaced. `ci_guards.sh` guard 12 runs `class_secret_corpus.py --check-baseline --base
<merge base>` with no build: an entry may be removed at any time, and added only for a case id the
merge base's list does not have, and never in a change that also drops or rewords a case of the
base or changes any file other than the list and the baseline (the kit's matcher, the `credmatch`
command, the harness, a module it imports, a workflow that runs it: a renamed case is an existing
case, and an added line can rebind an existing encoder). Only an untracked bytecode cache is
skipped; a committed one is a change, and the guard runs Python with a fresh `pycache_prefix` so no
committed cache runs in place of its source. A
connector that has adopted the shared credential kit
(`cli/internal/credkit/` exists) may have no entries at all. When a fix lands, retire what now
passes, or an advisory line back to its own exception, with `class_secret_corpus.py
--update-baseline --slug <slug>` (it removes, never adds).

**Run it on one connector, or all of them:**

```bash
python3 tools/maintainer/class_harness.py --slug halopsa --class secret-corpus -v   # as CI runs it
python3 tools/maintainer/class_secret_corpus.py --slug halopsa -v                   # every case, every verdict
python3 tools/maintainer/class_secret_corpus.py --slug halopsa --only succ-rot13    # one case
python3 tools/maintainer/class_secret_corpus.py --slug halopsa --strict             # known failures fail too
python3 tools/maintainer/class_secret_corpus.py --all --jobs 4 --json-out /tmp/leaks  # whole fleet
python3 tools/maintainer/class_secret_corpus.py --self-test                         # both directions
```

`--all` builds every connector (set `GOCACHE` to a scratch directory and delete it afterwards;
run `disk_guard.sh gate` first on a shared Mac). A redirect case sends each client process's first
two data requests across hosts (the first attempt and the retry path) and then answers normally,
so a connector that refuses the hop is not held in its own retry backoff.

**What it cannot see yet (known limits).**

- The fake vendor is plain http on loopback, so contract 2(c)'s "over plain http" is not judged;
  a request to any other host is (every foreign host goes through the second fake, and an https
  one shows up only as its `CONNECT`).
- It seeds what is declared today: secret environment variables, the config file's `auth_header`,
  `[headers]` and `base_url` user and password, minted tokens, `Set-Cookie` values and vendor
  fields. Other config credential fields, credential flags, stdin and legacy config paths need the
  per-connector declarations of contract section 7, which no ledger records yet.
- A credential that reaches a sink only in a reading the kit's matcher does not make is not seen
  here either: the list judges every unit by the kit's verdict alone (contract section 8). The
  kit withholds a short value after a letter or digit (its readings start at every byte offset),
  so a 3-character credential fails more often than a long one.
- A SQLite database's files (with its `-wal`, `-shm` or `-journal`) are judged besides the traced
  statements: its schema statements, its rows one unit each and each table's rows joined, and every
  other byte. Each page image (database page or WAL frame) is judged whole, values included as the
  file holds them (adjacent values spell one string there), and its records are read as units too:
  by the rows for a live table page, as record units otherwise (every index record, every record of
  a freed or replaced image). An image is live only when reachable from a root with no WAL frame and
  no journal. The headers, bytes past the database's size, the `-shm` and the journal are pieces
  too, split at runs of 64 or more NUL bytes unless a held value has a NUL byte. The kit runs out of
  budget on some ordinary pages read whole (auvik's index pages), so a piece the kit cannot read
  whole is read again in 1 KB windows overlapping by half: in such a piece a reading that needs more
  than 512 bytes is not read whole. A database SQLite cannot open fails the case.
- The child process sink is the argv and environment the MCP server gives its own CLI, one unit
  judged on the server's held set (recorded by a wrapper, with its stdin when a declared stdin flag
  is given); other children are not observed. A child's exchanges with the fakes cannot be told from its MCP
  server's, so each child is acquired over its server's exchanges.
- A connector whose API is JSON-RPC or otherwise shaped (bitdefender) rejects the fake's rows and
  retries, so its success-path cases mostly exercise its error path.

**Secret-echo is a subset.** Every check the retired `class_secret_echo.py` made is a line here
(each such line carries a `secret_echo` field naming the check): the 403 that reflects every
credential in seven encodings plus `X-Request-Id`, `Location` and a truncated title, over four
reads in `--json` and `--agent` and eight MCP read tools (`err-echo-all`, and one line per
encoding); the 200 that reflects them in headers only, with MCP in its own HOME
(`succ-header-echo`); and the `--dry-run` preview (`dry-run-preview`). Its decoder self-tests
(each encoding, JSON escapes, offset, nested and separated base64, the frozen pre-#439 decoder
union, the shared-HOME cache) moved into `class_secret_corpus.py --self-test`.
`python3 tools/maintainer/class_secret_corpus.py --slug X --covers secret-echo` runs only those
lines; `apply_secret_funnel.py` proves its wiring with them under `--strict`.

### Wiring the secret-echo funnel into a connector: `apply_secret_funnel.py`

Before press 4.34.0, a fresh or reprinted connector fails the leak list's secret-echo lines: the press's masks cover a
credential only as written or URL-escaped (cli-printing-press #5145). `apply_secret_funnel.py --slug <slug>`
makes the change #444 made by hand on 72 connectors. `onboard.py` runs it after vendoring.

Press 4.34.0 (#5169) masks encoded forms itself, and its raw output passes those lines. The funnel still
applies: a reprint keeps the connector's hand-written code (token endpoints, novel commands), which never passes
through the generated masks and which the raw class test never exercises. The CLI and MCP error backstops and
the held set cover that code.

- It installs the canonical funnel from `tools/maintainer/secret_funnel/`, the one copy of record: the decoder
  (`internal/cliutil/secret_funnel.go`) and the CLI and MCP error backstops, plus their tests.
- It makes the anchored edits press 4.19 to 4.34.2 need:
  - `client.go`: the masking funnel, the whole-body error check, the HTML summary, the token mint and refresh
    error bodies, and the held credentials;
  - `root.go` and every CLI error envelope;
  - the MCP server's outermost tool middleware;
  - the generated `auth login` exchange.

  Press 4.34.0 moved three anchors. `maskCredentialText` declares `secrets` and ends in `finishCredentialMask`, and
  the funnel wraps that call. `New` returns early when its HTTP client could not be built, so the hold goes
  before that `if httpClient == nil` guard and covers both returns. Any other `New` with more than one return is
  still refused.
- It writes the `secret-echo-decoded-funnel` entry (class `secret-echo`) in `handfixes.json`, asserting only the
  edits that are present. On a reprint it replaces asserts the reprint made stale: an assert for one press shape
  of the mask-return or error-body site is dropped only when the same site's assert for the current shape holds.
  Every other recorded assert is kept, so a hand-added one that no longer holds still fails the proof.
- It is idempotent: a re-run changes nothing. `--check` changes nothing and exits 0 only when the connector is
  fully wired.
- Success is measured, not read back from its own edits. Every run passes the ledger through
  `check_handfixes.py --slug`. Every apply (one that changes nothing too) must also pass `go build ./...`,
  `go vet ./...` and `class_secret_corpus.py --slug <slug> --covers secret-echo --strict` with 0 failing; a failure exits 1
  and names the proof and the sites the run touched. The class test must target `skills/<slug>` in the
  registry and must exercise the connector: an advisory "not exercised" is not a pass unless the entry
  declares `"hand_wired": {"class-test": "<how the error path was proven>"}`. `--check` stays cheap: no build.
- It fails closed. It names, by site id, every site it cannot anchor: a hand-edited `maskCredentialText`, a
  custom error-body helper, or a hand-written token exchange that formats server text with no funnel call.
  Wire such a site by hand, add asserts for your wiring, declare it on the entry as
  `"hand_wired": {"<site id>": "<how it is wired>"}`, and re-run with `--check`.
- It cannot see a credential a connector reads from the environment outside its `Config`; the class test finds
  those.
- `ci_guards.sh` guard 10 runs `--check-drift`: every connector's funnel copies must match the canonical
  source, so change `tools/maintainer/secret_funnel/` and re-apply, never one copy by hand. A ledger that is not
  readable JSON fails it.
- `--self-test` covers every generated shape and the fail-closed paths.

### Fleet-wide hand-fixes after a reprint: `fleet_handfixes.json` and `replay_handfixes.py`

Sixteen hand-fix ids each sit in 10 or more connector ledgers (678 of 1,485 entries on 2026-10-23), in the
same shared template files, so every reprint re-reconciled them by hand. `fleet_handfixes.json` settles each
one once against a press release, and `replay_handfixes.py --slug <slug>` applies that settlement.
`onboard.py` runs it after vendoring, before the secret funnel.

- **retire**: the press generates the behavior from `retire_at` on. On a connector whose recorded press
  version (manifest.json and `press/.printing-press.json`, which must agree) is at or above it, the tool
  removes the stale entry and keeps a `retired` record with the registry's reason, and
  `check_handfixes.py` stops demanding the entry and demands the registry's `native_asserts` instead. Below
  `retire_at`, with no or disagreeing versions, or for a `keep_manual` connector, the entry is demanded as
  before. A `retired` record the registry does not back fails.
- **replay**: the press still lacks it, and a named recipe re-applies it on the press 4.34.0 and 4.34.2 shapes.
  Hand-written files the entry asserts on that the reprint lacks are restored from the pre-reprint revision
  (`--base`, default `HEAD`), dropping declarations the press now generates (a stronger hand-fix copy is
  kept under its own name); generated files get anchored edits, each pinned as a new ledger assert. A file
  the reprint carries is kept as printed and never replaced by its pre-reprint copy, since the reprint may
  carry a fix that copy lacks (servosity `doctor_verdict.go` and cipp `auth_login.go` on 4.34.0). If a
  carried file lacks an entry's needle, the run names it and exits 1; every whole-file copy goes through
  one write that checks the vendored tree itself. An assert the replay would lose is named unless the
  recipe marks it obsolete with a reason. Three shapes each recipe knows by name:
  - `derive-lock-canonical` recognises the generated `derive_test.go` by its body: the module path and a
    leading copyright line are dropped before hashing, because the press stamps whoever printed it there.
  - `doctor-verdict` on a templated base URL (endpoint template variables such as `{tenant}`): the
    generated check reads the resolved base, so the recipe also refuses the shipped template with every
    variable at the default `config.Load` gives it, names the base-URL variable, and drops the
    `doctorBaseURLIsPlaceholder(cfg.BaseURL)` needle the shape has no place for. Defaults are read
    positively from the config package (the press's `} else {` and `} else if ... == "" {` fallbacks); any
    other line that writes or names `TemplateVars` there is named for a person, never read as no default.
  - `no-config-write` brings back the `### Keeping credentials off disk (optional)` section of
    `guide.md`, which onboard.py re-vendors from the press README, in front of the heading that followed
    it at `--base`; a reprinted guide.md that carries its own copy of the section is named, not replaced.
- **delegated**: `secret-echo-decoded-funnel` belongs to `apply_secret_funnel.py`.
- **floors**: a module the print resolves below a patched version (`golang.org/x/text` v0.42.0,
  `github.com/klauspost/compress` v1.18.7) is raised with `go get`, then `go mod tidy`, on a print at or
  above the floor's `applies_from`, never below what `--base` pinned. Older prints get a note only. A floor
  the press meets is a no-op; set `retire_at` when a press resolves it.
- Success is measured: `check_handfixes.py --slug` over the ids this tool owns, and on an apply `go build`,
  `go vet`, `go vet` with cgo off once per release OS in `registry.TARGETS` (the host build compiles only its
  own platform's files, so a name declared twice in a Windows-only file would pass it), `go test` of every
  package it wrote or restored, each id's registry gates (for example
  `check_mcp_file_flags.py`, `check_doctor_truth.py`, a class test, or a proof test run with
  `go test -overlay`, never written into the connector), and after a floor raise `go test ./...` and
  `check_security_gate.py --slug --require-scanners`. `--check` changes nothing and runs only the ledger
  check; `--list-outputs` prints every path an apply would write.
- It fails closed: exit 1 names every site it could not anchor, every restore it could not make, every
  assert it would lose and every proof that failed.
- `--self-test` drives every recipe, retirement, keep_manual, delegated ids, floors and the fail-closed paths
  on a throwaway git repository.

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
| auth-wire | 5 | 79 | 0 |
| write-safety | 10 | 47 | 27 (8 read-only not proven, see above; 21 whose write commands never reached the API even without `--dry-run`, so the dry run could not be proven clean) |

The leak test list is not in `class_baseline.json`: its known failures are per connector AND per
case in `corpus_baseline.json` (see [The credential leak test list](#the-credential-leak-test-list)),
so a connector listed for one case is still gated on every other.

Step 7 of the pipeline plan empties these lists. No connector is fixed here.

### Review gaps closed (#439)

The final review round of #436 left five findings open, each with a concrete failing input and a
one-rule fix. All five are closed, each with a self-test (in `--self-test`) that fails on the old
harness and passes on the fixed one:

1. **secret-echo decode.** The decoder adds a scan whose base64 candidate runs break on
   base64-alphabet boundaries only (`=` only as trailing padding), so a blob right after `.`, `%`,
   `&`, `;`, `\` or `=` is decoded on its own rather than shifted by the prefix. It decodes the
   UNION of that scan and the original one, so a blob with an interior `.` is still reconstructed:
   a decoder change may add reconstructions, never lose one. The self-test runs a corpus through the
   pre-#439 decoder (frozen in `class_secret_corpus.py` since secret-echo was retired) and fails if the live decoder misses any of
   its positives.
2. **write-safety attribution.** Token and authorization URLs point at a reserved path on the fake
   server (`TOKEN_PATH`), never its root, so a connector's data write to `POST /` is never exempted
   as a token exchange.
3. **write-safety attribution.** Every request sent while a read-only session runs counts as
   read-only, its `tools/list` discovery included, so a write at startup (remembered in HOME) is
   caught. A self-test drives `run()` against a stand-in MCP server that does exactly that.
4. **secret-echo surface parity.** The MCP success probe gets its own fresh HOME, so a response
   cache shared with the CLI probe cannot keep the reflected header from reaching MCP. A self-test
   drives `run()` against a stand-in CLI and MCP server that share such a cache.
5. **baseline guard.** `--assert-baseline-shrinks` proves the baseline absent at the base with
   `git ls-tree`; a tree that names the file but whose blob cannot be read (an unhydrated partial
   clone) fails closed instead of passing as "new".

Still open in #439: per-connector argument recipes for the advisory connectors.

### Known limits (accepted)

- The redirect hop changes port, not hostname. The connectors' own `CheckRedirect` compares
  host:port, which is what the 5 findings exercise; Go's built-in Authorization stripping compares
  hostnames only, so a connector relying on it alone is tested only for its custom headers.
- auth-wire and write-safety do not judge what a 200 body prints. The leak test list does: under
  the contract a held credential in a 200 body is withheld like any other unit.
- Advisory connectors are coverage gaps, surfaced so they are never mistaken for passes.

### Cost

Each run builds the connector's two binaries and drives a bounded set of probes (at most 4 read
commands, 8 MCP read tools, 12 MCP write tools and 4 write commands for the dry run, each probe
process capped at 20 s). Measured serially on a warm module cache, all three classes take 6-29 s
per connector (connectwise-automate 6, ninjaone 9, halopsa 11, threatdown 14, td-synnex 19,
saas-alerts 29: mirror tools shell out to the CLI, and a connector that refuses the cross-host
redirect retries with its own backoff). A whole-fleet run takes about 4 minutes with 8 workers.
The credential leak test list runs its cases 16 at a time and takes 2-28 s per connector after the
build on an M-series Mac (halopsa 14, zammad 15, saas-alerts 28); a connector that backs off on its
own after an error takes longer (cisco-duo 87, bitdefender 93, backup-radar 140). The whole fleet,
84 connectors at `--jobs 4`, took 443 s (582 s with a cold Go build cache). The classes run per changed connector in the `build` matrix row; a change
to the harness is MACHINERY (see `release_matrix.py`), so it re-runs the fleet.

## The shared credential kit: `credkit`

`tools/maintainer/credkit` is one Go package that makes a connector keep the
[credential leak contract](../../docs/credential-leak-contract.md): it knows every credential the program holds and
stops any of them from leaving through any output, in any of the encodings the contract lists. Every connector that
adopts it carries the same bytes, so the fleet enforces one contract with one implementation instead of 65 hand-made
filters.

What it does, by contract section:

- **Held credentials (1).** `Start`, the first statement of `main`, holds every credential known at startup before
  anything is written: the declared secret environment variables, every credential file (read token by token, so a
  file that does not parse still gives up its credentials; legacy paths and a `--config` path included), the password
  and query values inside the base URL, credential flags (scanned from argv before the flag parser runs, so a parse
  error that echoes one is withheld), and piped stdin when a stdin flag is declared. The outbound transport holds
  request credentials, `Set-Cookie` values and declared response fields before the program sees them: a plain response
  body is scanned as it streams, however long, and a field value reaches the program only once it is held; an encoded
  body is decoded first. The MCP wrappers hold declared tool arguments before any handler runs. A credential file that
  exists but cannot be read, or a response in an encoding the kit cannot read, withholds everything. A source is
  exempt only when an identifier names it exactly (a header name ignores case).
- **What counts as a leak (2) and the decoders (3).** The matcher finds a held value whole (a short one only as a
  whole token) or any 8-character piece of it, after every decoding the contract names, applied recursively at every
  offset, with NFKC and case folding on both sides. No decoder is limited to once per path; only a decoder reading
  its own output again directly is skipped where that is provably nothing new. It has a depth and decoded-bytes
  budget; running out of it withholds the unit, and a decoder error never makes a unit clean.
- **Outbound requests (2(c)).** `credkit.Transport` refuses a request, before it is sent, when it carries a held value
  to an origin outside that value's declared origins, over plain http, or across a redirect to a different origin.
- **What happens on a match (4) and the sinks (5).** The whole unit is replaced by a fixed notice that names the sink.
  stdout and stderr are redirected at the file-descriptor level, so `fmt`, `log`, a panic trace and an inherited child
  process all pass through it. Files (`WriteFile`, `AppendFile`, `Create`, `OpenFile`, `NewWriter`; a config write
  has no exemption), SQLite (`SQLDriver`), MCP (`MCPIn`, `MCPOut`, `MCPHTTP`, `MCPTools`) and child processes
  (`StartCommand`, `RunCommand`) each check one unit at a time. If the output layer cannot be installed, the program
  exits with status 70 before writing anything.
  A program about to exit 0 whose stdout did not all reach its reader (a refused write, or a unit dropped along with its
  notice) exits 74 instead (`ExitOutputLost`); a withheld unit whose notice arrived keeps the program's own status.
- **Child processes (1, 5).** A child's argv and environment are one unit, and a child that would carry a held value is
  refused. The connector's own program is not refused for its environment: its own binary, or its `<x>-mcp` / `<x>-cli`
  companion in the same directory (each name judged by the file it resolves to). A variable that would carry a held value
  goes down the inherited pipe with the held set, and `Start` in the child sets it again before `main` goes on, so a shell-out
  tool authenticates with a key the user set in the environment, with no wiring, and the child's environment stays clean.
  Any other program, a same-named one found through `PATH` or `<SLUG>_CLI_PATH` in another directory included, is refused,
  and a credential in argv is refused for every program.
- **A file other programs append to (5).** Several processes share the learn journal and the logs. A write that appends
  tolerates growth it did not make: it is still checked with everything this process wrote to the file, and the kernel lands
  it after what the others wrote. The kit no longer proves those bytes are its own, so a credential completed only by joining
  this process's bytes to another's is contract 6 item 5, two runs. A file that shrank, or grew under a write at an offset,
  still fails closed.
- **Declarations (7).** A connector fills in one `credkit.Declarations` literal in one hand-written wiring file: its
  secret environment variables, credential files and fields, credential flags and headers, response fields, MCP
  arguments, each credential's allowed origins, and each exempt identifier with its reason. Every entry point is a plain
  function call, so a type checker can find every declaration and every routed sink without running the program.

How a connector adopts it:

1. `python3 tools/maintainer/vendor_credkit.py --slug <slug>` copies the package to
   `skills/<slug>/cli/internal/credkit/`, adds `golang.org/x/text`, `mcp-go` and `golang.org/x/sys` to the connector's
   `go.mod` at the version most connectors use if it lacks them, routes the press template's
   `internal/cliutil/darwin_acl_darwin.go` (it runs `dsmemberutil`) through `credkit.RunCommand` in one canonical form
   whose reviewed security entries hold for every adopter, and proves the copy builds.
2. Write the wiring file with the `Declarations` literal, call `credkit.Start(...)` as the first statement of each
   `main` and end the program with `credkit.Exit(code)` (returning from `main` would drop output still in the pipe).
3. Route each sink through the kit: the HTTP client's transport, file writes, the SQLite driver, the MCP server's
   input, output and HTTP handler, child processes, and `credkit.Terminal(os.Stdout)` wherever the program asked
   whether its output is a terminal.

`Declarations.Files` finds a config file named on the command line with `credkit.FlagValues(args, "--config")`, not
a scan of its own: `check_mcp_file_flags.py` reads a call with a constant flag name as that flag's value (tied to the
flag the MCP server refuses), while any other argv scan that reaches a file is `os.Args` reaching a file and fails.

The fleet gates read the vendored kit only while `vendor_credkit.carrier_drift` proves it byte-identical to this one:
`check_env_schema.py` then takes the names the kit reads for the connector (`SecretEnv`, `BaseURL.Env`) from its
`Declarations` literal and requires them in the manifest; `check_mcp_file_flags.py` ties the kit's `--config` read
to the `--config` flag. The kit itself carries no `unsafe` import and no scanner finding, so the security gate scans
it like connector code with no suppression. `check_doctor_truth.py`, `class_harness.py` and the verify skill serve
their fake vendor over https to an adopter (`kit_tls.py`: a certificate made for the run, trusted through
`<PREFIX>_CA_CERT` and `SSL_CERT_FILE`), because the kit refuses to send a credential over plain http. Fixture
credentials are written so that no 8-character piece occurs in ordinary output: the kit holds them and withholds any
output sharing a piece (`check_doctor_truth.py --self-test` scans every connector's doctor source for its probe).

How drift is caught: `vendor_credkit.py --check` (ci_guards guard 11) fails when any connector's copy differs from
the kit by one byte, misses a file, or carries a file the kit does not have, and names the file. Change the kit here,
then re-run `--slug` for each connector. The `credkit` workflow (`.github/workflows/credkit.yml`) runs the kit's contract tests under the race detector,
vets it for Linux, macOS and Windows on amd64 and arm64, and vets it against every `mcp-go` version the fleet requires.

Not yet: no connector uses the kit in this change. The checker that proves a connector's wiring is complete is
`credkit_verify` (next section); the leak test list that drives every connector against the contract is a separate change.

Base URLs and other URLs held from startup: an explicit `--config` chooses the base URL over a discovered default (the
environment still overrides, and every file is still held), so a default's host never stays an allowed origin when the program
reads another file. `Declarations.URLEnv` (optional) names other variables that hold a URL, such as a token URL or a feedback
endpoint; the password and query values of each are held, and they leave a third-party child's environment. A proxy variable's
user and password (`HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, either case, the schemeless `ops:secret@proxy.example:3128` Go
accepts included) are held unconditionally, bound to that proxy's origin.

An MCPB host leaves `${user_config.<key>}` in a variable the user left empty. That text is no value anyone gave the
program, so the kit does not hold it, as `cliutil.EnvOverride` does not read it; any other value in that variable is held.

Known limits (accepted): a single write over 4 MiB to stdout or stderr is checked in 4 MiB units; an encoded
response body over 64 MiB, or one in an encoding other than gzip or deflate, withholds everything from then on; a
credential typed at a terminal prompt is held by the connector itself with `credkit.Hold("stdin", v)`; a credential of
a few characters is withheld wherever it appears as a token, decoded output included, so it withholds more often than a
long one. Checking every decoding recursively costs time: about 0.1 s for 15 KB of JSON and 1 s for 150 KB, and a unit
that runs out of budget is withheld, which a credential with non-ASCII characters makes far more likely. On Windows, a
stream the program captured before `Start` (a package-level logger) writes into the kit's pipe; if that stream was a
console, its writes fail rather than reach the console.

## The kit's wiring checker: `credkit_verify`

A connector wires the [shared credential kit](#the-shared-credential-kit-credkit) by hand. The checker does not wire
anything and does not guess: it reads the connector the way the Go compiler does (`go/packages` and `go/types`, so an
aliased import, a dot import, a function used as a value, a method promoted from an embedded field, a generic type
argument and a file that only one operating system or architecture compiles all resolve to what they are) and checks the wiring is complete. Anything it cannot
classify fails with its file and line, what it is, and what would satisfy it. That is a to-do for the connector, never
a guess.

```bash
go -C tools/maintainer/credkit_verify run . --slug <slug>      # or --dir skills/<slug>/cli
```

It loads the connector six times, as the release builds it (linux, darwin and windows, each on amd64 and arm64, cgo
off), and checks five rules. Exit 0 means nothing is unclassified; exit 1 lists the findings; a connector without
`cli/internal/credkit/` is skipped with a note and exits 0.

It checks each load twice and reports the union. The first pass walks the syntax tree with `go/types` (the rules
below). The second builds the program in SSA form (`golang.org/x/tools/go/ssa`, every generic instantiation with its
own body) and runs one switch over every SSA instruction and operand kind, each modeled or failing; a test fails the
build when a new `x/tools` adds a kind the switch lacks. In SSA a value that holds a command, client or transport
(anywhere in its type, a fixed point over fields, elements, type arguments and method signatures, and any interface a
command, client or transport satisfies by running or sending) flows only through the uses rule 2 allows; no generic, the connector's or another package's, may be instantiated with such a type. A call
through such an interface passes only when every method VTA finds for it is the connector's, and an assertion to such
an interface fails, the connector's own included. When such a value becomes an interface, the interface value is followed through connector code
(calls into connector functions, returns, locals, closures, unexported fields of connector structs, and calls of its
own type's methods) and fails where it leaves: another package's call, an exported field, an address taken, a call
the checker cannot resolve, or a return from a function another package might call (one whose value goes anywhere but
a direct call, a method whose bound value does, or a method of any type inside a type converted to an interface or
given as a type argument to another package's generic function or type, used or called: its elements, fields and type
arguments, which reflection reaches), whose callers the call
graph does not all hold. Before `Start` is computed on the SSA call graph (static callees plus VTA for function
values and interface calls); there a dynamic call with no resolved callee fails, unless it calls a function field of a
connector struct nothing ever sets, or a function value whose type names a connector type (only connector code, which
VTA sees, can make one). Any use of `reflect` or `unsafe`, any call into `syscall` or `golang.org/x/sys` (a constant
such as a signal number is not a call), cgo, a `//go:linkname` line, an assembly file, and a binary in the connector's
tree (git-ignored build output aside) fail unless the ledger lists them with a reason. These markers are the whole
defense against code written to defeat the check: such code is out of the checker's threat model, because its author
already holds the credential. A package outside the connector that imports a connector package fails too: it could build the
connector's types, which the checker otherwise trusts as built by connector code.

1. **The prologue.** The first statement of every `main` is `credkit.Start(...)`, given the connector's one
   `credkit.Declarations` literal (directly, or the package-level variable it initializes). The literal must be plain
   data the checker can read: constant strings, keyed fields, and `Files` as a function literal or a package-level
   function. The variable is used nowhere else, so nothing changes it after the checker read it. The site ledger cannot
   waive this rule.
2. **Every sink routed.** File writes (`os.WriteFile`, `os.Create`, `os.CreateTemp`, `os.OpenFile` with write flags or
   flags that are not a constant, `os.NewFile`, `os.CopyFS`, `ioutil`, `os.Root`) go through `credkit.WriteFile`,
   `Create`, `OpenFile` or `AppendFile`; a helper such as `AtomicWritePrivateFile` is caught at the primitive inside it.
   Every `http.Client` literal sets `Transport: credkit.Transport(...)`; an `http.Client` held by value anywhere else (a
   variable, `new`, `make`, an array, a struct field, or another package's type that holds one), `http.DefaultClient`, `http.Get`/`Head`/`Post`/`PostForm`, a client or round tripper
   built by another package (and anything from `golang.org/x/oauth2`), `http.DefaultTransport` outside
   `credkit.Transport(...)`, a direct `RoundTrip`, and a replaced `Client.Transport` all fail. `http.NewRequest*` is
   listed in the ledger with which routed client sends it. `sql.Open` names a constant driver registered as
   `sql.Register(name, credkit.SQLDriver(driver))` among the leading statements of an `init` function (only other such
   registrations may come before it, so no early return or branch can skip it) in the opening package or a package it
   imports; `sql.OpenDB` fails. A child process starts only through
   `credkit.StartCommand` or `RunCommand` (`Cmd.Start`, `Run`, `Output`, `CombinedOutput` and `os.StartProcess`
   fail). An MCP server gets `server.WithToolHandlerMiddleware(credkit.MCPTools)`, its stdio server listens on
   `credkit.MCPIn(os.Stdin)` and `credkit.MCPOut()`, and its HTTP server is served only inside `credkit.MCPHTTP(...)`
   (`ServeStdio` and `Start` fail).

   **Sink types are an allow-list.** For `*exec.Cmd`, `*http.Client` and `*http.Transport` the checker does not name
   unsafe shapes; it walks every typed expression and accepts only the uses it lists. A value of one of those types,
   or of a slice, map, channel, pointer, function or tuple holding one (or of a named type over one of those; a named
   struct is checked field by field where its fields are read), is checked for every one of the three it holds, and is made only by `exec.Command` or
   `exec.CommandContext` (a command), a checked `&http.Client{...}` literal (a client), a `&http.Transport{...}`
   literal, `new(http.Transport)` or `Clone()` (a transport, which must then be wrapped), or a connector function whose
   declared result is that type; it moves only to a place of exactly its own type (a variable, a field, an element, an
   argument of a connector function, a result); and its methods are only `Wait`, `String`, `Environ` and the output
   pipes (a command, which runs only through `credkit.StartCommand` or `RunCommand`), `Do`, `Get`, `Head`, `Post`,
   `PostForm` (a client) and `Clone` (a transport, which sends only inside `credkit.Transport`). Every other expression
   of those types fails: a conversion, a type assertion or type switch, an interface (including a tuple element
   assigned or passed to one), another package's function, field or variable, an `exec.Cmd` literal, `new` of a
   command or client, a method called through an interface. No generic function or type may be instantiated with a
   type that holds a command, client, transport, round tripper, `*os.File`, database handle, SQL driver, `slog` handler
   or `log.Logger`, or a type that holds a command, client or transport anywhere in its structure (a named struct's
   fields included): the checker does not follow a sink through a type parameter, so the instantiation fails outright.
   A conversion `T(x)` of a value that holds a command, client or transport anywhere in its structure to another type
   fails: the new type's fields would read as the connector's own while the sink inside came from elsewhere.
   Reflection and `unsafe` reach inside named structs, so a value holding a command, client or transport anywhere in its
   structure is never handed to a `reflect` function or converted to `unsafe.Pointer`, and no connector code writes or
   calls through `reflect` (a `reflect.Value` `Set` method, `Call`, `Method`, `Addr`, `Send`, `Clear`, `reflect.Copy`,
   `Swapper`, `NewAt`, `MakeFunc`); reading through `reflect` (`ValueOf`, `Kind`, `IsNil`, `Field`) is allowed only as
   a listed site, as every use of `reflect` and `unsafe` is.
   `unsafe` may only turn a pointer into a `uintptr` for a system call (`unsafe.Pointer(&x)`, then `uintptr(p)`) and
   measure (`Sizeof`, `Offsetof`, `Alignof`): an `unsafe.Pointer` made from a non-pointer, converted to anything but
   `uintptr`, or moved with `unsafe.Add`, `Slice`, `String`, `SliceData` or `StringData` fails. An `exec.Cmd` held by
   value (a field, variable, array or `new` of the struct type, not a pointer) fails as an `http.Client` by value does.
   A type assertion to an interface that a command, client or transport satisfies with a method that runs or sends
   (anything but a command's `String` and `Environ`, a client's `CloseIdleConnections`, a transport's `Clone`) fails:
   a value recovered from `any` (through `reflect`'s `Interface`, say) may be one the checker never saw built. A
   generic instantiated with such an interface fails too, since an assertion to its type parameter does the same, and
   so does an assertion or instantiation with a type that carries one (a pointer, container, function or unnamed
   struct holding it). Such an interface (one a client or command satisfies by sending or running) may not come from
   another package's function, method, variable or field, nor reach the connector as a parameter another package
   fills: a function, or a value with a method, whose parameter carries one may not be handed to another package
   (an argument, its variable or field, its composite literal), whether it names a function or a function variable.
   A value that holds a command, client or transport anywhere in its structure (a named struct's fields included) may
   not be handed to another package either: a decoder handed `&envelope` fills the client inside it. A range clause
   is a write: `for _, c.Transport = range ...` fails as an assignment does. A call of a function value is checked as its value is,
   so another package's function variable cannot produce a client; and a method or field reached by promotion
   through another package's embedded field that holds a command, client or transport fails as reading that field
   would.

   A value whose method would start one of these (a `*exec.Cmd`, a transport, an MCP
   server, or a type embedding one) may not become an interface outside the kit's own call (a generic type argument
   meeting its constraint counts, as do a range clause assigning to an interface and an interface map key), and a type assertion to an interface with such a method fails, so none can run out
   of sight. `os.Stdin`, the one open file the kit neither redirects nor opened, may only be read: a read method, the
   kit, or a parameter or variable of an interface type with no write method; asserting such a reader back to a type
   that can write fails. Standard output and error are redirected by the
   kit from `Start` on, so output is routed by construction; what the checker flags is output that runs before `Start`:
   `fmt`, `log`, `slog`, `print`, file writes, any call handed a writer, and any method of a value that writes (an
   `io.Writer` or a `slog` handler, whatever the method's name: `Handle`, `Flush`, `Write`, called as a method value or
   a method expression), in an `init` function, a
   package-level initializer, or any function or method one of those references or converts to an interface. Only
   `strings.Builder` and `bytes.Buffer`, by exact type, count as memory; `log.New`, `SetOutput` and `slog`'s handler
   constructors only capture a writer; a writer among a call's results handed straight on (`f(g())`) counts too. A request, `sql.Open` or a database call there fails too. A system call (`syscall`, `golang.org/x/sys`) is not classified and fails.
3. **Every field and variable classified.** Each environment variable the code reads (`os.Getenv`, `os.LookupEnv`,
   `os.ExpandEnv` of a constant, an `env:"NAME"` struct tag, and wrappers, followed through their call sites to a
   constant name) is in `SecretEnv`, is the base URL's `Env`, or is an identifier `env:NAME` with a reason. A name that
   is not a constant (`os.Getenv(prefix + s)`, `os.Environ`, a wrapper used as a value) fails until the ledger lists
   the site with every name it may read, each then classified. A wrapper that changes its parameter, or a wrapper
   method an interface can reach (through a tuple too), is not followed and fails the same way. Each field of the type
   every `source` read actually decodes into (wherever it is declared, a function-local type included), of every type
   named `Config`, and of any type the ledger names, that can hold a string or a number (numbers count; a bool holds one bit; an exported field of a type that decodes itself, such as `time.Time` or `big.Int`, or a struct that embeds one, counts too), nested and embedded structs included, is classified under each key it is
   written as (its `toml`, `json`, `yaml` and `mapstructure` tag names, or its Go name, which is also its `json` key when
   `encoding/json` rejects the tag's name (a tab in it, say) and its key `-` for a `json:"-,"` tag; and, for a type a `source`
   read decodes, the key that decoder reads, which is its Go name when only another format's tag is present: a
   `toml:"token"` field decoded by `encoding/json` is the key `Shadow`; every decode counts, `DecodeFile` included): a credential field, a stored
   record, the base URL key, `auth_header`, a `headers` table (a map or a struct: the kit holds what is beneath it), or
   an identifier `config:KEY` with a reason. A `Config` that is not a struct (or a pointer to one, a cycle of pointers
   reported, not followed) fails. A `source` read's content may only be decoded straight into a struct in the same
   function (`Unmarshal(b, &cfg)`, `NewDecoder(f).Decode(&cfg)`, through `io.ReadAll` too, or a `DecodeFile`); its bytes
   or file going anywhere else, or a target that is a map or an interface, fails, as does a yaml `,inline` or a
   mapstructure `,remain` map (it takes every key no other field takes, so no declaration can classify them), as does a target that holds,
   anywhere in its structure, a type outside the standard library (a package with no module) with its own decoding method (`UnmarshalJSON`,
   `UnmarshalText`, `UnmarshalTOML`, a `Decode` method, one promoted from an embedded interface included), or any
   interface (a decoder fills whatever value it holds): what either does with the file is not followed. A field a
   reprint adds fails until classified. A key declared both a credential and an identifier fails.
4. **Every file read listed.** The kit holds a credential file only when `Declarations.Files` names it, and the checker
   cannot tell from code which read is a config, credential store or token cache. So every read (`os.ReadFile`,
   `os.Open`, a read-only `os.OpenFile`, `os.DirFS`, `ioutil.ReadFile`, `os.Root`) is listed in the ledger: kind
   `source` (a file `Files` names) or `exempt` (not a credential file), with the reason. A `source` entry names `via`,
   a package-level function, and the checker proves the claim by shape, never by equal-looking values: `via` is
   `func() string` or `func(args []string) string` and pure (it reads no package-level variable, writes only its own
   locals, takes no address, and calls only builtins, conversions, `strings`, `strconv`, the `path` and `filepath` path
   functions, `os.Getenv`, `os.LookupEnv`, `os.UserConfigDir`, `os.UserHomeDir`, `os.TempDir`, `credkit.FlagValues`
   (how a path function reads `--config`, as `check_mcp_file_flags.py` also expects) and connector functions
   pure the same way, each proved again for each set of parameters given the command line; it passes no named
   function or method as a value, ranges over no map; and reads the command line only by `len`, `range` or an index, parentheses
   included); `Files` is pure too and returns a
   `[]string` literal holding `via()` (or `via(args)`, its own parameter) on every return; and the read's path is
   `via()` (or `via(os.Args[1:])`), directly or through parameters every caller fills with one. When a path reads the
   environment, an environment write anywhere (`os`, `syscall`, `golang.org/x/sys`) fails; when it reads the command line, a use of `os.Args` other than
   `len`, an index or `os.Args[1:]` handed to a declared `via` fails. While any `source` is declared, a change of the
   working directory (`os.Chdir`, `syscall.Chdir`, `Fchdir`, `(*os.File).Chdir`, Windows' `SetCurrentDirectory`) fails: a relative path Start held
   would then name another file. While any `source` is declared, a variable of `strings`, `strconv`, `unicode`,
   `unicode/utf8`, `path` or `path/filepath` (other than an error value) may only be an argument of its package's
   functions: `unicode.CaseRanges = nil` changes what `unicode.ToUpper` returns. A file moved or linked into place
   (`os.Rename`, `os.Link`, `os.Symlink`, and `os.Root`'s) writes its destination without the kit and fails like any
   other file write, so a file Start held cannot be swapped.
5. **Every credential has an origin.** Each declared credential (secret variables, credential and stored fields,
   credential flags, stdin, headers, request fields, response fields, MCP arguments) has an `Origins` entry with at
   least one allowed origin, each written as `scheme://host[:port]` or `base_url`. As in the kit, a credential's own
   entry decides when it has one; only without one does the `*` default apply.

**The site ledger**, `skills/<slug>/credkit-sites.json` (beside `handfixes.json`, outside `cli/`, so a reprint never
rewrites it), lists each site the checker cannot see routed, by file, enclosing function and primitive, with how many
such references that function holds:

```json
{
  "config_types": ["internal/config.Config"],
  "sites": [
    {"file": "internal/config/config.go", "func": "Load", "what": "os.ReadFile", "count": 1,
     "kind": "source", "via": "internal/config.Path", "reason": "the config file, which Declarations.Files names"},
    {"file": "internal/client/client.go", "func": "Client.do", "what": "net/http.NewRequestWithContext", "count": 1,
     "kind": "exempt", "reason": "sent only by c.http, built with credkit.Transport"}
  ]
}
```

Each finding prints the exact `file`, `func` and `what` to copy. An entry whose count no longer matches (a reprint
added a site), or that matches nothing (stale), fails, and so does one without a reason. Field and variable
classifications live in the `Declarations`, not the ledger.

How it is proved: `testdata/base` is a connector wired completely, and each case in `verify_test.go` changes one shape
and must yield exactly the findings it names (its routed or classified twin yields none), including an aliased import,
a dot import, a method value, an environment wrapper and a wrapper of it, a method promoted from an embedded field,
an embedded `Config` struct, a generic type argument, and files only the windows or the arm64 build compiles. Removing
any rule from a copy of the checker makes at least one case fail. The `credkit-verify` CI job runs them under the race
detector, and that job blocks. The build row also runs the checker for each connector, but that step is advisory for
now: findings show on the row without failing it, and `verify_connector.sh` does not run it. Real adopters measured 248
to 1652 unclassified sites, most of them press-template code, so the step goes hard, in both places, once a fleet sweep
has classified those sites centrally and comes back clean.

Known limits (accepted): a call through an interface into another package's code is not followed (a connector type
converted to an interface is: its methods join the before-`Start` closure); a third-party package that sends its own requests without returning a client is not seen, except
`golang.org/x/oauth2`, which is flagged whole; reflection that only reads is not followed; a file that only a cgo build compiles is not
checked.

## Security gate: `mcp-http-no-bearer-auth`

`check_security_gate.py` fires this MCP-surface rule when a connector's streamable HTTP server
can be served without a bearer-auth wrapper: every `NewStreamableHTTPServer(...)` must be an
argument of `requireBearerAuth` or `requireBearerAuthWithOrigins` (press 4.34.2: the Origin
allowlist, then the bearer check), or be held in a variable whose every later use is. Both names
are trusted alike. A near-miss name, a discarded wrapper result, or a name that appears only in a
comment or a string still fires.

Accepted limit: the rule checks placement, by name only. It cannot detect adversarial source, such
as a wrapper body that does not enforce the bearer, a local that shadows the wrapper, or a
redeclared identifier. Bearer enforcement is proven at runtime by the verify gate (feature 3,
`mcp-http`, below): no token, a wrong token and, on press 4.34.2 and later, a foreign Origin must
be refused. It blocks in every connector build row unless `verify_baseline.json` lists that
connector's `mcp-http` FAIL (prints before press 4.34.0). Per-PR Codex review covers adversarial
source.

## Verify gate

`.claude/skills/verify-msp-skills` drives one built connector through the user path (install,
MCP stdio, MCP HTTP token and loopback, write gating, read-only switch, local data) and writes
`verdict.json`. Every build row runs it through `verify_gate.py --slug <slug>` and uploads the
verdict as the `verify-<slug>` artifact (30 days), which `verify-watch.yml` reads (added in its own
PR, alongside `verify_maintained.json`).

The baseline was seeded from real Linux CI verdicts, not local runs: the PR that added the gate ran
the full matrix once in report-only mode, `verify_gate.py --seed-from <artifacts> --write` turned
those verdicts into the baseline, and the next push switched the step to gating.

A reprint on press 4.34.0 or later flips features 3 to 6 to PASS. The row then fails with
`now PASSs but verify_baseline.json still lists it`: remove those entries in the reprint PR itself.
An ERROR means the harness broke, so fix the harness (or the runner), not the connector. A new ERROR
blocks. The five the fleet had at seeding (auvik, fortinet, proxmox and zammad write-gating: the
harness finds no write endpoint to drive; n-central mcp-stdio: its auth flow rejects the fake vendor's
generic reply) are listed under `known_errors`: reported with the harness-error label, not gating, and
kept visible by `verify-watch.yml`, until the skill is fixed. A fixed one that now FAILs by design
moves to `connectors` in the same PR; the shrink guard allows that move and refuses the reverse. The skill's own map is kept current by `/maintain-verification-skill`; `verify-watch.yml`
opens a `verify-map-stale` issue when it falls behind.

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
main has not moved since the rebase, and the required approvals are current. It records whether
CodeRabbit reviewed that exact commit. A refused merge is re-validated from the top, never retried
blindly.

**CodeRabbit: a review of the exact head, not a green status.** CodeRabbit's commit status is
`success` whether it reviewed the commit, skipped it ("Review skipped: 336 files exceed the limit
of 100") or was rate-limited ("Review rate limited"). So the final gate reads what the CodeRabbit
app itself wrote (login `coderabbitai[bot]`, type Bot): a review on the head commit, or its
summary comment's record of a review covering the head. A review of an earlier head does not
count; squashing and rebasing make a new commit, which CodeRabbit must review again. States:
`reviewed`, `pending`, `skipped`, `rate-limited`, `failed`, `unreadable`, `none`. By default
(`--coderabbit report`) the state is recorded in the log, in the PR's state file (next to the
approvals) and in the `MERGED` line, and never blocks the merge. `--coderabbit require` stops on
anything but `reviewed` with exit 4 (`pending` and `unreadable` validate again first, bounded by
`--max-rebases`). The default is `report` because, measured on the 30 PRs merged before
2026-10-23, 12 had a CodeRabbit review of their final head, 9 were skipped for size and 9 were
rate-limited (Damien-authored PRs run on CodeRabbit's open-source allowance, about one review an
hour), and a PR over 100 files can never get one. The existing rule that every commit status
must finish still waits out CodeRabbit's "Review in progress". `status` prints the head's state
and the states recorded at final gates.

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
conflict it will not resolve; 4 an approval is not current, or (`--coderabbit require`) CodeRabbit
has not reviewed the exact head; 5 a check failed; 6 timed out; 7
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
