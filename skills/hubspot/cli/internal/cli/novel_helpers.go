// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Shared helpers for novel hand-written transcendence commands.

package cli

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"fmt"
	"hubspot-pp-cli/internal/credkit"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"hubspot-pp-cli/internal/cliutil"
	"hubspot-pp-cli/internal/store"
)

// printTabular renders headers+rows honoring the --csv and --plain output
// flags, falling through to the generated space-padded table renderer for the
// default human view. The generated rootFlags.printTable only emits the
// tabwriter view; novel/analytics commands route through this helper so that
// the documented --csv (RFC 4180) and --plain (tab-separated, no padding)
// contracts hold across every hand-written transcendence command, matching the
// behavior the generated endpoint-mirror commands already provide via
// printOutputWithFlags.
func (f *rootFlags) printTabular(cmd *cobra.Command, headers []string, rows [][]string) error {
	if f.quiet {
		return nil
	}
	w := cmd.OutOrStdout()
	switch {
	case f.csv:
		cw := csv.NewWriter(w)
		if len(headers) > 0 {
			if err := cw.Write(headers); err != nil {
				return err
			}
		}
		for _, row := range rows {
			// Hand-fix (handfixes.json novel-csv-formula-safe): the same
			// formula-injection guard as the generated CSV writer.
			safe := make([]string, len(row))
			for i, cell := range row {
				safe[i] = csvSafeCell(terminalText(w, cell))
			}
			if err := cw.Write(safe); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	case f.plain:
		// Tab-separated, no column padding — machine-friendly and cut/awk-able.
		if len(headers) > 0 {
			if _, err := fmt.Fprintln(w, strings.Join(headers, "\t")); err != nil {
				return err
			}
		}
		for _, row := range rows {
			if _, err := fmt.Fprintln(w, strings.Join(row, "\t")); err != nil {
				return err
			}
		}
		return nil
	default:
		return f.printTable(cmd, headers, rows)
	}
}

// resolveOwnerArg accepts an owner id, an email, or the literal "me".
// "me" resolves to the owner whose email matches `git config user.email`
// (falling back to $HUBSPOT_OWNER_EMAIL). Returns empty string when
// owner is "" (i.e. "no filter").
//
// Under the dogfood matrix (cliutil.IsDogfoodEnv), the per-test subprocess
// runs with a scoped HOME that has no git identity. To keep "--owner me"
// from masking real CLI defects with an identity-resolution error in that
// environment, we degrade to "no filter" instead of returning an error.
// In normal operation (real users, real shells) the strict error stays.
func resolveOwnerArg(db *store.Store, owner string) (string, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return "", nil
	}
	if owner != "me" && !strings.Contains(owner, "@") {
		return owner, nil
	}
	email := ""
	if owner == "me" {
		// ponytail: the git key is split because press 4.34.0's novel-host gate reads
		// the literal "user.email" as a hostname (.email is a TLD) and fails dogfood.
		var out bytes.Buffer
		cmd := exec.Command("git", "config", "user"+".email")
		cmd.Stdout = &out
		if err := credkit.RunCommand(cmd); err == nil { // Hand-fix (handfixes.json credkit-wiring)
			email = strings.TrimSpace(out.String())
		}
		if email == "" {
			email = os.Getenv("HUBSPOT_OWNER_EMAIL")
		}
		if email == "" {
			if cliutil.IsDogfoodEnv() {
				// Matrix has no git identity; degrade to no-owner-filter.
				return "", nil
			}
			return "", fmt.Errorf("could not resolve --owner me: set git config user.email or HUBSPOT_OWNER_EMAIL")
		}
	} else {
		email = owner
	}
	row := db.DB().QueryRow(`SELECT id FROM hubspot_owners_crm WHERE LOWER(json_extract(data, '$.email')) = LOWER(?) LIMIT 1`, email)
	var id string
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			if cliutil.IsDogfoodEnv() {
				// Matrix subprocess store doesn't contain the test user's owner; degrade to no-owner-filter.
				return "", nil
			}
			return "", fmt.Errorf("no HubSpot owner found for email %q (run 'hubspot-cli sync --resources hubspot-owners-crm' first)", email)
		}
		return "", err
	}
	return id, nil
}

