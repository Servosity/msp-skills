// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: stale-licenses — paid Workspace licences held by
// suspended, never-used, or inactive accounts across client tenants.

// pp:data-source live

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"google-workspace-pp-cli/internal/client"
)

const licensingBase = "https://licensing.googleapis.com/apps/licensing/v1"

type licenseAssignment struct {
	UserID      string `json:"userId"`
	ProductID   string `json:"productId"`
	SkuID       string `json:"skuId"`
	SkuName     string `json:"skuName"`
	ProductName string `json:"productName"`
}

type staleLicense struct {
	Tenant       string `json:"tenant"`
	Email        string `json:"email"`
	Sku          string `json:"sku"`
	SkuID        string `json:"sku_id"`
	ProductID    string `json:"product_id"`
	Reason       string `json:"reason"`
	LastLogin    string `json:"last_login"`
	DaysInactive int    `json:"days_inactive"`
	Created      string `json:"created"`
}

// tenantCustomerID returns the real customer ID (Licensing rejects my_customer).
func tenantCustomerID(ctx context.Context, c *client.Client, t gwsTenant) (string, error) {
	if t.CustomerID != "" && t.CustomerID != "my_customer" {
		return t.CustomerID, nil
	}
	raw, err := c.Get(ctx, "/admin/directory/v1/customers/my_customer", nil)
	if err != nil {
		return "", fmt.Errorf("resolving customer ID: %w", err)
	}
	var cust struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &cust); err != nil || cust.ID == "" {
		return "", fmt.Errorf("resolving customer ID: no id in customer record")
	}
	return cust.ID, nil
}

func listLicenses(ctx context.Context, c *client.Client, customerID, productID string) ([]licenseAssignment, error) {
	path := licensingBase + "/product/" + url.PathEscape(productID) + "/users"
	raw, err := listAll(ctx, c, path, map[string]string{"customerId": customerID, "maxResults": "1000"}, "items", 0)
	if err != nil {
		return nil, err
	}
	out := make([]licenseAssignment, 0, len(raw))
	for _, r := range raw {
		var a licenseAssignment
		if err := json.Unmarshal(r, &a); err != nil {
			return nil, fmt.Errorf("decoding licence: %w", err)
		}
		out = append(out, a)
	}
	return out, nil
}

// classifyStale returns why a licensed account is wasted, or "" if it is in
// use. New accounts inside the grace window are never flagged.
func classifyStale(u dirUser, found bool, now time.Time, inactiveDays, graceDays int) (string, int) {
	if !found {
		return "no_matching_user", -1
	}
	if u.Suspended {
		return "suspended", daysSince(u.LastLoginTime, now)
	}
	if created, err := time.Parse(time.RFC3339, u.CreationTime); err == nil && now.Sub(created) < time.Duration(graceDays)*24*time.Hour {
		return "", 0
	}
	if neverSignedIn(u) {
		return "never_signed_in", -1
	}
	d := daysSince(u.LastLoginTime, now)
	if d >= inactiveDays {
		return fmt.Sprintf("inactive_%dd", inactiveDays), d
	}
	return "", d
}

// freeLicenseSKUs are the Google-Apps SKUs Google does not bill for (Licensing
// API "Products and SKUs": Nonprofits, Education Fundamentals and Gmail
// Only). The paid Nonprofit editions reuse the general SKU IDs, so they stay
// counted.
var freeLicenseSKUs = map[string]bool{
	"Google-Apps-For-Nonprofits": true,
	"Google-Apps-For-Education":  true,
	"1010070001":                 true,
	"1010070004":                 true,
}

func freeLicenseSKU(l licenseAssignment) bool {
	return freeLicenseSKUs[l.SkuID]
}

