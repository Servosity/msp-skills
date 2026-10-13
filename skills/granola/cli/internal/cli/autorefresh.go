// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// agentContextAutoRefresh is the auto-refresh contract surfaced in the
// agent-context JSON so introspecting agents can discover the opt-out
// surface without scraping --help.
type agentContextAutoRefresh struct {
	Default      string   `json:"default"`       // "on"
	Flag         string   `json:"flag"`          // "--no-refresh"
	Env          string   `json:"env"`           // "GRANOLA_NO_AUTO_REFRESH"
	ProfileField string   `json:"profile_field"` // "no-refresh"
	Surfaces     []string `json:"surfaces"`      // ["api"]
	SkipList     []string `json:"skip_list"`     // command names that bypass refresh
}

// buildAutoRefreshContext mirrors the auto-refresh constants and skip
// list above so the agent-context JSON stays in sync with live behavior.
// If the skip list grows, this function must update in the same change —
// autorefresh tests pin the list.
func buildAutoRefreshContext() agentContextAutoRefresh {
	skip := make([]string, 0, len(noRefreshCommands))
	for name := range noRefreshCommands {
		skip = append(skip, name)
	}
	sort.Strings(skip)
	return agentContextAutoRefresh{
		Default:      "on",
		Flag:         "--no-refresh",
		Env:          "GRANOLA_NO_AUTO_REFRESH",
		ProfileField: "no-refresh",
		Surfaces:     []string{refreshSurfaceAPI},
		SkipList:     skip,
	}
}

// PATCH(auto-refresh): central dispatcher for the pre-command refresh.
// Whenever any command runs (outside the skip list and any opt-out),
// PersistentPreRunE calls runAutoRefresh as its first data action so
// downstream reads see current data without the user remembering to
// run `sync` first. Refresh is best-effort: failures print a one-line
// warning and the underlying command proceeds against whatever data
// the local store already has.

// noRefreshCommands lists command leaves and ancestors whose presence
// in the running command's lineage suppresses auto-refresh entirely.
// Walked via cmd.Parent() chain in shouldSkipAutoRefresh.
//
// Each entry is justified:
//
//	sync, sync-api: would recurse (auto-refresh calls them)
//	auth          : refresh requires auth — chicken/egg
//	doctor        : doctor reports current state; auto-refresh would
//	                side-effect that state (write SyncState) before
//	                doctor reads it
//	help, version, completion, agent-context, which:
//	              : no data dependency
//	profile       : profile management is local-only
//	feedback      : feedback is local-only (or remote-but-not-data)
//
// Names match cobra Use:; aliases (e.g. "sync-api") are matched as-is.
var noRefreshCommands = map[string]struct{}{
	"sync":          {},
	"sync-api":      {},
	"auth":          {},
	"doctor":        {},
	"help":          {},
	"version":       {},
	"completion":    {},
	"agent-context": {},
	"profile":       {},
	"feedback":      {},
	"which":         {},
	// Hand-wired (handfixes.json: mcp-auth-skips-auto-refresh): mcp-auth
	// status/verify/login/logout manage the Granola-MCP tokens themselves.
	// An auto-refresh in front of them could enrich private notes and so
	// refresh (and rotate) those tokens before a "passive" status ran.
	"mcp-auth": {},
}

// refreshSurface labels which auth path a refreshResult came from so
// the provenance line can distinguish "cache=ok api=skipped" from
// "cache=failed api=ok". Stable strings — used in tests.
const (
	refreshSurfaceCache = "cache"
	refreshSurfaceAPI   = "api"
)

// refreshPlan captures which auth surfaces auto-refresh should hit
// for this invocation. On Granola v7.4x+ the desktop cache is sealed, so
// the public REST API is the only surface; an empty plan is a legitimate
// "no auth configured" state and produces no refresh and no provenance line.
type refreshPlan struct {
	api bool
}

func (p refreshPlan) empty() bool { return !p.api }

// refreshResult is the per-surface outcome auto-refresh produces.
// Errors are captured here rather than returned upward — refresh
// must never block the user's command.
type refreshResult struct {
	surface  string
	ok       bool
	rows     int
	duration time.Duration
	err      error
}

// runAutoRefresh is the entry point the PersistentPreRunE hook calls.
// It is best-effort end-to-end: any panic, error, or unexpected state
// is captured into a refreshResult and the function returns without
// propagating an error. The caller's command always proceeds.
//
// Declared as a variable so integration tests can swap in a spy that
// records call count without invoking the real cache/API sync paths.
// Production reads runAutoRefreshImpl directly.
var runAutoRefresh = runAutoRefreshImpl

func runAutoRefreshImpl(cmd *cobra.Command, flags *rootFlags) {
	if cmd == nil || flags == nil {
		return
	}
	if shouldSkipAutoRefresh(cmd) {
		return
	}
	if autoRefreshOptedOut(flags) {
		return
	}
	// A dry run must not write the store: the client answers every request
	// with a {"dry_run":true} sentinel, which hydration would otherwise store
	// as an empty meeting (handfixes.json: dry-run-never-writes-store).
	if flags.dryRun {
		return
	}
	plan := detectRefreshPlan(flags)
	if plan.empty() {
		return
	}

	ctx, cancel := autoRefreshContext(cmd.Context(), flags.timeout)
	defer cancel()

	results := plan.run(ctx, flags)
	if shouldEmitProvenance(flags, cmd) {
		emitProvenanceLine(cmd.ErrOrStderr(), results)
	}
}

