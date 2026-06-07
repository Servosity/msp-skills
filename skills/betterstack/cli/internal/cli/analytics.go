// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built transcendence layer for betterstack-cli. Not generated.
//
// Better Stack Uptime is a JSON:API service: every synced resource is stored in
// the generic `resources` table as the full element {id, type, attributes:{…}}.
// These loaders read that blob via json_extract on `$.attributes.*`, CASTing
// every value to TEXT so JSON booleans/integers/strings all scan into one Go
// string column without per-type scan errors. The novel analytics commands
// (fleet, coverage, mttr, flapping, oncall-gaps, heartbeat-risk) all read from
// here — none of them depends on the generator's domain-table columns, which
// are NULL for JSON:API specs because the generator looks up top-level keys.
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"betterstack-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// openAnalyticsStore opens the local SQLite mirror for read-only analytics and
// returns a friendly "run sync first" hint when the store can't be opened.
func openAnalyticsStore(cmd *cobra.Command, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("betterstack-cli")
	}
	s, err := store.OpenWithContext(cmd.Context(), dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening local store: %w (run `betterstack-cli sync` first)", err)
	}
	return s, nil
}

func truthy(s string) bool { return s == "1" || s == "true" || s == "TRUE" }

func atoiSafe(s string) int {
	if s == "" {
		return 0
	}
	// JSON numbers may arrive as "30" or "30.0".
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}

