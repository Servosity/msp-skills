// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: shared plumbing for the fleet commands.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"prtg-pp-cli/internal/cliutil"
	"prtg-pp-cli/internal/fleet"
)

// fleetExe overrides the child binary; tests point it at a fake.
var fleetExe string

// fleetScope selects which registered cores a fleet command reads.
type fleetScope struct {
	only        []string
	tags        []string
	concurrency int
	reg         *fleet.Registry // set by resolve
}

func addFleetScopeFlags(cmd *cobra.Command, s *fleetScope) {
	cmd.Flags().StringSliceVar(&s.only, "cores", nil, "Limit to these registered core names (comma-separated)")
	cmd.Flags().StringSliceVar(&s.tags, "tag", nil, "Limit to cores carrying any of these tags")
	cmd.Flags().IntVar(&s.concurrency, "concurrency", 8, "Cores queried in parallel")
}

// resolve returns the cores in scope. A pinned --core is authoritative:
// tags must not widen it.
func (s *fleetScope) resolve() ([]fleet.Core, error) {
	reg, err := fleet.Load()
	if err != nil {
		return nil, configErr(err)
	}
	s.reg = reg
	only, tags := s.only, s.tags
	if pinnedCore != "" {
		// --core (or PRTG_CORE) pins this process to one core. A fleet
		// selection that names anything else is a conflict, never silently
		// replaced, so a write cannot land on a core the caller did not pick.
		if len(tags) > 0 || len(only) > 1 || (len(only) == 1 && only[0] != pinnedCore) {
			return nil, usageErr(fmt.Errorf("--core/PRTG_CORE %q conflicts with --cores/--tag; use one or the other", pinnedCore))
		}
		only, tags = []string{pinnedCore}, nil
	}
	cores, err := reg.Select(only, tags)
	if err != nil {
		return nil, usageErr(err)
	}
	return cores, nil
}

// resolveOrEmpty resolves the cores in scope; done is true when there are
// none and a human-readable notice was printed instead (machine formats go
// on to print an empty list).
func (s *fleetScope) resolveOrEmpty(cmd *cobra.Command, flags *rootFlags) (cores []fleet.Core, done bool, err error) {
	if cores, err = s.resolve(); err != nil {
		return nil, false, err
	}
	if len(cores) == 0 {
		return cores, s.emptyFleet(cmd, flags), nil
	}
	return cores, false, nil
}

func (s *fleetScope) runner(flags *rootFlags) fleet.Runner {
	return fleet.Runner{Exe: fleetExe, Timeout: flags.timeout, Insecure: skipTLSVerify()}
}

// collect fetches reqs from every core (one child process each) and turns
// the bodies into rows. A fetch or decode failure becomes errRow, rows keep
// fleet order, and failed counts the cores that could not be read.
func collect[R any](cmd *cobra.Command, s *fleetScope, flags *rootFlags, cores []fleet.Core, reqs []fleet.Request,
	build func(fleet.Core, []json.RawMessage) ([]R, error), errRow func(fleet.Core, error) R) (rows []R, failed int) {
	r := s.runner(flags)
	per := make([][]R, len(cores))
	bad := make([]bool, len(cores))
	fleet.Each(cmd.Context(), s.concurrency, cores, func(ctx context.Context, i int, c fleet.Core) {
		bodies, err := r.Fetch(ctx, c, reqs...)
		if err == nil {
			per[i], err = build(c, bodies)
		}
		if err != nil {
			per[i], bad[i] = []R{errRow(c, err)}, true
		}
	})
	rows = make([]R, 0, len(cores))
	for i := range per {
		rows = append(rows, per[i]...)
		if bad[i] {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: %d of %d cores could not be read; their rows carry the error\n", failed, len(cores))
	}
	return rows, failed
}

// bodyOrErr returns body unless it is a per-request failure from the child.
func bodyOrErr(body json.RawMessage) (json.RawMessage, error) {
	if e, ok := fleet.RequestError(body); ok {
		return nil, fmt.Errorf("%s", e.Message)
	}
	return body, nil
}

// render prints rows. Machine formats keep every field of the row type, so
// --agent compaction never drops the sparse who/why/error fields. Humans get
// a table with the given columns in order and full values; with no columns
// (fleet exec's free-form rows) the generic table is used.
func render[R any](cmd *cobra.Command, flags *rootFlags, rows []R, cols []string, empty string) error {
	if !wantsHumanTable(cmd.OutOrStdout(), flags) {
		fields := rowJSONFields[R]()
		keep := slices.Sorted(maps.Keys(fields))
		if len(keep) == 0 {
			keep = []string{"core", "error"}
		}
		if len(rows) == 0 && (flags.csv || flags.plain) {
			// An empty CSV/plain result still prints the column header.
			return printOutputWithFlagsMetaAndKeep(cmd.OutOrStdout(), json.RawMessage("[]"), flags, nil, keep, fields)
		}
		return printJSONFilteredKeep(cmd.OutOrStdout(), rows, flags, keep...)
	}
	if len(rows) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), empty)
		return nil
	}
	items, ok := any(rows).([]map[string]any) // fleet exec rows are maps already
	if !ok {
		data, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
	}
	if len(cols) == 0 {
		return printAutoTable(cmd.OutOrStdout(), items)
	}
	return printColumns(cmd.OutOrStdout(), items, cols)
}

