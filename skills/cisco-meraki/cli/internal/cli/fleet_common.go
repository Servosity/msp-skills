// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Shared plumbing for the `fleet` commands. One Meraki API key reaches every
// organization its admin can see; these helpers list those orgs, fan a read
// out across them under the client's rate limiter, and keep each org's
// failure attached to that org instead of dropping it or counting it as zero.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cisco-meraki-pp-cli/internal/client"
	"cisco-meraki-pp-cli/internal/cliutil"
)

// fleetConcurrency stays under the Dashboard's 10 req/s per-org budget; the
// shared client limiter (--rate-limit, default 10) caps the global rate.
const fleetConcurrency = 4

type fleetOrg struct {
	ID             string
	Name           string
	LicensingModel string
	APIEnabled     bool
}

// errOrgAPIDisabled marks an org whose Dashboard API access is turned off.
var errOrgAPIDisabled = errors.New("Dashboard API access is disabled for this organization")

// fleetOrgFilter matches --org values against an org ID exactly or its name
// case-insensitively. An empty filter keeps every org.
func fleetOrgFilter(values []string) func(fleetOrg) bool {
	want := make([]string, 0, len(values))
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if p := strings.TrimSpace(part); p != "" {
				want = append(want, strings.ToLower(p))
			}
		}
	}
	return func(o fleetOrg) bool {
		if len(want) == 0 {
			return true
		}
		for _, w := range want {
			if strings.ToLower(o.ID) == w || strings.ToLower(o.Name) == w {
				return true
			}
		}
		return false
	}
}

// loadFleetOrgs lists every organization the key can see (all Link pages)
// and applies the --org filter. Under live dogfood it keeps the first three
// orgs so a fan-out fits the matrix timeout.
func loadFleetOrgs(ctx context.Context, c *client.Client, orgFilter []string) ([]fleetOrg, error) {
	res, err := merakiGetAll(ctx, c, "/organizations", nil, 0)
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}
	keep := fleetOrgFilter(orgFilter)
	orgs := make([]fleetOrg, 0, len(res.Items))
	for _, raw := range res.Items {
		var o struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			API  *struct {
				Enabled bool `json:"enabled"`
			} `json:"api"`
			Licensing struct {
				Model string `json:"model"`
			} `json:"licensing"`
		}
		if json.Unmarshal(raw, &o) != nil || o.ID == "" {
			continue
		}
		org := fleetOrg{ID: o.ID, Name: o.Name, LicensingModel: o.Licensing.Model, APIEnabled: o.API == nil || o.API.Enabled}
		if keep(org) {
			orgs = append(orgs, org)
		}
	}
	if len(orgFilter) > 0 && len(orgs) == 0 {
		return nil, notFoundErr(fmt.Errorf("no organization matches --org %s (the key can see %d organizations)", strings.Join(orgFilter, ","), len(res.Items)))
	}
	sort.SliceStable(orgs, func(i, j int) bool { return strings.ToLower(orgs[i].Name) < strings.ToLower(orgs[j].Name) })
	if cliutil.IsDogfoodEnv() && len(orgs) > 3 {
		orgs = orgs[:3]
	}
	return orgs, nil
}

// fleetClientOrgs builds the client and lists the orgs in scope. Page walks
// read past the response cache anyway, so caching is off: nothing reads those
// per-cursor files back.
func fleetClientOrgs(ctx context.Context, cmd *cobra.Command, flags *rootFlags, orgFilter []string) (*client.Client, []fleetOrg, error) {
	c, err := flags.newClient()
	if err != nil {
		return nil, nil, err
	}
	c.NoCache = true
	orgs, err := loadFleetOrgs(ctx, c, orgFilter)
	if err != nil {
		return nil, nil, classifyAPIError(cmd.OutOrStdout(), err, flags)
	}
	return c, orgs, nil
}

func addOrgFlag(cmd *cobra.Command, orgFilter *[]string) {
	cmd.Flags().StringSliceVar(orgFilter, "org", nil, "Limit to organizations by ID or exact name (repeatable or comma-separated)")
}

// decodeItems decodes list items, skipping any that do not fit T.
func decodeItems[T any](items []json.RawMessage) []T {
	out := make([]T, 0, len(items))
	for _, raw := range items {
		var v T
		if json.Unmarshal(raw, &v) == nil {
			out = append(out, v)
		}
	}
	return out
}

func noMirrorHint(dbPath string) string {
	return fmt.Sprintf("no local mirror at %s\nrun: cisco-meraki-cli sync --resources organizations --db %s", dbPath, dbPath)
}

type orgOutcome[T any] struct {
	Org   fleetOrg
	Value T
	Err   error
}

