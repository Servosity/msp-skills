// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Shared helpers for the hand-built ThreatLocker transcendence commands
// (approvals triage/approve-batch, audit export/retention-check/drift,
// devices health). These are NOT generated; they implement the cross-tenant
// SQLite-backed features that differentiate this CLI from the read-only,
// single-tenant incumbents.
package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"threatlocker-pp-cli/internal/store"
)

// isTLTokenWellFormed validates a ThreatLocker Portal API token against its
// known shape: a 64-character lowercase hex string sent raw (no "Bearer "
// prefix) in the Authorization header. Returns a human reason either way — this
// powers the doctor's diagnosis of the recurring community 401.
func isTLTokenWellFormed(token string) (bool, string) {
	t := strings.TrimSpace(token)
	if t == "" {
		return false, "no token set"
	}
	if strings.HasPrefix(strings.ToLower(t), "bearer ") {
		return false, "token must NOT include a 'Bearer ' prefix; send the raw token as the Authorization header value"
	}
	if len(t) != 64 {
		return false, fmt.Sprintf("token is %d chars; ThreatLocker API tokens are 64 lowercase-hex chars", len(t))
	}
	for _, c := range t {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false, "token has non lowercase-hex characters; expected ^[a-f0-9]{64}$"
		}
	}
	return true, "well-formed (64 hex)"
}

// tlAuthDiagnosis maps a 401/403 to the most likely ThreatLocker-specific cause,
// given what the doctor already knows about the local config.
func tlAuthDiagnosis(hasToken, tokenOK, hasOrg bool) string {
	switch {
	case !hasToken:
		return "401/403 likely cause: no API token configured. Set THREATLOCKER_API_KEY to a 64-hex token from the portal (Administrators > API Users)."
	case !tokenOK:
		return "401/403 likely cause: malformed token — see token_format. Regenerate under Administrators > API Users."
	case !hasOrg:
		return "401/403 likely cause: missing ManagedOrganizationId. The New API version requires it — set THREATLOCKER_ORG_ID or pass --org <tenant-guid>."
	default:
		return "401/403 likely cause: token expired. ThreatLocker tokens renew-on-use and expire when idle; regenerate the API token in the portal. (Also confirm you are on the New API version, not the deprecated Old API.)"
	}
}

// tlNovelTenantScope is the one rule every local-store novel READ uses to pick
// the tenant its rows are filtered to. allTenants is the command's
// --all-tenants value; a command without that flag passes false. Only the
// explicit --org filters local rows: THREATLOCKER_ORG_ID scopes live requests
// but never hides local rows, because an MSP that sets it to its parent
// organization would otherwise see none of its child tenants' rows (see
// tenantOrgFlag in tenant_scope.go). Hand-wired (class: tenant scope of
// local-store commands).
func tlNovelTenantScope(allTenants bool) string {
	if allTenants {
		return ""
	}
	return tenantOrgFlag
}

// tlRequireTenantScope is the tenant scope for a command that WRITES per
// tenant. It uses the operator's tenant setting exactly as the HTTP client
// resolves it (--org, else THREATLOCKER_ORG_ID), so a write never reaches a
// tenant the operator did not choose, and it fails closed when neither
// --all-tenants nor a tenant is set, so a write never spans every synced
// tenant by default. The scope is returned either way, so a dry-run can show
// the refusal next to the plan.
func tlRequireTenantScope(allTenants bool) (string, error) {
	scope := ""
	if !allTenants {
		scope = ResolveTenantOrg(tenantOrgFlag)
	}
	if !allTenants && scope == "" {
		return "", fmt.Errorf("no tenant selected: pass --org <tenant-guid> or set %s to act on one tenant, or pass --all-tenants to act on every synced tenant", TenantOrgEnvVar)
	}
	return scope, nil
}

// tlTimeLayouts are the datetime shapes ThreatLocker timestamps appear in
// across ActionLog, approval, and audit payloads. Tried in order.
var tlTimeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"01/02/2006 15:04:05",
	"01/02/2006",
}

