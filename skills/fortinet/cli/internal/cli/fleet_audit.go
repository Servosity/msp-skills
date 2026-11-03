// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// fleet audit: the four posture checks across every device, with run history
// in a local SQLite file so --new shows only findings the last run lacked.
// pp:data-source live

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"fortinet-pp-cli/internal/cliutil"
	"fortinet-pp-cli/internal/fortios"
	"fortinet-pp-cli/internal/store"
)

var auditChecks = []string{"firmware", "sslvpn", "admins", "licenses"}

type auditRow struct {
	Device   string `json:"device"`
	Serial   string `json:"serial,omitempty"`
	Firmware string `json:"firmware,omitempty"`
	SSLVPN   string `json:"sslvpn,omitempty"`
	Admins   string `json:"admins,omitempty"`
	Licenses string `json:"licenses,omitempty"`
	Status   string `json:"status"`
}

type fleetAuditView struct {
	RunAt         string            `json:"run_at"`
	Matrix        []auditRow        `json:"matrix"`
	Findings      []fortios.Finding `json:"findings"`
	NewOnly       bool              `json:"new_only"`
	PreviousRun   string            `json:"previous_run,omitempty"`
	FetchFailures []fetchFailure    `json:"fetch_failures,omitempty"`
}

func newNovelFleetAuditCmd(flags *rootFlags) *cobra.Command {
	var fo fleetOpts
	var ao auditOpts
	var checks string
	var onlyNew bool
	cmd := &cobra.Command{
		Use:   "audit",
		Args:  rejectExtraArgs,
		Short: "One PASS/WARN/FAIL/UNKNOWN matrix across every FortiGate for firmware, SSL-VPN, admins and licences",
		Long: strings.Trim(`
Use this command for a one-pass security posture verdict across every inventory
device covering firmware, SSL-VPN exposure, admin accounts and FortiGuard licence
expiry. Do NOT use this command for the per-finding evidence of one check; use
'audit firmware', 'audit sslvpn', 'audit admins' or 'audit licenses' instead.
Do NOT use it for operational state such as HA sync, tunnels or SD-WAN; use
'fleet health' instead.

Every run is recorded locally (<data dir>/fleet_audit.sqlite). --new prints only the
WARN/FAIL/UNKNOWN findings that the previous run did not have. A device that
cannot be read is UNKNOWN, never PASS. --fail-on fail exits 6 for cron/CI.`, "\n"),
		Example:     "  fortinet-cli fleet audit --new --agent\n  fortinet-cli fleet audit --check sslvpn,admins --min-version 7.2.10,7.4.5",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet audit")
			}
			if err := validFailOn(ao.failOn); err != nil {
				return err
			}
			selected, err := parseChecks(checks)
			if err != nil {
				return err
			}
			if ao.broadPrefix < 0 || ao.broadPrefix > 32 {
				return usageErr(errBroadPrefix)
			}
			floors, err := ao.floors()
			if err != nil {
				return err
			}
			within, err := ao.withinDuration()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithCancel(cmd.Context()) // --timeout bounds each request, not the fleet: see fanOut
			defer cancel()
			targets, failures, err := loadFleet(flags, fo)
			if err != nil {
				return err
			}
			loadFailures := append([]fetchFailure(nil), failures...)
			type deviceResult struct {
				row      auditRow
				findings []fortios.Finding
			}
			per, fails := fanOut(ctx, targets, fo.concurrency, func(ctx context.Context, t fleetTarget) (deviceResult, error) {
				c, err := t.client(flags)
				if err != nil {
					return deviceResult{}, err
				}
				res := deviceResult{row: auditRow{Device: t.Name}}
				if selected["firmware"] {
					fw := collectFirmware(ctx, c, t, floors)
					res.row.Serial, res.row.Firmware = fw.Serial, fortios.Status(fw.Report.Findings)
					res.findings = append(res.findings, fw.Report.Findings...)
				}
				if selected["sslvpn"] {
					fs := collectSSLVPN(ctx, c, t)
					res.row.SSLVPN = fortios.Status(fs)
					res.findings = append(res.findings, fs...)
				}
				if selected["admins"] {
					fs := collectAdmins(ctx, c, t, ao.broadPrefix)
					res.row.Admins = fortios.Status(fs)
					res.findings = append(res.findings, fs...)
				}
				if selected["licenses"] {
					_, fs := collectLicenses(ctx, c, t, within)
					res.row.Licenses = fortios.Status(fs)
					res.findings = append(res.findings, fs...)
				}
				res.row.Status = fortios.Status(res.findings)
				return res, nil
			})
			failures = append(failures, fails...)
			view := fleetAuditView{RunAt: time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z"), Matrix: []auditRow{}, Findings: []fortios.Finding{}, NewOnly: onlyNew, FetchFailures: failures}
			var current []fortios.Finding
			for _, p := range per {
				view.Matrix = append(view.Matrix, p.row)
				for _, f := range p.findings {
					if f.Status != fortios.Pass {
						current = append(current, f)
					}
				}
			}
			for _, f := range failures {
				row := auditRow{Device: f.ID, Status: fortios.Unknown}
				for c := range selected {
					setCheck(&row, c, fortios.Unknown)
				}
				view.Matrix = append(view.Matrix, row)
				current = append(current, fortios.Finding{Device: f.ID, Check: "device", Status: fortios.Unknown, Code: "unreachable", Detail: f.Error})
			}
			warnUnchecked(cmd, current, len(targets)+len(failures)-len(fails))
			prevRun, prevKeys, err := recordAuditRun(ctx, auditScope(targets, loadFailures, selected), view.RunAt, current)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: audit history not saved: %v\n", err)
			}
			view.PreviousRun = prevRun
			if onlyNew {
				for _, f := range current {
					if !prevKeys[f.Key()] {
						view.Findings = append(view.Findings, f)
					}
				}
				if prevRun == "" {
					fmt.Fprintln(cmd.ErrOrStderr(), "hint: no previous fleet audit run recorded; every finding is new")
				}
			} else {
				view.Findings = append(view.Findings, current...)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFilteredKeep(cmd.OutOrStdout(), view, flags, "vdom", "object", "code", "check"); err != nil {
					return err
				}
				return failOnErr(ao.failOn, current)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8s %-8s %-8s %-8s %s\n", "DEVICE", "FIRMWARE", "SSLVPN", "ADMINS", "LICENSES", "OVERALL")
			for _, r := range view.Matrix {
				fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-8s %-8s %-8s %-8s %s\n", r.Device, dash(r.Firmware), dash(r.SSLVPN), dash(r.Admins), dash(r.Licenses), r.Status)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if onlyNew && len(view.Findings) == 0 && len(current) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No new findings; %d finding(s) from the previous run are still open (see the matrix above).\n", len(current))
			} else {
				printFindingsTable(cmd, view.Findings)
			}
			return failOnErr(ao.failOn, current)
		},
	}
	addFleetFlags(cmd, &fo)
	cmd.Flags().StringVar(&checks, "check", strings.Join(auditChecks, ","), "Checks to run (comma-separated): firmware,sslvpn,admins,licenses")
	cmd.Flags().BoolVar(&onlyNew, "new", false, "Show only findings that were not present in the previous run")
	cmd.Flags().StringVar(&ao.minVersion, "min-version", "", "Minimum FortiOS per branch for the firmware check (e.g. 7.2.10,7.4.5)")
	cmd.Flags().StringVar(&ao.within, "within", "60d", "Licence expiry warning window (e.g. 30d)")
	cmd.Flags().IntVar(&ao.broadPrefix, "broad-prefix", 16, "API-user trusted-host prefix shorter than this is flagged (0-32)")
	addFailOnFlag(cmd, &ao)
	return cmd
}

