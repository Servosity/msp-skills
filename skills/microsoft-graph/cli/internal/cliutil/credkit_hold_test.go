// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"errors"
	"path/filepath"
	"testing"

	"microsoft-graph-pp-cli/internal/credkit"
)

// credkit-wiring: every value a run-profile store saves under a credential
// flag is held, each member of a duplicate key included (json.Unmarshal keeps
// only the last), and an ordinary saved flag is not.
func TestHoldRunProfileStoreHoldsEveryDuplicate(t *testing.T) {
	store := `{"profiles":{"p":{"values":{` +
		`"client-secret":"` + "FirstDup" + `Secret-81","client-secret":"` + "SecondDup" + `Secret-92",` +
		`"api_token":"ProfTokenVal-73","base-url":"https://u:UrlPassw0rd-64@h.example.com",` +
		`"select":"OrdinarySelectVal-55"}}}}`
	HoldRunProfileStore([]byte(store))
	held := func(text string) bool {
		err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(text), 0o600)
		if err != nil && !errors.Is(err, credkit.ErrWithheld) {
			t.Fatalf("probe write: %v", err)
		}
		return errors.Is(err, credkit.ErrWithheld)
	}
	for _, v := range []string{"FirstDupSecret-81", "SecondDupSecret-92", "ProfTokenVal-73", "UrlPassw0rd-64"} {
		if !held(v) {
			t.Errorf("%q is not held", v)
		}
	}
	if held("OrdinarySelectVal-55") {
		t.Error("an ordinary saved flag value is held")
	}
}
