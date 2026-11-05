// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: hubspot-explicit-clears-survive-merge): the
// HubSpot exception to the generated keep-richer merge. Lives beside the
// generated store.go so regen-merge preserves it whole; store.go carries only
// the one-line hook in mergeKeepRicherJSON that calls into it.

package store

// hubspotPropertiesKey is the top-level key of HubSpot's documented CRM object
// shape (SimplePublicObject: {"id", "properties": {name: value}, ...}).
const hubspotPropertiesKey = "properties"

// applyAuthoritativeProperties makes every property HubSpot explicitly returned
// in this response win over the stored value, including a clear.
//
// The generated merge keeps the stored value when the incoming one is "" or
// null so a thin list payload cannot erase richer detail a fuller read stored.
// That reasoning is about keys a response leaves out. A key that IS present in
// HubSpot's properties map is the current value: HubSpot clears a property by
// setting it to "", a property value is a nullable string in the documented
// object shape, and the sync --with-history fill copies the newest history
// entry (which may be "") into this map as a present key. Keeping the old value there leaves a deal
// owned after its owner was cleared, so `deals unowned` hides it.
//
// The rule is scoped to what the response carried: a property absent from the
// incoming map is kept (the keep-richer reason still holds for it), and every
// other key in the row still goes through the generated policy unchanged.
// Container values are left to the generated merge; HubSpot property values
// are strings or null.
//
// merged must be the value mergeKeepRicherValue built from fresh decodes, so
// writing into its properties map cannot alias a caller's data.
func applyAuthoritativeProperties(merged, incoming any) any {
	mergedObj, ok := merged.(map[string]any)
	if !ok {
		return merged
	}
	incomingObj, ok := incoming.(map[string]any)
	if !ok {
		return merged
	}
	incomingProps, ok := incomingObj[hubspotPropertiesKey].(map[string]any)
	if !ok {
		return merged
	}
	mergedProps, ok := mergedObj[hubspotPropertiesKey].(map[string]any)
	if !ok {
		return merged
	}
	for name, value := range incomingProps {
		if isJSONContainer(value) {
			continue
		}
		mergedProps[name] = value
	}
	return merged
}
