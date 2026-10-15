// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/store"
)

type bypassExposure struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Path     string `json:"path"` // user-status | group | code-no-expiry | code-unlimited-reuse | code-old
	Detail   string `json:"detail"`
	IssuedBy string `json:"issued_by,omitempty"`
	Created  string `json:"created,omitempty"`
}

// localRows returns the stored JSON rows for one resource type, drained
// before returning so callers may run follow-up queries.
func localRows(ctx context.Context, db *store.Store, resourceType string) ([]json.RawMessage, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", resourceType, err)
	}
	out := make([]json.RawMessage, 0)
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan %s: %w", resourceType, err)
		}
		out = append(out, json.RawMessage(data))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	return out, rows.Close()
}

// auditView is the output of the local audits. complete=false (with the
// resources in unsynced) means an empty result is NOT a clean bill of health.
type auditView[T any] struct {
	Complete bool     `json:"complete"`
	Unsynced []string `json:"unsynced,omitempty"`
	// UndecodedRows counts stored rows that could not be parsed.
	UndecodedRows int    `json:"undecoded_rows,omitempty"`
	Rows          []T    `json:"rows"`
	Note          string `json:"note,omitempty"`
}

// syncGaps reports which resources were never synced, or whose last sync
// stopped part-way, and warns (once, with a 'hint:' prefix that MCP relays)
// about missing, partial or stale local data.
func syncGaps(cmd *cobra.Command, db *store.Store, maxAge time.Duration, resources ...string) []string {
	missing := make([]string, 0)
	partial := make([]string, 0)
	stale := make([]string, 0)
	for _, r := range resources {
		st, err := readSyncHintState(db, r)
		switch {
		case err != nil, !st.hasState: // unreadable sync state counts as never synced
			missing = append(missing, r)
		case lastSyncInterrupted(db, r):
			partial = append(partial, r)
		case maxAge > 0 && time.Since(st.lastSynced) > maxAge:
			stale = append(stale, r)
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s never synced; results are incomplete. Run '%s'.\n",
			strings.Join(missing, ", "), syncHint(missing))
	}
	if len(partial) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: the last sync of %s stopped part-way; results are incomplete. Run '%s'.\n",
			strings.Join(partial, ", "), syncHint(partial))
		missing = append(missing, partial...)
	}
	if len(stale) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s synced more than --max-age=%s ago. Run '%s' to refresh.\n",
			strings.Join(stale, ", "), maxAge, syncHint(stale))
	}
	return missing
}

// lastSyncInterrupted reports whether the most recent sync of resource did
// not finish (sync_state.last_attempt_complete = 0, written while a sync is
// paging and cleared only when it completes): rows on pages it never fetched
// may be missing, so an empty audit would be a false all-clear. An unreadable
// flag counts as interrupted.
func lastSyncInterrupted(db *store.Store, resource string) bool {
	var done int64
	err := db.DB().QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, resource).Scan(&done)
	return err != nil || done == 0
}

// syncHint is the sync command a 'hint:' line suggests. It carries the pinned
// --account-id, so following it fills the same subaccount store the audit
// reads instead of the parent account's.
func syncHint(resources []string) string {
	s := "cisco-duo-cli sync --resources " + strings.Join(resources, ",")
	if id := currentAccountID(); id != "" {
		s += " --account-id " + shellQuote(id)
	}
	return s
}

// noMirrorView is the result when the local store does not exist at all.
func noMirrorView[T any](resources []string) auditView[T] {
	return auditView[T]{Complete: false, Unsynced: resources, Rows: make([]T, 0), Note: "no local mirror; sync first"}
}

// newAuditView marks the result incomplete when data was never synced or
// could not be read, so an empty result is never mistaken for a clean tenant.
func newAuditView[T any](rows []T, unsynced []string, undecoded int) auditView[T] {
	v := auditView[T]{Complete: len(unsynced) == 0 && undecoded == 0, Unsynced: unsynced, UndecodedRows: undecoded, Rows: rows}
	if !v.Complete {
		v.Note = "some data was never synced, was only partly synced, or could not be read; an empty result is not a clean tenant"
	}
	return v
}

// loadAudit reads the stored rows of each resource an audit needs. ok is
// false (after a 'hint:' line) when no local store exists yet; unsynced lists
// resources that were never synced.
func loadAudit(cmd *cobra.Command, flags *rootFlags, dbPath string, resources ...string) (rows map[string][]json.RawMessage, unsynced []string, ok bool, err error) {
	if dbPath == "" {
		dbPath = defaultDBPath("cisco-duo-cli")
	}
	if _, statErr := os.Stat(dbPath); os.IsNotExist(statErr) {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s; results are incomplete. Run '%s'.\n",
			dbPath, syncHint(resources))
		return nil, resources, false, nil
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, nil, false, fmt.Errorf("opening database: %w", err)
	}
	defer db.Close()
	unsynced = syncGaps(cmd, db, flags.maxAge, resources...)
	rows = make(map[string][]json.RawMessage, len(resources))
	for _, r := range resources {
		if rows[r], err = localRows(ctx, db, r); err != nil {
			return nil, nil, false, err
		}
	}
	return rows, unsynced, true, nil
}

