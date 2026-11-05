// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: tenant mailboxes that are not backed up.
// pp:data-source live

package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/client"
	"dropsuite-pp-cli/internal/cliutil"
)

type coverageGap struct {
	OrgID        string `json:"org_id"`
	OrgEmail     string `json:"org_email"`
	TenantID     string `json:"tenant_id"`
	TenantDomain string `json:"tenant_domain"`
	TenantType   string `json:"tenant_type"`
	Email        string `json:"email"`
	UPN          string `json:"upn"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Licensed     bool   `json:"licensed"`
	Mailbox      bool   `json:"mailbox"`
}

type coverageOrg struct {
	OrgID     string `json:"org_id"`
	OrgEmail  string `json:"org_email"`
	Tenants   int    `json:"tenants"`
	Active    int    `json:"active"`
	Available int    `json:"available"`
	Excluded  int    `json:"excluded"`
	// PublicFolders counts tenant rows with mailbox=false; they are not
	// mailboxes, so they stay out of the counts above and the gaps.
	PublicFolders int      `json:"public_folders"`
	CoveragePct   *float64 `json:"coverage_pct"`
	AutoLicense   *bool    `json:"auto_license"`
	Status        string   `json:"status"`
	Note          string   `json:"note,omitempty"`
}

type coverageReport struct {
	GeneratedAt   string         `json:"generated_at"`
	LicensedOnly  bool           `json:"licensed_only"`
	Summary       coverageTotals `json:"summary"`
	Orgs          []coverageOrg  `json:"orgs"`
	Unprotected   []coverageGap  `json:"unprotected"`
	SkippedOrgs   []skippedOrg   `json:"skipped_orgs"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

type coverageTotals struct {
	Orgs        int      `json:"orgs"`
	Active      int      `json:"active"`
	Available   int      `json:"available"`
	Excluded    int      `json:"excluded"`
	CoveragePct *float64 `json:"coverage_pct"`
}

// coveragePct is active / (active + available); excluded mailboxes were
// deliberately left out and do not count against coverage. nil when there is
// nothing to cover.
func coveragePct(active, available int) *float64 {
	den := active + available
	if den == 0 {
		return nil
	}
	v := float64(int64(float64(active)/float64(den)*1000+0.5)) / 10
	return &v
}

// isRouteMissing reports an HTML 404 from an older deployment that lacks the
// /users/{id}/tenants routes.
func isRouteMissing(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

type tenantRef struct {
	ID     string
	Domain string
	Type   string
}

func listTenants(ctx context.Context, c *client.Client, orgID string, maxPages int) ([]tenantRef, error) {
	items, err := fetchList(ctx, c, "/users/"+url.PathEscape(orgID)+"/tenants", nil, pagedEnvelope, maxPages)
	if err != nil {
		return nil, err
	}
	out := make([]tenantRef, 0, len(items))
	missing := 0
	for _, m := range items {
		t := tenantRef{ID: str(m, "id"), Domain: str(m, "domain"), Type: strings.ToLower(str(m, "type"))}
		if t.ID == "" {
			missing++
			continue
		}
		if t.Type == "" {
			t.Type = "m365"
		}
		out = append(out, t)
	}
	if missing > 0 {
		// The tenant list is incomplete for every caller that counts on it.
		return out, fmt.Errorf("%d tenant(s) have no id and cannot be read", missing)
	}
	return out, nil
}

func listTenantAccounts(ctx context.Context, c *client.Client, orgID string, t tenantRef, status string, maxPages int) ([]map[string]any, error) {
	params := map[string]string{"type": t.Type}
	if status != "" {
		params["status"] = status
	}
	return fetchList(ctx, c, "/users/"+url.PathEscape(orgID)+"/tenants/"+url.PathEscape(t.ID)+"/accounts", params, pagedEnvelope, maxPages)
}

type orgCoverageResult struct {
	org   coverageOrg
	gaps  []coverageGap
	fails []fetchFailure
}

// orgCoverage reads one organization's tenants and mailbox inventory. admin
// sends the reseller token (GET /users/{id}); oc sends the organization's own
// token, which S1 requires for /users/{id}/tenants and its accounts.
func orgCoverage(ctx context.Context, admin, oc *client.Client, o dsOrg, licensedOnly, includeExcluded bool, maxPages int) orgCoverageResult {
	res := orgCoverageResult{org: coverageOrg{OrgID: o.ID, OrgEmail: o.Email, Status: "ok"}, gaps: make([]coverageGap, 0), fails: make([]fetchFailure, 0)}
	detail, err := admin.Get(ctx, "/users/"+url.PathEscape(o.ID), nil)
	var m map[string]any
	if err == nil {
		m, err = singleObject(detail)
	}
	if err != nil {
		// auto_license stays null: the subscription detail is unknown.
		res.fails = append(res.fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "organization detail (auto_license unknown)", Error: errText(err)})
	} else if _, ok := m["auto_license"]; ok {
		v := boolv(m, "auto_license")
		res.org.AutoLicense = &v
	}
	if oc == nil {
		res.org.Status = "unknown"
		res.org.Note = "organization has no authentication_token in the organizations list"
		res.fails = append(res.fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "organization token", Error: res.org.Note})
		return res
	}
	tenants, err := listTenants(ctx, oc, o.ID, maxPages)
	if err != nil {
		if isRouteMissing(err) {
			res.org.Status = "unsupported-on-deployment"
			res.org.Note = "this partner host does not serve /users/{id}/tenants; coverage cannot be computed here"
		} else {
			res.org.Status = "unknown"
			res.org.Note = errText(err)
		}
		res.fails = append(res.fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "tenants", Error: errText(err)})
		return res
	}
	res.org.Tenants = len(tenants)
	statuses := []string{"active", "available", "excluded"}
	for _, t := range tenants {
		for _, st := range statuses {
			items, err := listTenantAccounts(ctx, oc, o.ID, t, st, maxPages)
			if err != nil {
				res.org.Status = "partial"
				res.fails = append(res.fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "tenant " + t.Domain + " status=" + st, Error: errText(err)})
				continue
			}
			for _, m := range items {
				if v, ok := m["mailbox"]; ok && v == false {
					res.org.PublicFolders++
					continue
				}
				licensed := boolv(m, "licensed")
				switch st {
				case "active":
					res.org.Active++
					continue
				case "available":
					res.org.Available++
				case "excluded":
					res.org.Excluded++
					if !includeExcluded {
						continue
					}
				}
				if licensedOnly && !licensed {
					continue
				}
				res.gaps = append(res.gaps, coverageGap{
					OrgID: o.ID, OrgEmail: o.Email, TenantID: t.ID, TenantDomain: t.Domain, TenantType: t.Type,
					Email: str(m, "email"), UPN: str(m, "upn"), Name: str(m, "name"), Status: st,
					Licensed: licensed, Mailbox: boolv(m, "mailbox"),
				})
			}
		}
	}
	if res.org.Status == "ok" {
		res.org.CoveragePct = coveragePct(res.org.Active, res.org.Available)
	}
	return res
}

