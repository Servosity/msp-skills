package cli

// Transcendence commands: fleet-wide rollups, cross-entity correlation, and
// local-history analytics that the read-mostly, per-org Huntress API cannot
// return in a single call. All read from the local SQLite mirror populated by
// `sync`; run `sync` first or they return empty results honestly.
//
// STORE SHAPE (verified against the generated internal/store): sync writes every
// entity into the generic `resources(id, resource_type, data, synced_at,
// updated_at)` table, where `id` is the entity id and the entity's fields live
// inside the `data` JSON blob. There are NO typed per-entity columns. So every
// field is read via json_extract(data,'$.field') and every table is
// `resources WHERE resource_type = '<accounts_entity>'`. resource_type literals
// and JSON field names are taken from the store's upsert calls and the OpenAPI
// definitions respectively.
//
// Timestamps from the API are ISO8601 with 'T'/'Z', which SQLite's julianday()
// will not parse raw, so timestamp fields are wrapped via jdx() to normalize
// 'T'->' ' and strip trailing 'Z' around the json_extract.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"huntress-pp-cli/internal/store"
)

const transcendenceApp = "huntress-cli"

// resource_type literals (the values sync stores in resources.resource_type).
const (
	rtIncidents = "accounts_incident_reports"
	rtAgents    = "accounts_agents"
	rtOrgs      = "accounts_organizations"
	rtInvoices  = "accounts_invoices"
	rtPorts     = "accounts_external_ports"
	rtEscal     = "escalations"
)

// jx returns json_extract(<alias>.data,'$.<field>') for reading a blob field.
func jx(alias, field string) string {
	return "json_extract(" + alias + ".data,'$." + field + "')"
}

// jdx wraps a blob timestamp field in julianday() after normalizing the API's
// ISO8601 'T'/'Z' format so SQLite can parse it.
func jdx(alias, field string) string {
	return "julianday(replace(replace(json_extract(" + alias + ".data,'$." + field + "'),'T',' '),'Z',''))"
}

// resolveDB returns the dbPath override or the default store location.
func resolveDB(dbPath string) string {
	if dbPath == "" {
		return defaultDBPath(transcendenceApp)
	}
	return dbPath
}

