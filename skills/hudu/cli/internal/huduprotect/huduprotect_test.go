// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): pins the resource rule the
// client and the store share: which paths and store resource types are
// protected, which asset field types are secret, and how a field entry is
// decided on the wire and in the store.

package huduprotect

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClassifyPathByResource(t *testing.T) {
	cases := map[string]Class{
		"/asset_passwords":                      Vault,
		"/asset_passwords/7":                    Vault,
		"/companies/3/asset_passwords":          Vault,
		"/asset%5Fpasswords":                    Vault,
		"/asset%255Fpasswords/7":                Vault,
		"/api/v1/asset_passwords/7":             Vault,
		"/assets":                               Asset,
		"/assets/5":                             Asset,
		"/companies/3/assets":                   Asset,
		"/companies/3/assets/5/archive":         Asset,
		"/api/v1/assets":                        Asset,
		"/ASSETS/5":                             Asset,
		"/companies/3/%61ssets":                 Asset,
		"/companies%2F3%2Fassets":               Asset,
		"/assets/5/asset_passwords":             Vault,
		"/asset_layouts":                        None,
		"/asset_layouts/3":                      None,
		"/assets-report":                        None,
		"/companies":                            None,
		"/companies?type=asset_passwords":       None,
		"/companies/3?include=assets":           None,
		"/expirations?resource=asset_passwords": None,
		"":                                      None,
	}
	for p, want := range cases {
		if got := ClassifyPath(p); got != want {
			t.Errorf("ClassifyPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestClassifyStoreResourceTypeCoversPartitionNames(t *testing.T) {
	cases := map[string]Class{
		"asset-passwords":           Vault,
		"asset_passwords":           Vault,
		"companies-asset-passwords": Vault,
		"assets":                    Asset,
		"Assets":                    Asset,
		"companies-assets":          Asset,
		"companies_assets":          Asset,
		"assets-list-by-company":    Asset,
		"asset-layouts":             None,
		"asset_layouts":             None,
		"companies":                 None,
		"":                          None,
	}
	for rt, want := range cases {
		if got := ClassifyStoreResourceType(rt); got != want {
			t.Errorf("ClassifyStoreResourceType(%q) = %v, want %v", rt, got, want)
		}
	}
}

func TestFieldTypeIsSecretAllowlist(t *testing.T) {
	for _, ft := range []string{"Text", "RichText", "Heading", "CheckBox", "Website", "Email", "Number", "Date", "Dropdown", "Embed", "Phone", "AssetLink", "AssetTag", "ListSelect", "rich_text", "Asset Link", "list-select"} {
		if FieldTypeIsSecret(ft) {
			t.Errorf("FieldTypeIsSecret(%q) = true, want false", ft)
		}
	}
	for _, ft := range []string{"Password", "ConfidentialText", "Confidential Text", "Totp", "", "  "} {
		if !FieldTypeIsSecret(ft) {
			t.Errorf("FieldTypeIsSecret(%q) = false, want true", ft)
		}
	}
}

// The wire rule: any secret signal wins (the entry's own type or the
// layout's), a missing layout or label is secret, the decided type is
// written onto the entry, and empty values stay empty.
func TestFilterAssetBodyDecidesEveryFieldEntry(t *testing.T) {
	layouts := map[string]map[string]string{
		"3": {NormalizeLabel("Admin Password"): "Password", NormalizeLabel("Hostname"): "Text", NormalizeLabel("Notes"): "ConfidentialText"},
	}
	lookup := func(id string) (map[string]string, bool) {
		types, ok := layouts[id]
		return types, ok
	}
	body := `{"assets":[
		{"id":1,"asset_layout_id":3,"fields":[
			{"label":"Admin Password","value":"pw-1"},
			{"label":"Hostname","value":"dc01"},
			{"label":"Hostname","value":"dc01b","field_type":"Password"},
			{"label":"Notes","value":""},
			{"label":"Not In Layout","value":"x"}]},
		{"id":2,"asset_layout_id":9,"fields":[{"label":"Hostname","value":"dc02"}]},
		{"id":3,"fields":[{"label":"Hostname","value":"dc03"}]},
		{"id":4,"asset_layout_id":3,"custom_fields":[{"admin_password":"pw-4","hostname":"dc04","not_in_layout":"u-4"}]}
	],"meta":{"page":1}}`
	out, err := FilterAssetBody(json.RawMessage(body), lookup)
	if err != nil {
		t.Fatalf("FilterAssetBody: %v", err)
	}
	for _, leaked := range []string{"pw-1", "dc01b", "dc02", "dc03", "pw-4", "u-4", `"x"`} {
		if strings.Contains(string(out), leaked) {
			t.Errorf("value %s was not withheld: %s", leaked, out)
		}
	}
	for _, kept := range []string{`"value":"dc01"`, `"field_type":"Text"`, `"field_type":"Password"`, `"field_type":"Unknown"`, `"page":1`} {
		if !strings.Contains(string(out), kept) {
			t.Errorf("want %s in %s", kept, out)
		}
	}
	if !strings.Contains(string(out), `{"field_type":"ConfidentialText","label":"Notes","value":""}`) {
		t.Errorf("an empty secret value was changed: %s", out)
	}
	// custom_fields keys are Hudu's snake_case labels: admin_password is the
	// layout's Password field, hostname its Text field, not_in_layout unknown.
	if !strings.Contains(string(out), `"hostname":"dc04"`) {
		t.Errorf("a custom_fields Text value was not kept: %s", out)
	}

	// Filtering an already filtered body changes nothing.
	again, err := FilterAssetBody(out, lookup)
	if err != nil || string(again) != string(out) {
		t.Fatalf("a second pass changed the body: err=%v\n%s\n%s", err, out, again)
	}

	for _, bad := range []string{`"text"`, `42`, `not json`, `{"assets":7}`, `[1,2]`, `{"id":1,"fields":"pw"}`} {
		if _, err := FilterAssetBody(json.RawMessage(bad), lookup); err == nil {
			t.Errorf("FilterAssetBody(%s) classified a body it cannot read", bad)
		}
	}
}

// The store rule is the wire's conjunction: a value is kept only when both its
// own field_type (if any) and the stored layout's type for its label are plain;
// a layout that is not stored, or lacks the label, is secret.
func TestScrubAssetDocumentStoreRule(t *testing.T) {
	stored := func(id string) (map[string]string, bool) {
		if id == "3" {
			return map[string]string{NormalizeLabel("Hostname"): "Text", NormalizeLabel("Admin Password"): "Password", NormalizeLabel("Serial"): "Text", NormalizeLabel("Key"): "Text"}, true
		}
		return nil, false
	}
	row := `{"id":5,"asset_layout_id":3,"fields":[
		{"id":1,"label":"Hostname","value":"dc01"},
		{"id":2,"label":"Admin Password","value":"pw-old"},
		{"id":3,"label":"Serial","value":"sn-1","field_type":"Text"},
		{"id":4,"label":"Key","value":"k-1","field_type":"Password"},
		{"id":5,"label":"Admin Password","value":"pw-typed-plain","field_type":"Text"},
		{"id":6,"label":"Not In Layout","value":"nl-1","field_type":"Text"}]}`
	out := string(ScrubAssetDocument(json.RawMessage(row), stored))
	for _, leaked := range []string{"pw-old", "k-1", "pw-typed-plain", "nl-1"} {
		if strings.Contains(out, leaked) {
			t.Errorf("value %s survived the store rule: %s", leaked, out)
		}
	}
	for _, kept := range []string{"dc01", "sn-1"} {
		if !strings.Contains(out, kept) {
			t.Errorf("non-secret value %s was dropped: %s", kept, out)
		}
	}
	noLayout := string(ScrubAssetDocument(json.RawMessage(`{"id":6,"asset_layout_id":4,"fields":[{"label":"Hostname","value":"dc02"}]}`), stored))
	if strings.Contains(noLayout, "dc02") {
		t.Errorf("a field whose layout is not stored was kept: %s", noLayout)
	}
	untouched := `{"id":7,"name":"no fields"}`
	if got := string(ScrubAssetDocument(json.RawMessage(untouched), stored)); got != untouched {
		t.Errorf("a row with nothing to withhold lost content: %s", got)
	}
}

func TestVaultEntryAllowlistIsExact(t *testing.T) {
	out, err := FilterVaultBody(json.RawMessage(`{"asset_passwords":[{"id":1,"name":"vpn","password":"p","otp_secret":"o","otp_secret_at":"s","rotated_at":"r","updated_at":"u"}],"meta":{"page":1}}`))
	if err != nil {
		t.Fatalf("FilterVaultBody: %v", err)
	}
	for _, gone := range []string{`"password"`, `"otp_secret"`, "otp_secret_at", "rotated_at"} {
		if strings.Contains(string(out), gone) {
			t.Errorf("%s survived the exact allowlist: %s", gone, out)
		}
	}
	if !strings.Contains(string(out), `"updated_at":"u"`) || !strings.Contains(string(out), `"page":1`) {
		t.Errorf("allowlisted metadata was dropped: %s", out)
	}
	scrubbed := string(ScrubVaultDocument(json.RawMessage(`{"id":1,"name":"vpn","description":{"password":"p"},"password":"p"}`)))
	if strings.Contains(scrubbed, `"p"`) || !strings.Contains(scrubbed, `"name":"vpn"`) {
		t.Errorf("the store vault scrub kept a secret or lost metadata: %s", scrubbed)
	}
	if got := string(ScrubVaultDocument(json.RawMessage(`not json`))); got != `{}` {
		t.Errorf("an unreadable vault row became %s, want {}", got)
	}
}
