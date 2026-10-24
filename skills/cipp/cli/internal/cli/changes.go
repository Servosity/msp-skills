// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"cipp-pp-cli/internal/cliutil"
	"cipp-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// resourceSnapshotsSchema is the append-only history 'changes' compares. The
// generic resources table upserts in place, so it only ever holds the latest
// row; every fanout --save also appends here under one shared timestamp.
const resourceSnapshotsSchema = `
CREATE TABLE IF NOT EXISTS resource_snapshots (
	snapshot_ts   TEXT NOT NULL,
	resource_type TEXT NOT NULL,
	tenant        TEXT NOT NULL,
	row_key       TEXT NOT NULL,
	row_hash      TEXT NOT NULL,
	data          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_resource_snapshots_type_tenant_ts
	ON resource_snapshots(resource_type, tenant, snapshot_ts);
`

// snapshotMarkerKey is the row_key of the one marker row every snapshot
// carries; it is never a resource row.
const snapshotMarkerKey = ""

// fetchedTenants lists the tenants with at least one stored snapshot of
// resourceType: the tenants a fanout actually asked, including those that
// answered with nothing. Rollups use it so a tenant with zero rows (no
// Conditional Access policy at all) is reported, not silently absent.
func fetchedTenants(db *store.Store, resourceType string) ([]string, error) {
	var tables int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'resource_snapshots'`,
	).Scan(&tables); err != nil || tables == 0 {
		return nil, err
	}
	rows, err := db.Query(`SELECT DISTINCT tenant FROM resource_snapshots WHERE resource_type = ? ORDER BY tenant`, resourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// endpointForResource names the List endpoint whose saved rows are stored as
// resourceType: a preset or posture endpoint when one maps to it, else the
// List<Name> spelling (CIPP's Azure Functions routes are case-insensitive).
func endpointForResource(resourceType string) string {
	for _, eps := range fanoutPresets {
		for _, e := range eps {
			if resourceTypeForEndpoint(e) == resourceType {
				return e
			}
		}
	}
	for _, d := range postureDimensions {
		if d.resourceType == resourceType {
			return d.listEndpoint
		}
	}
	if resourceType == "" {
		return "/ListRoles"
	}
	return "/List" + strings.ToUpper(resourceType[:1]) + resourceType[1:]
}

// snapshotTimeFormat is RFC 3339 with fixed-width nanoseconds; time.Parse
// with time.RFC3339 reads it, and lexical order is chronological order.
const snapshotTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

// sortSnapshotTimes orders snapshot timestamps chronologically. Lexical order
// is not enough: a second-precision stamp written by an older build
// ("...:00Z") sorts after a nanosecond one in the same second ("...:00.5Z").
func sortSnapshotTimes(ts []string) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, aerr := time.Parse(time.RFC3339, ts[i])
		b, berr := time.Parse(time.RFC3339, ts[j])
		if aerr != nil || berr != nil {
			return ts[i] < ts[j]
		}
		return a.Before(b)
	})
}

// snapshotRetention is how many snapshots per (resource, tenant) are kept.
// ponytail: fixed retention; make it a flag if anyone needs a longer history.
const snapshotRetention = 20

// snapshotRowKey identifies one row across snapshots: its id when it has one
// (fanout gives id-less rows a synthetic _id), else a content hash.
func snapshotRowKey(obj map[string]any, rowHash string) string {
	for _, k := range []string{"id", "_id", "id_", "uuid"} {
		if v, ok := lookupField(obj, k); ok && v != nil {
			if s := strings.TrimSpace(fmt.Sprint(v)); s != "" {
				return s
			}
		}
	}
	return "sha256:" + rowHash
}

// appendResourceSnapshot records one tenant's rows of one resource under a
// shared snapshot timestamp, then prunes that series to snapshotRetention.
// Called by fanout after a successful store upsert (not inside its tx).
func appendResourceSnapshot(ctx context.Context, db *store.Store, snapshotTS, resourceType, tenant string, items []json.RawMessage) error {
	if _, err := db.DB().ExecContext(ctx, resourceSnapshotsSchema); err != nil {
		return fmt.Errorf("creating resource_snapshots table: %w", err)
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx,
		`INSERT INTO resource_snapshots (snapshot_ts, resource_type, tenant, row_key, row_hash, data) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	// The marker row records the snapshot itself, so an empty response is a
	// snapshot too: deleting a tenant's last role must show as a removal.
	if _, err := stmt.ExecContext(ctx, snapshotTS, resourceType, tenant, snapshotMarkerKey, "", ""); err != nil {
		return err
	}
	for _, it := range items {
		var obj map[string]any
		if json.Unmarshal(it, &obj) != nil {
			continue
		}
		// json.Marshal sorts map keys, so the hash is stable across runs.
		canon, _ := json.Marshal(obj)
		sum := sha256.Sum256(canon)
		h := hex.EncodeToString(sum[:])
		if _, err := stmt.ExecContext(ctx, snapshotTS, resourceType, tenant, snapshotRowKey(obj, h), h, string(canon)); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM resource_snapshots
		WHERE resource_type = ? AND tenant = ? AND snapshot_ts NOT IN (
			SELECT DISTINCT snapshot_ts FROM resource_snapshots
			WHERE resource_type = ? AND tenant = ? ORDER BY snapshot_ts DESC LIMIT ?)`,
		resourceType, tenant, resourceType, tenant, snapshotRetention); err != nil {
		return err
	}
	return tx.Commit()
}

type snapshotRow struct {
	ts, tenant, key, hash, data string
}

// loadResourceSnapshots reads every stored snapshot row of one resource. The
// rows are drained before returning, so callers can run follow-up queries.
func loadResourceSnapshots(db *store.Store, resourceType, tenant string) ([]snapshotRow, error) {
	var tables int
	if err := db.DB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'resource_snapshots'`,
	).Scan(&tables); err != nil {
		return nil, err
	}
	if tables == 0 {
		return nil, nil
	}
	q := `SELECT snapshot_ts, tenant, row_key, row_hash, data FROM resource_snapshots WHERE resource_type = ?`
	args := []any{resourceType}
	if tenant != "" {
		q += ` AND tenant = ?`
		args = append(args, tenant)
	}
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []snapshotRow
	for rows.Next() {
		var r snapshotRow
		if err := rows.Scan(&r.ts, &r.tenant, &r.key, &r.hash, &r.data); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// pickSnapshotPair returns the base and current snapshot timestamps from a
// sorted (ascending) distinct list: current is the newest; base is the newest
// at or before cutoff when cutoff is set, else the one before current. ok is
// false when there is nothing to compare, which is never filled in by guessing.
func pickSnapshotPair(sorted []string, cutoff time.Time) (base, current string, ok bool) {
	if len(sorted) < 2 {
		return "", "", false
	}
	current = sorted[len(sorted)-1]
	if cutoff.IsZero() {
		return sorted[len(sorted)-2], current, true
	}
	for i := len(sorted) - 2; i >= 0; i-- {
		if t, err := time.Parse(time.RFC3339, sorted[i]); err == nil && !t.After(cutoff) {
			return sorted[i], current, true
		}
	}
	return "", "", false
}

type changeRow struct {
	Tenant          string   `json:"tenant"`
	Resource        string   `json:"resource"`
	Change          string   `json:"change"`
	Key             string   `json:"key"`
	ChangedFields   []string `json:"changed_fields,omitempty"`
	BaseSnapshot    string   `json:"base_snapshot"`
	CurrentSnapshot string   `json:"current_snapshot"`
}

// changedFields lists the top-level keys whose values differ between two rows.
func changedFields(a, b string) []string {
	var x, y map[string]any
	_ = json.Unmarshal([]byte(a), &x)
	_ = json.Unmarshal([]byte(b), &y)
	keys := map[string]bool{}
	for k := range x {
		keys[k] = true
	}
	for k := range y {
		keys[k] = true
	}
	var out []string
	for k := range keys {
		xa, _ := json.Marshal(x[k])
		ya, _ := json.Marshal(y[k])
		if string(xa) != string(ya) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// computeChanges diffs each tenant's base and current snapshot. Tenants with
// fewer than two usable snapshots are counted in skipped, not reported.
func computeChanges(rows []snapshotRow, resourceType string, cutoff time.Time) (out []changeRow, skipped int) {
	byTenant := map[string]map[string]map[string]snapshotRow{} // tenant -> ts -> key -> row
	for _, r := range rows {
		if byTenant[r.tenant] == nil {
			byTenant[r.tenant] = map[string]map[string]snapshotRow{}
		}
		if byTenant[r.tenant][r.ts] == nil {
			byTenant[r.tenant][r.ts] = map[string]snapshotRow{}
		}
		if r.key == snapshotMarkerKey {
			continue // the snapshot exists; the marker is not a row
		}
		byTenant[r.tenant][r.ts][r.key] = r
	}
	tenants := make([]string, 0, len(byTenant))
	for t := range byTenant {
		tenants = append(tenants, t)
	}
	sort.Strings(tenants)
	out = []changeRow{}
	for _, t := range tenants {
		snaps := make([]string, 0, len(byTenant[t]))
		for ts := range byTenant[t] {
			snaps = append(snaps, ts)
		}
		sortSnapshotTimes(snaps)
		base, cur, ok := pickSnapshotPair(snaps, cutoff)
		if !ok {
			skipped++
			continue
		}
		b, c := byTenant[t][base], byTenant[t][cur]
		keys := map[string]bool{}
		for k := range b {
			keys[k] = true
		}
		for k := range c {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			br, inBase := b[k]
			cr, inCur := c[k]
			row := changeRow{Tenant: t, Resource: resourceType, Key: k, BaseSnapshot: base, CurrentSnapshot: cur}
			switch {
			case !inBase:
				row.Change = "added"
			case !inCur:
				row.Change = "removed"
			case br.hash != cr.hash:
				row.Change = "modified"
				row.ChangedFields = changedFields(br.data, cr.data)
			default:
				continue
			}
			out = append(out, row)
		}
	}
	return out, skipped
}

func newNovelChangesCmd(flags *rootFlags) *cobra.Command {
	var flagResource string
	var flagSince string
	var flagTenant string
	var flagDB string

	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Show rows of any stored resource added, removed or modified between two fanout snapshots.",
		Long: `Use this command for rows of a stored resource added, removed, or modified between two fanout snapshots.
Do NOT use this command for Standards baseline regression; use 'standards drift' instead.
Do NOT use it for which tenants deviate from the shared Conditional Access baseline right now; use 'ca diff' instead.

Every 'fanout --save' appends a snapshot of what it saved. This compares, per
tenant, the newest snapshot with the newest one at or before --since ago (or
with the previous snapshot when --since is not given). A tenant with fewer
than two snapshots is skipped, never guessed. Reads the local store only.

--resource takes the stored resource name (roles, oauthapps, users) or the
endpoint that produced it (/ListRoles).`,
		Example:     "  cipp-cli changes --resource roles --since 7d --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local", "pp:happy-args": "--resource=roles"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagResource == "" {
				if len(args) == 0 && cmd.Flags().NFlag() == 0 {
					return cmd.Help()
				}
				return usageErr(fmt.Errorf("--resource is required (e.g. --resource roles)"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "changes")
			}
			resourceType := strings.ToLower(strings.TrimSpace(flagResource))
			if strings.HasPrefix(resourceType, "/") {
				resourceType = resourceTypeForEndpoint(flagResource)
			}
			var cutoff time.Time
			if flagSince != "" {
				d, err := cliutil.ParseDurationLoose(flagSince)
				if err != nil || d <= 0 {
					return usageErr(fmt.Errorf("invalid --since %q: use a positive duration such as 7d, 24h or 1w", flagSince))
				}
				cutoff = time.Now().UTC().Add(-d)
			}
			dbPath := flagDB
			if dbPath == "" {
				dbPath = defaultDBPath("cipp-cli")
			}
			db, err := store.OpenWithContext(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w", err)
			}
			defer db.Close()

			rows, err := loadResourceSnapshots(db, resourceType, flagTenant)
			if err != nil {
				return fmt.Errorf("reading snapshots: %w", err)
			}
			if len(rows) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"hint: no snapshot history for %q; run 'cipp-cli fanout --endpoint %s --all-tenants --save' at two points in time to build it\n", resourceType, endpointForResource(resourceType))
			}
			out, skipped := computeChanges(rows, resourceType, cutoff)
			if skipped > 0 {
				if flagSince != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: %d tenant(s) skipped: no snapshot at or before %s to compare against (and one newer); save again later or shorten --since\n", skipped, cutoff.Format(time.RFC3339))
				} else {
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: %d tenant(s) skipped: fewer than two snapshots stored\n", skipped)
				}
			}
			if flags.asJSON {
				return printNovelJSON(cmd, flags, out)
			}
			headers := []string{"TENANT", "CHANGE", "KEY", "FIELDS"}
			table := make([][]string, 0, len(out))
			for _, r := range out {
				table = append(table, []string{r.Tenant, r.Change, r.Key, strings.Join(r.ChangedFields, ",")})
			}
			return flags.printTable(cmd, headers, table)
		},
	}
	cmd.Flags().StringVar(&flagResource, "resource", "", "Stored resource to compare (roles, oauthapps, users) or its endpoint (/ListRoles)")
	cmd.Flags().StringVar(&flagSince, "since", "", "Compare against the newest snapshot at or before this long ago (e.g. 7d, 24h); default: the previous snapshot")
	cmd.Flags().StringVar(&flagTenant, "tenant", "", "Only this tenant (its defaultDomainName)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Path to the local SQLite database (default: standard location)")
	return cmd
}
