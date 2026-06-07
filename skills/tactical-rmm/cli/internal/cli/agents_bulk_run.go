// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

// pp:data-source live
package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"tactical-rmm-pp-cli/internal/cliutil"
)

// novelAgentFilterWhere translates a cohort filter (k=v entries over local
// store agent fields) into parameterized SQL WHERE fragments plus their bound
// argument values. Fragments use `?` placeholders so apostrophe-bearing client
// names (e.g. O'Brien) match correctly without escaping hacks. Shared by
// bulk-run and maintenance set so the two cohort selectors behave identically.
// Recognised keys: os/plat, client, site, status, online.
func novelAgentFilterWhere(filter []string) (fragments []string, args []any) {
	fragments = []string{}
	args = []any{}
	for _, kv := range filter {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		p := strings.SplitN(kv, "=", 2)
		if len(p) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(p[0]))
		v := strings.TrimSpace(p[1])
		switch k {
		case "os", "plat":
			fragments = append(fragments, "json_extract(data,'$.plat')=?")
			args = append(args, v)
		case "client":
			fragments = append(fragments, "json_extract(data,'$.client_name')=?")
			args = append(args, v)
		case "site":
			fragments = append(fragments, "json_extract(data,'$.site_name')=?")
			args = append(args, v)
		case "status":
			fragments = append(fragments, "json_extract(data,'$.status')=?")
			args = append(args, v)
		case "online":
			if v == "true" {
				fragments = append(fragments, "json_extract(data,'$.status')=?")
				args = append(args, "online")
			} else {
				fragments = append(fragments, "json_extract(data,'$.status')<>?")
				args = append(args, "online")
			}
		}
	}
	return fragments, args
}

// novelSelectCohort resolves (agent_ids, hostnames) from the local store for a
// cohort filter. Returns empty slices when the store can't be opened. The where
// fragments use `?` placeholders bound to args (see novelAgentFilterWhere).
func novelSelectCohort(cmd *cobra.Command, flags *rootFlags, where []string, args []any) (ids, hosts []string) {
	ids = make([]string, 0)
	hosts = make([]string, 0)
	s := novelLocalRead(cmd, flags, "agents")
	if s == nil {
		return ids, hosts
	}
	defer s.Close()
	q := "SELECT json_extract(data,'$.agent_id'),json_extract(data,'$.hostname') FROM resources WHERE resource_type='agents'"
	if len(where) > 0 {
		q += " AND " + strings.Join(where, " AND ")
	}
	if rows, qe := s.DB().QueryContext(cmd.Context(), q, args...); qe == nil {
		defer rows.Close()
		for rows.Next() {
			var id, h sql.NullString
			if rows.Scan(&id, &h) == nil && id.String != "" {
				ids = append(ids, id.String)
				hosts = append(hosts, h.String)
			}
		}
	}
	return ids, hosts
}

func newNovelAgentsBulkRunCmd(flags *rootFlags) *cobra.Command {
	var script, timeout int
	var command, shell string
	var filter []string
	var execute bool
	cmd := &cobra.Command{
		Use:     "bulk-run",
		Short:   "Run a script or command across a filtered cohort of agents",
		Long:    "Resolves target agents from the local store using --filter (os/client/site/status/online), previews the cohort, and with --execute calls the Tactical RMM bulk action endpoint. Previews by default; never runs under verify. Use this to run a script/command on a cohort. To put a cohort into maintenance mode instead, use 'maintenance set'.",
		Example: "  tactical-rmm-cli agents bulk-run --command whoami --filter os=windows,online=true\n  tactical-rmm-cli agents bulk-run --command whoami --filter client=Acme --execute",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return err
			}
			if script == 0 && strings.TrimSpace(command) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("provide --script <id> or --command <text>"))
			}
			where, whereArgs := novelAgentFilterWhere(filter)
			ids, hosts := novelSelectCohort(cmd, flags, where, whereArgs)
			mode := "command"
			if script != 0 {
				mode = "script"
			}
			body := map[string]interface{}{"mode": mode, "target": "agents", "agent_ids": ids, "timeout": timeout, "output": "forget"}
			if mode == "script" {
				body["script"] = script
			} else {
				body["cmd"] = command
				if shell != "" {
					body["shell"] = shell
				}
			}
			bs, _ := json.Marshal(body)
			if !execute || cliutil.IsVerifyEnv() {
				note := "preview only; pass --execute to run"
				if cliutil.IsVerifyEnv() {
					note = "would submit bulk " + mode + " (verify mode: no network)"
				}
				return flags.printJSON(cmd, map[string]interface{}{"dry_run": true, "cohort_size": len(ids), "hostnames": hosts, "mode": mode, "request_body": json.RawMessage(bs), "note": note})
			}
			if len(ids) == 0 {
				return fmt.Errorf("no agents matched the filter")
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if _, _, err := c.Post(cmd.Context(), "/agents/actions/bulk/", body); err != nil {
				return classifyAPIError(err, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "bulk %s submitted to %d agents\n", mode, len(ids))
			return nil
		},
	}
	cmd.Flags().IntVar(&script, "script", 0, "Script ID to run")
	cmd.Flags().StringVar(&command, "command", "", "Raw command to run")
	cmd.Flags().StringVar(&shell, "shell", "", "Shell: cmd, powershell, or shell")
	cmd.Flags().StringSliceVar(&filter, "filter", nil, "Cohort filter, e.g. os=windows,online=true,client=Acme")
	cmd.Flags().IntVar(&timeout, "timeout", 30, "Per-agent timeout (seconds)")
	cmd.Flags().BoolVar(&execute, "execute", false, "Actually run (default: preview only)")
	return cmd
}
