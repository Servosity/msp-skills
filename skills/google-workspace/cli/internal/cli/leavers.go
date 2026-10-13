// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Novel feature: leavers - suspended accounts that were not fully offboarded:
// paid licence, group memberships, admin roles, OAuth tokens, no completed
// Drive/Calendar transfer.
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

type leaverFinding struct {
	Tenant           string   `json:"tenant"`
	Email            string   `json:"email"`
	Status           string   `json:"status"` // residual | unverified | clean | capped
	SuspensionReason string   `json:"suspension_reason,omitempty"`
	LastLogin        string   `json:"last_login,omitempty"`
	PaidLicenses     []string `json:"paid_licenses,omitempty"`
	Groups           int      `json:"groups"`
	AdminRoles       int      `json:"admin_roles"`
	SuperAdmin       bool     `json:"super_admin,omitempty"`
	OAuthTokens      int      `json:"oauth_tokens"`
	Transfer         string   `json:"transfer"` // completed | in_progress | failed | none | unknown
	Issues           []string `json:"issues,omitempty"`
}

var transferScopes = []string{"https://www.googleapis.com/auth/admin.datatransfer", "https://www.googleapis.com/auth/admin.datatransfer.readonly"}

func hasAnyScope(t gwsTenant, scopes []string) bool {
	for _, s := range scopes {
		if len(hasScopes(t, []string{s})) == 0 {
			return true
		}
	}
	return false
}

// leaverIssues is pure: it turns the joined facts about one suspended
// account into its issue list and status.
func leaverIssues(f *leaverFinding) {
	f.Issues = nil
	for _, sku := range f.PaidLicenses {
		f.Issues = append(f.Issues, "paid licence: "+sku)
	}
	if f.SuperAdmin {
		f.Issues = append(f.Issues, "still a super admin")
	}
	if f.AdminRoles > 0 {
		f.Issues = append(f.Issues, fmt.Sprintf("holds %d admin role(s)", f.AdminRoles))
	}
	if f.Groups > 0 {
		f.Issues = append(f.Issues, fmt.Sprintf("member of %d group(s)", f.Groups))
	}
	if f.OAuthTokens > 0 {
		f.Issues = append(f.Issues, fmt.Sprintf("%d OAuth token(s) not revoked", f.OAuthTokens))
	}
	switch f.Transfer {
	case "none", "failed":
		f.Issues = append(f.Issues, "no completed Drive transfer")
	case "in_progress":
		f.Issues = append(f.Issues, "latest Drive transfer still running")
	}
	f.Status = "clean"
	switch {
	case len(f.Issues) > 0:
		f.Status = "residual"
	case f.Transfer == "unknown":
		// Nothing else left, but the transfer was never checked: surface it
		// rather than call an unverified account clean.
		f.Status = "unverified"
		f.Issues = append(f.Issues, "Drive transfer not checked (tenant lacks admin.datatransfer.readonly)")
	}
}

func countItems(ctx context.Context, c *client.Client, path string, params map[string]string, key string) (int, error) {
	raw, err := listAll(ctx, c, path, params, key, 0)
	return len(raw), err
}

// driveAppID resolves the "Drive and Docs" data-transfer application ID
// (Google warns its published ID list can be stale).
func driveAppID(ctx context.Context, c *client.Client) (string, error) {
	apps, err := listAll(ctx, c, "/admin/datatransfer/v1/applications", nil, "applications", 0)
	if err != nil {
		return "", fmt.Errorf("listing transfer applications: %w", err)
	}
	for _, a := range apps {
		var app struct {
			ID   json.RawMessage `json:"id"`
			Name string          `json:"name"`
		}
		if err := json.Unmarshal(a, &app); err != nil {
			return "", err
		}
		if app.Name == "Drive and Docs" {
			return jsonScalar(app.ID), nil
		}
	}
	return "", fmt.Errorf("no Drive and Docs transfer application")
}

type dataTransfer struct {
	RequestTime string `json:"requestTime"`
	Status      string `json:"overallTransferStatusCode"`
	Apps        []struct {
		ApplicationID json.RawMessage `json:"applicationId"`
	} `json:"applicationDataTransfers"`
}

// latestDriveTransferStatus is pure: the status of the most recent transfer
// that included Drive, so an old completed transfer cannot hide a newer
// unfinished one and a Calendar-only transfer does not count.
func latestDriveTransferStatus(transfers []dataTransfer, driveID string) string {
	var latest time.Time
	status := "none"
	for _, tr := range transfers {
		hasDrive := false
		for _, a := range tr.Apps {
			if jsonScalar(a.ApplicationID) == driveID {
				hasDrive = true
			}
		}
		if !hasDrive {
			continue
		}
		// Compare instants, not strings: "...:00Z" sorts after "...:00.001Z".
		at, err := time.Parse(time.RFC3339Nano, tr.RequestTime)
		if err != nil {
			return "unknown" // cannot order transfers; do not guess clean
		}
		if !latest.IsZero() && at.Before(latest) {
			continue
		}
		latest = at
		switch strings.ToLower(tr.Status) {
		case "completed":
			status = "completed"
		case "failed":
			status = "failed"
		default: // inProgress, new
			status = "in_progress"
		}
	}
	return status
}

func transferStatus(ctx context.Context, c *client.Client, custID, userID, driveID string) (string, error) {
	raw, err := listAll(ctx, c, "/admin/datatransfer/v1/transfers", map[string]string{"oldOwnerUserId": userID, "customerId": custID}, "dataTransfers", 0)
	if err != nil {
		return "", err
	}
	transfers := make([]dataTransfer, 0, len(raw))
	for _, r := range raw {
		var tr dataTransfer
		if err := json.Unmarshal(r, &tr); err != nil {
			return "", err
		}
		transfers = append(transfers, tr)
	}
	return latestDriveTransferStatus(transfers, driveID), nil
}

