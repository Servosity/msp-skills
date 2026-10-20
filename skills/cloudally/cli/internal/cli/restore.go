// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"cloudally-pp-cli/internal/cliutil"
	"github.com/spf13/cobra"
)

// restoreCLIName is the data-directory name shared with the generated store.
const restoreCLIName = "cloudally-cli"

// Scope kinds recorded on a ledger batch.
const (
	scopeDistributor = "distributor"
	scopeReseller    = "reseller"
	scopeAccount     = "account"
)

// Ledger job statuses owned by this CLI. After a successful submit the raw
// RESTORE task status from the API replaces SUBMITTED.
//
// SUBMITTING is written and committed before a request leaves; a row still
// SUBMITTING when the next run starts belonged to a run that died mid-send
// and becomes UNKNOWN. UNKNOWN means the API may or may not have accepted the
// restore (timeout, reset after send, 408, 5xx, cancel while in flight), so
// --resume does not send it again unless --retry-unknown is given.
const (
	jobPlanned        = "PLANNED"
	jobSubmitting     = "SUBMITTING"
	jobSubmitted      = "SUBMITTED"
	jobFailed         = "FAILED"
	jobFailedRedirect = "FAILED_REDIRECT"
	jobUnknown        = "UNKNOWN"
	jobUnmatched      = "UNMATCHED"
	planNoBackup      = "NO_BACKUP"
	planOK            = "OK"
)

// restoreRunningStatuses is the single provisional table that decides which
// raw task statuses still count as running. Everything else is finished.
var restoreRunningStatuses = map[string]bool{
	"PENDING":    true,
	"QUEUED":     true,
	"DISPATCHED": true,
	"INPROCESS":  true,
	"ACTIVE":     true,
}

// restoreTaskRunning reports whether a raw task status is still in flight.
func restoreTaskRunning(status string) bool {
	return restoreRunningStatuses[strings.ToUpper(strings.TrimSpace(status))]
}

// restoreTaskListType is the task type filter for the status feed.
const restoreTaskListType = "RESTORE"

// restoreTaskPageSize is the page size used when polling RESTORE tasks.
const restoreTaskPageSize = 100

// restoreTaskMaxPages bounds a runaway pagination loop.
const restoreTaskMaxPages = 200

// restoreScope selects which API path family a restore command talks to.
type restoreScope struct {
	account  string
	reseller bool
}

func (s restoreScope) kind() string {
	switch {
	case s.account == "":
		return scopeAccount
	case s.reseller:
		return scopeReseller
	default:
		return scopeDistributor
	}
}

func (s restoreScope) validate() error {
	if s.reseller && s.account == "" {
		return usageErr(errors.New("--reseller requires --account <accountId>"))
	}
	return nil
}

func (s restoreScope) basePath() string {
	switch s.kind() {
	case scopeDistributor:
		return "/v2/distributors/accounts/" + url.PathEscape(s.account)
	case scopeReseller:
		return "/v2/resellers/accounts/" + url.PathEscape(s.account)
	default:
		return "/v2"
	}
}

func (s restoreScope) taskPath(taskID string) string {
	return s.basePath() + "/backup-tasks/" + url.PathEscape(taskID)
}

func (s restoreScope) tasksPath() string { return s.basePath() + "/backup-tasks" }

func (s restoreScope) restorePath() string { return s.basePath() + "/recovery/restore" }

// scopeFromKind rebuilds the API path selector from the values stored on a
// ledger batch. An account-scope batch talks to the /v2 paths with the
// account token; the account id it records (the customer, after
// use-account) names who it restored for and is not a path selector, so it
// must not turn the scope into the distributor family.
func scopeFromKind(kind, account string) restoreScope {
	if kind == scopeAccount {
		return restoreScope{}
	}
	return restoreScope{account: account, reseller: kind == scopeReseller}
}

func addRestoreScopeFlags(cmd *cobra.Command, s *restoreScope) {
	cmd.Flags().StringVar(&s.account, "account", "", "Customer account id (partner scope). Omit when the active token is an account token")
	cmd.Flags().BoolVar(&s.reseller, "reseller", false, "Use the reseller path family (/v2/resellers/accounts/...) instead of the distributor one")
}

// parseEntityFilter merges --entities values and an --entities-file into a
// case-insensitive set. The file holds one entity per line or a CSV whose
// first column is the entity; blank lines, '#' comments, and a header row
// named entity/email/account are skipped.
func parseEntityFilter(values []string, file string) (map[string]bool, []string, error) {
	set := map[string]bool{}
	var order []string
	add := func(v string) {
		v = strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"`))
		if v == "" {
			return
		}
		key := strings.ToLower(v)
		if !set[key] {
			set[key] = true
			order = append(order, v)
		}
	}
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			add(part)
		}
	}
	if file != "" {
		f, err := os.Open(file) // #nosec G304 -- user-supplied entity list path.
		if err != nil {
			return nil, nil, usageErr(fmt.Errorf("--entities-file: %w", err))
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		first := true
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			col := line
			if i := strings.IndexByte(line, ','); i >= 0 {
				col = line[:i]
			}
			col = strings.TrimSpace(strings.Trim(col, `"`))
			if first {
				first = false
				switch strings.ToLower(col) {
				case "entity", "email", "account", "mailbox", "user":
					continue
				}
			}
			add(col)
		}
		if err := sc.Err(); err != nil {
			return nil, nil, usageErr(fmt.Errorf("--entities-file: %w", err))
		}
	}
	return set, order, nil
}