// openStore opens the local store at the resolved path.
func openStore(ctx context.Context, dbPath string) (*store.Store, error) {
	path := resolveDB(dbPath)
	// Prefer a read-only handle when the schema already exists: read-only takes
	// no write lock, so parallel transcendence reads don't collide with
	// SQLITE_BUSY. Only fall back to a read-write open (which migrates) when the
	// DB is absent or not yet migrated. A short retry resolves the first-run
	// race where one command is mid-migration and another sees the half-built
	// file (read-only would otherwise hit "no such table: resources").
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := tryOpenReadOnlyMigrated(path); ok {
			return st, nil
		}
		st, err := store.OpenWithContext(ctx, path)
		if err == nil {
			return st, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return nil, fmt.Errorf("opening local store: %w (run `sync` first)", lastErr)
}

// tryOpenReadOnlyMigrated opens the store read-only only when the file exists
// AND the resources table is present (i.e. a prior read-write open finished
// migrating). Returns ok=false otherwise so the caller migrates via a
// read-write open.
func tryOpenReadOnlyMigrated(path string) (*store.Store, bool) {
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	st, err := store.OpenReadOnly(path)
	if err != nil {
		return nil, false
	}
	var one int
	if err := st.DB().QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='resources' LIMIT 1`).Scan(&one); err != nil {
		_ = st.Close()
		return nil, false
	}
	return st, true
}

// openStoreFor opens the local store for a transcendence command: rejects
// --data-source live (these commands have no live equivalent), then emits
// sync-freshness hints for each resource type the command reads.
func openStoreFor(cmd *cobra.Command, flags *rootFlags, dbPath string, resourceTypes ...string) (*store.Store, error) {
	if flags.dataSource == "live" {
		return nil, fmt.Errorf("this command reads only synced local data and has no live equivalent; use --data-source auto or local (run `sync` first)")
	}
	st, err := openStore(cmd.Context(), dbPath)
	if err != nil {
		return nil, err
	}
	for _, rt := range resourceTypes {
		if !hintIfUnsynced(cmd, st, rt) {
			hintIfStale(cmd, st, rt, flags.maxAge)
		}
	}
	return st, nil
}

// queryMaps runs a SELECT and returns each row as a map for JSON emission.
func queryMaps(ctx context.Context, db *sql.DB, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]interface{}{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func csvList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// inClause builds "<expr> IN (?,?,...)" plus args from a value slice.
func inClause(expr string, vals []string) (string, []interface{}) {
	if len(vals) == 0 {
		return "", nil
	}
	ph := make([]string, len(vals))
	args := make([]interface{}, len(vals))
	for i, v := range vals {
		ph[i] = "?"
		args[i] = v
	}
	return fmt.Sprintf("%s IN (%s)", expr, strings.Join(ph, ",")), args
}

// parseSinceDays converts "30d","12h","2w" (or a bare number = days) into days.
func parseSinceDays(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0
	}
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "h"):
		mult = 1.0 / 24.0
		s = strings.TrimSuffix(s, "h")
	case strings.HasSuffix(s, "d"):
		s = strings.TrimSuffix(s, "d")
	case strings.HasSuffix(s, "w"):
		mult = 7.0
		s = strings.TrimSuffix(s, "w")
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil || f < 0 {
		return 0
	}
	return f * mult
}

func toFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case []byte:
		var f float64
		_, err := fmt.Sscanf(string(n), "%g", &f)
		return f, err == nil
	case string:
		var f float64
		_, err := fmt.Sscanf(n, "%g", &f)
		return f, err == nil
	}
	return 0, false
}

func registerTranscendence(cmd *cobra.Command, flags *rootFlags) {
	cmd.AddCommand(newFleetIncidentsCmd(flags))
	cmd.AddCommand(newCoverageGapsCmd(flags))
	cmd.AddCommand(newBlastRadiusCmd(flags))
	cmd.AddCommand(newTriageAgeCmd(flags))
	cmd.AddCommand(newBillingReconcileCmd(flags))
	cmd.AddCommand(newDriftCmd(flags))
	cmd.AddCommand(newMttrCmd(flags))
	cmd.AddCommand(newCanaryWatchCmd(flags))
	cmd.AddCommand(newOrgScorecardCmd(flags))
	cmd.AddCommand(newStaleAgentsCmd(flags))
	cmd.AddCommand(newHandoffCmd(flags))
}

// orgJoin is the standard LEFT JOIN from an aliased incident/agent row to the
// organization name, matching json organization_id to the org's id.
func orgJoin(childAlias string) string {
	return "LEFT JOIN resources o ON o.resource_type='" + rtOrgs +
		"' AND " + jx("o", "id") + " = " + jx(childAlias, "organization_id")
}

// ---- fleet-incidents --------------------------------------------------------

// pp:data-source local
func newFleetIncidentsCmd(flags *rootFlags) *cobra.Command {
	var severity, status, indicatorType, sortBy, dbPath string
	var org, limit int
	cmd := &cobra.Command{
		Use:         "fleet-incidents",
		Short:       "Unified incident queue across every organization, age-sorted",
		Long:        "Fans incident reports out across all synced organizations, joins organization names,\nand returns one globally sorted queue — the cross-tenant view the per-org API can't return.\nDo NOT use it to enrich one incident with remediations/agent; use 'incident-detail' instead.\nDo NOT use it for the SLA-aging breakdown; use 'triage-age' instead.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli fleet-incidents --severity critical --status sent --sort age --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtIncidents, rtOrgs)
			if err != nil {
				return err
			}
			defer st.Close()

			where := []string{"i.resource_type = ?"}
			qargs := []interface{}{rtIncidents}
			if c, a := inClause(jx("i", "severity"), csvList(severity)); c != "" {
				where = append(where, c)
				qargs = append(qargs, a...)
			}
			if c, a := inClause(jx("i", "status"), csvList(status)); c != "" {
				where = append(where, c)
				qargs = append(qargs, a...)
			}
			if its := csvList(indicatorType); len(its) > 0 {
				var ors []string
				for _, it := range its {
					ors = append(ors, jx("i", "indicator_types")+" LIKE ?")
					qargs = append(qargs, "%"+it+"%")
				}
				where = append(where, "("+strings.Join(ors, " OR ")+")")
			}
			if org > 0 {
				where = append(where, jx("i", "organization_id")+" = ?")
				qargs = append(qargs, org)
			}
			order := jdx("i", "sent_at") + " ASC"
			if sortBy == "severity" {
				order = "CASE " + jx("i", "severity") + " WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'low' THEN 2 ELSE 3 END ASC, " + jdx("i", "sent_at") + " ASC"
			}
			if limit <= 0 {
				limit = 100
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS severity, %s AS status, %s AS indicator_types,
				%s AS organization_id, %s AS organization_name, %s AS agent_id, %s AS sent_at,
				CAST((julianday('now') - %s) * 24 AS INTEGER) AS hours_open,
				json_array_length(i.data,'$.remediations') AS remediations_count,
				%s AS subject, %s AS summary
			FROM resources i
			%s
			WHERE %s
			ORDER BY %s
			LIMIT %d`,
				jx("i", "id"), jx("i", "severity"), jx("i", "status"), jx("i", "indicator_types"),
				jx("i", "organization_id"), jx("o", "name"), jx("i", "agent_id"), jx("i", "sent_at"),
				jdx("i", "sent_at"), jx("i", "subject"), jx("i", "summary"),
				orgJoin("i"), strings.Join(where, " AND "), order, limit)

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&severity, "severity", "", "Filter by severity CSV (low,high,critical)")
	cmd.Flags().StringVar(&status, "status", "sent", "Filter by status CSV (sent,closed,dismissed,...); empty for all")
	cmd.Flags().StringVar(&indicatorType, "indicator-type", "", "Filter by indicator type CSV (footholds,ransomware_canaries,...)")
	cmd.Flags().IntVar(&org, "org", 0, "Filter by organization ID")
	cmd.Flags().StringVar(&sortBy, "sort", "age", "Sort order: age (oldest first) or severity")
	cmd.Flags().IntVar(&limit, "limit", 100, "Max rows to return")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- coverage-gaps ----------------------------------------------------------

