// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// rewst handfix novel-missing-input-is-an-error: the six carried novels read
// a GraphQL answer they could not decode, a null field the SDL declares
// nullable (workflowExecutionStats: WorkflowExecutionStats, workflowExecutions:
// [WorkflowExecution]) or a missing field as zero ("healthy", "no failures",
// "0 hours saved"); drift read a failed side as "every pack is missing there";
// and packsForOrg's documented default page (limit: Int = 100) truncated the
// comparison. Any failure signal wins.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"rewst-pp-cli/internal/cliutil/testenv"
)

var gqlFieldRe = regexp.MustCompile(`\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*[({]`)

// gqlFake answers every POST /graphql with answer(field, variables).
func gqlFake(t *testing.T, answer func(field string, vars map[string]any) string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		_ = json.Unmarshal(b, &req)
		field := ""
		if i := strings.Index(req.Query, "{"); i >= 0 {
			if m := gqlFieldRe.FindStringSubmatch(req.Query[i:]); m != nil {
				field = m[1]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(answer(field, req.Variables)))
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("REWST_BASE_URL", srv.URL)
	t.Setenv("REWST_API_TOKEN", "rwst-novel-inputs-token-8d2c")
}

var novelRuns = map[string][]string{
	"health":   {"health", "--org", "o-1", "--json"},
	"roi":      {"roi", "--org", "o-1", "--json"},
	"failures": {"failures", "--org", "o-1", "--json"},
	"dormant":  {"dormant", "--org", "o-1", "--json"},
	"drift":    {"drift", "--org", "o-1", "--against", "o-2", "--json"},
	"coverage": {"coverage", "--parent", "o-1", "--json"},
}

func TestNovelsRefuseMissingNullOrUndecodableData(t *testing.T) {
	for shape, body := range map[string]func(string) string{
		"null field":    func(f string) string { return fmt.Sprintf(`{"data":{%q:null}}`, f) },
		"missing field": func(string) string { return `{"data":{}}` },
		"no data":       func(string) string { return `{}` },
		"wrong type":    func(f string) string { return fmt.Sprintf(`{"data":{%q:"not-an-object-or-list"}}`, f) },
	} {
		for name, args := range novelRuns {
			t.Run(shape+"/"+name, func(t *testing.T) {
				gqlFake(t, func(field string, _ map[string]any) string { return body(field) })
				out, _, err := runRootArgs(t, args...)
				if err == nil {
					t.Fatalf("%s answered %s and exited 0:\n%s", name, shape, out)
				}
			})
		}
	}
}

// Codex review r1 (rewst 4.34.2 reprint), a NEW instance of the pre-closed
// class novel-missing-input-is-an-error: a null ELEMENT of a documented list
// was decoded into a zero struct and counted, so `failures` on the Rewst
// response {"data":{"workflowExecutions":[null]}} (the SDL field is
// [WorkflowExecution], nullable elements) reported scanned_executions:1 and
// failures:[] - a workflow execution the API could not return read as one it
// examined and found healthy. A null element is missing input, never zero.
func TestNovelsRefuseNullListElements(t *testing.T) {
	// A list answer with one null element among readable ones, for each novel.
	nullElem := map[string]string{
		"workflowExecutionStats":     `{"succeeded":1,"failed":0,"running":0,"pending":0,"delayed":0,"paused":0,"humanSecondsSaved":0}`, // object, no list to null
		"timeSavedGroupByWorkflow":   `[null]`,
		"workflowExecutions":         `[null]`,
		"workflows":                  `[null]`,
		"managedAndSubOrganizations": `[null]`,
		"orgVariables":               `[null]`,
		"packsForOrg":                `[null]`,
	}
	for name, args := range novelRuns {
		if name == "health" {
			continue // health reads an object, not a list
		}
		t.Run(name, func(t *testing.T) {
			gqlFake(t, func(field string, _ map[string]any) string {
				if body, ok := nullElem[field]; ok {
					return fmt.Sprintf(`{"data":{%q:%s}}`, field, body)
				}
				return fmt.Sprintf(`{"data":{%q:[]}}`, field)
			})
			out, _, err := runRootArgs(t, args...)
			if err == nil {
				t.Fatalf("%s decoded a null list element as zero and exited 0:\n%s", name, out)
			}
		})
	}
	// orgs: managedAndSubOrgs with a null element.
	t.Run("orgs", func(t *testing.T) {
		gqlFake(t, func(string, map[string]any) string {
			return `{"data":{"userOrganization":{"id":"o1","name":"Root","managedAndSubOrgs":[null]}}}`
		})
		out, _, err := runRootArgs(t, "orgs", "--json")
		if err == nil {
			t.Fatalf("orgs decoded a null managed-org as zero and exited 0:\n%s", out)
		}
	})
}

func TestDriftFailsWhenEitherSideFails(t *testing.T) {
	gqlFake(t, func(field string, vars map[string]any) string {
		if vars["orgId"] == "o-2" {
			return `{"errors":[{"message":"Not Authorized","extensions":{"code":"AUTH_ERR"}}],"data":null}`
		}
		if field == "orgVariables" {
			return `{"data":{"orgVariables":[{"name":"psa_board"}]}}`
		}
		return `{"data":{"packsForOrg":[{"name":"Microsoft","ref":"pack-ms"}]}}`
	})
	out, _, err := runRootArgs(t, novelRuns["drift"]...)
	if err == nil {
		t.Fatalf("drift exited 0 with one side unreadable (it reports that side's packs as missing):\n%s", out)
	}
}

// packsForOrg(limit: Int = 100) answers at most 100 packs per call: two orgs
// with the same 150 packs listed in different orders must show no drift.
func TestDriftReadsEveryPage(t *testing.T) {
	const n = 150
	refs := func(org string) []string {
		var r []string
		for i := 0; i < n; i++ {
			r = append(r, fmt.Sprintf("pack-%03d", i))
		}
		if org == "o-2" {
			for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
				r[i], r[j] = r[j], r[i]
			}
		}
		return r
	}
	gqlFake(t, func(field string, vars map[string]any) string {
		org, _ := vars["orgId"].(string)
		all := refs(org)
		limit, offset := 100, 0
		if v, ok := vars["limit"].(float64); ok {
			limit = int(v)
		}
		if v, ok := vars["offset"].(float64); ok {
			offset = int(v)
		}
		if offset > len(all) {
			offset = len(all)
		}
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		var items []string
		for _, ref := range all[offset:end] {
			if field == "orgVariables" {
				items = append(items, fmt.Sprintf(`{"name":%q}`, ref))
			} else {
				items = append(items, fmt.Sprintf(`{"name":%q,"ref":%q}`, ref, ref))
			}
		}
		return fmt.Sprintf(`{"data":{%q:[%s]}}`, field, strings.Join(items, ","))
	})
	out, _, err := runRootArgs(t, novelRuns["drift"]...)
	if err != nil {
		t.Fatalf("drift: %v\n%s", err, out)
	}
	var view driftView
	if err := json.Unmarshal([]byte(out[strings.Index(out, "{"):]), &view); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if len(view.PacksOnlyOrg)+len(view.PacksOnlyAgn)+len(view.VarsOnlyOrg)+len(view.VarsOnlyAgn) != 0 || view.PacksShared != n || view.VarsShared != n {
		t.Fatalf("identical orgs reported drift (a page was dropped): packs only-org %d only-against %d shared %d; vars only-org %d only-against %d shared %d",
			len(view.PacksOnlyOrg), len(view.PacksOnlyAgn), view.PacksShared, len(view.VarsOnlyOrg), len(view.VarsOnlyAgn), view.VarsShared)
	}
}

