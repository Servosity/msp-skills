// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// corkCVE mirrors the nested CVE object on a software vulnerability row.
type corkCVE struct {
	CVEID           string  `json:"cve_id"`
	CVSS            float64 `json:"cvss"`
	EPSS            float64 `json:"epss"`
	ImpactedVersion string  `json:"impacted_version"`
	IsKEV           bool    `json:"is_kev"`
	Priority        string  `json:"priority"`
}

// corkVuln mirrors a /vulnerabilities/software row. It carries no id of its
// own, only the client and device it belongs to, which is why this resource
// cannot be mirrored into the generic local resources table.
type corkVuln struct {
	ClientUUID string    `json:"client_uuid"`
	DeviceUUID string    `json:"device_uuid"`
	SwProduct  string    `json:"sw_product"`
	SwVendor   string    `json:"sw_vendor"`
	CVEs       []corkCVE `json:"cves"`
}

type triageRow struct {
	SwVendor        string   `json:"sw_vendor"`
	SwProduct       string   `json:"sw_product"`
	TopCVE          string   `json:"top_cve"`
	TopPriority     string   `json:"top_priority"`
	MaxCVSS         float64  `json:"max_cvss"`
	MaxEPSS         float64  `json:"max_epss"`
	KEVCount        int      `json:"kev_count"`
	CVECount        int      `json:"cve_count"`
	ImpactedDevices int      `json:"impacted_devices"`
	ImpactedClients int      `json:"impacted_clients"`
	Clients         []string `json:"clients"`
}

type triageView struct {
	Items         []triageRow `json:"items"`
	TotalProducts int         `json:"total_products"`
	ScannedVulns  int         `json:"scanned_vulnerabilities"`
	UndecodedRows int         `json:"undecoded_rows"`
	PagesRead     int         `json:"pages_read"`
	MaxScanPages  int         `json:"max_scan_pages"`
	ScanCapHit    bool        `json:"scan_cap_hit"`
	Note          string      `json:"note,omitempty"`
}

