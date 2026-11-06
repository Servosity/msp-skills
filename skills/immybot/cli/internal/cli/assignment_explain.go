// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Novel feature: deployment (target-assignment) resolution explainer.
// pp:data-source auto

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

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
	ShadowedByType string `json:"shadowed_by_database_type,omitempty"`
	// DatabaseType is Local or Global: the two number their ids separately.
	DatabaseType string `json:"database_type,omitempty"`
	// MayBeOverriddenBy lists unverified rules (membership the mirror cannot
	// evaluate) that are more specific than this winner for the same item.
	MayBeOverriddenBy string `json:"may_be_overridden_by,omitempty"`
	// PropagatedFrom is the ancestor tenant a rule set to propagate to child
	// tenants reaches this computer from; empty for the computer's own tenant.
	PropagatedFrom string `json:"propagated_from_tenant_id,omitempty"`
	// UnverifiedReason says why an unverified rule cannot be decided.
	UnverifiedReason string `json:"unverified_reason,omitempty"`
	// tenantUndecided: unverified because the tenant hierarchy is incomplete.
	tenantUndecided bool
	// computerTenantUnknown: unverified because no mirrored row gives this
	// computer's tenant id, so a rule bound to a tenant cannot be placed.
	computerTenantUnknown bool
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
	Unverified       []resolvedAssignment `json:"unverified"`
	LiveCrossCheck   []string             `json:"live_cross_check_ids,omitempty"`
	LiveCheckSkipped string               `json:"live_cross_check_skipped,omitempty"`
	Note             string               `json:"note,omitempty"`
}

