// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Round-trips a synthetic ESET PolicyData blob through the
// decoder: base64(base64(JSON + ar{endpoint.lzma})).
package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ulikunitz/xz/lzma"
)

func buildPolicyDataBlob(t *testing.T, settings string) string {
	t.Helper()
	var compressed bytes.Buffer
	w, err := lzma.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(settings)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	member := func(name string, data []byte) []byte {
		hdr := fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", name+"/", "0", "0", "0", "100644", len(data))
		out := append([]byte(hdr), data...)
		if len(data)%2 == 1 {
			out = append(out, '\n')
		}
		return out
	}
	payload := []byte(`{"version":3,"product":"endpoint"}`)
	payload = append(payload, []byte("!<arch>\n")...)
	payload = append(payload, member("readme.txt", []byte("odd"))...)
	payload = append(payload, member("endpoint.lzma", compressed.Bytes())...)
	inner := base64.StdEncoding.EncodeToString(payload)
	return base64.StdEncoding.EncodeToString([]byte(inner)) // two layers
}

func TestDecodePolicyDataEndpointArchive(t *testing.T) {
	blob := buildPolicyDataBlob(t, `{"Settings":{"Firewall":{"Enabled":true,"Rules":[{"Name":"Block SMB"}]}}}`)
	doc := map[string]any{
		"policy": map[string]any{
			"uuid": "p1",
			"features": []any{map[string]any{
				"featureId":     7,
				"configuration": map[string]any{"@type": "type.googleapis.com/era.policy.PolicyData", "product": float64(1), "data": blob},
			}},
		},
	}
	var found []decodedPolicyData
	collectPolicyData(doc, "$", &found)
	if len(found) != 1 || found[0].Error != "" {
		t.Fatalf("decode failed: %+v", found)
	}
	d := found[0]
	if d.Layers != 2 || string(d.Decoded) != `{"version":3,"product":"endpoint"}` {
		t.Fatalf("layers=%d decoded=%s", d.Layers, d.Decoded)
	}
	if d.Path != "$.policy.features[0].configuration" || len(d.ArchiveMembers) != 2 {
		t.Fatalf("path=%s members=%d", d.Path, len(d.ArchiveMembers))
	}
	ep := d.ArchiveMembers[1]
	if ep.Name != "endpoint.lzma" || ep.Error != "" || ep.DecompressedBytes == 0 {
		t.Fatalf("endpoint member: %+v", ep)
	}

	// --find and --path work over the decoded tree.
	var tree any
	raw, _ := json.Marshal(found)
	_ = json.Unmarshal(raw, &tree)
	var matches []policyMatch
	findInJSON(tree, "$", "firewall", 10, &matches)
	if len(matches) != 1 || matches[0].Path != "$[0].archive_members[1].decoded.Settings.Firewall" {
		t.Fatalf("find: %+v", matches)
	}
	v, err := valueAtPath(tree, "$[0].archive_members[1].decoded.Settings.Firewall.Rules[0].Name")
	if err != nil || v != "Block SMB" {
		t.Fatalf("path: %v %v", v, err)
	}

	// Raw blobs are elided for display.
	elided := omitPolicyDataBlobs(doc).(map[string]any)
	cfg := elided["policy"].(map[string]any)["features"].([]any)[0].(map[string]any)["configuration"].(map[string]any)
	if s, _ := cfg["data"].(string); s == blob || s == "" {
		t.Fatal("raw blob was not elided")
	}
}

func TestDecodePolicyDataRejectsGarbage(t *testing.T) {
	if _, err := decodePolicyDataBlob("%%%not-base64%%%"); err == nil {
		t.Fatal("want an error for non-base64 input")
	}
}

