// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"watchguard-pp-cli/internal/client"
	"watchguard-pp-cli/internal/cliutil"
)

type postureRow struct {
	AccountID      string   `json:"account_id"`
	AccountName    string   `json:"account_name,omitempty"`
	HostName       string   `json:"host_name"`
	DeviceID       string   `json:"device_id"`
	Managed        bool     `json:"managed"`
	Platform       string   `json:"platform,omitempty"`
	Protection     string   `json:"protection,omitempty"`
	Isolation      string   `json:"isolation,omitempty"`
	EngineUpdate   string   `json:"engine_update,omitempty"`
	License        string   `json:"license,omitempty"`
	LastConnection string   `json:"last_connection,omitempty"`
	Issues         []string `json:"issues"`
}

type postureView struct {
	AccountsChecked   int            `json:"accounts_checked"`
	DevicesScanned    int            `json:"devices_scanned"`
	Devices           []postureRow   `json:"devices"`
	TruncatedAccounts []string       `json:"truncated_accounts,omitempty"`
	FetchFailures     []fetchFailure `json:"fetch_failures,omitempty"`
	Note              string         `json:"note,omitempty"`
}

type accountPosture struct {
	scanned   int
	rows      []postureRow
	truncated bool
}

// postureFromRows turns protection-status and unmanaged-device rows for one
// account into at-risk rows (or every row when all is true).
func postureFromRows(sub subscriber, protection, unmanaged []map[string]any, all bool) []postureRow {
	out := make([]postureRow, 0)
	for _, m := range protection {
		issues := protectionIssues(m)
		if len(issues) == 0 && !all {
			continue
		}
		out = append(out, postureRow{
			AccountID: sub.ID, AccountName: sub.Name,
			HostName: strField(m, "host_name"), DeviceID: strField(m, "device_id"), Managed: true,
			Platform:       decodeCode(platformCodes, m, "platform_id"),
			Protection:     decodeCode(protectionCodes, m, "accumulated_protection_status"),
			Isolation:      decodeCode(isolationCodes, m, "isolation_state"),
			EngineUpdate:   decodeCode(engineCodes, m, "protection_engine_update_status"),
			License:        decodeCode(licenseCodes, m, "license_status"),
			LastConnection: strField(m, "last_connection_date"),
			Issues:         issues,
		})
	}
	for _, m := range unmanaged {
		issue := "unmanaged: no endpoint agent"
		if s := decodeCode(unmanagedCodes, m, "status"); s != "" {
			issue += " (" + s + ")"
		}
		out = append(out, postureRow{
			AccountID: sub.ID, AccountName: sub.Name,
			HostName: strField(m, "host_name"), DeviceID: strField(m, "device_id"),
			LastConnection: strField(m, "last_seen_datetime"),
			Issues:         []string{issue},
		})
	}
	return out
}

func fetchAccountPosture(ctx context.Context, c *client.Client, sub subscriber, maxPages int, withUnmanaged, all bool) (accountPosture, error) {
	base := "/rest/endpoint-security/management/api/v1/accounts/" + url.PathEscape(sub.ID)
	prot, trunc, err := pageOData(ctx, c, sub.ID, base+"/devicesprotectionstatus", maxPages)
	if err != nil {
		return accountPosture{}, err
	}
	var unmanaged []map[string]any
	if withUnmanaged {
		var t2 bool
		unmanaged, t2, err = pageOData(ctx, c, sub.ID, base+"/unmanageddevices", maxPages)
		if err != nil {
			return accountPosture{}, err
		}
		trunc = trunc || t2
	}
	return accountPosture{scanned: len(prot) + len(unmanaged), rows: postureFromRows(sub, prot, unmanaged, all), truncated: trunc}, nil
}

