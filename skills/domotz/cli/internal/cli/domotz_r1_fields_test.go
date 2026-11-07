// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 (domotz 4.34.2 reprint), two wrong-result classes on
// documented shapes, each test written to fail on the code before its fix:
//   - wrong-field-read: fleet inventory, breakdown and new read a device's
//     vendor and model only from user_data (DeviceUserDataModel, the custom
//     override), never the detected vendor and model LocalIpDevice carries, so
//     a Cisco C9300 Domotz detected read as an unknown vendor; fleet agents
//     read location.name, which AgentLocation (latitude, longitude) never
//     has, and printed the AgentVersion object as the version.
//   - time-compared-as-text: fleet speedtest picked the latest
//     NetworkSpeedSample and fleet new compared first_seen_on by string
//     order, so a sample stamped with another offset (ISO 8601 with a
//     timezone offset, as documented) won or lost against the wrong instant.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"domotz-pp-cli/internal/cliutil/testenv"
	"domotz-pp-cli/internal/store"
)

func TestFleetReadsDetectedVendorAndDocumentedAgentFields(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeDomotz(t, map[string]string{
		"100": `[{"id":2,"display_name":"Core Switch","protocol":"IP","status":"ONLINE","vendor":"Cisco","model":"C9300","user_data":null,"first_seen_on":"2026-10-29T08:00:00+00:00"},` +
			`{"id":3,"display_name":"Edge Router","protocol":"IP","status":"ONLINE","vendor":"Detected Co","model":"D-1","user_data":{"vendor":"Custom Co","model":"X1"},"first_seen_on":"2026-10-29T08:00:00+00:00"}]`,
	})
	fake.agents["100"] = `{"id":100,"display_name":"Site A","status":{"value":"OFFLINE","last_change":"2026-10-29T07:00:00+00:00"},` +
		`"version":{"agent":"5.4.1","package":"pkg-9"},"location":{"latitude":"45.07","longitude":"7.68"},"organization":{"id":1,"name":"Acme"}}`
	db := filepath.Join(t.TempDir(), "data.db")
	runDomotz(t, "sync", "--db", db, "--no-learn")

	var inv []map[string]any
	if err := json.Unmarshal([]byte(runDomotz(t, "fleet", "inventory", "--db", db, "--json", "--no-learn")), &inv); err != nil {
		t.Fatal(err)
	}
	byName := map[string]map[string]any{}
	for _, r := range inv {
		byName[asString(r["display_name"])] = r
	}
	if r := byName["Core Switch"]; asString(r["vendor"]) != "Cisco" || asString(r["model"]) != "C9300" {
		t.Errorf("fleet inventory: the detected vendor/model is not read: %v", r)
	}
	if r := byName["Edge Router"]; asString(r["vendor"]) != "Custom Co" || asString(r["model"]) != "X1" {
		t.Errorf("fleet inventory: the user's custom vendor/model does not win: %v", r)
	}
	if out := runDomotz(t, "fleet", "breakdown", "--by", "vendor", "--db", db, "--json", "--no-learn"); !strings.Contains(out, `"vendor": "Cisco"`) || strings.Contains(out, "(unknown)") {
		t.Errorf("fleet breakdown --by vendor counts a detected Cisco device as unknown:\n%s", out)
	}
	if out := runDomotz(t, "fleet", "new", "--since", "36500d", "--db", db, "--json", "--no-learn"); !strings.Contains(out, `"vendor": "Cisco"`) {
		t.Errorf("fleet new does not read the detected vendor:\n%s", out)
	}
	out := runDomotz(t, "fleet", "agents", "--db", db, "--json", "--no-learn")
	for _, w := range []string{`"location": "45.07,7.68"`, `"version": "5.4.1"`, `"organization": "Acme"`} {
		if !strings.Contains(out, w) {
			t.Errorf("fleet agents: missing %s in\n%s", w, out)
		}
	}
}

func TestFleetTimesCompareAsInstants(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now().UTC()
	// older is 3h old, written at +05:00 (its wall clock reads later than the
	// 1h cutoff); newer is 30m old, written at -05:00 (its wall clock reads
	// earlier than the cutoff).
	older := now.Add(-3 * time.Hour).In(time.FixedZone("", 5*3600)).Format(time.RFC3339)
	newer := now.Add(-30 * time.Minute).In(time.FixedZone("", -5*3600)).Format(time.RFC3339)
	fake := newFakeDomotz(t, map[string]string{
		"100": `[{"id":1,"display_name":"Old Laptop","protocol":"IP","status":"ONLINE","first_seen_on":"` + older + `"},` +
			`{"id":2,"display_name":"Rogue Phone","protocol":"IP","status":"ONLINE","first_seen_on":"` + newer + `"}]`,
	})
	// NetworkSpeedSample, the Codex input: the 11:00Z sample is the latest
	// (12:00+02:00 is 10:00Z).
	fake.bodies["/agent/100/history/network/speed"] = `[{"timestamp":"2026-10-29T11:00:00Z","values":[100000000,10000000]},{"timestamp":"2026-10-29T12:00:00+02:00","values":[1000000,1000000]}]`
	db := filepath.Join(t.TempDir(), "data.db")
	runDomotz(t, "sync", "--db", db, "--no-learn")

	out := runDomotz(t, "fleet", "new", "--since", "1h", "--db", db, "--json", "--no-learn")
	if !strings.Contains(out, "Rogue Phone") || strings.Contains(out, "Old Laptop") {
		t.Errorf("fleet new --since 1h compared first_seen_on as text (want only Rogue Phone):\n%s", out)
	}
	out = runDomotz(t, "fleet", "speedtest", "--json", "--no-learn")
	if !strings.Contains(out, `"download_mbps": 100`) || !strings.Contains(out, `"timestamp": "2026-10-29T11:00:00Z"`) {
		t.Errorf("fleet speedtest did not pick the latest sample (100 Mbps at 11:00Z):\n%s", out)
	}
}