// rewst handfix novel-scan-caps-said: a list that came back exactly --limit
// long is flagged, so "a pack in 3/3 orgs" cannot read as full coverage of an
// MSP with more orgs, and dormant says workflows past the cap were not seen.
func TestScanCapsAreSaid(t *testing.T) {
	gqlFake(t, func(field string, vars map[string]any) string {
		switch field {
		case "managedAndSubOrganizations":
			return `{"data":{"managedAndSubOrganizations":[{"id":"a","name":"A","installedPacks":[]},{"id":"b","name":"B","installedPacks":[]}]}}`
		case "workflows":
			return `{"data":{"workflows":[{"id":"w1","name":"one"},{"id":"w2","name":"two"}]}}`
		default:
			return `{"data":{"workflowExecutions":[]}}`
		}
	})
	out, _, err := runRootArgs(t, "coverage", "--parent", "msp", "--limit", "2", "--json")
	if err != nil || !strings.Contains(out, `"scan_capped": true`) {
		t.Fatalf("coverage at its --limit did not say so (err %v):\n%s", err, out)
	}
	out, _, err = runRootArgs(t, "dormant", "--org", "o-1", "--limit", "2", "--json")
	if err != nil || !strings.Contains(out, `"workflows_capped": true`) {
		t.Fatalf("dormant at its --limit did not say so (err %v):\n%s", err, out)
	}
	out, _, err = runRootArgs(t, "coverage", "--parent", "msp", "--limit", "5", "--json")
	if err != nil || !strings.Contains(out, `"scan_capped": false`) {
		t.Fatalf("coverage below its --limit claimed a cap (err %v):\n%s", err, out)
	}
}