func parseChecks(csv string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, c := range splitCSV(csv) {
		ok := false
		for _, known := range auditChecks {
			if c == known {
				ok = true
			}
		}
		if !ok {
			return nil, usageErr(fmt.Errorf("--check %q: want one of %s", c, strings.Join(auditChecks, ",")))
		}
		out[c] = true
	}
	if len(out) == 0 {
		return nil, usageErr(fmt.Errorf("--check needs at least one of %s", strings.Join(auditChecks, ",")))
	}
	return out, nil
}

func setCheck(r *auditRow, check, status string) {
	switch check {
	case "firmware":
		r.Firmware = status
	case "sslvpn":
		r.SSLVPN = status
	case "admins":
		r.Admins = status
	case "licenses":
		r.Licenses = status
	}
}

// recordAuditRun returns the previous run's timestamp and finding keys, then
// records this run. History lives in its own file so it never mixes with a
// credential-scoped sync store.
// auditScope identifies which devices, VDOMs and checks a run covered, so
// --new only compares runs over the same selection. It uses stable identities
// (device names and API URLs, the --vdoms override, check set), never fetch
// outcomes. The URL keeps two firewalls that share an inventory name (an "hq"
// in two clients' devices files) from sharing history, where the second
// one's first --new would hide its findings as already seen.
func auditScope(targets []fleetTarget, failures []fetchFailure, selected map[string]bool) string {
	var parts []string
	for _, t := range targets {
		parts = append(parts, "d:"+t.Name+"|"+t.apiURL+"|"+strings.Join(t.vdomList(), ","))
	}
	for _, f := range failures {
		v := f.vdoms
		if len(v) == 0 {
			v = []string{"(all)"}
		}
		parts = append(parts, "d:"+f.ID+"|"+f.url+"|"+strings.Join(v, ","))
	}
	for c := range selected {
		parts = append(parts, "c:"+c)
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:8])
}

