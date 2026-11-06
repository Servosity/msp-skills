// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel-feature support: shared helpers for the cross-fleet
// (transcendence) commands. These commands answer questions the agent-scoped
// Domotz Public API cannot answer in a single call by reading the local
// SQLite mirror (store-backed commands) or by fanning out across every agent
// (live commands).

package cli

import (
	"context"
	"domotz-pp-cli/internal/client"
	"domotz-pp-cli/internal/cliutil"
	"domotz-pp-cli/internal/store"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// addFleetDBFlag registers the shared --db flag used by every store-backed
// fleet command so they all resolve the same local mirror.
func addFleetDBFlag(cmd *cobra.Command, dbPath *string) {
	cmd.Flags().StringVar(dbPath, "db", "", "Local store path (default: ~/.local/share/domotz-cli/data.db)")
}

// openFleetStore opens the local SQLite mirror at dbPath, or the default path
// when empty. A non-existent file is created with the schema, so commands stay
// safe (and return empty results) before the first sync.
func openFleetStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("domotz-cli")
	}
	return store.OpenWithContext(ctx, dbPath)
}

// fleetDeviceSource is the Collector device list `sync` writes, shaped like
// the typed device table the novels were written against (id, agent_id, data,
// synced_at). domotz handfix device-list-synced: GET /agent/{agent_id}/device
// is the per-agent sync dependent agent_device (spec x-pp-sync-walker), stored
// as generic resources rows with the parent agent id injected; no sync writes
// devices to the typed "device" table (4.24 filled it with application and
// variable rows). Only devices of an agent the store still has count: a
// Collector removed from the account takes its devices out of every report.
const fleetDeviceSource = `(SELECT
  CAST(json_extract(data, '$.id') AS TEXT) AS id,
  CAST(COALESCE(json_extract(data, '$.agent_id'), json_extract(data, '$.parent_id')) AS TEXT) AS agent_id,
  data,
  synced_at
FROM resources
WHERE resource_type = 'agent_device'
  AND CAST(COALESCE(json_extract(data, '$.agent_id'), json_extract(data, '$.parent_id')) AS TEXT) IN (SELECT id FROM "agent"))`

// fleetField is one field a store-backed novel prints: the documented JSON
// paths it reads, in order (the first that is set wins), or, with sep, every
// path's value joined by sep (null when any is missing).
type fleetField struct {
	paths []string
	sep   string
}

// fleetDeviceFields and fleetAgentFields are the only fields the store-backed
// novels read from a stored device (the items of GET
// /agent/{agent_id}/device: LocalIpDevice, ExternalIpDevice, SubnetIpDevice,
// DummyDevice) or agent (the items of GET /agent: AgentResponseModel). Codex
// r1 (domotz 4.34.2) class wrong-field-read: fleet inventory, breakdown and
// new read vendor and model only from user_data (DeviceUserDataModel, the
// user's custom override), never the detected vendor and model the device
// carries, so a Cisco C9300 Domotz detected read as an unknown vendor; fleet
// agents read location.name, which AgentLocation (latitude, longitude) never
// has, and printed the AgentVersion object as the version. Every path is
// pinned against spec.json by TestFleetFieldsAreDocumented, and no novel file
// but this one may hold a JSON path of its own.
var fleetDeviceFields = map[string]fleetField{
	"display_name":          {paths: []string{"$.display_name"}},
	"type":                  {paths: []string{"$.type.label"}},
	"vendor":                {paths: []string{"$.user_data.vendor", "$.vendor"}},
	"model":                 {paths: []string{"$.user_data.model", "$.model"}},
	"os":                    {paths: []string{"$.os.name"}},
	"serial":                {paths: []string{"$.details.serial"}},
	"importance":            {paths: []string{"$.importance"}},
	"status":                {paths: []string{"$.status"}},
	"first_seen_on":         {paths: []string{"$.first_seen_on"}},
	"last_status_change":    {paths: []string{"$.last_status_change"}},
	"authentication_status": {paths: []string{"$.authentication_status"}},
	"snmp_status":           {paths: []string{"$.snmp_status"}},
}

var fleetAgentFields = map[string]fleetField{
	"status":       {paths: []string{"$.status.value"}},
	"status_since": {paths: []string{"$.status.last_change"}},
	"version":      {paths: []string{"$.version.agent"}},
	"location":     {paths: []string{"$.location.latitude", "$.location.longitude"}, sep: ","},
	"organization": {paths: []string{"$.organization.name"}},
}

// deviceField is the SQL expression that reads a device field of the row
// aliased alias ("" for an unaliased data column).
func deviceField(alias, name string) string { return fleetFieldSQL(fleetDeviceFields, alias, name) }

// agentField is the SQL expression that reads an agent field of the row
// aliased alias ("" for an unaliased data column).
func agentField(alias, name string) string { return fleetFieldSQL(fleetAgentFields, alias, name) }

func fleetFieldSQL(fields map[string]fleetField, alias, name string) string {
	f, ok := fields[name]
	if !ok || len(f.paths) == 0 {
		panic("fleet field " + name + " is not declared in fleet_shared.go")
	}
	col := "data"
	if alias != "" {
		col = alias + ".data"
	}
	if len(f.paths) == 1 {
		return fmt.Sprintf("json_extract(%s, '%s')", col, f.paths[0])
	}
	parts := make([]string, len(f.paths))
	for i, p := range f.paths {
		parts[i] = fmt.Sprintf("NULLIF(json_extract(%s, '%s'), '')", col, p)
	}
	if f.sep != "" {
		return "(" + strings.Join(parts, " || '"+f.sep+"' || ") + ")"
	}
	return "COALESCE(" + strings.Join(parts, ", ") + ")"
}

