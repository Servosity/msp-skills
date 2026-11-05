// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: script blast radius.
// pp:data-source local

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"immybot-pp-cli/internal/store"
)

type blastTask struct {
	TaskID   string   `json:"task_id"`
	TaskName string   `json:"task_name,omitempty"`
	Roles    []string `json:"roles"`
}

type blastSoftware struct {
	SoftwareID   string   `json:"software_id"`
	SoftwareName string   `json:"software_name,omitempty"`
	DatabaseType string   `json:"database_type,omitempty"`
	Roles        []string `json:"roles"`
}

type blastAssignment struct {
	ID           string `json:"id"`
	DatabaseType string `json:"database_type,omitempty"`
	TargetName   string `json:"target_name,omitempty"`
	TargetText   string `json:"target_text,omitempty"`
	TargetType   string `json:"target_type,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
	TenantName   string `json:"tenant_name,omitempty"`
	ViaTaskID    string `json:"via_task_id"`
	ViaTaskName  string `json:"via_task_name,omitempty"`
	// ViaSoftware* name the software package when the deployment reaches the
	// script through a software role instead of a maintenance task.
	ViaSoftwareID   string `json:"via_software_id,omitempty"`
	ViaSoftwareName string `json:"via_software_name,omitempty"`
	// PropagatesToChildTenants: the deployment also reaches every descendant
	// of its tenant.
	PropagatesToChildTenants bool `json:"propagates_to_child_tenants,omitempty"`
}

type blastRadiusView struct {
	ScriptID        string            `json:"script_id"`
	ScriptName      string            `json:"script_name,omitempty"`
	ScriptScope     string            `json:"script_scope,omitempty"`
	ScannedTasks    int               `json:"scanned_tasks"`
	ScannedAssigns  int               `json:"scanned_assignments"`
	Tasks           []blastTask       `json:"consuming_tasks"`
	Software        []blastSoftware   `json:"consuming_software"`
	FleetWide       bool              `json:"fleet_wide"`
	Assignments     []blastAssignment `json:"deployments"`
	TenantsAffected []string          `json:"tenants_affected"`
	ComputerCount   int               `json:"computers_in_affected_tenants"`
	// ReachLowerBound: the mirror could not resolve every deployment's reach
	// (a child tenant or a targeted computer it does not hold), so the true
	// tenants and computers may be more; the note says what to sync.
	ReachLowerBound bool   `json:"reach_is_lower_bound"`
	Note            string `json:"note,omitempty"`
}

func newNovelScriptBlastRadiusCmd(flags *rootFlags) *cobra.Command {
	var (
		dbPath    string
		flagScope string
	)

	cmd := &cobra.Command{
		Use:   "script-blast-radius [script]",
		Short: "Everything a script reaches before you edit it",
		Long: "Every maintenance task, software package, deployment, and tenant that a script " +
			"reaches before you edit it. Global and local scripts number their ids separately: " +
			"pass --scope when an id names both. A deployment set to propagate to child tenants " +
			"reaches every descendant of its tenant; when the mirror cannot resolve the whole reach, " +
			"reach_is_lower_bound is true and the note names what to sync.\n\n" +
			"Use this command for the full downstream reach of one script or package. Do NOT use " +
			"this command for the inverse question of what a single computer receives; use " +
			"'assignment-explain' instead.",
		Example: strings.Trim(`
  immybot-cli script-blast-radius 312 --scope global
  immybot-cli script-blast-radius "Install Chrome" --agent
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
			scope := ""
			switch strings.ToLower(strings.TrimSpace(flagScope)) {
			case "":
			case "global":
				scope = "Global"
			case "local":
				scope = "Local"
			default:
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("--scope must be global or local, got %q", flagScope))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			dbPath = immyMirrorPath(dbPath)
			view := blastRadiusView{
				ScriptID:        needle,
				Tasks:           make([]blastTask, 0),
				Software:        make([]blastSoftware, 0),
				Assignments:     make([]blastAssignment, 0),
				TenantsAffected: make([]string, 0),
			}
			if immyMirrorMissing(cmd, dbPath, immyBlastRadiusSyncResources) {
				if !wantsHumanTable(cmd.OutOrStdout(), flags) {
					return printJSONFiltered(cmd.OutOrStdout(), view, flags)
				}
				return nil
			}
			db, err := immyOpenMirror(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if !hintIfUnsynced(cmd, db, "maintenance-tasks") {
				hintIfStale(cmd, db, "maintenance-tasks", flags.maxAge)
			}
			if err := immyBlastRadius(ctx, db, needle, scope, &view); err != nil {
				return err
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Tasks) == 0 && len(view.Software) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Script %s (%s %s)\n\n", firstNonEmpty(view.ScriptName, view.ScriptID), view.ScriptScope, view.ScriptID)
			if len(view.Tasks) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "CONSUMING TASKS\n")
				for _, t := range view.Tasks {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-38s roles: %s\n", immyTruncate(firstNonEmpty(t.TaskName, t.TaskID), 38), strings.Join(t.Roles, ","))
				}
			}
			if len(view.Software) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "CONSUMING SOFTWARE\n")
				for _, sw := range view.Software {
					fmt.Fprintf(cmd.OutOrStdout(), "  %-38s roles: %s\n", immyTruncate(firstNonEmpty(sw.SoftwareName, sw.SoftwareID), 38), strings.Join(sw.Roles, ","))
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nDEPLOYMENTS (%d)\n", len(view.Assignments))
			for _, a := range view.Assignments {
				via := firstNonEmpty(a.ViaTaskName, a.ViaTaskID, a.ViaSoftwareName, a.ViaSoftwareID)
				fmt.Fprintf(cmd.OutOrStdout(), "  %-30s via %-28s tenant %s\n",
					immyTruncate(firstNonEmpty(a.TargetName, a.TargetText), 30),
					immyTruncate(via, 28),
					immyTruncate(firstNonEmpty(a.TenantName, a.TenantID, "(all tenants)"), 24))
			}
			reach := fmt.Sprintf("%d tenant(s)", len(view.TenantsAffected))
			if view.FleetWide {
				reach = "every tenant (a deployment has no tenant)"
			}
			if view.ReachLowerBound {
				reach = "at least " + reach
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nReaches %s, %d computer(s) in those tenants.\n", reach, view.ComputerCount)
			if view.Note != "" {
				fmt.Fprintln(cmd.OutOrStdout(), view.Note)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flagScope, "scope", "", "The script's database type, global or local (needed when an id names both)")
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

// immyBlastRadiusSyncResources is every mirror resource the blast radius
// walks: both script stores, both task stores, both software stores, both
// assignment stores, tenants and computers.
const immyBlastRadiusSyncResources = "scripts-global,scripts-local,maintenance-tasks,maintenance-tasks-local," +
	"software,software-local,target-assignments,target-assignments-global,tenants,computers"

var immyDatabaseTypeNames = []string{"Global", "Local"}

// immyDatabaseTypeOf reads a row's DatabaseType ("Global"/"Local" or 0/1),
// falling back to the store the row came from.
func immyDatabaseTypeOf(raw any, resourceType string) string {
	if v := immyEnumName(anyToString(raw), immyDatabaseTypeNames); v == "Global" || v == "Local" {
		return v
	}
	if strings.HasSuffix(resourceType, "-local") {
		return "Local"
	}
	return "Global"
}

// immyScriptRefRoles returns every role in which obj references the script
// (id, databaseType): each <role>ScriptId key with its <role>ScriptType. A
// reference whose type is absent (not a vendor shape) still counts, so the
// blast radius never under-reports; a typed reference must match the type.
func immyScriptRefRoles(obj map[string]any, scriptID, scriptType, prefix string) []string {
	roles := make([]string, 0)
	for key, v := range obj {
		if !strings.HasSuffix(key, "ScriptId") || anyToString(v) != scriptID {
			continue
		}
		role := strings.TrimSuffix(key, "ScriptId")
		if t, ok := obj[role+"ScriptType"]; ok && t != nil {
			if immyEnumName(anyToString(t), immyDatabaseTypeNames) != scriptType {
				continue
			}
		}
		roles = append(roles, prefix+role)
	}
	sort.Strings(roles)
	return roles
}

type immyMirrorRow struct {
	id, resourceType string
	obj              map[string]any
}

// immyReadRows decodes every mirrored row of the given resource types. A
// mirror without the resources table yields no rows.
func immyReadRows(ctx context.Context, db *store.Store, resourceTypes ...string) ([]immyMirrorRow, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(resourceTypes)), ",")
	args := make([]any, 0, len(resourceTypes))
	for _, rt := range resourceTypes {
		args = append(args, rt)
	}
	rows, err := db.DB().QueryContext(ctx,
		`SELECT id, resource_type, data FROM resources WHERE resource_type IN (`+placeholders+`)`, args...)
	if err != nil {
		if immyNoSuchTableErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("querying %s: %w", strings.Join(resourceTypes, ","), err)
	}
	out := make([]immyMirrorRow, 0)
	for rows.Next() {
		var id, rt string
		var data []byte
		if err := rows.Scan(&id, &rt, &data); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scanning %s: %w", strings.Join(resourceTypes, ","), err)
		}
		var obj map[string]any
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.UseNumber()
		if dec.Decode(&obj) != nil || obj == nil {
			continue
		}
		out = append(out, immyMirrorRow{id: id, resourceType: rt, obj: obj})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterating %s: %w", strings.Join(resourceTypes, ","), err)
	}
	return out, rows.Close()
}

