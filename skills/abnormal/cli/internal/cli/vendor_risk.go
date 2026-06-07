// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: vendor-email-compromise risk profile. Hand-authored; preserved across regenerations.

// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"abnormal-pp-cli/internal/cliutil"

	"github.com/spf13/cobra"
)

type vendorCaseRow struct {
	VendorCaseID      json.Number `json:"vendorCaseId"`
	VendorDomain      string      `json:"vendorDomain,omitempty"`
	FirstObservedTime string      `json:"firstObservedTime,omitempty"`
	LastModifiedTime  string      `json:"lastModifiedTime,omitempty"`
}

type vendorRiskFailure struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

type vendorRiskView struct {
	VendorDomain       string              `json:"vendorDomain"`
	Details            json.RawMessage     `json:"details,omitempty"`
	Activity           json.RawMessage     `json:"activity,omitempty"`
	OpenCases          []vendorCaseRow     `json:"open_cases"`
	ScannedVendorCases int                 `json:"scanned_vendor_cases"`
	MaxScanPages       int                 `json:"max_scan_pages"`
	Note               string              `json:"note,omitempty"`
	FetchFailures      []vendorRiskFailure `json:"fetch_failures,omitempty"`
}

func newNovelVendorRiskCmd(flags *rootFlags) *cobra.Command {
	var maxScanPages int

	cmd := &cobra.Command{
		Use:   "vendor-risk <vendor-domain>",
		Short: "One vendor-email-compromise picture per vendor: details, recent activity, open vendor cases",
		Long: strings.Trim(`
Use this command to assemble one vendor-email-compromise picture for a vendor
domain: vendor details, recent activity, and vendor cases naming the domain.
Do NOT use it for an account-takeover picture; use 'employee-risk'.

Vendor-case matching scans recent vendor cases for the domain;
scanned_vendor_cases reports how many were examined.`, "\n"),
		Example: strings.Trim(`
  abnormal-cli vendor-risk acme-supplies.com
  abnormal-cli vendor-risk acme-supplies.com --agent
  abnormal-cli vendor-risk acme-supplies.com --max-scan-pages 5`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "vendor-domain=acme-supplies.com",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) < 1 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("vendor-risk requires a vendor domain (e.g. acme-supplies.com)"))
			}
			domain := strings.ToLower(strings.TrimSpace(args[0]))
			if domain == "" || !strings.Contains(domain, ".") {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("%q does not look like a vendor domain", args[0]))
			}
			if flags.dataSource == "local" {
				return usageErr(fmt.Errorf("vendor-risk queries live vendor endpoints; no local data source"))
			}
			if dryRunOK(flags) {
				fmt.Fprintf(cmd.OutOrStdout(), "would join vendor details, activity, and vendor cases for %s\n", domain)
				return nil
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			esc := url.PathEscape(domain)
			view := vendorRiskView{VendorDomain: domain, OpenCases: make([]vendorCaseRow, 0), MaxScanPages: maxScanPages, FetchFailures: make([]vendorRiskFailure, 0)}
			var mu sync.Mutex
			var wg sync.WaitGroup
			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()
			fail := func(source string, err error) {
				mu.Lock()
				view.FetchFailures = append(view.FetchFailures, vendorRiskFailure{Source: source, Error: err.Error()})
				mu.Unlock()
			}
			wg.Add(2)
			go func() {
				defer wg.Done()
				data, err := c.Get(ctx, "/vendors/"+esc+"/details", nil)
				if err != nil {
					fail("details", err)
					return
				}
				mu.Lock()
				view.Details = data
				mu.Unlock()
			}()
			go func() {
				defer wg.Done()
				data, err := c.Get(ctx, "/vendors/"+esc+"/activity", nil)
				if err != nil {
					fail("activity", err)
					return
				}
				mu.Lock()
				view.Activity = data
				mu.Unlock()
			}()
			wg.Wait()

			// Scan recent vendor cases for this domain (no domain filter upstream).
			pages := maxScanPages
			if cliutil.IsDogfoodEnv() && pages > 1 {
				pages = 1
			}
			scanCapHit := true
			for page := 1; page <= pages; page++ {
				data, err := c.Get(ctx, "/vendor-cases", map[string]string{
					"pageSize":   "100",
					"pageNumber": strconv.Itoa(page),
				})
				if err != nil {
					fail("vendor-cases", err)
					scanCapHit = false // failure, not a cap — fetch_failures carries the cause
					break
				}
				var pageDoc struct {
					VendorCases    []vendorCaseRow `json:"vendorCases"`
					NextPageNumber *int            `json:"nextPageNumber"`
				}
				if err := json.Unmarshal(data, &pageDoc); err != nil {
					fail("vendor-cases", fmt.Errorf("parsing vendor cases page %d: %w", page, err))
					scanCapHit = false
					break
				}
				for _, vc := range pageDoc.VendorCases {
					view.ScannedVendorCases++
					if strings.EqualFold(strings.TrimSpace(vc.VendorDomain), domain) {
						view.OpenCases = append(view.OpenCases, vc)
					}
				}
				if pageDoc.NextPageNumber == nil || len(pageDoc.VendorCases) == 0 {
					scanCapHit = false
					break
				}
			}
			if len(view.OpenCases) == 0 && scanCapHit {
				view.Note = fmt.Sprintf("scanned %d recent vendor cases without a match for %s; raise --max-scan-pages to widen the search", view.ScannedVendorCases, domain)
			}
			if len(view.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of 3 sources failed; profile assembled from the remaining sources\n", len(view.FetchFailures))
			}
			if view.Details == nil && view.Activity == nil && len(view.OpenCases) == 0 {
				return classifyAPIError(fmt.Errorf("all vendor sources failed for %s; first error: %s", domain, view.FetchFailures[0].Error), flags)
			}
			return printJSONFiltered(cmd.OutOrStdout(), view, flags)
		},
	}
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 3, "Maximum vendor-case pages (100 cases each) to scan for this vendor")
	return cmd
}