// fleetDecodeTypes are the Go types the live novels decode each Domotz
// response into, by the endpoint whose 200 schema they read.
var fleetDecodeTypes = map[string]any{
	"/agent/{agent_id}/activity-log":                 activityLogItem{},
	"/agent/{agent_id}/history/network/speed":        speedSample{},
	"/agent/{agent_id}/ip-conflict":                  ipConflictItem{},
	"/agent/{agent_id}/device/eye/tcp":               tcpEyeItem{},
	"/alert-profile":                                 alertProfileItem{},
	"/alert-profile/binding/agent/{agent_id}/device": alertProfileDeviceBinding{},
	"/agent/{agent_id}/network-topology":             topologyDoc{},
}

// fleetReadPaths is every field path the novels read, by endpoint: the
// store-backed field maps (device list and agent list) and the json tags of
// the live novels' decode types (unalertedDeviceItem reads the device list).
func fleetReadPaths() map[string][]string {
	out := map[string]map[string]bool{}
	add := func(ep, p string) {
		if out[ep] == nil {
			out[ep] = map[string]bool{}
		}
		out[ep][p] = true
	}
	for ep, fields := range map[string]map[string]fleetField{"/agent/{agent_id}/device": fleetDeviceFields, "/agent": fleetAgentFields} {
		for _, f := range fields {
			for _, p := range f.paths {
				add(ep, strings.TrimPrefix(p, "$."))
			}
		}
	}
	var walk func(ep, prefix string, t reflect.Type)
	walk = func(ep, prefix string, t reflect.Type) {
		for t.Kind() == reflect.Slice || t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct || t == reflect.TypeOf(json.RawMessage{}) {
			return
		}
		for i := 0; i < t.NumField(); i++ {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			if name == "" || name == "-" {
				continue
			}
			add(ep, prefix+name)
			walk(ep, prefix+name+".", t.Field(i).Type)
		}
	}
	for ep, v := range fleetDecodeTypes {
		walk(ep, "", reflect.TypeOf(v))
	}
	walk("/agent/{agent_id}/device", "", reflect.TypeOf(unalertedDeviceItem{}))
	res := map[string][]string{}
	for ep, set := range out {
		for p := range set {
			res[ep] = append(res[ep], p)
		}
		sort.Strings(res[ep])
	}
	return res
}

// specAlts expands a schema into its alternatives ($ref, anyOf, oneOf; allOf
// merged).
func specAlts(spec map[string]any, x any) []map[string]any {
	m, _ := x.(map[string]any)
	for m != nil {
		ref, ok := m["$ref"].(string)
		if !ok {
			break
		}
		comps, _ := spec["components"].(map[string]any)
		schemas, _ := comps["schemas"].(map[string]any)
		m, _ = schemas[ref[strings.LastIndex(ref, "/")+1:]].(map[string]any)
	}
	if m == nil {
		return nil
	}
	var out []map[string]any
	for _, k := range []string{"anyOf", "oneOf"} {
		if list, ok := m[k].([]any); ok {
			for _, y := range list {
				out = append(out, specAlts(spec, y)...)
			}
		}
	}
	if out != nil {
		return out
	}
	if list, ok := m["allOf"].([]any); ok {
		merged := map[string]any{"type": "object", "properties": map[string]any{}}
		for _, y := range list {
			for _, a := range specAlts(spec, y) {
				if props, ok := a["properties"].(map[string]any); ok {
					for k, v := range props {
						merged["properties"].(map[string]any)[k] = v
					}
				}
			}
		}
		return []map[string]any{merged}
	}
	return []map[string]any{m}
}

// specTypes is the set of types path resolves to under schema x (descending
// into array items); empty when no alternative has the path.
func specTypes(spec map[string]any, x any, path []string) []string {
	var out []string
	for _, a := range specAlts(spec, x) {
		if a["type"] == "array" && len(path) > 0 {
			out = append(out, specTypes(spec, a["items"], path)...)
			continue
		}
		if len(path) == 0 {
			t, _ := a["type"].(string)
			out = append(out, t)
			continue
		}
		if props, ok := a["properties"].(map[string]any); ok {
			if child, ok := props[path[0]]; ok {
				out = append(out, specTypes(spec, child, path[1:])...)
			}
		}
	}
	return out
}