// parseTLTime parses a ThreatLocker timestamp string. Returns ok=false when the
// value is empty or matches no known layout, so callers can distinguish
// "unparseable age" from a real zero time rather than silently fabricating one.
func parseTLTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range tlTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// ageBucketOf classifies an age into the triage buckets MSP admins reason in.
func ageBucketOf(d time.Duration) string {
	switch {
	case d < 0:
		return "unknown"
	case d >= 7*24*time.Hour:
		return ">7d"
	case d >= 48*time.Hour:
		return ">48h"
	case d >= 24*time.Hour:
		return ">24h"
	default:
		return "<24h"
	}
}

// resolveSince accepts either a relative window ("7d", "12h", "30m") or an
// absolute datetime, and returns the cutoff instant. ok=false when the input
// is non-empty but unparseable (callers should error rather than guess).
func resolveSince(s string, now time.Time) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, true // empty means "no lower bound"
	}
	if len(s) >= 2 {
		unit := s[len(s)-1]
		if n, err := strconv.Atoi(s[:len(s)-1]); err == nil {
			switch unit {
			case 'd', 'D':
				return now.Add(-time.Duration(n) * 24 * time.Hour), true
			case 'h', 'H':
				return now.Add(-time.Duration(n) * time.Hour), true
			case 'm', 'M':
				return now.Add(-time.Duration(n) * time.Minute), true
			}
		}
	}
	if t, ok := parseTLTime(s); ok {
		return t, true
	}
	return time.Time{}, false
}

// tlFreshOnlineJoin is the one rule devices health and organizations posture
// use to join online_devices to computers (aliased c and od). A sync never
// prunes an online_devices row the API stopped returning, so a row counts as
// evidence the endpoint is online only when it was synced no earlier than the
// computers row it joins; an older one would outvote the computer's fresher
// status. julianday() compares instants, so the RFC 3339 stamps the store
// writes and SQLite's CURRENT_TIMESTAMP default order correctly. Hand-wired
// (class: stale join membership).
const tlFreshOnlineJoin = `LEFT JOIN online_devices od ON od.computer_id = c.computer_id
	AND julianday(od.synced_at) >= julianday(c.synced_at)`

// tlString extracts a non-null string from a *string scan target.
func tlString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// tlOpenStore opens the local mirror for the read-only novel commands:
// read-only when the schema exists (no write lock -> no SQLITE_BUSY under
// parallel reads), else read-write-migrate, with a short retry to ride out the
// first-run create/migrate race. Non-nil on a nil error.
//
// Under THREATLOCKER_NO_LOCAL_DATA=1 it opens an existing store read-only and
// never creates or migrates one: reading local data stays available, writing
// any is refused. Hand-wired (class: local-data opt-out bypass).
func tlOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("threatlocker-cli")
	}
	if localDataDisabled() {
		if st, ok := tlTryReadOnlyMigrated(dbPath); ok {
			return st, nil
		}
		return nil, fmt.Errorf("no readable local store at %s, and %s=1 forbids creating one", dbPath, noLocalDataEnvVar)
	}
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		if st, ok := tlTryReadOnlyMigrated(dbPath); ok {
			return st, nil
		}
		st, err := store.OpenWithContext(ctx, dbPath)
		if err == nil {
			return st, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		time.Sleep(time.Duration(20*(attempt+1)) * time.Millisecond)
	}
	return nil, lastErr
}

func tlTryReadOnlyMigrated(path string) (*store.Store, bool) {
	if _, err := os.Stat(path); err != nil {
		return nil, false
	}
	st, err := store.OpenReadOnly(path)
	if err != nil {
		return nil, false
	}
	var one int
	if err := st.DB().QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name='resources' LIMIT 1`).Scan(&one); err != nil {
		_ = st.Close()
		return nil, false
	}
	return st, true
}