func newNovelCoverageCmd(flags *rootFlags) *cobra.Command {
	var licensedOnly, includeExcluded, includeInactive bool
	var concurrency, maxPages, limit int
	var sweepTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "coverage",
		Short: "List tenant mailboxes that are not added to backup, with coverage percent per client",
		Long: `Use this command for tenant mailboxes that exist in a client's M365 or Google Workspace tenant but are not added to backup (available or excluded). Do NOT use this command for protected objects whose backups are failing or stale; use 'health' instead.

For every organization it reads the tenants (GET /users/{id}/tenants) and each
tenant's mailbox inventory (GET /users/{id}/tenants/{tenant_id}/accounts with
status active, available and excluded) with that organization's own token
(resolved from the organizations list). Coverage is
active / (active + available); excluded mailboxes were deliberately left out
and are counted separately. Older partner hosts that do not serve the tenants
route are reported as unsupported-on-deployment, never as fully covered.`,
		Example: strings.Trim(`
  dropsuite-cli coverage --licensed-only --agent
  dropsuite-cli coverage --org 26561-14 --include-excluded`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if err := rejectArgs(args); err != nil {
				return err
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "coverage")
			}
			ctx, cancel := sweepCtx(cmd, sweepTimeout)
			defer cancel()
			admin, err := adminClient(flags)
			if err != nil {
				return err
			}
			orgs, err := fetchOrgs(ctx, admin)
			if err != nil {
				return err
			}
			picked, skipped, err := selectOrgs(orgs, orgSelector, includeInactive)
			if err != nil {
				return err
			}
			results, errs := cliutil.FanoutRun(ctx, picked,
				func(o dsOrg) string { return o.ID },
				func(ctx context.Context, o dsOrg) (orgCoverageResult, error) {
					oc, err := orgClient(flags, o)
					if err != nil {
						oc = nil
					}
					return orgCoverage(ctx, admin, oc, o, licensedOnly, includeExcluded, maxPages), nil
				},
				cliutil.WithConcurrency(concurrency),
			)
			rep := coverageReport{
				GeneratedAt:   nowRFC3339(),
				LicensedOnly:  licensedOnly,
				Orgs:          make([]coverageOrg, 0, len(results)),
				Unprotected:   make([]coverageGap, 0),
				SkippedOrgs:   skipped,
				FetchFailures: make([]fetchFailure, 0),
			}
			for _, r := range results {
				rep.Orgs = append(rep.Orgs, r.Value.org)
				rep.Unprotected = append(rep.Unprotected, r.Value.gaps...)
				rep.FetchFailures = append(rep.FetchFailures, r.Value.fails...)
			}
			for _, e := range errs {
				rep.FetchFailures = append(rep.FetchFailures, fetchFailure{OrgID: e.Source, Scope: "organization", Error: errText(e.Err)})
			}
			for _, o := range rep.Orgs {
				if o.Status != "ok" {
					continue
				}
				rep.Summary.Orgs++
				rep.Summary.Active += o.Active
				rep.Summary.Available += o.Available
				rep.Summary.Excluded += o.Excluded
			}
			rep.Summary.CoveragePct = coveragePct(rep.Summary.Active, rep.Summary.Available)
			sort.SliceStable(rep.Unprotected, func(i, j int) bool {
				a, b := rep.Unprotected[i], rep.Unprotected[j]
				if a.OrgID != b.OrgID {
					return a.OrgID < b.OrgID
				}
				return a.Email < b.Email
			})
			if limit > 0 && len(rep.Unprotected) > limit {
				rep.Unprotected = rep.Unprotected[:limit]
			}
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d read(s) failed; affected organizations are marked unknown or partial, have no coverage_pct, and are excluded from the summary\n", len(rep.FetchFailures))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
			}
			w := cmd.OutOrStdout()
			if rep.Summary.CoveragePct != nil {
				fmt.Fprintf(w, "%d organizations: %d protected, %d available to add, %d excluded (coverage %.1f%%)\n", rep.Summary.Orgs, rep.Summary.Active, rep.Summary.Available, rep.Summary.Excluded, *rep.Summary.CoveragePct)
			} else {
				fmt.Fprintf(w, "%d organizations: no tenant mailboxes reported\n", rep.Summary.Orgs)
			}
			if len(rep.Unprotected) == 0 {
				fmt.Fprintln(w, "No unprotected mailboxes found.")
				return nil
			}
			rows := make([][]string, 0, len(rep.Unprotected))
			for _, g := range rep.Unprotected {
				rows = append(rows, []string{g.OrgEmail, g.TenantDomain, g.Email, g.Status, fmt.Sprintf("%t", g.Licensed)})
			}
			return flags.printTable(cmd, []string{"ORG", "TENANT", "MAILBOX", "STATUS", "LICENSED"}, rows)
		},
	}
	cmd.Flags().BoolVar(&licensedOnly, "licensed-only", false, "Only list unprotected mailboxes that hold a license (M365 licensed=true)")
	cmd.Flags().BoolVar(&includeExcluded, "include-excluded", false, "Also list mailboxes deliberately excluded from backup")
	cmd.Flags().BoolVar(&includeInactive, "include-inactive", false, "Also check organizations whose customer is deactivated or suspended")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Organizations read in parallel")
	cmd.Flags().IntVar(&maxPages, "max-pages", 20, "Maximum 100-row pages read per tenant and status")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum unprotected rows to return (0 = all)")
	cmd.Flags().DurationVar(&sweepTimeout, "sweep-timeout", defaultSweepTimeout, "Budget for the whole run across all organizations (each request still uses --timeout)")
	return cmd
}
