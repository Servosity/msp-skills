// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source live

package cli

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"jira-service-management-pp-cli/internal/cliutil"
)

type reconcileMissingAssets struct {
	Key  string            `json:"key"`
	Line int               `json:"line"`
	Row  map[string]string `json:"row"`
}

type reconcileMissingInventory struct {
	Key       string `json:"key"`
	ObjectKey string `json:"object_key"`
	Label     string `json:"label"`
}

type reconcileMismatch struct {
	Key       string `json:"key"`
	Line      int    `json:"line"`
	ObjectKey string `json:"object_key"`
	Field     string `json:"field"`
	Assets    string `json:"assets"`
	Inventory string `json:"inventory"`
}

type reconcileDuplicate struct {
	Key        string   `json:"key"`
	ObjectKeys []string `json:"object_keys,omitempty"`
	Lines      []int    `json:"lines,omitempty"`
}

type reconcileView struct {
	ObjectType               string                      `json:"object_type"`
	AssetsObjects            int                         `json:"assets_objects"`
	InventoryRows            int                         `json:"inventory_rows"`
	Matched                  int                         `json:"matched"`
	MissingInAssets          []reconcileMissingAssets    `json:"missing_in_assets"`
	UnverifiedInventory      []reconcileMissingAssets    `json:"unverified_inventory,omitempty"`
	MissingInInventory       []reconcileMissingInventory `json:"missing_in_inventory"`
	Mismatches               []reconcileMismatch         `json:"mismatches"`
	DuplicateAssetsKeys      []reconcileDuplicate        `json:"duplicate_assets_keys"`
	DuplicateInventoryKeys   []reconcileDuplicate        `json:"duplicate_inventory_keys"`
	AssetsWithoutKey         []string                    `json:"assets_objects_without_key"`
	InventoryLinesWithoutKey []int                       `json:"inventory_lines_without_key"`
	Truncated                bool                        `json:"truncated,omitempty"`
	Note                     string                      `json:"note,omitempty"`
}