// autoRefreshContext returns a derived context with a deadline if the
// parent context has none and the user-configured timeout is positive.
// Without this, a hung refresh could pin the entire command — the
// user's command should still be able to run on stale data within a
// reasonable bound.
func autoRefreshContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	if _, hasDeadline := parent.Deadline(); hasDeadline {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

// shouldSkipAutoRefresh returns true when the command's lineage
// includes any name in noRefreshCommands. Walks via cmd.Parent()
// so deep subcommands like "auth login" or "profile save" are
// matched on the ancestor's name.
func shouldSkipAutoRefresh(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if _, skip := noRefreshCommands[c.Name()]; skip {
			return true
		}
	}
	return false
}

// autoRefreshOptedOut applies a two-tier precedence: --no-refresh=true
// (possibly set by a profile via ApplyProfileToFlags during Phase 5.3 of
// root.go's PersistentPreRunE) wins; otherwise GRANOLA_NO_AUTO_REFRESH
// decides. Refresh is on by default.
//
// PATCH(autorefresh-doc-comment-honest): a bool flag carries no
// "was-explicitly-set-to-false" signal at this call site, so the env var
// runs whenever flags.noRefresh is false regardless of whether the user
// or a profile said `--no-refresh=false` or simply left the flag at its
// default. That means a profile setting `no_refresh: false` cannot
// override a truthy GRANOLA_NO_AUTO_REFRESH — env beats the explicit
// "leave it on" profile value. Captured here so the doc matches the code.
func autoRefreshOptedOut(flags *rootFlags) bool {
	if flags.noRefresh {
		return true
	}
	if envBoolish(os.Getenv("GRANOLA_NO_AUTO_REFRESH")) {
		return true
	}
	return false
}

// envBoolish matches the convention the rest of the CLI uses for
// boolean env vars (see how the existing feedback-auto-send env is
// parsed): "1", "true", "yes" (case-insensitive) are truthy; anything
// else is falsy.
func envBoolish(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// detectRefreshPlan inspects the user's environment and config and decides
// whether auto-refresh should run the public-REST sync. On Granola v7.4x+
// the desktop cache is sealed, so the API is the only refreshable surface;
// a user with no API credential configured gets nothing.
func detectRefreshPlan(flags *rootFlags) refreshPlan {
	return refreshPlan{
		api: apiKeyConfigured(flags),
	}
}

// run executes the refresh plan, calling the public-REST sync core when the
// plan selected it. Each result captures errors locally so the caller never
// sees an error return — best-effort is enforced at the type level.
func (p refreshPlan) run(ctx context.Context, flags *rootFlags) []refreshResult {
	var out []refreshResult
	if p.api {
		res, err := runApiSync(ctx, flags)
		out = append(out, refreshResult{
			surface:  refreshSurfaceAPI,
			ok:       err == nil,
			rows:     res.TotalRows(),
			duration: res.Duration,
			err:      err,
		})
	}
	return out
}

// shouldEmitProvenance decides whether the one-line stderr summary
// fires. Suppressed under any mode that suggests the caller is a
// machine (--agent, --json, --compact, --quiet) or when stderr is
// not a TTY (CI logs, piped consumers). The TTY check is the most
// important — agents often forget to pass --agent.
func shouldEmitProvenance(flags *rootFlags, _ *cobra.Command) bool {
	if flags.agent || flags.asJSON || flags.compact || flags.quiet {
		return false
	}
	return stderrIsTerminal()
}

// stderrIsTerminal is a tiny seam so tests can probe shouldEmitProvenance
// without mucking with os.Stderr. Production checks the character-device
// bit via os.Stderr.Stat() — pipes and files do not have ModeCharDevice
// set, terminals do. Stdlib-only; avoids pulling in golang.org/x/term.
var stderrIsTerminal = func() bool {
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// emitProvenanceLine writes the one-line summary to the provided
// writer. Format:
//
//	auto-refresh: cache=ok (1.2s, 47 docs)  api=ok (820ms, 12 docs)
//
// Failures render as cache=failed: <short reason>. Surfaces not in
// the plan are omitted entirely (no "api=skipped" noise for users
// without an API key).
func emitProvenanceLine(w io.Writer, results []refreshResult) {
	if len(results) == 0 {
		return
	}
	parts := make([]string, 0, len(results))
	for _, r := range results {
		parts = append(parts, formatRefreshFragment(r))
	}
	fmt.Fprintln(w, "auto-refresh: "+strings.Join(parts, "  "))
}

// formatRefreshFragment renders a single surface's outcome.
// Externalized so the provenance line composition is trivial to test
// per-surface.
func formatRefreshFragment(r refreshResult) string {
	dur := formatRefreshDuration(r.duration)
	if r.ok {
		return fmt.Sprintf("%s=ok (%s, %d rows)", r.surface, dur, r.rows)
	}
	if r.err == nil {
		// ok=false with no err is a programming bug; show something
		// truthful rather than a misleading "ok".
		return fmt.Sprintf("%s=failed (%s)", r.surface, dur)
	}
	return fmt.Sprintf("%s=failed: %s (%s)", r.surface, shortErr(r.err), dur)
}

// formatRefreshDuration renders durations as humans read them:
// sub-second → "230ms", sub-minute → "1.2s". Avoid Go's default
// "1.234567s" which puts microsecond precision on a refresh line
// nobody will read at that resolution.
func formatRefreshDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// shortErr trims an error message to a single short line for
// inclusion in the provenance string. Keeps long upstream wrapped
// errors from blowing up the line width.
func shortErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if idx := strings.IndexAny(msg, "\n\r"); idx >= 0 {
		msg = msg[:idx]
	}
	if len(msg) > 80 {
		msg = msg[:77] + "..."
	}
	return msg
}
