// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: script blast radius.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type blastTask struct {
	TaskID   string   `json:"task_id"`
	TaskName string   `json:"task_name,omitempty"`
	Roles    []string `json:"roles"`
}

type blastAssignment struct {
	ID          string `json:"id"`
	TargetName  string `json:"target_name,omitempty"`
	TargetText  string `json:"target_text,omitempty"`
	TargetType  string `json:"target_type,omitempty"`
	TenantID    string `json:"tenant_id,omitempty"`
	TenantName  string `json:"tenant_name,omitempty"`
	ViaTaskID   string `json:"via_task_id"`
	ViaTaskName string `json:"via_task_name,omitempty"`
}

type blastRadiusView struct {
	ScriptID        string            `json:"script_id"`
	ScriptName      string            `json:"script_name,omitempty"`
	ScriptScope     string            `json:"script_scope,omitempty"`
	ScannedTasks    int               `json:"scanned_tasks"`
	ScannedAssigns  int               `json:"scanned_assignments"`
	Tasks           []blastTask       `json:"consuming_tasks"`
	Assignments     []blastAssignment `json:"deployments"`
	TenantsAffected []string          `json:"tenants_affected"`
	ComputerCount   int               `json:"computers_in_affected_tenants"`
	Note            string            `json:"note,omitempty"`
}

func newNovelScriptBlastRadiusCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "script-blast-radius [script]",
		Short: "Everything a script reaches before you edit it",
		Long: "Every maintenance task, deployment, and tenant that a script reaches before you " +
			"edit it.\n\n" +
			"Use this command for the full downstream reach of one script or package. Do NOT use " +
			"this command for the inverse question of what a single computer receives; use " +
			"'assignment-explain' instead.",
		Example: strings.Trim(`
  immybot-pp-cli script-blast-radius 312
  immybot-pp-cli script-blast-radius "Install Chrome" --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "script=1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "script-blast-radius")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a script id or name is required"))
			}
			needle := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := blastRadiusView{
				ScriptID:        needle,
				Tasks:           make([]blastTask, 0),
				Assignments:     make([]blastAssignment, 0),
				TenantsAffected: make([]string, 0),
			}
			if immyMirrorMissing(cmd, dbPath, "scripts,maintenance-tasks,target-assignments,tenants") {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := store.OpenWithContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "maintenance-tasks") {
				hintIfStale(cmd, db, "maintenance-tasks", flags.maxAge)
			}

			// Hop 1: identify the script.
			srows, err := db.DB().QueryContext(ctx, `
				SELECT id, json_extract(data,'$.name'), json_extract(data,'$.databaseType')
				FROM resources
				WHERE resource_type IN ('scripts','scripts-local','scripts-global','scripts-dx')`)
			if err != nil {
				return fmt.Errorf("querying scripts: %w", err)
			}
			type scriptRow struct{ id, name, scope string }
			scripts := make([]scriptRow, 0)
			for srows.Next() {
				var id, name, scope sql.NullString
				if err := srows.Scan(&id, &name, &scope); err != nil {
					_ = srows.Close()
					return fmt.Errorf("scanning script: %w", err)
				}
				scripts = append(scripts, scriptRow{nullStr(id), nullStr(name), nullStr(scope)})
			}
			if err := srows.Err(); err != nil {
				_ = srows.Close()
				return fmt.Errorf("iterating scripts: %w", err)
			}
			if err := srows.Close(); err != nil {
				return fmt.Errorf("closing scripts: %w", err)
			}
			for _, s := range scripts {
				if s.id == needle || strings.EqualFold(s.name, needle) {
					view.ScriptID = s.id
					view.ScriptName = s.name
					view.ScriptScope = s.scope
					break
				}
			}

			// Hop 2: maintenance tasks referencing the script in any role.
			trows, err := db.DB().QueryContext(ctx, `
				SELECT id, json_extract(data,'$.name'),
					json_extract(data,'$.getScriptId'),
					json_extract(data,'$.setScriptId'),
					json_extract(data,'$.testScriptId')
				FROM resources
				WHERE resource_type IN ('maintenance-tasks','maintenance-tasks-local')`)
			if err != nil {
				return fmt.Errorf("querying maintenance tasks: %w", err)
			}
			taskNames := map[string]string{}
			for trows.Next() {
				var id, name, getID, setID, testID sql.NullString
				if err := trows.Scan(&id, &name, &getID, &setID, &testID); err != nil {
					_ = trows.Close()
					return fmt.Errorf("scanning maintenance task: %w", err)
				}
				view.ScannedTasks++
				roles := make([]string, 0, 3)
				if nullStr(getID) == view.ScriptID && view.ScriptID != "" {
					roles = append(roles, "get")
				}
				if nullStr(setID) == view.ScriptID && view.ScriptID != "" {
					roles = append(roles, "set")
				}
				if nullStr(testID) == view.ScriptID && view.ScriptID != "" {
					roles = append(roles, "test")
				}
				if len(roles) == 0 {
					continue
				}
				view.Tasks = append(view.Tasks, blastTask{TaskID: nullStr(id), TaskName: nullStr(name), Roles: roles})
				taskNames[nullStr(id)] = nullStr(name)
			}
			if err := trows.Err(); err != nil {
				_ = trows.Close()
				return fmt.Errorf("iterating maintenance tasks: %w", err)
			}
			if err := trows.Close(); err != nil {
				return fmt.Errorf("closing maintenance tasks: %w", err)
			}

			// Hop 3: deployments bound to those tasks.
			if len(taskNames) > 0 {
				arows, err := db.DB().QueryContext(ctx, `
					SELECT id,
						json_extract(data,'$.maintenanceIdentifier'),
						json_extract(data,'$.targetName'),
						json_extract(data,'$.targetText'),
						json_extract(data,'$.targetTypeName'),
						json_extract(data,'$.tenantId')
					FROM resources
					WHERE resource_type IN ('target-assignments','target-assignments-global')`)
				if err != nil {
					return fmt.Errorf("querying target assignments: %w", err)
				}
				tenantSet := map[string]struct{}{}
				for arows.Next() {
					var id, mid, tname, ttext, ttype, tenantID sql.NullString
					if err := arows.Scan(&id, &mid, &tname, &ttext, &ttype, &tenantID); err != nil {
						_ = arows.Close()
						return fmt.Errorf("scanning target assignment: %w", err)
					}
					view.ScannedAssigns++
					taskID := nullStr(mid)
					name, ok := taskNames[taskID]
					if !ok {
						continue
					}
					view.Assignments = append(view.Assignments, blastAssignment{
						ID:          nullStr(id),
						TargetName:  nullStr(tname),
						TargetText:  nullStr(ttext),
						TargetType:  nullStr(ttype),
						TenantID:    nullStr(tenantID),
						ViaTaskID:   taskID,
						ViaTaskName: name,
					})
					if t := nullStr(tenantID); t != "" {
						tenantSet[t] = struct{}{}
					}
				}
				if err := arows.Err(); err != nil {
					_ = arows.Close()
					return fmt.Errorf("iterating target assignments: %w", err)
				}
				if err := arows.Close(); err != nil {
					return fmt.Errorf("closing target assignments: %w", err)
				}

				// Hop 4: name the tenants and count their machines. Safe now
				// that every earlier result set is closed.
				if len(tenantSet) > 0 {
					names := map[string]string{}
					nrows, err := db.DB().QueryContext(ctx, `
						SELECT id, json_extract(data,'$.name') FROM resources WHERE resource_type = 'tenants'`)
					if err == nil {
						for nrows.Next() {
							var id, name sql.NullString
							if err := nrows.Scan(&id, &name); err != nil {
								break
							}
							names[nullStr(id)] = nullStr(name)
						}
						_ = nrows.Err()
						_ = nrows.Close()
					}
					for i := range view.Assignments {
						view.Assignments[i].TenantName = names[view.Assignments[i].TenantID]
					}
					labels := make([]string, 0, len(tenantSet))
					for t := range tenantSet {
						if n := names[t]; n != "" {
							labels = append(labels, n)
						} else {
							labels = append(labels, t)
						}
					}
					sort.Strings(labels)
					view.TenantsAffected = labels

					crows, err := db.DB().QueryContext(ctx, `
						SELECT json_extract(data,'$.tenantId') FROM resources
						WHERE resource_type IN ('computers','computers-paged','computers-dx')`)
					if err == nil {
						for crows.Next() {
							var t sql.NullString
							if err := crows.Scan(&t); err != nil {
								break
							}
							if _, ok := tenantSet[nullStr(t)]; ok {
								view.ComputerCount++
							}
						}
						_ = crows.Err()
						_ = crows.Close()
					}
				}
			}

			sort.Slice(view.Assignments, func(i, j int) bool { return view.Assignments[i].ID < view.Assignments[j].ID })
			sort.Slice(view.Tasks, func(i, j int) bool { return view.Tasks[i].TaskID < view.Tasks[j].TaskID })

			if len(view.Tasks) == 0 {
				view.Note = fmt.Sprintf("no maintenance task references script %q (scanned %d tasks); "+
					"the script may be unused, or run 'immybot-pp-cli sync --resources scripts,maintenance-tasks' first",
					needle, view.ScannedTasks)
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Tasks) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Script %s (%s)\n\n", firstNonEmpty(view.ScriptName, view.ScriptID), view.ScriptID)
			fmt.Fprintf(cmd.OutOrStdout(), "CONSUMING TASKS\n")
			for _, t := range view.Tasks {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-38s roles: %s\n", immyTruncate(firstNonEmpty(t.TaskName, t.TaskID), 38), strings.Join(t.Roles, ","))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nDEPLOYMENTS (%d)\n", len(view.Assignments))
			for _, a := range view.Assignments {
				fmt.Fprintf(cmd.OutOrStdout(), "  %-30s via %-28s tenant %s\n",
					immyTruncate(firstNonEmpty(a.TargetName, a.TargetText), 30),
					immyTruncate(firstNonEmpty(a.ViaTaskName, a.ViaTaskID), 28),
					immyTruncate(firstNonEmpty(a.TenantName, a.TenantID), 24))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nReaches %d tenant(s), %d computer(s) in those tenants.\n",
				len(view.TenantsAffected), view.ComputerCount)
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}
