// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix posture-report).
// pp:data-source live
// pp:client-call

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"keeper-pp-cli/internal/keeper"
	"keeper-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// selfCompanyID names the account's own enterprise when it is not an MSP.
const selfCompanyID = "self"

type postureCompanyResult struct {
	CompanyID   string         `json:"company_id"`
	CompanyName string         `json:"company_name"`
	OK          bool           `json:"ok"`
	Error       string         `json:"error,omitempty"`
	Metrics     *postureRecord `json:"metrics,omitempty"`
	Events      int            `json:"events_raised"`
}

// postureRecord is what one company's run stores: the metrics plus whether
// the security audit answered with scores.
type postureRecord struct {
	keeper.Metrics
}

func newNovelPostureReportCmd(flags *rootFlags) *cobra.Command {
	var mcs []string
	var days int
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Fetch every managed company's 2FA, password health, BreachWatch, inactivity and seats, and record the run",
		Long: `Use this command to fetch fresh posture for every managed company (or --mc) and record a new snapshot.
Do NOT use this command to see what changed between snapshots; use 'posture diff' instead.
Do NOT use it to list individual problem users; use 'users risk' or 'users hygiene' instead.

For each company it switches the Service Mode session into the company, reads users (status,
roles, 2FA), roles (admin, 2FA enforcement), last logins within --days, the security audit and
the BreachWatch summary, then switches back. Companies are read one at a time. The run is stored
locally and compared with each company's previous good run; changes become events
('events list'). A company whose reads fail is recorded as failed and never compared, so a
failed read never looks like an improvement. An account that is not an MSP is reported as its
own enterprise (company_id "self").`,
		Example: strings.Trim(`
  keeper-cli posture report --days 90 --agent
  keeper-cli posture report --mc 4421 --mc 5310 --agent`, "\n"),
		Args: cobra.NoArgs,
		// Reads Keeper only; writes only this CLI's own local store.
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if days < 1 || days > 3650 {
				return usageErr(fmt.Errorf("invalid --days %d: use 1 to 3650", days))
			}
			wanted := map[string]bool{}
			for _, m := range mcs {
				v, err := validateMC(m)
				if err != nil {
					return err
				}
				wanted[v] = true
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "read msp-info, then for each managed company switch in and read users, roles, last logins, the security audit and BreachWatch, switch back, and store the run")
			}
			p, err := newKeeperPoster(flags)
			if err != nil {
				return err
			}
			db, err := store.OpenWithContext(cmd.Context(), defaultDBPath("keeper-cli"))
			if err != nil {
				return fmt.Errorf("opening local store: %w", err)
			}
			defer db.Close()
			return runPosture(cmd, flags, p, db, wanted, days)
		},
	}
	cmd.Flags().StringArrayVar(&mcs, "mc", nil, "Only this managed company id (repeatable); default every managed company")
	cmd.Flags().IntVar(&days, "days", 90, "A user with no login in this many days counts as inactive (1 to 3650)")
	return cmd
}

type postureTarget struct {
	id, name string
	row      keeper.Row
}

// postureTargets lists the companies to read. A non-MSP account (Commander's
// "restricted to Keeper MSP administrators") is its own single target.
func postureTargets(cmd *cobra.Command, p keeper.Poster, wanted map[string]bool) ([]postureTarget, error) {
	rows, err := readKeeperRows(cmd, p, "", keeper.Companies())
	if err != nil {
		if strings.Contains(err.Error(), "restricted to Keeper MSP administrators") {
			if len(wanted) > 0 {
				return nil, usageErr(fmt.Errorf("--mc needs an MSP account; this Service Mode session is not one"))
			}
			return []postureTarget{{id: selfCompanyID, name: "own enterprise"}}, nil
		}
		return nil, err
	}
	var out []postureTarget
	seen := map[string]bool{}
	for _, r := range rows {
		id := fmt.Sprint(r["company_id"])
		if _, err := keeper.CompanyID(id); err != nil {
			return nil, apiErr(fmt.Errorf("msp-info returned a company without a numeric company_id"))
		}
		if len(wanted) > 0 && !wanted[id] {
			continue
		}
		seen[id] = true
		name, _ := r["company_name"].(string)
		out = append(out, postureTarget{id: id, name: name, row: r})
	}
	for id := range wanted {
		if !seen[id] {
			return nil, usageErr(fmt.Errorf("managed company %s is not in 'companies list'", id))
		}
	}
	return out, nil
}

