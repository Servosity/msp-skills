// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared plumbing for the fleet commands.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/fleet"
)

// exitFleetProblems is the typed exit code for --fail-on style alerting:
// the command ran fine but found problems at or above the requested level.
const exitFleetProblems = 8

func fleetProblemsErr(format string, a ...any) error {
	return &cliError{code: exitFleetProblems, err: fmt.Errorf(format, a...)}
}

// fleetScope selects which registered PBXes a fleet command reads.
type fleetScope struct {
	only        []string
	tags        []string
	concurrency int
	reg         *fleet.Registry // set by resolve
}

func addFleetScopeFlags(cmd *cobra.Command, s *fleetScope) {
	cmd.Flags().StringSliceVar(&s.only, "only", nil, "Limit to these registered PBX names (comma-separated)")
	cmd.Flags().StringSliceVar(&s.tags, "tag", nil, "Limit to PBXes carrying any of these tags")
	cmd.Flags().IntVar(&s.concurrency, "concurrency", 8, "PBXes queried in parallel")
}

// resolve returns the PBXes in scope and whether that is the whole
// registry. A pinned --pbx is authoritative: tags must not widen it.
func (s *fleetScope) resolve() ([]fleet.PBX, bool, error) {
	reg, err := fleet.Load()
	if err != nil {
		return nil, false, configErr(err)
	}
	s.reg = reg
	only, tags := s.only, s.tags
	if pinnedPBX != "" {
		only, tags = []string{pinnedPBX}, nil
	}
	pbxes, err := reg.Select(only, tags)
	if err != nil {
		return nil, false, usageErr(err)
	}
	return pbxes, len(only) == 0 && len(tags) == 0, nil
}

// collect fetches reqs from every PBX (one child process each) and turns
// the bodies into rows. A fetch or decode failure becomes errRow, rows keep
// fleet order, and failed counts the PBXes that could not be read.
func collect[R any](cmd *cobra.Command, s *fleetScope, flags *rootFlags, pbxes []fleet.PBX, reqs []fleet.Request,
	build func(fleet.PBX, []json.RawMessage) ([]R, error), errRow func(fleet.PBX, error) R) (rows []R, failed int) {
	r := fleet.Runner{Concurrency: s.concurrency, Timeout: flags.timeout, Registry: s.reg}
	per := make([][]R, len(pbxes))
	bad := make([]bool, len(pbxes))
	fleet.Each(cmd.Context(), s.concurrency, pbxes, func(ctx context.Context, i int, p fleet.PBX) {
		bodies, err := r.Fetch(ctx, p, reqs...)
		if err == nil {
			per[i], err = build(p, bodies)
		}
		if err != nil {
			per[i], bad[i] = []R{errRow(p, err)}, true
		}
	})
	rows = make([]R, 0, len(pbxes))
	for i := range per {
		rows = append(rows, per[i]...)
		if bad[i] {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d PBXes could not be read; their rows carry the error\n", failed, len(pbxes))
	}
	return rows, failed
}

// render prints rows: machine formats keep every row field even under --agent
// compaction; humans get a table or the empty message.
func render[R any](cmd *cobra.Command, flags *rootFlags, rows []R, keep []string, empty string, item func(R) map[string]any) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		if len(rows) == 0 && (flags.csv || flags.plain) {
			// An empty CSV/plain result still prints the column header so a
			// spreadsheet import or a cut/awk pipeline sees the columns
			// instead of a "(no rows)" marker.
			return printOutputWithFlagsMetaAndKeep(cmd.OutOrStdout(), json.RawMessage("[]"), flags, nil, keep, rowJSONFields[R]())
		}
		// Every field of a report row survives --agent: frequency compaction would
		// drop one a minority of rows carry (unknown, error), and that row then
		// reads as healthy.
		for _, field := range slices.Sorted(maps.Keys(rowJSONFields[R]())) {
			keep = append(keep, field)
		}
		return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, keep...)
	}
	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), empty)
		return nil
	}
	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, item(r))
	}
	return printAutoTable(cmd.OutOrStdout(), items)
}

// rowJSONFields lists the JSON field names of a row struct, used as the
// declared header for an empty CSV/plain result.
func rowJSONFields[R any]() map[string]bool {
	fields := map[string]bool{}
	t := reflect.TypeOf((*R)(nil)).Elem()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return fields
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fields[name] = true
	}
	return fields
}

// emptyFleet handles a run with no PBXes in scope. Machine formats still
// print an empty list; a human gets "no PBXes" instead of an all-clear line.
func emptyFleet(cmd *cobra.Command, flags *rootFlags) bool {
	hintEmptyFleet(cmd)
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		fmt.Fprintln(cmd.OutOrStdout(), "No PBXes in scope, so nothing was checked.")
		return true
	}
	return false
}

// hintEmptyFleet tells the caller how to register the first PBX. The
// "hint:" prefix lets the MCP layer pass it through to agents.
func hintEmptyFleet(cmd *cobra.Command) {
	fmt.Fprintf(cmd.ErrOrStderr(), "hint: no PBXes in scope; register one with: %s fleet add <name> --fqdn <host> --client-id <dn>\n", cmd.Root().Name())
}

// allFailed turns "every PBX in scope failed" into a non-zero exit after the
// error rows are printed; partial failures stay exit 0 with error rows.
func allFailed(failed, total int) error {
	if total > 0 && failed == total {
		return apiErr(fmt.Errorf("none of the %d PBX(es) in scope could be read; see the error field on each row", total))
	}
	return nil
}

// saveRun records a health or trunk run for 'fleet changes', unless the
// caller opted out, nothing was queried, or a test harness is running.
func saveRun(cmd *cobra.Command, noSave bool, snap fleet.Snapshot) {
	if noSave || len(snap.Scope) == 0 || cliutil.IsAnyHarness() {
		return
	}
	if err := fleet.SaveSnapshot(snap); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not save fleet history: %v\n", err)
	}
}

// decodeInto unmarshals bodies[i] into targets[i]. A slice target takes
// the OData collection's "value" array.
func decodeInto(bodies []json.RawMessage, targets ...any) error {
	for i, t := range targets {
		body := bodies[i]
		if reflect.TypeOf(t).Elem().Kind() == reflect.Slice {
			var page struct {
				Value json.RawMessage `json:"value"`
			}
			if err := json.Unmarshal(body, &page); err != nil {
				return err
			}
			if len(page.Value) == 0 {
				continue
			}
			body = page.Value
		}
		if err := json.Unmarshal(body, t); err != nil {
			return err
		}
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

// triState shows a verdict the PBX may not have reported: true, false, or
// "unknown" (never false) when it is nil.
func triState(b *bool) any {
	if b == nil {
		return "unknown"
	}
	return *b
}
