// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// applications hunt — cross-tenant application/file hunt over the local store.
// Given a hash, certificate substring, or path substring, shows every synced
// tenant and endpoint where that binary is present (trusted by an application
// definition) or pending/processed in the approval queue. The per-tenant
// Portal API forces N separate searches; the offline mirror answers it in one.
//
// pp:data-source local
package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"threatlocker-pp-cli/internal/store"
)

// huntFileHit is one application-file rule that matches the query: the file is
// already trusted by (part of) an application definition.
type huntFileHit struct {
	ApplicationID string `json:"applicationId"`
	Application   string `json:"application,omitempty"`
	FullPath      string `json:"fullPath,omitempty"`
	Cert          string `json:"cert,omitempty"`
	Hash          string `json:"hash,omitempty"`
}

// huntApprovalHit is one approval-request row that matches the query, carrying
// the tenant and endpoint where the file surfaced.
type huntApprovalHit struct {
	Organization string `json:"organization"`
	Computer     string `json:"computer,omitempty"`
	FileName     string `json:"fileName,omitempty"`
	FullPath     string `json:"fullPath,omitempty"`
	Hash         string `json:"hash,omitempty"`
	Status       string `json:"status,omitempty"`
	Requested    string `json:"requested,omitempty"`
}

// huntTenantRollup summarizes per-tenant exposure for the hunted file. Counts
// are computed over the FULL store (no --limit), so they are authoritative
// even when the detail list below is truncated.
type huntTenantRollup struct {
	Organization string `json:"organization"`
	Pending      int    `json:"pending"`
	Processed    int    `json:"processed"`
	Computers    int    `json:"computers"`
}

type huntView struct {
	Query        map[string]string  `json:"query"`
	TenantRollup []huntTenantRollup `json:"tenantRollup"`
	Approvals    []huntApprovalHit  `json:"approvals"`
	TrustedFiles []huntFileHit      `json:"trustedFiles"`
	Note         string             `json:"note,omitempty"`
}

