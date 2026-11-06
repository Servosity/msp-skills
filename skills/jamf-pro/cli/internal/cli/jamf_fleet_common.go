// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared plumbing for the fleet commands.

package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/fleet"
)

// exitFleetProblems is the typed exit code for --fail-on style alerting:
// the command ran fine but found problems at or above the requested level.
const exitFleetProblems = 8

func fleetProblemsErr(format string, a ...any) error {
	return &cliError{code: exitFleetProblems, err: fmt.Errorf(format, a...)}
}

// fleetExe overrides the child binary; tests point it at a fake.
var fleetExe string

// fleetScope selects which registered instances a fleet command reads.
type fleetScope struct {
	only        []string
	tags        []string
	concurrency int
	freshToken  bool            // children mint a new token (instance test)
	reg         *fleet.Registry // set by resolve
}

func addFleetScopeFlags(cmd *cobra.Command, s *fleetScope) {
	cmd.Flags().StringSliceVar(&s.only, "only", nil, "Limit to these registered instance names (comma-separated)")
	cmd.Flags().StringSliceVar(&s.tags, "tag", nil, "Limit to instances carrying any of these tags")
	cmd.Flags().IntVar(&s.concurrency, "concurrency", 8, "Instances queried in parallel")
}

// resolve returns the instances in scope. A pinned --instance is
// authoritative: tags must not widen it.
func (s *fleetScope) resolve() ([]fleet.Instance, error) {
	reg, err := fleet.Load()
	if err != nil {
		return nil, configErr(err)
	}
	s.reg = reg
	only, tags := s.only, s.tags
	if pinnedInstance != "" {
		only, tags = []string{pinnedInstance}, nil
	}
	instances, err := reg.Select(only, tags)
	if err != nil {
		return nil, usageErr(err)
	}
	return instances, nil
}

// collect fetches reqs from every instance (one child process each) and turns
// the bodies into rows. A fetch or decode failure becomes errRow, rows keep
// fleet order, and failed counts the instances that could not be read.
func collect[R any](cmd *cobra.Command, s *fleetScope, flags *rootFlags, instances []fleet.Instance, reqs []fleet.Request,
	build func(fleet.Instance, []json.RawMessage) ([]R, error), errRow func(fleet.Instance, error) R) (rows []R, failed int) {
	return collectFn(cmd, s, flags, instances, func(fetch fetchFunc, p fleet.Instance) ([]R, error) {
		bodies, err := fetch(reqs...)
		if err != nil {
			return nil, err
		}
		return build(p, bodies)
	}, errRow)
}

// fetchFunc runs GETs against one instance in a single child process.
type fetchFunc func(reqs ...fleet.Request) ([]json.RawMessage, error)

// collectFn is collect for commands whose later requests depend on earlier
// answers (a list, then one call per item): build gets a fetch bound to the
// instance and may call it more than once.
func collectFn[R any](cmd *cobra.Command, s *fleetScope, flags *rootFlags, instances []fleet.Instance,
	build func(fetch fetchFunc, p fleet.Instance) ([]R, error), errRow func(fleet.Instance, error) R) (rows []R, failed int) {
	r := fleet.Runner{Exe: fleetExe, Concurrency: s.concurrency, Timeout: flags.timeout, Registry: s.reg, FreshToken: s.freshToken}
	per := make([][]R, len(instances))
	bad := make([]bool, len(instances))
	fleet.Each(cmd.Context(), s.concurrency, instances, func(ctx context.Context, i int, p fleet.Instance) {
		fetch := func(reqs ...fleet.Request) ([]json.RawMessage, error) { return r.Fetch(ctx, p, reqs...) }
		out, err := build(fetch, p)
		if err != nil {
			per[i], bad[i] = []R{errRow(p, err)}, true
			return
		}
		per[i] = out
	})
	rows = make([]R, 0, len(instances))
	for i := range per {
		rows = append(rows, per[i]...)
		if bad[i] {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d instances could not be read; their rows carry the error\n", failed, len(instances))
	}
	return rows, failed
}

// jamfGet is a one-request helper for building fleet.Request values.
func jamfGet(path string, kv ...string) fleet.Request {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Add(kv[i], kv[i+1])
	}
	return fleet.Request{Path: path, Query: q}
}

