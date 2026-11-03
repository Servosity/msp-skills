// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: secret-never-row-identity).

package store

import "watchguard-pp-cli/internal/cliutil"

// identityView returns obj without any secret-named field (cliutil.IsSecretFieldKey,
// at any depth), so no id-resolution path - the override, the generic and
// "<base>Key" suffix fallbacks, dotted paths, nested array-item identity or the
// parameter fingerprint - can use a credential as, or fold one into, a row
// identity. Without it an id-less activation line item keyed on its own
// activationKey, writing the key into resources.id, the typed table and FTS
// before the data blob was redacted. obj itself is returned when it holds no
// secret field.
func identityView(obj map[string]any) map[string]any {
	if obj == nil || !holdsSecretField(obj) {
		return obj
	}
	out, _ := withoutSecretFields(obj).(map[string]any)
	return out
}

func holdsSecretField(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if cliutil.IsSecretFieldKey(k) && cliutil.SecretFieldValue(child) {
				return true
			}
			if holdsSecretField(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if holdsSecretField(child) {
				return true
			}
		}
	}
	return false
}

func withoutSecretFields(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, child := range t {
			if cliutil.IsSecretFieldKey(k) && cliutil.SecretFieldValue(child) {
				continue
			}
			out[k] = withoutSecretFields(child)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, child := range t {
			out[i] = withoutSecretFields(child)
		}
		return out
	}
	return v
}

// watchguardIDFields are the vendor's own identities for resources whose spec
// rows carry no id field, tried in order ("a+b" is a composite). A
// recent-activations row is a batch (batchId), an activation-batch-status row
// a line item (lineItemId). Endpoint Security list rows (hand-fix,
// handfixes.json endpoint-security-odata-paging) carry device_id, and the
// per-device lists add what makes a row unique on that device; without these
// every Endpoint Security sync failed with "stored 0".
var watchguardIDFields = map[string][]string{
	"activations":      {"batchId", "lineItemId"},
	"endpoint_devices": {"device_id"},
	"endpoint_devices-devicesprotectionstatus": {"device_id"},
	"endpoint_unmanaged_devices":               {"device_id"},
	"endpoint_risks-devicesrisk":               {"device_id"},
	"endpoint_patches":                         {"device_id+patch_id", "device_id+patch_name"},
	"endpoint_software":                        {"device_id+name+version", "device_id+name"},
	"endpoint_risks-detectedrisks":             {"risk_factor"},
	"endpoint_risks-configuration":             {"factor"},
	"endpoint_licenses":                        {"product_id+license_type", "product_id"},
}

func watchguardResourceID(resourceType string, obj map[string]any) string {
	for _, key := range watchguardIDFields[resourceType] {
		if s := canonicalIDFromOverride(obj, key); s != "" {
			return s
		}
	}
	return ""
}