// parseDomotzTime is the one parser for every Domotz timestamp a novel
// compares or orders (documented as ISO 8601 with a timezone offset,
// 2025-11-17T09:10:58+00:00; RFC3339 with or without fractional seconds, and
// the bare forms read as UTC). Codex r1 (domotz 4.34.2) class
// time-compared-as-text: fleet speedtest and fleet new compared the strings,
// so 12:00+02:00 (10:00Z) read as later than 11:00Z. ok is false when s does
// not parse.
func parseDomotzTime(s string) (t time.Time, ok bool) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z0700", "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// queryFleetRows runs a read-only SELECT and returns the rows as column->value
// maps in result order. Always returns a non-nil slice so empty results
// marshal to [] rather than null.
func queryFleetRows(ctx context.Context, db *store.Store, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			m[c] = normalizeFleetValue(vals[i])
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// normalizeFleetValue renders SQLite scalar values JSON-friendly: []byte (the
// driver's representation of TEXT/JSON columns) becomes a string.
func normalizeFleetValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// asString coerces a scanned SQLite value to a string.
func asString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// asInt coerces a scanned SQLite aggregate value (which may be int64, float64,
// or NULL) to an int.
func asInt(v any) int {
	switch t := v.(type) {
	case nil:
		return 0
	case int64:
		return int(t)
	case float64:
		return int(t)
	case int:
		return t
	case []byte:
		n, _ := strconv.Atoi(strings.TrimSpace(string(t)))
		return n
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

// parseSinceCutoff turns a window like "24h", "90m", or "7d" into an absolute
// cutoff time relative to now. time.ParseDuration handles h/m/s; the "d"
// (days) suffix is handled here because Go durations have no day unit.
func parseSinceCutoff(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return now.Add(-24 * time.Hour), nil
	}
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil || days < 0 {
			return time.Time{}, fmt.Errorf("invalid --since %q: use forms like 24h, 90m, or 7d", s)
		}
		return now.AddDate(0, 0, -days), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return time.Time{}, fmt.Errorf("invalid --since %q: use forms like 24h, 90m, or 7d", s)
	}
	return now.Add(-d), nil
}

// fleetAgentRef is a lightweight agent identity used by the live fan-out
// commands (events, ip-conflicts, speedtest, triggers).
type fleetAgentRef struct {
	ID   string `json:"agent_id"`
	Name string `json:"site"`
}

// fleetAgentsLive fetches the agent list directly from the API so live
// fan-out commands work without a prior sync. IDs are normalized to bare
// strings (the API encodes them as JSON numbers).
//
// domotz handfix live-fanout-walks-every-collector: GET /agent pages with
// page_size (default 10) and page_number (default 0), so one unpaged read
// returned the first 10 Collectors and every live novel answered for those
// alone. The list is read page by page until a page is empty or brings no
// Collector not already seen (an API that ignored page_number would repeat
// page 0: that is the whole list, never an endless walk).
func fleetAgentsLive(ctx context.Context, c *client.Client) ([]fleetAgentRef, error) {
	const pageSize = 100
	refs := make([]fleetAgentRef, 0)
	seen := map[string]bool{}
	for page := 0; ; page++ {
		data, err := c.Get(ctx, "/agent", map[string]string{"page_size": strconv.Itoa(pageSize), "page_number": strconv.Itoa(page)})
		if err != nil {
			return nil, err
		}
		var raw []map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parsing agent list: %w", err)
		}
		added := 0
		for _, a := range raw {
			ref := fleetAgentRef{ID: trimJSONString(a["id"])}
			if dn, ok := a["display_name"]; ok {
				ref.Name = trimJSONString(dn)
			}
			if ref.ID != "" && !seen[ref.ID] {
				seen[ref.ID] = true
				refs = append(refs, ref)
				added++
			}
		}
		if added == 0 {
			return refs, nil
		}
	}
}

// fanoutIncomplete fails a live fan-out command that could not read every
// Collector (domotz handfix live-fanout-partial-is-not-whole). The rows it
// did read are already printed and each failure is already on stderr; an
// exit status of 0 told a script or an agent that the sites it never read
// had nothing to report.
func fanoutIncomplete(errs []cliutil.FanoutError, total int) error {
	if len(errs) == 0 {
		return nil
	}
	names := make([]string, 0, len(errs))
	for _, e := range errs {
		names = append(names, e.Source)
	}
	return apiErr(fmt.Errorf("%d of %d Collectors could not be read (%s); the results above leave them out", len(errs), total, strings.Join(names, ", ")))
}

// trimJSONString renders a raw JSON scalar (number or quoted string) as a
// bare Go string: `12345` -> "12345", `"abc"` -> "abc".
func trimJSONString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		var unq string
		if err := json.Unmarshal(raw, &unq); err == nil {
			return unq
		}
		return strings.Trim(s, "\"")
	}
	return s
}

// site reports the human site label for a fan-out agent ref: the display
// name when present, else the raw id.
func (a fleetAgentRef) site() string {
	if a.Name != "" {
		return a.Name
	}
	return a.ID
}
