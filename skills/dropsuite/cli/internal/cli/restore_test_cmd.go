// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: local ledger of restore tests performed in the
// Dropsuite portal. The API has no restore endpoint; this never restores.
// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var restoreKinds = map[string]bool{
	kindMailbox: true, kindOneDrive: true, kindSharePoint: true, kindTeams: true,
	kindSharedDrive: true, kindGoogleDrive: true, "calendar": true, "contact": true, "task": true,
}

var restoreResults = map[string]bool{"pass": true, "fail": true, "partial": true}

func newNovelRestoreTestCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "restore-test",
		Short: "Record the result of a restore test you performed in the portal and see which clients are due for one.",
		Long: `Use this command to record the result of a restore test a technician performed in the Dropsuite portal, or to list orgs due for one; it never performs a restore. Do NOT use this command to produce an audit or insurance report; use 'evidence' instead.

The Dropsuite API has no restore endpoint, so restores happen in the portal
(open it with 'portal get-login-link --org <id>'). This ledger lives only in
the local store and feeds the 'evidence' pack.`,
		Example: strings.Trim(`
  dropsuite-cli restore-test record --org 26561-14 --object jane@example.com --kind mailbox --result pass --note "restored 3 messages to a test folder"
  dropsuite-cli restore-test due --every 90d --pick --agent
  dropsuite-cli restore-test list --org 26561-14 --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newRestoreTestRecordCmd(flags), newRestoreTestListCmd(flags), newRestoreTestDueCmd(flags))
	return cmd
}

func newRestoreTestRecordCmd(flags *rootFlags) *cobra.Command {
	var object, kind, result, note, technician, testedAtRaw, dbPath string
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record a restore test you performed in the portal (local ledger only)",
		Example: strings.Trim(`
  dropsuite-cli restore-test record --org 26561-14 --object jane@example.com --kind mailbox --result pass
  dropsuite-cli restore-test record --org owner@example.com --object "example.com/Finance" --kind sharepoint --result partial --note "2 of 3 files"`, "\n"),
		Annotations: map[string]string{"mcp:local-write": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "restore-test record")
			}
			org := strings.TrimSpace(orgSelector)
			kind = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(kind)), "-", "_")
			result = strings.ToLower(strings.TrimSpace(result))
			switch {
			case org == "":
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--org is required (organization id or owner email)"))
			case strings.TrimSpace(object) == "":
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--object is required (the mailbox, site, team or drive you restored)"))
			case !restoreKinds[kind]:
				return usageErr(fmt.Errorf("--kind must be one of mailbox, onedrive, sharepoint, teams, shared_drive, google_drive, calendar, contact, task; got %q", kind))
			case !restoreResults[result]:
				return usageErr(fmt.Errorf("--result must be pass, fail or partial; got %q", result))
			}
			now := time.Now().UTC()
			testedAt := now
			if strings.TrimSpace(testedAtRaw) != "" {
				t, err := time.Parse(time.RFC3339, testedAtRaw)
				if err != nil {
					if d, err2 := time.Parse("2006-01-02", testedAtRaw); err2 == nil {
						t = d
					} else {
						return usageErr(fmt.Errorf("--tested-at must be RFC3339 or YYYY-MM-DD; got %q", testedAtRaw))
					}
				}
				if t.After(now.Add(time.Minute)) {
					return usageErr(fmt.Errorf("--tested-at %s is in the future", testedAtRaw))
				}
				testedAt = t.UTC()
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetDB(ctx, dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			ref, err := canonicalOrgRef(ctx, db.DB(), org)
			if err != nil {
				return err
			}
			// An unresolved reference is stored as given: lowercasing it could
			// turn a case-sensitive external id into another organization's.
			t := restoreTest{
				OrgRef: ref, Object: strings.TrimSpace(object), Kind: kind, Result: result,
				Note: note, Technician: technician,
				TestedAt: testedAt.Format(time.RFC3339), RecordedAt: now.Format(time.RFC3339),
			}
			id, err := insertRestoreTest(ctx, db.DB(), t)
			if err != nil {
				return fmt.Errorf("recording restore test: %w", err)
			}
			t.ID = id
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), t, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Recorded restore test #%d: %s %s %s for organization %s\n", id, t.Kind, t.Object, t.Result, t.OrgRef)
			return nil
		},
	}
	cmd.Flags().StringVar(&object, "object", "", "What you restored: mailbox address, SharePoint site, team, or drive")
	cmd.Flags().StringVar(&kind, "kind", "mailbox", "Object kind: mailbox, onedrive, sharepoint, teams, shared_drive, google_drive, calendar, contact or task")
	cmd.Flags().StringVar(&result, "result", "", "Outcome: pass, fail or partial")
	cmd.Flags().StringVar(&note, "note", "", "What was restored and how it was checked")
	cmd.Flags().StringVar(&technician, "technician", "", "Who performed the test")
	cmd.Flags().StringVar(&testedAtRaw, "tested-at", "", "When the test was performed (RFC3339 or YYYY-MM-DD; default now)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's data.db)")
	return cmd
}

func newRestoreTestListCmd(flags *rootFlags) *cobra.Command {
	var sinceRaw, dbPath string
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List recorded restore tests, newest first",
		Example:     "  dropsuite-cli restore-test list --org 26561-14 --since 180d --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "restore-test list")
			}
			var since time.Time
			if strings.TrimSpace(sinceRaw) != "" {
				d, err := parseDurationFlag("since", sinceRaw, "")
				if err != nil {
					return err
				}
				since = time.Now().UTC().Add(-d)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			tests := make([]restoreTest, 0)
			db, err := openFleetDBReadOnly(ctx, dbPath)
			if err != nil {
				return err
			}
			if db != nil {
				defer db.Close()
				tests, err = listRestoreTests(ctx, db.DB(), since)
				if err != nil {
					return err
				}
				if sel := strings.TrimSpace(orgSelector); sel != "" {
					all, err := knownOrgs(ctx, db.DB())
					if err != nil {
						return err
					}
					target := knownOrg{ID: sel}
					switch o, n := resolveKnownOrg(all, sel); {
					case n > 1:
						return ambiguousOrgRef(sel, n)
					case n == 1:
						target = o
					}
					tests = testsFor(all, target, tests)
				}
			}
			if limit > 0 && len(tests) > limit {
				tests = tests[:limit]
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), tests, flags)
			}
			if len(tests) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No restore tests recorded.")
				return nil
			}
			rows := make([][]string, 0, len(tests))
			for _, t := range tests {
				rows = append(rows, []string{t.TestedAt, t.OrgRef, t.Kind, t.Object, t.Result, t.Technician, t.Note})
			}
			return flags.printTable(cmd, []string{"TESTED AT", "ORG", "KIND", "OBJECT", "RESULT", "BY", "NOTE"}, rows)
		},
	}
	cmd.Flags().StringVar(&sinceRaw, "since", "", "Only tests performed within this window (e.g. 90d)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's data.db)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum tests to return (0 = all)")
	return cmd
}

type dueOrg struct {
	OrgID           string `json:"org_id"`
	OrgEmail        string `json:"org_email"`
	LastPassAt      string `json:"last_pass_at"`
	DaysSince       *int   `json:"days_since_last_pass"`
	Suggested       string `json:"suggested_object,omitempty"`
	SuggestedKind   string `json:"suggested_kind,omitempty"`
	SuggestedBackup string `json:"suggested_last_backup,omitempty"`
}

type dueReport struct {
	Every string   `json:"every"`
	Orgs  int      `json:"orgs_known"`
	Due   []dueOrg `json:"due"`
	Note  string   `json:"note,omitempty"`
}

type knownOrg struct {
	ID             string
	Email          string
	ExternalID     string
	OrganizationID string
	Inactive       bool // deactivated or suspended: kept for alias ownership, not due
	// OtherEmails are earlier owner emails health snapshots recorded for this
	// organization; a test recorded under one still names it.
	OtherEmails []string
}

// matches reports whether a ledger or --org reference names this
// organization (id, owner email, external_id or organization_id).
func (o knownOrg) matches(ref string) bool {
	ref = strings.TrimSpace(ref)
	for _, e := range o.OtherEmails {
		if ref != "" && strings.EqualFold(ref, e) {
			return true
		}
	}
	return ref != "" && (strings.EqualFold(ref, o.ID) ||
		(o.Email != "" && strings.EqualFold(ref, o.Email)) ||
		(o.ExternalID != "" && ref == o.ExternalID) ||
		(o.OrganizationID != "" && ref == o.OrganizationID))
}

// resolveKnownOrg returns the one organization a reference names and how
// many it matched. An id match wins; an alias (owner email, external_id,
// organization_id) shared by several organizations names none of them.
func resolveKnownOrg(orgs []knownOrg, ref string) (knownOrg, int) {
	ref = strings.TrimSpace(ref)
	for _, o := range orgs {
		if ref != "" && strings.EqualFold(ref, o.ID) {
			return o, 1
		}
	}
	var hit knownOrg
	n := 0
	for _, o := range orgs {
		if o.matches(ref) {
			hit, n = o, n+1
		}
	}
	return hit, n
}

func knownFromOrgs(orgs []dsOrg) []knownOrg {
	out := make([]knownOrg, 0, len(orgs))
	for _, o := range orgs {
		out = append(out, knownOrg{ID: o.ID, Email: o.Email, ExternalID: o.ExternalID, OrganizationID: o.OrganizationID, Inactive: o.inactive()})
	}
	return out
}

// mergeKnown joins live organizations with local history: an organization in
// both keeps its live identity plus every email history recorded for it.
func mergeKnown(live, local []knownOrg) []knownOrg {
	out := append([]knownOrg(nil), live...)
	idx := map[string]int{}
	for i, o := range out {
		idx[o.ID] = i
	}
	for _, h := range local {
		i, ok := idx[h.ID]
		if !ok {
			idx[h.ID] = len(out)
			out = append(out, h)
			continue
		}
		for _, e := range append([]string{h.Email}, h.OtherEmails...) {
			if e != "" && !out[i].matches(e) {
				out[i].OtherEmails = append(out[i].OtherEmails, e)
			}
		}
	}
	return out
}

func ambiguousOrgRef(ref string, n int) error {
	return usageErr(fmt.Errorf("--org %q matches %d organizations (a shared owner email or external_id); use the organization id", ref, n))
}

// knownOrgs collects organizations from the synced organizations table and
// the latest health snapshot (drain-first; no nested queries).
func knownOrgs(ctx context.Context, db *sql.DB) ([]knownOrg, error) {
	seen := map[string]knownOrg{}
	rows, err := db.QueryContext(ctx, `SELECT data FROM resources WHERE resource_type IN ('organizations', 'users')`)
	if err != nil && !isMissingTable(err) {
		return nil, err
	}
	if err == nil {
		raws := make([]string, 0)
		for rows.Next() {
			var data string
			if err := rows.Scan(&data); err != nil {
				_ = rows.Close()
				return nil, err
			}
			raws = append(raws, data)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
		for _, raw := range raws {
			var m map[string]any
			dec := json.NewDecoder(strings.NewReader(raw))
			dec.UseNumber()
			if dec.Decode(&m) == nil {
				if id := str(m, "id"); id != "" {
					seen[id] = knownOrg{ID: id, Email: str(m, "email"), ExternalID: str(m, "external_id"), OrganizationID: str(m, "organization_id"),
						Inactive: boolv(m, "customer_deactivated") || boolv(m, "flg_suspended")}
				}
			}
		}
	}
	// Every organization any health run has seen and every owner email it
	// was seen with (a later health --org run must not hide the others, and an
	// owner email change must not orphan tests recorded under the old one).
	hist, err := db.QueryContext(ctx, `SELECT org_id, org_email, MAX(taken_at) FROM ds_health_snapshots_v2 GROUP BY org_id, org_email ORDER BY MAX(taken_at) DESC`)
	if err != nil && !isMissingTable(err) {
		return nil, err
	}
	if err == nil {
		type seenOrg struct{ id, email string }
		found := make([]seenOrg, 0)
		for hist.Next() {
			var id, email, at string
			if err := hist.Scan(&id, &email, &at); err != nil {
				_ = hist.Close()
				return nil, err
			}
			found = append(found, seenOrg{id, email})
		}
		if err := hist.Err(); err != nil {
			_ = hist.Close()
			return nil, err
		}
		_ = hist.Close()
		for _, f := range found { // newest first: the newest email is the primary one
			o, ok := seen[f.id]
			switch {
			case !ok:
				seen[f.id] = knownOrg{ID: f.id, Email: f.email}
			case f.email != "" && !o.matches(f.email):
				o.OtherEmails = append(o.OtherEmails, f.email)
				seen[f.id] = o
			}
		}
	}
	out := make([]knownOrg, 0, len(seen))
	for _, o := range seen {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// canonicalOrgRef maps a reference to the organization id when the local
// store knows it; an unknown reference is kept as given, an ambiguous one is a
// usage error.
func canonicalOrgRef(ctx context.Context, db *sql.DB, ref string) (string, error) {
	orgs, err := knownOrgs(ctx, db)
	if err != nil {
		return "", err
	}
	o, n := resolveKnownOrg(orgs, ref)
	switch {
	case n > 1:
		return "", ambiguousOrgRef(ref, n)
	case n == 1:
		return o.ID, nil
	}
	return ref, nil
}

// testsFor keeps the ledger rows that name target: a reference that
// resolves to target alone among all known organizations or, when it
// resolves to none, one that names target itself (emails compare without
// case, anything else exactly).
func testsFor(all []knownOrg, target knownOrg, tests []restoreTest) []restoreTest {
	out := make([]restoreTest, 0)
	for _, t := range tests {
		o, n := resolveKnownOrg(all, t.OrgRef)
		switch {
		case n == 1 && o.ID == target.ID:
			out = append(out, t)
		case n == 0 && sameRef(t.OrgRef, target.ID):
			out = append(out, t)
		}
	}
	return out
}

func sameRef(a, b string) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if strings.Contains(a, "@") {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// lastPassByOrg maps org id to the newest passing test time. A test recorded
// under an alias counts only when that alias names exactly one of all known
// organizations.
func lastPassByOrg(all []knownOrg, tests []restoreTest) map[string]time.Time {
	out := map[string]time.Time{}
	for _, t := range tests {
		if t.Result != "pass" {
			continue
		}
		o, n := resolveKnownOrg(all, t.OrgRef)
		if n != 1 {
			continue
		}
		at, err := time.Parse(time.RFC3339, t.TestedAt)
		if err != nil {
			continue
		}
		if at.After(out[o.ID]) {
			out[o.ID] = at
		}
	}
	return out
}

func newRestoreTestDueCmd(flags *rootFlags) *cobra.Command {
	var everyRaw, dbPath string
	var pick bool
	cmd := &cobra.Command{
		Use:   "due",
		Short: "List organizations with no passing restore test within --every",
		Long: `List organizations (from the synced organizations table and the latest
health snapshot) with no passing restore test recorded within --every. With
--pick, suggest one object per organization to test: the most recently backed
up object graded ok in the latest health snapshot.`,
		Example:     "  dropsuite-cli restore-test due --every 90d --pick --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "restore-test due")
			}
			every, err := parseDurationFlag("every", everyRaw, "90d")
			if err != nil {
				return err
			}
			rep := dueReport{Every: fmtDuration(every), Due: make([]dueOrg, 0)}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			db, err := openFleetDBReadOnly(ctx, dbPath)
			if err != nil {
				return err
			}
			if db == nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "no local store at %s\nrun: dropsuite-cli sync --resources organizations (and health) first\n", fleetDBPath(dbPath))
				rep.Note = "no local store yet; run 'sync --resources organizations' or 'health' first"
				return printDue(cmd, flags, rep)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "organizations") {
				hintIfStale(cmd, db, "organizations", flags.maxAge)
			}
			orgs, err := knownOrgs(ctx, db.DB())
			if err != nil {
				return err
			}
			all := orgs
			active := make([]knownOrg, 0, len(orgs))
			for _, o := range orgs {
				if !o.Inactive {
					active = append(active, o)
				}
			}
			orgs = active
			if sel := strings.TrimSpace(orgSelector); sel != "" {
				kept := make([]knownOrg, 0, 1)
				switch o, n := resolveKnownOrg(all, sel); {
				case n > 1:
					return ambiguousOrgRef(sel, n)
				case n == 1:
					kept = append(kept, o)
				default:
					// Not seen locally yet: its ledger rows still match by the reference.
					kept = append(kept, knownOrg{ID: sel})
					all = append(append([]knownOrg{}, all...), kept[0])
				}
				orgs = kept
			}
			rep.Orgs = len(orgs)
			tests, err := listRestoreTests(ctx, db.DB(), time.Time{})
			if err != nil {
				return err
			}
			lastPass := lastPassByOrg(all, tests)
			var latest []snapshotRow
			if pick {
				runs, err := listSnapshotRuns(ctx, db.DB())
				if err != nil {
					return err
				}
				if len(runs) > 0 {
					if latest, err = loadSnapshotRows(ctx, db.DB(), runs[len(runs)-1].RunID, ""); err != nil {
						return err
					}
				}
			}
			now := time.Now().UTC()
			for _, o := range orgs {
				last, ok := lastPass[o.ID]
				if ok && now.Sub(last) <= every {
					continue
				}
				d := dueOrg{OrgID: o.ID, OrgEmail: o.Email}
				if ok {
					d.LastPassAt = last.Format(time.RFC3339)
					days := int(now.Sub(last).Hours() / 24)
					d.DaysSince = &days
				}
				if pick {
					best := snapshotRow{}
					for _, r := range latest {
						if r.OrgID == o.ID && r.Grade == "ok" && r.LastBackup > best.LastBackup {
							best = r
						}
					}
					if best.Object != "" {
						d.Suggested, d.SuggestedKind, d.SuggestedBackup = best.Object, best.Kind, best.LastBackup
					}
				}
				rep.Due = append(rep.Due, d)
			}
			if len(orgs) == 0 {
				rep.Note = "no organizations known locally; run 'sync --resources organizations' or 'health' first"
			}
			return printDue(cmd, flags, rep)
		},
	}
	cmd.Flags().StringVar(&everyRaw, "every", "90d", "Required restore-test interval per organization (e.g. 30d, 90d)")
	cmd.Flags().BoolVar(&pick, "pick", false, "Suggest one freshly backed-up object per organization from the latest health snapshot")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local store path (default: the CLI's data.db)")
	return cmd
}

func printDue(cmd *cobra.Command, flags *rootFlags, rep dueReport) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
	}
	w := cmd.OutOrStdout()
	if rep.Note != "" {
		fmt.Fprintln(w, rep.Note)
	}
	if len(rep.Due) == 0 {
		fmt.Fprintln(w, "No organizations are due for a restore test.")
		return nil
	}
	rows := make([][]string, 0, len(rep.Due))
	for _, d := range rep.Due {
		last := "never"
		if d.LastPassAt != "" {
			last = d.LastPassAt
		}
		rows = append(rows, []string{d.OrgID, d.OrgEmail, last, d.SuggestedKind, d.Suggested})
	}
	return flags.printTable(cmd, []string{"ORG", "OWNER", "LAST PASS", "SUGGESTED KIND", "SUGGESTED OBJECT"}, rows)
}
