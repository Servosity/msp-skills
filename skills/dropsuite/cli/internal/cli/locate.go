// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: find the client organization that owns an
// email address and that mailbox's protection state.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"dropsuite-pp-cli/internal/cliutil"
)

type locateObject struct {
	Email            string `json:"email"`
	Status           string `json:"status"`
	LastBackup       string `json:"last_backup"`
	Items            *int64 `json:"items"`
	Storage          *int64 `json:"storage"`
	DeactivatedSince string `json:"deactivated_since,omitempty"`
	Deleted          bool   `json:"deleted,omitempty"`
}

// protected reports a live backup: neither deactivated nor deleted.
func (o *locateObject) protected() bool {
	return o != nil && o.DeactivatedSince == "" && !o.Deleted
}

func countText(v *int64) string {
	if v == nil {
		return "unknown"
	}
	return strconv.FormatInt(*v, 10)
}

type locateMatch struct {
	OrgID        string        `json:"org_id"`
	OrgEmail     string        `json:"org_email"`
	MatchedBy    string        `json:"matched_by"`
	TenantDomain string        `json:"tenant_domain,omitempty"`
	TenantState  string        `json:"tenant_state"`
	Protected    bool          `json:"protected"`
	Mailbox      *locateObject `json:"mailbox"`
	OneDrive     *locateObject `json:"onedrive"`
	SSOLink      string        `json:"sso_link,omitempty"`
}

