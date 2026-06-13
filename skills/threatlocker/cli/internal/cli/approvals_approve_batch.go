// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"threatlocker-pp-cli/internal/cliutil"
)

// batchTarget is one pending approval that matches the requested hash.
type batchTarget struct {
	ApprovalRequestID string `json:"approvalRequestId"`
	OrganizationID    string `json:"organizationId"`
	OrganizationName  string `json:"organizationName"`
	ComputerID        string `json:"computerId"`
	FileName          string `json:"fileName"`
}

func newNovelApprovalsApproveBatchCmd(flags *rootFlags) *cobra.Command {
	var flagHash string
	var flagAllTenants bool
	var flagPolicyLevel int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "approve-batch",
		Short:       "Approve the same file (by SHA256) across every tenant where it is pending, in one command, with a dry-run plan first.",
		Annotations: map[string]string{"mcp:read-only": "false"},
		Long: `Approve-batch resolves every PENDING approval request matching a file hash from
the local store and permits the application in each. Always preview with
--dry-run first; it prints the exact set of (tenant, request) pairs that would be
approved without sending anything.

Sync first: threatlocker-cli sync --resources approvals`,
		Example: strings.Trim(`
  threatlocker-cli approvals approve-batch --hash <sha256> --all-tenants --dry-run
  threatlocker-cli approvals approve-batch --hash <sha256> --all-tenants --yes
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			hash := strings.ToLower(strings.TrimSpace(flagHash))

			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}

			// Resolve the target set from the local store (no network).
			var targets []batchTarget
			if hash != "" {
				db, err := tlOpenStore(cmd.Context(), dbPath)
				if err != nil {
					return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources approvals' first.", err)
				}
				defer db.Close()
				query := `SELECT approval_request_id, organization_id, organization_name, computer_id, file_name
					FROM approvals WHERE status_id = 1 AND lower(hash) = ?`
				sqlArgs := []any{hash}
				if !flagAllTenants && flags.orgID != "" {
					query += ` AND organization_id = ?`
					sqlArgs = append(sqlArgs, flags.orgID)
				}
				rows, err := db.DB().QueryContext(cmd.Context(), query, sqlArgs...)
				if err != nil {
					return fmt.Errorf("querying approvals: %w", err)
				}
				defer rows.Close()
				for rows.Next() {
					var reqID, orgID, orgName, compID, fileName *string
					if err := rows.Scan(&reqID, &orgID, &orgName, &compID, &fileName); err != nil {
						return fmt.Errorf("scanning approval: %w", err)
					}
					targets = append(targets, batchTarget{
						ApprovalRequestID: tlString(reqID),
						OrganizationID:    tlString(orgID),
						OrganizationName:  tlString(orgName),
						ComputerID:        tlString(compID),
						FileName:          tlString(fileName),
					})
				}
				if err := rows.Err(); err != nil {
					return fmt.Errorf("iterating approvals: %w", err)
				}
			}

			// Dry-run (and verify probes) print the plan and never mutate. A
			// missing --hash under dry-run is not an error — it yields an empty
			// plan, so verify's `--dry-run` probe exits 0.
			if flags.dryRun || cliutil.IsVerifyEnv() {
				plan := map[string]any{"action": "approve-batch", "hash": hash, "dryRun": true, "wouldApprove": len(targets), "targets": targets}
				if flags.asJSON {
					return flags.printJSON(cmd, plan)
				}
				if hash == "" {
					fmt.Fprintln(out, "dry-run: no --hash provided; nothing would be approved")
					return nil
				}
				fmt.Fprintf(out, "dry-run: would approve %d pending request(s) for hash %s:\n", len(targets), hash)
				for _, t := range targets {
					tenant := t.OrganizationName
					if tenant == "" {
						tenant = t.OrganizationID
					}
					fmt.Fprintf(out, "  - %s (%s) request %s\n", t.FileName, tenant, t.ApprovalRequestID)
				}
				return nil
			}

			if hash == "" {
				return fmt.Errorf("required flag \"hash\" not set: pass --hash <sha256> (use --dry-run to preview)")
			}
			if len(targets) == 0 {
				fmt.Fprintf(out, "No pending approval requests matching hash %s in the local store.\n", hash)
				return nil
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			type batchResult struct {
				batchTarget
				Approved bool   `json:"approved"`
				Error    string `json:"error,omitempty"`
			}
			results := make([]batchResult, 0, len(targets))
			approved := 0
			for _, t := range targets {
				body := map[string]any{
					"organizationId": t.OrganizationID,
					"policyLevel":    flagPolicyLevel,
				}
				if t.ComputerID != "" {
					body["computerId"] = t.ComputerID
				}
				// Scope each write to ITS tenant. The New Portal API scopes
				// calls by the ManagedOrganizationId header; relying on the
				// single header newClient() bakes in (from --org, or absent)
				// would issue cross-tenant approvals under the wrong tenant
				// context even though the body names the right organization.
				headers := map[string]string{}
				if t.OrganizationID != "" {
					headers["ManagedOrganizationId"] = t.OrganizationID
					headers["OverrideManagedOrganizationId"] = t.OrganizationID
				}
				_, status, err := c.PostWithParamsAndHeaders(cmd.Context(), "/ApprovalRequest/ApprovalRequestPermitApplication", map[string]string{}, body, headers)
				r := batchResult{batchTarget: t}
				if err != nil {
					r.Error = err.Error()
				} else if status >= 200 && status < 300 {
					r.Approved = true
					approved++
				} else {
					r.Error = fmt.Sprintf("HTTP %d", status)
				}
				results = append(results, r)
			}
			if flags.asJSON {
				return flags.printJSON(cmd, map[string]any{"hash": hash, "approved": approved, "total": len(targets), "results": results})
			}
			fmt.Fprintf(out, "Approved %d/%d pending request(s) for hash %s\n", approved, len(targets), hash)
			for _, r := range results {
				if !r.Approved {
					fmt.Fprintf(out, "  FAILED %s (%s): %s\n", r.FileName, r.OrganizationName, r.Error)
				}
			}
			if approved < len(targets) {
				return fmt.Errorf("%d of %d approvals failed", len(targets)-approved, len(targets))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagHash, "hash", "", "SHA256 of the file to approve across tenants (required for a live run)")
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Approve across every synced organization (default scopes to --org when set)")
	cmd.Flags().IntVar(&flagPolicyLevel, "policy-level", 0, "Permit policy scope level passed to the approve call")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
