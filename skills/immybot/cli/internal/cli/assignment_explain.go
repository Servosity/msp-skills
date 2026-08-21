// Copyright 2026 Abhi Saini and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: deployment (target-assignment) resolution explainer.
// pp:data-source auto

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"immybot-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

type resolvedAssignment struct {
	ID             string `json:"id"`
	MaintenanceID  string `json:"maintenance_identifier,omitempty"`
	MaintenanceTyp string `json:"maintenance_type,omitempty"`
	TargetName     string `json:"target_name,omitempty"`
	TargetText     string `json:"target_text,omitempty"`
	TargetType     string `json:"target_type,omitempty"`
	TargetScope    string `json:"target_scope,omitempty"`
	Specificity    int    `json:"specificity"`
	ScopeMatched   string `json:"scope_matched"`
	Excluded       bool   `json:"excluded"`
	OnboardingOnly bool   `json:"onboarding_only"`
	Enforcement    string `json:"enforcement,omitempty"`
	Version        string `json:"desired_version,omitempty"`
	Shadowed       bool   `json:"shadowed"`
	ShadowedBy     string `json:"shadowed_by,omitempty"`
}

type assignmentExplainView struct {
	ComputerID       string               `json:"computer_id"`
	ComputerName     string               `json:"computer_name,omitempty"`
	Tenant           string               `json:"tenant,omitempty"`
	TenantID         string               `json:"tenant_id,omitempty"`
	DataSource       string               `json:"data_source"`
	ScannedAssigns   int                  `json:"scanned_assignments"`
	Effective        []resolvedAssignment `json:"effective"`
	Shadowed         []resolvedAssignment `json:"shadowed"`
	LiveCrossCheck   []string             `json:"live_cross_check_ids,omitempty"`
	LiveCheckSkipped string               `json:"live_cross_check_skipped,omitempty"`
	Note             string               `json:"note,omitempty"`
}

// scopeSpecificity ranks how narrowly an assignment targets a machine. A more
// specific rule shadows a broader one for the same maintenance item, which is
// the question this command exists to answer.
func scopeSpecificity(targetType, scope string) (int, string) {
	t := strings.ToLower(targetType + " " + scope)
	// Order matters: ImmyBot's fleet-wide target type is literally named
	// "All Computers", so the global test has to run before the computer test
	// or a fleet-wide rule is misread as the narrowest possible scope and
	// wrongly shadows every real per-machine rule.
	switch {
	case strings.Contains(t, "all"), strings.Contains(t, "global"), strings.Contains(t, "everyone"):
		return 1, "global"
	case strings.Contains(t, "computer"), strings.Contains(t, "device"):
		return 4, "computer"
	case strings.Contains(t, "group"), strings.Contains(t, "tag"), strings.Contains(t, "filter"):
		return 3, "group-or-tag"
	case strings.Contains(t, "tenant"), strings.Contains(t, "client"):
		return 2, "tenant"
	default:
		return 2, "tenant"
	}
}

func newNovelAssignmentExplainCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "assignment-explain [computer]",
		Short: "Explain which target assignments resolve onto one computer",
		Long: "Show every target assignment that resolves onto one computer, which scope matched, " +
			"and which rules are shadowed.\n\n" +
			"Use this command for why one computer receives the deployments it does. Do NOT use " +
			"this command for finding every computer a script or package reaches; use " +
			"'script-blast-radius' instead.",
		Example: strings.Trim(`
  immybot-pp-cli assignment-explain 4821
  immybot-pp-cli assignment-explain 4821 --agent
`, "\n"),
		Annotations: map[string]string{
			"mcp:read-only": "true",
			"pp:happy-args": "computer=1",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "assignment-explain")
			}
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a computer id or name is required"))
			}
			needle := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			strategy := strings.ToLower(strings.TrimSpace(flags.dataSource))
			if strategy == "" {
				strategy = "auto"
			}
			view := assignmentExplainView{
				ComputerID: needle,
				DataSource: strategy,
				Effective:  make([]resolvedAssignment, 0),
				Shadowed:   make([]resolvedAssignment, 0),
			}

			if strategy != "live" {
				dbPath = immyMirrorPath(dbPath)
				if immyMirrorMissing(cmd, dbPath, "computers,target-assignments") {
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
				if !hintIfUnsynced(cmd, db, "target-assignments") {
					hintIfStale(cmd, db, "target-assignments", flags.maxAge)
				}

				// Step 1: resolve the computer. Drain fully before the second query.
				crows, err := db.DB().QueryContext(ctx, `
					SELECT id, json_extract(data,'$.name'), json_extract(data,'$.tenant'), json_extract(data,'$.tenantId')
					FROM resources
					WHERE resource_type IN ('computers','computers-paged','computers-dx','computers-my')`)
				if err != nil {
					return fmt.Errorf("querying computers: %w", err)
				}
				type comp struct{ id, name, tenant, tenantID string }
				comps := make([]comp, 0)
				for crows.Next() {
					var id, name, tenant, tenantID sql.NullString
					if err := crows.Scan(&id, &name, &tenant, &tenantID); err != nil {
						_ = crows.Close()
						return fmt.Errorf("scanning computer: %w", err)
					}
					comps = append(comps, comp{nullStr(id), nullStr(name), nullStr(tenant), nullStr(tenantID)})
				}
				if err := crows.Err(); err != nil {
					_ = crows.Close()
					return fmt.Errorf("iterating computers: %w", err)
				}
				if err := crows.Close(); err != nil {
					return fmt.Errorf("closing computers: %w", err)
				}

				var match *comp
				for i := range comps {
					if comps[i].id == needle || strings.EqualFold(comps[i].name, needle) {
						match = &comps[i]
						break
					}
				}
				if match == nil {
					view.Note = fmt.Sprintf("no computer with id or name %q in the local mirror; run 'immybot-pp-cli sync --resources computers'", needle)
					if !wantsHumanTable(cmd.OutOrStdout(), flags) {
						return printJSONFiltered(cmd.OutOrStdout(), view, flags)
					}
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
					return nil
				}
				view.ComputerID = match.id
				view.ComputerName = match.name
				view.Tenant = match.tenant
				view.TenantID = match.tenantID

				// Step 2: assignments. Safe now that the computers rows are closed.
				arows, err := db.DB().QueryContext(ctx, `
					SELECT id,
						json_extract(data,'$.maintenanceIdentifier'),
						json_extract(data,'$.maintenanceType'),
						json_extract(data,'$.targetName'),
						json_extract(data,'$.targetText'),
						json_extract(data,'$.targetTypeName'),
						json_extract(data,'$.targetScopeName'),
						json_extract(data,'$.tenantId'),
						json_extract(data,'$.target'),
						json_extract(data,'$.excluded'),
						json_extract(data,'$.onboardingOnly'),
						json_extract(data,'$.targetEnforcement'),
						json_extract(data,'$.softwareSemanticVersion')
					FROM resources
					WHERE resource_type IN ('target-assignments','target-assignments-global')`)
				if err != nil {
					return fmt.Errorf("querying target assignments: %w", err)
				}
				candidates := make([]resolvedAssignment, 0)
				for arows.Next() {
					var id, mid, mtype, tname, ttext, ttype, tscope, tenantID, target, excluded, onboarding, enforce, version sql.NullString
					if err := arows.Scan(&id, &mid, &mtype, &tname, &ttext, &ttype, &tscope, &tenantID, &target, &excluded, &onboarding, &enforce, &version); err != nil {
						_ = arows.Close()
						return fmt.Errorf("scanning target assignment: %w", err)
					}
					view.ScannedAssigns++
					aTenant := nullStr(tenantID)
					aTarget := nullStr(target)
					spec, scopeName := scopeSpecificity(nullStr(ttype), nullStr(tscope))

					// An assignment reaches this computer when it names the
					// machine directly, shares its tenant, or is tenant-less
					// (global). Anything else belongs to another tenant.
					matched := false
					switch {
					case aTarget != "" && (aTarget == match.id || strings.EqualFold(aTarget, match.name)):
						matched, spec, scopeName = true, 4, "computer"
					case aTenant != "" && match.tenantID != "" && aTenant == match.tenantID:
						matched = true
					case aTenant == "":
						matched = true
						if spec > 1 {
							spec, scopeName = 1, "global"
						}
					}
					if !matched {
						continue
					}
					candidates = append(candidates, resolvedAssignment{
						ID:             nullStr(id),
						MaintenanceID:  nullStr(mid),
						MaintenanceTyp: nullStr(mtype),
						TargetName:     nullStr(tname),
						TargetText:     nullStr(ttext),
						TargetType:     nullStr(ttype),
						TargetScope:    nullStr(tscope),
						Specificity:    spec,
						ScopeMatched:   scopeName,
						Excluded:       truthy(nullStr(excluded)),
						OnboardingOnly: truthy(nullStr(onboarding)),
						Enforcement:    nullStr(enforce),
						Version:        nullStr(version),
					})
				}
				if err := arows.Err(); err != nil {
					_ = arows.Close()
					return fmt.Errorf("iterating target assignments: %w", err)
				}
				if err := arows.Close(); err != nil {
					return fmt.Errorf("closing target assignments: %w", err)
				}

				// Shadowing: for one maintenance item, the most specific scope
				// wins and the rest are reported as shadowed rather than dropped.
				bestFor := map[string]int{}
				winnerID := map[string]string{}
				for _, c := range candidates {
					key := c.MaintenanceTyp + "\x00" + c.MaintenanceID
					if c.Specificity > bestFor[key] {
						bestFor[key] = c.Specificity
						winnerID[key] = c.ID
					}
				}
				for _, c := range candidates {
					key := c.MaintenanceTyp + "\x00" + c.MaintenanceID
					if c.MaintenanceID != "" && winnerID[key] != c.ID {
						c.Shadowed = true
						c.ShadowedBy = winnerID[key]
						view.Shadowed = append(view.Shadowed, c)
						continue
					}
					view.Effective = append(view.Effective, c)
				}
				sortAssignments(view.Effective)
				sortAssignments(view.Shadowed)
			}

			// Live cross-check against the authoritative resolver. Advisory:
			// a failure here never fails the command, because the local
			// resolution is the product and this only annotates it.
			if strategy != "local" {
				c, err := flags.newClient()
				if err != nil {
					view.LiveCheckSkipped = err.Error()
				} else {
					path := replacePathParam("/api/v1/computers/{computerId}/resolve-onboarding-overridable-target-assignments", "computerId", view.ComputerID)
					data, err := c.Get(ctx, path, map[string]string{})
					if err != nil {
						view.LiveCheckSkipped = err.Error()
					} else {
						var items []map[string]any
						if err := json.Unmarshal(data, &items); err == nil {
							ids := make([]string, 0, len(items))
							for _, it := range items {
								if v, ok := it["id"]; ok {
									ids = append(ids, fmt.Sprintf("%v", v))
								}
							}
							sort.Strings(ids)
							view.LiveCrossCheck = ids
						}
					}
				}
			}

			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.Effective) == 0 && len(view.Shadowed) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No target assignments resolve onto %s (scanned %d).\n", view.ComputerID, view.ScannedAssigns)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Computer %s (%s), tenant %s\n\n", view.ComputerName, view.ComputerID, view.Tenant)
			fmt.Fprintf(cmd.OutOrStdout(), "EFFECTIVE\n%-14s %-34s %-14s %s\n", "SCOPE", "MAINTENANCE ITEM", "TYPE", "TARGET")
			for _, a := range view.Effective {
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-34s %-14s %s\n",
					a.ScopeMatched, immyTruncate(firstNonEmpty(a.TargetName, a.MaintenanceID), 34),
					immyTruncate(a.MaintenanceTyp, 14), immyTruncate(firstNonEmpty(a.TargetText, a.TargetType), 40))
			}
			if len(view.Shadowed) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nSHADOWED (a more specific rule wins)\n")
				for _, a := range view.Shadowed {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-34s shadowed by %s\n",
						a.ScopeMatched, immyTruncate(firstNonEmpty(a.TargetName, a.MaintenanceID), 34), a.ShadowedBy)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Local mirror database path")
	return cmd
}

func sortAssignments(in []resolvedAssignment) {
	sort.Slice(in, func(i, j int) bool {
		if in[i].Specificity != in[j].Specificity {
			return in[i].Specificity > in[j].Specificity
		}
		return in[i].ID < in[j].ID
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
