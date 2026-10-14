// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sophos-central-pp-cli/internal/cliutil"
)

// triageRow is one item in the cross-tenant work queue.
type triageRow struct {
	TenantID    string    `json:"tenant_id"`
	TenantName  string    `json:"tenant_name"`
	Kind        string    `json:"kind"` // alert | unhealthy | tamper_off | isolated | stale
	Severity    string    `json:"severity"`
	Subject     string    `json:"subject"`
	ID          string    `json:"id"`
	At          time.Time `json:"at,omitzero"`
	AgeHours    int       `json:"age_hours"`
	New         bool      `json:"new"`
	NextCommand string    `json:"next_command"`
}

type triageView struct {
	Items        []triageRow   `json:"items"`
	Total        int           `json:"total"`
	BlindTenants []blindTenant `json:"blind_tenants"`
	SyncedAt     *time.Time    `json:"synced_at"`
	Note         string        `json:"note,omitempty"`
}

var severityRank = map[string]int{"high": 0, "medium": 1, "low": 2}

func newNovelFleetTriageCmd(flags *rootFlags) *cobra.Command {
	var flagSeverity, flagStale, dbPath string
	var flagLimit int

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "One ranked work queue across all customers: open high and medium alerts, bad or suspicious endpoints",
		Long: `Use this command for today's device- and alert-level work queue across all tenants (open high/medium alerts, bad or suspicious endpoints, tamper protection off, isolated or stale devices). Do NOT use this command to rank tenants by Account Health Check score; use 'fleet health' instead. Do NOT use it to list posture or upsell gaps per tenant; use 'fleet gaps' instead.

Reads the local store filled by 'fleet refresh'. Severity: alerts keep their own; health bad is high, suspicious is medium; tamper protection off and isolated devices are medium; stale devices (not seen within --stale) are low. Rows sort by severity, then oldest first. "new" marks alerts raised since the previous fleet refresh. blind_tenants lists tenants the last sync could not read.`,
		Example: strings.Trim(`
  sophos-central-cli fleet triage --severity high,medium --stale 14d --agent
  sophos-central-cli fleet triage --severity high --limit 20
  sophos-central-cli fleet triage --tenant 6f3c4a2e-1b9d-4e8f-a7c2-5d0e9b1f3a64 --severity high,medium,low`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet triage")
			}
			severities := map[string]bool{}
			for _, s := range strings.Split(flagSeverity, ",") {
				if s = strings.TrimSpace(strings.ToLower(s)); s != "" {
					if _, ok := severityRank[s]; !ok {
						return usageErr(fmt.Errorf("--severity: %q is not high, medium or low", s))
					}
					severities[s] = true
				}
			}
			if len(severities) == 0 {
				return usageErr(fmt.Errorf("--severity: name at least one of high, medium, low"))
			}
			stale, err := cliutil.ParseDurationLoose(flagStale)
			if err != nil || stale <= 0 {
				return usageErr(fmt.Errorf("--stale %q: want a duration like 14d", flagStale))
			}
			if flagLimit <= 0 {
				return usageErr(fmt.Errorf("--limit must be positive"))
			}
			path := fleetDBPath(dbPath)
			db, exists, err := openFleetStore(cmd.Context(), path)
			if err != nil {
				return err
			}
			if !exists {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local mirror at %s\nrun: sophos-central-cli fleet refresh --db %s\n", path, path)
				if wantsHumanTable(cmd.OutOrStdout(), flags) {
					fmt.Fprintln(cmd.OutOrStdout(), "No fleet refresh has run yet. Run: sophos-central-cli fleet refresh")
					return nil
				}
				return printJSONFiltered(cmd.OutOrStdout(), triageView{Items: make([]triageRow, 0), BlindTenants: make([]blindTenant, 0), Note: fleetNeverRefreshedNote}, flags)
			}
			defer db.Close()
			filter, err := fleetTenantFilter()
			if err != nil {
				return err
			}
			view, err := buildTriage(db, severities, stale, cmd.Flags().Changed("stale"), time.Now().UTC(), filter)
			if err != nil {
				return err
			}
			var freshNote string
			view.SyncedAt, freshNote = fleetFreshness(cmd.ErrOrStderr(), loadFleetMeta(db), flags.maxAge)
			if view.Note == "" {
				view.Note = freshNote
			}
			if len(view.Items) > flagLimit {
				view.Items = view.Items[:flagLimit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Items) == 0 {
				if view.SyncedAt == nil {
					fmt.Fprintln(cmd.OutOrStdout(), "No fleet refresh has run yet. Run: sophos-central-cli fleet refresh")
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Nothing in the triage queue (fleet data from %s).\n", view.SyncedAt.Format(time.RFC3339))
				}
			} else {
				rows := make([]map[string]any, 0, len(view.Items))
				for _, r := range view.Items {
					rows = append(rows, map[string]any{"severity": r.Severity, "tenant": r.TenantName, "kind": r.Kind, "subject": r.Subject, "age_h": r.AgeHours, "next": r.NextCommand})
				}
				if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
					return err
				}
			}
			warnBlind(cmd.ErrOrStderr(), len(view.BlindTenants))
			return nil
		},
	}
	cmd.Flags().StringVar(&flagSeverity, "severity", "high,medium", "Comma-separated severities to include: high,medium,low")
	cmd.Flags().StringVar(&flagStale, "stale", "14d", "Flag endpoints not seen for this long (e.g. 7d, 14d); passing --stale lists stale devices whatever --severity says")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "Maximum rows to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	return cmd
}