func TestDecodeArArchiveReportsTruncation(t *testing.T) {
	archive := []byte("!<arch>\n" + fmt.Sprintf("%-16s%-12s%-6s%-6s%-8s%-10d`\n", "endpoint.lzma/", "0", "0", "0", "100644", 500) + "short")
	members, problem := decodeArArchive(archive)
	if problem == "" || !strings.Contains(problem, "truncated") || len(members) != 0 {
		t.Fatalf("truncated member must be reported, got members=%d problem=%q", len(members), problem)
	}
	if _, problem := decodeArArchive([]byte("garbage-after-json")); problem == "" {
		t.Fatal("non-archive trailing bytes must be reported")
	}
}

func TestDecodeLZMAMemberRefusesHugeDictionary(t *testing.T) {
	// Classic .lzma header: props byte, 4-byte little-endian dict size, 8-byte size.
	hdr := []byte{0x5d, 0, 0, 0, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	var m policyArchiveMember
	decodeLZMAMember(&m, hdr, policyLZMAMaxBytes)
	if m.Error == "" || !strings.Contains(m.Error, "dictionary") {
		t.Fatalf("a 2 GiB dictionary header must be refused before allocation, got %q", m.Error)
	}
}

func TestCollectPolicyDataSharesOneBudget(t *testing.T) {
	blob := buildPolicyDataBlob(t, `{"Settings":{"x":"`+strings.Repeat("a", 1000)+`"}}`)
	features := make([]any, 0, 3)
	for i := 0; i < 3; i++ {
		features = append(features, map[string]any{"configuration": map[string]any{"@type": "x/PolicyData", "data": blob}})
	}
	doc := map[string]any{"policy": map[string]any{"features": features}}
	budget := 2500 // room for two decoded members, not three
	var found []decodedPolicyData
	collectPolicyDataBudget(doc, "$", &found, &budget)
	if len(found) != 3 {
		t.Fatalf("want 3 blobs, got %d", len(found))
	}
	if found[2].ArchiveMembers[1].Error == "" || found[0].ArchiveMembers[1].Error != "" {
		t.Fatalf("the shared budget must stop the third blob, not the first: %+v / %+v", found[0].ArchiveMembers[1], found[2].ArchiveMembers[1])
	}
}

func TestPolicyDataExactNumbersTrailingBytesAndPaths(t *testing.T) {
	var doc map[string]any
	if err := esetDecodeExact([]byte(`{"policy":{"features":[{"featureId":9007199254740993}]}}`), &doc); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(doc)
	if !strings.Contains(string(out), "9007199254740993") {
		t.Fatalf("large feature id was rounded: %s", out)
	}

	var compressed bytes.Buffer
	w, _ := lzma.NewWriter(&compressed)
	_, _ = w.Write([]byte(`{"Settings":{"FirewallEnabled":false}}TRUNCATED_EXTRA`))
	_ = w.Close()
	var m policyArchiveMember
	decodeLZMAMember(&m, compressed.Bytes(), policyLZMAMaxBytes)
	if len(m.Decoded) == 0 || !strings.Contains(m.Error, "unexpected bytes") {
		t.Fatalf("trailing data after the settings JSON must be reported: %+v", m)
	}

	tree := []any{map[string]any{"arr": []any{1, 2}}}
	if _, err := valueAtPath(tree, "[0].arr[-1]"); err == nil {
		t.Fatal("a negative index must be refused")
	}
	if v, err := valueAtPath(tree, "[0].arr[1]"); err != nil || v != 2 {
		t.Fatalf("index 1 = %v %v", v, err)
	}
}

func TestEsetDecodeExactAndArchivePrefixStrict(t *testing.T) {
	var v any
	for _, bad := range []string{`{}]`, `{}}`, `{} {}`} {
		if err := esetDecodeExact([]byte(bad), &v); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
	if err := esetDecodeExact([]byte(" {\"a\":1} \n"), &v); err != nil {
		t.Fatalf("trailing whitespace is fine: %v", err)
	}
	if _, problem := decodeArArchive([]byte("junk!<arch>\n")); !strings.Contains(problem, "before the archive marker") {
		t.Fatalf("bytes before the marker must be reported, got %q", problem)
	}
}
