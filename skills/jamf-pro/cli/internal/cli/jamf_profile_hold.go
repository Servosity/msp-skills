// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): handfixes.json credkit-wiring.

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"

	"jamf-pro-pp-cli/internal/cliutil"
	"jamf-pro-pp-cli/internal/credkit"
)

// runProfileAllowedFlags is the explicit allow-list of flags a run profile may
// save or apply: output shaping and read filters. Every other flag, every
// approval and every credential stays out of the store.
var runProfileAllowedFlags = map[string]bool{
	"json": true, "compact": true, "csv": true, "plain": true, "quiet": true, "select": true,
	"no-color": true, "human-friendly": true, "agent": true,
	"data-source": true, "max-age": true, "timeout": true,
	"limit": true, "page": true, "page-size": true, "sort": true, "filter": true, "section": true,
	"since": true, "days": true, "group": true, "tag": true, "only": true,
}

// HoldProfileStores holds what the run profile stores under the current home
// carry as a credential, for the kit's startup discovery. A profile is free text
// the CLI prints back. Its fields are held by name and by the URLs in them, not
// through Declarations.Files: the kit answers nested JSON by holding every string,
// so a profile named "p" or a saved --limit 7 would refuse every later start.
// runProfileAllowedFlags still decides which values a run applies.
func HoldProfileStores() {
	var paths []string
	if dir, err := cliutil.ConfigDir(); err == nil { // not profileStorePath, which creates the directory
		paths = append(paths, filepath.Join(dir, "profiles.json"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".jamf-pro-cli", "profiles.json"))
	}
	for _, path := range paths {
		data := cliutil.ReadRegular(path)
		var tree any
		if json.Unmarshal(data, &tree) == nil {
			holdProfileValue("", tree)
		} else if len(data) > 0 {
			credkit.HoldBody(data) // a store that does not parse: the kit reads every token of it
		}
	}
}

func holdProfileValue(key string, v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			holdProfileValue(k, e)
		}
	case []any:
		for _, e := range x {
			holdProfileValue(key, e)
		}
	case string:
		if credentialLikeRunProfileFlag(key) || cliutil.SecretField(key) {
			cliutil.Hold(cliutil.FieldSource("config:"+key), x)
		}
		cliutil.HoldURLField(x)
	}
}
