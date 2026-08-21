// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored shared plumbing for the ImmyBot novel commands. Kept in its own
// file so `generate --force` preserves it.
//
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// immyMirrorPath resolves the local SQLite mirror path, honouring an explicit
// --db value and otherwise falling back to the generated default location.
func immyMirrorPath(dbPath string) string {
	if strings.TrimSpace(dbPath) != "" {
		return dbPath
	}
	return defaultDBPath("immybot-pp-cli")
}

// immyMirrorMissing reports whether the local mirror is absent and, when it is,
// prints an actionable sync hint to stderr. Callers must still emit an empty
// machine-readable result so agents receive valid output rather than a SQLite
// open failure. A missing mirror is an empty-cache state, not a usage error.
func immyMirrorMissing(cmd *cobra.Command, dbPath, syncResources string) bool {
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"no local mirror at %s\nrun: immybot-pp-cli sync --resources %s --db %s\n",
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
	immyGUIDRe   = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	immyPathRe   = regexp.MustCompile(`(?i)[a-z]:\\[^\s"']+`)
	immyNumberRe = regexp.MustCompile(`\b\d+\b`)
	immyWSRe     = regexp.MustCompile(`\s+`)
)

// immyNormalizeReason collapses a per-machine failure message into a cluster
// key. Machine-specific detail -- GUIDs, Windows paths, bare numbers -- is
// replaced with placeholders so the same underlying failure on forty endpoints
// collapses to one cluster. This is deliberately mechanical string work, not
// classification: no judgement about what the message means.
func immyNormalizeReason(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = immyGUIDRe.ReplaceAllString(s, "<id>")
	s = immyPathRe.ReplaceAllString(s, "<path>")
	s = immyNumberRe.ReplaceAllString(s, "<n>")
	s = immyWSRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// immyIsFailureStatus reports whether a maintenance-action status name reads as
// a failure. ImmyBot's status enum is not published in the spec, so this
// matches the conventional failure vocabulary and is overridable per command
// with an explicit --status filter.
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
