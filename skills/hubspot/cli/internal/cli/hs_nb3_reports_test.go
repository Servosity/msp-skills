// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Next-batch round 1:
//   - handfixes.json hubspot-reports-read-only-collected-properties: every
//     report that reads a stored object's properties refuses when a stored
//     record lacks one sync collects (press/tools-manifest.json:1624, the
//     properties projection of a by-id read), and every property it reads is
//     one sync collects. Checked over every report source, then driven for the
//     two reports that lacked it.
//   - handfixes.json sync-collects-deal-pipelines: GET /crm/v3/pipelines/deals
//     returns every pipeline (CollectionResponsePipelineNoPaging,
//     press/tools-manifest.json:4540), so a pipeline it no longer lists was
//     deleted (:4477) and never answers a deal report.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
)

func TestNB3EveryReportReadingStoredPropertiesRequiresThemCollected(t *testing.T) {
	tables := map[string]string{}
	for res := range hsSyncProperties {
		tables[strings.ReplaceAll(res, "-", "_")] = res
	}
	var engagements []string
	for _, info := range engagementTables {
		engagements = append(engagements, strings.ReplaceAll(info.Table, "_", "-"))
	}
	// A file named here reads a stored property without the guard, and a
	// property named here is read but never collected, for the reason given.
	exempt := map[string]string{
		"contacts_bulk_update.go": "a row whose email the store lacks is refused (no contact found, sync first); an absence is never acted on",
	}
	displayOnly := map[string]string{
		"hs_engagement_title": "not a HubSpot property (press/tools-manifest.json has none); a title fallback that reads its absence as the next title",
	}
	readRe := regexp.MustCompile(`(?:FROM|JOIN) (hubspot_[a-z0-9_]+_crm)\b`)
	guardRe := regexp.MustCompile(`hsRequireCollected\([^\n]*"(hubspot-[a-z0-9-]+-crm)"`)
	dynamicGuardRe := regexp.MustCompile(`hsRequireCollected\([^\n]*info\.Table`)
	propRe := regexp.MustCompile(`\$\.properties\.([A-Za-z0-9_]+)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || exempt[f] != "" || f == "hs_collected.go" {
			continue
		}
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		read := map[string]bool{}
		for _, m := range readRe.FindAllStringSubmatch(src, -1) {
			if res, ok := tables[m[1]]; ok {
				read[res] = true
			}
		}
		guarded := map[string]bool{}
		for _, m := range guardRe.FindAllStringSubmatch(src, -1) {
			guarded[m[1]] = true
		}
		if dynamicGuardRe.MatchString(src) {
			for _, res := range engagements {
				read[res], guarded[res] = true, true
			}
		}
		if len(read) == 0 || !propRe.MatchString(src) {
			continue
		}
		checked++
		for res := range read {
			if !guarded[res] {
				t.Errorf("%s reads stored %s properties without hsRequireCollected for it", f, res)
			}
		}
		for _, m := range propRe.FindAllStringSubmatch(src, -1) {
			collected := displayOnly[m[1]] != ""
			for res := range read {
				for _, p := range strings.Split(hsCollectedProperties(res, ""), ",") {
					collected = collected || p == m[1]
				}
			}
			if !collected {
				t.Errorf("%s reads %s, which sync does not collect for any of %v", f, m[1], read)
			}
		}
	}
	if checked < 10 {
		t.Fatalf("checked only %d report files; the scan no longer finds the reports", checked)
	}
}

var nb3OpenDeal = map[string][]map[string]string{
	"0-3": {{"hs_object_id": "1", "dealname": "Open", "amount": "10000", "closedate": "2026-11-15T00:00:00Z", "dealstage": "qualifiedtobuy", "pipeline": "default", "hs_is_closed": "false", "hs_is_closed_won": "false", "hs_lastmodifieddate": "2026-10-20T00:00:00Z"}},
}

// nb3ServeTCPipelines puts the deal pipelines in front of the portal, so sync
// stores them in this test's store (the default store is scoped per portal).
func nb3ServeTCPipelines(t *testing.T) {
	t.Helper()
	portal, err := url.Parse(os.Getenv("HUBSPOT_BASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(portal)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/crm/v3/pipelines/deals" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, tcPipelines)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-pipelines-crm", "--json"); err != nil {
		t.Fatalf("pipelines sync: %v\n%s", err, stderr)
	}
}

func TestNB3ForecastAndVelocityRefuseAnUncollectedProperty(t *testing.T) {
	for _, report := range [][]string{{"deals", "forecast", "--json"}, {"deals", "velocity", "--json"}} {
		t.Run(report[1], func(t *testing.T) {
			testenv.Isolate(t)
			newNBPortal(t, nb3OpenDeal)
			nb3ServeTCPipelines(t)
			if _, stderr, err := r3dRun(t, "hubspot-deals-crm", "get-v3-objects-0-3-deal-id-get-by-id", "1", "--properties", "pipeline,dealstage,hs_is_closed", "--json"); err != nil {
				t.Fatalf("projected read: %v\n%s", err, stderr)
			}
			out, stderr, err := r3dRun(t, report...)
			if err == nil {
				t.Fatalf("deals %s answered from a deal stored without amount or closedate:\n%s", report[1], out)
			}
			if msg := err.Error() + stderr; !strings.Contains(msg, "hubspot-cli sync --resources hubspot-deals-crm --full") {
				t.Errorf("refusal %q does not name the full sync", msg)
			}
			if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-deals-crm", "--full", "--json"); err != nil {
				t.Fatalf("full sync: %v\n%s", err, stderr)
			}
			out, stderr, err = r3dRun(t, report...)
			if err != nil {
				t.Fatalf("deals %s after the named sync: %v\n%s", report[1], err, stderr)
			}
			if report[1] == "forecast" && !strings.Contains(out, `"total_weighted": 4000`) {
				t.Errorf("forecast after the named sync does not weigh the $10,000 deal at 0.4:\n%s", out)
			}
		})
	}
}

// Pipeline 12345 "Renewals" is deleted and 67890 is created with the same
// label and an open $10,000 deal.
func nb3PipelineListing(id string) string {
	return `{"results":[{"id":"` + id + `","label":"Renewals","displayOrder":0,"archived":false,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","stages":[` +
		`{"id":"renew-` + id + `","label":"Renewing","displayOrder":0,"archived":false,"metadata":{"isClosed":"false","probability":"0.5"}}]}]}`
}

func TestNB3APipelineTheFullListingNoLongerHasNeverAnswersADealReport(t *testing.T) {
	for _, how := range []string{"sync", "live-read"} {
		t.Run(how, func(t *testing.T) {
			testenv.Isolate(t)
			var listing atomic.Value
			listing.Store(nb3PipelineListing("12345"))
			deals := `{"results":[{"id":"9","properties":{"dealname":"Renewal","amount":"10000","closedate":"2026-11-15T00:00:00Z","dealstage":"renew-67890","pipeline":"67890","createdate":"2026-01-01T00:00:00Z","hs_lastmodifieddate":"2026-10-20T00:00:00Z","hs_object_id":"9","hs_is_closed":"false","hs_is_closed_won":"false","hubspot_owner_id":"7","notes_last_contacted":null,"notes_last_updated":null},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-10-20T00:00:00Z","archived":false}]}`
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/crm/v3/pipelines/deals":
					_, _ = fmt.Fprint(w, listing.Load().(string))
				case "/crm/v3/objects/0-3":
					_, _ = fmt.Fprint(w, nbHonourProperties(r, deals))
				default:
					_, _ = fmt.Fprint(w, `{"results":[]}`)
				}
			}))
			t.Cleanup(srv.Close)
			t.Setenv("HUBSPOT_BASE_URL", srv.URL)
			t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a33")
			if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-pipelines-crm", "--json"); err != nil {
				t.Fatalf("pipelines sync: %v\n%s", err, stderr)
			}
			listing.Store(nb3PipelineListing("67890"))
			if how == "live-read" {
				if _, stderr, err := r3dRun(t, "hubspot-pipelines-crm", "get-v3-pipelines-object-type-get-all", "deals", "--json"); err != nil {
					t.Fatalf("live pipelines read: %v\n%s", err, stderr)
				}
				if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-deals-crm", "--full", "--json"); err != nil {
					t.Fatalf("deals sync: %v\n%s", err, stderr)
				}
			} else if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-pipelines-crm,hubspot-deals-crm", "--full", "--json"); err != nil {
				t.Fatalf("full sync: %v\n%s", err, stderr)
			}
			out, stderr, err := r3dRun(t, "pipeline-health", "Renewals", "--json")
			if err != nil {
				t.Fatalf("pipeline-health Renewals: %v\n%s", err, stderr)
			}
			if !strings.Contains(out, `"open_amount": 10000`) || strings.Contains(out, "12345") {
				t.Errorf("pipeline-health Renewals answered from the deleted pipeline 12345:\n%s", out)
			}
		})
	}
}
