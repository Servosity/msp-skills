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

	"netsapiens-pp-cli/internal/cliutil"
	"netsapiens-pp-cli/internal/store"
)

// The API has no modified-since filter, so change detection is a diff of the
// synced mirror against a baseline snapshot of a fixed, non-secret field set.
const fleetBaselineDDL = `CREATE TABLE IF NOT EXISTS fleet_baseline (
	entity TEXT NOT NULL,
	key TEXT NOT NULL,
	domain TEXT NOT NULL,
	fields JSON NOT NULL,
	captured_at TEXT NOT NULL,
	PRIMARY KEY (entity, key)
)`

// fleetBaselineMetaDDL records that a baseline exists even when it has zero
// entities, so later additions are reported instead of becoming a new baseline.
const fleetBaselineMetaDDL = `CREATE TABLE IF NOT EXISTS fleet_baseline_meta (
	id INTEGER PRIMARY KEY CHECK (id = 1),
	captured_at TEXT NOT NULL
)`

// fleetBaselineCoverageDDL lists the entity types the baseline actually covers;
// a type absent here was unknown when recorded and cannot yield "added".
const fleetBaselineCoverageDDL = `CREATE TABLE IF NOT EXISTS fleet_baseline_coverage (
	entity TEXT PRIMARY KEY
)`

type snapEntry struct {
	Entity string
	Key    string
	Domain string
	Fields map[string]string
}

type fieldDiff struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type fleetChange struct {
	Entity string               `json:"entity"`
	Key    string               `json:"key"`
	Domain string               `json:"domain"`
	Change string               `json:"change"` // added | removed | changed
	Fields map[string]fieldDiff `json:"fields,omitempty"`
	// PreviousDomain is the baseline's domain when the entity moved tenants
	// (a DID re-assigned from acme to beta is a change for both).
	PreviousDomain string `json:"previous_domain,omitempty"`
}

type changesReport struct {
	BaselineCapturedAt string        `json:"baseline_captured_at,omitempty"`
	ComparedAt         string        `json:"compared_at"`
	FirstRun           bool          `json:"first_run"` // no baseline existed before this run
	SkippedEntities    []string      `json:"skipped_entities,omitempty"`
	Rebaselined        bool          `json:"rebaselined"`
	Added              int           `json:"added"`
	Removed            int           `json:"removed"`
	Changed            int           `json:"changed"`
	Changes            []fleetChange `json:"changes"`
	Note               string        `json:"note,omitempty"`
	// Complete is true only when a baseline exists and every tracked entity
	// type was compared from a covered, fresh sync.
	Complete bool     `json:"complete"`
	Stale    []string `json:"stale_resources,omitempty"`
}

// fleetSnapshotSpec names the tracked entities and their compared fields.
var fleetSnapshotSpec = []struct {
	entity, rt string
	id         func(fleetRow) string
	fields     map[string][]string
}{
	{"domain", rtDomains, func(r fleetRow) string { return r.fstr("domain") }, map[string][]string{
		"description": {"description"}, "reseller": {"reseller"}, "max_users": {"limits-max-users"},
		"emergency_caller_id": {"caller-id-number-emergency"}, "time_zone": {"time-zone"}}},
	{"user", rtUsers, func(r fleetRow) string { return r.domainOf() + "/" + r.fstr("user") }, map[string][]string{
		"name": {"name-first-name"}, "last_name": {"name-last-name"}, "email": {"email"}, "scope": {"user-scope"},
		"site": {"site"}, "department": {"department"}, "caller_id": {"caller-id-number"}, "emergency_caller_id": {"caller-id-number-emergency"}}},
	{"device", rtDevices, func(r fleetRow) string { return r.domainOf() + "/" + r.fstr("device") }, map[string][]string{
		"user": {"user"}, "mac": {"device-provisioning-mac-address"}, "model": {"device-models-model"}}},
	{"phonenumber", rtPhonenumbers, func(r fleetRow) string { return r.fstr("phonenumber") }, map[string][]string{
		"domain": {"domain"}, "enabled": {"enabled"}, "application": {"dial-rule-application"},
		"destination_user": {"dial-rule-translation-destination-user"}, "destination_host": {"dial-rule-translation-destination-host"}}},
	{"smsnumber", rtSmsnumbers, func(r fleetRow) string { return r.domainOf() + "/" + r.fstr("number") }, map[string][]string{
		"application": {"application"}, "destination": {"dest"}}},
}