func newNovelEndpointDevicesPostureCmd(flags *rootFlags) *cobra.Command {
	var tf tenantFlags
	var all, noUnmanaged bool
	var limit int
	cmd := &cobra.Command{
		Use:   "posture",
		Short: "List endpoints that are unprotected, out of date, isolated, unlicensed or waiting on a reboot, across every subscriber or --accounts",
		Long: "Use this command for a device-level list of endpoints that are unprotected, out of date, isolated, unlicensed or awaiting reboot across subscriber accounts. " +
			"Do NOT use this command for one-row-per-customer counts across products; use 'accounts health' instead. " +
			"Do NOT use it for missing patches or CVEs; use 'endpoint-patches exposure' instead.\n\n" +
			"Built for a WatchGuard Service Provider account: each managed account is reached with its own audience-scoped token, so one credential covers every customer. " +
			"A Subscriber credential checks only its own account. Integer status codes are decoded to labels; discovered devices with no endpoint agent are listed as unmanaged.\n\n" +
			"Exit codes: 0 all accounts read, 5 every account failed, 6 some accounts failed (see fetch_failures; --allow-partial-failure exits 0).",
		Example: strings.Trim(`
  watchguard-cli endpoint-devices posture --agent
  watchguard-cli endpoint-devices posture --accounts ACC-1234567,ACC-7654321 --json
  watchguard-cli endpoint-devices posture --include-self --all --csv`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live", "pp:typed-exit-codes": "0,6"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "endpoint-devices posture")
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			subs, accTrunc, err := resolveSubscribers(ctx, c, tf)
			if err != nil {
				return classifyAPIErrorOnly(err)
			}
			results, errs := cliutil.FanoutRun(ctx, subs, func(s subscriber) string { return s.ID },
				func(ctx context.Context, s subscriber) (accountPosture, error) {
					return fetchAccountPosture(ctx, c, s, tf.pages(), !noUnmanaged, all)
				}, cliutil.WithConcurrency(tf.workers()))
			view := postureView{Devices: make([]postureRow, 0), FetchFailures: fanoutFailures(errs)}
			for _, r := range results {
				view.AccountsChecked++
				view.DevicesScanned += r.Value.scanned
				view.Devices = append(view.Devices, r.Value.rows...)
				if r.Value.truncated {
					view.TruncatedAccounts = append(view.TruncatedAccounts, r.Source)
				}
			}
			sort.SliceStable(view.Devices, func(i, j int) bool {
				a, b := view.Devices[i], view.Devices[j]
				if len(a.Issues) != len(b.Issues) {
					return len(a.Issues) > len(b.Issues)
				}
				if a.AccountID != b.AccountID {
					return a.AccountID < b.AccountID
				}
				return a.HostName < b.HostName
			})
			if limit > 0 && len(view.Devices) > limit {
				view.Devices = view.Devices[:limit]
			}
			if len(view.TruncatedAccounts) > 0 {
				view.Note = "some accounts had more devices than --max-pages allowed; raise it to read them all"
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+view.Note+": "+strings.Join(view.TruncatedAccounts, ", "))
			}
			if n := accountsTruncatedNote(cmd.ErrOrStderr(), accTrunc); n != "" {
				view.Note = strings.TrimPrefix(view.Note+"; "+n, "; ")
			}
			if err := renderPosture(cmd, flags, view); err != nil {
				return err
			}
			return fanoutVerdict(cmd.ErrOrStderr(), flags, len(errs), len(subs))
		},
	}
	tf.register(cmd.Flags())
	cmd.Flags().BoolVar(&all, "all", false, "Include healthy devices, not only at-risk ones")
	cmd.Flags().BoolVar(&noUnmanaged, "no-unmanaged", false, "Skip discovered devices that have no endpoint agent")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum device rows to return (0 = all)")
	return cmd
}

func renderPosture(cmd *cobra.Command, flags *rootFlags, view postureView) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), view, flags)
	}
	if len(view.Devices) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No at-risk endpoints across %d account(s), %d device(s) scanned.\n", view.AccountsChecked, view.DevicesScanned)
		return nil
	}
	tw := newTabWriter(cmd.OutOrStdout())
	fmt.Fprintln(tw, "ACCOUNT\tHOST\tISSUES")
	for _, r := range view.Devices {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", subscriber{ID: r.AccountID, Name: r.AccountName}.label(), r.HostName, strings.Join(r.Issues, "; "))
	}
	return tw.Flush()
}