type locateReport struct {
	Email         string         `json:"email"`
	Domain        string         `json:"domain"`
	Matches       []locateMatch  `json:"matches"`
	ScannedOrgs   int            `json:"scanned_orgs"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
	Note          string         `json:"note,omitempty"`
}

func emailDomain(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}

func objectFromMap(m map[string]any, itemsKey string) *locateObject {
	lb, _ := timeField(m, "last_backup")
	ds, _ := timeField(m, "deactivated_since")
	return &locateObject{
		Email: str(m, "email"), Status: str(m, "current_backup_status"), LastBackup: formatTime(lb),
		Items: int64Ptr(m, itemsKey), Storage: int64Ptr(m, "storage"), DeactivatedSince: formatTime(ds),
		Deleted: boolv(m, "flg_deleted"),
	}
}

// firstEmailMatch returns the item whose email equals target (the API filter
// may also match an owner email, so the exact address is re-checked).
func firstEmailMatch(items []map[string]any, target string) map[string]any {
	for _, m := range items {
		if strings.EqualFold(str(m, "email"), target) {
			return m
		}
	}
	return nil
}

type locateCandidate struct {
	org       dsOrg
	matchedBy string
	tenant    *tenantRef
}

func newNovelLocateCmd(flags *rootFlags) *cobra.Command {
	var ssoLink bool
	var concurrency, maxScanOrgs, maxScanPages int
	var sweepTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "locate [email]",
		Short: "Find which client owns an email address, whether its mailbox and OneDrive are protected, and when they last backed up",
		Long: `Use this command for one person: which client org owns an email address, whether its mailbox and OneDrive are protected, and when they last backed up. Do NOT use this command for a fleet-wide failure list; use 'health' instead. Do NOT use it to list every unprotected user; use 'coverage' instead.

It matches the address's domain against every organization's tenant domains
(GET /users/{id}/tenants, read with each organization's own token), then the
owner email domain. When neither matches it
asks each organization's /accounts?email= directly, up to --max-scan-orgs.
For the owning organization it reports the backed-up mailbox and OneDrive and,
when the mailbox is not protected, its tenant state (available or excluded).
--sso-link adds a one-time 30-minute portal sign-in link for that organization
(it invalidates the organization's previous link; treat it as a secret).`,
		Example: strings.Trim(`
  dropsuite-cli locate jane@example.com --agent
  dropsuite-cli locate jane@example.com --sso-link`, "\n"),
		Annotations: map[string]string{"mcp:read-only-unless-flags": "sso-link", "pp:data-source": "live", "pp:happy-args": "email=jane@example.com"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "locate")
			}
			if len(args) != 1 || emailDomain(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("pass exactly one email address, for example: locate jane@example.com"))
			}
			target := strings.TrimSpace(args[0])
			domain := emailDomain(target)
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
			rep := locateReport{Email: target, Domain: domain, Matches: make([]locateMatch, 0), FetchFailures: make([]fetchFailure, 0)}

			// 1. Tenant-domain match.
			tenantHits, errs := cliutil.FanoutRun(ctx, orgs,
				func(o dsOrg) string { return o.ID },
				func(ctx context.Context, o dsOrg) (*tenantRef, error) {
					oc, err := orgClient(flags, o)
					if err != nil {
						return nil, err
					}
					tenants, err := listTenants(ctx, oc, o.ID, maxScanPages)
					if err != nil {
						if isRouteMissing(err) {
							return nil, nil
						}
						return nil, err
					}
					for _, t := range tenants {
						if strings.EqualFold(t.Domain, domain) {
							tt := t
							return &tt, nil
						}
					}
					return nil, nil
				},
				cliutil.WithConcurrency(concurrency),
			)
			for _, e := range errs {
				rep.FetchFailures = append(rep.FetchFailures, fetchFailure{OrgID: e.Source, Scope: "tenants", Error: errText(e.Err)})
			}
			orgByID := map[string]dsOrg{}
			for _, o := range orgs {
				orgByID[o.ID] = o
			}
			candidates := make([]locateCandidate, 0)
			for _, h := range tenantHits {
				if h.Value != nil {
					candidates = append(candidates, locateCandidate{org: orgByID[h.Source], matchedBy: "tenant domain", tenant: h.Value})
				}
			}
			// 2. Owner email domain.
			if len(candidates) == 0 {
				for _, o := range orgs {
					if emailDomain(o.Email) == domain {
						candidates = append(candidates, locateCandidate{org: o, matchedBy: "owner email domain"})
					}
				}
			}
			// Domain matches are guesses: keep one only when its inventory
			// confirms the address, or when a failed read leaves it unknown.
			checked := map[string]bool{}
			confirmed := false
			domainOwners := make([]string, 0)
			for _, cand := range candidates {
				checked[cand.org.ID] = true
				m := locateInOrg(ctx, flags, cand, target, maxScanPages, &rep.FetchFailures)
				switch {
				case m.confirmed():
					confirmed = true
					rep.Matches = append(rep.Matches, m)
				case m.TenantState == "unknown":
					rep.Matches = append(rep.Matches, m)
				case cand.tenant != nil:
					domainOwners = append(domainOwners, cand.org.label())
				}
			}
			// 3. No guess confirmed it: ask the other organizations directly.
			if !confirmed {
				scan := make([]dsOrg, 0, len(orgs))
				for _, o := range orgs {
					if !checked[o.ID] {
						scan = append(scan, o)
					}
				}
				if maxScanOrgs > 0 && len(scan) > maxScanOrgs {
					rep.Note = fmt.Sprintf("no confirmed domain match; asked the first %d of %d other organizations directly (raise --max-scan-orgs to widen)", maxScanOrgs, len(scan))
					scan = scan[:maxScanOrgs]
				}
				rep.ScannedOrgs = len(scan)
				hits, errs := cliutil.FanoutRun(ctx, scan,
					func(o dsOrg) string { return o.ID },
					func(ctx context.Context, o dsOrg) (bool, error) {
						c, err := orgClient(flags, o)
						if err != nil {
							return false, err
						}
						// A OneDrive-only user has no mailbox row, so both
						// inventories are asked; partial rows still count.
						var firstErr error
						items, err := fetchList(ctx, c, "/accounts", map[string]string{"email": target}, pagedSingle, 1)
						if firstEmailMatch(items, target) != nil {
							return true, nil
						}
						firstErr = err
						items, err = fetchList(ctx, c, "/onedrives", map[string]string{"email": target}, pagedByPage, maxScanPages)
						if firstEmailMatch(items, target) != nil {
							return true, nil
						}
						if firstErr == nil {
							firstErr = err
						}
						return false, firstErr
					},
					cliutil.WithConcurrency(concurrency),
				)
				for _, e := range errs {
					rep.FetchFailures = append(rep.FetchFailures, fetchFailure{OrgID: e.Source, Scope: "accounts/onedrives", Error: errText(e.Err)})
				}
				for _, h := range hits {
					if h.Value {
						cand := locateCandidate{org: orgByID[h.Source], matchedBy: "mailbox lookup"}
						rep.Matches = append(rep.Matches, locateInOrg(ctx, flags, cand, target, maxScanPages, &rep.FetchFailures))
					}
				}
			}
			if ssoLink {
				// Only an organization whose inventory confirmed the address
				// gets a sign-in link.
				for i := range rep.Matches {
					if rep.Matches[i].confirmed() {
						mintLocateSSO(ctx, flags, orgByID[rep.Matches[i].OrgID], &rep.Matches[i], &rep.FetchFailures)
					}
				}
			}
			if len(rep.Matches) == 0 && rep.Note == "" {
				rep.Note = "no organization owns this address's domain and no organization reports a backed-up mailbox for it"
				if len(domainOwners) > 0 {
					rep.Note = "the domain belongs to organization(s) " + strings.Join(domainOwners, ", ") + ", but no inventory lists this address"
				}
			}
			if len(rep.FetchFailures) > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d read(s) failed; a match may be missing (see fetch_failures)\n", len(rep.FetchFailures))
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), rep, flags)
			}
			w := cmd.OutOrStdout()
			if len(rep.Matches) == 0 {
				fmt.Fprintln(w, rep.Note)
				return nil
			}
			for _, m := range rep.Matches {
				fmt.Fprintf(w, "%s -> organization %s (%s), matched by %s; tenant state %s; protected=%t\n", target, m.OrgID, m.OrgEmail, m.MatchedBy, m.TenantState, m.Protected)
				if m.Mailbox != nil {
					fmt.Fprintf(w, "  mailbox: %s, last backup %s, %s messages\n", m.Mailbox.Status, m.Mailbox.LastBackup, countText(m.Mailbox.Items))
				}
				if m.OneDrive != nil {
					fmt.Fprintf(w, "  onedrive: %s, last backup %s, %s files\n", m.OneDrive.Status, m.OneDrive.LastBackup, countText(m.OneDrive.Items))
				}
				if m.SSOLink != "" {
					fmt.Fprintf(w, "  portal sign-in (one-time, 30 minutes): %s\n", m.SSOLink)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&ssoLink, "sso-link", false, "Also generate a one-time 30-minute portal sign-in link for the owning organization (invalidates its previous link)")
	cmd.Flags().IntVar(&concurrency, "concurrency", 4, "Organizations read in parallel")
	cmd.Flags().IntVar(&maxScanOrgs, "max-scan-orgs", 50, "When no domain matches, ask at most this many organizations directly (0 = all)")
	cmd.Flags().IntVar(&maxScanPages, "max-scan-pages", 5, "Maximum 100-row pages read per tenant list")
	cmd.Flags().DurationVar(&sweepTimeout, "sweep-timeout", defaultSweepTimeout, "Budget for the whole run across all organizations (each request still uses --timeout)")
	return cmd
}