// pp:data-source local
func newCoverageGapsCmd(flags *rootFlags) *cobra.Command {
	var staleDays int
	var dbPath string
	cmd := &cobra.Command{
		Use:         "coverage-gaps",
		Short:       "Per-org posture exposure: stale callbacks and unhealthy agents",
		Long:        "Rolls up agent health across every organization: total agents and how many have a\nstale callback, disabled Defender, or disabled firewall — ordered worst-first.\nDo NOT use it for the one-line fleet glance; use 'fleet-summary' instead.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli coverage-gaps --stale-days 7 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtOrgs, rtAgents)
			if err != nil {
				return err
			}
			defer st.Close()

			healthy := "('enabled','active','running','healthy','on')"
			q := fmt.Sprintf(`SELECT %s AS organization_id, %s AS organization_name,
				COUNT(a.id) AS agent_count,
				SUM(CASE WHEN %s IS NOT NULL AND %s != ''
					AND julianday('now') - %s > ? THEN 1 ELSE 0 END) AS stale_agents,
				SUM(CASE WHEN %s IS NOT NULL AND %s != ''
					AND lower(%s) NOT IN %s THEN 1 ELSE 0 END) AS defender_flagged,
				SUM(CASE WHEN %s IS NOT NULL AND %s != ''
					AND lower(%s) NOT IN %s THEN 1 ELSE 0 END) AS firewall_flagged
			FROM resources o
			LEFT JOIN resources a ON a.resource_type='%s' AND %s = %s
			WHERE o.resource_type='%s'
			GROUP BY %s, %s
			HAVING stale_agents > 0 OR defender_flagged > 0 OR firewall_flagged > 0
			ORDER BY (stale_agents + defender_flagged + firewall_flagged) DESC, agent_count DESC`,
				jx("o", "id"), jx("o", "name"),
				jx("a", "last_callback_at"), jx("a", "last_callback_at"), jdx("a", "last_callback_at"),
				jx("a", "defender_status"), jx("a", "defender_status"), jx("a", "defender_status"), healthy,
				jx("a", "firewall_status"), jx("a", "firewall_status"), jx("a", "firewall_status"), healthy,
				rtAgents, jx("a", "organization_id"), jx("o", "id"),
				rtOrgs, jx("o", "id"), jx("o", "name"))

			res, err := queryMaps(ctx, st.DB(), q, staleDays)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().IntVar(&staleDays, "stale-days", 7, "Agents with no callback in this many days count as stale")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- blast-radius -----------------------------------------------------------

// pp:data-source local
func newBlastRadiusCmd(flags *rootFlags) *cobra.Command {
	var indicator, dbPath string
	cmd := &cobra.Command{
		Use:         "blast-radius",
		Short:       "Correlate an indicator (IP, hash, host) across the whole fleet",
		Long:        "Given an indicator, searches incident reports, agents, and external ports across every\norganization for matches — the cross-entity correlation the API offers no single query for.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli blast-radius --indicator 203.0.113.7 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if indicator == "" && len(args) > 0 {
				indicator = args[0]
			}
			if indicator == "" {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtIncidents, rtAgents, rtPorts)
			if err != nil {
				return err
			}
			defer st.Close()
			like := "%" + indicator + "%"

			incQ := fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS status, %s AS indicator_types, %s AS sent_at, %s AS subject, %s AS summary
			FROM resources i %s
			WHERE i.resource_type='%s' AND (%s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ?)
			ORDER BY %s DESC LIMIT 200`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "status"), jx("i", "indicator_types"), jx("i", "sent_at"), jx("i", "subject"), jx("i", "summary"),
				orgJoin("i"), rtIncidents,
				jx("i", "summary"), jx("i", "body"), jx("i", "subject"), jx("i", "indicator_types"),
				jx("i", "sent_at"))
			incidents, err := queryMaps(ctx, st.DB(), incQ, like, like, like, like)
			if err != nil {
				return err
			}

			agQ := fmt.Sprintf(`SELECT %s AS id, %s AS hostname, %s AS organization_id, %s AS organization_name,
				%s AS platform, %s AS external_ip, %s AS ipv4_address, %s AS last_callback_at
			FROM resources a %s
			WHERE a.resource_type='%s' AND (%s LIKE ? OR %s LIKE ? OR %s LIKE ? OR %s LIKE ?)
			ORDER BY %s DESC LIMIT 200`,
				jx("a", "id"), jx("a", "hostname"), jx("a", "organization_id"), jx("o", "name"),
				jx("a", "platform"), jx("a", "external_ip"), jx("a", "ipv4_address"), jx("a", "last_callback_at"),
				orgJoin("a"), rtAgents,
				jx("a", "external_ip"), jx("a", "ipv4_address"), jx("a", "hostname"), jx("a", "mac_addresses"),
				jx("a", "last_callback_at"))
			agents, err := queryMaps(ctx, st.DB(), agQ, like, like, like, like)
			if err != nil {
				return err
			}

			portQ := fmt.Sprintf(`SELECT %s AS id, %s AS ip_address, %s AS port, %s AS protocol,
				%s AS service, %s AS risky_service, %s AS last_external_scan_at, %s AS organization_ids
			FROM resources p
			WHERE p.resource_type='%s' AND (%s LIKE ? OR %s LIKE ?)
			ORDER BY %s DESC LIMIT 200`,
				jx("p", "id"), jx("p", "ip_address"), jx("p", "port"), jx("p", "protocol"),
				jx("p", "service"), jx("p", "risky_service"), jx("p", "last_external_scan_at"), jx("p", "organization_ids"),
				rtPorts, jx("p", "ip_address"), jx("p", "service"),
				jx("p", "last_external_scan_at"))
			ports, err := queryMaps(ctx, st.DB(), portQ, like, like)
			if err != nil {
				return err
			}

			result := map[string]interface{}{
				"indicator":        indicator,
				"incident_reports": incidents,
				"agents":           agents,
				"external_ports":   ports,
				"match_count":      len(incidents) + len(agents) + len(ports),
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&indicator, "indicator", "", "Indicator to correlate: external IP, hostname, MAC, or text fragment")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- triage-age -------------------------------------------------------------

// pp:data-source local
func newTriageAgeCmd(flags *rootFlags) *cobra.Command {
	var buckets, status, dbPath string
	cmd := &cobra.Command{
		Use:         "triage-age",
		Short:       "SLA aging of open incidents, bucketed by hours-open",
		Long:        "Buckets open incidents across all organizations by how many hours they've been open and\nbreaks them out by severity, surfacing SLA breaches. The API has no aging concept.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli triage-age --buckets 4,24,72 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			thresholds := []float64{}
			for _, b := range csvList(buckets) {
				var f float64
				if _, e := fmt.Sscanf(b, "%g", &f); e == nil && f > 0 {
					thresholds = append(thresholds, f)
				}
			}
			if len(thresholds) == 0 {
				thresholds = []float64{4, 24, 72}
			}
			where := []string{"i.resource_type = ?", jx("i", "sent_at") + " IS NOT NULL", jx("i", "sent_at") + " != ''"}
			qargs := []interface{}{rtIncidents}
			if c, a := inClause(jx("i", "status"), csvList(status)); c != "" {
				where = append(where, c)
				qargs = append(qargs, a...)
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS severity,
				(julianday('now') - %s) * 24 AS hours_open
			FROM resources i WHERE %s`, jx("i", "id"), jx("i", "severity"), jdx("i", "sent_at"), strings.Join(where, " AND "))
			rows, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			type bucket struct {
				Label    string         `json:"bucket"`
				Total    int            `json:"total"`
				BySev    map[string]int `json:"by_severity"`
				Breached bool           `json:"sla_breached"`
			}
			labels := make([]string, 0, len(thresholds)+1)
			for i := range thresholds {
				if i == 0 {
					labels = append(labels, fmt.Sprintf("<%gh", thresholds[0]))
				} else {
					labels = append(labels, fmt.Sprintf("%g-%gh", thresholds[i-1], thresholds[i]))
				}
			}
			labels = append(labels, fmt.Sprintf(">%gh", thresholds[len(thresholds)-1]))
			out := make([]*bucket, len(labels))
			for i := range out {
				out[i] = &bucket{Label: labels[i], BySev: map[string]int{}}
			}
			for _, r := range rows {
				h, _ := toFloat(r["hours_open"])
				idx := len(thresholds)
				for i, t := range thresholds {
					if h < t {
						idx = i
						break
					}
				}
				out[idx].Total++
				sev, _ := r["severity"].(string)
				if sev == "" {
					sev = "unknown"
				}
				out[idx].BySev[sev]++
			}
			// SLA breach is a fact about incidents, not bucket scaffolding:
			// only flag the oldest bucket when something actually aged into it.
			out[len(out)-1].Breached = out[len(out)-1].Total > 0
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringVar(&buckets, "buckets", "4,24,72", "Hour thresholds CSV defining aging buckets")
	cmd.Flags().StringVar(&status, "status", "sent", "Incident status to age (default open=sent); empty for all")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- billing-reconcile ------------------------------------------------------

// pp:data-source local
func newBillingReconcileCmd(flags *rootFlags) *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:         "billing-reconcile",
		Short:       "Invoiced seats vs deployed agents for the account",
		Long:        "Compares total invoiced seat quantity against the count of agents actually present in\nthe local mirror, flagging the delta. Joins billing and fleet data the API never\ncorrelates. Account-wide (the API credential scopes one account).\nDo NOT use it for a reseller's per-account roll-up; use 'reseller-rollup' instead. Run `sync` first.",
		Example:     "  huntress-cli billing-reconcile --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtInvoices, rtAgents)
			if err != nil {
				return err
			}
			defer st.Close()

			q := fmt.Sprintf(`SELECT
				(SELECT COALESCE(SUM(json_extract(data,'$.quantity')),0) FROM resources WHERE resource_type='%s') AS invoiced_seats,
				(SELECT COUNT(*) FROM resources WHERE resource_type='%s') AS deployed_agents`, rtInvoices, rtAgents)
			res, err := queryMaps(ctx, st.DB(), q)
			if err != nil {
				return err
			}
			row := map[string]interface{}{"invoiced_seats": 0, "deployed_agents": 0, "delta": 0}
			if len(res) > 0 {
				inv, _ := toFloat(res[0]["invoiced_seats"])
				dep, _ := toFloat(res[0]["deployed_agents"])
				row["invoiced_seats"] = int(inv)
				row["deployed_agents"] = int(dep)
				row["delta"] = int(inv) - int(dep)
			}
			return flags.printJSON(cmd, row)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- drift (file-snapshot history) ------------------------------------------

// pp:data-source local
func newDriftCmd(flags *rootFlags) *cobra.Command {
	var entity, dbPath string
	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "Diff the current fleet against the prior drift snapshot",
		Long:        "Captures a snapshot of agents (id, org, last callback) each run and diffs it against the\nprevious one: added, removed, and went-quiet agents. Real history the point-in-time API lacks.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli drift --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if entity != "agents" {
				return fmt.Errorf("unsupported --entity %q (only 'agents' is supported)", entity)
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtAgents)
			if err != nil {
				return err
			}
			defer st.Close()

			q := fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS hostname, %s AS last_callback_at
				FROM resources a WHERE a.resource_type='%s'`,
				jx("a", "id"), jx("a", "organization_id"), jx("a", "hostname"), jx("a", "last_callback_at"), rtAgents)
			cur, err := queryMaps(ctx, st.DB(), q)
			if err != nil {
				return err
			}
			curByID := map[string]map[string]interface{}{}
			for _, r := range cur {
				curByID[fmt.Sprintf("%v", r["id"])] = r
			}

			snapDir := filepath.Join(filepath.Dir(resolveDB(dbPath)), "snapshots")
			if err := os.MkdirAll(snapDir, 0o700); err != nil {
				return fmt.Errorf("creating snapshot dir: %w", err)
			}
			snapPath := filepath.Join(snapDir, "drift-agents.json")

			prevByID := map[string]map[string]interface{}{}
			// snapPath is derived from the app's own resolved data dir plus a
			// fixed filename, not from user input; safe to read.
			if b, e := os.ReadFile(snapPath); e == nil { // #nosec G304 -- path is app-internal, not user-tainted
				var prev []map[string]interface{}
				if json.Unmarshal(b, &prev) == nil {
					for _, r := range prev {
						prevByID[fmt.Sprintf("%v", r["id"])] = r
					}
				}
			}

			added := []map[string]interface{}{}
			removed := []map[string]interface{}{}
			wentQuiet := []map[string]interface{}{}
			for id, r := range curByID {
				if _, ok := prevByID[id]; !ok {
					added = append(added, r)
				}
			}
			for id, pr := range prevByID {
				cr, ok := curByID[id]
				if !ok {
					removed = append(removed, pr)
					continue
				}
				pc, _ := pr["last_callback_at"].(string)
				cc, _ := cr["last_callback_at"].(string)
				if pc != "" && cc == pc {
					wentQuiet = append(wentQuiet, cr)
				}
			}

			if b, e := json.Marshal(cur); e == nil {
				if werr := os.WriteFile(snapPath, b, 0o600); werr != nil {
					return fmt.Errorf("writing drift snapshot: %w", werr)
				}
			}

			result := map[string]interface{}{
				"entity":           entity,
				"had_prior":        len(prevByID) > 0,
				"current_count":    len(curByID),
				"added":            added,
				"removed":          removed,
				"went_quiet":       wentQuiet,
				"added_count":      len(added),
				"removed_count":    len(removed),
				"went_quiet_count": len(wentQuiet),
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&entity, "entity", "agents", "Entity to track (only 'agents' supported)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- mttr -------------------------------------------------------------------

// pp:data-source local
func newMttrCmd(flags *rootFlags) *cobra.Command {
	var groupBy, since, dbPath string
	cmd := &cobra.Command{
		Use:         "mttr",
		Short:       "Mean time-to-resolve for closed incidents",
		Long:        "Computes mean hours from sent to closed for resolved incidents, grouped by org or severity.\nThe API exposes the timestamps but never the metric. Reads the local store; run `sync` first.",
		Example:     "  huntress-cli mttr --group-by org --since 30d --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			var groupExpr, groupName, joinOrg, groupKey string
			if groupBy == "severity" {
				groupExpr = jx("i", "severity")
				groupName = ""
				joinOrg = ""
				groupKey = "severity"
			} else {
				groupExpr = jx("i", "organization_id")
				groupName = ", " + jx("o", "name") + " AS organization_name"
				joinOrg = orgJoin("i")
				groupKey = "organization_id"
			}
			where := []string{"i.resource_type = ?",
				jx("i", "closed_at") + " IS NOT NULL", jx("i", "closed_at") + " != ''",
				jx("i", "sent_at") + " IS NOT NULL", jx("i", "sent_at") + " != ''"}
			qargs := []interface{}{rtIncidents}
			if d := parseSinceDays(since); d > 0 {
				where = append(where, "julianday('now') - "+jdx("i", "closed_at")+" <= ?")
				qargs = append(qargs, d)
			}
			q := fmt.Sprintf(`SELECT %s AS %s%s,
				COUNT(*) AS resolved_count,
				ROUND(AVG((%s - %s) * 24), 2) AS mttr_hours
			FROM resources i %s
			WHERE %s
			GROUP BY %s
			ORDER BY mttr_hours DESC`,
				groupExpr, groupKey, groupName,
				jdx("i", "closed_at"), jdx("i", "sent_at"),
				joinOrg, strings.Join(where, " AND "), groupExpr)

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&groupBy, "group-by", "org", "Group by 'org' or 'severity'")
	cmd.Flags().StringVar(&since, "since", "", "Only incidents closed within this window, e.g. 30d, 12h")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- canary-watch -----------------------------------------------------------

// pp:data-source local
func newCanaryWatchCmd(flags *rootFlags) *cobra.Command {
	var window, dbPath string
	cmd := &cobra.Command{
		Use:         "canary-watch",
		Short:       "Ransomware-canary and foothold incidents in a time window",
		Long:        "Surfaces only the highest-signal early-ransomware indicators (ransomware canaries and\nfootholds) across the fleet within a window — a curated view the API can't assemble in one call.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli canary-watch --window 24h --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			where := []string{"i.resource_type = ?",
				"(" + jx("i", "indicator_types") + " LIKE '%ransomware_canaries%' OR " + jx("i", "indicator_types") + " LIKE '%footholds%')"}
			qargs := []interface{}{rtIncidents}
			if d := parseSinceDays(window); d > 0 {
				where = append(where, jx("i", "sent_at")+" IS NOT NULL AND julianday('now') - "+jdx("i", "sent_at")+" <= ?")
				qargs = append(qargs, d)
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS status, %s AS indicator_types, %s AS sent_at, %s AS subject, %s AS summary
			FROM resources i %s
			WHERE %s
			ORDER BY %s DESC`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "status"), jx("i", "indicator_types"), jx("i", "sent_at"), jx("i", "subject"), jx("i", "summary"),
				orgJoin("i"), strings.Join(where, " AND "), jdx("i", "sent_at"))

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().StringVar(&window, "window", "24h", "Look-back window, e.g. 24h, 7d")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- org-scorecard ----------------------------------------------------------

