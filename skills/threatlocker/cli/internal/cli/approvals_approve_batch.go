// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source auto

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"threatlocker-pp-cli/internal/client"
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
the local store and permits the application in each. Preview with --dry-run
first; it prints the exact set of (tenant, request) pairs that would be approved
without sending anything. A live run requires --yes.

A live run first re-checks each request against the live API under its own
tenant and approves only the ones still pending with this hash, so a request
approved or ignored in the portal since the last sync is skipped and reported.

Tenant scope: the plan covers only the tenant selected by --org or, when --org
is absent, THREATLOCKER_ORG_ID. Pass --all-tenants to approve in every synced
tenant where the file is pending. With no tenant selected and no --all-tenants,
a live run refuses and sends nothing, and --dry-run shows that refusal with an
empty plan.

Sync first: threatlocker-cli sync --resources approvals`,
		Example: strings.Trim(`
  threatlocker-cli approvals approve-batch --hash <sha256> --all-tenants --dry-run
  threatlocker-cli approvals approve-batch --hash <sha256> --all-tenants --yes
  threatlocker-cli approvals approve-batch --hash <sha256> --org <tenant-guid> --dry-run
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			hash := strings.ToLower(strings.TrimSpace(flagHash))

			if dbPath == "" {
				dbPath = defaultDBPath("threatlocker-cli")
			}

			// The tenant scope is the operator's tenant setting (--org, else
			// THREATLOCKER_ORG_ID), or every synced tenant with --all-tenants.
			// With neither, scopeErr refuses the live run below: a write must
			// never span every tenant by default. Hand-wired (class: tenant
			// setting ignored by local-store commands).
			scope, scopeErr := tlRequireTenantScope(flagAllTenants)

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
				if scope != "" {
					query += ` AND organization_id = ?`
					sqlArgs = append(sqlArgs, scope)
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
				// Unscoped, the live run refuses, so the plan is empty; the
				// cross-tenant matches are only counted, never planned.
				planned := targets
				if scopeErr != nil {
					planned = []batchTarget{}
				}
				plan := map[string]any{"action": "approve-batch", "hash": hash, "dryRun": true, "allTenants": flagAllTenants, "tenantScope": scope, "wouldApprove": len(planned), "targets": planned}
				if scopeErr != nil {
					plan["refused"] = "a live run would refuse: " + scopeErr.Error()
					plan["matchedAcrossTenants"] = len(targets)
				} else {
					plan["liveRecheck"] = approvalRecheckNote
				}
				if flags.asJSON {
					return flags.printJSON(cmd, plan)
				}
				if hash == "" {
					fmt.Fprintln(out, "dry-run: no --hash provided; nothing would be approved")
					return nil
				}
				if scopeErr != nil {
					fmt.Fprintf(out, "dry-run: a live run would refuse: %v\n", scopeErr)
					fmt.Fprintf(out, "dry-run: nothing would be approved (%d pending request(s) for hash %s match across every synced tenant)\n", len(targets), hash)
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
				fmt.Fprintf(out, "dry-run: %s\n", approvalRecheckNote)
				return nil
			}

			if hash == "" {
				return fmt.Errorf("required flag \"hash\" not set: pass --hash <sha256> (use --dry-run to preview)")
			}
			if scopeErr != nil {
				return usageErr(fmt.Errorf("refusing to approve: %w", scopeErr))
			}
			if len(targets) == 0 {
				if scope != "" {
					fmt.Fprintf(out, "No pending approval requests matching hash %s for tenant %s in the local store (pass --all-tenants to include every synced tenant).\n", hash, scope)
					return nil
				}
				fmt.Fprintf(out, "No pending approval requests matching hash %s in the local store.\n", hash)
				return nil
			}

			// A live run approves the file in every listed tenant. It needs the
			// explicit --yes the examples always showed; the MCP mirror blocks
			// --yes, so an agent can only preview. Hand-wired (handfixes.json:
			// approve-batch-requires-yes).
			if !flags.yes {
				tenants := map[string]bool{}
				for _, t := range targets {
					tenants[t.OrganizationID] = true
				}
				return usageErr(fmt.Errorf("refusing to approve %d pending request(s) across %d tenant(s) without confirmation: preview with --dry-run, then re-run with --yes", len(targets), len(tenants)))
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			type batchResult struct {
				batchTarget
				Approved bool `json:"approved"`
				// Outcome is approved, skipped_not_pending (the live API says
				// the request left Pending), skipped_unverified (the live
				// re-check did not confirm it, so no write was sent), or
				// failed (the write was sent and did not succeed).
				Outcome      string `json:"outcome"`
				LiveStatusID *int64 `json:"liveStatusId,omitempty"`
				Reason       string `json:"reason,omitempty"`
				Error        string `json:"error,omitempty"`
			}
			results := make([]batchResult, 0, len(targets))
			approved, skippedNotPending, failed := 0, 0, 0
			for _, t := range targets {
				// Scope each call to ITS tenant. The New Portal API scopes
				// calls by the ManagedOrganizationId header; relying on the
				// single header newClient() bakes in (from --org, or absent)
				// would issue cross-tenant approvals under the wrong tenant
				// context even though the body names the right organization.
				headers := map[string]string{}
				if t.OrganizationID != "" {
					headers[TenantOrgHeader] = t.OrganizationID
					headers[OverrideTenantOrgHeader] = t.OrganizationID
				}
				r := batchResult{batchTarget: t}
				// The local row is a snapshot: sync fetches statusId=1 only,
				// so a request approved or ignored upstream since then still
				// reads pending here. Re-confirm it live, under its tenant,
				// immediately before its write; anything short of a positive
				// "still pending, this hash" sends nothing. Hand-wired
				// (class: a write acts on a stale local snapshot).
				check := approvalLiveRecheck(cmd.Context(), c, t, hash, headers)
				r.LiveStatusID = check.statusID
				switch check.verdict {
				case approvalNotPending:
					r.Outcome, r.Reason = "skipped_not_pending", check.reason
					skippedNotPending++
					results = append(results, r)
					continue
				case approvalUnconfirmed:
					r.Outcome, r.Reason, r.Error = "skipped_unverified", check.reason, check.reason
					failed++
					results = append(results, r)
					continue
				}
				body := map[string]any{
					"organizationId": t.OrganizationID,
					"policyLevel":    flagPolicyLevel,
				}
				if t.ComputerID != "" {
					body["computerId"] = t.ComputerID
				}
				data, status, err := c.PostWithParamsAndHeaders(cmd.Context(), "/ApprovalRequest/ApprovalRequestPermitApplication", map[string]string{}, body, headers)
				if failure := approvalWriteFailure(data, status, err); failure != "" {
					r.Outcome, r.Reason, r.Error = "failed", failure, failure
					failed++
				} else {
					r.Approved, r.Outcome = true, "approved"
					approved++
				}
				results = append(results, r)
			}
			// A request that merely left Pending upstream is the re-check
			// working, not a failure. A target skipped because its re-check
			// errored or did not match, or a write that failed, is.
			var outcome error
			if failed > 0 {
				outcome = fmt.Errorf("%d of %d approval request(s) were not approved: their live re-check or their write failed", failed, len(targets))
			}
			if flags.asJSON {
				if err := flags.printJSON(cmd, map[string]any{"hash": hash, "approved": approved, "skipped_not_pending": skippedNotPending, "failed": failed, "total": len(targets), "results": results}); err != nil {
					return err
				}
				return outcome
			}
			fmt.Fprintf(out, "Approved %d/%d request(s) for hash %s; %d skipped as no longer pending upstream, %d failed\n", approved, len(targets), hash, skippedNotPending, failed)
			for _, r := range results {
				tenant := r.OrganizationName
				if tenant == "" {
					tenant = r.OrganizationID
				}
				switch r.Outcome {
				case "skipped_not_pending":
					fmt.Fprintf(out, "  SKIPPED %s (%s) request %s: %s\n", r.FileName, tenant, r.ApprovalRequestID, r.Reason)
				case "skipped_unverified":
					fmt.Fprintf(out, "  NOT SENT %s (%s) request %s: %s\n", r.FileName, tenant, r.ApprovalRequestID, r.Reason)
				case "failed":
					fmt.Fprintf(out, "  FAILED %s (%s) request %s: %s\n", r.FileName, tenant, r.ApprovalRequestID, r.Error)
				}
			}
			return outcome
		},
	}
	cmd.Flags().StringVar(&flagHash, "hash", "", "SHA256 of the file to approve across tenants (required for a live run)")
	cmd.Flags().BoolVar(&flagAllTenants, "all-tenants", false, "Approve in every synced organization where the file is pending; without it the plan covers only the --org (else THREATLOCKER_ORG_ID) tenant, and a live run with neither refuses")
	cmd.Flags().IntVar(&flagPolicyLevel, "policy-level", 0, "Permit policy scope level passed to the approve call")
	cmd.Flags().StringVar(&dbPath, "db", "", "Database path")
	return cmd
}

