// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

// auditCategories and auditEventTypes are the documented tables at
// https://api.vsax.net/#get-all-audit-log-entries ("Supported Categories",
// "Supported Event Types"). Unknown IDs render as "category <n>", never dropped.
var auditCategories = map[int]string{
	1: "User Command", 2: "System Command", 3: "Device or Application Registration", 4: "Account",
	5: "Server Administration", 6: "Remote Control", 7: "Package Import", 8: "Datto BCDR Integrations",
	9: "Workflow Action", 10: "Virtualization Integrations", 11: "Api Tokens", 12: "Managed Files",
	13: "Datto Networking Integrations", 14: "Bulk Action", 15: "KaseyaOne", 16: "Psa Integrations",
	17: "Datto Endpoint Backup Integrations", 18: "BitDefender Integrations", 19: "Datto EDR/AV Integrations",
	20: "Automation Hub", 21: "MDM Commands", 22: "MDM Policies", 23: "MDM Enrollment", 24: "Device",
	25: "Webroot Integrations", 26: "File Transfer", 27: "Acronis",
}

var auditEventTypes = map[int]string{1: "Information", 2: "Warning", 3: "Error"}

// Categories that change access or reach into endpoints; listed individually in the digest.
var auditSensitiveCategories = map[int]bool{4: true, 5: true, 6: true, 11: true, 14: true, 26: true}

type auditEntry struct {
	RecordID    int64  `json:"AuditLogRecordId"`
	DateTime    string `json:"AuditLogRecordDateTime"`
	User        string `json:"User"`
	CategoryID  int    `json:"CategoryId"`
	EventTypeID int    `json:"EventTypeId"`
	DeviceName  string `json:"DeviceName"`
	Description string `json:"EventDescription"`
	Source      string `json:"RequestSource"`
}

type auditDigestRow struct {
	RecordID  int64  `json:"record_id"`
	Time      string `json:"time"`
	User      string `json:"user"`
	Category  string `json:"category"`
	EventType string `json:"event_type"`
	Device    string `json:"device,omitempty"`
	Event     string `json:"event"` // EventDescription; named "event" so --agent compaction keeps it
}

type auditUserRow struct {
	User       string         `json:"user"`
	Total      int            `json:"total"`
	Errors     int            `json:"errors"`
	Warnings   int            `json:"warnings"`
	ByCategory map[string]int `json:"by_category"`
}

type auditDigest struct {
	Since            string           `json:"since"`
	Until            string           `json:"until"`
	Entries          int              `json:"entries"`
	Users            []auditUserRow   `json:"users"`
	ByCategory       map[string]int   `json:"by_category"`
	Sensitive        []auditDigestRow `json:"sensitive"`
	ErrorsAndWarning []auditDigestRow `json:"errors_and_warnings"`
	Unparsed         int              `json:"unparsed_timestamps"`
	SyncedRows       int              `json:"synced_rows"`
	Note             string           `json:"note,omitempty"`
}

// formatCounts renders a category->count map as "Api Tokens=1, Remote Control=3" in name order.
func formatCounts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

func auditCategoryName(id int) string {
	if n, ok := auditCategories[id]; ok {
		return n
	}
	return "category " + strconv.Itoa(id)
}

func auditEventTypeName(id int) string {
	if n, ok := auditEventTypes[id]; ok {
		return n
	}
	return "event type " + strconv.Itoa(id)
}