// confirmed reports whether the organization's own inventory holds the
// address (a backed-up mailbox or OneDrive, or a tenant account row).
func (m locateMatch) confirmed() bool {
	switch m.TenantState {
	case "active", "available", "excluded":
		return true
	}
	return m.Mailbox != nil || m.OneDrive != nil
}

func locateInOrg(ctx context.Context, flags *rootFlags, cand locateCandidate, target string, maxScanPages int, fails *[]fetchFailure) locateMatch {
	o := cand.org
	m := locateMatch{OrgID: o.ID, OrgEmail: o.Email, MatchedBy: cand.matchedBy, TenantState: "not_found"}
	if cand.tenant != nil {
		m.TenantDomain = cand.tenant.Domain
	}
	c, err := orgClient(flags, o)
	if err != nil {
		*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "organization token", Error: errText(err)})
		m.TenantState = "unknown"
		return m
	}
	// A failed or truncated read still searches the rows already read, but
	// absence is only reported when every read completed.
	complete := true
	items, err := fetchList(ctx, c, "/accounts", map[string]string{"email": target}, pagedSingle, 1)
	if err != nil {
		complete = false
		*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "accounts", Error: errText(err)})
	}
	if hit := firstEmailMatch(items, target); hit != nil {
		m.Mailbox = objectFromMap(hit, "msg_count")
	}
	items, err = fetchList(ctx, c, "/onedrives", map[string]string{"email": target}, pagedByPage, maxScanPages)
	if err != nil {
		complete = false
		*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "onedrives", Error: errText(err)})
	}
	if hit := firstEmailMatch(items, target); hit != nil {
		m.OneDrive = objectFromMap(hit, "file_count")
	}
	if m.Mailbox.protected() {
		m.TenantState = "active"
		m.Protected = true
	} else if cand.tenant != nil {
		for _, st := range []string{"active", "available", "excluded"} {
			items, err := listTenantAccounts(ctx, c, o.ID, *cand.tenant, st, maxScanPages)
			if err != nil {
				// A truncated list still holds the pages already read.
				complete = false
				*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "tenant accounts status=" + st, Error: errText(err)})
			}
			found := false
			for _, it := range items {
				if strings.EqualFold(str(it, "email"), target) || strings.EqualFold(str(it, "upn"), target) {
					found = true
					break
				}
			}
			if found {
				m.TenantState = st
				// An explicitly deleted or deactivated mailbox stays unprotected.
				m.Protected = st == "active" && (m.Mailbox == nil || m.Mailbox.protected())
				break
			}
		}
	}
	// Mailbox state is uncertain after any failed read, even when a OneDrive
	// row already confirmed the owner.
	if m.TenantState == "not_found" && !complete {
		m.TenantState = "unknown"
	}
	return m
}

// mintLocateSSO adds a one-time portal sign-in link for o to m.
func mintLocateSSO(ctx context.Context, flags *rootFlags, o dsOrg, m *locateMatch, fails *[]fetchFailure) {
	if cliutil.IsAnyHarness() {
		*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "sso link", Error: "skipped under a Printing Press harness"})
		return
	}
	c, err := orgClient(flags, o)
	if err != nil {
		*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "sso link", Error: errText(err)})
		return
	}
	// GetMutating: each call mints a link and invalidates the previous one, so
	// it must not be retried.
	if data, err := c.GetMutating(ctx, "/users/generate_redirect_link", nil); err != nil {
		*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "sso link", Error: errText(err)})
	} else if link, err := ssoLinkFrom(data); err != nil {
		*fails = append(*fails, fetchFailure{OrgID: o.ID, OrgEmail: o.Email, Scope: "sso link", Error: errText(err)})
	} else {
		m.SSOLink = link
	}
}
