// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored shared plumbing for the ImmyBot novel commands. Kept in its own
// file so `generate --force` preserves it.
//
// pp:data-source local

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"immybot-pp-cli/internal/store"
)

// immyMirrorPath resolves the local SQLite mirror path, honouring an explicit
// --db value and otherwise falling back to the generated default location.
func immyMirrorPath(dbPath string) string {
	if strings.TrimSpace(dbPath) != "" {
		return dbPath
	}
	return defaultDBPath("immybot-cli")
}

// immyMirrorMissing reports whether the local mirror is absent and, when it is,
// prints an actionable sync hint to stderr. Callers must still emit an empty
// machine-readable result so agents receive valid output rather than a SQLite
// open failure. A missing mirror is an empty-cache state, not a usage error.
func immyMirrorMissing(cmd *cobra.Command, dbPath, syncResources string) bool {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"no local mirror at %s\nrun: immybot-cli sync --resources %s --db %s\n",
			dbPath, syncResources, dbPath)
		return true
	}
	return false
}

// nullStr copies a nullable SQLite text column, treating NULL as "". Every
// json_extract of an optional field can return NULL, and scanning that into a
// bare string errors, which silently drops the whole row inside a scan loop.
func nullStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

var (
	immyGUIDRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	immyPathRe = regexp.MustCompile(`(?i)(?:[a-z]:\\|\\\\)[^\s"']+`)
	// ISO-8601 and US-style timestamps, and bare clock times.
	immyTimeRe = regexp.MustCompile(`(?i)\b\d{4}-\d{2}-\d{2}(?:[t ]\d{1,2}:\d{2}(?::\d{2}(?:\.\d+)?)?(?:z|[+-]\d{2}:?\d{2})?)?\b|` +
		`\b\d{1,2}/\d{1,2}/\d{4}(?: \d{1,2}:\d{2}(?::\d{2})?(?: ?[ap]m)?)?|\b\d{1,2}:\d{2}:\d{2}(?:\.\d+)?(?: ?[ap]m)?`)
	// A positive decimal of six or more digits reads as a row, session or
	// process id. Shorter numbers (exit codes 1603, 1618, 3010), signed
	// decimal HRESULTs and 0x hex codes are kept: they name the cause.
	immyLongIDRe = regexp.MustCompile(`(^|[^\w-])\d{6,}\b`)
	immyWSRe     = regexp.MustCompile(`\s+`)
)

// immyNormalizeReason collapses a per-machine failure message into a cluster
// key. What varies per machine -- GUIDs, file paths, timestamps, long ids --
// is replaced with placeholders so the same underlying failure on forty
// endpoints collapses to one cluster. Exit codes and HRESULTs are kept, so
// distinct causes never merge. Deliberately mechanical string work, not
// classification: no judgement about what the message means.
func immyNormalizeReason(s string) string {
	return immyNormalizeReasonFor(s, "")
}

// immyNormalizeReasonFor is immyNormalizeReason for one computer's message: the
// computer's own name, which differs on every row, is masked as well.
func immyNormalizeReasonFor(s, computerName string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if name := strings.TrimSpace(computerName); name != "" {
		// Word boundaries where the name starts or ends with a word
		// character, so a short name never rewrites part of another word.
		pat := regexp.QuoteMeta(name)
		if immyWordByte(name[0]) {
			pat = `\b` + pat
		}
		if immyWordByte(name[len(name)-1]) {
			pat += `\b`
		}
		s = regexp.MustCompile(`(?i)`+pat).ReplaceAllString(s, "<computer>")
	}
	s = immyGUIDRe.ReplaceAllString(s, "<id>")
	s = immyPathRe.ReplaceAllString(s, "<path>")
	s = immyTimeRe.ReplaceAllString(s, "<time>")
	s = immyLongIDRe.ReplaceAllString(s, "${1}<n>")
	s = immyWSRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// immyIsFailureStatus reports whether one outcome or status name reads as a
// failure. It is the vocabulary behind immyParseActionOutcome
// (immybot_action_outcome.go), which every action-outcome reader must use: the
// vendor puts the outcome in result/resultName (MaintenanceActionResult), and
// statusName is only the lifecycle.
func immyIsFailureStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "errored", "faulted", "cancelled", "canceled", "timedout", "timeout":
		return true
	}
	return false
}

// immyTruncate bounds a display string so a single pathological failure message
// cannot dominate table output.
func immyTruncate(s string, n int) string {
	if n <= 3 || len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// immyCompareVersions orders two dotted version strings numerically segment by
// segment, falling back to a string compare for non-numeric segments. Returns
// -1, 0 or 1. Plain string ordering sorts "9.0" above "140.0", which is the
// specific bug this exists to avoid.
func immyCompareVersions(a, b string) int {
	as := immySplitVersion(a)
	bs := immySplitVersion(b)
	n := len(as)
	if len(bs) > n {
		n = len(bs)
	}
	for i := range n {
		// A missing trailing segment is an implicit zero: "2.0" and "2.0.0"
		// name the same release and must compare equal.
		av, bv := "0", "0"
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		ai, aerr := strconv.Atoi(av)
		bi, berr := strconv.Atoi(bv)
		if aerr == nil && berr == nil {
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
			continue
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

func immySplitVersion(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if v == "" {
		return nil
	}
	return strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '+' || r == '_'
	})
}

// immyOpenMirror opens the local mirror read-only. Every mirror command only
// reads (fleet-diff --snapshot opens its own writable handle), and a
// read-write open would run schema migrations as a side effect and contend
// with a running sync for SQLite's single writer.
func immyOpenMirror(ctx context.Context, dbPath string) (*store.Store, error) {
	return store.OpenReadOnlyContext(ctx, dbPath)
}

// immyTenantNames maps mirrored tenant ids to names. A mirror without tenants
// yields an empty map: callers show the id alone rather than failing.
func immyTenantNames(ctx context.Context, db *store.Store) (map[string]string, error) {
	names := map[string]string{}
	rows, err := db.DB().QueryContext(ctx,
		`SELECT id, json_extract(data,'$.name') FROM resources WHERE resource_type = 'tenants'`)
	if err != nil {
		if immyNoSuchTableErr(err) {
			return names, nil
		}
		return nil, fmt.Errorf("querying tenants: %w", err)
	}
	for rows.Next() {
		var id, name sql.NullString
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		names[nullStr(id)] = nullStr(name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterating tenants: %w", err)
	}
	return names, rows.Close()
}

// immyNoSuchTableErr reports SQLite's missing-table error: a read-only open of
// a store that sync never wrote has no resources table yet.
func immyNoSuchTableErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// immyPrintable replaces control characters (C0, DEL, C1) with spaces for the
// human table, so a vendor string cannot move the cursor or recolor the
// terminal. JSON output is left exact.
func immyPrintable(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return ' '
		}
		return r
	}, s)
}

// immyTenantLabel names a row's tenant: tenantName, else "tenant <tenantId>"
// (tenantName is nullable in the vendor shape; a row without it is still a
// tenant's computer and must not drop out of a per-tenant report).
func immyTenantLabel(name, id string) string {
	if n := strings.TrimSpace(name); n != "" {
		return n
	}
	if id = strings.TrimSpace(id); id != "" {
		return "tenant " + id
	}
	return ""
}

// immyParseTime parses a vendor date-time: RFC 3339 (fractional seconds
// allowed), or the offset-less form .NET writes for an unspecified-kind
// DateTime, read as UTC.
func immyParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02T15:04:05.999999999", s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

func immyWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
