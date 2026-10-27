// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package huduprotect is the one rule for Hudu tenant secrets (hand-fix
// handfixes.json vault-secrets-withheld). Two Hudu resources answer with
// secrets in plaintext: the password vault (/asset_passwords returns each
// entry's `password` and `otp_secret`) and assets (/assets returns the value of
// every asset layout field, Password and ConfidentialText fields included, as
// `fields: [{"label","value"}]` with no type). Both the HTTP client and the
// SQLite store import this package, so the decision is made once and the same
// way on the wire, in the response cache, in the store funnel and in the
// store's one-time upgrade scrub.
//
// The rule is decided by RESOURCE, never by the shape of a body: any response
// whose requested or answered URL names a protected resource is filtered, and
// any stored row whose resource type names one is scrubbed.
package huduprotect

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Class is the protected-resource class of a URL path or a store resource type.
type Class int

const (
	// None is not a protected resource.
	None Class = iota
	// Asset is Hudu's assets resource; password-type field values are secret.
	Asset
	// Vault is Hudu's password vault; only an allowlist of metadata is kept.
	Vault
)

// WithheldFieldMarker replaces the value of a non-empty password-type asset
// field, so an audit still sees the field as filled.
const WithheldFieldMarker = "[withheld: password field]"

// UnknownFieldType is the decided field_type written onto an asset field entry
// whose layout type could not be resolved. It is not an allowlisted type, so
// the store treats the entry as secret too.
const UnknownFieldType = "Unknown"

// vaultResourceMarker names the vault in a path. The singular form also
// matches the plural route and any route nested under it.
const vaultResourceMarker = "asset_password"

// assetResourceSegment is the path segment that names the assets resource.
const assetResourceSegment = "assets"

// Strongest returns the stronger of two classes: Vault over Asset over None.
func Strongest(a, b Class) Class {
	if a > b {
		return a
	}
	return b
}

// ClassifyPath reports the protected class a request or answered URL path
// names. The query (after the first literal '?') is ignored. The rest is
// lowercased and percent-decoded until it stops changing, so encoded and
// double-encoded forms still match. Vault wins over Asset.
func ClassifyPath(path string) Class {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	p := strings.ToLower(path)
	for {
		next := strings.ToLower(lenientPercentDecode(p))
		if next == p {
			break
		}
		p = next
	}
	if strings.Contains(p, vaultResourceMarker) {
		return Vault
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == assetResourceSegment {
			return Asset
		}
	}
	return None
}

// ClassifyStoreResourceType reports the protected class of a store resource
// type. The name is lowercased with '_' read as '-', so the sync partition
// (asset-passwords, assets), a snake_case legacy name, and a parent-sharded
// write-through partition (companies-assets, companies_asset_passwords) all
// match.
func ClassifyStoreResourceType(resourceType string) Class {
	rt := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(resourceType)), "_", "-")
	switch {
	case strings.Contains(rt, "asset-password"):
		return Vault
	case rt == "assets", strings.HasPrefix(rt, "assets-"), strings.HasSuffix(rt, "-assets"):
		return Asset
	default:
		return None
	}
}

