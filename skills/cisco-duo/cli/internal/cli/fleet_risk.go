// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-duo-pp-cli/internal/client"
	"cisco-duo-pp-cli/internal/cliutil"
)

type riskCounts struct {
	Users           int `json:"users_scanned"`
	Bypass          int `json:"bypass"`
	LockedOut       int `json:"locked_out"`
	Disabled        int `json:"disabled"`
	PendingDeletion int `json:"pending_deletion"`
	Unenrolled      int `json:"unenrolled_active"`
	InBypassGroup   int `json:"in_bypass_group"`
	InDisabledGroup int `json:"in_disabled_group"`
	BypassCodes     int `json:"bypass_codes_scanned"`
	StandingCodes   int `json:"standing_bypass_codes"`
}

type fleetRiskRow struct {
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	riskCounts
	RiskScore     int  `json:"risk_score"`
	ScanCapHit    bool `json:"scan_cap_hit"`
	UndecodedRows int  `json:"undecoded_rows,omitempty"`
}

// countRisk tallies MFA-risk states. A standing code never expires or has
// unlimited reuse (Duo returns null for both); expired codes are ignored.
func countRisk(users []duoUser, codes []duoBypassCode, now time.Time) riskCounts {
	var r riskCounts
	r.Users, r.BypassCodes = len(users), len(codes)
	for _, u := range users {
		switch strings.ToLower(u.Status) {
		case "bypass":
			r.Bypass++
		case "locked out":
			r.LockedOut++
		case "disabled":
			r.Disabled++
		case "pending deletion":
			r.PendingDeletion++
		case "active":
			if !u.IsEnrolled {
				r.Unenrolled++
			}
		}
		var inBypass, inDisabled bool
		for _, g := range u.Groups {
			switch strings.ToLower(g.Status) {
			case "bypass":
				inBypass = true
			case "disabled":
				inDisabled = true
			}
		}
		if inBypass {
			r.InBypassGroup++
		}
		if inDisabled {
			r.InDisabledGroup++
		}
	}
	for _, c := range codes {
		if !c.expired(now) && c.standing() {
			r.StandingCodes++
		}
	}
	return r
}

// riskScore weights states that let someone skip MFA highest.
func riskScore(r riskCounts) int {
	return 3*(r.Bypass+r.InBypassGroup+r.StandingCodes) + 2*r.Unenrolled + r.LockedOut
}

func decodeAll[T any](items []json.RawMessage) []T {
	out, _ := decodeAllCount[T](items)
	return out
}

// decodeAllCount also reports how many rows could not be decoded, so callers
// never present a partial decode as a complete result.
func decodeAllCount[T any](items []json.RawMessage) ([]T, int) {
	out := make([]T, 0, len(items))
	rejected := 0
	for _, raw := range items {
		var v T
		if json.Unmarshal(raw, &v) != nil {
			rejected++
			continue
		}
		out = append(out, v)
	}
	return out, rejected
}

func newNovelFleetRiskCmd(flags *rootFlags) *cobra.Command {
	var only string
	var maxScanPages int
	cmd := &cobra.Command{
		Use:   "risk",
		Short: "Per-subaccount counts of bypass, locked-out, unenrolled users and standing bypass codes",
		Long: strings.Trim(`
Use this command for per-customer counts of MFA-risk user states across all
subaccounts: users in bypass, locked out, disabled, pending deletion or
active-but-unenrolled, members of Bypass or Disabled groups, and standing
bypass codes (no expiry or unlimited reuse). Rows are sorted by a risk score
that weights MFA-skip paths highest. Each subaccount scans at most
--max-scan-pages pages of users (300 per page) and bypass codes (500 per page);
scan_cap_hit marks rows where the cap stopped the scan.

Do NOT use it for user-level detail inside one tenant; use 'audit bypass' or
'audit stale' instead. Do NOT use it for seats or credits; use 'fleet summary'
instead.`, "\n"),
		Example: strings.Trim(`
  cisco-duo-cli fleet risk --agent
  cisco-duo-cli fleet risk --only "Acme Corp" --max-scan-pages 20`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet risk")
			}
			if maxScanPages < 1 {
				return usageErr(fmt.Errorf("--max-scan-pages must be at least 1"))
			}
			if cliutil.IsDogfoodEnv() {
				maxScanPages = 1
			}
			pinned := duoFleetPrep()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			accounts, err := fleetAccounts(ctx, c, only)
			if err != nil {
				return err
			}
			now := time.Now()
			rows, failures := fanoutAccounts(ctx, accounts, func(ctx context.Context, a client.DuoSubaccount) (fleetRiskRow, error) {
				users, _, capUsers, err := duoPageAll(ctx, c, a.AccountID, "/admin/v1/users", 300, maxScanPages)
				if err != nil {
					return fleetRiskRow{}, err
				}
				codes, _, capCodes, err := duoPageAll(ctx, c, a.AccountID, "/admin/v1/bypass_codes", 500, maxScanPages)
				if err != nil {
					return fleetRiskRow{}, err
				}
				du, badUsers := decodeAllCount[duoUser](users)
				dc, badCodes := decodeAllCount[duoBypassCode](codes)
				counts := countRisk(du, dc, now)
				return fleetRiskRow{AccountID: a.AccountID, Name: a.Name, riskCounts: counts, RiskScore: riskScore(counts),
					ScanCapHit: capUsers || capCodes, UndecodedRows: badUsers + badCodes}, nil
			})
			sort.Slice(rows, func(i, j int) bool {
				if rows[i].RiskScore != rows[j].RiskScore {
					return rows[i].RiskScore > rows[j].RiskScore
				}
				return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
			})
			view := struct {
				Accounts      int            `json:"accounts"`
				MaxScanPages  int            `json:"max_scan_pages"`
				Rows          []fleetRiskRow `json:"rows"`
				FetchFailures []fleetFailure `json:"fetch_failures,omitempty"`
				Note          string         `json:"note,omitempty"`
			}{Accounts: len(accounts), MaxScanPages: maxScanPages, Rows: rows, FetchFailures: failures}
			view.Note = fleetNote(pinned, len(accounts))
			warnFleetFailures(cmd, failures, len(accounts))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printMarkedRowsJSON(cmd.OutOrStdout(), view, flags); err != nil {
					return err
				}
				return allFleetFailed(len(failures), len(accounts))
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-28s %5s %6s %6s %6s %6s %6s %6s\n", "SUBACCOUNT", "SCORE", "BYPASS", "BPGRP", "CODES", "UNENR", "LOCKED", "USERS")
			for _, r := range rows {
				capMark := ""
				if r.ScanCapHit {
					capMark = " (capped)"
				}
				if r.UndecodedRows > 0 {
					capMark += fmt.Sprintf(" (%d unreadable rows)", r.UndecodedRows)
				}
				fmt.Fprintf(w, "%-28s %5d %6d %6d %6d %6d %6d %6d%s\n", trunc(r.Name, 28), r.RiskScore, r.Bypass, r.InBypassGroup,
					r.StandingCodes, r.Unenrolled, r.LockedOut, r.Users, capMark)
			}
			printFleetFailures(w, failures)
			if view.Note != "" {
				fmt.Fprintln(cmd.ErrOrStderr(), view.Note)
			}
			return allFleetFailed(len(failures), len(accounts))
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "Comma-separated subaccount IDs or names to include")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 10, "Maximum pages of users and of bypass codes to scan per subaccount")
	return cmd
}