// approvalWriteFailure returns why one approval write failed, or "" when it
// succeeded. Any failure signal wins: a transport error, a non-2xx status, and
// a 2xx body that declares failure in-band (the ActionResult success flag the
// spec documents, via the shared envelope detector sync uses). Hand-wired
// (class: in-band failure ignored on a write).
func approvalWriteFailure(data json.RawMessage, status int, err error) string {
	switch {
	case err != nil:
		return err.Error()
	case status < 200 || status >= 300:
		return fmt.Sprintf("HTTP %d", status)
	case responseDeclaresFailure(data):
		var result struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &result) == nil && strings.TrimSpace(result.Message) != "" {
			return "the API reported failure: " + strings.TrimSpace(result.Message)
		}
		return "the API reported failure"
	}
	return ""
}

// approvalRecheckNote is what the dry-run tells the operator about the live run.
const approvalRecheckNote = "the live run re-checks each request against the live API under its own tenant first, and approves only the ones still pending with this hash"

// approvalRecheckPath is the spec's approvals_get endpoint; it returns one
// ApprovalRequest (statusId 1=Pending, 4=Approved, 10=Ignored, 13=Escalated).
const approvalRecheckPath = "/ApprovalRequest/ApprovalRequestGetById"

type approvalRecheckVerdict int

