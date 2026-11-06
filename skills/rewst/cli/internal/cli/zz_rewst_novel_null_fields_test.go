// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// rewst handfix novel-missing-input-is-an-error, class "null field inside a
// row" (Codex review r2, rewst 4.34.2 reprint): the novels refused a null
// top-level field and a null list element, but decoded a null FIELD inside a
// row as zero. The Rewst SDL declares those fields nullable
// (TimeSavedGroupByWorkflow.secondsSaved: Int, WorkflowExecution.status:
// String, Organization.id: ID, Organization.isDeleted: Boolean, ...), so
// `roi --org o --agent` on
// {"timeSavedGroupByWorkflow":[{"workflowId":"w","secondsSaved":null,...}]}
// reported zero savings and exited 0, and `orgs` dropped an org whose id was
// null. This test nulls every field of every row each novel reads, one at a
// time, and requires an error, except the few fields a row may legitimately
// lack (labels and optional relationships), which must come out as null,
// never as a zero value.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"
)

// novelFullAnswers is a complete, readable answer per GraphQL field.
func novelFullAnswers() map[string]string {
	now := time.Now().UTC().Format(time.RFC3339)
	return map[string]string{
		"workflowExecutionStats":     `{"succeeded":3,"failed":1,"running":0,"pending":0,"delayed":0,"paused":0,"humanSecondsSaved":120}`,
		"timeSavedGroupByWorkflow":   `[{"workflowId":"w1","workflowName":"QBR","secondsSaved":3600,"totalExecutions":4,"successfulExecutions":3,"failedExecutions":1}]`,
		"workflowExecutions":         `[{"id":"e1","status":"FAILED","createdAt":"` + now + `","numSuccessfulTasks":1,"numAwaitingResponseTasks":0,"workflow":{"id":"w1","name":"QBR"}}]`,
		"workflows":                  `[{"id":"w1","name":"QBR","updatedAt":"2026-01-01T00:00:00Z","humanSecondsSaved":60},{"id":"w2","name":"Onboard","updatedAt":"2026-01-01T00:00:00Z","humanSecondsSaved":30}]`,
		"orgVariables":               `[{"name":"psa_board"}]`,
		"packsForOrg":                `[{"name":"Microsoft","ref":"pack-ms"}]`,
		"managedAndSubOrganizations": `[{"id":"c1","name":"Client One","installedPacks":[{"name":"Microsoft","ref":"pack-ms"}]}]`,
		"userOrganization":           `{"id":"o1","name":"Root MSP","isMsp":true,"managingOrgId":null,"managedAndSubOrgs":[{"id":"c1","name":"Client One","managingOrgId":"o1","isEnabled":true,"isDeleted":false}]}`,
	}
}

// novelFieldRuns: which GraphQL fields each novel reads.
var novelFieldRuns = map[string]struct {
	args   []string
	fields []string
}{
	"health":   {[]string{"health", "--org", "o-1", "--json"}, []string{"workflowExecutionStats"}},
	"roi":      {[]string{"roi", "--org", "o-1", "--json"}, []string{"timeSavedGroupByWorkflow"}},
	"failures": {[]string{"failures", "--org", "o-1", "--json"}, []string{"workflowExecutions"}},
	"dormant":  {[]string{"dormant", "--org", "o-1", "--json"}, []string{"workflows", "workflowExecutions"}},
	"drift":    {[]string{"drift", "--org", "o-1", "--against", "o-2", "--json"}, []string{"orgVariables", "packsForOrg"}},
	"coverage": {[]string{"coverage", "--parent", "o-1", "--json"}, []string{"managedAndSubOrganizations"}},
	"orgs":     {[]string{"orgs", "--json"}, []string{"userOrganization"}},
}

// A row may lack these (the SDL types them nullable and a null is a real
// state, not missing data): the novel must show null, never a zero value.
// Paths are <field>/<json path inside the answer> with [] for list elements.
var novelNullableLeaves = map[string]bool{
	"timeSavedGroupByWorkflow/[]/workflowName":            true, // a deleted workflow keeps its stats
	"workflowExecutions/[]/workflow/name":                 true,
	"workflows/[]/name":                                   true,
	"workflows/[]/updatedAt":                              true,
	"userOrganization/isMsp":                              true,
	"userOrganization/managedAndSubOrgs/[]/isEnabled":     true,
	"userOrganization/managedAndSubOrgs/[]/managingOrgId": true,
	// pack name and ref: either may be null, not both (checked below).
	"packsForOrg/[]/name": true,
	"packsForOrg/[]/ref":  true,
	"managedAndSubOrganizations/[]/installedPacks/[]/name": true,
	"managedAndSubOrganizations/[]/installedPacks/[]/ref":  true,
}

