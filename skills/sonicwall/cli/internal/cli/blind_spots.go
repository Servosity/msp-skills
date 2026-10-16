// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"sonicwall-pp-cli/internal/client"
	"sonicwall-pp-cli/internal/posture"
)

// mswProduct is one content.prodList entry of MySonicWall POST /api/users/productlist-customer.
type mswProduct struct {
	SerNo           string          `json:"serNo"`
	FriendlyName    string          `json:"friendlyName"`
	ProductName     string          `json:"productName"`
	ProductNameRaw  string          `json:"productNameRaw"`
	ProductLine     string          `json:"productLine"`
	ProductType     string          `json:"productType"`
	ProdLineDesc    string          `json:"prodLineDesc"`
	FirmwareVersion string          `json:"firmwareVersion"`
	SupportExpiry   string          `json:"supportExpiry"`
	SupportExpired  json.RawMessage `json:"isSupportExpired"` // string, boolean or null
	ProductGroup    string          `json:"productGroup"`
	OrgName         string          `json:"OrgName"`
}

type blindSpotRow struct {
	Serial         string   `json:"serial"`
	Name           string   `json:"name"`
	Product        string   `json:"product"`
	Tenant         string   `json:"msw_tenant,omitempty"`
	Firmware       string   `json:"registered_firmware,omitempty"`
	SupportExpiry  string   `json:"support_expiry,omitempty"`
	SupportExpired *bool    `json:"support_expired"` // null when neither the flag nor the date says
	Advisories     []string `json:"advisories"`
	AdvisoryStatus string   `json:"advisory_status"` // fail, pass or unknown
	ProductClass   string   `json:"product_class"`   // firewall, other (a known non-firewall family), or unknown (unnamed or unrecognised: listed, never assumed not a firewall)
}

// firewallProduct matches every SonicWall firewall family by its family word
// (TZ, NSa, NSsp, NSv, SOHO, SuperMassive, the NSA E-Class and PRO series) or
// by the line description naming a firewall. A family word may be followed by
// a space, a model number or nothing ("TZ 370", "TZ370W", "NSA E7500", "NSa Series").
var firewallProduct = regexp.MustCompile(`(?i)\b(tz|nsa|nssp|nsv|soho|supermassive|pro)(\b|[0-9])|\bfirewall`)

// otherProduct matches the SonicWall product families that are not firewalls:
// Secure Mobile Access / SRA, SonicWave and SonicPoint access points, SWS
// switches, the Capture Security Appliance, Email Security, Capture Client,
// Cloud App Security, Cloud Edge, WXA, and the GMS / NSM / Analytics
// management products sold as their own registrations.
var otherProduct = regexp.MustCompile(`(?i)\b(sma|sra|sonicwave|sonicpoint|access point|sws|switch|csa|capture security appliance|email security|esa|capture client|cloud app security|cloud edge|wxa|wan acceleration|gms|nsm|analytics)(\b|[0-9])`)

// productClass is "firewall" when a product name or line names a firewall
// family, "other" only when they name a known non-firewall family, and
// "unknown" otherwise: a product MySonicWall left unnamed, or a model this
// build does not recognise, is listed, never assumed not a firewall.
// ponytail: family-word heuristic over MySonicWall's free-text product fields; --all-products bypasses it.
func productClass(p mswProduct) string {
	text := strings.TrimSpace(strings.Join([]string{p.ProductName, p.ProductNameRaw, p.ProductLine, p.ProductType, p.ProdLineDesc}, " "))
	switch {
	case firewallProduct.MatchString(text):
		return "firewall"
	case otherProduct.MatchString(text):
		return "other"
	}
	return "unknown"
}

const (
	mswProductPageSize = 500
	mswMaxProductPages = 100
)