// parseTime tolerates the timestamp shapes Better Stack emits (RFC3339 with or
// without fractional seconds, "Z" or offset). Returns ok=false on empty/unparseable.
func parseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02 15:04:05 -0700", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// queryTextRows runs a SELECT whose columns are all TEXT and returns each row as
// a []string. Keeps the loaders below uniform and free of per-type scan logic.
func queryTextRows(ctx context.Context, s *store.Store, query string, ncols int, args ...any) ([][]string, error) {
	rows, err := s.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		cols := make([]sql.NullString, ncols)
		ptrs := make([]any, ncols)
		for i := range cols {
			ptrs[i] = &cols[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		rec := make([]string, ncols)
		for i, c := range cols {
			if c.Valid {
				rec[i] = c.String
			}
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// jx builds COALESCE(CAST(json_extract(data,'$.<path>') AS TEXT),”) for a column.
func jx(path string) string {
	return "COALESCE(CAST(json_extract(data,'$." + path + "') AS TEXT),'')"
}

// ---- typed rows ----

type monitorRow struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Type     string `json:"monitor_type"`
	Status   string `json:"status"`
	Paused   bool   `json:"paused"`
	PolicyID string `json:"policy_id"`
	GroupID  string `json:"monitor_group_id"`
	Email    bool   `json:"email"`
	SMS      bool   `json:"sms"`
	Call     bool   `json:"call"`
	Push     bool   `json:"push"`
}

func loadMonitors(ctx context.Context, s *store.Store) ([]monitorRow, error) {
	q := fmt.Sprintf(`SELECT %s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s FROM resources WHERE resource_type='monitors'`,
		jx("id"), jx("attributes.pronounceable_name"), jx("attributes.url"), jx("attributes.monitor_type"),
		jx("attributes.status"), jx("attributes.paused"), jx("attributes.policy_id"), jx("attributes.monitor_group_id"),
		jx("attributes.email"), jx("attributes.sms"), jx("attributes.call"), jx("attributes.push"))
	recs, err := queryTextRows(ctx, s, q, 12)
	if err != nil {
		return nil, err
	}
	out := make([]monitorRow, 0, len(recs))
	for _, r := range recs {
		out = append(out, monitorRow{
			ID: r[0], Name: r[1], URL: r[2], Type: r[3], Status: r[4],
			Paused: truthy(r[5]), PolicyID: r[6], GroupID: r[7],
			Email: truthy(r[8]), SMS: truthy(r[9]), Call: truthy(r[10]), Push: truthy(r[11]),
		})
	}
	return out, nil
}

type heartbeatRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Period int    `json:"period"`
	Grace  int    `json:"grace"`
	Paused bool   `json:"paused"`
}

func loadHeartbeats(ctx context.Context, s *store.Store) ([]heartbeatRow, error) {
	q := fmt.Sprintf(`SELECT %s,%s,%s,%s,%s,%s FROM resources WHERE resource_type='heartbeats'`,
		jx("id"), jx("attributes.name"), jx("attributes.status"), jx("attributes.period"), jx("attributes.grace"), jx("attributes.paused"))
	recs, err := queryTextRows(ctx, s, q, 6)
	if err != nil {
		return nil, err
	}
	out := make([]heartbeatRow, 0, len(recs))
	for _, r := range recs {
		out = append(out, heartbeatRow{
			ID: r[0], Name: r[1], Status: r[2],
			Period: atoiSafe(r[3]), Grace: atoiSafe(r[4]), Paused: truthy(r[5]),
		})
	}
	return out, nil
}

type incidentRow struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	Status         string `json:"status"`
	StartedAt      string `json:"started_at"`
	AcknowledgedAt string `json:"acknowledged_at"`
	ResolvedAt     string `json:"resolved_at"`
	Source         string `json:"source"`
}

func loadIncidents(ctx context.Context, s *store.Store) ([]incidentRow, error) {
	// Source is the monitored resource: prefer the JSON:API relationship id,
	// then any monitor_id attribute, then the monitored url/name.
	source := "COALESCE(" +
		"NULLIF(CAST(json_extract(data,'$.relationships.monitor.data.id') AS TEXT),'')," +
		"NULLIF(CAST(json_extract(data,'$.attributes.monitor_id') AS TEXT),'')," +
		"NULLIF(CAST(json_extract(data,'$.attributes.url') AS TEXT),'')," +
		"NULLIF(CAST(json_extract(data,'$.attributes.name') AS TEXT),'')," +
		"'unknown')"
	q := fmt.Sprintf(`SELECT %s,%s,%s,%s,%s,%s,%s,%s FROM resources WHERE resource_type='incidents'`,
		jx("id"), jx("attributes.name"), jx("attributes.url"), jx("attributes.status"),
		jx("attributes.started_at"), jx("attributes.acknowledged_at"), jx("attributes.resolved_at"), source)
	recs, err := queryTextRows(ctx, s, q, 8)
	if err != nil {
		return nil, err
	}
	out := make([]incidentRow, 0, len(recs))
	for _, r := range recs {
		out = append(out, incidentRow{
			ID: r[0], Name: r[1], URL: r[2], Status: r[3],
			StartedAt: r[4], AcknowledgedAt: r[5], ResolvedAt: r[6], Source: r[7],
		})
	}
	return out, nil
}

type onCallRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	IsDefault   bool   `json:"default_calendar"`
	OnCallUsers int    `json:"on_call_user_count"`
}

func loadOnCalls(ctx context.Context, s *store.Store) ([]onCallRow, error) {
	// on_call_users is an array under attributes; count it, falling back to 0.
	userCount := "COALESCE(json_array_length(json_extract(data,'$.attributes.on_call_users')),0)"
	q := fmt.Sprintf(`SELECT %s,%s,%s,%s FROM resources WHERE resource_type='on-calls'`,
		jx("id"), jx("attributes.name"), jx("attributes.default_calendar"), userCount)
	recs, err := queryTextRows(ctx, s, q, 4)
	if err != nil {
		return nil, err
	}
	out := make([]onCallRow, 0, len(recs))
	for _, r := range recs {
		out = append(out, onCallRow{
			ID: r[0], Name: r[1], IsDefault: truthy(r[2]), OnCallUsers: atoiSafe(r[3]),
		})
	}
	return out, nil
}

// monitorDown reports whether a monitor's status counts as "not up". Better Stack
// monitor statuses include: up, down, validating, paused, pending, maintenance.
func monitorDown(status string) bool {
	switch strings.ToLower(status) {
	case "down", "validating", "pending":
		return true
	default:
		return false
	}
}

// hasAlertChannel reports whether a monitor will notify anyone directly.
func (m monitorRow) hasAlertChannel() bool {
	return m.Email || m.SMS || m.Call || m.Push
}
