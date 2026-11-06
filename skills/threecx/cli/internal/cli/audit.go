// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: audit finds call routing that points nowhere.
// pp:data-source live

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"threecx-pp-cli/internal/fleet"
)

// auditReqs fetch the routing graph. Members, Agents and Forwards are OData
// navigation properties, so they only come back when $expand asks for them;
// the generated sync never does, which is why the audit reads live.
var auditReqs = []fleet.Request{
	{Path: "/Peers", Params: map[string]string{"$select": "Number,Name,Type"}},
	{Path: "/Users", Params: map[string]string{"$select": "Number,FirstName,LastName,Enabled"}},
	{Path: "/RingGroups", Params: map[string]string{"$expand": "Members"}},
	{Path: "/Queues", Params: map[string]string{"$expand": "Agents"}},
	{Path: "/Receptionists", Params: map[string]string{"$expand": "Forwards"}},
	{Path: "/InboundRules"},
}

func newNovelAuditCmd(flags *rootFlags) *cobra.Command {
	var scope fleetScope
	var problem string

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Find ring groups, queues, IVRs and inbound rules that route to deleted, disabled or missing extensions, on one PBX (--pbx) or the whole fleet",
		Long: `Find ring groups, queues, IVRs (digital receptionists) and inbound rules that
route calls to an extension, queue, ring group or IVR that no longer exists, to a
disabled user, or that have no members at all.

Reads the live routing graph of the selected phone system (--pbx) or of every
registered one, so it reflects a change ticket the moment it is saved.
Each finding names the object, the field (for example Forwards[1] or
OutOfOfficeHoursDestination), the target number, and the problem:
missing, disabled, or empty.`,
		Example: strings.Trim(`
  threecx-cli audit --pbx acme --agent
  threecx-cli audit --problem missing --json`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "audit")
			}
			switch problem {
			case "", "missing", "disabled", "empty":
			default:
				return usageErr(fmt.Errorf("--problem %q: use missing, disabled or empty", problem))
			}
			pbxes, _, err := scope.resolve()
			if err != nil {
				return err
			}
			if len(pbxes) == 0 && emptyFleet(cmd, flags) {
				return nil
			}
			findings, failed := collect(cmd, &scope, flags, pbxes, auditReqs,
				func(p fleet.PBX, bodies []json.RawMessage) ([]fleet.Finding, error) {
					var g fleet.RoutingGraph
					if err := decodeGraph(bodies, &g); err != nil {
						return nil, err
					}
					out := make([]fleet.Finding, 0)
					for _, f := range fleet.Audit(p.Name, g) {
						if problem == "" || f.Problem == problem {
							out = append(out, f)
						}
					}
					return out, nil
				}, func(p fleet.PBX, err error) fleet.Finding {
					return fleet.Finding{PBX: p.Name, Problem: "error", Error: err.Error()}
				})
			err = render(cmd, flags, findings, []string{"pbx", "object_type", "object", "field",
				"target", "target_type", "problem", "error"}, "No broken routing found.", func(f fleet.Finding) map[string]any {
				return map[string]any{"pbx": f.PBX, "object": f.ObjectType + " " + f.Object, "field": f.Field,
					"target": f.Target, "problem": f.Problem, "error": f.Error}
			})
			if err != nil {
				return err
			}
			return allFailed(failed, len(pbxes))
		},
	}
	addFleetScopeFlags(cmd, &scope)
	cmd.Flags().StringVar(&problem, "problem", "", "Only this kind of finding: missing, disabled or empty")
	return cmd
}

// decodeGraph reads the six collections fetched by auditReqs, in order.
// The child follows top-level @odata.nextLink pages; a nextLink left in a
// body belongs to an $expand-ed list (Members, Agents, Forwards), so the
// routing graph would be incomplete and the PBX is not audited.
func decodeGraph(bodies []json.RawMessage, g *fleet.RoutingGraph) error {
	for i, b := range bodies {
		if bytes.Contains(b, []byte("@odata.nextLink")) {
			return fmt.Errorf("%s: an expanded list was paged; the routing graph would be incomplete, so this PBX is not audited", auditReqs[i].Path)
		}
	}
	if err := decodeInto(bodies, &g.Peers, &g.Users, &g.RingGroups, &g.Queues, &g.Receptionists, &g.InboundRules); err != nil {
		return fmt.Errorf("decoding routing graph: %w", err)
	}
	return nil
}