// fetchMSWProducts pages through the MySonicWall product list. An HTTP-200
// application failure, a missing product list, or hitting the page cap is an
// error: blind-spots must never conclude "every firewall is visible" from a
// list it could not read in full.
func fetchMSWProducts(ctx context.Context, c *client.Client) ([]mswProduct, error) {
	out := make([]mswProduct, 0)
	seen := map[string]bool{}
	expected := 0 // the largest product count MySonicWall reported
	for page := 1; ; page++ {
		if page > mswMaxProductPages {
			return nil, fmt.Errorf("MySonicWall returned more than %d products; refusing to compare a truncated registry", mswProductPageSize*mswMaxProductPages)
		}
		raw, _, err := c.Post(ctx, mswBase+"/api/users/productlist-customer", map[string]any{
			"isPaged": "true", "pageSize": mswProductPageSize, "pageNumber": page,
		})
		if err != nil {
			return nil, err
		}
		if err := mswStatusError(raw); err != nil {
			return nil, fmt.Errorf("productlist-customer: %w", err)
		}
		var doc struct {
			Content *struct {
				TotalProducts     json.RawMessage `json:"totalProducts"`
				FilterResultCount json.RawMessage `json:"filterResultCount"`
				ProdList          *[]mswProduct   `json:"prodList"`
			} `json:"content"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, fmt.Errorf("parsing productlist-customer: %w", err)
		}
		if doc.Content == nil || doc.Content.ProdList == nil {
			// Only an explicit zero total proves an empty registry; anything else is unreadable.
			if page == 1 && doc.Content != nil && strings.Trim(strings.TrimSpace(string(doc.Content.TotalProducts)), `"`) == "0" {
				break
			}
			return nil, fmt.Errorf("parsing productlist-customer: no readable content.prodList in the response")
		}
		added := 0
		for _, p := range *doc.Content.ProdList {
			key := strings.ToUpper(strings.TrimSpace(p.SerNo))
			if key == "" {
				return nil, fmt.Errorf("parsing productlist-customer: product without serNo")
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, p)
			added++
		}
		if reported, ok := mswReportedCount(doc.Content.FilterResultCount, doc.Content.TotalProducts); ok && reported > expected {
			expected = reported
		}
		if len(*doc.Content.ProdList) < mswProductPageSize {
			break
		}
		if added == 0 {
			return nil, fmt.Errorf("MySonicWall ignored paging at %d products; refusing to compare a possibly truncated registry", len(out))
		}
	}
	if len(out) < expected {
		return nil, fmt.Errorf("MySonicWall reported %d products but only %d could be read; refusing to compare a truncated registry", expected, len(out))
	}
	return out, nil
}

// mswReportedCount reads the first numeric count MySonicWall reports
// (filterResultCount, then totalProducts); both arrive as strings.
func mswReportedCount(fields ...json.RawMessage) (int, bool) {
	for _, f := range fields {
		v := strings.Trim(strings.TrimSpace(string(f)), `"`)
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return n, true
		}
	}
	return 0, false
}