func snapshotRows(entity string, rows []fleetRow, id func(fleetRow) string, fields map[string][]string) []snapEntry {
	out := make([]snapEntry, 0, len(rows))
	for _, r := range rows {
		k := id(r)
		if strings.Trim(k, "/") == "" || strings.HasSuffix(k, "/") {
			continue
		}
		// Only fields the record carries are snapshotted: an omitted field is
		// unknown, and recording it as "" would fabricate a clearing later.
		f := make(map[string]string, len(fields))
		for name, keys := range fields {
			for _, k := range keys {
				if !has(r, k) {
					continue
				}
				if name == "mac" {
					f[name] = normMAC(r.fstr(k))
				} else {
					f[name] = r.fstr(k)
				}
				break
			}
		}
		out = append(out, snapEntry{Entity: entity, Key: k, Domain: r.domainOf(), Fields: f})
	}
	return out
}

// diffSnapshots reports added, removed and changed entities between two snapshots.
func diffSnapshots(base, cur []snapEntry) []fleetChange {
	idx := func(es []snapEntry) map[string]snapEntry {
		m := make(map[string]snapEntry, len(es))
		for _, e := range es {
			m[e.Entity+"\x00"+e.Key] = e
		}
		return m
	}
	b, c := idx(base), idx(cur)
	out := []fleetChange{}
	for k, e := range c {
		old, ok := b[k]
		if !ok {
			out = append(out, fleetChange{Entity: e.Entity, Key: e.Key, Domain: e.Domain, Change: "added"})
			continue
		}
		diffs := map[string]fieldDiff{}
		for f, v := range e.Fields {
			if ov, known := old.Fields[f]; known && ov != v { // compare only fields known on both sides
				diffs[f] = fieldDiff{From: ov, To: v}
			}
		}
		if len(diffs) > 0 {
			ch := fleetChange{Entity: e.Entity, Key: e.Key, Domain: e.Domain, Change: "changed", Fields: diffs}
			if old.Domain != e.Domain {
				ch.PreviousDomain = old.Domain
			}
			out = append(out, ch)
		}
	}
	for k, e := range b {
		if _, ok := c[k]; !ok {
			out = append(out, fleetChange{Entity: e.Entity, Key: e.Key, Domain: e.Domain, Change: "removed"})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		if out[i].Entity != out[j].Entity {
			return out[i].Entity < out[j].Entity
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// loadBaseline returns the saved snapshot and its capture time; ok=false when
// no baseline was ever recorded (tables absent or meta row missing).
func loadBaseline(ctx context.Context, db *store.Store) (snap []snapEntry, captured string, baseCov map[string]bool, ok bool, err error) {
	if err := db.DB().QueryRowContext(ctx, `SELECT captured_at FROM fleet_baseline_meta WHERE id = 1`).Scan(&captured); err != nil {
		return nil, "", nil, false, nil // no meta table or row: no baseline yet
	}
	baseCov = map[string]bool{}
	if crows, cerr := db.DB().QueryContext(ctx, `SELECT entity FROM fleet_baseline_coverage`); cerr == nil {
		for crows.Next() {
			var e string
			if crows.Scan(&e) == nil {
				baseCov[e] = true
			}
		}
		_ = crows.Close()
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT entity, key, domain, fields FROM fleet_baseline`)
	if err != nil {
		return nil, "", nil, false, fmt.Errorf("reading baseline: %w", err)
	}
	for rows.Next() {
		var e snapEntry
		var fields string
		if err := rows.Scan(&e.Entity, &e.Key, &e.Domain, &fields); err != nil {
			_ = rows.Close()
			return nil, "", nil, false, err
		}
		if err := json.Unmarshal([]byte(fields), &e.Fields); err != nil {
			_ = rows.Close()
			return nil, "", nil, false, fmt.Errorf("baseline row %s/%s: %w", e.Entity, e.Key, err)
		}
		snap = append(snap, e)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, "", nil, false, err
	}
	return snap, captured, baseCov, true, rows.Close()
}

func saveBaseline(ctx context.Context, db *store.Store, snap []snapEntry, cov map[string]bool, at string) error {
	for _, ddl := range []string{fleetBaselineDDL, fleetBaselineMetaDDL, fleetBaselineCoverageDDL} {
		if _, err := db.DB().ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("creating baseline table: %w", err)
		}
	}
	tx, err := db.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM fleet_baseline`); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO fleet_baseline (entity, key, domain, fields, captured_at) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range snap {
		f, _ := json.Marshal(e.Fields)
		if _, err := stmt.ExecContext(ctx, e.Entity, e.Key, e.Domain, string(f), at); err != nil {
			return fmt.Errorf("saving baseline %s/%s: %w", e.Entity, e.Key, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM fleet_baseline_coverage`); err != nil {
		return err
	}
	for e, ok := range cov {
		if ok {
			if _, err := tx.ExecContext(ctx, `INSERT INTO fleet_baseline_coverage (entity) VALUES (?)`, e); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO fleet_baseline_meta (id, captured_at) VALUES (1, ?) ON CONFLICT(id) DO UPDATE SET captured_at = excluded.captured_at`, at); err != nil {
		return err
	}
	return tx.Commit()
}

// mergeSnapshot overlays the current snapshot onto the baseline: fields the
// current records omit keep their baseline value instead of being erased.
func mergeSnapshot(base, cur []snapEntry) []snapEntry {
	old := make(map[string]snapEntry, len(base))
	for _, e := range base {
		old[e.Entity+"\x00"+e.Key] = e
	}
	out := make([]snapEntry, 0, len(cur))
	for _, e := range cur {
		if prev, ok := old[e.Entity+"\x00"+e.Key]; ok {
			merged := make(map[string]string, len(prev.Fields)+len(e.Fields))
			for k, v := range prev.Fields {
				merged[k] = v
			}
			for k, v := range e.Fields {
				merged[k] = v
			}
			e.Fields = merged
		}
		out = append(out, e)
	}
	return out
}

// onlyEntities keeps snapshot entries whose entity type is in keep.
func onlyEntities(es []snapEntry, keep map[string]bool) []snapEntry {
	out := es[:0:0]
	for _, e := range es {
		if keep[e.Entity] {
			out = append(out, e)
		}
	}
	return out
}

func newNovelFleetChangesCmd(flags *rootFlags) *cobra.Command {
	var dbPath, domain string
	var rebaseline bool
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Users, devices, numbers and domains added, removed or re-pointed since the last baseline",
		Long: `Use this command to see what was added, removed, or re-pointed across clients since the last baseline.
Do NOT use this command for current counts against seat limits; use 'fleet seats' instead.

Run sync first; this compares the synced mirror with a baseline stored in the same database.
Read-only by default. Record a baseline with --rebaseline (it reports first, then saves the
current mirror as the new baseline). Entity types whose latest sync did not complete are
skipped rather than reported as removed. Only non-secret fields are tracked.`,
		Example: strings.Trim(`
  netsapiens-cli fleet changes --agent
  netsapiens-cli fleet changes --rebaseline
  netsapiens-cli fleet changes --domain acme.example --select changes.entity,changes.key,changes.change`, "\n"),
		// Read-only by default; --rebaseline writes only the baseline tables in the CLI's own mirror, and the MCP tool refuses it.
		Annotations: map[string]string{"pp:data-source": "local", "mcp:read-only": "true", "mcp:read-only-unless-flags": "rebaseline"},
		RunE: func(cmd *cobra.Command, args []string) error {
			// The MCP tool is read-only and has no confirm step: moving the baseline
			// erases what the next comparison would report, so an agent cannot do it.
			if rebaseline && cliutil.IsMCPChildEnv() {
				return usageErr(fmt.Errorf("--rebaseline moves the baseline and is CLI only; ask the operator to run: netsapiens-cli fleet changes --rebaseline"))
			}
			// --domain only trims the report; the baseline moves for every client. Together
			// they would show one client's changes and erase the others' unseen.
			if rebaseline && domain != "" {
				return usageErr(fmt.Errorf("--rebaseline moves the baseline for every client: review the whole report, then run it without --domain"))
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet changes")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			now := time.Now().UTC().Format(time.RFC3339)
			path := fleetDBPath(dbPath)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				// No mirror means no baseline either: say so on stdout, not only in the
				// stderr hint, so a machine reader cannot take the zero counts as "no changes".
				_, err := openFleetStore(ctx, cmd, flags, dbPath, changesReport{ComparedAt: now, FirstRun: true, Changes: []fleetChange{},
					Note: "no local mirror and no baseline yet; run sync, then netsapiens-cli fleet changes --rebaseline to record one"})
				return err
			}
			open := store.OpenReadOnlyContext
			if rebaseline {
				open = store.OpenWithContext // the only write: the baseline tables in this mirror
			}
			db, err := open(ctx, path)
			if err != nil {
				return fmt.Errorf("opening local mirror %s: %w", path, err)
			}
			defer db.Close()
			var cur []snapEntry
			covered := map[string]bool{}
			cov := fleetCov{}
			for _, sp := range fleetSnapshotSpec {
				rows, err := loadCovered(ctx, db, cov, sp.rt) // only complete, fully tenant-attributed tables
				if err != nil {
					return err
				}
				cur = append(cur, snapshotRows(sp.entity, rows, sp.id, sp.fields)...)
				covered[sp.entity] = cov[sp.rt]
			}
			base, captured, baseCov, haveBase, err := loadBaseline(ctx, db)
			if err != nil {
				return err
			}
			rep := changesReport{ComparedAt: now, BaselineCapturedAt: captured, Changes: []fleetChange{}}
			if !haveBase {
				rep.FirstRun = true
				rep.Note = "no baseline yet; run netsapiens-cli fleet changes --rebaseline to record one"
			} else {
				// Compare only entity types covered both now and when the baseline was
				// recorded: an unknown side can prove neither a removal nor an addition.
				both := map[string]bool{}
				var notSynced, notInBaseline []string // the note names the cause of each skip
				for _, sp := range fleetSnapshotSpec {
					if covered[sp.entity] && baseCov[sp.entity] {
						both[sp.entity] = true
					} else {
						rep.SkippedEntities = append(rep.SkippedEntities, sp.entity)
						if !covered[sp.entity] {
							notSynced = append(notSynced, sp.entity)
						} else {
							notInBaseline = append(notInBaseline, sp.entity)
						}
					}
				}
				rep.Changes = diffSnapshots(onlyEntities(base, both), onlyEntities(cur, both))
				var notes []string
				if len(notSynced) > 0 {
					notes = append(notes, "not compared (latest sync incomplete or never run): "+strings.Join(notSynced, ", "))
				}
				if len(notInBaseline) > 0 {
					notes = append(notes, "not compared (not in the baseline; run netsapiens-cli fleet changes --rebaseline to add): "+strings.Join(notInBaseline, ", "))
				}
				rep.Note = strings.Join(notes, "; ")
			}
			if rebaseline {
				// Keep the old baseline for entity types whose sync did not complete, so a
				// change hidden by a partial sync is still reported after the next full one.
				uncovered := map[string]bool{}
				for _, sp := range fleetSnapshotSpec {
					if !covered[sp.entity] {
						uncovered[sp.entity] = true
					}
				}
				next := onlyEntities(cur, covered)
				if haveBase {
					next = mergeSnapshot(onlyEntities(base, covered), next)
					next = append(next, onlyEntities(base, uncovered)...)
				} // first baseline: covered tables only; rows from a cut-short sync are not a baseline
				nextCov := map[string]bool{}
				for _, sp := range fleetSnapshotSpec { // covered now, or preserved from a covered baseline
					nextCov[sp.entity] = covered[sp.entity] || baseCov[sp.entity]
				}
				if err := saveBaseline(ctx, db, next, nextCov, now); err != nil {
					return fmt.Errorf("saving baseline: %w", err)
				}
				rep.Rebaselined = true
				rep.BaselineCapturedAt = now
				if rep.FirstRun {
					// Count what was stored (covered tables only), not everything in the mirror.
					rep.Note = fmt.Sprintf("baseline recorded with %d entities; run again after the next sync to see changes", len(next))
					var missing []string
					for _, sp := range fleetSnapshotSpec {
						if !nextCov[sp.entity] {
							missing = append(missing, sp.entity)
						}
					}
					if len(missing) > 0 {
						rep.Note += "; not in the baseline (sync incomplete or never run): " + strings.Join(missing, ", ") +
							"; after a full sync, run --rebaseline again to add them"
					}
				}
			}
			var rts []string
			for _, sp := range fleetSnapshotSpec {
				if covered[sp.entity] {
					rts = append(rts, sp.rt)
				}
			}
			rep.Stale = fleetStale(cmd, flags, db, rts...)
			rep.Complete = haveBase && len(rep.SkippedEntities) == 0 && len(rep.Stale) == 0
			if domain != "" {
				kept := rep.Changes[:0]
				for _, c := range rep.Changes {
					if c.Domain == domain || c.PreviousDomain == domain {
						kept = append(kept, c)
					}
				}
				rep.Changes = kept
			}
			for _, c := range rep.Changes {
				switch c.Change {
				case "added":
					rep.Added++
				case "removed":
					rep.Removed++
				default:
					rep.Changed++
				}
			}
			ac := auditCoverage{Complete: rep.Complete, Missing: rep.SkippedEntities, Stale: rep.Stale}
			if !haveBase {
				ac.Missing = append([]string{"baseline"}, ac.Missing...)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				// keep: --agent/--compact drop keys present on few rows, and only "changed"
				// rows carry the from/to diff.
				return printFleetAudit(cmd, flags, ac, rep, rep.Changes, "fields", "previous_domain", "skipped_entities")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Since %s: %d added, %d removed, %d changed\n", firstNonEmpty(captured, "(new baseline)"), rep.Added, rep.Removed, rep.Changed)
			if rep.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), rep.Note)
			}
			if haveBase {
				printIncomplete(cmd, ac)
			}
			table := make([]map[string]any, 0, len(rep.Changes))
			for _, c := range rep.Changes {
				var fs []string
				for _, f := range sortedKeys(c.Fields) {
					fs = append(fs, fmt.Sprintf("%s: %q->%q", f, c.Fields[f].From, c.Fields[f].To))
				}
				table = append(table, map[string]any{"domain": c.Domain, "entity": c.Entity, "key": c.Key, "change": c.Change, "fields": strings.Join(fs, "; ")})
			}
			if len(table) == 0 {
				return nil
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror path (default: the CLI data directory)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only report changes for this client domain (not with --rebaseline, which moves every client's baseline)")
	cmd.Flags().BoolVar(&rebaseline, "rebaseline", false, "Make the current mirror the new baseline after reporting (CLI only: the MCP tool refuses it)")
	return cmd
}
