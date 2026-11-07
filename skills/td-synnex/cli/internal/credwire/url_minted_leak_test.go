// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire_test

// Two credential classes Codex round 1 found on the built binaries
// (handfixes.json credkit-wiring), each test seen failing on the binaries
// built before the fix:
//   - a URL-valued source read at startup (a saved run profile's base-url or
//     another URL-valued profile value, a config or store base_url, the base URL
//     and URL environment variables) whose password reached output;
//   - a credential Spacefast mints (an anonymous page's key) printed before it
//     was held.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestSavedURLCredentialHeldAtStartup (class "saved-profile URL credential not
// held"): the password inside every URL-valued value the program reads at
// startup is held before the first byte of output, every duplicate included.
func TestSavedURLCredentialHeldAtStartup(t *testing.T) {
	cliBin, _ := binaries(t)
	profiles := func(home, body string) {
		writeFile(t, filepath.Join(home, "config", "profiles.json"), body)
	}

	t.Run("profile base-url (Codex r1 input)", func(t *testing.T) {
		const pw = "rK7nQ2" + "vL9xP4"
		home := newHome(t)
		profiles(home, `{"profiles":{"p":{"name":"p","values":{"base-url":"https://ops:`+pw+`@ion.tdsynnex.com"}}}}`)
		env := closedEnv(home)
		assertAbsent(t, "profile show p --json", run(t, cliBin, env, "profile", "show", "p", "--json"), pw)
		assertAbsent(t, "profile show p", run(t, cliBin, env, "profile", "show", "p"), pw)
		assertAbsent(t, "profile use p", run(t, cliBin, env, "profile", "use", "p"), pw)
	})

	t.Run("every duplicate member, struct keys in any case", func(t *testing.T) {
		// encoding/json keeps the last base-url and matches "Profiles" and
		// "VALUES" to the struct fields; the first member is held too. Profile
		// q carries it under a name that is no URL, so showing q proves it.
		const first = "dupFst" + "Aq7wE3rT"
		const last = "dupLst" + "Bz8xC4vB"
		home := newHome(t)
		profiles(home, `{"Profiles":{"p":{"name":"p","VALUES":{"base-url":"https://ops:`+first+`@ion.tdsynnex.com","base-url":"https://ops:`+last+`@ion.tdsynnex.com"}},`+
			`"q":{"name":"q","values":{"select":"`+first+`"}}}}`)
		env := closedEnv(home)
		assertAbsent(t, "profile show p --json", run(t, cliBin, env, "profile", "show", "p", "--json"), first, last)
		assertAbsent(t, "profile show q --json", run(t, cliBin, env, "profile", "show", "q", "--json"), first, last)
	})

	t.Run("another URL-valued profile value", func(t *testing.T) {
		const pw = "expUrl" + "Hn52vR8q"
		home := newHome(t)
		profiles(home, `{"profiles":{"p":{"name":"p","values":{"expected-base-url":"https://ops:`+pw+`@ion.tdsynnex.com/api/v3"}}}}`)
		assertAbsent(t, "profile show p --json", run(t, cliBin, closedEnv(home), "profile", "show", "p", "--json"), pw)
	})

	t.Run("base URL written without a scheme", func(t *testing.T) {
		// The kit's base URL reader holds a well-formed URL's password; the
		// program prints one written as user:password@host back in doctor.
		const envPw = "envNsc" + "Sc61pL4k"
		const cfgPw = "cfgNsc" + "Jt83mW5n"
		home := newHome(t)
		out := run(t, cliBin, closedEnv(home, "TD_SYNNEX_BASE_URL=ops:"+envPw+"@ion.tdsynnex.com/api/v3"), "doctor")
		assertAbsent(t, "doctor (TD_SYNNEX_BASE_URL)", out, envPw)
		home = newHome(t)
		writeFile(t, filepath.Join(home, "config", "config.toml"), "base_url = \"ops:"+cfgPw+"@ion.tdsynnex.com/api/v3\"\n")
		assertAbsent(t, "doctor (config base_url)", run(t, cliBin, closedEnv(home), "doctor"), cfgPw)
	})
}

// TestSpacefastPageKeyHeldWhenMinted (class "minted credential printed before
// it is held"): the key Spacefast returns for a new anonymous page is held the
// moment the receipt is parsed, so neither link that carries it is printed; the
// notice names the 0600 record that keeps both.
func TestSpacefastPageKeyHeldWhenMinted(t *testing.T) {
	cliBin, _ := binaries(t)
	const key = "sfc_" + "7Qp9Lm2V8n4Z"
	var hits atomic.Int64
	// Spacefast over plain http on loopback: deliver_spacefast.go allows it for
	// a local server, and the anonymous publish carries no credential.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"space":{"id":"spc_demo"},"claim":{"key":"` + key + `","url":"https://demo-page.space.fast/__/` + key + `","claimUrl":"https://spacefast.com/claim#` + key + `"}}}`))
	}))
	t.Cleanup(srv.Close)
	home := newHome(t)
	out := run(t, cliBin, closedEnv(home, "SPACEFAST_API_URL="+srv.URL), "version", "--deliver", "spacefast:demo-page")
	assertAbsent(t, "version --deliver spacefast:demo-page", out, key, key[4:])
	if hits.Load() == 0 {
		t.Fatalf("no publish reached the Spacefast stub:\n%s", out)
	}
	if !strings.Contains(out, "td-synnex-cli") || !strings.Contains(out, "Published on Spacefast") {
		t.Errorf("the version line or the publish notice is missing:\n%s", out)
	}
	records, _ := filepath.Glob(filepath.Join(home, "state", "spacefast-deliveries", "*.json"))
	if len(records) != 1 {
		t.Fatalf("want one remembered page, got %v", records)
	}
	data, err := os.ReadFile(records[0])
	if err != nil || !strings.Contains(string(data), key) {
		t.Errorf("the record %s does not keep the page key (%v)", records[0], err)
	}
	if !strings.Contains(out, records[0]) {
		t.Errorf("the notice does not say where the links are kept (%s):\n%s", records[0], out)
	}
}