// TestFleetFieldsAreDocumented closes the class wrong-field-read: every field
// a novel reads resolves in the endpoint's documented 200 response
// (spec.json, when present; the paths are pinned in
// testdata/domotz-fleet-fields.json for a tree that ships no spec), every
// store-backed field resolves to a scalar, and no novel file holds a JSON
// path of its own (fleet_shared.go's field maps are the only reader).
func TestFleetFieldsAreDocumented(t *testing.T) {
	got := fleetReadPaths()
	pinFile := filepath.Join("testdata", "domotz-fleet-fields.json")
	if os.Getenv("DOMOTZ_PIN_FLEET_FIELDS") == "1" {
		b, _ := json.MarshalIndent(got, "", "  ")
		if err := os.WriteFile(pinFile, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var pinned map[string][]string
	b, err := os.ReadFile(pinFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &pinned); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, pinned) {
		t.Errorf("the fields the novels read changed; check each against spec.json and re-pin %s (DOMOTZ_PIN_FLEET_FIELDS=1):\n got %v\npin %v", pinFile, got, pinned)
	}
	if raw, err := os.ReadFile(filepath.Join("..", "..", "spec.json")); err == nil {
		var spec map[string]any
		if err := json.Unmarshal(raw, &spec); err != nil {
			t.Fatalf("spec.json: %v", err)
		}
		paths, _ := spec["paths"].(map[string]any)
		storePaths := map[string]map[string]bool{"/agent/{agent_id}/device": {}, "/agent": {}}
		for ep, fields := range map[string]map[string]fleetField{"/agent/{agent_id}/device": fleetDeviceFields, "/agent": fleetAgentFields} {
			for _, f := range fields {
				for _, p := range f.paths {
					storePaths[ep][strings.TrimPrefix(p, "$.")] = true
				}
			}
		}
		for ep, list := range pinned {
			op, _ := paths[ep].(map[string]any)
			get, _ := op["get"].(map[string]any)
			resp, _ := get["responses"].(map[string]any)
			ok200, _ := resp["200"].(map[string]any)
			content, _ := ok200["content"].(map[string]any)
			appJSON, _ := content["application/json"].(map[string]any)
			schema := appJSON["schema"]
			if schema == nil {
				t.Errorf("spec.json documents no 200 JSON response for GET %s", ep)
				continue
			}
			for _, p := range list {
				types := specTypes(spec, schema, strings.Split(p, "."))
				if len(types) == 0 {
					t.Errorf("GET %s documents no field %s (spec.json)", ep, p)
					continue
				}
				// A store-backed field is printed as it is read: it must be a
				// value, never an object or a list.
				if storePaths[ep][p] && (!slices.ContainsFunc(types, isSpecScalar) || slices.Contains(types, "object") || slices.Contains(types, "array")) {
					t.Errorf("GET %s field %s is %v, not a value a novel can print", ep, p, types)
				}
			}
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		n := e.Name()
		novel := strings.HasPrefix(n, "fleet_") || n == "drift.go" || n == "topology.go" || n == "novel_helpers.go"
		if !novel || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || n == "fleet_shared.go" {
			continue
		}
		src, err := os.ReadFile(n)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(src, []byte("json_extract(")) || bytes.Contains(src, []byte("'$.")) {
			t.Errorf("%s reads a JSON path of its own; read it through fleetDeviceFields or fleetAgentFields (fleet_shared.go)", n)
		}
	}
}

func isSpecScalar(s string) bool {
	return s == "string" || s == "integer" || s == "number" || s == "boolean"
}

// time-compared-as-text, the store's own timestamps: sync writes a
// network_topology row stamped RFC3339 ("2026-10-29T01:00:00Z") and
// topology's live fetch cached its row with CURRENT_TIMESTAMP
// ("2026-10-29 09:00:00"), so --cached ordered the strings and, within a day,
// returned the older sync row ('T' sorts after ' ').
func TestTopologyCachedReadsTheLatestRow(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.OpenWithContext(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range [][3]string{
		{"sync-row-100", `{"edges":[{"from":1,"to":2}]}`, "2026-10-29T01:00:00Z"},
		{"100", `{"edges":[{"from":3,"to":4},{"from":3,"to":5}]}`, "2026-10-29 09:00:00"},
	} {
		if _, err := s.DB().Exec(`INSERT INTO "network_topology" (id, agent_id, data, synced_at) VALUES (?, '100', ?, ?)`, r[0], r[1], r[2]); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Close()
	out := runDomotz(t, "topology", "--agent-id", "100", "--cached", "--db", db, "--json", "--no-learn")
	if !strings.Contains(out, `"edges": 2`) {
		t.Errorf("topology --cached did not read the latest cached row (2 edges, 09:00):\n%s", out)
	}
}
