// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-built post-sync steps (blumira handfix sync-post-hooks), called once by
// sync after its resource walks finish:
//   - record the finding snapshot history the over-time commands (drift,
//     velocity, audit, recurring) read, so every sync is a history point;
//   - fill the device list of every MSP sub-account (documented
//     GET /msp/accounts/{account_id}/agents/devices), which the flat sync
//     partitions never cover, with a per-account coverage record the device
//     commands (exposure, dc-roster, overview) use to name every sub-account
//     whose devices they could not read.

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"blumira-pp-cli/internal/cliutil"
	"blumira-pp-cli/internal/store"
)

// mspDevicesSyncName names the sub-account device walk in sync's accounting.
const mspDevicesSyncName = "msp-accounts-agents-devices"

// mspDeviceMaxPages bounds one sub-account's device walk (200 rows a page).
const mspDeviceMaxPages = 100

const mspDeviceTablesDDL = `CREATE TABLE IF NOT EXISTS msp_account_devices (
	account_id TEXT NOT NULL,
	device_id  TEXT NOT NULL,
	data       TEXT NOT NULL,
	fetched_at TEXT NOT NULL,
	PRIMARY KEY (account_id, device_id)
);
CREATE TABLE IF NOT EXISTS msp_account_device_coverage (
	account_id   TEXT PRIMARY KEY,
	account_name TEXT,
	complete     INTEGER NOT NULL,
	devices      INTEGER NOT NULL,
	error        TEXT,
	checked_at   TEXT NOT NULL,
	complete_at  TEXT
)`

