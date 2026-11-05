// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: fleet-wide backup health.
// pp:data-source live

package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil"
)

// Workload kinds the health sweep grades.
const (
	kindMailbox     = "mailbox"
	kindOneDrive    = "onedrive"
	kindSharePoint  = "sharepoint"
	kindTeams       = "teams"
	kindSharedDrive = "shared_drive"
	kindGoogleDrive = "google_drive"
)

var allHealthKinds = []string{kindMailbox, kindOneDrive, kindSharePoint, kindTeams, kindSharedDrive, kindGoogleDrive}

// objectRec is one protected object as the API reports it.
type objectRec struct {
	Kind        string
	ID          string // stable identity (vendor id); Object is the display label
	Object      string
	Status      string
	LastBackup  time.Time
	AddedOn     time.Time
	Items       *int64 // nil when the API returned no count
	Storage     *int64
	Deactivated bool
	ErrText     string
}

type healthRow struct {
	OrgID      string  `json:"org_id"`
	OrgEmail   string  `json:"org_email"`
	Kind       string  `json:"kind"`
	Object     string  `json:"object"`
	ObjectID   string  `json:"object_id"`
	Status     string  `json:"status"`
	LastBackup string  `json:"last_backup"`
	AgeHours   float64 `json:"age_hours"`
	Items      *int64  `json:"items"`
	Storage    *int64  `json:"storage"`
	Grade      string  `json:"grade"`
	Reason     string  `json:"reason"`
}

type healthOrgSummary struct {
	OrgID       string `json:"org_id"`
	OrgEmail    string `json:"org_email"`
	Objects     int    `json:"objects"`
	OK          int    `json:"ok"`
	Warn        int    `json:"warn"`
	Crit        int    `json:"crit"`
	Unknown     int    `json:"unknown"`
	Deactivated int    `json:"deactivated"`
}

type healthTotals struct {
	Orgs        int `json:"orgs"`
	Objects     int `json:"objects"`
	OK          int `json:"ok"`
	Warn        int `json:"warn"`
	Crit        int `json:"crit"`
	Unknown     int `json:"unknown"`
	Deactivated int `json:"deactivated"`
}

type healthReport struct {
	GeneratedAt   string             `json:"generated_at"`
	StaleAfter    string             `json:"stale_after"`
	CriticalAfter string             `json:"critical_after"`
	Kinds         []string           `json:"kinds"`
	Summary       healthTotals       `json:"summary"`
	Orgs          []healthOrgSummary `json:"orgs"`
	Rows          []healthRow        `json:"rows"`
	SkippedOrgs   []skippedOrg       `json:"skipped_orgs"`
	FetchFailures []fetchFailure     `json:"fetch_failures"`
	Advisories    []fetchFailure     `json:"advisories,omitempty"`
	SnapshotRunID string             `json:"snapshot_run_id,omitempty"`
	Note          string             `json:"note,omitempty"`
}

// inProgressStatuses are transient states that are fine while the last
// completed backup is still fresh.
var inProgressStatuses = map[string]bool{"running": true, "preparing backup": true, "connecting": true}

// gradeObject grades one protected object. Deactivated objects return "skip".
func gradeObject(o objectRec, now time.Time, staleAfter, criticalAfter time.Duration) (grade, reason string) {
	if o.Deactivated {
		return "skip", "backup deactivated"
	}
	status := strings.ToLower(strings.TrimSpace(o.Status))
	if status == "connection failed" {
		return "crit", "Connection Failed"
	}
	if o.LastBackup.IsZero() {
		if !o.AddedOn.IsZero() && now.Sub(o.AddedOn) > criticalAfter {
			return "crit", fmt.Sprintf("no completed backup since it was added %s", formatTime(o.AddedOn))
		}
		if o.Status != "" {
			return "warn", "no completed backup yet (" + o.Status + ")"
		}
		return "warn", "no completed backup yet"
	}
	age := now.Sub(o.LastBackup)
	switch {
	case age > criticalAfter:
		return "crit", fmt.Sprintf("last backup %s ago", humanAge(age))
	case age > staleAfter:
		if inProgressStatuses[status] {
			return "warn", fmt.Sprintf("last backup %s ago, still %s", humanAge(age), o.Status)
		}
		return "warn", fmt.Sprintf("last backup %s ago", humanAge(age))
	}
	if o.ErrText != "" {
		return "warn", "vendor reported errors: " + truncate(o.ErrText, 160)
	}
	return "ok", ""
}

func humanAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d >= 48*time.Hour {
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
	return strconv.Itoa(int(d.Hours())) + "h"
}

// stableID is an object's identity across health runs: its vendor id (scoped
// by its domain for nested workloads), or the display label when the API
// returned no id. Display names repeat and change, ids do not.
func stableID(m map[string]any, scope, label string) string {
	id := str(m, "id")
	switch {
	case id == "":
		return label
	case scope != "":
		return scope + "/" + id
	}
	return id
}

func recFromMap(kind, id, object string, m map[string]any, itemsKey string) objectRec {
	r := objectRec{
		Kind:    kind,
		ID:      id,
		Object:  object,
		Status:  str(m, "current_backup_status"),
		Items:   int64Ptr(m, itemsKey),
		Storage: int64Ptr(m, "storage"),
	}
	r.LastBackup, _ = timeField(m, "last_backup")
	r.AddedOn, _ = timeField(m, "added_on")
	if _, ok := timeField(m, "deactivated_since"); ok {
		r.Deactivated = true
	}
	if boolv(m, "flg_deleted") {
		r.Deactivated = true
	}
	if v, present := m["active"]; present && v == false {
		r.Deactivated = true
	}
	if bad, text := hasErrors(m); bad {
		r.ErrText = text
	}
	return r
}

type kindFailure struct {
	Kind string
	Err  error
}

// isVendorEmpty500 reports the HTTP 500 Dropsuite returns on the SharePoint
// and Teams domain lists for organizations without that data.
func isVendorEmpty500(f kindFailure) bool {
	if f.Kind != kindSharePoint && f.Kind != kindTeams {
		return false
	}
	var apiErr *client.APIError
	return errors.As(f.Err, &apiErr) && apiErr.StatusCode == 500 &&
		(strings.HasSuffix(apiErr.Path, "/sharepoints/domains") || strings.HasSuffix(apiErr.Path, "/teams_and_groups/domains"))
}