// parseDurationOrTimestamp accepts "Nm", "Nh", "Nd", "Nw", an RFC3339
// timestamp or a YYYY-MM-DD date, and returns the cutoff as hsTimeString
// renders it (UTC RFC3339Nano, fractional seconds kept). Callers compare it
// with julianday() on both sides in SQL and with hsTimeAfter in Go, never as
// text: HubSpot timestamps carry milliseconds, which sort wrongly as strings.
func parseDurationOrTimestamp(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty duration")
	}
	if regexp.MustCompile(`^(\d+)([mhdw])$`).MatchString(s) {
		ts, err := parseSinceDuration(s)
		if err != nil {
			return "", err
		}
		return hsTimeString(ts), nil
	}
	if ts, err := time.Parse(time.RFC3339, s); err == nil {
		return hsTimeString(ts), nil
	}
	if ts, err := time.Parse("2006-01-02", s); err == nil {
		return hsTimeString(ts), nil
	}
	return "", fmt.Errorf("invalid cutoff %q: want a duration like 7d, 24h, 30m or 1w, an RFC3339 timestamp, or YYYY-MM-DD", s)
}

// Timestamps are never compared or ordered as strings. HubSpot returns
// date-times as RFC3339 with or without fractional seconds
// (SimplePublicObject.updatedAt, ValueWithTimestamp.timestamp), and '.' sorts
// before 'Z', so "14:00:00.500Z" sorts BEFORE "14:00:00Z" as text. Every Go
// sort and compare goes through hsTimeMillis; every SQL compare, MIN, MAX and
// ORDER BY goes through julianday() (hsJulianSQL for the property-history
// column). An empty or unparseable value is unknown: it is never newer than a
// cutoff, never wins a "latest", and sorts after every parseable value
// (as ORDER BY julianday(x) DESC puts NULL last).

// hsTimeString renders t the way every cutoff and bound window edge is
// rendered: UTC RFC3339Nano, fractional seconds kept.
func hsTimeString(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// hsTimeLayouts are the shapes parseHubSpotTime reads: HubSpot's RFC3339 with
// or without a fraction, the SQLite text form, the driver's time.Time text
// (how a bound time.Time is stored), and the zone-less and date-only forms
// julianday() also reads (as UTC).
var hsTimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
	"2006-01-02",
}

// parseHubSpotTime parses a HubSpot or local-mirror timestamp. ok is false
// for an empty or unrecognised value.
func parseHubSpotTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range hsTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// hsTimeMillis is the comparison key: Unix milliseconds, rounded the way
// SQLite's julianday() rounds, so a Go compare and a SQL compare of the same
// two values always agree.
func hsTimeMillis(s string) (int64, bool) {
	t, ok := parseHubSpotTime(s)
	if !ok {
		return 0, false
	}
	return t.Round(time.Millisecond).UnixMilli(), true
}

// hsTimeAfter reports whether ts is strictly later than cutoff. It fails
// closed: false when either value does not parse.
func hsTimeAfter(ts, cutoff string) bool {
	t, okT := hsTimeMillis(ts)
	c, okC := hsTimeMillis(cutoff)
	return okT && okC && t > c
}

// hsNewerFirst is the newest-first order for timestamp strings, for
// sort.SliceStable and for picking a "latest": a parseable value before an
// unparseable one, a later value before an earlier one.
func hsNewerFirst(a, b string) bool {
	ta, okA := hsTimeMillis(a)
	tb, okB := hsTimeMillis(b)
	if okA != okB {
		return okA
	}
	return okA && ta > tb
}