func ensureMSPDeviceTables(db *sql.DB) error {
	for _, stmt := range strings.Split(mspDeviceTablesDDL, ";\n") {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// blumiraPostSync runs the post-sync steps for the resources this sync walked
// and returns one result per step it ran, for sync's success/failure counts.
// The history step runs when at least one findings partition was walked
// successfully; the device step runs when the MSP account list was walked.
func blumiraPostSync(ctx context.Context, c blumiraPageGetter, db *store.Store, resources, failed []string, events io.Writer, human bool) []syncResult {
	walked := map[string]bool{}
	for _, r := range resources {
		walked[r] = true
	}
	failedSet := map[string]bool{}
	for _, r := range failed {
		failedSet[r] = true
	}
	var out []syncResult
	report := func(res syncResult) {
		if human {
			if res.Err != nil {
				fmt.Fprintf(os.Stderr, "  %s: error: %v\n", res.Resource, res.Err)
			} else {
				fmt.Fprintf(os.Stderr, "  %s: %d synced (done)\n", res.Resource, res.Count)
			}
		} else if res.Err != nil {
			fmt.Fprintf(events, `{"event":"sync_error","resource":%q,"error":%q}`+"\n", res.Resource, res.Err.Error())
		}
		out = append(out, res)
	}

	// A findings partition whose walk failed keeps its earlier rows, which are
	// still each finding's last observed state, so one successful findings
	// walk is enough to record a history point.
	findingsRefreshed := false
	for _, rt := range findingResourceTypes {
		findingsRefreshed = findingsRefreshed || (walked[rt] && !failedSet[rt])
	}
	if findingsRefreshed {
		started := time.Now()
		findings, err := loadFindings(db)
		if err == nil {
			_, err = recordFindingHistory(db.DB(), db, findings, time.Now().UTC(), true)
		}
		res := syncResult{Resource: "finding-history", Count: len(findings), Duration: time.Since(started)}
		if err != nil {
			res.Err = fmt.Errorf("recording finding history: %w", err)
		}
		report(res)
	}

	if walked["msp"] && !cliutil.IsDogfoodEnv() {
		started := time.Now()
		n, gaps, err := syncMSPAccountDevices(ctx, c, db.DB(), db, time.Now().UTC())
		res := syncResult{Resource: mspDevicesSyncName, Count: n, Duration: time.Since(started)}
		switch {
		case err != nil:
			res.Err = err
		case len(gaps) > 0:
			res.Err = fmt.Errorf("device lists of %d MSP sub-account(s) could not be read completely: %s", len(gaps), strings.Join(gaps, "; "))
		}
		report(res)
	}
	return out
}

// syncMSPAccountDevices walks the device list of every sub-account in the
// synced msp partition. A complete walk replaces that account's stored
// devices; a walk that fails or cannot be proven complete keeps the earlier
// rows and records the account as incomplete. Accounts no longer listed lose
// their rows. Returns the devices stored and one line per incomplete account.
func syncMSPAccountDevices(ctx context.Context, c blumiraPageGetter, db *sql.DB, s *store.Store, now time.Time) (int, []string, error) {
	if err := ensureMSPDeviceTables(db); err != nil {
		return 0, nil, err
	}
	accounts, err := loadMSPAccounts(s)
	if err != nil {
		return 0, nil, err
	}
	if err := dropUnlistedMSPAccounts(db, accounts); err != nil {
		return 0, nil, err
	}
	stamp := now.UTC().Format(time.RFC3339)
	total := 0
	var gaps []string
	for _, a := range accounts {
		rows, walkErr := fetchAllBlumiraPages(ctx, c, "/msp/accounts/"+url.PathEscape(a.ID)+"/agents/devices", mspDeviceMaxPages)
		if walkErr == nil {
			walkErr = replaceMSPAccountDevices(db, a, rows, stamp)
		}
		if walkErr != nil {
			if err := ctx.Err(); err != nil {
				return total, gaps, err
			}
			gaps = append(gaps, a.ID+": "+walkErr.Error())
			if _, err := db.Exec(`INSERT INTO msp_account_device_coverage (account_id, account_name, complete, devices, error, checked_at)
				VALUES (?, ?, 0, (SELECT COUNT(*) FROM msp_account_devices WHERE account_id = ?), ?, ?)
				ON CONFLICT(account_id) DO UPDATE SET account_name = excluded.account_name, complete = 0,
				devices = excluded.devices, error = excluded.error, checked_at = excluded.checked_at`,
				a.ID, a.Name, a.ID, walkErr.Error(), stamp); err != nil {
				return total, gaps, err
			}
			continue
		}
		total += len(rows)
	}
	return total, gaps, nil
}

// dropUnlistedMSPAccounts removes the device rows and coverage records of
// every account the synced MSP account list no longer carries.
func dropUnlistedMSPAccounts(db *sql.DB, accounts []mspAccount) error {
	listed := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		listed[a.ID] = true
	}
	rows, err := db.Query(`SELECT account_id FROM msp_account_devices UNION SELECT account_id FROM msp_account_device_coverage`)
	if err != nil {
		return err
	}
	var stale []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		if !listed[id] {
			stale = append(stale, id)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range stale {
		if _, err := db.Exec(`DELETE FROM msp_account_devices WHERE account_id = ?`, id); err != nil {
			return err
		}
		if _, err := db.Exec(`DELETE FROM msp_account_device_coverage WHERE account_id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

// replaceMSPAccountDevices stores one complete sub-account walk in one
// transaction. A row without device_id cannot be keyed, so the walk then
// counts as incomplete.
func replaceMSPAccountDevices(db *sql.DB, a mspAccount, rows []json.RawMessage, stamp string) error {
	type keyed struct{ id, data string }
	items := make([]keyed, 0, len(rows))
	for i, raw := range rows {
		m, err := store.DecodeJSONObject(raw)
		if err != nil {
			return fmt.Errorf("device row %d is not a JSON object", i+1)
		}
		id := strings.TrimSpace(mapString(m, "device_id"))
		if id == "" {
			return fmt.Errorf("device row %d has no device_id", i+1)
		}
		items = append(items, keyed{id, string(raw)})
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM msp_account_devices WHERE account_id = ?`, a.ID); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO msp_account_devices (account_id, device_id, data, fetched_at) VALUES (?, ?, ?, ?)`,
			a.ID, it.id, it.data, stamp); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO msp_account_device_coverage (account_id, account_name, complete, devices, error, checked_at, complete_at)
		VALUES (?, ?, 1, ?, NULL, ?, ?)
		ON CONFLICT(account_id) DO UPDATE SET account_name = excluded.account_name, complete = 1, devices = excluded.devices,
		error = NULL, checked_at = excluded.checked_at, complete_at = excluded.complete_at`,
		a.ID, a.Name, len(items), stamp, stamp); err != nil {
		return err
	}
	return tx.Commit()
}

type mspAccount struct{ ID, Name string }

// loadMSPAccounts lists the synced sub-accounts (subaccounts_data_record).
func loadMSPAccounts(s *store.Store) ([]mspAccount, error) {
	rows, err := s.List("msp", analyticsListLimit)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []mspAccount
	for _, raw := range rows {
		m, err := store.DecodeJSONObject(raw)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(mapString(m, "account_id"))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, mspAccount{ID: id, Name: mapString(m, "name")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// deviceCoverageGap is one MSP sub-account whose devices the device commands
// could not read: its walk failed or never ran.
type deviceCoverageGap struct {
	Account     string
	AccountName string
	Detail      string
}

// deviceCoverage says which accounts' device lists the store holds in full.
type deviceCoverage struct {
	Complete map[string]bool     // sub-accounts with a complete device walk, plus accounts with direct-org devices
	Gaps     []deviceCoverageGap // synced sub-accounts without a complete walk
}

// loadDeviceCoverage reads the per-account device walk records. Every synced
// sub-account without a complete walk is a gap, so a device report names it
// instead of reading as clean.
//
// Coverage comes only from a completion record, never from row presence: the
// direct org's devices are complete only when sync's last walk of
// org-agents-devices finished (sync_state.last_attempt_complete = 1, which a
// --max-pages or --latest-only cut, an error or an interrupted walk leaves at
// 0); otherwise every account those rows name is a gap.
func loadDeviceCoverage(s *store.Store) (deviceCoverage, error) {
	cov := deviceCoverage{Complete: map[string]bool{}}
	direct, err := s.List("org-agents-devices", analyticsListLimit)
	if err != nil {
		return cov, err
	}
	directComplete, err := lastSyncWalkComplete(s.DB(), "org-agents-devices")
	if err != nil {
		return cov, err
	}
	directSeen := map[string]bool{}
	for _, raw := range direct {
		m, err := store.DecodeJSONObject(raw)
		if err != nil {
			continue
		}
		acct := mapString(m, "org_id")
		if acct == "" || directSeen[acct] {
			continue
		}
		directSeen[acct] = true
		if directComplete {
			cov.Complete[acct] = true
		} else {
			cov.Gaps = append(cov.Gaps, deviceCoverageGap{acct, "", "devices-not-synced: the last org device walk did not finish (a --max-pages or --latest-only cut, or an error); run 'sync --full --resources org-agents-devices'"})
		}
	}
	accounts, err := loadMSPAccounts(s)
	if err != nil || len(accounts) == 0 {
		return cov, err
	}
	db := s.DB()
	if err := ensureMSPDeviceTables(db); err != nil {
		return cov, err
	}
	type walk struct {
		complete           bool
		errText, completed string
	}
	walks := map[string]walk{}
	rows, err := db.Query(`SELECT account_id, complete, error, complete_at FROM msp_account_device_coverage`)
	if err != nil {
		return cov, err
	}
	for rows.Next() {
		var id string
		var complete int
		var errText, completed sql.NullString
		if err := rows.Scan(&id, &complete, &errText, &completed); err != nil {
			_ = rows.Close()
			return cov, err
		}
		walks[id] = walk{complete == 1, errText.String, completed.String}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return cov, err
	}
	for _, a := range accounts {
		w, ok := walks[a.ID]
		switch {
		case !ok:
			delete(cov.Complete, a.ID)
			cov.Gaps = append(cov.Gaps, deviceCoverageGap{a.ID, a.Name, "devices-not-synced: no device list read yet; run 'sync' (it walks every sub-account after the msp account list)"})
		case !w.complete:
			delete(cov.Complete, a.ID)
			detail := "devices-not-synced: last device walk failed: " + w.errText
			if w.completed != "" {
				detail += " (rows shown are from the complete walk at " + w.completed + ")"
			}
			cov.Gaps = append(cov.Gaps, deviceCoverageGap{a.ID, a.Name, detail})
		default:
			cov.Complete[a.ID] = true
		}
	}
	return cov, nil
}

// lastSyncWalkComplete reports whether sync's last walk of resource finished
// (sync_state.last_attempt_complete = 1). No record means no finished walk.
func lastSyncWalkComplete(db *sql.DB, resource string) (bool, error) {
	var complete int
	err := db.QueryRow(`SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, resource).Scan(&complete)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return complete == 1, nil
}

// warnDeviceCoverageGaps tells the operator which sub-accounts a device
// report could not cover.
func warnDeviceCoverageGaps(w io.Writer, gaps []deviceCoverageGap) {
	if len(gaps) == 0 {
		return
	}
	names := make([]string, 0, len(gaps))
	for _, g := range gaps {
		names = append(names, g.Account)
	}
	fmt.Fprintf(w, "warning: device data for %d MSP sub-account(s) is missing or incomplete (%s); they are listed as devices-not-synced, not as healthy\n", len(gaps), strings.Join(names, ", "))
}