// collectOrgObjects reads every protected object of the requested kinds for
// one organization. Each kind fails independently.
func collectOrgObjects(ctx context.Context, c *client.Client, kinds []string, maxPages int) ([]objectRec, []kindFailure) {
	objs := make([]objectRec, 0)
	fails := make([]kindFailure, 0)
	want := map[string]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	if want[kindMailbox] {
		items, err := fetchList(ctx, c, "/accounts", nil, pagedSingle, 1)
		if err != nil {
			fails = append(fails, kindFailure{kindMailbox, err})
		}
		for _, m := range items {
			objs = append(objs, recFromMap(kindMailbox, stableID(m, "", str(m, "email")), str(m, "email"), m, "msg_count"))
		}
	}
	if want[kindOneDrive] {
		items, err := fetchList(ctx, c, "/onedrives", nil, pagedByPage, maxPages)
		if err != nil {
			fails = append(fails, kindFailure{kindOneDrive, err})
		}
		for _, m := range items {
			objs = append(objs, recFromMap(kindOneDrive, stableID(m, "", str(m, "email")), str(m, "email"), m, "file_count"))
		}
	}
	if want[kindSharePoint] {
		domains, err := fetchList(ctx, c, "/sharepoints/domains", nil, pagedByPage, maxPages)
		if err != nil {
			fails = append(fails, kindFailure{kindSharePoint, err})
		}
		for _, d := range domains {
			id := str(d, "id")
			if id == "" {
				// A domain that cannot be traversed leaves the workload unread.
				fails = append(fails, kindFailure{kindSharePoint, fmt.Errorf("domain %q has no id, so its objects cannot be listed", str(d, "domain_name"))})
				continue
			}
			sites, err := fetchList(ctx, c, "/sharepoints/domains/"+url.PathEscape(id)+"/sites", nil, pagedByPage, maxPages)
			if err != nil {
				fails = append(fails, kindFailure{kindSharePoint, fmt.Errorf("domain %s sites: %w", str(d, "domain_name"), err)})
				// Rows read before the failure are still graded.
			}
			for _, s := range sites {
				label := str(d, "domain_name") + "/" + str(s, "sp_site")
				objs = append(objs, recFromMap(kindSharePoint, stableID(s, id, label), label, s, "file_count"))
			}
		}
	}
	if want[kindTeams] {
		domains, err := fetchList(ctx, c, "/teams_and_groups/domains", nil, pagedByPage, maxPages)
		if err != nil {
			fails = append(fails, kindFailure{kindTeams, err})
		}
		for _, d := range domains {
			id := str(d, "id")
			if id == "" {
				// A domain that cannot be traversed leaves the workload unread.
				fails = append(fails, kindFailure{kindTeams, fmt.Errorf("domain %q has no id, so its objects cannot be listed", str(d, "domain_name"))})
				continue
			}
			teams, err := fetchList(ctx, c, "/teams_and_groups/domains/"+url.PathEscape(id)+"/teams", nil, pagedByPage, maxPages)
			if err != nil {
				fails = append(fails, kindFailure{kindTeams, fmt.Errorf("domain %s teams: %w", str(d, "domain_name"), err)})
				// Rows read before the failure are still graded.
			}
			for _, t := range teams {
				label := str(d, "domain_name") + "/" + str(t, "group_name")
				objs = append(objs, recFromMap(kindTeams, stableID(t, id, label), label, t, "file_count"))
			}
		}
	}
	if want[kindSharedDrive] {
		domains, err := fetchList(ctx, c, "/shareddrives/domains", nil, pagedByPage, maxPages)
		if err != nil {
			fails = append(fails, kindFailure{kindSharedDrive, err})
		}
		for _, d := range domains {
			id := str(d, "id")
			if id == "" {
				// A domain that cannot be traversed leaves the workload unread.
				fails = append(fails, kindFailure{kindSharedDrive, fmt.Errorf("domain %q has no id, so its objects cannot be listed", str(d, "sd_domain"))})
				continue
			}
			drives, err := fetchList(ctx, c, "/shareddrives/domains/"+url.PathEscape(id)+"/drives", nil, pagedByPage, maxPages)
			if err != nil {
				fails = append(fails, kindFailure{kindSharedDrive, fmt.Errorf("domain %s drives: %w", str(d, "sd_domain"), err)})
				// Rows read before the failure are still graded.
			}
			for _, dr := range drives {
				label := str(d, "sd_domain") + "/" + str(dr, "sd_drive")
				objs = append(objs, recFromMap(kindSharedDrive, stableID(dr, id, label), label, dr, "file_count"))
			}
		}
	}
	if want[kindGoogleDrive] {
		items, err := fetchList(ctx, c, "/googledrives", nil, pagedByPage, maxPages)
		if err != nil {
			fails = append(fails, kindFailure{kindGoogleDrive, err})
		}
		for _, m := range items {
			name := str(m, "gd_tenant") + ":" + str(m, "drive_account_id")
			objs = append(objs, recFromMap(kindGoogleDrive, name, name, m, "file_count"))
		}
	}
	return objs, fails
}

func parseKinds(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), allHealthKinds...), nil
	}
	valid := map[string]bool{}
	for _, k := range allHealthKinds {
		valid[k] = true
	}
	out := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		k := strings.ToLower(strings.TrimSpace(part))
		k = strings.ReplaceAll(k, "-", "_")
		if k == "" {
			continue
		}
		if !valid[k] {
			return nil, usageErr(fmt.Errorf("--kinds: unknown kind %q (valid: %s)", part, strings.Join(allHealthKinds, ",")))
		}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil, usageErr(fmt.Errorf("--kinds names no workload (valid: %s)", strings.Join(allHealthKinds, ",")))
	}
	return out, nil
}

type orgHealthResult struct {
	org   dsOrg
	objs  []objectRec
	fails []kindFailure
	err   error
}

// sweepOrgs runs collectOrgObjects for every org with bounded concurrency.
func sweepOrgs(ctx context.Context, flags *rootFlags, orgs []dsOrg, kinds []string, maxPages, concurrency int) []orgHealthResult {
	results, errs := cliutil.FanoutRun(ctx, orgs,
		func(o dsOrg) string { return o.ID },
		func(ctx context.Context, o dsOrg) (orgHealthResult, error) {
			c, err := orgClient(flags, o)
			if err != nil {
				return orgHealthResult{org: o, err: err}, nil
			}
			objs, fails := collectOrgObjects(ctx, c, kinds, maxPages)
			return orgHealthResult{org: o, objs: objs, fails: fails}, nil
		},
		cliutil.WithConcurrency(concurrency),
	)
	byID := map[string]orgHealthResult{}
	for _, r := range results {
		byID[r.Source] = r.Value
	}
	for _, e := range errs {
		for _, o := range orgs {
			if o.ID == e.Source {
				byID[e.Source] = orgHealthResult{org: o, err: e.Err}
			}
		}
	}
	out := make([]orgHealthResult, 0, len(orgs))
	for _, o := range orgs {
		if r, ok := byID[o.ID]; ok {
			out = append(out, r)
		}
	}
	return out
}