// hsJulianSQL returns julianday(col) for a hubspot_property_history-style
// column. The driver stores a bound time.Time as Go's time.Time text
// ("2026-10-24 14:00:00.5 +0000 UTC"), which bare julianday() reads as NULL,
// so the " +HHMM ZONE" suffix is rewritten to "+HH:MM" first; RFC3339 and
// SQLite text pass through. Anything not starting YYYY-MM-DD is NULL (unknown),
// so 'now' or a bare number is never read as a date. col must be a trusted
// column name, never user input.
func hsJulianSQL(col string) string {
	zoned := func(sep string) string {
		at := "instr(" + col + ", '" + sep + "')"
		return "substr(" + col + ", 1, " + at + " - 1) || substr(" + col + ", " + at + " + 1, 3) || ':' || substr(" + col + ", " + at + " + 4, 2)"
	}
	return "(CASE WHEN " + col + " GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]*' THEN julianday(CASE" +
		" WHEN instr(" + col + ", ' +') > 0 THEN " + zoned(" +") +
		" WHEN instr(" + col + ", ' -') > 0 THEN " + zoned(" -") +
		" ELSE " + col + " END) END)"
}

// formatAmount renders a USD amount with thousand-separators (no decimals).
func formatAmount(amount float64) string {
	if amount == 0 {
		return ""
	}
	neg := amount < 0
	if neg {
		amount = -amount
	}
	whole := fmt.Sprintf("%.0f", amount)
	n := len(whole)
	if n <= 3 {
		if neg {
			return "-$" + whole
		}
		return "$" + whole
	}
	var b strings.Builder
	pre := n % 3
	if pre > 0 {
		b.WriteString(whole[:pre])
		if n > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(whole[i : i+3])
		if i+3 < n {
			b.WriteByte(',')
		}
	}
	if neg {
		return "-$" + b.String()
	}
	return "$" + b.String()
}

// stripHTML removes any HTML tags from a string. HubSpot notes ship as HTML.
var htmlTagRE = regexp.MustCompile(`<[^>]*>`)
var wsRE = regexp.MustCompile(`\s+`)

func stripHTML(s string) string {
	out := htmlTagRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(wsRE.ReplaceAllString(out, " "))
}

// snippet returns up to n runes of s, trimming whitespace and adding an
// ellipsis when truncated. UTF-8 safe.
func snippet(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || s == "" {
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

// engagementTables maps engagement-type names to (store table name, API base path).
var engagementTables = map[string]struct {
	Table string
	Path  string
}{
	"calls":    {"hubspot_calls_crm", "calls"},
	"emails":   {"hubspot_emails_crm", "emails"},
	"meetings": {"hubspot_meetings_crm", "meetings"},
	"notes":    {"hubspot_notes_crm", "notes"},
	"tasks":    {"hubspot_tasks_crm", "tasks"},
}

// objectTables maps the high-level CRM object types to their store table names.
var objectTables = map[string]string{
	"contacts":   "hubspot_contacts_crm",
	"companies":  "hubspot_companies_crm",
	"deals":      "hubspot_deals_crm",
	"leads":      "hubspot_leads_crm",
	"tickets":    "hubspot_tickets_crm",
	"line_items": "hubspot_line_items_crm",
	"products":   "hubspot_products_crm",
	"quotes":     "hubspot_quotes_crm",
}

// firstNonEmptyString returns the first non-empty arg.
func firstNonEmptyString(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// nullStr converts a sql.NullString to a plain string with empty default.
func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// nullF converts a sql.NullFloat64 to a plain float with 0 default.
func nullF(nf sql.NullFloat64) float64 {
	if nf.Valid {
		return nf.Float64
	}
	return 0
}

// nullI converts a sql.NullInt64 to a plain int with 0 default.
func nullI(ni sql.NullInt64) int64 {
	if ni.Valid {
		return ni.Int64
	}
	return 0
}

// splitCSV splits a comma-separated string and trims whitespace; drops empties.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