func newNovelAuditLogsDigestCmd(flags *rootFlags) *cobra.Command {
	var flagSince, flagUser, flagCategory, dbPath string
	var limit int

	cmd := &cobra.Command{
		Use:   "digest",
		Short: "See who did what in the tenant over a time window, with audit categories and event types decoded to names.",
		Long: `Summarizes the locally synced audit log over a time window: a user x category matrix with the
documented category and event-type IDs decoded to names, every entry in a sensitive category
(Account, Server Administration, Remote Control, Api Tokens, Bulk Action, File Transfer), and every
Warning or Error event. Run 'kaseya-vsa-cli sync --resources audit_logs' first.

Use this command for a who-did-what rollup of tenant audit activity over a time window. Do NOT use
this command for one device's recent activity; use 'devices brief' instead.`,
		Example: strings.Trim(`
  kaseya-vsa-cli audit-logs digest --since 7d --agent
  kaseya-vsa-cli audit-logs digest --since 30d --category "Remote Control"
  kaseya-vsa-cli audit-logs digest --since 24h --user admin --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit-logs digest")
			}
			window, err := cliutil.ParseDurationLoose(flagSince)
			if err != nil || window <= 0 {
				return usageErr(fmt.Errorf("invalid --since %q (use e.g. 24h, 7d, 4w)", flagSince))
			}
			categoryID := 0
			if flagCategory != "" {
				if categoryID, err = parseAuditCategory(flagCategory); err != nil {
					return usageErr(err)
				}
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			now := time.Now().UTC()
			db, err := openFleetStore(cmd, flags, dbPath, "audit_logs")
			if err != nil {
				return err
			}
			var rows []fleetRow
			if db != nil {
				defer db.Close()
				if rows, err = loadFleetRows(ctx, db, "audit_logs"); err != nil {
					return err
				}
			}
			entries := make([]auditEntry, 0, len(rows))
			for _, r := range rows {
				var e auditEntry
				if err := json.Unmarshal(r.Data, &e); err != nil {
					return fmt.Errorf("decode audit row %s: %w", r.ID, err)
				}
				entries = append(entries, e)
			}
			d := buildAuditDigest(entries, now.Add(-window), now, flagUser, categoryID, limit)
			d.SyncedRows = len(entries)
			if d.SyncedRows == 0 {
				d.Note = "no synced audit-log rows: run 'kaseya-vsa-cli sync --resources audit_logs' first; an empty digest here does not mean no activity"
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// --agent compaction would otherwise drop which device each sensitive or failed event touched.
				return printJSONFilteredKeep(cmd.OutOrStdout(), d, flags, "device", "event")
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%d audit entries between %s and %s (%d rows synced)\n", d.Entries, d.Since, d.Until, d.SyncedRows)
			if d.Note != "" {
				fmt.Fprintln(w, d.Note)
			}
			for _, u := range d.Users {
				fmt.Fprintf(w, "  %-24s total=%-5d errors=%-3d warnings=%-3d %s\n", u.User, u.Total, u.Errors, u.Warnings, formatCounts(u.ByCategory))
			}
			if len(d.Sensitive) > 0 {
				fmt.Fprintln(w, "sensitive:")
				for _, r := range d.Sensitive {
					fmt.Fprintf(w, "  %s %-16s %-20s %s\n", r.Time, r.User, r.Category, r.Event)
				}
			}
			if len(d.ErrorsAndWarning) > 0 {
				fmt.Fprintln(w, "errors and warnings:")
				for _, r := range d.ErrorsAndWarning {
					fmt.Fprintf(w, "  %s %-16s %-8s %s\n", r.Time, r.User, r.EventType, r.Event)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSince, "since", "7d", "Time window to summarize, ending now (e.g. 24h, 7d, 4w)")
	cmd.Flags().StringVar(&flagUser, "user", "", "Only entries by this user (case-insensitive exact match)")
	cmd.Flags().StringVar(&flagCategory, "category", "", "Only this audit category (name such as \"Remote Control\", or its numeric ID)")
	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum rows in each of the sensitive and errors_and_warnings lists (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database path (default: the store sync writes)")
	return cmd
}

func parseAuditCategory(s string) (int, error) {
	s = strings.TrimSpace(s)
	if id, err := strconv.Atoi(s); err == nil {
		return id, nil
	}
	for id, name := range auditCategories {
		if strings.EqualFold(name, s) {
			return id, nil
		}
	}
	return 0, fmt.Errorf("unknown --category %q (use a documented category name or its numeric ID)", s)
}

func buildAuditDigest(entries []auditEntry, since, until time.Time, user string, categoryID, limit int) auditDigest {
	d := auditDigest{Since: since.Format(time.RFC3339), Until: until.Format(time.RFC3339),
		Users: make([]auditUserRow, 0), ByCategory: map[string]int{},
		Sensitive: make([]auditDigestRow, 0), ErrorsAndWarning: make([]auditDigestRow, 0)}
	users := map[string]*auditUserRow{}
	type timedEntry struct {
		auditEntry
		at time.Time
	}
	kept := make([]timedEntry, 0, len(entries))
	for _, e := range entries {
		t, ok := parseVSATime(e.DateTime)
		if !ok {
			d.Unparsed++
			continue
		}
		if t.Before(since) || t.After(until) {
			continue
		}
		if user != "" && !strings.EqualFold(e.User, user) {
			continue
		}
		if categoryID != 0 && e.CategoryID != categoryID {
			continue
		}
		kept = append(kept, timedEntry{e, t})
	}
	// Newest first by parsed instant (string order breaks across offsets and fractional
	// seconds); record ID breaks ties deterministically.
	sort.Slice(kept, func(i, j int) bool {
		if !kept[i].at.Equal(kept[j].at) {
			return kept[i].at.After(kept[j].at)
		}
		return kept[i].RecordID > kept[j].RecordID
	})
	for _, te := range kept {
		e := te.auditEntry
		d.Entries++
		cat := auditCategoryName(e.CategoryID)
		d.ByCategory[cat]++
		u := users[e.User]
		if u == nil {
			u = &auditUserRow{User: e.User, ByCategory: map[string]int{}}
			users[e.User] = u
		}
		u.Total++
		u.ByCategory[cat]++
		row := auditDigestRow{RecordID: e.RecordID, Time: e.DateTime, User: e.User, Category: cat,
			EventType: auditEventTypeName(e.EventTypeID), Device: e.DeviceName, Event: e.Description}
		switch e.EventTypeID {
		case 3:
			u.Errors++
		case 2:
			u.Warnings++
		}
		if auditSensitiveCategories[e.CategoryID] && (limit <= 0 || len(d.Sensitive) < limit) {
			d.Sensitive = append(d.Sensitive, row)
		}
		if (e.EventTypeID == 2 || e.EventTypeID == 3) && (limit <= 0 || len(d.ErrorsAndWarning) < limit) {
			d.ErrorsAndWarning = append(d.ErrorsAndWarning, row)
		}
	}
	for _, u := range users {
		d.Users = append(d.Users, *u)
	}
	sort.Slice(d.Users, func(i, j int) bool {
		if d.Users[i].Total != d.Users[j].Total {
			return d.Users[i].Total > d.Users[j].Total
		}
		return d.Users[i].User < d.Users[j].User
	})
	return d
}