// leafPaths lists every member path inside v (objects and list elements
// descend; the top-level value itself is the answer field, not a leaf).
func leafPaths(v any, prefix string, out *[]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			p := prefix + "/" + k
			*out = append(*out, p)
			leafPaths(child, p, out)
		}
	case []any:
		if len(t) > 0 {
			leafPaths(t[0], prefix+"/[]", out)
		}
	}
}

// setNull sets the member at path (relative, "/a/[]/b") to null in v; a
// list step nulls the member in its FIRST element only.
func setNull(v any, steps []string) any {
	if len(steps) == 0 {
		return nil
	}
	switch t := v.(type) {
	case map[string]any:
		cp := make(map[string]any, len(t))
		for k, child := range t {
			cp[k] = child
		}
		cp[steps[0]] = setNull(t[steps[0]], steps[1:])
		return cp
	case []any:
		cp := append([]any(nil), t...)
		cp[0] = setNull(t[0], steps[1:])
		return cp
	}
	return v
}

func TestNovelsRefuseNullFieldsInsideRows(t *testing.T) {
	full := novelFullAnswers()
	names := make([]string, 0, len(novelFieldRuns))
	for n := range novelFieldRuns {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		run := novelFieldRuns[name]
		for _, field := range run.fields {
			var parsed any
			if err := json.Unmarshal([]byte(full[field]), &parsed); err != nil {
				t.Fatalf("fixture %s: %v", field, err)
			}
			var paths []string
			leafPaths(parsed, "", &paths)
			sort.Strings(paths)
			for _, p := range paths {
				key := field + p
				if key == "userOrganization/managingOrgId" {
					continue // already null in the full answer (the root org manages itself)
				}
				nulled, _ := json.Marshal(setNull(parsed, strings.Split(strings.TrimPrefix(p, "/"), "/")))
				t.Run(name+"/"+key, func(t *testing.T) {
					gqlFake(t, func(f string, _ map[string]any) string {
						body, ok := full[f]
						if f == field {
							body = string(nulled)
						} else if !ok {
							body = `[]`
						}
						return fmt.Sprintf(`{"data":{%q:%s}}`, f, body)
					})
					out, _, err := runRootArgs(t, run.args...)
					if novelNullableLeaves[key] {
						if err != nil {
							t.Fatalf("%s refused a row whose %s may be null: %v\n%s", name, key, err, out)
						}
						return
					}
					if err == nil {
						t.Fatalf("%s read a null %s as a value and exited 0:\n%s", name, key, out)
					}
				})
			}
		}
	}
}

// A pack with neither a name nor a ref cannot be counted or compared.
func TestNovelsRefusePackWithNoNameAndNoRef(t *testing.T) {
	for _, tc := range []struct {
		name, field, body string
		args              []string
	}{
		{"drift", "packsForOrg", `[{"name":null,"ref":null}]`, novelFieldRuns["drift"].args},
		{"coverage", "managedAndSubOrganizations", `[{"id":"c1","name":"C","installedPacks":[{"name":null,"ref":null}]}]`, novelFieldRuns["coverage"].args},
	} {
		t.Run(tc.name, func(t *testing.T) {
			full := novelFullAnswers()
			gqlFake(t, func(f string, _ map[string]any) string {
				body, ok := full[f]
				if f == tc.field {
					body = tc.body
				} else if !ok {
					body = `[]`
				}
				return fmt.Sprintf(`{"data":{%q:%s}}`, f, body)
			})
			out, _, err := runRootArgs(t, tc.args...)
			if err == nil {
				t.Fatalf("%s counted a pack with no name and no ref and exited 0:\n%s", tc.name, out)
			}
		})
	}
}

// A nullable label comes out as null, never as an empty or zero value.
func TestNovelsShowNullLabelsAsNull(t *testing.T) {
	full := novelFullAnswers()
	gqlFake(t, func(f string, _ map[string]any) string {
		if f == "timeSavedGroupByWorkflow" {
			return `{"data":{"timeSavedGroupByWorkflow":[{"workflowId":"w1","workflowName":null,"secondsSaved":3600,"totalExecutions":4,"successfulExecutions":3,"failedExecutions":1}]}}`
		}
		return fmt.Sprintf(`{"data":{%q:%s}}`, f, full[f])
	})
	out, _, err := runRootArgs(t, "roi", "--org", "o-1", "--json")
	if err != nil {
		t.Fatalf("roi: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"workflow_name": null`) {
		t.Fatalf("roi rendered a null workflow name as a value:\n%s", out)
	}
}
