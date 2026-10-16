// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"fmt"
	"strings"

	"bitdefender-pp-cli/internal/client"

	"github.com/spf13/cobra"
)

type fleetFindView struct {
	Query            string         `json:"query"`
	Matches          []snapEndpoint `json:"matches"`
	Source           string         `json:"source"`
	TakenAt          string         `json:"taken_at,omitempty"`
	ScannedCompanies int            `json:"scanned_companies"`
	FetchFailures    []fetchFailure `json:"fetch_failures"`
	Note             string         `json:"note,omitempty"`
}

func newNovelFleetLocateCmd(flags *rootFlags) *cobra.Command {
	var maxPages, limit int
	cmd := &cobra.Command{
		Use:   "locate [hostname|ip|mac|label]",
		Short: "Find a machine by hostname, IP, MAC or label across every child company",
		Long: `Resolves a machine to its GravityZone endpoint id and company across every child company.
Hostname and label match as case-insensitive substrings (also against the FQDN), an IPv4
address matches exactly, and a MAC address matches in any notation. By default it reads the
latest 'fleet health' snapshot and falls back to a live fan-out over every company when
none exists; --data-source live always fans out live (and saves the result as a snapshot);
--data-source local reads the snapshot only. Matching is always done locally so names,
FQDNs, labels, IPs and MACs in any notation are all searched.

Use this command to locate a machine by hostname, IP, MAC or label across every child company
and get its endpoint id and company. Do NOT use this command for per-company status counts;
use 'fleet health' instead.`,
		Example:     "  bitdefender-cli fleet locate LAPTOP-7 --agent\n  bitdefender-cli fleet locate 00:1A:2B:3C:4D:5E --data-source live --json",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto", "pp:happy-args": "query=LAPTOP-7"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "fleet locate")
			}
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return fleetUsageErr(cmd, flags, fmt.Errorf("pass exactly one hostname, IP, MAC or label"))
			}
			q := strings.TrimSpace(args[0])
			// No whole-command deadline: a fan-out over many companies runs far
			// longer than --timeout, which the generated client applies per request.
			ctx := cmd.Context()
			s, err := openGZStore(ctx)
			if err != nil {
				return err
			}
			defer s.Close()
			inv, err := fleetInventory(ctx, func() (*client.Client, error) { return flags.newClient() }, s.DB(), flags.dataSource, flags.maxAge, maxPages)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			view := fleetFindView{Query: q, Matches: make([]snapEndpoint, 0), Source: inv.Source, TakenAt: inv.TakenAt, ScannedCompanies: inv.Companies, FetchFailures: make([]fetchFailure, 0)}
			view.FetchFailures = append(view.FetchFailures, inv.Failures...)
			for _, e := range inv.Endpoints {
				if endpointMatches(e, q) {
					view.Matches = append(view.Matches, e)
					if limit > 0 && len(view.Matches) >= limit {
						break
					}
				}
			}
			switch {
			case inv.Note != "":
				view.Note = inv.Note
			case inv.Source == "snapshot" && inv.TakenAt == "":
				view.Note = "no fleet snapshot yet; run 'bitdefender-cli fleet health' or use --data-source live"
			case len(view.Matches) == 0 && inv.Source == "snapshot":
				view.Note = "no match in the snapshot from " + inv.TakenAt + "; try --data-source live"
			case inv.Stale:
				view.Note = "snapshot from " + inv.TakenAt + " is older than --max-age; run 'fleet health' to refresh"
			}
			if !inv.Complete && inv.TakenAt != "" {
				view.Note = strings.TrimSpace(view.Note + " inventory is incomplete (see fetch_failures): a machine in an unfetched company would not be found.")
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			humanNote(cmd, view.Note)
			if len(view.Matches) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No match for "+q+".")
				return nil
			}
			rows := make([]map[string]any, 0, len(view.Matches))
			for _, e := range view.Matches {
				rows = append(rows, map[string]any{"name": e.Name, "company": e.CompanyName, "ip": e.IP, "managed": e.IsManaged,
					"isolated": e.IsIsolated, "policy": e.PolicyName, "endpoint_id": e.EndpointID})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().IntVar(&maxPages, "max-pages", 20, "Maximum getEndpointsList pages (1000 endpoints each) per company on a live search")
	cmd.Flags().IntVar(&limit, "limit", 50, "Return at most this many matches (0 = all)")
	return cmd
}

func endpointMatches(e snapEndpoint, q string) bool {
	lq := strings.ToLower(q)
	if net4(q) != "" {
		return e.IP == q
	}
	if mq := normalizeMAC(q); mq != "" {
		for _, m := range e.MACs {
			if normalizeMAC(m) == mq {
				return true
			}
		}
		return false
	}
	for _, field := range []string{e.Name, e.FQDN, e.Label} {
		if field != "" && strings.Contains(strings.ToLower(field), lq) {
			return true
		}
	}
	return false
}
