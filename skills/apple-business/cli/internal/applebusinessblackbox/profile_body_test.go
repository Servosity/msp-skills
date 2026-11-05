// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package applebusinessblackbox

// A saved profile can feed a request body in two ways, and neither may get past the hold
// that runs on the bytes before any parser keeps only a duplicate key's last member.
// Seen failing on the code it fixes: .review/more/tests5-before.log.

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestProfileBodiesAreHeldBeforeAnyParse(t *testing.T) {
	bin := binaries(t)
	const dupe = `{"data":{"type":"blueprints","attributes":{"name":"Fleet","password":"` + dupeFixture + `","password":"","description":"` + dupeFixture + `"}}}`
	saved := func(t *testing.T, f *fake, home, key, value string) {
		t.Helper()
		if out, code := runCLI(t, bin, home, key, f, "profile", "save", "p", "--compact"); code != 0 {
			t.Fatalf("profile save: exit %d\n%s", code, out)
		}
		store := findFile(t, home, "profiles.json")
		if store == "" {
			t.Fatal("no profile store")
		}
		if err := os.WriteFile(store, []byte(`{"profiles":{"p":{"name":"p","values":{`+value+`}}}}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("a profile cannot turn on --stdin", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		saved(t, f, home, key, `"stdin":"true"`)
		out := runStdin(t, bin, home, key, f, dupe, nil, "--profile", "p", "blueprints", "create", "--dry-run", "--json", "--no-learn")
		if strings.Contains(out, dupeFixture) {
			t.Errorf("the preview prints the stdin body's password:\n%s", out)
		}
	})
	t.Run("a JSON value on a body flag from a profile", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		saved(t, f, home, key, `"data-relationships-apps":`+strconv.Quote(`{"password":"`+dupeFixture+`","password":"","note":"`+dupeFixture+`"}`))
		out := runStdin(t, bin, home, key, f, "", nil, "--profile", "p", "blueprints", "create", "--data-attributes-name", "Fleet", "--dry-run", "--json", "--no-learn")
		if strings.Contains(out, dupeFixture) {
			t.Errorf("the preview prints the flag value's password:\n%s", out)
		}
	})
	t.Run("control: --stdin on the command line still reads the body", func(t *testing.T) {
		f := newFake(t)
		home, key := t.TempDir(), keyFile(t, f)
		out := runStdin(t, bin, home, key, f, `{"data":{"type":"blueprints","attributes":{"name":"FleetName"}}}`, nil, "blueprints", "create", "--stdin", "--dry-run", "--json", "--no-learn")
		if !strings.Contains(out, "FleetName") {
			t.Errorf("the preview lost the body:\n%s", out)
		}
	})
}