func newNovelAssignmentExplainCmd(flags *rootFlags) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "assignment-explain [computer]",
		Short: "Explain which target assignments resolve onto one computer",
		Long: "Show every target assignment that resolves onto one computer, which scope matched, " +
			"and which rules are shadowed. A rule set to propagate to child tenants reaches computers " +
			"in every descendant tenant; when the mirrored tenants cannot place this computer's tenant " +
			"under the rule's, the rule is listed as unverified with its reason, and so is every rule " +
			"bound to a tenant when no mirrored computer row gives this computer's tenant id. The " +
			"computer is an id, or a name only one mirrored computer carries.\n\n" +
			"Use this command for why one computer receives the deployments it does. Do NOT use " +
			"this command for finding every computer a script or package reaches; use " +
			"'script-blast-radius' instead.",
		Example: strings.Trim(`
  immybot-cli assignment-explain 4821
  immybot-cli assignment-explain 4821 --agent
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
				Unverified: make([]resolvedAssignment, 0),
			}

			if strategy != "live" {
				dbPath = immyMirrorPath(dbPath)
				if immyMirrorMissing(cmd, dbPath, "computers,target-assignments,tenants") {
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
				if !hintIfUnsynced(cmd, db, "target-assignments") {
					hintIfStale(cmd, db, "target-assignments", flags.maxAge)
				}

				// Step 1: resolve the computer from every computer resource,
				// each read with its own schema's field names
				// (immyComputerShapes).
				roster, err := immyLoadComputers(ctx, db, immyComputerIdentityTypes)
				if err != nil {
					return err
				}
				match, ambiguous, found := roster.find(needle)
				if !found {
					view.Note = immyComputerNotFound(needle, ambiguous)
					if !wantsHumanTable(cmd.OutOrStdout(), flags) {
						return printJSONFiltered(cmd.OutOrStdout(), view, flags)
					}
					fmt.Fprintln(cmd.OutOrStdout(), view.Note)
					return nil
				}
				view.ComputerID = match.ID
				view.ComputerName = match.Name
				view.TenantID = match.TenantID
				if view.Tenant, err = immyComputerTenantName(ctx, db, match); err != nil {
					return err
				}
				tree, err := immyLoadTenantTree(ctx, db)
				if err != nil {
					return err
				}

				// Step 2: assignments. Safe now that the computers rows are closed.
				arows, err := db.DB().QueryContext(ctx, `
					SELECT id, resource_type,
						json_extract(data,'$.maintenanceIdentifier'),
						json_extract(data,'$.maintenanceType'),
						json_extract(data,'$.targetName'),
						json_extract(data,'$.targetText'),
						json_extract(data,'$.targetType'),
						json_extract(data,'$.targetTypeName'),
						json_extract(data,'$.targetScopeName'),
						json_extract(data,'$.tenantId'),
						json_extract(data,'$.target'),
						json_extract(data,'$.excluded'),
						json_extract(data,'$.onboardingOnly'),
						json_extract(data,'$.targetEnforcement'),
						COALESCE(json_extract(data,'$.softwareSemanticVersionString'), json_extract(data,'$.softwareSemanticVersion')),
						json_extract(data,'$.propagateToChildTenants')
					FROM resources
					WHERE resource_type IN ('target-assignments','target-assignments-global')`)
				if err != nil {
					return fmt.Errorf("querying target assignments: %w", err)
				}
				candidates := make([]resolvedAssignment, 0)
				unverified := make([]resolvedAssignment, 0)
				for arows.Next() {
					var id, rtype, mid, mtype, tname, ttext, ttype, ttypeName, tscope, tenantID, target, excluded, onboarding, enforce, version, propagate sql.NullString
					if err := arows.Scan(&id, &rtype, &mid, &mtype, &tname, &ttext, &ttype, &ttypeName, &tscope, &tenantID, &target, &excluded, &onboarding, &enforce, &version, &propagate); err != nil {
						_ = arows.Close()
						return fmt.Errorf("scanning target assignment: %w", err)
					}
					view.ScannedAssigns++
					typeName := immyTargetTypeName(nullStr(ttype), nullStr(ttypeName))
					m := immyTargetAppliesPropagated(typeName, nullStr(tenantID), nullStr(target), truthy(nullStr(propagate)),
						match.ID, match.TenantID, tree)
					if m.verdict == immyTargetNo && match.TenantID == "" {
						m = immyTargetWithoutComputerTenant(typeName, nullStr(tenantID), nullStr(target), match.ID)
					}
					if m.verdict == immyTargetNo {
						continue
					}
					a := resolvedAssignment{
						ID:             nullStr(id),
						DatabaseType:   immyAssignmentDatabaseType(nullStr(rtype)),
						MaintenanceID:  nullStr(mid),
						MaintenanceTyp: immyEnumName(nullStr(mtype), immyMaintenanceTypeNames),
						TargetName:     nullStr(tname),
						TargetText:     nullStr(ttext),
						TargetType:     firstNonEmpty(typeName, nullStr(ttypeName), nullStr(ttype)),
						TargetScope:    nullStr(tscope),
						Specificity:    m.spec,
						ScopeMatched:   m.scope,
						Excluded:       truthy(nullStr(excluded)),
						OnboardingOnly: truthy(nullStr(onboarding)),
						Enforcement:    nullStr(enforce),
						Version:        nullStr(version),
						PropagatedFrom: m.propagatedFrom,
					}
					if m.verdict == immyTargetUnverified {
						a.UnverifiedReason, a.tenantUndecided = m.reason, m.tenantUndecided
						a.computerTenantUnknown = m.computerTenantUnknown
						unverified = append(unverified, a)
						continue
					}
					candidates = append(candidates, a)
				}
				if err := arows.Err(); err != nil {
					_ = arows.Close()
					return fmt.Errorf("iterating target assignments: %w", err)
				}
				if err := arows.Close(); err != nil {
					return fmt.Errorf("closing target assignments: %w", err)
				}

				// Shadowing: for one maintenance item, the most specific
				// VERIFIED scope wins and the rest are reported as shadowed
				// rather than dropped. Winners are compared by database type
				// and id: local and global assignments number ids separately.
				uid := func(a resolvedAssignment) string { return a.DatabaseType + ":" + a.ID }
				itemKey := func(a resolvedAssignment) string { return a.MaintenanceTyp + "\x00" + a.MaintenanceID }
				bestFor := map[string]int{}
				winner := map[string]resolvedAssignment{}
				for _, c := range candidates {
					key := itemKey(c)
					// At equal specificity a tenant's own rule outranks one it
					// inherits from a parent tenant: the narrower scope wins.
					w, seen := winner[key]
					inheritedTie := seen && c.Specificity == w.Specificity && w.PropagatedFrom != "" && c.PropagatedFrom == ""
					if c.Specificity > bestFor[key] || inheritedTie {
						bestFor[key] = c.Specificity
						winner[key] = c
					}
				}
				// A rule the mirror cannot evaluate (tag, group, filter script,
				// person) is listed apart and never wins; a winner it could
				// outrank says so.
				overriders := map[string][]string{}
				for _, u := range unverified {
					key := itemKey(u)
					if w, ok := winner[key]; ok && u.Specificity > w.Specificity {
						overriders[key] = append(overriders[key], uid(u))
					}
				}
				for _, c := range candidates {
					key := itemKey(c)
					w := winner[key]
					if c.MaintenanceID != "" && uid(w) != uid(c) {
						c.Shadowed = true
						c.ShadowedBy = w.ID
						c.ShadowedByType = w.DatabaseType
						view.Shadowed = append(view.Shadowed, c)
						continue
					}
					if o := overriders[key]; len(o) > 0 {
						sort.Strings(o)
						c.MayBeOverriddenBy = strings.Join(o, ",")
					}
					view.Effective = append(view.Effective, c)
				}
				view.Unverified = unverified
				sortAssignments(view.Effective)
				sortAssignments(view.Shadowed)
				sortAssignments(view.Unverified)
				if len(view.Unverified) > 0 {
					membership, hierarchy, noTenant := 0, 0, 0
					for _, u := range view.Unverified {
						switch {
						case u.computerTenantUnknown:
							noTenant++
						case u.tenantUndecided:
							hierarchy++
						default:
							membership++
						}
					}
					parts := make([]string, 0, 3)
					if noTenant > 0 {
						parts = append(parts, fmt.Sprintf("%d rule(s) are bound to a tenant, and no mirrored computer row gives this computer's tenant id; "+
							"run 'immybot-cli sync --resources computers'", noTenant))
					}
					if membership > 0 {
						parts = append(parts, fmt.Sprintf("%d rule(s) target a tag, group, filter script or person whose membership the local mirror cannot evaluate", membership))
					}
					if hierarchy > 0 {
						parts = append(parts, fmt.Sprintf("%d rule(s) propagate from a tenant the local mirror cannot place this computer's tenant under; %s",
							hierarchy, immyTenantsSyncHint))
					}
					view.Note = strings.Join(parts, "; ") + "; they are listed under unverified and never win"
				}
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
			if len(view.Effective) == 0 && len(view.Shadowed) == 0 && len(view.Unverified) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No target assignments resolve onto %s (scanned %d).\n", view.ComputerID, view.ScannedAssigns)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Computer %s (%s), tenant %s\n\n", view.ComputerName, view.ComputerID, view.Tenant)
			fmt.Fprintf(cmd.OutOrStdout(), "EFFECTIVE\n%-14s %-34s %-14s %s\n", "SCOPE", "MAINTENANCE ITEM", "TYPE", "TARGET")
			for _, a := range view.Effective {
				scope := a.ScopeMatched
				if a.PropagatedFrom != "" {
					scope += " via " + a.PropagatedFrom
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-34s %-14s %s\n",
					scope, immyTruncate(firstNonEmpty(a.TargetName, a.MaintenanceID), 34),
					immyTruncate(a.MaintenanceTyp, 14), immyTruncate(firstNonEmpty(a.TargetText, a.TargetType), 40))
			}
			if len(view.Shadowed) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nSHADOWED (a more specific rule wins)\n")
				for _, a := range view.Shadowed {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-34s shadowed by %s %s\n",
						a.ScopeMatched, immyTruncate(firstNonEmpty(a.TargetName, a.MaintenanceID), 34), a.ShadowedByType, a.ShadowedBy)
				}
			}
			if len(view.Unverified) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\nUNVERIFIED (the local mirror cannot decide whether these reach this computer)\n")
				for _, a := range view.Unverified {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s %-34s %s %s: %s\n",
						immyTruncate(a.TargetType, 14), immyTruncate(firstNonEmpty(a.TargetName, a.MaintenanceID), 34), a.DatabaseType, a.ID,
						a.UnverifiedReason)
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

// immyTargetTypeNames is the vendor TargetType enum by integer value ("the
// string or integer value may be used").
var immyTargetTypeNames = map[int]string{
	1: "All", 2: "Computer", 5: "Metascript", 7: "Person", 8: "AzureGroup", 20: "TenantMetascript",
	21: "AllForTenant", 22: "ProviderDeviceGroup", 23: "ProviderClientGroup", 24: "TenantFilterScript",
	25: "FilterScript", 28: "AllTenants", 29: "SpecificTenant", 30: "Tag", 31: "TenantTag", 40: "CloudTag",
	42: "CloudProviderClientGroup",
}

var immyMaintenanceTypeNames = []string{"GlobalSoftware", "LocalSoftware", "WindowsUpdate", "ChocolateySoftware",
	"NiniteSoftware", "LocalMaintenanceTask", "GlobalMaintenanceTask", "AgentUpdate"}

// immyTargetTypeName resolves targetType (enum name or integer), falling back
// to targetTypeName only when it spells an enum name. "" means unknown.
func immyTargetTypeName(targetType, targetTypeName string) string {
	for _, raw := range []string{targetType, targetTypeName} {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if n, err := strconv.Atoi(s); err == nil {
			if name, ok := immyTargetTypeNames[n]; ok {
				return name
			}
			continue
		}
		folded := strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "_", "")
		for _, name := range immyTargetTypeNames {
			if strings.EqualFold(name, folded) {
				return name
			}
		}
	}
	return ""
}

type immyTargetVerdict int

const (
	immyTargetNo immyTargetVerdict = iota
	immyTargetYes
	immyTargetUnverified
)

// immyTargetApplies evaluates one target type's own predicate for one
// computer and returns whether the rule reaches it, its specificity and scope
// label. A Computer rule needs its target to be this computer; a tenant rule
// needs this computer's tenant; a rule whose membership the mirror does not
// hold (tag, group, filter script, metascript, person) is unverified, never a
// match. Any rule bound to another tenant does not apply.
func immyTargetApplies(typeName, aTenant, aTarget, computerID, computerTenant string) (immyTargetVerdict, int, string) {
	aTenant, aTarget = strings.TrimSpace(aTenant), strings.TrimSpace(aTarget)
	sameTenant := aTenant != "" && computerTenant != "" && aTenant == computerTenant
	otherTenant := aTenant != "" && !sameTenant
	switch typeName {
	case "Computer":
		// The target is the computer's id; a name never stands for a
		// computer (names repeat across tenants).
		if aTarget != "" && aTarget == computerID {
			return immyTargetYes, 4, "computer"
		}
		return immyTargetNo, 0, ""
	case "AllForTenant":
		if sameTenant {
			return immyTargetYes, 2, "tenant"
		}
		return immyTargetNo, 0, ""
	case "SpecificTenant":
		if (aTarget != "" && aTarget == computerTenant) || sameTenant {
			return immyTargetYes, 2, "tenant"
		}
		return immyTargetNo, 0, ""
	case "All":
		switch {
		case aTenant == "":
			return immyTargetYes, 1, "global"
		case sameTenant:
			return immyTargetYes, 2, "tenant"
		}
		return immyTargetNo, 0, ""
	case "AllTenants":
		return immyTargetYes, 1, "global"
	}
	if otherTenant {
		return immyTargetNo, 0, ""
	}
	return immyTargetUnverified, 3, "unverified"
}

// immyMembershipReason is why a rule immyTargetApplies calls unverified
// cannot be decided.
const immyMembershipReason = "the local mirror cannot evaluate this target's membership (tag, group, filter script, metascript or person)"

// immyTargetMatch is one rule's verdict on one computer.
type immyTargetMatch struct {
	verdict immyTargetVerdict
	spec    int
	scope   string
	// propagatedFrom is the ancestor tenant the rule reaches the computer from.
	propagatedFrom  string
	reason          string
	tenantUndecided bool
	// computerTenantUnknown: the computer's tenant id is not mirrored.
	computerTenantUnknown bool
}

// immyComputerTenantReason is why a tenant-bound rule is unverified for a
// computer whose tenant id the mirror does not hold.
const immyComputerTenantReason = "the local mirror holds no tenant id for this computer, so a rule bound to a tenant cannot be placed"

// immyTargetWithoutComputerTenant decides a rule immyTargetApplies rejected
// for a computer whose tenant id is unknown. A rule bound to a tenant (its
// tenantId, or the tenant a SpecificTenant target names) would apply if that
// were the computer's tenant, and nothing proves it is not, so it is
// unverified rather than dropped. A Computer rule names its machine by target
// and needs no tenant, so its rejection stands.
func immyTargetWithoutComputerTenant(typeName, aTenant, aTarget, computerID string) immyTargetMatch {
	aTenant, aTarget = strings.TrimSpace(aTenant), strings.TrimSpace(aTarget)
	bound := aTenant
	if bound == "" && typeName == "SpecificTenant" {
		bound = aTarget
	}
	if bound == "" || typeName == "Computer" {
		return immyTargetMatch{}
	}
	v, spec, _ := immyTargetApplies(typeName, aTenant, aTarget, computerID, bound)
	if v == immyTargetNo {
		return immyTargetMatch{}
	}
	reason := immyComputerTenantReason
	if v == immyTargetUnverified {
		reason += "; " + immyMembershipReason
	}
	return immyTargetMatch{verdict: immyTargetUnverified, spec: spec, scope: "unverified", reason: reason,
		computerTenantUnknown: true}
}

// immyTargetAppliesPropagated is immyTargetApplies plus tenant propagation. A
// rule with propagateToChildTenants true that is bound to an ancestor of the
// computer's tenant applies exactly as the same rule bound to the computer's
// own tenant would. When the mirrored hierarchy cannot place the computer's
// tenant under the rule's, a rule that would then apply is unverified, never
// dropped. A non-propagating rule is immyTargetApplies unchanged.
func immyTargetAppliesPropagated(typeName, aTenant, aTarget string, propagate bool,
	computerID, computerTenant string, tree immyTenantTree) immyTargetMatch {
	verdict, spec, scope := immyTargetApplies(typeName, aTenant, aTarget, computerID, computerTenant)
	m := immyTargetMatch{verdict: verdict, spec: spec, scope: scope}
	if verdict == immyTargetUnverified {
		m.reason = immyMembershipReason
	}
	computerTenant = strings.TrimSpace(computerTenant)
	if verdict != immyTargetNo || !propagate || computerTenant == "" {
		return m
	}
	aTenant, aTarget = strings.TrimSpace(aTenant), strings.TrimSpace(aTarget)
	// The tenants the rule is bound to: its tenantId, and for SpecificTenant
	// also the tenant its target names.
	roots := []string{aTenant}
	if typeName == "SpecificTenant" {
		roots = append(roots, aTarget)
	}
	var undecided immyTargetMatch
	for _, root := range roots {
		if root == "" || root == computerTenant {
			continue
		}
		anc, gap := tree.within(computerTenant, root)
		if anc == immyAncestryNo {
			continue
		}
		rebase := func(s string) string {
			if s == root {
				return computerTenant
			}
			return s
		}
		rTarget := aTarget
		if typeName == "SpecificTenant" {
			rTarget = rebase(aTarget)
		}
		v, s, sc := immyTargetApplies(typeName, rebase(aTenant), rTarget, computerID, computerTenant)
		if v == immyTargetNo {
			continue
		}
		if anc == immyAncestryYes {
			out := immyTargetMatch{verdict: v, spec: s, scope: sc, propagatedFrom: root}
			if v == immyTargetUnverified {
				out.reason = immyMembershipReason
			}
			return out
		}
		if undecided.verdict == immyTargetNo {
			reason := tree.gapReason(root, computerTenant, gap)
			if v == immyTargetUnverified {
				reason += "; " + immyMembershipReason
			}
			undecided = immyTargetMatch{verdict: immyTargetUnverified, spec: s, scope: "unverified",
				reason: reason, tenantUndecided: true}
		}
	}
	if undecided.verdict != immyTargetNo {
		return undecided
	}
	return m
}

// immyKnownDatabaseType reads a DatabaseType value ("Global"/"Local" or 0/1);
// anything else is "" (unknown), never guessed.
func immyKnownDatabaseType(raw string) string {
	if v := immyEnumName(raw, []string{"Global", "Local"}); v == "Global" || v == "Local" {
		return v
	}
	return ""
}

// immyAssignmentDatabaseType names the store an assignment row came from.
func immyAssignmentDatabaseType(resourceType string) string {
	if strings.HasSuffix(resourceType, "-global") {
		return "Global"
	}
	return "Local"
}
