// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command (keeper handfix posture-report).
// pp:data-source local

package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var hygieneKinds = []string{"invited", "locked", "inactive", "transfer"}

func newNovelUsersHygieneCmd(flags *rootFlags) *cobra.Command {
	var kinds []string
	var mc string
	cmd := &cobra.Command{
		Use:   "hygiene",
		Short: "List stale invites (with how long they have been pending), locked, inactive and pending-transfer users across every client",
		Long: `Use this command for account-lifecycle cleanup across all companies: stale invites, locked users, inactive users, pending transfers.
Do NOT use it for 2FA, weak/reused passwords or BreachWatch exposure; use 'users risk' instead.
Do NOT use it for a live last-login read of one company; use 'users last-login --mc <id>' instead.

Reads each company's newest stored posture run (no network call). inactive means no login within
that run's --days window. Invite age is how long the user has appeared as invited, under the same
Keeper user id, in consecutive stored runs (Keeper does not return an invite date; a deleted and
re-invited user starts over), so it is a lower bound.`,
		Example: strings.Trim(`
  keeper-cli users hygiene --agent
  keeper-cli users hygiene --kind invited,inactive --mc 4421 --agent`, "\n"),
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return err
			}
			want, err := parseKinds(kinds, hygieneKinds)
			if err != nil {
				return err
			}
			if mc, err = validateMC(mc); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "users hygiene")
			}
			snaps, err := latestSnapshots(cmd.Context(), mc)
			if err != nil {
				return err
			}
			hintStalePosture(cmd.ErrOrStderr(), flags, snaps)
			out := []map[string]any{}
			for _, s := range snaps {
				first, err := firstSeenFor(cmd.Context(), s.CompanyID)
				if err != nil {
					return err
				}
				for _, email := range sortedEmails(s.Users) {
					f := s.Users[email]
					st := strings.ToLower(f.Status)
					tr := strings.ToLower(f.TransferStatus)
					var hits []string
					if want["invited"] && st == "invited" {
						hits = append(hits, "invited")
					}
					if want["locked"] && strings.HasPrefix(st, "locked") {
						hits = append(hits, "locked")
					}
					if want["inactive"] && f.Inactive {
						hits = append(hits, "inactive")
					}
					if want["transfer"] && (tr == "pending transfer" || tr == "partially accepted") {
						hits = append(hits, "transfer")
					}
					for _, k := range hits {
						row := map[string]any{"company_id": s.CompanyID, "company_name": s.CompanyName, "email": email, "name": f.Name,
							"kind": k, "status": f.Status, "transfer_status": f.TransferStatus, "last_login": f.LastLogin, "run_id": s.RunID, "taken_at": s.TakenAt}
						if k == "invited" {
							row["first_seen"] = first[email]
							row["invited_days_at_least"] = daysSince(first[email])
						}
						out = append(out, row)
					}
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().StringSliceVar(&kinds, "kind", nil, "Kinds to list: invited, locked, inactive, transfer (default all)")
	cmd.Flags().StringVar(&mc, "mc", "", "Only this managed company id (company_id; \"self\" for a non-MSP account)")
	return cmd
}

// parseKinds validates a --kind list against allowed (empty means all).
func parseKinds(kinds, allowed []string) (map[string]bool, error) {
	want := map[string]bool{}
	if len(kinds) == 0 {
		for _, k := range allowed {
			want[k] = true
		}
		return want, nil
	}
	for _, k := range kinds {
		k = strings.TrimSpace(k)
		ok := false
		for _, a := range allowed {
			ok = ok || a == k
		}
		if !ok {
			return nil, usageErr(fmt.Errorf("unknown --kind %q: use %s", k, strings.Join(allowed, ", ")))
		}
		want[k] = true
	}
	return want, nil
}

// daysSince is whole days from an RFC 3339 time to now; nil when unknown.
func daysSince(at string) any {
	t, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil
	}
	return int(time.Since(t).Hours() / 24)
}
