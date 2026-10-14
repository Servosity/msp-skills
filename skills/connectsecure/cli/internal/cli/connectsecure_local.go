// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored helpers shared by the ConnectSecure novel commands.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"connectsecure-pp-cli/internal/cliutil"
	"connectsecure-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// Sync resource names the local novel commands read. They are part of the
// curated default `sync` set.
const (
	csResCompanies        = "companies"
	csResAgents           = "agents"
	csResAssets           = "asset_view"
	csResSuppressions     = "suppress_vulnerability"
	csResProblemsByCo     = "report_queries-problems-summary-group-by-companies"
	csResProblemsGlobal   = "report_queries-problems-summary-global"
	csResRemediations     = "report_queries-sw-problems-remediations-view"
	csResVelocity         = "report_queries-remediation-velocity-company"
	csResRiskScore        = "report_queries-risk-score"
	csResADUsers          = "report_queries-ad-users-view"
	csResADPolicies       = "report_queries-ad-password-policies"
	csResAzureSecureScore = "report_queries-azure-secure-score"
)

type csRow = map[string]any

// connectsecureDefaultSyncResources is the curated set a bare `sync` pulls:
// every resource the local novel commands read, plus company stats, tags and
// the vulnerability details report. The generated defaultSyncResources()
// delegates here (hand-fix curated-default-sync); every other resource stays
// reachable with `sync --resources <name>`.
func connectsecureDefaultSyncResources() []string {
	return []string{
		csResAgents, csResAssets, csResCompanies, "company_stats", csResSuppressions, "tags",
		csResADPolicies, csResADUsers, csResAzureSecureScore,
		csResProblemsGlobal, csResProblemsByCo, csResVelocity, csResRiskScore, csResRemediations,
		"report_queries-vulnerabilities-details",
	}
}

// csLocalData loads the named resources from the synced store (read-only)
// and prints one stderr hint naming any resource that was never synced. With
// no local mirror it returns no data and every resource as unsynced, so the
// caller emits an empty result instead of failing. Hints carry the "hint:"
// prefix so the MCP shell-out forwards them to agents.
func csLocalData(cmd *cobra.Command, flags *rootFlags, dbPath string, resources ...string) (map[string][]csRow, []string, error) {
	if dbPath == "" {
		dbPath = defaultDBPath("connectsecure-cli")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no local mirror at %s; run: connectsecure-cli sync --resources %s\n", dbPath, strings.Join(resources, ","))
		return nil, resources, nil
	}
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	db, err := store.OpenReadOnlyContext(ctx, dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening local store: %w", err)
	}
	defer db.Close()
	unsynced, partial := make([]string, 0), make([]string, 0)
	staleChecked := false
	data := make(map[string][]csRow, len(resources))
	for _, r := range resources {
		state, err := readSyncHintState(db, r)
		switch {
		case err == nil && !state.hasState:
			unsynced = append(unsynced, r)
		case err == nil && !staleChecked:
			staleChecked = hintIfStale(cmd, db, r, flags.maxAge)
		}
		var complete int
		if db.DB().QueryRowContext(ctx, `SELECT last_attempt_complete FROM sync_state WHERE resource_type = ?`, r).Scan(&complete) == nil && complete == 0 {
			partial = append(partial, r)
		}
		if data[r], err = csLocalRows(ctx, db, r); err != nil {
			return nil, nil, err
		}
	}
	if len(unsynced) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: no synced data yet for %s; run: connectsecure-cli sync --resources %s\n", strings.Join(unsynced, ", "), strings.Join(unsynced, ","))
	}
	if len(partial) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "hint: the last sync of %s did not finish, so these results may be incomplete; run: connectsecure-cli sync --full --resources %s\n", strings.Join(partial, ", "), strings.Join(partial, ","))
	}
	return data, unsynced, nil
}