func newNovelBlindSpotsCmd(flags *rootFlags) *cobra.Command {
	var allProducts bool
	cmd := &cobra.Command{
		Use:   "blind-spots",
		Short: "Find firewalls registered in MySonicWall that NSM cannot see, with their registered firmware's advisories and support expiry; needs SONICWALL_MSW_API_KEY and an NSM token",
		Long: "Use this command to find firewalls registered in MySonicWall that NSM cannot see (posture blind spots), with\n" +
			"their registered firmware's advisories and support expiry. Do NOT use this command for the posture of\n" +
			"NSM-managed firewalls; use 'posture' instead.\n\n" +
			"Every other posture command only sees firewalls onboarded to NSM SaaS. This lists registered firewall\n" +
			"serials missing from NSM's device list, then asks NSM which advisories apply to each one's registered\n" +
			"firmware build. A product that is neither a known firewall family nor a known non-firewall family\n" +
			"(unnamed, or a model this build does not recognise) is listed with product_class unknown, and the\n" +
			"command exits 6 when NSM firewalls are missing from the registry the key reads (the comparison is then\n" +
			"incomplete). Needs SONICWALL_MSW_API_KEY and an NSM token.",
		Example: strings.Trim(`
  sonicwall-cli blind-spots --agent
  sonicwall-cli blind-spots --all-products --json`, "\n"),
		Annotations: map[string]string{"pp:data-source": "live", "mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "blind-spots")
			}
			if !mswKeyConfigured(flags) {
				return authErr(fmt.Errorf("blind-spots reads the MySonicWall product registry: set SONICWALL_MSW_API_KEY"))
			}
			ctx := cmd.Context()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			products, err := fetchMSWProducts(ctx, c)
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			devices, err := fetchFleet(ctx, c, "", "")
			if err != nil {
				return classifyReadError(cmd.OutOrStdout(), err, flags)
			}
			inNSM := map[string]bool{}
			for _, d := range devices {
				inNSM[strings.ToUpper(strings.TrimSpace(d.SerialNumber))] = true
			}
			rows := make([]blindSpotRow, 0)
			advisoryCache := map[string][]string{}
			advisoryErr := map[string]bool{}
			for _, p := range products {
				serial := strings.ToUpper(strings.TrimSpace(p.SerNo))
				class := productClass(p)
				if serial == "" || inNSM[serial] || (!allProducts && class == "other") {
					continue
				}
				product := p.ProductName
				if strings.TrimSpace(product) == "" {
					product = p.ProductNameRaw
				}
				r := blindSpotRow{Serial: serial, Name: p.FriendlyName, Product: product, Tenant: p.ProductGroup, ProductClass: class,
					Firmware: p.FirmwareVersion, SupportExpiry: p.SupportExpiry, Advisories: make([]string, 0),
					SupportExpired: supportExpiredState(p.SupportExpired, p.SupportExpiry, time.Now())}
				fw := strings.TrimSpace(p.FirmwareVersion)
				switch {
				case fw == "":
					r.AdvisoryStatus = "unknown"
				default:
					if _, seen := advisoryCache[fw]; !seen && !advisoryErr[fw] {
						if advs, aerr := advisoriesForFirmware(ctx, c, fw); aerr != nil {
							advisoryErr[fw] = true
						} else {
							advisoryCache[fw] = advisoryIDs(advs)
						}
					}
					if advisoryErr[fw] {
						r.AdvisoryStatus = "unknown"
					} else {
						r.Advisories = advisoryCache[fw]
						switch {
						case len(r.Advisories) > 0:
							r.AdvisoryStatus = "fail"
						case class == "firewall":
							r.AdvisoryStatus = "pass"
						default:
							r.AdvisoryStatus = "unknown" // a SonicOS advisory lookup proves nothing about a non-firewall
						}
					}
				}
				rows = append(rows, r)
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Serial < rows[j].Serial })
			// Every NSM firewall is registered in MySonicWall, so an NSM serial
			// the registry lacks proves this key reads only part of it: blind
			// spots outside that part cannot be ruled out.
			registered := map[string]bool{}
			for _, p := range products {
				registered[strings.ToUpper(strings.TrimSpace(p.SerNo))] = true
			}
			unregistered := 0
			for serial := range inNSM {
				if !registered[serial] {
					unregistered++
				}
			}
			var partial error
			if unregistered > 0 {
				note := fmt.Sprintf("%d of %d NSM firewalls are not in the MySonicWall registry this key reads, so the comparison is incomplete", unregistered, len(inNSM))
				fmt.Fprintln(cmd.ErrOrStderr(), "warning: "+note)
				partial = partialFailureErr(errors.New(note))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				if err := printJSONFiltered(cmd.OutOrStdout(), rows, flags); err != nil {
					return err
				}
				return partial
			}
			if len(rows) == 0 {
				if partial == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Every registered firewall (%d registered products checked) is visible in NSM.\n", len(products))
				}
				return partial
			}
			table := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				table = append(table, map[string]any{"serial": r.Serial, "name": r.Name, "product": r.Product, "class": r.ProductClass, "firmware": r.Firmware,
					"advisory_status": r.AdvisoryStatus, "advisories": strings.Join(r.Advisories, ","), "support_expiry": r.SupportExpiry})
			}
			if err := printOrderedTable(cmd.OutOrStdout(), []string{"serial", "name", "product", "class", "firmware", "advisory_status", "advisories", "support_expiry"}, table); err != nil {
				return err
			}
			return partial
		},
	}
	cmd.Flags().BoolVar(&allProducts, "all-products", false, "Include every registered product, not only firewalls")
	return cmd
}

// supportExpiredState combines MySonicWall's isSupportExpired flag (a string,
// a boolean or null) with the supportExpiry date. Either one saying expired
// wins; a confirmed "not expired" needs the flag or a future date; with
// neither reported the answer is unknown (nil), never "not expired".
func supportExpiredState(flag json.RawMessage, expiry string, now time.Time) *bool {
	expired, current := false, false
	var v any
	if json.Unmarshal(flag, &v) == nil {
		switch t := v.(type) {
		case bool:
			expired, current = t, !t
		case string:
			switch strings.ToLower(strings.TrimSpace(t)) {
			case "true", "yes", "1":
				expired = true
			case "false", "no", "0":
				current = true
			}
		}
	}
	if d, ok := posture.ParseLifecycleDate(expiry); ok {
		if d.Before(now) {
			expired = true
		} else {
			current = true
		}
	}
	switch {
	case expired:
		return &expired
	case current:
		no := false
		return &no
	}
	return nil
}
