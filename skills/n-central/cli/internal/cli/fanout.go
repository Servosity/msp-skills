// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel feature. Not generated.

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"n-central-pp-cli/internal/store"
)

// fanoutResult is one merged search hit annotated with the server it came from.
// The server + resourceType fields lead so cross-tenant results read cleanly,
// and the original record is flattened in via the inline map.
type fanoutResult struct {
	Server       string `json:"server"`
	ResourceType string `json:"resourceType"`
	record       map[string]any
}

// MarshalJSON flattens the record fields up alongside server/resourceType.
func (r fanoutResult) MarshalJSON() ([]byte, error) {
	out := map[string]any{}
	for k, v := range r.record {
		out[k] = v
	}
	out["server"] = r.Server
	out["resourceType"] = r.ResourceType
	return json.Marshal(out)
}

func newNovelFanoutCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var dbPaths []string

	cmd := &cobra.Command{
		Use:   "fanout <query>",
		Short: "Search every configured N-central server at once — find a device, customer, or anything across tenants in one query.",
		Long: `Search the local SQLite mirror(s) with one full-text query and return
matches across every synced resource type, each row tagged with the server
it came from.

The default DB is the configured local mirror. Pass --db one or more times to
union additional server DB files into the same search — this is the
multi-server story: one query, many tenants.

Local-only: fanout never hits the network. Run 'sync' first to populate a DB.`,
		Example: `  # Search the default local mirror
  n-central-cli fanout "acme"

  # Union across multiple server mirrors
  n-central-cli fanout "exchange" --db /path/server-a.db --db /path/server-b.db --json`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			query := args[0]
			if limit <= 0 {
				limit = 50
			}

			// Build the list of (path, label) DBs to search. The default DB's
			// label is the configured server host when we can derive it,
			// otherwise its file basename; each --db is labeled by basename.
			type dbTarget struct {
				path  string
				label string
			}
			var targets []dbTarget

			defaultPath := defaultDBPath("n-central-cli")
			defaultLabel := serverLabelForDB(defaultPath)
			if c, err := flags.newClient(); err == nil && c.BaseURL != "" {
				if h := hostFromBaseURL(c.BaseURL); h != "" {
					defaultLabel = h
				}
			}
			targets = append(targets, dbTarget{path: defaultPath, label: defaultLabel})
			for _, p := range dbPaths {
				targets = append(targets, dbTarget{path: p, label: serverLabelForDB(p)})
			}

			var results []fanoutResult
			var searchErrs []string
			for _, t := range targets {
				db, err := store.OpenWithContext(cmd.Context(), t.path)
				if err != nil {
					searchErrs = append(searchErrs, fmt.Sprintf("%s: %v", t.path, err))
					continue
				}
				rows, err := db.Search(query, limit)
				if err != nil {
					searchErrs = append(searchErrs, fmt.Sprintf("%s: %v", t.path, err))
					db.Close()
					continue
				}
				for _, row := range rows {
					rec := decodeObj(row)
					if rec == nil {
						continue
					}
					results = append(results, fanoutResult{
						Server:       t.label,
						ResourceType: fanoutResourceType(rec),
						record:       rec,
					})
				}
				db.Close()
			}

			// Surface any per-DB errors to stderr but don't fail the whole run
			// when at least one DB returned — partial cross-tenant results beat
			// a hard error.
			for _, e := range searchErrs {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: fanout search failed for %s\n", e)
			}
			if len(results) == 0 && len(searchErrs) == len(targets) {
				return fmt.Errorf("fanout: no searchable database available; run 'n-central-cli sync' first")
			}

			if wantsHumanTable(cmd.OutOrStdout(), flags) {
				if len(results) == 0 {
					fmt.Fprintf(cmd.ErrOrStderr(), "No matches for %q across %d server(s).\n", query, len(targets))
					return nil
				}
				tw := newTabWriter(cmd.OutOrStdout())
				fmt.Fprintln(tw, bold("SERVER")+"\t"+bold("TYPE")+"\t"+bold("NAME / ID"))
				for _, r := range results {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Server, r.ResourceType, fanoutDisplayName(r.record))
				}
				return tw.Flush()
			}

			return flags.printJSON(cmd, results)
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum results per database")
	cmd.Flags().StringArrayVar(&dbPaths, "db", nil, "Additional server DB file path to union into the search (repeatable)")
	return cmd
}

// fanoutResourceType guesses the resource type of a decoded record from its
// shape, since the FTS search returns raw records without a type column.
func fanoutResourceType(rec map[string]any) string {
	switch {
	case firstField(rec, "deviceId", "device_id") != nil || firstField(rec, "longName", "long_name") != nil:
		return "device"
	case firstField(rec, "siteId", "site_id") != nil:
		return "site"
	case firstField(rec, "isServiceOrg", "is_service_org") != nil && asString(firstField(rec, "isServiceOrg", "is_service_org")) == "true":
		return "service_org"
	case firstField(rec, "customerId", "customer_id") != nil || firstField(rec, "customerName", "customer_name") != nil:
		return "customer"
	case firstField(rec, "orgUnitId", "org_unit_id") != nil:
		return "org_unit"
	case firstField(rec, "taskId", "task_id") != nil:
		return "scheduled_task"
	default:
		return "record"
	}
}

// fanoutDisplayName picks the most identifying human label from a record.
func fanoutDisplayName(rec map[string]any) string {
	for _, keys := range [][]string{
		{"longName", "long_name"},
		{"customerName", "customer_name"},
		{"siteName", "site_name"},
		{"orgUnitName", "org_unit_name"},
		{"name"},
		{"discoveredName", "discovered_name"},
		{"deviceId", "device_id"},
		{"id"},
	} {
		if v := firstField(rec, keys...); v != nil {
			if s := asString(v); s != "" {
				return s
			}
		}
	}
	return ""
}
