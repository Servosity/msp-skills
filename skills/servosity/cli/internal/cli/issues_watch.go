// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): `issues watch` and `events list`. Wired by
// the init hook below. Pinned by this connector's handfixes.json under the
// "issues-watch-events" entry.
//
// Servosity has no webhooks yet, so `issues watch` polls the partner issue
// list and records each change as a row in vendor_events, the same table and
// event shape the zammad connector's webhook listener fills. `events list`
// (CLI and the read-only MCP tool events_list) reads it by cursor. When
// Servosity ships signed issue webhooks, the receiver verifies a delivery,
// re-reads the issue through the API and hands it to recordIssueObservations,
// the one function that turns an observed issue into rows, so webhook and
// poll rows share one event_id scheme and deduplicate.

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"servosity-msp-pp-cli/internal/client"
	"servosity-msp-pp-cli/internal/cliutil"
	"servosity-msp-pp-cli/internal/store"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, c := range root.Commands() {
			if c.Name() == "issues" {
				addNovelCommandIfAbsent(c, newIssuesWatchCmd(flags))
			}
		}
		addNovelCommandIfAbsent(root, newEventsCmd(flags))
	})
}

// The event catalog: the names MCP Events (SEP-3415) would list.
const (
	issueEventCreated     = "servosity.issue.created"
	issueEventUpdated     = "servosity.issue.updated"
	issueEventArchived    = "servosity.issue.archived"
	issueEventIgnored     = "servosity.issue.ignored"
	issueEventReactivated = "servosity.issue.reactivated"
)

var issueEventTypes = []string{issueEventCreated, issueEventUpdated, issueEventArchived, issueEventIgnored, issueEventReactivated}

// issueWatchSource keys the poll's watermark; no watermark means the next
// poll records a baseline instead of events.
const issueWatchSource = "poll:issues"

// The values the Servosity Issue model can hold (servosity-api
// servosity_base/models.py: Issue.States, SeverityLevels, Audiences). Any
// other value is stored as "".
var (
	issueStates     = []string{"ACTIVE", "IGNORED", "ARCHIVED"}
	issueSeverities = []string{"INFO", "SUCCESS", "WARNING", "DANGER"}
	issueAudiences  = []string{"STAFF", "OPS", "SUPPORT", "DRAAS", "BILLING", "RESELLER"}
)

