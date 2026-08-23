// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel command. Preserved across `generate --force`.
// pp:data-source live

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type exposureRow struct {
	ClientUUID      string  `json:"client_uuid"`
	Client          string  `json:"client"`
	DeviceUUID      string  `json:"device_uuid"`
	Device          string  `json:"device"`
	SwVendor        string  `json:"sw_vendor"`
	SwProduct       string  `json:"sw_product"`
	ImpactedVersion string  `json:"impacted_version"`
	CVSS            float64 `json:"cvss"`
	EPSS            float64 `json:"epss"`
	IsKEV           bool    `json:"is_kev"`
	Priority        string  `json:"priority"`
}

type exposureView struct {
	CVE             string        `json:"cve"`
	Exposures       []exposureRow `json:"exposures"`
	ImpactedClients int           `json:"impacted_clients"`
	ImpactedDevices int           `json:"impacted_devices"`
	ScannedVulns    int           `json:"scanned_vulnerabilities"`
	PagesRead       int           `json:"pages_read"`
	MaxScanPages    int           `json:"max_scan_pages"`
	ScanCapHit      bool          `json:"scan_cap_hit"`
	Note            string        `json:"note,omitempty"`
}

func newNovelVulnerabilitiesExposureCmd(flags *rootFlags) *cobra.Command {
	var flagMaxScanPages int
	var flagClientUUID string
	var flagLimit int
	var flagDB string

	cmd := &cobra.Command{
		Use:   "exposure [cve-id]",
		Short: "List every affected client, device, product, and version for a single CVE id.",
		Long: "List every affected client, device, product, and version for a single CVE id.\n\n" +
			"Cork exposes CVE ids only nested inside each vulnerability row and offers no\n" +
			"CVE filter on any endpoint, so this question cannot be asked upstream at any\n" +
			"page size. This command scans the vulnerability collection and matches\n" +
			"locally, resolving client and device UUIDs against the local mirror.\n\n" +
			"Use this command when a specific CVE is named and you need the list of\n" +
			"affected clients and devices. Do NOT use this command to build a general\n" +
			"patch queue with no CVE in hand; use 'vulnerabilities triage' instead.",
		Example: "  cork-cli vulnerabilities exposure CVE-2026-1234 --agent",
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "cve-id=CVE-2026-1234",
			// A CVE the fleet is not exposed to is a legitimate, useful answer,
			// not an error; exit 3 carries that without implying failure.
			"pp:typed-exit-codes": "0,3",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "vulnerabilities exposure")
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a CVE id is required, for example CVE-2026-1234"))
			}
			want := strings.ToUpper(strings.TrimSpace(args[0]))
			if !strings.HasPrefix(want, "CVE-") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("%q does not look like a CVE id; expected the form CVE-2026-1234", args[0]))
			}

			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			params := map[string]string{}
			if flagClientUUID != "" {
				params["client_uuid"] = flagClientUUID
			}
			raw, pages, capHit, err := corkFetchPages(ctx, c, "/vulnerabilities/software", params, flagMaxScanPages)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			clientNames := map[string]string{}
			deviceNames := map[string]string{}
			if db, ok, openErr := corkOpenStore(ctx, flagDB, cmd.ErrOrStderr(), cmd.OutOrStdout(), "clients"); openErr == nil && ok {
				defer db.Close()
				if clients, loadErr := corkLoadClients(ctx, db); loadErr == nil {
					clientNames = corkClientNames(clients)
				}
				if dn, devErr := corkDeviceNames(ctx, db); devErr == nil {
					deviceNames = dn
				}
			}

			rows := make([]exposureRow, 0)
			clientSet := map[string]struct{}{}
			deviceSet := map[string]struct{}{}
			scanned := 0
			undecoded := 0
			for _, r := range raw {
				var v corkVuln
				if json.Unmarshal(r, &v) != nil {
					undecoded++
					continue
				}
				scanned++
				for _, cve := range v.CVEs {
					if !strings.EqualFold(strings.TrimSpace(cve.CVEID), want) {
						continue
					}
					rows = append(rows, exposureRow{
						ClientUUID:      v.ClientUUID,
						Client:          corkResolve(clientNames, v.ClientUUID),
						DeviceUUID:      v.DeviceUUID,
						Device:          corkResolve(deviceNames, v.DeviceUUID),
						SwVendor:        v.SwVendor,
						SwProduct:       v.SwProduct,
						ImpactedVersion: cve.ImpactedVersion,
						CVSS:            cve.CVSS,
						EPSS:            cve.EPSS,
						IsKEV:           cve.IsKEV,
						Priority:        cve.Priority,
					})
					if v.ClientUUID != "" {
						clientSet[v.ClientUUID] = struct{}{}
					}
					if v.DeviceUUID != "" {
						deviceSet[v.DeviceUUID] = struct{}{}
					}
				}
			}

			// "Not exposed" is only a safe thing to say if the rows were actually
			// read. An all-undecodable scan must error, not return a reassuring
			// exit 3.
			if scanned == 0 && undecoded > 0 {
				return corkDecodeFailure("vulnerability row(s)", undecoded)
			}
			if undecoded > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d vulnerability row(s) could not be decoded and were not searched for %s\n", undecoded, want)
			}

			corkSortStable(rows, func(a, b exposureRow) bool {
				if a.Client != b.Client {
					return a.Client < b.Client
				}
				return a.Device < b.Device
			})
			if flagLimit > 0 && len(rows) > flagLimit {
				rows = rows[:flagLimit]
			}

			view := exposureView{
				CVE:             want,
				Exposures:       rows,
				ImpactedClients: len(clientSet),
				ImpactedDevices: len(deviceSet),
				ScannedVulns:    scanned,
				PagesRead:       pages,
				MaxScanPages:    flagMaxScanPages,
				ScanCapHit:      capHit,
			}

			if len(rows) == 0 {
				view.Note = fmt.Sprintf("no exposure to %s found across %d vulnerability rows in %d page(s)", want, scanned, pages)
				if capHit {
					view.Note += "; the scan cap was reached, so this is not a clean bill of health — raise --max-scan-pages to widen the search"
				}
				if undecoded > 0 {
					view.Note += fmt.Sprintf("; %d row(s) could not be decoded and were not searched", undecoded)
				}
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					if err := printJSONFiltered(cmd.OutOrStdout(), view, flags); err != nil {
						return err
					}
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				}
				// Exit 3 distinguishes "scanned and found nothing" from "failed".
				return notFoundErr(fmt.Errorf("no exposure to %s", want))
			}
			if capHit {
				view.Note = fmt.Sprintf("scan stopped at the --max-scan-pages cap of %d; additional exposure may exist", flagMaxScanPages)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			tw := newTabWriter(cmd.OutOrStdout())
			fmt.Fprintln(tw, "CLIENT\tDEVICE\tVENDOR\tPRODUCT\tVERSION\tCVSS\tEPSS\tKEV")
			for _, r := range rows {
				kev := ""
				if r.IsKEV {
					kev = "yes"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%.1f\t%.3f\t%s\n",
					truncate(r.Client, 28), truncate(r.Device, 24), truncate(r.SwVendor, 20),
					truncate(r.SwProduct, 24), r.ImpactedVersion, r.CVSS, r.EPSS, kev)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%s: %d device(s) across %d client(s)\n", want, view.ImpactedDevices, view.ImpactedClients)
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&flagMaxScanPages, "max-scan-pages", corkDefaultScanPages, "Maximum vulnerability pages to scan before returning partial results")
	cmd.Flags().StringVar(&flagClientUUID, "client-uuid", "", "Restrict the scan to one client")
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "Maximum rows to return (0 for all)")
	cmd.Flags().StringVar(&flagDB, "db", "", "Database path used to resolve client and device names")
	return cmd
}
