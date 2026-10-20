// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "encoding/json"

// esetMetadataOnlyPage reports whether a list page holds nothing but paging
// metadata, and returns its next-page token. Protobuf JSON can omit an empty
// repeated field, so ESET may answer an empty page as `{}` or
// `{"nextPageToken":"B"}` (handfixes.json: sync-metadata-only-pages).
func esetMetadataOnlyPage(data json.RawMessage) (bool, string) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(data, &obj) != nil || obj == nil {
		return false, ""
	}
	for k := range obj {
		switch k {
		case "nextPageToken", "totalSize", "pageToken", "totalCount":
		default:
			return false, ""
		}
	}
	var token string
	if raw, ok := obj["nextPageToken"]; ok {
		_ = json.Unmarshal(raw, &token)
	}
	return true, token
}