// fleetReader is the subset of the store the fleet reads use, so the logic is
// testable against an in-memory fake.
type fleetReader interface {
	ListScan(resourceType string, fn func(id string, data json.RawMessage) bool) error
	Get(resourceType, id string) (json.RawMessage, error)
}

// buildTriage assembles the queue. Stale rows are severity low; staleExplicit
// (--stale was passed) includes them whatever --severity says, because asking
// for a stale window is asking to see stale devices.
func buildTriage(db fleetReader, severities map[string]bool, stale time.Duration, staleExplicit bool, now time.Time, filter map[string]bool) (triageView, error) {
	ledger, err := loadFleetLedger(db)
	if err != nil {
		return triageView{}, fmt.Errorf("reading fleet ledger: %w", err)
	}
	meta := loadFleetMeta(db)
	names := tenantNames(db, ledger)
	unreadable := unreadableRows{}
	rows := make([]triageRow, 0)
	add := func(r triageRow) {
		if filter != nil && !filter[r.TenantID] {
			return
		}
		if !severities[r.Severity] && !(staleExplicit && r.Kind == "stale") {
			return
		}
		if r.TenantName == "" {
			r.TenantName = names[r.TenantID]
		}
		if !r.At.IsZero() {
			r.AgeHours = int(now.Sub(r.At).Hours())
		}
		rows = append(rows, r)
	}

	err = db.ListScan("alerts", func(_ string, data json.RawMessage) bool {
		var a map[string]any
		if json.Unmarshal(data, &a) != nil {
			unreadable.add(data, "alerts")
			return true
		}
		at, _ := time.Parse(time.RFC3339, jsonString(a, "raisedAt"))
		subject := jsonString(a, "description")
		if host := jsonString(a, "managedAgent", "name"); host != "" {
			subject = host + ": " + subject
		}
		tenantID := jsonString(a, "tenant", "id")
		add(triageRow{
			TenantID: tenantID, TenantName: jsonString(a, "tenant", "name"), Kind: "alert",
			Severity: strings.ToLower(jsonString(a, "severity")), Subject: subject, ID: jsonString(a, "id"), At: at,
			New:         !meta.PrevStartedAt.IsZero() && at.After(meta.PrevStartedAt),
			NextCommand: fmt.Sprintf("sophos-central-cli alerts get %s --tenant %s", jsonString(a, "id"), tenantID),
		})
		return true
	})
	if err != nil {
		return triageView{}, fmt.Errorf("reading alerts: %w", err)
	}

	err = db.ListScan("endpoints", func(_ string, data json.RawMessage) bool {
		var e map[string]any
		if json.Unmarshal(data, &e) != nil {
			unreadable.add(data, "endpoints")
			return true
		}
		id, host, tenantID := jsonString(e, "id"), jsonString(e, "hostname"), jsonString(e, "tenant", "id")
		lastSeen, _ := time.Parse(time.RFC3339, jsonString(e, "lastSeenAt"))
		contain := fmt.Sprintf("sophos-central-cli contain %s --tenant %s --scan", id, tenantID)
		switch jsonString(e, "health", "overall") {
		case "bad":
			add(triageRow{TenantID: tenantID, Kind: "unhealthy", Severity: "high", Subject: host + ": health bad", ID: id, At: lastSeen, NextCommand: contain})
		case "suspicious":
			add(triageRow{TenantID: tenantID, Kind: "unhealthy", Severity: "medium", Subject: host + ": health suspicious", ID: id, At: lastSeen, NextCommand: contain})
		}
		if supported, _ := jsonPath(e, "tamperProtectionSupported").(bool); supported {
			if enabled, ok := jsonPath(e, "tamperProtectionEnabled").(bool); ok && !enabled {
				add(triageRow{TenantID: tenantID, Kind: "tamper_off", Severity: "medium", Subject: host + ": tamper protection off", ID: id, At: lastSeen,
					NextCommand: "sophos-central-cli fleet tamper --tenant " + tenantID})
			}
		}
		if jsonString(e, "isolation", "status") == "isolated" {
			add(triageRow{TenantID: tenantID, Kind: "isolated", Severity: "medium", Subject: host + ": isolated", ID: id, At: lastSeen,
				NextCommand: fmt.Sprintf("sophos-central-cli contain %s --tenant %s --release", id, tenantID)})
		}
		if !lastSeen.IsZero() && now.Sub(lastSeen) > stale {
			add(triageRow{TenantID: tenantID, Kind: "stale", Severity: "low", Subject: fmt.Sprintf("%s: not seen for %dd", host, int(now.Sub(lastSeen).Hours()/24)), ID: id, At: lastSeen,
				NextCommand: fmt.Sprintf("sophos-central-cli endpoints get %s --tenant %s", id, tenantID)})
		}
		return true
	})
	if err != nil {
		return triageView{}, fmt.Errorf("reading endpoints: %w", err)
	}

	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := severityRank[rows[i].Severity], severityRank[rows[j].Severity]
		if ri != rj {
			return ri < rj
		}
		return rows[i].AgeHours > rows[j].AgeHours
	})
	view := triageView{Items: rows, Total: len(rows), BlindTenants: append(blindTenantsFrom(ledger, filter), unreadable.blind(names, filter)...)}
	if len(rows) == 0 && len(ledger) == 0 {
		view.Note = "no fleet refresh has run yet; run 'sophos-central-cli fleet refresh' to fill the store"
	}
	return view, nil
}