// warnUndecoded tells a human reader that stored rows were skipped.
func warnUndecoded(cmd *cobra.Command, n int) {
	if n > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d stored row(s) could not be read; results are incomplete. Re-run sync.\n", n)
	}
}

// classifyBypass lists every path by which a user can skip MFA.
func classifyBypass(users []duoUser, codes []duoBypassCode, olderThan time.Duration, now time.Time) []bypassExposure {
	out := make([]bypassExposure, 0)
	for _, u := range users {
		if strings.EqualFold(u.Status, "bypass") {
			out = append(out, bypassExposure{UserID: u.UserID, Username: u.Username, Path: "user-status",
				Detail: "user status is bypass (skips MFA after primary auth)"})
		}
		for _, g := range u.Groups {
			if strings.EqualFold(g.Status, "bypass") {
				out = append(out, bypassExposure{UserID: u.UserID, Username: u.Username, Path: "group",
					Detail: "member of Bypass group " + g.Name + " (" + g.GroupID + ")"})
			}
		}
	}
	for _, c := range codes {
		if c.expired(now) {
			continue
		}
		e := bypassExposure{IssuedBy: c.AdminEmail, Created: unixTime(c.Created)}
		if c.User != nil {
			e.UserID, e.Username = c.User.UserID, c.User.Username
		}
		switch {
		case c.Expiration == nil:
			e.Path, e.Detail = "code-no-expiry", "bypass code "+c.BypassCodeID+" never expires"
		case c.ReuseCount == nil:
			e.Path, e.Detail = "code-unlimited-reuse", "bypass code "+c.BypassCodeID+" has unlimited uses"
		case olderThan > 0 && c.Created > 0 && now.Sub(time.Unix(c.Created, 0)) > olderThan:
			e.Path, e.Detail = "code-old", "bypass code "+c.BypassCodeID+" is older than "+olderThan.String()+" and still valid"
		default:
			continue
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Username != out[j].Username {
			return out[i].Username < out[j].Username
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func newNovelAuditBypassCmd(flags *rootFlags) *cobra.Command {
	var olderThan, dbPath string
	cmd := &cobra.Command{
		Use:   "bypass",
		Short: "List every synced user who can skip MFA: bypass status, Bypass group, standing codes",
		Long: strings.Trim(`
Use this command to list every user in the synced tenant who can skip MFA
(bypass status, Bypass group or standing bypass code). It reads the local
store: users (with their groups) and fleet bypass codes. A standing code
never expires or has unlimited reuse; --older-than also flags valid codes
older than that age. Each row names the admin who issued the code.

Do NOT use it for inactive, unenrolled or locked accounts; use 'audit stale'
instead. Do NOT use it for fleet-wide counts; use 'fleet risk' instead.

Run 'cisco-duo-cli sync --resources users,bypass_codes' first (add
--account-id to audit one subaccount). JSON output is {complete, unsynced,
rows}: complete=false means a resource was never synced or its last sync
stopped part-way, so an empty rows list is not a clean tenant.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli audit bypass --agent
  cisco-duo-cli audit bypass --older-than 30d --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit bypass")
			}
			age, err := duoParseSince(olderThan, 0)
			if err != nil {
				return err
			}
			data, unsynced, ok, err := loadAudit(cmd, flags, dbPath, "users", "bypass_codes")
			if err != nil {
				return err
			}
			if !ok {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), noMirrorView[bypassExposure](unsynced), flags)
				}
				return nil
			}
			users, badUsers := decodeAllCount[duoUser](data["users"])
			codes, badCodes := decodeAllCount[duoBypassCode](data["bypass_codes"])
			view := newAuditView(classifyBypass(users, codes, age, time.Now()), unsynced, badUsers+badCodes)
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			warnUndecoded(cmd, view.UndecodedRows)
			switch {
			case len(view.Rows) == 0 && !view.Complete:
				fmt.Fprintln(w, "No MFA-skip paths found, but the local data is incomplete (see the hint above).")
			case len(view.Rows) == 0:
				fmt.Fprintf(w, "No MFA-skip paths found in %d synced users and %d bypass codes.\n", len(data["users"]), len(data["bypass_codes"]))
			}
			for _, r := range view.Rows {
				fmt.Fprintf(w, "%-24s %-22s %s %s\n", r.Username, r.Path, r.Detail, r.IssuedBy)
			}
			return nil
		},
	}
	useAccountFlag(cmd)
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Also flag still-valid bypass codes older than this age, e.g. 30d")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local database path (default: the CLI's synced store)")
	return cmd
}