// render prints rows. JSON keeps the sparse fields named in keep even under
// --agent compaction. Human tables, CSV and plain text use cols in that order
// (instance first), so columns never shuffle between runs, and list values
// are joined with commas instead of JSON-encoded.
func render[R any](cmd *cobra.Command, flags *rootFlags, rows []R, keep []string, empty string, cols []string, item func(R) map[string]any) error {
	tabular := flags.csv || flags.plain
	if !tabular && !wantsHumanTable(cmd.OutOrStdout(), flags) {
		return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, keep...)
	}
	if sel := strings.TrimSpace(flags.selectFields); sel != "" && tabular {
		want := map[string]bool{}
		for _, f := range strings.Split(sel, ",") {
			want[strings.TrimSpace(f)] = true
		}
		var kept []string
		for _, c := range cols {
			if want[c] {
				kept = append(kept, c)
			}
		}
		if len(kept) > 0 {
			cols = kept
		}
	}
	if len(rows) == 0 && !tabular {
		fmt.Fprintln(cmd.OutOrStdout(), empty)
		return nil
	}
	table := make([][]string, 0, len(rows))
	for _, r := range rows {
		m := item(r)
		line := make([]string, len(cols))
		for i, c := range cols {
			// Device names and other values come from the instance: strip
			// terminal controls so a crafted name cannot rewrite the report.
			line[i] = cliutil.ScrubTerminal(cellString(m[c]))
		}
		table = append(table, line)
	}
	out := cmd.OutOrStdout()
	switch {
	case flags.csv:
		w := csv.NewWriter(out)
		if err := w.Write(cols); err != nil {
			return err
		}
		// Values come from the instance: a name is data in a spreadsheet, never a formula.
		for _, line := range table {
			for i := range line {
				line[i] = csvSafeCell(line[i])
			}
		}
		if err := w.WriteAll(table); err != nil {
			return err
		}
		w.Flush()
		return w.Error()
	case flags.plain:
		for _, line := range table {
			fmt.Fprintln(out, strings.Join(line, "\t"))
		}
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = strings.ToUpper(c)
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, line := range table {
		fmt.Fprintln(tw, strings.Join(line, "\t"))
	}
	return tw.Flush()
}

// cellString renders one table cell; lists are comma-joined.
func cellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []string:
		return strings.Join(t, ",")
	case []any:
		parts := make([]string, len(t))
		for i, x := range t {
			parts[i] = cellString(x)
		}
		return strings.Join(parts, ",")
	case *int:
		if t == nil {
			return ""
		}
		return strconv.Itoa(*t)
	case *bool:
		if t == nil {
			return ""
		}
		return strconv.FormatBool(*t)
	case map[string]any:
		b, _ := json.Marshal(t)
		return string(b)
	}
	return fmt.Sprint(v)
}

// emptyFleet handles a run with no instances in scope. Machine formats still
// print an empty list; a human gets "nothing checked" instead of an all-clear.
// emptyFleet reports that no instance is in scope. Machine output says so
// explicitly ({"instances_in_scope":0,...}) so an agent never reads an empty
// list as "everything checked and clean".
func emptyFleet(cmd *cobra.Command, flags *rootFlags) bool {
	hintEmptyFleet(cmd)
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		fmt.Fprintln(cmd.OutOrStdout(), "No instances in scope, so nothing was checked.")
		return true
	}
	_ = printJSONFiltered(cmd.OutOrStdout(), map[string]any{"instances_in_scope": 0, "checked": false,
		"note": "no instances in scope, so nothing was checked; register one with 'instance add'"}, flags)
	return true
}

// hintEmptyFleet tells the caller how to register the first instance. The
// "hint:" prefix lets the MCP layer pass it through to agents.
func hintEmptyFleet(cmd *cobra.Command) {
	fmt.Fprintf(cmd.ErrOrStderr(), "hint: no instances in scope; register one with: %s instance add <name> --server <host> --client-id <id> --secret-env <ENV_VAR>\n", cmd.Root().Name())
}

// allFailed turns "every instance in scope failed" into a non-zero exit after
// the error rows are printed; partial failures stay exit 0 with error rows.
func allFailed(failed, total int) error {
	if total > 0 && failed == total {
		return apiErr(fmt.Errorf("none of the %d instance(s) in scope could be read; see the error field on each row", total))
	}
	return nil
}

// optDays reads an optional day-window flag; -1 means not set.
func optDays(flag, v string) (int, error) {
	if strings.TrimSpace(v) == "" {
		return -1, nil
	}
	return parseDays(flag, v)
}

// parseDays reads "30", "30d", "6w" or "720h" as whole days.
func parseDays(flag, v string) (int, error) {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return n, nil
	}
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("--%s %q: use days like 30d, 6w or 30", flag, v))
	}
	return int(d.Hours() / 24), nil
}

// jamfPageTotal reads a Jamf Pro API page's totalCount (0 included).
func jamfPageTotal(obj map[string]json.RawMessage) (int, bool) {
	raw, ok := obj["totalCount"]
	var n int
	if !ok || json.Unmarshal(raw, &n) != nil {
		return 0, false
	}
	return n, true
}

// jamfCheckWalk fails a finished generic --all walk over Jamf Pro API pages
// that did not read each record exactly once: the total changed between
// pages, a record repeated, or the record count differs from totalCount (the
// walker also stops early at its page cap or on an empty page).
func jamfCheckWalk(path string, items []json.RawMessage, totals []int) error {
	if len(totals) == 0 {
		return nil
	}
	for _, n := range totals[1:] {
		if n != totals[0] {
			return apiErr(fmt.Errorf("%s: totalCount changed from %d to %d while paging; run again", path, totals[0], n))
		}
	}
	if err := noRepeats(path, items, map[string]bool{}); err != nil {
		return err
	}
	if len(items) != totals[0] {
		return apiErr(fmt.Errorf("%s: read %d of %d records; --all reads at most %d pages, so raise --page-size for a large collection, or run again", path, len(items), totals[0], paginatedGetMaxPages))
	}
	return nil
}
