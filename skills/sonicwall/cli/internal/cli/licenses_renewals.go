// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/cliutil"
	"sonicwall-pp-cli/internal/posture"
)

type renewalRow struct {
	Serial         string `json:"serial"`
	Name           string `json:"name"`
	TenantID       string `json:"tenant_id,omitempty"`
	Service        string `json:"service"`
	ExpiryDate     string `json:"expiry_date,omitempty"`
	DaysLeft       *int   `json:"days_left,omitempty"`
	Expired        *bool  `json:"expired"`                  // null when the expiry is unknown
	ExpiryUnknown  bool   `json:"expiry_unknown,omitempty"` // no usable expiry date: listed so it is not silently dropped
	Model          string `json:"model,omitempty"`
	EndOfSupport   string `json:"end_of_support,omitempty"`
	ReplaceInstead string `json:"replace_instead"` // yes, no, or unknown when end of support could not be read
}

type renewalFailure struct {
	Serial string `json:"serial"`
	Error  string `json:"error"`
}

type renewalsResult struct {
	Days          int              `json:"days"`
	Renewals      []renewalRow     `json:"renewals"`
	FetchFailures []renewalFailure `json:"fetch_failures,omitempty"`
}

func newNovelLicensesRenewalsCmd(flags *rootFlags) *cobra.Command {
	var tenant string
	var days int
	cmd := &cobra.Command{
		Use:   "renewals",
		Short: "List licences and security services expiring within --days (default 90, expired included) per firewall and client, flagging hardware near end of support; needs SONICWALL_MSW_API_KEY",
		Long: "Use this command for the per-firewall, per-service list of licences expiring within N days, grouped by client,\n" +
			"with a replace-instead flag for hardware near end of support. Do NOT use this command for an overall\n" +
			"insurance posture row per firewall; use 'posture' instead.\n\n" +
			"Reads MySonicWall's per-serial licence list (needs SONICWALL_MSW_API_KEY) for every NSM firewall and\n" +
			"NSM's product life cycle. replace_instead is yes when end of support falls on or before the end of the window, no\n" +
			"when it falls later, and unknown when end of support could not be read.\n" +
			"Already-expired services, and services with no readable expiry date (expiry_unknown), are included.",
		Example: strings.Trim(`
  sonicwall-cli licenses renewals --days 90 --agent
  sonicwall-cli licenses renewals --days 30 --tenant 64b0c1d2 --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "licenses renewals")
			}
			if days < 0 {
				return usageErr(fmt.Errorf("--days must be zero or more, got %d", days))
			}
			if !mswKeyConfigured(flags) {
				return authErr(fmt.Errorf("licenses renewals reads MySonicWall licence data: set SONICWALL_MSW_API_KEY"))
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			devices, err := fetchFleet(ctx, c, tenant, "")
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			devices, _ = capForDogfood(devices)
			if len(devices) == 0 {
				return noFirewallsErr(cmd.OutOrStdout(), flags)
			}
			now := time.Now().UTC()
			windowEnd := now.Add(time.Duration(days) * 24 * time.Hour)
			results, fails := cliutil.FanoutRun(ctx, devices,
				func(d fleetDevice) string { return d.SerialNumber },
				func(ctx context.Context, d fleetDevice) ([]renewalRow, error) {
					raw, err := mswServiceInfo(ctx, c, d.SerialNumber)
					if err != nil {
						return nil, err
					}
					ls, ok := posture.Licences(raw, now)
					if !ok {
						return nil, fmt.Errorf("no serviceTypeList in MySonicWall serviceInfo response")
					}
					if len(ls) == 0 {
						// Nothing to grade is not "nothing expiring".
						return nil, fmt.Errorf("MySonicWall reported no services for this serial; licence expiry is unknown")
					}
					lc, lcErr := fetchLifecycle(ctx, c, d.SerialNumber)
					eos, eosOK := posture.ParseLifecycleDate(lc.EOS)
					replace := "unknown"
					if lcErr == nil && eosOK {
						replace = "no"
						if !eos.After(windowEnd) {
							replace = "yes"
						}
					}
					rows := make([]renewalRow, 0)
					for _, l := range ls {
						due := l.Expired || l.Undated || (l.DaysToExpiry != nil && *l.DaysToExpiry <= days)
						if !due {
							continue
						}
						var expired *bool
						if !l.Undated {
							expired = &l.Expired
						}
						rows = append(rows, renewalRow{Serial: d.SerialNumber, Name: d.FriendlyName, TenantID: d.TenantID,
							Service: l.Service, ExpiryDate: l.Expiry, DaysLeft: l.DaysToExpiry, Expired: expired, ExpiryUnknown: l.Undated,
							Model: lc.Model, EndOfSupport: lc.EOS, ReplaceInstead: replace})
					}
					return rows, nil
				})
			res := renewalsResult{Days: days, Renewals: make([]renewalRow, 0)}
			for _, r := range results {
				res.Renewals = append(res.Renewals, r.Value...)
			}
			for _, f := range fails {
				res.FetchFailures = append(res.FetchFailures, renewalFailure{Serial: f.Source, Error: errText(f.Err)})
			}
			partial := interrupted(ctx)
			if partial == nil && len(res.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: licence lookup failed for %d of %d firewalls; see fetch_failures\n", len(res.FetchFailures), len(devices))
				if len(res.FetchFailures) == len(devices) && len(fails) > 0 {
					partial = classifyAPIErrorOnly(fails[0].Err)
				} else {
					partial = partialFailureErr(fmt.Errorf("licence lookup failed for %d of %d firewalls", len(res.FetchFailures), len(devices)))
				}
			}
			sort.SliceStable(res.Renewals, func(i, j int) bool {
				di, dj := -1<<30, -1<<30
				if res.Renewals[i].DaysLeft != nil {
					di = *res.Renewals[i].DaysLeft
				}
				if res.Renewals[j].DaysLeft != nil {
					dj = *res.Renewals[j].DaysLeft
				}
				if res.Renewals[i].TenantID != res.Renewals[j].TenantID {
					return res.Renewals[i].TenantID < res.Renewals[j].TenantID
				}
				return di < dj
			})
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), res, flags); err != nil {
					return err
				}
				return partial
			}
			if len(res.Renewals) == 0 {
				if partial != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "No expiring licences found among the %d firewall(s) that could be read; %d could not be checked.\n", len(devices)-len(res.FetchFailures), len(res.FetchFailures))
					return partial
				}
				fmt.Fprintf(cmd.OutOrStdout(), "No licences expire within %d days across %d firewall(s).\n", days, len(devices))
				return nil
			}
			table := make([]map[string]any, 0, len(res.Renewals))
			for _, r := range res.Renewals {
				left := "?"
				if r.DaysLeft != nil {
					left = fmt.Sprint(*r.DaysLeft)
				}
				table = append(table, map[string]any{"tenant": r.TenantID, "serial": r.Serial, "name": r.Name, "service": r.Service,
					"days_left": left, "expiry": r.ExpiryDate, "replace_instead": r.ReplaceInstead})
			}
			if err := printOrderedTable(cmd.OutOrStdout(), []string{"tenant", "serial", "name", "service", "days_left", "expiry", "replace_instead"}, table); err != nil {
				return err
			}
			return partial
		},
	}
	cmd.Flags().IntVar(&days, "days", 90, "Include licences expiring within this many days (expired ones are always included)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Only firewalls in this NSM tenant ID")
	return cmd
}
