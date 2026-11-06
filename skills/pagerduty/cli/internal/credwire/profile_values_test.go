// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated). pagerduty handfix profile-values-held: the kit's
// startup discovery does not read the run profile store, so a value saved under
// a credential flag's name is held at startup by cli.HoldProfileStores. Without
// that, profile show prints it.

package credwire

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pagerduty-pp-cli/internal/cliutil/testenv"
)

func TestSavedProfileCredentialIsHeldAtStartup(t *testing.T) {
	const saved = "Zt4Hn8Qw2Pb6Jc9Vm3Xr"
	srv, ca, _ := kitServer(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	home := testenv.Isolate(t)
	// The store the CLI reads under PAGERDUTY_HOME; written by hand, as profile
	// save never takes a credential flag.
	dir := filepath.Join(home, "config")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"profiles":{"p":{"name":"p","values":{"oauth-client-client-secret":%q,"limit":"7"}}}}`, saved)
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := kitRun(t, srv, ca, home, "profile", "show", "p", "--json")
	if strings.Contains(out, saved) {
		t.Fatalf("profile show printed a saved credential:\n%s", out)
	}
	if !strings.Contains(out, "[credkit: withheld from") {
		t.Fatalf("profile show did not read the store, so nothing was tested:\n%s", out)
	}
	// The control: a store with no credential flag shows the rest.
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(`{"profiles":{"p":{"name":"p","values":{"limit":"7"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, _ := kitRun(t, srv, ca, home, "profile", "show", "p", "--json"); strings.Contains(out, "credkit: withheld") || !strings.Contains(out, `"limit"`) {
		t.Fatalf("a profile with no credential was not shown:\n%s", out)
	}
}
