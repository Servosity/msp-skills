// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. fleet add on a registered name changes only what is passed.

package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
)

func registryEntry(t *testing.T, name string) (fleet.NAS, bool) {
	t.Helper()
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	r, err := fleet.LoadRegistry(filepath.Join(dir, "fleet.json"))
	if err != nil {
		t.Fatal(err)
	}
	return r.Get(name)
}

type addOut struct {
	NAS     fleet.NAS `json:"nas"`
	Updated bool      `json:"updated"`
	Changes []struct {
		Field string `json:"field"`
		From  any    `json:"from"`
		To    any    `json:"to"`
	} `json:"changes"`
	Checked bool `json:"checked_login"`
}

func runAdd(t *testing.T, args ...string) (addOut, string, int) {
	t.Helper()
	out, errText, code := runCLIErr(t, append([]string{"fleet", "add"}, append(args, "--json")...)...)
	var res addOut
	if code == 0 {
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("fleet add %v: %v\n%s", args, err, out)
		}
	}
	return res, out + errText, code
}

// TestFleetAddReAddKeepsEveryUnpassedField: re-running fleet add on a
// registered name used to replace the entry whole, dropping --client, a
// custom --env-prefix (so a later sync sent another env var's password to
// that URL) and the certificate pin, with exit 0 and no note; and
// `fleet add <name> --check` on a registered NAS failed asking for --url.
// A re-add now keeps every field the caller did not pass and lists each
// field it changed; a new URL for a pinned NAS is refused; --check works
// on the registered entry.
func TestFleetAddReAddKeepsEveryUnpassedField(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	pin := strings.Repeat("ab", 32)
	const pinnedURL = "https://192.0.2.10:5001"
	if _, text, code := runAdd(t, "lab", "--url", pinnedURL, "--client", "Lab Co", "--env-prefix", "LAB_NAS", "--tls-fingerprint", pin); code != 0 {
		t.Fatalf("register: %d %s", code, text)
	}
	want := fleet.NAS{Name: "lab", URL: pinnedURL, Client: "Lab Co", EnvPrefix: "LAB_NAS", TLSFingerprint: pin}

	// Same URL, nothing else passed: nothing changes, and the output says so.
	res, text, code := runAdd(t, "lab", "--url", pinnedURL)
	if got, _ := registryEntry(t, "lab"); code != 0 || got != want || !res.Updated || len(res.Changes) != 0 {
		t.Fatalf("re-add with the same URL: exit %d, registry %+v, result %+v\n%s", code, got, res, text)
	}
	// No --url at all: the registered address is kept; only --client changes.
	res, text, code = runAdd(t, "lab", "--client", "Lab Company")
	want.Client = "Lab Company"
	if got, _ := registryEntry(t, "lab"); code != 0 || got != want || len(res.Changes) != 1 || res.Changes[0].Field != "client" ||
		res.Changes[0].From != "Lab Co" || res.Changes[0].To != "Lab Company" {
		t.Fatalf("re-add with --client only: exit %d, registry %+v, changes %+v\n%s", code, got, res.Changes, text)
	}
	// A new URL for a pinned NAS is refused and changes nothing.
	if _, text, code = runAdd(t, "lab", "--url", "https://192.0.2.99:5001"); code != 2 || !strings.Contains(text, "pinned certificate") {
		t.Fatalf("new URL for a pinned NAS: exit %d\n%s", code, text)
	}
	if got, _ := registryEntry(t, "lab"); got != want {
		t.Fatalf("a refused re-add changed the registry: %+v", got)
	}

	// An unpinned NAS may move; its custom prefix stays, also after the NAS
	// that owned the default prefix is removed (the prefix used to reset to
	// SYNOLOGY_LAB2 and send that NAS's password to this address).
	if _, text, code = runAdd(t, "other", "--url", "http://192.0.2.30:5000", "--env-prefix", "SYNOLOGY_LAB2"); code != 0 {
		t.Fatalf("register other: %d %s", code, text)
	}
	if _, text, code = runAdd(t, "lab2", "--url", "http://192.0.2.20:5000", "--client", "Lab Two", "--env-prefix", "LAB2_NAS", "--insecure"); code != 0 {
		t.Fatalf("register lab2: %d %s", code, text)
	}
	if _, text, code = runCLIErr(t, "fleet", "remove", "other"); code != 0 {
		t.Fatalf("remove other: %d %s", code, text)
	}
	res, text, code = runAdd(t, "lab2", "--url", "http://192.0.2.21:5000")
	got, _ := registryEntry(t, "lab2")
	if code != 0 || got.EnvPrefix != "LAB2_NAS" || got.Client != "Lab Two" || !got.Insecure || got.URL != "http://192.0.2.21:5000" {
		t.Fatalf("moving an unpinned NAS dropped a field: exit %d, registry %+v\n%s", code, got, text)
	}
	if len(res.Changes) != 1 || res.Changes[0].Field != "url" || res.Changes[0].To != "http://192.0.2.21:5000" {
		t.Fatalf("changes = %+v, want only the url", res.Changes)
	}
}

// TestFleetAddCheckOnARegisteredNAS: `fleet add <name> --check` checks the
// registered NAS without --url (the README's way to run the first
// two-factor login), keeping its client and prefix.
func TestFleetAddCheckOnARegisteredNAS(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	srv := fakeABBNAS(t)
	t.Setenv("ACME_NAS_ACCOUNT", "msp-admin")
	t.Setenv("ACME_NAS_PASSWORD", "s3cret")
	if _, text, code := runAdd(t, "acme", "--url", srv.URL, "--client", "Acme", "--env-prefix", "ACME_NAS"); code != 0 {
		t.Fatalf("register: %d %s", code, text)
	}
	res, text, code := runAdd(t, "acme", "--check")
	if code != 0 || !res.Checked || res.NAS.Client != "Acme" || res.NAS.EnvPrefix != "ACME_NAS" || len(res.Changes) != 0 {
		t.Fatalf("fleet add acme --check on a registered NAS: exit %d %+v\n%s", code, res, text)
	}
	if _, text, code = runAdd(t, "ghost", "--check"); code != 2 || !strings.Contains(text, "is not registered") {
		t.Fatalf("an unregistered name without --url: exit %d\n%s", code, text)
	}
	// Human output names each changed field.
	defer func(old bool) { humanFriendly = old }(humanFriendly)
	out, errText, code := runCLIErr(t, "fleet", "add", "acme", "--client", "Acme Corp", "--human-friendly")
	if code != 0 || !strings.Contains(out, "changed client: Acme -> Acme Corp") || !strings.Contains(out, "updated acme") {
		t.Fatalf("human output: exit %d\n%s%s", code, out, errText)
	}
}
