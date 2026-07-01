package store

import "encoding/json"

// Maxio Advanced Billing (Chargify) wraps each list item in a singular
// envelope object: GET /customers.json returns [{"customer": {...}}, ...],
// GET /subscriptions.json returns [{"subscription": {...}}, ...], invoices
// wrap as {"invoice": {...}} (id field is "uid"), and so on. The generic
// store extracts the primary key from the top level of each item, which for an
// envelope is just the single wrapper key — so every row fails id extraction
// and the local store stays empty. unwrapSingularEnvelope detects that shape
// and returns the inner record so both id extraction and the stored `data`
// column see the flat object.
//
// It is deliberately generic (no hardcoded "customer"/"subscription"): it only
// unwraps a single-key object whose sole value is itself an object carrying an
// id-like field (per genericIDFieldFallbacks, which includes id/uid/uuid/...).
// That guard keeps it from mangling a legitimate single-field record whose lone
// value happens to be an object.
func unwrapSingularEnvelope(obj map[string]any, item json.RawMessage) (map[string]any, json.RawMessage, bool) {
	if len(obj) != 1 {
		return nil, nil, false
	}
	for _, v := range obj {
		inner, ok := v.(map[string]any)
		if !ok {
			return nil, nil, false
		}
		if !innerHasIDLike(inner) {
			return nil, nil, false
		}
		raw, err := json.Marshal(inner)
		if err != nil {
			return nil, nil, false
		}
		return inner, raw, true
	}
	return nil, nil, false
}

// innerHasIDLike reports whether obj carries any of the generic id-like fields
// the store uses for primary-key extraction. Shares genericIDFieldFallbacks and
// lookupFieldValue with ExtractResourceID so the unwrap decision and the
// subsequent id extraction never disagree.
func innerHasIDLike(obj map[string]any) bool {
	for _, key := range genericIDFieldFallbacks {
		if v := lookupFieldValue(obj, key); v != nil {
			if s := ResourceIDString(v); s != "" && s != "<nil>" {
				return true
			}
		}
	}
	return false
}