type immyItemKey struct{ dbType, id string }

// immyBlastRadius walks script -> tasks and software -> deployments -> tenants
// and computers. Every join carries the database type: a global and a local
// script, task, package or assignment may share an id.
func immyBlastRadius(ctx context.Context, db *store.Store, needle, scope string, view *blastRadiusView) error {
	// Hop 1: identify the script in the two script stores.
	scripts, err := immyReadRows(ctx, db, "scripts-global", "scripts-local")
	if err != nil {
		return err
	}
	type scriptMatch struct{ id, dbType, name string }
	matches := make([]scriptMatch, 0)
	for _, r := range scripts {
		id := firstNonEmpty(anyToString(r.obj["id"]), r.id)
		name := anyToString(r.obj["name"])
		dbType := immyDatabaseTypeOf(r.obj["databaseType"], r.resourceType)
		if scope != "" && dbType != scope {
			continue
		}
		if id == needle || strings.EqualFold(name, needle) {
			matches = append(matches, scriptMatch{id, dbType, name})
		}
	}
	switch {
	case len(matches) == 1:
		view.ScriptID, view.ScriptScope, view.ScriptName = matches[0].id, matches[0].dbType, matches[0].name
	case len(matches) > 1:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, fmt.Sprintf("%s %s %q", m.dbType, m.id, m.name))
		}
		sort.Strings(names)
		view.Note = fmt.Sprintf("%q names %d scripts (%s); pass --scope global or --scope local, or the script id", needle,
			len(matches), strings.Join(names, "; "))
		return nil
	case scope != "":
		// Not mirrored, but the operator named the store: walk by id.
		view.ScriptScope = scope
	default:
		view.Note = fmt.Sprintf("no script with id or name %q in the local mirror; run 'immybot-cli sync --resources %s' "+
			"or pass --scope global or --scope local to walk the id anyway", needle, immyBlastRadiusSyncResources)
		return nil
	}
	scriptID, scriptType := view.ScriptID, view.ScriptScope

	// Hop 2a: maintenance tasks that reference the script in any role.
	tasks, err := immyReadRows(ctx, db, "maintenance-tasks", "maintenance-tasks-local")
	if err != nil {
		return err
	}
	consumingTasks := map[immyItemKey]string{}
	for _, r := range tasks {
		view.ScannedTasks++
		roles := immyScriptRefRoles(r.obj, scriptID, scriptType, "")
		if len(roles) == 0 {
			continue
		}
		id := firstNonEmpty(anyToString(r.obj["id"]), r.id)
		dbType := immyDatabaseTypeOf(r.obj["databaseType"], r.resourceType)
		consumingTasks[immyItemKey{dbType, id}] = anyToString(r.obj["name"])
		view.Tasks = append(view.Tasks, blastTask{TaskID: id, TaskName: anyToString(r.obj["name"]), Roles: roles})
	}

	// Hop 2b: software packages that reference the script, directly, in a
	// version, or through a consuming configuration task.
	software, err := immyReadRows(ctx, db, "software", "software-local")
	if err != nil {
		return err
	}
	consumingSoftware := map[immyItemKey]string{}
	softwareByIdentifier := map[immyItemKey]string{}
	for _, r := range software {
		roles := immyScriptRefRoles(r.obj, scriptID, scriptType, "")
		if versions, ok := r.obj["softwareVersions"].([]any); ok {
			for _, v := range versions {
				if vm, ok := v.(map[string]any); ok {
					roles = append(roles, immyScriptRefRoles(vm, scriptID, scriptType, "version-")...)
				}
			}
		}
		if taskID := anyToString(r.obj["maintenanceTaskId"]); taskID != "" {
			taskType := immyDatabaseTypeOf(r.obj["maintenanceTaskType"], r.resourceType)
			if _, ok := consumingTasks[immyItemKey{taskType, taskID}]; ok {
				roles = append(roles, "configuration-task")
			}
		}
		if len(roles) == 0 {
			continue
		}
		id := firstNonEmpty(anyToString(r.obj["id"]), r.id)
		dbType := immyDatabaseTypeOf(r.obj["databaseType"], r.resourceType)
		name := anyToString(r.obj["name"])
		consumingSoftware[immyItemKey{dbType, id}] = name
		if ident := anyToString(r.obj["identifier"]); ident != "" {
			softwareByIdentifier[immyItemKey{dbType, ident}] = id
		}
		view.Software = append(view.Software, blastSoftware{SoftwareID: id, SoftwareName: name,
			DatabaseType: dbType, Roles: immyDedupSorted(roles)})
	}

	if len(view.Tasks) == 0 && len(view.Software) == 0 {
		view.Note = fmt.Sprintf("no maintenance task or software package references %s script %q (scanned %d tasks, %d packages); "+
			"the script may be unused, or run 'immybot-cli sync --resources %s' first",
			strings.ToLower(scriptType), needle, view.ScannedTasks, len(software), immyBlastRadiusSyncResources)
		return nil
	}

	// Hop 3: deployments bound to a consuming task or package, matched on
	// maintenanceType + maintenanceIdentifier.
	assigns, err := immyReadRows(ctx, db, "target-assignments", "target-assignments-global")
	if err != nil {
		return err
	}
	tenantSet := map[string]struct{}{}
	computerTargets := map[string]struct{}{}
	// propagating holds the tenants whose deployments also reach their
	// descendant tenants.
	propagating := map[string]struct{}{}
	for _, r := range assigns {
		view.ScannedAssigns++
		mid := anyToString(r.obj["maintenanceIdentifier"])
		if mid == "" {
			continue
		}
		var taskDB, softwareDB []string
		switch immyEnumName(anyToString(r.obj["maintenanceType"]), immyMaintenanceTypeNames) {
		case "GlobalMaintenanceTask":
			taskDB = []string{"Global"}
		case "LocalMaintenanceTask":
			taskDB = []string{"Local"}
		case "GlobalSoftware":
			softwareDB = []string{"Global"}
		case "LocalSoftware":
			softwareDB = []string{"Local"}
		case "":
			// No maintenanceType (not a vendor shape): never under-report.
			taskDB, softwareDB = immyDatabaseTypeNames, immyDatabaseTypeNames
		}
		a := blastAssignment{
			ID:           firstNonEmpty(anyToString(r.obj["id"]), r.id),
			DatabaseType: immyAssignmentDatabaseType(r.resourceType),
			TargetName:   anyToString(r.obj["targetName"]),
			TargetText:   anyToString(r.obj["targetText"]),
			TargetType: firstNonEmpty(immyTargetTypeName(anyToString(r.obj["targetType"]), anyToString(r.obj["targetTypeName"])),
				anyToString(r.obj["targetTypeName"])),
			TenantID:                 anyToString(r.obj["tenantId"]),
			PropagatesToChildTenants: truthy(anyToString(r.obj["propagateToChildTenants"])),
		}
		matched := false
		for _, t := range taskDB {
			if name, ok := consumingTasks[immyItemKey{t, mid}]; ok {
				a.ViaTaskID, a.ViaTaskName, matched = mid, name, true
				break
			}
		}
		for _, t := range softwareDB {
			if matched {
				break
			}
			id := mid
			if _, ok := consumingSoftware[immyItemKey{t, id}]; !ok {
				id = softwareByIdentifier[immyItemKey{t, mid}]
			}
			if name, ok := consumingSoftware[immyItemKey{t, id}]; ok && id != "" {
				a.ViaSoftwareID, a.ViaSoftwareName, matched = id, name, true
			}
		}
		if !matched {
			continue
		}
		view.Assignments = append(view.Assignments, a)
		// Reach: a tenant-bound deployment reaches its tenant. A tenant-less
		// one reaches the tenant or computer its target names; any other
		// tenant-less (or AllTenants) deployment reaches every tenant. One
		// set to propagate also reaches every descendant of that tenant (a
		// Computer target is one machine, so it has none).
		target := strings.TrimSpace(anyToString(r.obj["target"]))
		bound := ""
		switch {
		case a.TargetType == "AllTenants":
			view.FleetWide = true
		case a.TenantID != "":
			bound = a.TenantID
		case a.TargetType == "SpecificTenant" && target != "":
			bound = target
		case a.TargetType == "Computer" && target != "":
			computerTargets[target] = struct{}{}
		default:
			view.FleetWide = true
		}
		if bound != "" {
			tenantSet[bound] = struct{}{}
			if a.PropagatesToChildTenants && a.TargetType != "Computer" {
				propagating[bound] = struct{}{}
			}
		}
	}

	// Hop 4: name the tenants and count their machines. A deployment with no
	// tenant (or an AllTenants target) reaches every tenant.
	names, err := immyTenantNames(ctx, db)
	if err != nil {
		return err
	}
	for i := range view.Assignments {
		view.Assignments[i].TenantName = names[view.Assignments[i].TenantID]
	}
	// The roster, each resource read with its own schema's field names
	// (immyComputerShapes), merged by computer id.
	computers, err := immyLoadComputers(ctx, db, immyComputerRosterTypes)
	if err != nil {
		return err
	}
	computerTenant := map[string]string{}
	for _, c := range computers.list() {
		computerTenant[c.ID] = c.TenantID
	}
	unresolved := make([]string, 0)
	for target := range computerTargets {
		if t := computerTenant[target]; t != "" {
			tenantSet[t] = struct{}{}
		} else {
			unresolved = append(unresolved, target)
		}
	}
	notes := make([]string, 0, 2)
	if view.FleetWide {
		for id := range names {
			tenantSet[id] = struct{}{}
		}
	} else if len(propagating) > 0 {
		tree, err := immyLoadTenantTree(ctx, db)
		if err != nil {
			return err
		}
		if gaps := immyAddDescendantTenants(tree, propagating, computerTenant, tenantSet); len(gaps) > 0 {
			view.ReachLowerBound = true
			roots := make([]string, 0, len(propagating))
			for r := range propagating {
				roots = append(roots, r)
			}
			sort.Strings(roots)
			what := fmt.Sprintf("the mirror cannot tell whether tenant(s) %s descend from them", immyListSome(gaps, 10))
			if tree.empty() {
				what = "the local mirror holds no tenants, so their child tenants are unknown"
			}
			notes = append(notes, fmt.Sprintf("deployments on tenant(s) %s propagate to child tenants, and %s; "+
				"the reach is a lower bound; %s", immyListSome(roots, 10), what, immyTenantsSyncHint))
		}
	}
	labels := make([]string, 0, len(tenantSet))
	for t := range tenantSet {
		labels = append(labels, firstNonEmpty(names[t], t))
	}
	sort.Strings(labels)
	view.TenantsAffected = labels
	for _, t := range computerTenant {
		if _, ok := tenantSet[t]; ok || view.FleetWide {
			view.ComputerCount++
		}
	}
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		// A fleet-wide reach already counts every mirrored tenant.
		view.ReachLowerBound = view.ReachLowerBound || !view.FleetWide
		notes = append(notes, fmt.Sprintf("deployments target computer(s) %s that are not in the local mirror, so their tenants are not counted; "+
			"run 'immybot-cli sync --resources computers'", strings.Join(unresolved, ", ")))
	}
	view.Note = strings.Join(notes, ". ")

	sort.Slice(view.Assignments, func(i, j int) bool {
		if view.Assignments[i].DatabaseType != view.Assignments[j].DatabaseType {
			return view.Assignments[i].DatabaseType < view.Assignments[j].DatabaseType
		}
		return view.Assignments[i].ID < view.Assignments[j].ID
	})
	sort.Slice(view.Tasks, func(i, j int) bool { return view.Tasks[i].TaskID < view.Tasks[j].TaskID })
	sort.Slice(view.Software, func(i, j int) bool { return view.Software[i].SoftwareID < view.Software[j].SoftwareID })
	return nil
}