const (
	approvalStillPending approvalRecheckVerdict = iota
	approvalNotPending
	approvalUnconfirmed
)

type approvalRecheck struct {
	verdict  approvalRecheckVerdict
	statusID *int64
	reason   string
}

var approvalStatusNames = map[int64]string{1: "Pending", 4: "Approved", 10: "Ignored", 13: "Escalated"}

// approvalLiveRecheck re-reads one target from the live API, under that
// target's tenant headers and bypassing the response cache (a cached answer is
// another stale snapshot), and returns whether it may be approved. Only a
// single well-formed ApprovalRequest object whose statusId is 1 and whose hash
// is the requested one confirms it; a row that names a different request or
// tenant does not. Every other answer (transport or HTTP error, in-band
// failure, unparsable or duplicated fields, missing statusId or hash) is
// approvalUnconfirmed: any failure signal wins, so nothing is approved on an
// ambiguous answer. Hand-wired (class: a write acts on a stale local snapshot).
func approvalLiveRecheck(ctx context.Context, c *client.Client, t batchTarget, hash string, headers map[string]string) approvalRecheck {
	unconfirmed := func(format string, args ...any) approvalRecheck {
		return approvalRecheck{verdict: approvalUnconfirmed, reason: "not approved: the live re-check " + fmt.Sprintf(format, args...)}
	}
	reqID := strings.TrimSpace(t.ApprovalRequestID)
	org := strings.TrimSpace(t.OrganizationID)
	if reqID == "" {
		return unconfirmed("cannot run: the local row has no approval request id")
	}
	if org == "" {
		return unconfirmed("cannot run: the local row names no tenant to re-check it under")
	}
	data, err := c.GetWithHeadersNoCache(ctx, approvalRecheckPath, map[string]string{"approvalRequestId": reqID}, headers)
	if err != nil {
		return unconfirmed("failed: %v", err)
	}
	if responseDeclaresFailure(data) {
		return unconfirmed("failed: the API reported failure")
	}
	fields, err := approvalObjectFields(data)
	if err != nil {
		return unconfirmed("returned an unreadable answer: %v", err)
	}
	if id, present, err := approvalStringField(fields, "approvalRequestId"); err != nil {
		return unconfirmed("returned an unusable approvalRequestId: %v", err)
	} else if present && !strings.EqualFold(strings.TrimSpace(id), reqID) {
		return unconfirmed("returned request %q, not %q", id, reqID)
	}
	if liveOrg, present, err := approvalStringField(fields, "organizationId"); err != nil {
		return unconfirmed("returned an unusable organizationId: %v", err)
	} else if present && !strings.EqualFold(strings.TrimSpace(liveOrg), org) {
		return unconfirmed("returned a request in tenant %q, not %q", liveOrg, org)
	}
	liveHash, present, err := approvalStringField(fields, "hash")
	switch {
	case err != nil:
		return unconfirmed("returned an unusable hash: %v", err)
	case !present || strings.TrimSpace(liveHash) == "":
		return unconfirmed("returned no hash to match against --hash")
	case !strings.EqualFold(strings.TrimSpace(liveHash), hash):
		return unconfirmed("returned hash %q, which does not match --hash %s", liveHash, hash)
	}
	status, err := approvalIntField(fields, "statusId")
	if err != nil {
		return unconfirmed("returned no usable statusId: %v", err)
	}
	if status != 1 {
		name := approvalStatusNames[status]
		if name != "" {
			name = ", " + name
		}
		return approvalRecheck{verdict: approvalNotPending, statusID: &status, reason: fmt.Sprintf("no longer pending upstream (live statusId %d%s)", status, name)}
	}
	return approvalRecheck{verdict: approvalStillPending, statusID: &status}
}