func newNovelVulnerabilitiesTriageCmd(flags *rootFlags) *cobra.Command {
	var flagKevOnly bool
	var flagMinEpss float64
	var flagMinCvss float64
	var flagMinPriority string
	var flagClientUUID string
	var flagLimit int
	var flagMaxScanPages int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "triage",
		Short: "Rank software products by exploitability (KEV, then EPSS, then CVSS) with a blast-radius device count",
		Long: "Rank vulnerabilities across all clients by known-exploited status, then EPSS,\n" +
			"then CVSS, rolled up per software product with a blast-radius device count.\n\n" +
			"Cork's API can only sort by software vendor or product, so exploitability\n" +
			"ranking cannot be requested upstream. Vulnerability rows also carry bare\n" +
			"client UUIDs, which this command resolves against the local mirror.\n\n" +
			"Use this command to build a ranked patch queue across clients. Do NOT use this\n" +
			"command to answer \"are we exposed to a specific CVE\"; use\n" +
			"'vulnerabilities exposure' instead.",
		Example: "  cork-cli vulnerabilities triage --kev-only --min-epss 0.3 --limit 25 --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "vulnerabilities triage")
			}
			if flagMinPriority != "" {
				switch flagMinPriority {
				case "critical", "accelerated", "routine":
				default:
					_ = cmd.Usage()
					return usageErr(fmt.Errorf("invalid --min-priority %q: must be one of critical, accelerated, routine", flagMinPriority))
				}
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			// Push every filter the API does support upstream; only the ranking
			// has to happen locally.
			params := map[string]string{}
			if flagKevOnly {
				params["only_known_exploited"] = "true"
			}
			if flagMinEpss > 0 {
				params["minimum_epss_score"] = strconv.FormatFloat(flagMinEpss, 'f', -1, 64)
			}
			if flagMinCvss > 0 {
				params["minimum_cvss_score"] = strconv.FormatFloat(flagMinCvss, 'f', -1, 64)
			}
			if flagMinPriority != "" {
				params["minimum_priority"] = flagMinPriority
			}
			if flagClientUUID != "" {
				params["client_uuid"] = flagClientUUID
			}

			raw, pages, capHit, err := corkFetchPages(ctx, c, "/vulnerabilities/software", params, flagMaxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			// Resolve UUIDs to names from the local mirror when it exists. A
			// missing mirror is not fatal: the ranking still works and names
			// degrade to short uuids.
			names := map[string]string{}
			if db, ok, openErr := corkOpenStore(ctx, flagDB, cmd.ErrOrStderr(), cmd.OutOrStdout(), "clients"); openErr == nil && ok {
				defer db.Close()
				if clients, loadErr := corkLoadClients(ctx, db); loadErr == nil {
					names = corkClientNames(clients)
				}
			}

			type agg struct {
				row      triageRow
				devices  map[string]struct{}
				clients  map[string]struct{}
				cves     map[string]struct{}
				kevCVEs  map[string]struct{}
				topIsKEV bool
				topEPSS  float64
				topCVSS  float64
			}
			byProduct := map[string]*agg{}
			scanned := 0
			undecoded := 0
			for _, r := range raw {
				var v corkVuln
				if json.Unmarshal(r, &v) != nil {
					undecoded++
					continue
				}
				scanned++
				key := strings.ToLower(v.SwVendor + "\x00" + v.SwProduct)
				a, ok := byProduct[key]
				if !ok {
					a = &agg{
						row:     triageRow{SwVendor: v.SwVendor, SwProduct: v.SwProduct},
						devices: map[string]struct{}{},
						clients: map[string]struct{}{},
						cves:    map[string]struct{}{},
						kevCVEs: map[string]struct{}{},
					}
					byProduct[key] = a
				}
				if v.DeviceUUID != "" {
					a.devices[v.DeviceUUID] = struct{}{}
				}
				if v.ClientUUID != "" {
					a.clients[v.ClientUUID] = struct{}{}
				}
				for _, cve := range v.CVEs {
					if cve.CVEID != "" {
						a.cves[cve.CVEID] = struct{}{}
						if cve.IsKEV {
							// Count DISTINCT KEV CVEs. Counting occurrences would
							// report kev_count 40 for one CVE seen on 40 devices.
							a.kevCVEs[cve.CVEID] = struct{}{}
						}
					}
					if cve.CVSS > a.row.MaxCVSS {
						a.row.MaxCVSS = cve.CVSS
					}
					if cve.EPSS > a.row.MaxEPSS {
						a.row.MaxEPSS = cve.EPSS
					}
					// Pick the CVE that drives the ranking under the command's
					// stated contract: KEV outranks non-KEV outright, and only
					// within the same KEV class do EPSS then CVSS decide. A
					// non-KEV CVE must never displace a KEV one, whatever its
					// EPSS, and the result must not depend on array order.
					replace := false
					switch {
					case a.row.TopCVE == "":
						replace = true
					case cve.IsKEV && !a.topIsKEV:
						replace = true
					case cve.IsKEV == a.topIsKEV:
						if cve.EPSS > a.topEPSS || (cve.EPSS == a.topEPSS && cve.CVSS > a.topCVSS) {
							replace = true
						}
					}
					if replace {
						a.row.TopCVE = cve.CVEID
						a.row.TopPriority = cve.Priority
						a.topIsKEV = cve.IsKEV
						a.topEPSS = cve.EPSS
						a.topCVSS = cve.CVSS
					}
				}
			}

			rows := make([]triageRow, 0, len(byProduct))
			for _, a := range byProduct {
				a.row.ImpactedDevices = len(a.devices)
				a.row.ImpactedClients = len(a.clients)
				a.row.CVECount = len(a.cves)
				a.row.KEVCount = len(a.kevCVEs)
				cl := make([]string, 0, len(a.clients))
				for cu := range a.clients {
					cl = append(cl, corkResolve(names, cu))
				}
				corkSortStable(cl, func(x, y string) bool { return x < y })
				a.row.Clients = cl
				rows = append(rows, a.row)
			}

			// Exploitability-first ordering: KEV presence, then Cork priority,
			// then EPSS, then CVSS, then blast radius. This is precisely the
			// ordering the API cannot express.
			corkSortStable(rows, func(a, b triageRow) bool {
				aKEV, bKEV := a.KEVCount > 0, b.KEVCount > 0
				if aKEV != bKEV {
					return aKEV
				}
				if pa, pb := corkPriorityRank(a.TopPriority), corkPriorityRank(b.TopPriority); pa != pb {
					return pa < pb
				}
				if a.MaxEPSS != b.MaxEPSS {
					return a.MaxEPSS > b.MaxEPSS
				}
				if a.MaxCVSS != b.MaxCVSS {
					return a.MaxCVSS > b.MaxCVSS
				}
				return a.ImpactedDevices > b.ImpactedDevices
			})

			// A page of rows that all failed to decode is a read failure, not an
			// empty patch queue.
			if scanned == 0 && undecoded > 0 {
				return corkDecodeFailure("vulnerability row(s)", undecoded)
			}
			if undecoded > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d vulnerability row(s) could not be decoded and were not ranked\n", undecoded)
			}

			total := len(rows)
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			view := triageView{
				Items:         rows,
				TotalProducts: total,
				ScannedVulns:  scanned,
				UndecodedRows: undecoded,
				PagesRead:     pages,
				MaxScanPages:  flagMaxScanPages,
				ScanCapHit:    capHit || undecoded > 0,
			}
			switch {
			case total == 0:
				view.Note = fmt.Sprintf("scanned %d vulnerability rows across %d page(s) with no match", scanned, pages)
				if capHit {
					view.Note += "; the scan cap was reached, so raise --max-scan-pages to widen the search"
				}
			case total > len(rows):
				view.Note = fmt.Sprintf("showing %d of %d affected product(s); raise --limit to see more", len(rows), total)
				if capHit {
					view.Note += fmt.Sprintf(" (scan also stopped at the --max-scan-pages cap of %d)", flagMaxScanPages)
				}
			case capHit:
				view.Note = fmt.Sprintf("scan stopped at the --max-scan-pages cap of %d; results are partial", flagMaxScanPages)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(rows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "VENDOR\tPRODUCT\tTOP CVE\tKEV\tEPSS\tCVSS\tDEVICES\tCLIENTS")
			for _, r := range rows {
				kev := ""
				if r.KEVCount > 0 {
					kev = "yes"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%.3f\t%.1f\t%d\t%d\n",
					truncate(r.SwVendor, 24), truncate(r.SwProduct, 28), r.TopCVE, kev, r.MaxEPSS, r.MaxCVSS, r.ImpactedDevices, r.ImpactedClients)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "\n"+view.Note)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&flagKevOnly, "kev-only", false, "Only include CVEs on the known-exploited list")
	cmd.Flags().Float64Var(&flagMinEpss, "min-epss", 0, "Minimum EPSS exploitation probability (0-1)")
	cmd.Flags().Float64Var(&flagMinCvss, "min-cvss", 0, "Minimum CVSS base score (0-10)")
	cmd.Flags().StringVar(&flagMinPriority, "min-priority", "", "Minimum Cork priority (critical, accelerated, routine)")
	cmd.Flags().StringVar(&flagClientUUID, "client-uuid", "", "Restrict the queue to one client")
	cmd.Flags().IntVar(&flagLimit, "limit", 25, "Maximum products to return (0 for all)")
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum vulnerability pages to scan before returning partial results")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path used to resolve client names")
	return cmd
}