// lenientPercentDecode decodes every well-formed %XX escape and leaves a
// malformed one in place, so one bad escape cannot stop the rest of the path
// from being decoded. Each pass that changes the string makes it shorter, so
// the caller's decode-until-stable loop always ends.
func lenientPercentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2]) {
			b.WriteByte(unhexDigit(s[i+1])<<4 | unhexDigit(s[i+2]))
			i += 2
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isHexDigit(c byte) bool {
	return ('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

func unhexDigit(c byte) byte {
	switch {
	case '0' <= c && c <= '9':
		return c - '0'
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10
	default:
		return c - 'A' + 10
	}
}

// ---------------------------------------------------------------------------
// Password vault
// ---------------------------------------------------------------------------

// The two envelope keys Hudu wraps vault entries in: a list answers
// {"asset_passwords":[...]}, a single get, create or update answers
// {"asset_password":{...}}.
const (
	vaultListEnvelopeKey   = "asset_passwords"
	vaultSingleEnvelopeKey = "asset_password"
)

// vaultEntryKeys is the exact allowlist a vault entry keeps. There is no
// suffix rule: every other key, including `password`, `otp_secret`, any
// `*_at` key Hudu adds later and anything else unknown, is dropped.
var vaultEntryKeys = map[string]bool{
	"id":                   true,
	"company_id":           true,
	"company_name":         true,
	"passwordable_id":      true,
	"passwordable_type":    true,
	"name":                 true,
	"username":             true,
	"url":                  true,
	"login_url":            true,
	"description":          true,
	"password_type":        true,
	"password_folder_id":   true,
	"password_folder_name": true,
	"slug":                 true,
	"archived":             true,
	"in_portal":            true,
	"object_type":          true,
	"created_at":           true,
	"updated_at":           true,
}

// VaultEntryKey reports whether a vault entry keeps this key.
func VaultEntryKey(key string) bool {
	return vaultEntryKeys[key]
}

// SecretKeyName reports whether a field name looks like it holds a secret:
// password, *_password, passwd, anything with otp or secret in it.
// `password_folder_name` and `password_type` are metadata and do not match.
func SecretKeyName(key string) bool {
	k := strings.ToLower(key)
	return k == "password" || strings.HasSuffix(k, "_password") || strings.Contains(k, "passwd") ||
		strings.Contains(k, "otp") || strings.Contains(k, "secret")
}

// FilterVaultBody rewrites one successful vault-resource body. The shapes it
// classifies: an empty body or JSON null (nothing to withhold), a bare array
// of entries, a bare entry object, and an object carrying the asset_passwords
// and/or asset_password envelope keys. Other envelope keys are kept untouched
// unless their name or anything nested under them names a secret field, in
// which case the whole body is refused. Anything else is refused.
func FilterVaultBody(data json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return data, nil
	}
	if !json.Valid(trimmed) {
		return nil, errors.New("the response is not JSON")
	}
	switch trimmed[0] {
	case '[':
		entries, err := filterVaultEntryList(trimmed)
		if err != nil {
			return nil, err
		}
		return Marshal(entries)
	case '{':
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &envelope); err != nil {
			return nil, errors.New("the response is not a JSON object the filter can read")
		}
		_, hasList := envelope[vaultListEnvelopeKey]
		_, hasSingle := envelope[vaultSingleEnvelopeKey]
		if !hasList && !hasSingle {
			entry, err := filterVaultEntry(envelope)
			if err != nil {
				return nil, err
			}
			return Marshal(entry)
		}
		out := make(map[string]json.RawMessage, len(envelope))
		for key, value := range envelope {
			switch key {
			case vaultListEnvelopeKey:
				if isJSONNull(value) {
					out[key] = value
					continue
				}
				entries, err := filterVaultEntryList(value)
				if err != nil {
					return nil, err
				}
				encoded, err := Marshal(entries)
				if err != nil {
					return nil, err
				}
				out[key] = encoded
			case vaultSingleEnvelopeKey:
				if isJSONNull(value) {
					out[key] = value
					continue
				}
				var obj map[string]json.RawMessage
				if err := json.Unmarshal(value, &obj); err != nil || obj == nil {
					return nil, fmt.Errorf("its %q value is not an object", vaultSingleEnvelopeKey)
				}
				entry, err := filterVaultEntry(obj)
				if err != nil {
					return nil, err
				}
				encoded, err := Marshal(entry)
				if err != nil {
					return nil, err
				}
				out[key] = encoded
			default:
				if SecretKeyName(key) || rawNamesSecret(value) {
					return nil, fmt.Errorf("its %q field carries secret fields outside the vault entries", key)
				}
				out[key] = value
			}
		}
		return Marshal(out)
	default:
		return nil, errors.New("the response is a bare JSON value, not a vault entry")
	}
}

func filterVaultEntryList(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("its entry list is not a JSON array")
	}
	out := make([]map[string]json.RawMessage, 0, len(items))
	for _, item := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil || obj == nil {
			return nil, errors.New("its entry list holds a value that is not an object")
		}
		entry, err := filterVaultEntry(obj)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, nil
}

