// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "encoding/json"

// esetMetadataOnlyPage reports whether a list page holds nothing but paging
// metadata, and returns its next-page token. Protobuf JSON can omit an empty
// repeated field, so ESET may answer an empty page as `{}` or
// `{"nextPageToken":"B"}` (handfixes.json: sync-metadata-only-pages).
// tokenOK is false when a token is present but is not a string (or null): the
// caller must not read that as "no more pages".
func esetMetadataOnlyPage(data json.RawMessage) (meta bool, token string, tokenOK bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil || obj == nil {
		return false, "", true
	}
	for k := range obj {
		switch k {
		case "nextPageToken", "totalSize", "pageToken", "totalCount":
		default:
			return false, "", true
		}
	}
	if raw, ok := obj["nextPageToken"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &token) != nil {
			return true, "", false
		}
	}
	return true, token, true
}