// readCompany runs the five posture reads inside one switch.
func readCompany(ctx context.Context, cmd *cobra.Command, p keeper.Poster, t postureTarget, days int) (keeper.Inputs, error) {
	in := keeper.Inputs{Company: t.row, InactiveDays: days}
	ll, err := keeper.LastLogin(days)
	if err != nil {
		return in, err
	}
	mc := t.id
	if mc == selfCompanyID {
		mc = ""
	}
	warn := func(msg string) { fmt.Fprintln(cmd.ErrOrStderr(), "note: "+msg) }
	err = keeper.WithCompany(ctx, p, mc, warn, func() error {
		var err error
		if in.Users, err = keeper.Run(ctx, p, keeper.UserPosture()); err != nil {
			return err
		}
		if in.Roles, err = keeper.Run(ctx, p, keeper.RolePosture()); err != nil {
			return err
		}
		if in.LastLogin, err = keeper.Run(ctx, p, ll); err != nil {
			return err
		}
		// A security-audit error report means the password numbers are unknown, and
		// unknown is not zero: the read fails, so the run is no good run for this
		// company and every view keeps its last good numbers.
		if in.Security, err = keeper.Run(ctx, p, keeper.SecurityAudit(false)); err != nil {
			return err
		}
		in.BreachWatch, err = keeper.Run(ctx, p, keeper.SecurityAudit(true))
		if state, ok := keeper.BreachWatchUnavailable(err); ok {
			// Codex r1 P2 breachwatch-required: no BreachWatch add-on
			// (or privilege) keeps the company's other reads.
			in.BreachWatch, in.BreachWatchState, err = nil, state, nil
		}
		return err
	})
	return in, err
}

func runPosture(cmd *cobra.Command, flags *rootFlags, p keeper.Poster, db *store.Store, wanted map[string]bool, days int) error {
	ctx := cmd.Context()
	targets, err := postureTargets(cmd, p, wanted)
	if err != nil {
		return err
	}
	runID, err := db.BeginPostureRun(ctx, days)
	if err != nil {
		return err
	}
	results := make([]postureCompanyResult, 0, len(targets))
	failures, events := 0, 0
	var firstErr string
	for _, t := range targets {
		res := postureCompanyResult{CompanyID: t.id, CompanyName: t.name}
		in, rerr := readCompany(ctx, cmd, p, t, days)
		if rerr != nil {
			failures++
			res.Error = rerr.Error()
			if firstErr == "" {
				firstErr = t.id + ": " + res.Error
			}
			if err := db.PutCompanyPosture(ctx, store.CompanyPosture{RunID: runID, CompanyID: t.id, CompanyName: t.name, OK: false, Error: res.Error}, nil); err != nil {
				return err
			}
			if errors.Is(rerr, keeper.ErrSecurityAuditErrorReport) {
				if err := raiseAuditErrorEvent(ctx, db, runID, t); err != nil {
					return err
				}
			}
			results = append(results, res)
			continue
		}
		m, users := keeper.Compute(in)
		rec := postureRecord{Metrics: m}
		n, err := storeAndDiff(ctx, db, runID, t, rec, users)
		if err != nil {
			return err
		}
		res.OK, res.Metrics, res.Events = true, &rec, n
		events += n
		results = append(results, res)
	}
	status := "complete"
	switch {
	case len(targets) > 0 && failures == len(targets):
		status = "failed"
	case failures > 0:
		status = "partial"
	}
	if err := db.FinishPostureRun(ctx, runID, status); err != nil {
		return err
	}
	out := map[string]any{"run_id": runID, "status": status, "inactive_days": days, "companies": results,
		"fetch_failures": failures, "events_raised": events}
	if firstErr != "" {
		out["first_error"] = firstErr
	}
	if err := flags.printJSON(cmd, out); err != nil {
		return err
	}
	if status == "failed" {
		return apiErr(fmt.Errorf("posture run %d: every company read failed (first: %s)", runID, firstErr))
	}
	return nil
}