// pp:data-source local
func newOrgScorecardCmd(flags *rootFlags) *cobra.Command {
	var org int
	var dbPath string
	cmd := &cobra.Command{
		Use:         "org-scorecard",
		Short:       "Per-client QBR rollup: agents, incidents, MTTR",
		Long:        "Assembles one organization's security story — agent count, open/closed incident counts,\nand mean time-to-resolve — into a single rollup the API returns nowhere pre-aggregated.\nDo NOT use it for the whole-fleet top-line; use 'fleet-summary' instead.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli org-scorecard --org 4821 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if org == 0 && len(args) > 0 {
				// Ignore parse errors: a non-numeric arg leaves org==0 and the
				// guard below returns help, which is the intended UX.
				_, _ = fmt.Sscanf(args[0], "%d", &org)
			}
			if org == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtOrgs, rtAgents, rtIncidents)
			if err != nil {
				return err
			}
			defer st.Close()

			card := map[string]interface{}{"organization_id": org}
			if rows, e := queryMaps(ctx, st.DB(),
				fmt.Sprintf(`SELECT %s AS name FROM resources o WHERE o.resource_type='%s' AND %s = ?`, jx("o", "name"), rtOrgs, jx("o", "id")), org); e == nil && len(rows) > 0 {
				card["organization_name"] = rows[0]["name"]
			}
			if rows, e := queryMaps(ctx, st.DB(),
				fmt.Sprintf(`SELECT COUNT(*) AS n FROM resources a WHERE a.resource_type='%s' AND %s = ?`, rtAgents, jx("a", "organization_id")), org); e == nil && len(rows) > 0 {
				card["agent_count"] = rows[0]["n"]
			}
			if rows, e := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT
				SUM(CASE WHEN %s='sent' THEN 1 ELSE 0 END) AS open_incidents,
				SUM(CASE WHEN %s='closed' THEN 1 ELSE 0 END) AS closed_incidents,
				COUNT(*) AS total_incidents
			FROM resources i WHERE i.resource_type='%s' AND %s = ?`,
				jx("i", "status"), jx("i", "status"), rtIncidents, jx("i", "organization_id")), org); e == nil && len(rows) > 0 {
				card["open_incidents"] = rows[0]["open_incidents"]
				card["closed_incidents"] = rows[0]["closed_incidents"]
				card["total_incidents"] = rows[0]["total_incidents"]
			}
			if rows, e := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT ROUND(AVG((%s-%s)*24),2) AS mttr_hours
			FROM resources i WHERE i.resource_type='%s' AND %s = ? AND %s IS NOT NULL AND %s != '' AND %s IS NOT NULL AND %s != ''`,
				jdx("i", "closed_at"), jdx("i", "sent_at"), rtIncidents, jx("i", "organization_id"),
				jx("i", "closed_at"), jx("i", "closed_at"), jx("i", "sent_at"), jx("i", "sent_at")), org); e == nil && len(rows) > 0 {
				card["mttr_hours"] = rows[0]["mttr_hours"]
			}
			return flags.printJSON(cmd, card)
		},
	}
	cmd.Flags().IntVar(&org, "org", 0, "Organization ID to score")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- stale-agents -----------------------------------------------------------