// watchIssue is everything the watcher keeps about an issue: ids and triage
// fields. The description, names and comment bodies are never read.
type watchIssue struct {
	ID           string `json:"issue_id"`
	State        string `json:"state"`
	Severity     string `json:"severity,omitempty"`
	Priority     int64  `json:"priority,omitempty"`
	Audience     string `json:"audience,omitempty"`
	Company      int64  `json:"company,omitempty"`
	Backup       int64  `json:"backup,omitempty"`
	DRBackup     int64  `json:"drbackup,omitempty"`
	ResticBackup int64  `json:"resticbackup,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	LastSeenAt   string `json:"last_seen_at,omitempty"`
}

// watchRecord is a stored issue: the issue plus when the watcher last read it.
type watchRecord struct {
	watchIssue
	CheckedAt string `json:"checked_at,omitempty"`
}

func oneOf(v string, allowed []string) string {
	if slices.Contains(allowed, v) {
		return v
	}
	return ""
}

// issueTime is v re-written from its parsed time, or "" when it is not a
// timestamp, so no response text survives in a time field.
func issueTime(v string) string {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// parseWatchIssue reads one API issue into the allowlisted record: the id as
// sent when it is an issue id (a UUID or integer; anything else is refused, so
// no response text reaches a request path), integers, the model's enum values
// and re-written timestamps. Nothing else from the response is kept.
func parseWatchIssue(raw json.RawMessage) (watchIssue, bool) {
	var r struct {
		ID           any    `json:"id"`
		State        string `json:"state"`
		Severity     string `json:"severity"`
		Priority     any    `json:"priority"`
		Audience     string `json:"audience"`
		Company      any    `json:"company"`
		Backup       any    `json:"backup"`
		DRBackup     any    `json:"drbackup"`
		ResticBackup any    `json:"resticbackup"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
		LastSeenAt   string `json:"last_seen_at"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return watchIssue{}, false
	}
	id := fmt.Sprint(r.ID)
	if f, ok := r.ID.(float64); ok {
		id = strconv.FormatFloat(f, 'f', -1, 64)
	}
	if !triageIssueIDPattern.MatchString(id) {
		return watchIssue{}, false
	}
	return watchIssue{
		ID: id, State: oneOf(r.State, issueStates), Severity: oneOf(r.Severity, issueSeverities), Priority: coerceID(r.Priority),
		Audience: oneOf(r.Audience, issueAudiences), Company: coerceID(r.Company), Backup: coerceID(r.Backup), DRBackup: coerceID(r.DRBackup),
		ResticBackup: coerceID(r.ResticBackup), CreatedAt: issueTime(r.CreatedAt), UpdatedAt: issueTime(r.UpdatedAt), LastSeenAt: issueTime(r.LastSeenAt),
	}, true
}

// issueOlder reports whether a is an older read of the issue than b. The
// model bumps updated_at on every save but a recurrence saves only
// last_seen_at, so the pair orders reads; an unparsed time orders nothing.
func issueOlder(a, b watchIssue) bool {
	for _, f := range [][2]string{{a.UpdatedAt, b.UpdatedAt}, {a.LastSeenAt, b.LastSeenAt}} {
		ta, errA := time.Parse(time.RFC3339Nano, f[0])
		tb, errB := time.Parse(time.RFC3339Nano, f[1])
		if errA != nil || errB != nil {
			return false
		}
		if !ta.Equal(tb) {
			return ta.Before(tb)
		}
	}
	return false
}

// issueTransition names the event a change from prev to cur is, or "" when
// nothing the watcher compares changed, and lists the changed fields.
func issueTransition(prev, cur watchIssue) (string, []string) {
	var changed []string
	for _, f := range []struct {
		name string
		a, b any
	}{
		{"state", prev.State, cur.State}, {"severity", prev.Severity, cur.Severity}, {"priority", prev.Priority, cur.Priority},
		{"updated_at", prev.UpdatedAt, cur.UpdatedAt}, {"last_seen_at", prev.LastSeenAt, cur.LastSeenAt},
	} {
		if f.a != f.b {
			changed = append(changed, f.name)
		}
	}
	if len(changed) == 0 {
		return "", nil
	}
	switch {
	case prev.State == cur.State:
		return issueEventUpdated, changed
	case cur.State == "ARCHIVED":
		return issueEventArchived, changed
	case cur.State == "IGNORED":
		return issueEventIgnored, changed
	case cur.State == "ACTIVE" && (prev.State == "ARCHIVED" || prev.State == "IGNORED"):
		return issueEventReactivated, changed
	}
	return issueEventUpdated, changed
}

// issueEventRow builds the vendor_events row for one change. event_id is
// the upstream identity of the change (the type, the issue and its compared
// fields; the model bumps updated_at or last_seen_at on every save), so any
// source that sees the same change writes the same id.
func issueEventRow(kind string, prev, cur watchIssue, changed []string, source string, now time.Time) store.VendorEvent {
	at := now.UTC().Format(time.RFC3339)
	eventID := kind + ":" + cur.ID
	switch {
	case kind == issueEventCreated:
		if cur.CreatedAt != "" {
			at = cur.CreatedAt
		}
	case slices.Contains(changed, "updated_at") && cur.UpdatedAt != "":
		at = cur.UpdatedAt
	case slices.Contains(changed, "last_seen_at") && cur.LastSeenAt != "":
		at = cur.LastSeenAt
	}
	if kind != issueEventCreated {
		sum := sha256.Sum256([]byte(strings.Join([]string{cur.State, cur.Severity, strconv.FormatInt(cur.Priority, 10), cur.UpdatedAt, cur.LastSeenAt}, "\x00")))
		eventID += ":" + hex.EncodeToString(sum[:8])
	}
	data, _ := json.Marshal(struct {
		watchIssue
		PreviousState string   `json:"previous_state,omitempty"`
		Changed       []string `json:"changed,omitempty"`
	}{cur, prev.State, changed})
	return store.VendorEvent{EventID: eventID, Type: kind, Source: source, OccurredAt: at, ReceivedAt: now.UTC().Format(time.RFC3339),
		ResourceType: "issue", ResourceID: cur.ID, Data: data}
}

// recordIssueObservations is the one place issue events enter vendor_events.
// snap is the issue_watch_state the caller read before calling the API;
// observed are issues as the API returned them; forget are ids the API no
// longer shows us; sent are the credentials the caller's client sends
// (client.SentCredentials). `issues watch` calls it with source "poll". A future signed
// Servosity issue webhook receiver calls it with source "webhook" after it
// verifies the signature, reads the store and then re-reads the issue through
// the API (never trusting the payload), so a webhook row and a poll row for
// the same change are one row.
//
// Nothing applies when another watcher committed since snap was read (its
// generation moved), and every keep and forget is also a compare-and-swap on
// the record in snap: a skipped write and its events count as Stale and the
// next poll decides. A read older
// than the recorded one is dropped. An issue whose kept fields repeat a held
// credential is withheld by the credential kit's SQL driver and not recorded at
// all (Withheld counts it). The first poll (no watermark) records a
// baseline: it stores every issue and writes no events, and only from a
// complete read of the list.
func recordIssueObservations(ctx context.Context, db *store.Store, source string, snap store.IssueWatchSnapshot, observed []watchIssue, forget []string, complete bool, now time.Time) (issueWatchResult, error) {
	based := snap.Records
	var res issueWatchResult
	checked := now.UTC().Format(time.RFC3339)
	applied, err := db.ApplyIssueWatch(ctx, issueWatchSource, source == "poll", checked, snap.Gen,
		func(watched bool) ([]store.IssueWrite, error) {
			res.Baseline = source == "poll" && !watched
			if res.Baseline && !complete {
				return nil, errors.New("the first poll could not read every page of the issue list, so no baseline was recorded; run it again")
			}
			var writes []store.IssueWrite
			for _, cur := range observed {
				var prev watchRecord
				prevJSON, known := based[cur.ID]
				if known && (json.Unmarshal([]byte(prevJSON), &prev) != nil || issueOlder(cur, prev.watchIssue)) {
					continue
				}
				raw, _ := json.Marshal(watchRecord{cur, checked})
				w := store.IssueWrite{ID: cur.ID, Expect: prevJSON, Data: string(raw)}
				switch {
				case res.Baseline:
				case !known:
					w.Events = []store.VendorEvent{issueEventRow(issueEventCreated, watchIssue{}, cur, nil, source, now)}
				default:
					if kind, changed := issueTransition(prev.watchIssue, cur); kind != "" {
						w.Events = []store.VendorEvent{issueEventRow(kind, prev.watchIssue, cur, changed, source, now)}
					}
				}
				writes = append(writes, w)
			}
			for _, id := range forget {
				if prevJSON, known := based[id]; known {
					writes = append(writes, store.IssueWrite{ID: id, Expect: prevJSON})
				}
			}
			return writes, nil
		})
	res.Events, res.Stale, res.Withheld, res.Cursor = applied.Added, applied.Stale, applied.Withheld, applied.Cursor
	return res, err
}

type issueWatchResult struct {
	Baseline bool  `json:"baseline"`
	Issues   int   `json:"issues"`
	Events   int   `json:"events"`
	Cursor   int64 `json:"cursor,string"`
	Complete bool  `json:"complete"`
	Withheld int   `json:"withheld,omitempty"`
	Stale    int   `json:"stale,omitempty"`
}

// issueWatchMaxPages bounds one list read. ponytail: 1000 pages of 50 (the
// API's page size); a larger book reads as incomplete and skips gone-issue
// checks.
const issueWatchMaxPages = 1000

// issueWatchMaxReread caps the per-issue reads one poll makes. They go
// oldest-read first, so none starves. ponytail: past 50 the rest wait for
// the next poll.
const issueWatchMaxReread = 50

// issueWatchIgnoredRecheck is how often an ignored issue is re-read: it is
// not in the list, and can still be archived (servosity-api prune_issues,
// archive_by_hash) or recur. Archived issues are final (archive clears the
// hash that reactivate needs), so they are never re-read.
const issueWatchIgnoredRecheck = time.Hour

// issueWatchNow is the poll clock; tests move it.
var issueWatchNow = time.Now

// pollIssues reads every page of the partner issue list, re-reads active
// issues that left it and ignored issues not read for an hour to learn their
// state, and records the result. Partner tokens are refused /issues/,
// /issues/archived/ and /issues/ignored/ (admin-only, PR #178;
// servosity-api api_v1/views/issues.py IsAdminUser), so the list is the
// reseller's (active issues only) and every other state comes from
// GET /issues/{id}/.
func pollIssues(ctx context.Context, c *client.Client, db *store.Store, listPath string) (issueWatchResult, error) {
	now := issueWatchNow()
	// Read before the API: every write this poll makes is conditional on it.
	prevRaw, err := db.IssueWatchState(ctx)
	if err != nil {
		return issueWatchResult{}, err
	}
	var observed []watchIssue
	listed := map[string]bool{}
	complete := false
	params := map[string]string{}
	seenCursor := map[string]bool{}
	for page := 0; page < issueWatchMaxPages; page++ {
		data, err := c.GetNoCache(ctx, listPath, params)
		if err != nil {
			return issueWatchResult{}, err
		}
		for _, item := range unwrapList(data) {
			if w, ok := parseWatchIssue(item); ok && !listed[w.ID] {
				listed[w.ID] = true
				observed = append(observed, w)
			}
		}
		var env struct {
			Next string `json:"next"`
		}
		_ = json.Unmarshal(data, &env)
		if env.Next == "" {
			complete = true
			break
		}
		u, err := url.Parse(env.Next)
		next := ""
		if err == nil {
			next = u.Query().Get("cursor")
		}
		if next == "" || seenCursor[next] {
			break
		}
		seenCursor[next] = true
		params = map[string]string{"cursor": next}
	}
	stale := now.Add(-issueWatchIgnoredRecheck).UTC().Format(time.RFC3339)
	var reread []watchRecord
	for id, raw := range prevRaw.Records {
		var p watchRecord
		if listed[id] || json.Unmarshal([]byte(raw), &p) != nil || p.ID != id || !triageIssueIDPattern.MatchString(id) {
			continue
		}
		if (complete && p.State == "ACTIVE") || (p.State == "IGNORED" && p.CheckedAt < stale) {
			reread = append(reread, p)
		}
	}
	sort.Slice(reread, func(i, j int) bool {
		if reread[i].CheckedAt != reread[j].CheckedAt {
			return reread[i].CheckedAt < reread[j].CheckedAt
		}
		return reread[i].ID < reread[j].ID
	})
	var forget []string
	var readErr error
	for i, p := range reread {
		if i == issueWatchMaxReread {
			break
		}
		data, err := c.GetNoCache(ctx, "/issues/"+p.ID+"/", nil)
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusForbidden) {
			// Gone, or no longer ours (servosity-api IssueViewSet.retrieve
			// answers 403 for an issue outside the token's companies while the
			// list above just accepted the token).
			forget = append(forget, p.ID)
			continue
		}
		if err != nil {
			readErr = err
			break
		}
		if w, ok := parseWatchIssue(data); ok && w.ID == p.ID {
			observed = append(observed, w)
		}
	}
	res, err := recordIssueObservations(ctx, db, "poll", prevRaw, observed, forget, complete, now)
	if err != nil {
		return issueWatchResult{}, err
	}
	res.Issues, res.Complete = len(listed), complete
	return res, readErr
}

// issueWatchBackoff is the wait after failures consecutive failed polls:
// the interval doubled per failure, never more than an hour (or the
// interval, if that is longer).
func issueWatchBackoff(interval time.Duration, failures int) time.Duration {
	limit := max(interval, time.Hour)
	d := interval
	for i := 0; i < failures && d < limit; i++ {
		d *= 2
	}
	return min(d, limit)
}

// issueWatchAuthFailure reports an answer that retrying cannot fix.
func issueWatchAuthFailure(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden)
}

// runIssueWatchLoop polls until ctx ends or the credential is refused,
// waiting interval between polls and backing off after failures. The
// client's own limiter and Retry-After handling pace each request. Every
// status line passes the credential kit at stderr.
func runIssueWatchLoop(ctx context.Context, interval time.Duration, errOut io.Writer, poll func(context.Context) (issueWatchResult, error), wait func(context.Context, time.Duration) bool) error {
	failures := 0
	for {
		res, err := poll(ctx)
		next := interval
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil && issueWatchAuthFailure(err):
			return err
		case err != nil:
			failures++
			next = issueWatchBackoff(interval, failures)
			fmt.Fprintf(errOut, "issues watch: poll failed (%s); retrying in %s\n", err.Error(), next)
		default:
			failures = 0
			line := fmt.Sprintf("issues watch: %d issues, %d new events, cursor %d", res.Issues, res.Events, res.Cursor)
			if res.Baseline {
				line = fmt.Sprintf("issues watch: baseline recorded for %d issues (no events)", res.Issues)
			}
			if res.Withheld > 0 {
				line += fmt.Sprintf("; %d issues not recorded because a field repeated a credential", res.Withheld)
			}
			if res.Stale > 0 {
				line += fmt.Sprintf("; %d issues changed under another watcher, left for the next poll", res.Stale)
			}
			fmt.Fprintln(errOut, line)
		}
		if !wait(ctx, next) {
			return nil
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func newIssuesWatchCmd(flags *rootFlags) *cobra.Command {
	var interval time.Duration
	var once bool
	var dbPath string
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Poll the partner issue list and record each change as an event (read-only on Servosity)",
		Long: `Servosity has no issue webhooks yet, so this polls your reseller's issue list
(every page) and records each change in the local event table that
'events list' reads: servosity.issue.created, .updated (state, severity,
priority, updated_at or last_seen_at changed), .archived, .ignored and
.reactivated. The list holds active issues only, so an active issue that
leaves it is re-read to learn whether it was archived or ignored, and an
ignored issue is re-read at most hourly to catch it being archived or
changed.

The first run records a baseline and no events. Each event carries ids and
triage fields only, never the description or comments. Only GET requests
are sent. A failed poll waits twice as long each time (at most an hour); a
refused credential stops the watch. Ctrl+C stops it.`,
		Example: `  # Poll every 5 minutes until Ctrl+C
  servosity-cli issues watch

  # One poll, for cron
  servosity-cli issues watch --once --json`,
		// pp:data-source live
		Annotations: map[string]string{"mcp:hidden": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if interval < time.Minute {
				return usageErr(fmt.Errorf("--interval must be at least 1m, got %s", interval))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "issues watch")
			}
			if cliutil.IsDogfoodEnv() {
				once = true
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			c.NoCache = true
			if dbPath == "" {
				dbPath = defaultDBPath("servosity-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			resellerID, err := resolveResellerID(cmd.Context(), c)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("resolving reseller ID: %w", err), flags)
			}
			listPath := fmt.Sprintf("/resellers/%d/issues/", resellerID)
			poll := func(ctx context.Context) (issueWatchResult, error) {
				return pollIssues(ctx, c, db, listPath)
			}
			if once {
				res, err := poll(cmd.Context())
				if err != nil {
					return classifyAPIError(cmd.OutOrStdout(), err, flags)
				}
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					fmt.Fprintf(cmd.OutOrStdout(), "%d issues, %d new events, cursor %d (baseline %t, complete %t)\n", res.Issues, res.Events, res.Cursor, res.Baseline, res.Complete)
					return nil
				}
				return flags.printJSON(cmd, res)
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			if err := runIssueWatchLoop(ctx, interval, cmd.ErrOrStderr(), poll, sleepCtx); err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Minute, "Time between polls (at least 1m)")
	cmd.Flags().BoolVar(&once, "once", false, "Poll once and exit")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

func newEventsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "events",
		Short:       "Read the Servosity issue events 'issues watch' recorded",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	var since int64
	var eventType, dbPath string
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List recorded Servosity issue events after a cursor, oldest first",
		Long: `Reads the local event table that 'issues watch' fills (no Servosity call).
Pass the returned cursor as --since next time to get only newer events.
Each event is {eventId, name, timestamp, data, cursor, source}; data holds
issue ids and triage fields, never the description or comments: run
'issues read <id>' for the rest.`,
		Example: `  servosity-cli events list --since 0
  servosity-cli events list --since 41 --type servosity.issue.created --json`,
		// pp:data-source local
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			if eventType != "" && !slices.Contains(issueEventTypes, eventType) {
				return usageErr(fmt.Errorf("unknown --type %q; one of %s", eventType, strings.Join(issueEventTypes, ", ")))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "events list")
			}
			if limit < 1 || limit > 1000 {
				limit = 100
			}
			if dbPath == "" {
				dbPath = defaultDBPath("servosity-cli")
			}
			db, err := store.OpenReadOnlyContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("%w\nNo events recorded yet: run 'servosity-cli issues watch'.", err)
			}
			defer db.Close()
			rows, more, err := db.VendorEventsSince(cmd.Context(), since, eventType, limit)
			if err != nil {
				return err
			}
			events := make([]map[string]any, 0, len(rows))
			cursor := since
			for _, e := range rows {
				cursor = e.Seq
				events = append(events, map[string]any{"eventId": e.EventID, "name": e.Type, "timestamp": e.OccurredAt,
					"data": e.Data, "cursor": strconv.FormatInt(e.Seq, 10), "source": e.Source})
			}
			return flags.printJSON(cmd, map[string]any{"events": events, "cursor": strconv.FormatInt(cursor, 10), "hasMore": more})
		},
	}
	list.Flags().Int64Var(&since, "since", 0, "Cursor: return events after this one (0 for all)")
	list.Flags().StringVar(&eventType, "type", "", "Only this event: "+strings.Join(issueEventTypes, ", "))
	list.Flags().IntVar(&limit, "limit", 100, "Maximum events (1 to 1000)")
	list.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	cmd.AddCommand(list)
	return cmd
}