// huntTrustedFiles queries application_files (joined to applications for the
// display name) for rules matching the hash/cert/path query. The scope owns
// the rows lifecycle so callers get a clean defer.
func huntTrustedFiles(ctx context.Context, db *store.Store, query map[string]string, limit int) ([]huntFileHit, error) {
	clauses := []string{}
	args := []any{}
	if h := query["hash"]; h != "" {
		clauses = append(clauses, `LOWER(COALESCE(af.hash, '')) = ?`)
		args = append(args, h)
	}
	if c := query["cert"]; c != "" {
		clauses = append(clauses, `COALESCE(af.cert, '') LIKE ?`)
		args = append(args, "%"+c+"%")
	}
	if p := query["path"]; p != "" {
		clauses = append(clauses, `(COALESCE(af.full_path, '') LIKE ? OR COALESCE(af.process_path, '') LIKE ?)`)
		args = append(args, "%"+p+"%", "%"+p+"%")
	}
	sql := `SELECT af.application_id, COALESCE(a.name, ''), af.full_path, af.cert, af.hash
		FROM application_files af
		LEFT JOIN applications a ON a.application_id = af.application_id
		` + whereClause(clauses) + ` LIMIT ?`
	args = append(args, limit)
	rows, err := db.DB().QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("querying application files: %w", err)
	}
	defer rows.Close()
	hits := make([]huntFileHit, 0)
	for rows.Next() {
		var appID, appName, fullPath, cert, hash *string
		if err := rows.Scan(&appID, &appName, &fullPath, &cert, &hash); err != nil {
			return nil, fmt.Errorf("scanning application file: %w", err)
		}
		hits = append(hits, huntFileHit{
			ApplicationID: tlString(appID),
			Application:   tlString(appName),
			FullPath:      tlString(fullPath),
			Cert:          tlString(cert),
			Hash:          tlString(hash),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating application files: %w", err)
	}
	return hits, nil
}

// huntApprovalClauses builds the approvals-table predicate for the query.
// Hash matches are exact. Path matches by path/file-name substring. Cert-only
// hunts cannot match the approvals table directly (it has no cert column), so
// the caller resolves the cert to concrete hashes via the trusted-file hits
// and passes them as certHashes; with no resolvable hashes the approvals
// section is skipped rather than silently substituting a filename match.
func huntApprovalClauses(query map[string]string, certHashes []string) ([]string, []any) {
	clauses := []string{}
	args := []any{}
	if h := query["hash"]; h != "" {
		clauses = append(clauses, `LOWER(COALESCE(hash, '')) = ?`)
		args = append(args, h)
		return clauses, args
	}
	or := []string{}
	if p := query["path"]; p != "" {
		or = append(or, `COALESCE(full_path, '') LIKE ?`, `COALESCE(file_name, '') LIKE ?`)
		args = append(args, "%"+p+"%", "%"+p+"%")
	}
	if len(certHashes) > 0 {
		placeholders := make([]string, len(certHashes))
		for i, h := range certHashes {
			placeholders[i] = "?"
			args = append(args, strings.ToLower(h))
		}
		or = append(or, `LOWER(COALESCE(hash, '')) IN (`+strings.Join(placeholders, ",")+`)`)
	}
	if len(or) == 0 {
		return nil, nil
	}
	clauses = append(clauses, "("+strings.Join(or, " OR ")+")")
	return clauses, args
}

func newNovelApplicationsHuntCmd(flags *rootFlags) *cobra.Command {
	var flagHash string
	var flagCert string
	var flagPath string
	var flagLimit int
	var dbPath string

	cmd := &cobra.Command{
		Use:         "hunt",
		Short:       "Locate a file (by hash, cert, or path) across every tenant and endpoint in one offline query",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Long: `Use this command to locate a specific file (by hash, certificate, or path)
across every tenant and endpoint for incident response: where it is trusted by
an application definition, and where it is pending or processed in the approval
queue — one offline query over all synced tenants instead of N portal searches.
Do NOT use this command to manage policy coverage; use 'policies copy' to push
a policy to other tenants.

The tenantRollup counts are computed over the full local store; --limit caps
only the per-row detail lists.

Sync first: threatlocker-cli sync --resources approvals,application-files,applications`,
		Example: strings.Trim(`
  threatlocker-cli applications hunt --hash 3a7bd3e2360a3d29eea436fcfb7e44c735d117c42d1c1835420b6b9942dd4f1b --agent
  threatlocker-cli applications hunt --path chrome.exe --json
  threatlocker-cli applications hunt --cert "Google LLC" --json
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would hunt the local store for matching files across all synced tenants")
				return nil
			}
			if flagHash == "" && flagCert == "" && flagPath == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("at least one of --hash, --cert, or --path is required"))
			}
			out := cmd.OutOrStdout()
			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}
			db, err := tlOpenStore(cmd.Context(), dbPath)
			if err != nil {
				return fmt.Errorf("opening local database: %w\nRun 'threatlocker-cli sync --resources approvals,application-files,applications' first.", err)
			}
			defer db.Close()

			view := huntView{Query: map[string]string{}}
			if flagHash != "" {
				view.Query["hash"] = strings.ToLower(strings.TrimSpace(flagHash))
			}
			if flagCert != "" {
				view.Query["cert"] = flagCert
			}
			if flagPath != "" {
				view.Query["path"] = flagPath
			}

			// 1. Trusted files: application_files rules matching the query.
			view.TrustedFiles, err = huntTrustedFiles(cmd.Context(), db, view.Query, flagLimit)
			if err != nil {
				return err
			}

			// 2. Approval surfacings. Cert-only hunts resolve the cert to the
			//    hashes its file rules carry, then match approvals on those.
			var certHashes []string
			if view.Query["hash"] == "" && view.Query["cert"] != "" {
				seen := map[string]bool{}
				for _, f := range view.TrustedFiles {
					if f.Hash != "" && !seen[strings.ToLower(f.Hash)] {
						seen[strings.ToLower(f.Hash)] = true
						certHashes = append(certHashes, f.Hash)
					}
				}
			}
			apprClauses, apprArgs := huntApprovalClauses(view.Query, certHashes)
			certOnlyUnresolved := apprClauses == nil && view.Query["cert"] != "" && view.Query["hash"] == "" && view.Query["path"] == ""

			view.Approvals = make([]huntApprovalHit, 0)
			view.TenantRollup = make([]huntTenantRollup, 0)
			if apprClauses != nil {
				// 2a. Authoritative per-tenant rollup over the FULL store —
				//     no LIMIT, so blast-radius counts never undercount.
				rollupSQL := `SELECT COALESCE(NULLIF(organization_name, ''), NULLIF(organization_id, ''), '(unknown tenant)') AS org,
					SUM(CASE WHEN status_id = 1 THEN 1 ELSE 0 END),
					SUM(CASE WHEN status_id IS NULL OR status_id <> 1 THEN 1 ELSE 0 END),
					COUNT(DISTINCT NULLIF(computer_name, ''))
					FROM approvals ` + whereClause(apprClauses) + ` GROUP BY org`
				rRows, err := db.DB().QueryContext(cmd.Context(), rollupSQL, apprArgs...)
				if err != nil {
					return fmt.Errorf("querying approval rollup: %w", err)
				}
				for rRows.Next() {
					var org string
					var pending, processed, computers int
					if err := rRows.Scan(&org, &pending, &processed, &computers); err != nil {
						_ = rRows.Close()
						return fmt.Errorf("scanning approval rollup: %w", err)
					}
					view.TenantRollup = append(view.TenantRollup, huntTenantRollup{
						Organization: org, Pending: pending, Processed: processed, Computers: computers,
					})
				}
				if err := rRows.Err(); err != nil {
					_ = rRows.Close()
					return fmt.Errorf("iterating approval rollup: %w", err)
				}
				_ = rRows.Close()
				sort.Slice(view.TenantRollup, func(i, j int) bool {
					if view.TenantRollup[i].Pending != view.TenantRollup[j].Pending {
						return view.TenantRollup[i].Pending > view.TenantRollup[j].Pending
					}
					return view.TenantRollup[i].Organization < view.TenantRollup[j].Organization
				})

				// 2b. Detail rows, newest first, capped by --limit.
				detailSQL := `SELECT organization_id, organization_name, computer_name, file_name, full_path, hash, status, date_requested
					FROM approvals ` + whereClause(apprClauses) + ` ORDER BY date_requested DESC LIMIT ?`
				detailArgs := append(append([]any{}, apprArgs...), flagLimit)
				aRows, err := db.DB().QueryContext(cmd.Context(), detailSQL, detailArgs...)
				if err != nil {
					return fmt.Errorf("querying approvals: %w", err)
				}
				defer aRows.Close()
				for aRows.Next() {
					var orgID, orgName, computer, fileName, fullPath, hash, status, dateReq *string
					if err := aRows.Scan(&orgID, &orgName, &computer, &fileName, &fullPath, &hash, &status, &dateReq); err != nil {
						return fmt.Errorf("scanning approval: %w", err)
					}
					org := tlString(orgName)
					if org == "" {
						org = tlString(orgID)
					}
					if org == "" {
						org = "(unknown tenant)"
					}
					view.Approvals = append(view.Approvals, huntApprovalHit{
						Organization: org,
						Computer:     tlString(computer),
						FileName:     tlString(fileName),
						FullPath:     tlString(fullPath),
						Hash:         tlString(hash),
						Status:       tlString(status),
						Requested:    tlString(dateReq),
					})
				}
				if err := aRows.Err(); err != nil {
					return fmt.Errorf("iterating approvals: %w", err)
				}
			}

			totalRollupRows := 0
			for _, t := range view.TenantRollup {
				totalRollupRows += t.Pending + t.Processed
			}
			switch {
			case len(view.Approvals) == 0 && len(view.TrustedFiles) == 0:
				view.Note = "no matches in the local store; sync first with 'threatlocker-cli sync --resources approvals,application-files,applications' or widen the query"
			case certOnlyUnresolved:
				view.Note = "cert-only hunt: no hashes resolved from matching application file rules, so the approval queue was not searched; add --hash or --path to search it"
			case totalRollupRows > len(view.Approvals):
				view.Note = fmt.Sprintf("approval detail rows capped at --limit %d; tenantRollup counts cover all %d matching rows", flagLimit, totalRollupRows)
			}

			if flags.asJSON {
				return flags.printJSON(cmd, view)
			}
			if len(view.Approvals) == 0 && len(view.TrustedFiles) == 0 {
				fmt.Fprintln(out, view.Note)
				return nil
			}
			if len(view.TenantRollup) > 0 {
				fmt.Fprintln(out, "Tenant exposure (from approval queue):")
				headers := []string{"TENANT", "PENDING", "PROCESSED", "COMPUTERS"}
				rows := make([][]string, 0, len(view.TenantRollup))
				for _, t := range view.TenantRollup {
					rows = append(rows, []string{t.Organization, fmt.Sprintf("%d", t.Pending), fmt.Sprintf("%d", t.Processed), fmt.Sprintf("%d", t.Computers)})
				}
				if err := flags.printTable(cmd, headers, rows); err != nil {
					return err
				}
			}
			if len(view.TrustedFiles) > 0 {
				fmt.Fprintf(out, "\nTrusted by %d application file rule(s):\n", len(view.TrustedFiles))
				headers := []string{"APPLICATION", "PATH", "CERT", "HASH"}
				rows := make([][]string, 0, len(view.TrustedFiles))
				for _, f := range view.TrustedFiles {
					name := f.Application
					if name == "" {
						name = f.ApplicationID
					}
					hash := f.Hash
					if len(hash) > 16 {
						hash = hash[:16] + "…"
					}
					rows = append(rows, []string{name, f.FullPath, f.Cert, hash})
				}
				if err := flags.printTable(cmd, headers, rows); err != nil {
					return err
				}
			}
			if view.Note != "" {
				fmt.Fprintf(out, "\nnote: %s\n", view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagHash, "hash", "", "SHA256 hash to hunt (exact match, case-insensitive)")
	cmd.Flags().StringVar(&flagCert, "cert", "", "Certificate subject substring to hunt")
	cmd.Flags().StringVar(&flagPath, "path", "", "File path or name substring to hunt")
	cmd.Flags().IntVar(&flagLimit, "limit", 200, "Maximum detail rows per section (tenantRollup is always computed over the full store)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}