// printColumns writes a tab-aligned table with the given columns in order,
// leaving out columns that are empty on every row.
func printColumns(w io.Writer, items []map[string]any, cols []string) error {
	used := make([]string, 0, len(cols))
	for _, c := range cols {
		for _, it := range items {
			if cell(it[c]) != "" {
				used = append(used, c)
				break
			}
		}
	}
	tw := newTabWriter(w)
	head := make([]string, len(used))
	for i, c := range used {
		head[i] = strings.ToUpper(c)
	}
	fmt.Fprintln(tw, strings.Join(head, "\t"))
	for _, it := range items {
		vals := make([]string, len(used))
		for i, c := range used {
			// API text (names, messages) must not drive the terminal;
			// ScrubTerminal also turns tabs into spaces.
			vals[i] = cliutil.ScrubTerminal(cell(it[c]))
		}
		fmt.Fprintln(tw, strings.Join(vals, "\t"))
	}
	return tw.Flush()
}

func cell(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(t))
		for _, x := range t {
			parts = append(parts, cell(x))
		}
		return strings.Join(parts, "; ")
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
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

// emptyFleet handles a run with no cores in scope. Machine formats still
// print an empty list; a human gets "no cores" instead of an all-clear line.
func (s *fleetScope) emptyFleet(cmd *cobra.Command, flags *rootFlags) bool {
	if s.reg != nil && len(s.reg.Cores) > 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: no registered core matches the --cores/--tag selection, so nothing was checked")
	} else {
		hintEmptyFleet(cmd)
	}
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		fmt.Fprintln(cmd.OutOrStdout(), "No cores in scope, so nothing was checked.")
		return true
	}
	return false
}

// allFailed turns "every core in scope failed" into a non-zero exit after the
// error rows are printed; partial failures stay exit 0 with error rows.
func allFailed(failed, total int) error {
	if total > 0 && failed == total {
		return apiErr(fmt.Errorf("none of the %d core(s) in scope could be read; see the error field on each row", total))
	}
	return nil
}

// parseWindow reads a duration flag such as 12h, 7d, 2w or 90m.
func parseWindow(flag, v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	d, err := cliutil.ParseDurationLoose(v)
	if err != nil || d < 0 {
		return 0, usageErr(fmt.Errorf("--%s %q: use a duration like 30m, 12h, 7d or 2w", flag, v))
	}
	return d, nil
}

// alarmListRequest pages the core's alarm list (PRTG's "sensors with
// alarms": down, warning, unusual and acknowledged sensors).
func alarmListRequest(filter string) fleet.Request {
	params := map[string]string{} // GETALL pages at PRTG's 3000-row maximum
	if strings.TrimSpace(filter) != "" {
		params["filter"] = filter
	}
	return fleet.Request{Method: fleet.MethodGetAll, Path: "/sensors/alarms", Params: params}
}