func recordAuditRun(ctx context.Context, scope, runAt string, findings []fortios.Finding) (string, map[string]bool, error) {
	prevKeys := map[string]bool{}
	dir, err := cliutil.DataDir()
	if err != nil {
		return "", prevKeys, err
	}
	db, err := store.OpenWithContext(ctx, filepath.Join(dir, "fleet_audit.sqlite"))
	if err != nil {
		return "", prevKeys, err
	}
	defer db.Close()
	conn := db.DB()
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS fleet_audit_runs (run_at TEXT PRIMARY KEY, scope TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS fleet_audit_findings (run_at TEXT NOT NULL, finding_key TEXT NOT NULL, device TEXT, check_name TEXT, status TEXT, detail TEXT)`,
		`CREATE INDEX IF NOT EXISTS fleet_audit_findings_run ON fleet_audit_findings(run_at)`,
	} {
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			return "", prevKeys, err
		}
	}
	// History files from before scoped runs lack the column; add it once.
	if _, err := conn.ExecContext(ctx, `ALTER TABLE fleet_audit_runs ADD COLUMN scope TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(err.Error(), "duplicate column") {
		return "", prevKeys, err
	}
	var prev string
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_at), '') FROM fleet_audit_runs WHERE scope = ?`, scope).Scan(&prev); err != nil {
		return "", prevKeys, err
	}
	if prev != "" {
		rows, err := conn.QueryContext(ctx, `SELECT finding_key FROM fleet_audit_findings WHERE run_at = ?`, prev)
		if err != nil {
			return prev, prevKeys, err
		}
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				_ = rows.Close()
				return prev, prevKeys, err
			}
			prevKeys[k] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return prev, prevKeys, err
		}
		_ = rows.Close()
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return prev, prevKeys, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO fleet_audit_runs(run_at, scope) VALUES (?, ?)`, runAt, scope); err != nil {
		_ = tx.Rollback()
		return prev, prevKeys, err
	}
	for _, f := range findings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO fleet_audit_findings(run_at, finding_key, device, check_name, status, detail) VALUES (?,?,?,?,?,?)`,
			runAt, f.Key(), f.Device, f.Check, f.Status, f.Detail); err != nil {
			_ = tx.Rollback()
			return prev, prevKeys, err
		}
	}
	return prev, prevKeys, tx.Commit()
}