func newNovelLeaversCmd(flags *rootFlags) *cobra.Command {
	var ff fanoutFlags
	var maxAccounts int
	var includeClean bool
	cmd := &cobra.Command{
		Use:   "leavers",
		Short: "Suspended accounts still holding a licence, groups, admin roles, OAuth tokens, or no completed transfer",
		Long: "Checks every suspended account per tenant for what offboarding should have removed:\n" +
			"a paid Google Workspace licence (suspended accounts bill like active ones; an Archived\n" +
			"User licence is the right one), group memberships, admin roles, unrevoked OAuth tokens,\n" +
			"and whether the latest Drive transfer completed (needs admin.datatransfer.readonly in\n" +
			"the tenant's scopes, otherwise transfer reads unknown).\n" +
			"Use this command to check that already-suspended accounts were fully offboarded (residual\n" +
			"licence, groups, admin role, tokens, unfinished transfer). Do NOT use this command to\n" +
			"offboard someone; use 'offboard' instead. Do NOT use it to find idle active accounts;\n" +
			"use 'stale-licenses' instead.",
		Example: "  google-workspace-cli leavers --all-tenants\n" +
			"  google-workspace-cli leavers --all-tenants --agent --select results.tenant,results.email,results.issues",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if done, err := helpOrDryRun(cmd, args, flags, "check suspended accounts for residual access per tenant"); done {
				return err
			}
			if maxAccounts < 1 {
				return usageErr(fmt.Errorf("--max-accounts must be >= 1"))
			}
			res, err := fanout(cmd, flags, &ff, func(ctx context.Context, t gwsTenant, c *client.Client) ([]leaverFinding, error) {
				users, err := listUsers(ctx, c, t, "isSuspended=true")
				if err != nil {
					return nil, err
				}
				custID, err := tenantCustomerID(ctx, c, t)
				if err != nil {
					return nil, err
				}
				licenses, err := listLicenses(ctx, c, custID, "Google-Apps")
				if err != nil {
					return nil, fmt.Errorf("licences: %w", err)
				}
				paid := map[string][]string{}
				for _, l := range licenses {
					k := strings.ToLower(l.UserID)
					paid[k] = append(paid[k], l.SkuName)
				}
				canTransfer := hasAnyScope(t, transferScopes) || t.KeyFile == ""
				driveID := ""
				if canTransfer {
					if driveID, err = driveAppID(ctx, c); err != nil {
						return nil, err
					}
				}
				out := []leaverFinding{}
				for i, u := range users {
					if !u.Suspended {
						continue
					}
					if i >= maxAccounts {
						out = append(out, leaverFinding{Tenant: t.Name, Status: "capped", Transfer: "unknown",
							Issues: []string{fmt.Sprintf("%d more suspended account(s) not checked; raise --max-accounts", len(users)-maxAccounts)}})
						break
					}
					f := leaverFinding{Tenant: t.Name, Email: u.PrimaryEmail, SuspensionReason: u.SuspensionReason,
						LastLogin: u.LastLoginTime, PaidLicenses: paid[strings.ToLower(u.PrimaryEmail)], SuperAdmin: u.IsAdmin, Transfer: "unknown"}
					id := url.PathEscape(u.ID)
					if f.Groups, err = countItems(ctx, c, "/admin/directory/v1/groups", map[string]string{"userKey": u.ID, "maxResults": "200"}, "groups"); err != nil {
						return nil, fmt.Errorf("%s groups: %w", u.PrimaryEmail, err)
					}
					if f.AdminRoles, err = countItems(ctx, c, "/admin/directory/v1/customer/"+url.PathEscape(t.customer())+"/roleassignments", map[string]string{"userKey": u.ID}, "items"); err != nil {
						return nil, fmt.Errorf("%s role assignments: %w", u.PrimaryEmail, err)
					}
					if f.OAuthTokens, err = countItems(ctx, c, "/admin/directory/v1/users/"+id+"/tokens", nil, "items"); err != nil {
						return nil, fmt.Errorf("%s tokens: %w", u.PrimaryEmail, err)
					}
					if canTransfer {
						if f.Transfer, err = transferStatus(ctx, c, custID, u.ID, driveID); err != nil {
							return nil, fmt.Errorf("%s transfers: %w", u.PrimaryEmail, err)
						}
					}
					leaverIssues(&f)
					if f.Status != "clean" || includeClean {
						out = append(out, f)
					}
				}
				return out, nil
			})
			if err != nil {
				return err
			}
			sort.SliceStable(res.Results, func(i, j int) bool {
				a, b := res.Results[i], res.Results[j]
				if a.Tenant != b.Tenant {
					return a.Tenant < b.Tenant
				}
				return len(a.Issues) > len(b.Issues)
			})
			return emitFanout(cmd, flags, res, func(rs []leaverFinding) ([]string, [][]string) {
				body := [][]string{}
				for _, r := range rs {
					body = append(body, []string{r.Tenant, r.Email, r.Status, r.Transfer, strings.Join(r.Issues, "; ")})
				}
				return []string{"TENANT", "EMAIL", "STATUS", "TRANSFER", "ISSUES"}, body
			})
		},
	}
	cmd.Flags().BoolVar(&ff.all, "all-tenants", false, allTenantsUsage)
	ff.bind(cmd)
	cmd.Flags().IntVar(&maxAccounts, "max-accounts", 100, "Suspended accounts checked per tenant (each costs 3-4 API calls)")
	cmd.Flags().BoolVar(&includeClean, "include-clean", false, "Also list suspended accounts with nothing left to clean up")
	return cmd
}