func staleLicensesFor(tenant string, licenses []licenseAssignment, users []dirUser, now time.Time, inactiveDays, graceDays int) []staleLicense {
	byEmail := make(map[string]dirUser, len(users))
	for _, u := range users {
		byEmail[strings.ToLower(u.PrimaryEmail)] = u
	}
	out := []staleLicense{}
	for _, l := range licenses {
		if freeLicenseSKU(l) {
			continue
		}
		u, ok := byEmail[strings.ToLower(l.UserID)]
		reason, days := classifyStale(u, ok, now, inactiveDays, graceDays)
		if reason == "" {
			continue
		}
		out = append(out, staleLicense{
			Tenant: tenant, Email: l.UserID, Sku: l.SkuName, SkuID: l.SkuID, ProductID: l.ProductID,
			Reason: reason, LastLogin: u.LastLoginTime, DaysInactive: days, Created: u.CreationTime,
		})
	}
	return out
}

func newNovelStaleLicensesCmd(flags *rootFlags) *cobra.Command {
	var ff fanoutFlags
	var inactiveDays, graceDays int
	var products string
	cmd := &cobra.Command{
		Use:   "stale-licenses",
		Short: "Paid licences held by suspended, never-used, or inactive accounts across client tenants",
		Long: "Joins each tenant's licence assignments (Enterprise License Manager) to its directory\n" +
			"users and flags licences on suspended accounts, accounts that never signed in, and\n" +
			"accounts idle past --inactive-days. New accounts inside --grace-days are skipped.\n" +
			"Use this command to find licence waste to reclaim (suspended, never-signed-in, or idle\n" +
			"accounts holding a paid licence). Do NOT use this command to check whether a leaver\n" +
			"still has access; use 'leavers' instead. Do NOT use it for 2SV coverage; use 'mfa-gaps' instead.",
		Example: "  google-workspace-cli stale-licenses --all-tenants\n" +
			"  google-workspace-cli stale-licenses --all-tenants --inactive-days 90 --agent --select results.tenant,results.email,results.sku,results.reason",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "list stale licences per tenant"); done {
				return err
			}
			if inactiveDays < 1 || graceDays < 0 {
				return usageErr(fmt.Errorf("--inactive-days must be >= 1 and --grace-days >= 0"))
			}
			now := time.Now().UTC()
			res, err := fanout(cmd, flags, &ff, func(ctx context.Context, t gwsTenant, c *client.Client) ([]staleLicense, error) {
				custID, err := tenantCustomerID(ctx, c, t)
				if err != nil {
					return nil, err
				}
				var licenses []licenseAssignment
				for _, p := range splitTenantCSV(products) {
					ls, err := listLicenses(ctx, c, custID, p)
					if err != nil {
						return nil, fmt.Errorf("licences for %s: %w", p, err)
					}
					licenses = append(licenses, ls...)
				}
				users, err := listUsers(ctx, c, t, "")
				if err != nil {
					return nil, err
				}
				return staleLicensesFor(t.Name, licenses, users, now, inactiveDays, graceDays), nil
			})
			if err != nil {
				return err
			}
			rows := res.Results
			sort.SliceStable(rows, func(i, j int) bool {
				if rows[i].Tenant != rows[j].Tenant {
					return rows[i].Tenant < rows[j].Tenant
				}
				return rows[i].DaysInactive > rows[j].DaysInactive
			})
			return emitFanout(cmd, flags, res, func(rs []staleLicense) ([]string, [][]string) {
				body := [][]string{}
				for _, r := range rs {
					body = append(body, []string{r.Tenant, r.Email, r.Sku, r.Reason, itoaOrDash(r.DaysInactive)})
				}
				return []string{"TENANT", "EMAIL", "SKU", "REASON", "DAYS_INACTIVE"}, body
			})
		},
	}
	cmd.Flags().BoolVar(&ff.all, "all-tenants", false, allTenantsUsage)
	ff.bind(cmd)
	cmd.Flags().IntVar(&inactiveDays, "inactive-days", 60, "Flag accounts with no sign-in for this many days")
	cmd.Flags().IntVar(&graceDays, "grace-days", 14, "Skip accounts created within this many days")
	cmd.Flags().StringVar(&products, "products", "Google-Apps", "Licensing product IDs, comma-separated (Google-Apps covers Workspace editions)")
	return cmd
}