// immyAddDescendantTenants adds to reach every tenant (mirrored, or holding a
// mirrored computer) that descends from a propagating tenant, and returns,
// sorted, the tenants whose ancestry the mirror cannot decide. An empty tree
// decides nothing: every candidate is a gap, and a tree with no candidates
// still reports the propagating tenants themselves as undecided.
func immyAddDescendantTenants(tree immyTenantTree, propagating map[string]struct{}, computerTenant map[string]string,
	reach map[string]struct{}) []string {
	candidates := map[string]struct{}{}
	for _, id := range tree.ids() {
		candidates[id] = struct{}{}
	}
	for _, t := range computerTenant {
		if t = strings.TrimSpace(t); t != "" {
			candidates[t] = struct{}{}
		}
	}
	gaps := make([]string, 0)
	for t := range candidates {
		if _, ok := reach[t]; ok {
			continue
		}
		undecided := false
		for root := range propagating {
			anc, _ := tree.within(t, root)
			if anc == immyAncestryYes {
				reach[t] = struct{}{}
				undecided = false
				break
			}
			if anc == immyAncestryUnknown {
				undecided = true
			}
		}
		if undecided {
			gaps = append(gaps, t)
		}
	}
	if tree.empty() && len(gaps) == 0 {
		for root := range propagating {
			gaps = append(gaps, root)
		}
	}
	sort.Strings(gaps)
	return gaps
}

func immyDedupSorted(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for i, v := range in {
		if i == 0 || v != in[i-1] {
			out = append(out, v)
		}
	}
	return out
}