// fanOrgs runs fn once per org and returns one outcome per org in input
// order, success or failure, so callers can report every gap.
func fanOrgs[T any](ctx context.Context, orgs []fleetOrg, fn func(context.Context, fleetOrg) (T, error)) []orgOutcome[T] {
	results, errs := cliutil.FanoutRun(ctx, orgs, func(o fleetOrg) string { return o.ID }, fn, cliutil.WithConcurrency(fleetConcurrency))
	byID := make(map[string]orgOutcome[T], len(orgs))
	for _, r := range results {
		byID[r.Source] = orgOutcome[T]{Value: r.Value}
	}
	for _, e := range errs {
		byID[e.Source] = orgOutcome[T]{Err: e.Err}
	}
	out := make([]orgOutcome[T], 0, len(orgs))
	for _, o := range orgs {
		oc := byID[o.ID]
		oc.Org = o
		out = append(out, oc)
	}
	return out
}

// reportFleetFailures writes the partial-failure warning the build contract
// requires: the failed count, the denominator, and each failed org.
func reportFleetFailures[T any](w io.Writer, outcomes []orgOutcome[T]) {
	failed := 0
	for _, oc := range outcomes {
		if oc.Err != nil {
			failed++
		}
	}
	if failed == 0 {
		return
	}
	fmt.Fprintf(w, "warning: %d of %d organizations failed; results cover the remaining %d\n", failed, len(outcomes), len(outcomes)-failed)
	for _, oc := range outcomes {
		if oc.Err != nil {
			fmt.Fprintf(w, "  %s (%s): %s\n", oc.Org.Name, oc.Org.ID, oc.Err.Error())
		}
	}
}

// partialRows carries one org's rows plus a note when a secondary read for
// that org failed, so a late failure does not discard the rows already read.
type partialRows[T any] struct {
	Rows    []T
	Partial string
}

func warnPartial(w io.Writer, org fleetOrg, partial string) {
	if partial != "" {
		fmt.Fprintf(w, "warning: %s (%s): partial result, %s\n", org.Name, org.ID, partial)
	}
}

// orgPath substitutes the organization ID into an org-scoped path.
func orgPath(template, orgID string) string {
	return strings.ReplaceAll(template, "{organizationId}", url.PathEscape(orgID))
}

// fleetNetworks maps network ID -> name for one org (all pages).
func fleetNetworks(ctx context.Context, c *client.Client, orgID string) (map[string]string, error) {
	res, err := merakiGetAll(ctx, c, orgPath("/organizations/{organizationId}/networks", orgID), nil, 0)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(res.Items))
	for _, n := range decodeItems[struct{ ID, Name string }](res.Items) {
		if n.ID != "" {
			names[n.ID] = n.Name
		}
	}
	return names, nil
}

// parseWindow parses a --since/--within style duration (7d, 24h, 2w) and
// rejects zero or negative values.
func parseWindow(flag, value string) (time.Duration, error) {
	d, err := cliutil.ParseDurationLoose(value)
	if err != nil {
		return 0, usageErr(fmt.Errorf("--%s %q: %w (use forms like 24h, 7d, 2w)", flag, value, err))
	}
	if d <= 0 {
		return 0, usageErr(fmt.Errorf("--%s must be positive, got %q", flag, value))
	}
	return d, nil
}

// fixedWindow turns a lookback into absolute t0/t1 bounds so a slow read cannot
// return events newer than the "now" the caller computes with. A window at the
// endpoint's maximum keeps five minutes of margin so t0 is never past the
// API's lookback limit by the time the request lands.
func fixedWindow(now time.Time, window, max time.Duration) (t0, t1 time.Time, clamped bool) {
	if window >= max {
		window, clamped = max-5*time.Minute, window > max
	}
	return now.Add(-window), now, clamped
}

func parseMerakiTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "Jan 2, 2006 MST", "Jan _2, 2006 MST"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// humanDuration renders minutes as a compact 3d4h / 5h12m / 7m string.
func humanDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	m := int64(d / time.Minute)
	switch {
	case m >= 1440:
		return fmt.Sprintf("%dd%dh", m/1440, (m%1440)/60)
	case m >= 60:
		return fmt.Sprintf("%dh%dm", m/60, m%60)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// printFleetRows routes rows to JSON/CSV/select output, or renders the named
// columns as a human table.
func printFleetRows(cmd *cobra.Command, flags *rootFlags, rows any, columns []string, empty string) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFiltered(cmd.OutOrStdout(), rows, flags)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	var items []map[string]any
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), empty)
		return nil
	}
	table := make([][]string, 0, len(items))
	for _, it := range items {
		row := make([]string, 0, len(columns))
		for _, col := range columns {
			row = append(row, cellString(it[col]))
		}
		table = append(table, row)
	}
	return flags.printTable(cmd, columns, table)
}

func cellString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return cliutil.ScrubTerminal(x)
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%.1f", x)
	case bool:
		if x {
			return "yes"
		}
		return "no"
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, cellString(p))
		}
		return strings.Join(parts, ",")
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// isHTTPStatus reports whether err is an API error with the given status.
func isHTTPStatus(err error, status int) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == status
}