// csLocalRows returns every synced row of one resource, decoded. Rows are
// drained before returning so callers may run follow-up queries.
// ponytail: decodes whole rows; project needed keys with json_extract if
// 20k+ asset tenants make exposure/portfolio slow.
func csLocalRows(ctx context.Context, db *store.Store, resourceType string) ([]csRow, error) {
	rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type = ?`, resourceType)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", resourceType, err)
	}
	defer rows.Close()
	out := make([]csRow, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scanning %s: %w", resourceType, err)
		}
		var obj csRow
		if json.Unmarshal([]byte(raw), &obj) == nil && obj != nil {
			out = append(out, obj)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating %s: %w", resourceType, err)
	}
	return out, nil
}

// csIndex maps key -> row; rows with an empty key are skipped, last one wins.
func csIndex(rows []csRow, key string) map[string]csRow {
	out := make(map[string]csRow, len(rows))
	for _, r := range rows {
		if k := csString(r[key]); k != "" {
			out[k] = r
		}
	}
	return out
}

// csCompanyMatch implements --company: a case-insensitive name substring or
// an exact company id. An empty filter matches everything.
func csCompanyMatch(filter, id, name string) bool {
	f := strings.ToLower(strings.TrimSpace(filter))
	return f == "" || id == f || strings.Contains(strings.ToLower(name), f)
}

func csString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == math.Trunc(t) && math.Abs(t) < 1e15 {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case json.Number:
		return t.String()
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func csFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	case bool:
		if t {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

func csFloat0(v any) float64 {
	f, _ := csFloat(v)
	return f
}

// csBool reads true/false that the API may encode as bool, 0/1, or "true"/"yes".
func csBool(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "1", "y":
			return true
		}
	}
	return false
}

func csStringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		if s := csString(v); s != "" {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s := csString(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

var csTimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02",
}

// csTime parses the timestamp shapes ConnectSecure returns: RFC3339, naive
// ISO, space-separated, date-only, or epoch seconds/milliseconds. Zero,
// negative and non-finite epochs mean "never" and report false.
func csTime(v any) (time.Time, bool) {
	f, isNum := v.(float64)
	if !isNum {
		s := csString(v)
		if s == "" {
			return time.Time{}, false
		}
		for _, layout := range csTimeLayouts {
			if t, err := time.Parse(layout, s); err == nil {
				return t.UTC(), true
			}
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}, false
		}
		f = n
	}
	switch {
	case math.IsNaN(f) || math.IsInf(f, 0) || f <= 0:
		return time.Time{}, false
	case f > 1e12:
		return time.UnixMilli(int64(f)).UTC(), true
	default:
		return time.Unix(int64(f), 0).UTC(), true
	}
}

// csDuration parses a --flag duration that accepts 7d/2w shorthand; "" or "0" disables.
func csDuration(flagName, value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}
	d, err := cliutil.ParseDurationLoose(value)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("--%s %q is not a duration (examples: 24h, 14d, 2w)", flagName, value))
	}
	return d, nil
}

func csSeverityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// csSeveritySet parses --severity and rejects values ConnectSecure never uses,
// so a typo cannot silently drop a whole severity from a report.
func csSeveritySet(s string) (map[string]bool, error) {
	set := csCSVSet(s)
	for v := range set {
		if csSeverityRank(v) == 0 {
			return nil, usageErr(fmt.Errorf("--severity %q is not one of Critical, High, Medium, Low", v))
		}
	}
	return set, nil
}

func csCSVSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, p := range cliutil.SplitCSV(s) {
		out[strings.ToLower(p)] = true
	}
	return out
}

// csCompanyNames maps company id -> name from synced companies.
func csCompanyNames(companies []csRow) map[string]string {
	out := make(map[string]string, len(companies))
	for _, c := range companies {
		if id := csString(c["id"]); id != "" {
			out[id] = csString(c["name"])
		}
	}
	return out
}

// csKEVProblems returns the problem names the synced remediation view marks
// as CISA KEV. ConnectSecure fills `cisa` only for KEV entries; an explicit
// negative marker is ignored.
func csKEVProblems(remediations []csRow) map[string]bool {
	out := map[string]bool{}
	for _, r := range remediations {
		mark := csString(r["cisa"])
		switch strings.ToLower(mark) {
		case "", "no", "false", "0", "n/a", "none", "null":
			continue
		}
		if name := csString(r["problem_name"]); name != "" {
			out[name] = true
		}
	}
	return out
}

func csAssetName(a csRow) string {
	for _, k := range []string{"host_name", "visible_name", "name", "ip"} {
		if s := csString(a[k]); s != "" {
			return s
		}
	}
	return ""
}

func csRound(f float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(f*p) / p
}

func csMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	s := append([]float64(nil), values...)
	sort.Float64s(s)
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// csEmit prints rows as a {meta, results} envelope for machine output (the
// shape generated list commands use), or a table for humans. When a resource
// the report needs was never synced, meta.unsynced_resources says so and the
// human message tells the user to sync, so an empty result never reads as an
// all-clear.
func csEmit[T any](cmd *cobra.Command, flags *rootFlags, rows []T, unsynced []string, empty string) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		meta := map[string]any{"source": "local"}
		if len(unsynced) > 0 {
			meta["unsynced_resources"] = unsynced
		}
		return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"meta": meta, "results": rows}, flags)
	}
	if len(rows) == 0 {
		if len(unsynced) > 0 {
			empty = "No local data for this report yet: run 'connectsecure-cli sync' first."
		}
		fmt.Fprintln(cmd.OutOrStdout(), empty)
		return nil
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return err
	}
	return printAutoTable(cmd.OutOrStdout(), items)
}