// buildHealthReport grades every object. Unreadable kinds become "unknown"
// rows so a failed read is never mistaken for a healthy one.
func buildHealthReport(results []orgHealthResult, kinds []string, now time.Time, staleAfter, criticalAfter time.Duration, treat500AsEmpty bool) healthReport {
	rep := healthReport{
		GeneratedAt:   now.UTC().Format(time.RFC3339),
		StaleAfter:    fmtDuration(staleAfter),
		CriticalAfter: fmtDuration(criticalAfter),
		Kinds:         kinds,
		Orgs:          make([]healthOrgSummary, 0, len(results)),
		Rows:          make([]healthRow, 0),
		SkippedOrgs:   make([]skippedOrg, 0),
		FetchFailures: make([]fetchFailure, 0),
	}
	for _, r := range results {
		sum := healthOrgSummary{OrgID: r.org.ID, OrgEmail: r.org.Email}
		if r.err != nil {
			rep.FetchFailures = append(rep.FetchFailures, fetchFailure{OrgID: r.org.ID, OrgEmail: r.org.Email, Scope: "organization", Error: errText(r.err)})
			rep.Rows = append(rep.Rows, healthRow{OrgID: r.org.ID, OrgEmail: r.org.Email, Kind: "*", Object: "*", ObjectID: "*", Grade: "unknown", Reason: errText(r.err)})
			sum.Unknown++
			rep.Orgs = append(rep.Orgs, sum)
			continue
		}
		for _, f := range r.fails {
			if treat500AsEmpty && isVendorEmpty500(f) {
				rep.Advisories = append(rep.Advisories, fetchFailure{OrgID: r.org.ID, OrgEmail: r.org.Email, Scope: f.Kind, Error: "treated as no " + f.Kind + " data (--treat-500-as-empty): " + errText(f.Err)})
				continue
			}
			rep.FetchFailures = append(rep.FetchFailures, fetchFailure{OrgID: r.org.ID, OrgEmail: r.org.Email, Scope: f.Kind, Error: errText(f.Err)})
			rep.Rows = append(rep.Rows, healthRow{OrgID: r.org.ID, OrgEmail: r.org.Email, Kind: f.Kind, Object: "*", ObjectID: "*", Grade: "unknown", Reason: errText(f.Err)})
			sum.Unknown++
		}
		for _, o := range r.objs {
			grade, reason := gradeObject(o, now, staleAfter, criticalAfter)
			if grade == "skip" {
				sum.Deactivated++
				continue
			}
			sum.Objects++
			switch grade {
			case "ok":
				sum.OK++
			case "warn":
				sum.Warn++
			case "crit":
				sum.Crit++
			}
			rep.Rows = append(rep.Rows, healthRow{
				OrgID: r.org.ID, OrgEmail: r.org.Email, Kind: o.Kind, Object: o.Object, ObjectID: o.ID,
				Status: o.Status, LastBackup: formatTime(o.LastBackup), AgeHours: ageHours(o.LastBackup, now),
				Items: o.Items, Storage: o.Storage, Grade: grade, Reason: reason,
			})
		}
		rep.Orgs = append(rep.Orgs, sum)
	}
	for _, s := range rep.Orgs {
		rep.Summary.Orgs++
		rep.Summary.Objects += s.Objects
		rep.Summary.OK += s.OK
		rep.Summary.Warn += s.Warn
		rep.Summary.Crit += s.Crit
		rep.Summary.Unknown += s.Unknown
		rep.Summary.Deactivated += s.Deactivated
	}
	gradeRank := map[string]int{"crit": 0, "unknown": 1, "warn": 2, "ok": 3}
	sort.SliceStable(rep.Rows, func(i, j int) bool {
		a, b := rep.Rows[i], rep.Rows[j]
		if gradeRank[a.Grade] != gradeRank[b.Grade] {
			return gradeRank[a.Grade] < gradeRank[b.Grade]
		}
		if a.OrgID != b.OrgID {
			return a.OrgID < b.OrgID
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Object != b.Object {
			return a.Object < b.Object
		}
		return a.ObjectID < b.ObjectID
	})
	return rep
}

// scopeMarker rows record which organization/workload pairs a health run
// read completely, so drift only compares what both runs actually saw.
const scopeMarker = "#read"

func scopeRows(results []orgHealthResult, kinds []string) []healthRow {
	rows := make([]healthRow, 0)
	for _, r := range results {
		if r.err != nil {
			continue
		}
		failed := map[string]bool{}
		for _, f := range r.fails {
			failed[f.Kind] = true
		}
		for _, k := range kinds {
			if !failed[k] {
				rows = append(rows, healthRow{OrgID: r.org.ID, OrgEmail: r.org.Email, Kind: k, Object: scopeMarker, ObjectID: scopeMarker, Grade: "scope"})
			}
		}
	}
	return rows
}

func newRunID(now time.Time) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:])
}

