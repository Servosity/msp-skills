// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written tests for the iocs novel feature.

package cli

import "testing"

func sampleReports() []forensicReport {
	return []forensicReport{
		{
			Name:  "https://evil.example.test/payload",
			Scope: "threat",
			Type:  "url",
			ID:    "threat-1",
			Forensics: []forensicEvidence{
				{
					Type:      "attachment",
					Malicious: true,
					What:      map[string]any{"sha256": "aa11", "md5": "bb22"},
					Platforms: []forensicPlatform{{Name: "Win10", OS: "windows"}},
				},
				{
					Type:      "url",
					Malicious: false,
					What:      map[string]any{"url": "https://evil.example.test/dl", "ip": "203.0.113.9"},
				},
				{
					Type:      "dns",
					Malicious: true,
					What:      map[string]any{"host": "evil.example.test", "cnames": []any{"alias.example.test"}, "ips": []any{"198.51.100.4"}},
				},
				{
					Type:      "registry",
					Malicious: true,
					What:      map[string]any{"key": `HKCU\Software\Run\updater`},
				},
				{
					Type:      "screenshot",
					Malicious: false,
					What:      map[string]any{"url": "https://screens.example.test/1.png"},
				},
			},
		},
	}
}

func TestFlattenForensicReportsExtractsAllIndicatorTypes(t *testing.T) {
	rows := flattenForensicReports(sampleReports(), false)

	want := map[string]string{
		"sha256":       "aa11",
		"md5":          "bb22",
		"url":          "https://evil.example.test/dl",
		"ip":           "203.0.113.9",
		"registry_key": `HKCU\Software\Run\updater`,
	}
	got := map[string]string{}
	domains := 0
	ips := 0
	for _, row := range rows {
		if row.IndicatorType == "domain" {
			domains++
			continue
		}
		if row.IndicatorType == "ip" {
			ips++
		}
		if _, tracked := want[row.IndicatorType]; tracked && got[row.IndicatorType] == "" {
			got[row.IndicatorType] = row.Value
		}
		if row.IndicatorType == "screenshot" {
			t.Fatalf("screenshot evidence must not produce indicator rows, got %+v", row)
		}
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("indicator %s = %q, want %q", k, got[k], v)
		}
	}
	if domains != 2 { // host + cname
		t.Errorf("domains = %d, want 2 (host + cname)", domains)
	}
	if ips != 2 { // url evidence ip + dns ip
		t.Errorf("ips = %d, want 2 (url ip + dns ip)", ips)
	}
	for _, row := range rows {
		if row.ReportID != "threat-1" {
			t.Errorf("row %s missing report attribution: %+v", row.IndicatorType, row)
		}
	}
}

func TestFlattenForensicReportsMaliciousOnly(t *testing.T) {
	rows := flattenForensicReports(sampleReports(), true)
	for _, row := range rows {
		if !row.Malicious {
			t.Fatalf("malicious-only output contains non-malicious row: %+v", row)
		}
		if row.IndicatorType == "url" && row.Value == "https://evil.example.test/dl" {
			t.Fatalf("non-malicious url evidence leaked through the filter")
		}
	}
	if len(rows) == 0 {
		t.Fatal("expected malicious rows to survive the filter")
	}
}

func TestFlattenForensicReportsEmpty(t *testing.T) {
	if rows := flattenForensicReports(nil, false); len(rows) != 0 {
		t.Fatalf("nil reports must produce zero rows, got %d", len(rows))
	}
	if rows := flattenForensicReports([]forensicReport{{ID: "t", Forensics: []forensicEvidence{{Type: "behavior"}}}}, false); len(rows) != 0 {
		t.Fatalf("indicator-less evidence must produce zero rows, got %d", len(rows))
	}
}