// filterVaultEntry keeps only the allowlisted keys of one entry. A kept value
// that is itself an object or array must not name a secret field.
func filterVaultEntry(obj map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(obj))
	for key, value := range obj {
		if !VaultEntryKey(key) {
			continue
		}
		if rawNamesSecret(value) {
			return nil, fmt.Errorf("its %q field carries secret fields", key)
		}
		out[key] = value
	}
	return out, nil
}

// ScrubVaultDocument is the store's form of the vault rule: it never fails.
// A row FilterVaultBody accepts is stored as filtered. Any other row keeps
// only its allowlisted keys that hold a scalar; a row that is not a JSON
// object becomes {}.
func ScrubVaultDocument(data json.RawMessage) json.RawMessage {
	if out, err := FilterVaultBody(data); err == nil && len(bytes.TrimSpace(out)) > 0 && !isJSONNull(out) {
		return out
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(data), &obj); err != nil || obj == nil {
		return json.RawMessage(`{}`)
	}
	out := make(map[string]json.RawMessage, len(obj))
	for key, value := range obj {
		if !VaultEntryKey(key) || rawIsContainer(value) {
			continue
		}
		out[key] = value
	}
	encoded, err := Marshal(out)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

func rawIsContainer(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

// rawNamesSecret reports whether any object nested anywhere in raw has a field
// whose name looks like a secret. Scalars never do.
func rawNamesSecret(raw json.RawMessage) bool {
	if !rawIsContainer(raw) {
		return false
	}
	v, err := decodeValue(raw)
	if err != nil {
		// Unreadable nested JSON cannot be proven clean.
		return true
	}
	return valueNamesSecret(v)
}

func valueNamesSecret(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for key, child := range t {
			if SecretKeyName(key) || valueNamesSecret(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if valueNamesSecret(child) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Assets: password-type field values
// ---------------------------------------------------------------------------

// nonSecretFieldTypes is the allowlist of asset layout field types whose
// values are not secret, normalized by normalizeFieldType. Every other type,
// Password, ConfidentialText, a type Hudu adds later, or no type at all, is
// secret.
var nonSecretFieldTypes = map[string]bool{
	"text":       true,
	"richtext":   true,
	"heading":    true,
	"checkbox":   true,
	"website":    true,
	"email":      true,
	"number":     true,
	"date":       true,
	"dropdown":   true,
	"embed":      true,
	"phone":      true,
	"assetlink":  true,
	"assettag":   true,
	"listselect": true,
}

func normalizeFieldType(fieldType string) string {
	t := strings.ToLower(strings.TrimSpace(fieldType))
	return strings.NewReplacer(" ", "", "_", "", "-", "").Replace(t)
}

// FieldTypeIsSecret reports whether an asset layout field type holds a secret
// value. Only the allowlisted non-secret types return false.
func FieldTypeIsSecret(fieldType string) bool {
	return !nonSecretFieldTypes[normalizeFieldType(fieldType)]
}

// NormalizeLabel is the form field labels are compared in: lowercased, with
// spaces, underscores and hyphens removed, so a layout label ("Admin
// Password") matches both a `fields` entry label and the snake_case key Hudu
// uses for it in `custom_fields` ("admin_password").
func NormalizeLabel(label string) string {
	return strings.NewReplacer(" ", "", "_", "", "-", "").Replace(strings.ToLower(strings.TrimSpace(label)))
}

// LayoutLookup returns the field types of one asset layout, keyed by
// NormalizeLabel(label). ok is false when the layout could not be read.
type LayoutLookup func(layoutID string) (types map[string]string, ok bool)

// ParseLayoutFieldTypes reads an asset layout body: Hudu answers GET
// /asset_layouts/{id} with {"asset_layout":{...,"fields":[{"label","field_type"}]}},
// and the store keeps the bare layout object. A label declared more than once
// keeps a secret type if any declaration is secret. ok is false when the body
// has no fields list.
func ParseLayoutFieldTypes(data json.RawMessage) (map[string]string, bool) {
	v, err := decodeValue(data)
	if err != nil {
		return nil, false
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	if inner, ok := obj["asset_layout"].(map[string]any); ok {
		obj = inner
	}
	fields, ok := obj["fields"].([]any)
	if !ok {
		return nil, false
	}
	types := make(map[string]string, len(fields))
	for _, f := range fields {
		entry, ok := f.(map[string]any)
		if !ok {
			continue
		}
		label, _ := entry["label"].(string)
		if strings.TrimSpace(label) == "" {
			continue
		}
		fieldType, _ := entry["field_type"].(string)
		key := NormalizeLabel(label)
		if prior, seen := types[key]; seen && FieldTypeIsSecret(prior) {
			continue
		}
		types[key] = fieldType
	}
	return types, true
}

// AssetLayoutID returns an asset's asset_layout_id as a decimal string, or ""
// when it is absent or not a positive integer (so it can never steer a lookup
// path anywhere else).
func AssetLayoutID(asset map[string]any) string {
	var s string
	switch t := asset["asset_layout_id"].(type) {
	case json.Number:
		s = t.String()
	case float64:
		s = strconv.FormatFloat(t, 'f', -1, 64)
	case string:
		s = strings.TrimSpace(t)
	default:
		return ""
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n == 0 {
		return ""
	}
	return strconv.FormatUint(n, 10)
}

// FilterAssetBody is the client's asset rule for one successful asset-resource
// body. Every object at any depth that carries a field-values key is scrubbed
// as an asset, whatever envelope holds it ({"assets":[...]}, {"asset":{...}},
// {"data":{"asset":{...}}}, a bare array or a bare object): for every field
// entry it decides by the entry's own field_type if present AND by the
// layout's type for that label (any secret signal wins; a layout that cannot
// be read, an asset with no layout id, or a layout that lacks the label is a
// secret signal). The decided type is written onto the entry as field_type and
// a secret non-empty value is replaced by WithheldFieldMarker. Each layout is
// looked up once per body, so every asset of one layout in it is decided the
// same way and no earlier body's answer is reused. The inspected tree is always
// what is returned, never the input bytes, so a member the decoder did not keep
// (a duplicate key) cannot carry a value past it. An empty body or JSON null
// passes; any other body it cannot classify is refused.
func FilterAssetBody(data json.RawMessage, layout LayoutLookup) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return data, nil
	}
	if !json.Valid(trimmed) {
		return nil, errors.New("the response is not JSON")
	}
	v, err := decodeValue(trimmed)
	if err != nil {
		return nil, errors.New("the response is not JSON the filter can read")
	}
	switch v.(type) {
	case map[string]any, []any:
	default:
		return nil, errors.New("the response is a bare JSON value, not an asset")
	}
	s := &assetScrubber{layout: layout, client: true}
	if err := checkAssetEnvelope(v); err != nil {
		return nil, err
	}
	if err := s.scrubTree(v); err != nil {
		return nil, err
	}
	return Marshal(v)
}

// ScrubAssetDocument is the store's form of the asset rule: it never fails.
// Every object at any depth that carries a field-values key is scrubbed: a
// field entry is secret unless both its own field_type (if any) and the stored
// layout's type for its label are allowlisted non-secret types; a layout that
// is not stored, or lacks the label, makes it secret. A secret non-empty value
// is replaced by WithheldFieldMarker. The row is always re-encoded from the
// scrubbed tree, never kept as stored. A row that is not JSON becomes {}.
func ScrubAssetDocument(data json.RawMessage, layout LayoutLookup) json.RawMessage {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return data
	}
	v, err := decodeValue(trimmed)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	switch v.(type) {
	case map[string]any, []any:
	default:
		return json.RawMessage(`{}`)
	}
	s := &assetScrubber{layout: layout}
	_ = s.scrubTree(v)
	encoded, err := Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

// ScrubUnknownProvenanceDocument is the store's rule for a row of an
// UNPROTECTED resource type written before this binary recorded the URL that
// answered each response. An older binary that followed a redirect into the
// vault or assets stored that answer under the requested resource's name, and
// nothing on disk says which rows those are, so every such row is treated as
// possibly protected: every key named like a secret (SecretKeyName) is dropped
// at any depth, and every object carrying field values is scrubbed as an
// asset (ScrubAssetDocument's rule). No documented unprotected Hudu shape
// holds either, so those rows keep their content; each is re-encoded from the
// scrubbed tree. A row that is not JSON becomes {}.
func ScrubUnknownProvenanceDocument(data json.RawMessage, layout LayoutLookup) json.RawMessage {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return data
	}
	v, err := decodeValue(trimmed)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	s := &assetScrubber{layout: layout}
	s.dropSecretKeys(v)
	_ = s.scrubTree(v)
	encoded, err := Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return encoded
}

// isAssetFieldKey reports the keys that carry layout field values: `fields`
// is Hudu's read shape; `custom_fields` is its write shape, which a row or an
// echoed body may also carry. Case, spaces, underscores and hyphens are
// ignored, so `Fields` or `customFields` is caught too.
func isAssetFieldKey(key string) bool {
	k := NormalizeLabel(key)
	return k == "fields" || k == "customfields"
}

type assetScrubber struct {
	layout LayoutLookup
	// client is true for the wire rule (decide by entry type AND layout type,
	// write the decided type, refuse what it cannot classify) and false for
	// the store rule (the same conjunction against the stored layout; never
	// refuse).
	client bool
	// layouts holds each layout's answer for this one body or row.
	layouts map[string]layoutAnswer
}

type layoutAnswer struct {
	types map[string]string
	ok    bool
}

// lookupLayout asks the layout source once per layout id per body.
func (s *assetScrubber) lookupLayout(id string) (map[string]string, bool) {
	if id == "" || s.layout == nil {
		return nil, false
	}
	if a, seen := s.layouts[id]; seen {
		return a.types, a.ok
	}
	types, ok := s.layout(id)
	if s.layouts == nil {
		s.layouts = map[string]layoutAnswer{}
	}
	s.layouts[id] = layoutAnswer{types: types, ok: ok}
	return types, ok
}

// checkAssetEnvelope refuses the documented envelopes when their shape is
// wrong: an "assets" value that is not a list, an "asset" value that is not an
// object, and a list of assets holding a value that is not an object.
func checkAssetEnvelope(v any) error {
	checkList := func(items []any) error {
		for _, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return errors.New("its asset list holds a value that is not an object")
			}
		}
		return nil
	}
	switch t := v.(type) {
	case []any:
		return checkList(t)
	case map[string]any:
		if list, has := t["assets"]; has && list != nil {
			items, ok := list.([]any)
			if !ok {
				return errors.New(`its "assets" value is not a list`)
			}
			if err := checkList(items); err != nil {
				return err
			}
		}
		if single, has := t["asset"]; has && single != nil {
			if _, ok := single.(map[string]any); !ok {
				return errors.New(`its "asset" value is not an object`)
			}
		}
	}
	return nil
}

// scrubTree scrubs, as an asset, every object at any depth of v that carries
// a field-values key, and keeps walking below it, so no envelope or nesting
// hides a field value from the rule.
func (s *assetScrubber) scrubTree(v any) error {
	switch t := v.(type) {
	case map[string]any:
		if err := s.scrubAsset(t); err != nil {
			return err
		}
		for _, child := range t {
			if err := s.scrubTree(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range t {
			if err := s.scrubTree(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// dropSecretKeys removes every key named like a secret, at any depth.
func (s *assetScrubber) dropSecretKeys(v any) {
	switch t := v.(type) {
	case map[string]any:
		for key, child := range t {
			if SecretKeyName(key) {
				delete(t, key)
				continue
			}
			s.dropSecretKeys(child)
		}
	case []any:
		for _, child := range t {
			s.dropSecretKeys(child)
		}
	}
}

func (s *assetScrubber) scrubAsset(asset map[string]any) error {
	var (
		looked bool
		types  map[string]string
		ok     bool
	)
	layoutTypes := func() (map[string]string, bool) {
		if !looked {
			looked = true
			types, ok = s.lookupLayout(AssetLayoutID(asset))
		}
		return types, ok
	}
	for key, raw := range asset {
		if !isAssetFieldKey(key) || raw == nil {
			continue
		}
		replaced, err := s.scrubFieldValues(raw, layoutTypes)
		if err != nil {
			return err
		}
		asset[key] = replaced
	}
	return nil
}

// scrubFieldValues handles a fields value: a list of {label,value} entries,
// a list of label-to-value maps, or one label-to-value map.
func (s *assetScrubber) scrubFieldValues(raw any, layoutTypes func() (map[string]string, bool)) (any, error) {
	switch t := raw.(type) {
	case []any:
		for i, item := range t {
			switch entry := item.(type) {
			case map[string]any:
				if _, isEntry := entry["label"]; isEntry {
					s.scrubFieldEntry(entry, layoutTypes)
				} else {
					s.scrubLabelValueMap(entry, layoutTypes)
				}
			case nil:
			default:
				if s.client {
					return nil, errors.New("its fields list holds a value that is not an object")
				}
				if !valueIsEmpty(entry) {
					t[i] = WithheldFieldMarker
				}
			}
		}
		return t, nil
	case map[string]any:
		s.scrubLabelValueMap(t, layoutTypes)
		return t, nil
	default:
		if s.client {
			return nil, errors.New("its fields value is not a list")
		}
		if valueIsEmpty(t) {
			return t, nil
		}
		return WithheldFieldMarker, nil
	}
}

// scrubFieldEntry decides one {label,value} entry. A second spelling of
// label, value or field_type (`Value`, `field-type`) makes the entry secret,
// and every spelling of value is withheld with it, since a reader that matches
// keys loosely would take it for the value.
func (s *assetScrubber) scrubFieldEntry(entry map[string]any, layoutTypes func() (map[string]string, bool)) {
	label, _ := entry["label"].(string)
	entryType, hasEntryType := entry["field_type"].(string)
	loose := false
	for key := range entry {
		switch NormalizeLabel(key) {
		case "label", "value", "fieldtype":
			loose = loose || (key != "label" && key != "value" && key != "field_type")
		}
	}
	secret := loose
	if s.client {
		types, ok := layoutTypes()
		layoutType, hasLayoutType := "", false
		if ok && strings.TrimSpace(label) != "" {
			layoutType, hasLayoutType = types[NormalizeLabel(label)]
		}
		entrySecret := hasEntryType && FieldTypeIsSecret(entryType)
		layoutSecret := !hasLayoutType || FieldTypeIsSecret(layoutType)
		secret = secret || entrySecret || layoutSecret
		decided := layoutType
		switch {
		case hasLayoutType && FieldTypeIsSecret(layoutType):
			decided = layoutType
		case entrySecret:
			decided = entryType
		case secret:
			decided = UnknownFieldType
		}
		if !hasEntryType || entryType != decided {
			entry["field_type"] = decided
		}
	} else {
		// The same conjunction as the wire: an entry's own plain type never
		// overrides the stored layout, and a layout that is not stored, or
		// lacks the label, makes the value secret.
		secret = secret || (hasEntryType && FieldTypeIsSecret(entryType)) || labelIsSecret(label, layoutTypes)
	}
	if !secret {
		return
	}
	for key, value := range entry {
		if NormalizeLabel(key) == "value" && !valueIsEmpty(value) {
			entry[key] = WithheldFieldMarker
		}
	}
}

// scrubLabelValueMap decides each key of a label-to-value map by the layout,
// since the map form carries no per-field type.
func (s *assetScrubber) scrubLabelValueMap(m map[string]any, layoutTypes func() (map[string]string, bool)) {
	for label, value := range m {
		if labelIsSecret(label, layoutTypes) && !valueIsEmpty(value) {
			m[label] = WithheldFieldMarker
		}
	}
}

func labelIsSecret(label string, layoutTypes func() (map[string]string, bool)) bool {
	if strings.TrimSpace(label) == "" {
		return true
	}
	types, ok := layoutTypes()
	if !ok {
		return true
	}
	layoutType, found := types[NormalizeLabel(label)]
	return !found || FieldTypeIsSecret(layoutType)
}

// valueIsEmpty: null, "", [] and {} stay as they are, so an audit still sees
// an unfilled field as unfilled.
func valueIsEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case []any:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func decodeValue(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data after the JSON value")
	}
	return v, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// Marshal encodes without HTML escaping so kept values such as URLs keep
// their characters.
func Marshal(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, errors.New("the filtered response could not be encoded")
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}