func newNovelHealthCmd(flags *rootFlags) *cobra.Command {
	var staleAfterRaw, criticalAfterRaw, kindsRaw, dbPath string
	var failing, includeInactive, noSnapshot, exitCode, treat500AsEmpty bool
	var concurrency, maxPages, limit int
	var sweepTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "health",
		Short: "Grade every backed-up mailbox, OneDrive, SharePoint site, Team and drive across all clients",
		Long: `Use this command for the current backup state of every protected mailbox, OneDrive, SharePoint site, Team, shared drive and Google Drive across all client orgs (failed, stuck, stale). Do NOT use this command for what changed since an earlier run; use 'drift' instead. Do NOT use it for one named person's mailbox; use 'locate' instead.

It lists organizations with your reseller token, then reads each organization's
objects with that organization's own token. Grades: crit (Connection Failed, or
no completed backup within --critical-after), warn (older than --stale-after,
never completed, or vendor errors), ok, and unknown (the vendor would not
answer for that organization or workload; never treated as healthy).
Deactivated objects are counted, not graded. Each run is saved as a local
snapshot for 'drift' and 'evidence' (--no-snapshot to skip).

Dropsuite answers HTTP 500 on the SharePoint and Teams domain lists for an
organization with no such data; those become unknown rows unless
--treat-500-as-empty is passed (which would also hide a real outage on those
two lists).

With --exit-code the command follows the Nagios convention: 2 when any row is
crit, 3 when any row is unknown (an incomplete read), 1 when any row is warn,
and 0 otherwise. The report is printed first in every case.`,
		Example: strings.Trim(`
  dropsuite-cli health --stale-after 48h --failing --agent
  dropsuite-cli health --org 26561-14 --kinds mailbox,onedrive
  dropsuite-cli health --failing --exit-code --json`, "\n"),
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "health")
			}
			staleAfter, err := parseDurationFlag("stale-after", staleAfterRaw, "48h")
			if err != nil {
				return err
			}
			criticalAfter, err := parseDurationFlag("critical-after", criticalAfterRaw, "7d")
			if err != nil {
				return err
			}
			if criticalAfter < staleAfter {
				return usageErr(fmt.Errorf("--critical-after (%s) must not be shorter than --stale-after (%s)", criticalAfter, staleAfter))
			}
			kinds, err := parseKinds(kindsRaw)
			if err != nil {
				return err
			}
			ctx, cancel := sweepCtx(cmd, sweepTimeout)
			defer cancel()
			admin, err := adminClient(flags)
			if err != nil {
				return err
			}
			orgs, err := fetchOrgs(ctx, admin)
			if err != nil {
				return err
			}
			picked, skipped, err := selectOrgs(orgs, orgSelector, includeInactive)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			results := sweepOrgs(ctx, flags, picked, kinds, maxPages, concurrency)
			rep := buildHealthReport(results, kinds, now, staleAfter, criticalAfter, treat500AsEmpty)
			rep.SkippedOrgs = skipped
			if len(picked) == 0 {
				rep.Note = "no active organizations to check; pass --include-inactive to include deactivated or suspended customers"
			}
			if !noSnapshot && len(picked) > 0 {
				// A fresh context: the sweep budget may be spent, and the
				// report must still be saved and printed.
				sctx, scancel := context.WithTimeout(context.Background(), 30*time.Second)
				runID := newRunID(now)
				if db, err := openFleetDB(sctx, dbPath); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: snapshot not saved: %v\n", err)
				} else {
					err = saveHealthSnapshot(sctx, db.DB(), runID, now, append(append([]healthRow{}, rep.Rows...), scopeRows(results, kinds)...))
					_ = db.Close()
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "warning: snapshot not saved: %v\n", err)
					} else {
						rep.SnapshotRunID = runID
					}
				}
				scancel()
			}
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d read(s) failed; those organizations/workloads are reported as unknown, not healthy\n", len(rep.FetchFailures))
			}
			if failing {
				kept := make([]healthRow, 0, len(rep.Rows))
				for _, r := range rep.Rows {
					if r.Grade != "ok" {
						kept = append(kept, r)
					}
				}
				rep.Rows = kept
			}
			if limit > 0 && len(rep.Rows) > limit {
				rep.Rows = rep.Rows[:limit]
			}
			if err := renderHealth(cmd, flags, rep); err != nil {
				return err
			}
			if exitCode {
				switch {
				case rep.Summary.Crit > 0:
					return &cliError{code: 2, err: fmt.Errorf("health: %d critical, %d warning, %d unknown", rep.Summary.Crit, rep.Summary.Warn, rep.Summary.Unknown)}
				case rep.Summary.Unknown > 0:
					return &cliError{code: 3, err: fmt.Errorf("health: %d unknown (incomplete read), %d warning", rep.Summary.Unknown, rep.Summary.Warn)}
				case rep.Summary.Warn > 0:
					return &cliError{code: 1, err: fmt.Errorf("health: %d warning", rep.Summary.Warn)}
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&staleAfterRaw, "stale-after", "48h", "Warn when an object's last completed backup is older than this (e.g. 24h, 48h, 3d)")
	cmd.Flags().StringVar(&criticalAfterRaw, "critical-after", "7d", "Critical when an object's last completed backup is older than this")
	cmd.Flags().BoolVar(&failing, "failing", false, "Only return warn, crit and unknown rows")
	cmd.Flags().StringVar(&kindsRaw, "kinds", "", "Comma-separated workloads to check: mailbox,onedrive,sharepoint,teams,shared_drive,google_drive (default all)")
	cmd.Flags().BoolVar(&includeInactive, "include-inactive", false, "Also check organizations whose customer is deactivated or suspended")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Organizations read in parallel")
	cmd.Flags().IntVar(&maxPages, "max-pages", 40, "Maximum 25-row pages read per list (per organization and workload)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum rows to return (0 = all)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path for snapshots (default: the CLI's data.db)")
	cmd.Flags().BoolVar(&noSnapshot, "no-snapshot", false, "Do not save this run as a local snapshot")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "Nagios exit codes: 2 any crit, 3 any unknown, 1 any warn, 0 all ok")
	cmd.Flags().BoolVar(&treat500AsEmpty, "treat-500-as-empty", false, "Treat HTTP 500 on the SharePoint/Teams domain lists as 'no data' (an advisory, not unknown); also hides a real outage on those lists")
	cmd.Flags().DurationVar(&sweepTimeout, "sweep-timeout", defaultSweepTimeout, "Budget for the whole run across all organizations (each request still uses --timeout)")
	return cmd
}

func renderHealth(cmd *cobra.Command, flags *rootFlags, rep healthReport) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
	}
	w := cmd.OutOrStdout()
	s := rep.Summary
	fmt.Fprintf(w, "%d organizations, %d objects: %d ok, %d warn, %d crit, %d unknown (%d deactivated not graded)\n",
		s.Orgs, s.Objects, s.OK, s.Warn, s.Crit, s.Unknown, s.Deactivated)
	if len(rep.Rows) == 0 {
		fmt.Fprintln(w, "No rows to show.")
		return nil
	}
	rows := make([][]string, 0, len(rep.Rows))
	for _, r := range rep.Rows {
		rows = append(rows, []string{strings.ToUpper(r.Grade), r.OrgEmail, r.Kind, r.Object, r.Status, r.LastBackup, r.Reason})
	}
	return flags.printTable(cmd, []string{"GRADE", "ORG", "KIND", "OBJECT", "STATUS", "LAST BACKUP", "REASON"}, rows)
}
