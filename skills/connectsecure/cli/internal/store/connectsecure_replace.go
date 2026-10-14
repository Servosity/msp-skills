// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: rows the local reports read are replaced on sync.

package store

// connectsecureReplaceOnSync are the resources exposure, portfolio,
// blind-spots, suppression-audit and ad-hygiene read. A fresh row replaces
// the stored one: the generated keep-richer merge would retain a field the
// vendor has since cleared (a suppression's approved_by), and a report would
// draw its conclusion from the stale value. Keep in sync with
// connectsecureLocalResources in internal/cli (a test checks it).
var connectsecureReplaceOnSync = map[string]bool{
	"agents": true, "asset_view": true, "companies": true, "company_stats": true,
	"suppress_vulnerability": true, "tags": true,
	"report_queries-ad-password-policies": true, "report_queries-ad-users-view": true,
	"report_queries-azure-secure-score": true, "report_queries-problems-summary-global": true,
	"report_queries-problems-summary-group-by-companies": true,
	"report_queries-remediation-velocity-company":        true, "report_queries-risk-score": true,
	"report_queries-sw-problems-remediations-view": true, "report_queries-vulnerabilities-details": true,
}

// ConnectsecureReplaceOnSync reports whether a stored row of resourceType is
// replaced, not merged, by a fresh one.
func ConnectsecureReplaceOnSync(resourceType string) bool {
	return connectsecureReplaceOnSync[resourceType]
}