// restoreShortCircuit reports whether a command must stop before any
// network call or ledger write and only describe what it would do.
func restoreShortCircuit(flags *rootFlags) bool {
	return dryRunOK(flags) || cliutil.IsVerifyEnv()
}

// writeWould reports a skipped action. JSON callers get a structured
// envelope so --dry-run never leaves stdout empty.
func writeWould(w io.Writer, flags *rootFlags, action, would string, extra map[string]any) error {
	if flags != nil && flags.asJSON {
		out := map[string]any{"dry_run": true, "action": action, "would": would}
		for k, v := range extra {
			out[k] = v
		}
		return json.NewEncoder(w).Encode(out)
	}
	_, err := termf(w, "dry-run: would %s\n", would)
	return err
}

// termSafe is the one funnel every human-readable value from a tenant, the
// API, or the ledger passes through before it reaches a terminal.
// cliutil.ScrubTerminal turns tabs and newlines into spaces and drops the
// C0 and C1 controls (ESC, BEL, CR, DEL, U+0080-U+009F such as CSI and
// OSC), so a mailbox name cannot retitle the window, clear the screen, or
// start a fake line. The Unicode bidi embedding, override, and isolate
// controls are dropped too, since they reorder how the rest of the line
// reads.
func termSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if isBidiControl(r) {
			return -1
		}
		return r
	}, cliutil.ScrubTerminal(s))
}

// isBidiControl reports U+202A-U+202E (LRE, RLE, PDF, LRO, RLO) and
// U+2066-U+2069 (LRI, RLI, FSI, PDI).
func isBidiControl(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

// termf is fmt.Fprintf for human-readable restore output. Every string,
// []string, error, and fmt.Stringer argument goes through termSafe; the
// format string itself is trusted. JSON output never goes through here.
func termf(w io.Writer, format string, args ...any) (int, error) {
	for i, a := range args {
		switch v := a.(type) {
		case string:
			args[i] = termSafe(v)
		case []string:
			safe := make([]string, len(v))
			for k := range v {
				safe[k] = termSafe(v[k])
			}
			args[i] = safe
		case error:
			args[i] = termSafe(v.Error())
		case fmt.Stringer:
			args[i] = termSafe(v.String())
		}
	}
	return fmt.Fprintf(w, format, args...)
}

// printRestoreTable renders a fixed-column table. Every header and cell
// goes through termSafe, which also keeps a tab or newline inside a value
// from breaking the columns.
func printRestoreTable(w io.Writer, headers []string, rows [][]string) error {
	tw := newTabWriter(w)
	upper := make([]string, len(headers))
	for i, h := range headers {
		upper[i] = termSafe(strings.ToUpper(h))
	}
	fmt.Fprintln(tw, strings.Join(upper, "\t"))
	for _, r := range rows {
		cells := make([]string, len(r))
		for i, c := range r {
			cells[i] = termSafe(c)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

// formatAge renders a duration as days and hours, or hours and minutes.
func formatAge(d time.Duration) string {
	neg := d < 0
	if neg {
		d = -d
	}
	var s string
	switch {
	case d >= 24*time.Hour:
		days := int(d / (24 * time.Hour))
		hours := int((d % (24 * time.Hour)) / time.Hour)
		s = fmt.Sprintf("%dd%dh", days, hours)
	case d >= time.Hour:
		s = fmt.Sprintf("%dh%dm", int(d/time.Hour), int((d%time.Hour)/time.Minute))
	case d >= time.Minute:
		s = fmt.Sprintf("%dm", int(d/time.Minute))
	default:
		s = fmt.Sprintf("%ds", int(d/time.Second))
	}
	if neg {
		return "-" + s
	}
	return s
}

func isoUTC(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func newRestoreCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Plan, submit, watch, and review bulk restores",
		Long: strings.Trim(`
Bulk restore workflow for one backup task: 'plan' previews which backup each
mailbox would be restored from, 'submit' sends the restores and records them
in a local ledger, 'watch' follows a batch to completion, and 'history' reads
the ledger offline.
`, "\n"),
		Example: strings.Trim(`
  cloudally-cli restore plan --task 64b0e9 --date 2026-10-01
  cloudally-cli restore submit --task 64b0e9 --date 2026-10-01 --yes
  cloudally-cli restore watch --batch 20261015-120000-ab12
  cloudally-cli restore history --since 2026-09-01
  # partner login (auth login --partner): name the customer account
  cloudally-cli restore plan --account 5f1c2a --task 64b0e9 --date 2026-10-01
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:typed-exit-codes": "0,2"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	cmd.AddCommand(newRestorePlanCmd(flags))
	cmd.AddCommand(newRestoreSubmitCmd(flags))
	cmd.AddCommand(newRestoreWatchCmd(flags))
	cmd.AddCommand(newRestoreHistoryCmd(flags))
	// Errors carry API response bodies and tenant names to the root's
	// "Error:" line; scrubRunE (download_text.go) makes them safe to print
	// and keeps errors.As and the exit code.
	for _, sub := range cmd.Commands() {
		if sub.RunE != nil {
			sub.RunE = scrubRunE(sub.RunE)
		}
	}
	return cmd
}

// newNovelRestoreCmd keeps the generated root wiring compiling; the root
// dedupes it against the command added by newRestoreCmd.
func newNovelRestoreCmd(flags *rootFlags) *cobra.Command { return newRestoreCmd(flags) }