// raiseAuditErrorEvent records the change the first time a company's security audit
// turns into an error report: the event is once, the failed run says why every time.
func raiseAuditErrorEvent(ctx context.Context, db *store.Store, runID int64, t postureTarget) error {
	last, err := db.PreviousCompanyError(ctx, t.id, runID)
	if err != nil || strings.Contains(last, keeper.ErrSecurityAuditErrorReport.Error()) {
		return err
	}
	e := keeper.Event{Type: "company.security_audit_error", Resource: t.id, ResourceTy: "company", After: "error_report"}
	data, err := json.Marshal(map[string]any{"after": e.After, "run_id": runID})
	if err != nil {
		return err
	}
	_, err = db.InsertVendorEvent(ctx, store.VendorEvent{EventID: keeper.EventID(runID, e), Type: e.Type, Source: "poll",
		OccurredAt: time.Now().UTC().Format(time.RFC3339), ResourceType: e.ResourceTy, ResourceID: e.Resource, Client: t.id, Data: data})
	return err
}

// storeAndDiff writes one company's run and raises its change events against
// the company's previous good run.
func storeAndDiff(ctx context.Context, db *store.Store, runID int64, t postureTarget, rec postureRecord, users map[string]keeper.UserFacts) (int, error) {
	metrics, err := json.Marshal(rec)
	if err != nil {
		return 0, err
	}
	ups := make([]store.UserPosture, 0, len(users))
	for email, f := range users {
		b, err := json.Marshal(f)
		if err != nil {
			return 0, err
		}
		ups = append(ups, store.UserPosture{CompanyID: t.id, Email: email, Facts: b})
	}
	prev, had, err := db.LastGoodCompanyPosture(ctx, t.id, runID)
	if err != nil {
		return 0, err
	}
	if err := db.PutCompanyPosture(ctx, store.CompanyPosture{RunID: runID, CompanyID: t.id, CompanyName: t.name, OK: true, Metrics: metrics}, ups); err != nil {
		return 0, err
	}
	if !had {
		return 0, nil // the first good run is the baseline
	}
	var prevRec postureRecord
	if err := json.Unmarshal(prev.Metrics, &prevRec); err != nil {
		return 0, err
	}
	prevUsers, err := loadUserFacts(ctx, db, prev.RunID, t.id)
	if err != nil {
		return 0, err
	}
	evs := keeper.Diff(t.id, prevRec.Metrics, rec.Metrics, prevUsers, users)
	now := time.Now().UTC().Format(time.RFC3339)
	n := 0
	for _, e := range evs {
		data, err := json.Marshal(map[string]any{"before": e.Before, "after": e.After, "run_id": runID, "previous_run_id": prev.RunID})
		if err != nil {
			return n, err
		}
		seq, err := db.InsertVendorEvent(ctx, store.VendorEvent{EventID: keeper.EventID(runID, e), Type: e.Type, Source: "poll",
			OccurredAt: now, ResourceType: e.ResourceTy, ResourceID: e.Resource, Client: t.id, Data: data})
		if err != nil {
			return n, err
		}
		if seq > 0 {
			n++
		}
	}
	return n, nil
}

func loadUserFacts(ctx context.Context, db *store.Store, runID int64, companyID string) (map[string]keeper.UserFacts, error) {
	rows, err := db.UserPostures(ctx, runID, companyID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]keeper.UserFacts, len(rows))
	for _, u := range rows {
		var f keeper.UserFacts
		if err := json.Unmarshal(u.Facts, &f); err != nil {
			return nil, err
		}
		out[u.Email] = f
	}
	return out, nil
}