func newNovelCmdbReconcileCmd(flags *rootFlags) *cobra.Command {
	var objType, file, key, assetsKey, compare, filter string
	var maxObjects int

	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Diff an Assets object type against an RMM/PSA CSV export: missing either side, plus attribute mismatches.",
		Long: strings.Trim(`
Walks every Assets object of --type (AQL objectType = "<type>"), flattens attribute
values to names, and joins them to a CSV export on --key (case-insensitive). Reports
rows missing from Assets, objects missing from the inventory, and differences in the
--compare columns. Column names match Assets attribute names; Key and Label are
also available on the Assets side. Needs Assets (JSM Premium/Enterprise) plus
JIRA_SERVICE_MANAGEMENT_CLOUD_ID and _WORKSPACE_ID, or run 'cmdb discover --save' once.

Use this command to diff Assets objects against an RMM/PSA inventory export file.
Do NOT use this command to find tickets connected to an object; use 'cmdb tickets' instead.`, "\n"),
		Example: strings.Trim(`
  jira-service-management-cli cmdb reconcile --type Laptop --file rmm.csv --key Name --compare Serial,OS --agent
  jira-service-management-cli cmdb reconcile --type Server --file psa.csv --key Hostname --assets-key Name`, "\n"),
		// Not mcp:read-only: it reads a local file the caller names, so MCP
		// hosts should ask before an agent runs it.
		Annotations: map[string]string{"pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "cmdb reconcile")
			}
			missing := make([]string, 0)
			for name, v := range map[string]string{"--type": objType, "--file": file, "--key": key} {
				if strings.TrimSpace(v) == "" {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				sort.Strings(missing)
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("%s required", strings.Join(missing, ", ")))
			}
			key, assetsKey = strings.TrimSpace(key), strings.TrimSpace(assetsKey)
			if assetsKey == "" {
				assetsKey = key
			}
			if maxObjects < 1 {
				return usageErr(fmt.Errorf("--max-objects must be at least 1"))
			}
			f, err := openInventoryFile(file)
			if err != nil {
				return usageErr(err)
			}
			headers, rows, err := readCSVRows(f, strings.ToLower(filepath.Ext(file)))
			_ = f.Close()
			if err != nil {
				return usageErr(fmt.Errorf("reading --file %s: %w", file, err))
			}
			compareCols := cliutil.SplitCSV(compare)
			for _, col := range append([]string{key}, compareCols...) {
				if !headerHas(headers, col) {
					// Header values are not echoed: --file can name any local file.
					return usageErr(fmt.Errorf("column %q is not in the header row of %s (%d columns); check the --key/--compare spelling", col, file, len(headers)))
				}
			}
			if cliutil.IsDogfoodEnv() && maxObjects > 100 {
				maxObjects = 100
			}
			ql := "objectType = " + aqlQuote(objType)
			if strings.TrimSpace(filter) != "" {
				ql += " AND (" + filter + ")"
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			objs, names, truncated, err := assetsAQL(ctx, c, ql, maxObjects)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			flat := make([]reconcileObject, 0, len(objs))
			for _, o := range objs {
				flat = append(flat, reconcileObject{ObjectKey: o.ObjectKey, Label: o.Label, Attrs: flattenObject(o, names)})
			}
			if len(flat) > 0 && !anyHasAttr(flat, assetsKey) {
				return usageErr(fmt.Errorf("no %s object has an attribute %q; pass --assets-key (attributes seen: %s)", objType, assetsKey, termSafe(strings.Join(attrNames(flat), ", "))))
			}
			// A compare column that is not an attribute of the type would report
			// every row as a mismatch; blank-everywhere attributes are fine.
			for _, col := range compareCols {
				if len(flat) > 0 && !attrDeclared(names, col) && !anyHasAttr(flat, col) {
					return usageErr(fmt.Errorf("%s has no attribute %q for --compare; Assets attributes seen: %s", objType, col, termSafe(strings.Join(attrNames(flat), ", "))))
				}
			}
			view := reconcileDiff(objType, flat, rows, key, assetsKey, compareCols, truncated)
			if truncated {
				view.Note = fmt.Sprintf("stopped at --max-objects %d; unmatched inventory rows are listed as unverified_inventory, not missing; raise --max-objects", maxObjects)
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%s: %d Assets objects, %d inventory rows, %d matched\n", objType, view.AssetsObjects, view.InventoryRows, view.Matched)
			fmt.Fprintf(w, "missing in Assets: %d\nmissing in inventory: %d\nmismatches: %d\nunverified: %d\nduplicate keys: %d Assets, %d inventory\nblank keys: %d Assets, %d inventory\n",
				len(view.MissingInAssets), len(view.MissingInInventory), len(view.Mismatches), len(view.UnverifiedInventory),
				len(view.DuplicateAssetsKeys), len(view.DuplicateInventoryKeys), len(view.AssetsWithoutKey), len(view.InventoryLinesWithoutKey))
			for _, m := range view.Mismatches {
				fmt.Fprintf(w, "  %s %s: assets=%q inventory=%q\n", termSafe(m.Key), termSafe(m.Field), termSafe(m.Assets), termSafe(m.Inventory))
			}
			if view.Note != "" {
				fmt.Fprintln(w, view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&objType, "type", "", "Assets object type name, e.g. Laptop")
	cmd.Flags().StringVar(&file, "file", "", "RMM/PSA export, first row the header: .csv (comma), .tsv (tab), or .txt (tab or comma, detected)")
	cmd.Flags().StringVar(&key, "key", "", "CSV column that identifies a device, e.g. Name or Serial")
	cmd.Flags().StringVar(&assetsKey, "assets-key", "", "Assets attribute matched against --key (default: same name as --key)")
	cmd.Flags().StringVar(&compare, "compare", "", "Columns to compare when both sides have the device, comma-separated, e.g. Serial,OS")
	cmd.Flags().StringVar(&filter, "aql-filter", "", "Extra AQL ANDed onto the object-type query, e.g. Status = \"In use\"")
	cmd.Flags().IntVar(&maxObjects, "max-objects", 5000, "Maximum Assets objects to read")
	return cmd
}

// maxInventoryBytes caps --file; RMM/PSA exports are far smaller.
const maxInventoryBytes = 64 << 20

// openInventoryFile opens a regular CSV/TSV/TXT file, never a symlink or a
// device. Under the MCP server the resolved path must also sit inside the
// working directory, and a working directory of / or $HOME is refused, so an
// agent cannot use reconcile to read arbitrary files (keys, tokens, password
// exports) back through missing_in_assets rows.
func openInventoryFile(path string) (*os.File, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".csv", ".tsv", ".txt":
	default:
		return nil, fmt.Errorf("--file must be a .csv, .tsv or .txt export")
	}
	if os.Getenv("JIRA_SERVICE_MANAGEMENT_LEARN_SURFACE") == "mcp" {
		if err := inventoryPathAllowedUnderMCP(path); err != nil {
			return nil, err
		}
	}
	f, err := openNoFollow(path) // per-OS twins: cmdb_reconcile_open_{unix,windows}.go
	if err != nil {
		return nil, fmt.Errorf("opening --file (symlinks are refused): %w", err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("--file must be a regular file, not a symlink or device")
	}
	if info.Size() > maxInventoryBytes {
		_ = f.Close()
		return nil, fmt.Errorf("--file is larger than %d MB", maxInventoryBytes>>20)
	}
	return f, nil
}

func inventoryPathAllowedUnderMCP(path string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	if wd, err = filepath.EvalSymlinks(wd); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		home, _ = filepath.EvalSymlinks(home)
	}
	if wd == string(filepath.Separator) || (home != "" && wd == home) {
		return fmt.Errorf("under MCP, run from a project directory (not / or your home directory) and put the export there")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if abs, err = filepath.EvalSymlinks(abs); err != nil {
		return fmt.Errorf("opening --file: %w", err)
	}
	if rel, err := filepath.Rel(wd, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("under MCP, --file must be inside the working directory")
	}
	return nil
}

type reconcileObject struct {
	ObjectKey string
	Label     string
	Attrs     map[string]string
}

type csvRow struct {
	Line   int
	Fields map[string]string
}

// readCSVRows reads a header row plus records. Every record must have the
// header's width (csv reports the line otherwise) and headers must be unique
// ignoring case, so no cell is silently dropped or overwritten.
func readCSVRows(r io.Reader, ext string) ([]string, []csvRow, error) {
	br := bufio.NewReader(r)
	tab := ext == ".tsv"
	if ext == ".txt" {
		// Excel "Text (Tab delimited)" saves .txt: sniff the header line.
		first, _ := br.Peek(64 << 10)
		line, _, _ := strings.Cut(string(first), "\n")
		tab = strings.Contains(line, "\t") && !strings.Contains(line, ",")
	}
	cr := csv.NewReader(br)
	if tab {
		cr.Comma = '\t'
		cr.LazyQuotes = true // `24" Monitor` is normal in tab exports
	}
	header, err := cr.Read()
	if err == io.EOF {
		return nil, nil, fmt.Errorf("file is empty")
	}
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]bool{}
	for i := range header {
		header[i] = strings.TrimSpace(strings.TrimPrefix(header[i], "\ufeff"))
		k := normKey(header[i])
		if k == "" {
			return nil, nil, fmt.Errorf("header column %d is blank", i+1)
		}
		if seen[k] {
			return nil, nil, fmt.Errorf("duplicate header in column %d", i+1)
		}
		seen[k] = true
	}
	rows := make([]csvRow, 0)
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		line, _ := cr.FieldPos(0)
		row := csvRow{Line: line, Fields: make(map[string]string, len(header))}
		for i, h := range header {
			row.Fields[h] = strings.TrimSpace(rec[i])
		}
		rows = append(rows, row)
	}
	return header, rows, nil
}

func normKey(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func headerHas(headers []string, name string) bool {
	for _, h := range headers {
		if strings.EqualFold(h, strings.TrimSpace(name)) {
			return true
		}
	}
	return false
}

// lookupFold finds a map value by case-insensitive key.
func lookupFold(m map[string]string, k string) (string, bool) {
	if v, ok := m[k]; ok {
		return v, true
	}
	for mk, v := range m {
		if strings.EqualFold(mk, k) {
			return v, true
		}
	}
	return "", false
}

// attrDeclared reports whether the object type declares attr (from the AQL
// response's objectTypeAttributes), whether or not any object fills it.
func attrDeclared(names map[string]string, attr string) bool {
	for _, n := range names {
		if strings.EqualFold(strings.TrimSpace(n), strings.TrimSpace(attr)) {
			return true
		}
	}
	return false
}

func anyHasAttr(objs []reconcileObject, attr string) bool {
	for _, o := range objs {
		if v, ok := lookupFold(o.Attrs, attr); ok && strings.TrimSpace(v) != "" {
			return true
		}
	}
	return false
}

func attrNames(objs []reconcileObject) []string {
	set := map[string]bool{}
	for _, o := range objs {
		for k := range o.Attrs {
			set[k] = true
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// reconcileDiff is the pure join behind cmdb reconcile. Every input row and
// object is accounted for: duplicates and blank keys are reported, not
// dropped. When the Assets scan was truncated, unmatched inventory rows are
// "unverified" because the object may sit past the cap.
func reconcileDiff(objType string, objs []reconcileObject, rows []csvRow, key, assetsKey string, compare []string, truncated bool) reconcileView {
	view := reconcileView{ObjectType: objType, AssetsObjects: len(objs), InventoryRows: len(rows), Truncated: truncated,
		MissingInAssets: make([]reconcileMissingAssets, 0), MissingInInventory: make([]reconcileMissingInventory, 0),
		Mismatches: make([]reconcileMismatch, 0), DuplicateAssetsKeys: make([]reconcileDuplicate, 0),
		DuplicateInventoryKeys: make([]reconcileDuplicate, 0), AssetsWithoutKey: make([]string, 0), InventoryLinesWithoutKey: make([]int, 0)}

	byKey := map[string][]reconcileObject{}
	assetOrder := make([]string, 0)
	for _, o := range objs {
		v, _ := lookupFold(o.Attrs, assetsKey)
		k := normKey(v)
		if k == "" {
			view.AssetsWithoutKey = append(view.AssetsWithoutKey, o.ObjectKey)
			continue
		}
		if _, ok := byKey[k]; !ok {
			assetOrder = append(assetOrder, k)
		}
		byKey[k] = append(byKey[k], o)
	}
	for _, k := range assetOrder {
		if group := byKey[k]; len(group) > 1 {
			d := reconcileDuplicate{Key: k}
			for _, o := range group {
				d.ObjectKeys = append(d.ObjectKeys, o.ObjectKey)
			}
			view.DuplicateAssetsKeys = append(view.DuplicateAssetsKeys, d)
		}
	}

	invLines := map[string][]int{}
	invOrder := make([]string, 0)
	for _, row := range rows {
		v, _ := lookupFold(row.Fields, key)
		k := normKey(v)
		if k == "" {
			view.InventoryLinesWithoutKey = append(view.InventoryLinesWithoutKey, row.Line)
			continue
		}
		if _, ok := invLines[k]; !ok {
			invOrder = append(invOrder, k)
		}
		invLines[k] = append(invLines[k], row.Line)
		group, ok := byKey[k]
		if !ok {
			miss := reconcileMissingAssets{Key: v, Line: row.Line, Row: row.Fields}
			if truncated {
				view.UnverifiedInventory = append(view.UnverifiedInventory, miss)
			} else {
				view.MissingInAssets = append(view.MissingInAssets, miss)
			}
			continue
		}
		view.Matched++
		for _, col := range compare {
			inv, _ := lookupFold(row.Fields, col)
			for _, o := range group {
				as, _ := lookupFold(o.Attrs, col)
				if normKey(inv) != normKey(as) {
					view.Mismatches = append(view.Mismatches, reconcileMismatch{Key: v, Line: row.Line, ObjectKey: o.ObjectKey, Field: col, Assets: as, Inventory: inv})
				}
			}
		}
	}
	for _, k := range invOrder {
		if lines := invLines[k]; len(lines) > 1 {
			view.DuplicateInventoryKeys = append(view.DuplicateInventoryKeys, reconcileDuplicate{Key: k, Lines: lines})
		}
	}
	for _, k := range assetOrder {
		if _, ok := invLines[k]; ok {
			continue
		}
		for _, o := range byKey[k] {
			v, _ := lookupFold(o.Attrs, assetsKey)
			view.MissingInInventory = append(view.MissingInInventory, reconcileMissingInventory{Key: v, ObjectKey: o.ObjectKey, Label: o.Label})
		}
	}
	return view
}
