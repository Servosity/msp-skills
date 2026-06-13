// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

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
func tlOpenStore(ctx context.Context, dbPath string) (*store.Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("threatlocker-cli")
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