// approvalObjectFields decodes exactly one top-level JSON object and returns
// every value per lower-cased key. Duplicates are kept rather than letting the
// last one win, so a body carrying "statusId" twice (or "statusId" and
// "StatusId") is seen as ambiguous instead of silently resolved.
func approvalObjectFields(data []byte) (map[string][]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("empty body")
		}
		return nil, errors.New("not JSON")
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("not a single JSON object")
	}
	fields := map[string][]json.RawMessage{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, errors.New("malformed JSON object")
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, errors.New("malformed JSON object")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, errors.New("malformed JSON object")
		}
		lower := strings.ToLower(key)
		fields[lower] = append(fields[lower], value)
	}
	if _, err := dec.Token(); err != nil {
		return nil, errors.New("malformed JSON object")
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("data after the JSON object")
	}
	return fields, nil
}

// approvalSingleField returns the one value for name, or present=false when it
// is absent or null. More than one value (in any letter case) is an error.
func approvalSingleField(fields map[string][]json.RawMessage, name string) (json.RawMessage, bool, error) {
	values := fields[strings.ToLower(name)]
	switch {
	case len(values) == 0:
		return nil, false, nil
	case len(values) > 1:
		return nil, false, fmt.Errorf("%s appears %d times", name, len(values))
	}
	raw := bytes.TrimSpace(values[0])
	if bytes.Equal(raw, []byte("null")) {
		return nil, false, nil
	}
	return raw, true, nil
}

func approvalStringField(fields map[string][]json.RawMessage, name string) (string, bool, error) {
	raw, present, err := approvalSingleField(fields, name)
	if err != nil || !present {
		return "", false, err
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false, fmt.Errorf("%s is not a string", name)
	}
	return value, true, nil
}

func approvalIntField(fields map[string][]json.RawMessage, name string) (int64, error) {
	raw, present, err := approvalSingleField(fields, name)
	if err != nil {
		return 0, err
	}
	if !present {
		return 0, fmt.Errorf("%s is missing", name)
	}
	value, err := strconv.ParseInt(string(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s is not an integer", name)
	}
	return value, nil
}