// pp:data-source local
func newStaleAgentsCmd(flags *rootFlags) *cobra.Command {
	var days int
	var platform, dbPath string
	cmd := &cobra.Command{
		Use:         "stale-agents",
		Short:       "Agents whose last callback exceeds a threshold",
		Long:        "Lists agents that haven't called back in --days days, across all organizations and optionally\nfiltered by platform — decommissioned-but-billed machines and broken installs.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli stale-agents --days 14 --platform windows --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtAgents)
			if err != nil {
				return err
			}
			defer st.Close()

			where := []string{"a.resource_type = ?",
				jx("a", "last_callback_at") + " IS NOT NULL", jx("a", "last_callback_at") + " != ''",
				"julianday('now') - " + jdx("a", "last_callback_at") + " > ?"}
			qargs := []interface{}{rtAgents, days}
			if platform != "" {
				where = append(where, jx("a", "platform")+" = ?")
				qargs = append(qargs, platform)
			}
			q := fmt.Sprintf(`SELECT %s AS id, %s AS hostname, %s AS platform, %s AS organization_id, %s AS organization_name,
				%s AS last_callback_at,
				CAST(julianday('now') - %s AS INTEGER) AS days_since_callback
			FROM resources a %s
			WHERE %s
			ORDER BY %s ASC`,
				jx("a", "id"), jx("a", "hostname"), jx("a", "platform"), jx("a", "organization_id"), jx("o", "name"),
				jx("a", "last_callback_at"), jdx("a", "last_callback_at"),
				orgJoin("a"), strings.Join(where, " AND "), jdx("a", "last_callback_at"))

			res, err := queryMaps(ctx, st.DB(), q, qargs...)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, res)
		},
	}
	cmd.Flags().IntVar(&days, "days", 7, "Flag agents with no callback in this many days")
	cmd.Flags().StringVar(&platform, "platform", "", "Filter by platform (windows, darwin, linux)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}

// ---- handoff ----------------------------------------------------------------

// pp:data-source local
func newHandoffCmd(flags *rootFlags) *cobra.Command {
	var since, dbPath string
	cmd := &cobra.Command{
		Use:         "handoff",
		Short:       "What changed since a timestamp — shift-change report",
		Long:        "Summarizes fleet activity since --since: new incidents, resolved incidents, and new\nescalations across all organizations — ready to paste into a shift handoff note.\nReads the local store; run `sync` first.",
		Example:     "  huntress-cli handoff --since 8h --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx := cmd.Context()
			st, err := openStoreFor(cmd, flags, dbPath, rtIncidents, rtEscal)
			if err != nil {
				return err
			}
			defer st.Close()

			d := parseSinceDays(since)
			if d <= 0 {
				d = 0.5
			}
			newInc, err := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS indicator_types, %s AS sent_at
			FROM resources i %s
			WHERE i.resource_type='%s' AND %s IS NOT NULL AND %s != '' AND julianday('now') - %s <= ?
			ORDER BY %s DESC`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "indicator_types"), jx("i", "sent_at"),
				orgJoin("i"), rtIncidents, jx("i", "sent_at"), jx("i", "sent_at"), jdx("i", "sent_at"), jdx("i", "sent_at")), d)
			if err != nil {
				return err
			}
			resolved, err := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT %s AS id, %s AS organization_id, %s AS organization_name,
				%s AS severity, %s AS closed_at
			FROM resources i %s
			WHERE i.resource_type='%s' AND %s IS NOT NULL AND %s != '' AND julianday('now') - %s <= ?
			ORDER BY %s DESC`,
				jx("i", "id"), jx("i", "organization_id"), jx("o", "name"),
				jx("i", "severity"), jx("i", "closed_at"),
				orgJoin("i"), rtIncidents, jx("i", "closed_at"), jx("i", "closed_at"), jdx("i", "closed_at"), jdx("i", "closed_at")), d)
			if err != nil {
				return err
			}
			// Escalations use created_at (no sent_at) and resource_type 'escalations'.
			newEsc, err := queryMaps(ctx, st.DB(), fmt.Sprintf(`SELECT %s AS id, %s AS severity, %s AS status, %s AS subject, %s AS created_at
			FROM resources e
			WHERE e.resource_type='%s' AND %s IS NOT NULL AND %s != '' AND julianday('now') - %s <= ?
			ORDER BY %s DESC`,
				jx("e", "id"), jx("e", "severity"), jx("e", "status"), jx("e", "subject"), jx("e", "created_at"),
				rtEscal, jx("e", "created_at"), jx("e", "created_at"), jdx("e", "created_at"), jdx("e", "created_at")), d)
			if err != nil {
				return err
			}
			result := map[string]interface{}{
				"window_days":      d,
				"new_incidents":    newInc,
				"resolved":         resolved,
				"new_escalations":  newEsc,
				"new_incident_n":   len(newInc),
				"resolved_n":       len(resolved),
				"new_escalation_n": len(newEsc),
			}
			return flags.printJSON(cmd, result)
		},
	}
	cmd.Flags().StringVar(&since, "since", "12h", "Look-back window, e.g. 8h, 1d")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path (default: ~/.local/share/huntress-cli/data.db)")
	return cmd
}
