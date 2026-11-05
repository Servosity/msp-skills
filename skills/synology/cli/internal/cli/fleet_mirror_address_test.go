// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. The fleet mirror never records a NAS address with a password.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/fleet"
	"synology-pp-cli/internal/store"
)

// secretInDataDir names every file under the data directory holding secret.
func secretInDataDir(t *testing.T, secret string) []string {
	t.Helper()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	var hits []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && bytes.Contains(b, []byte(secret)) {
				hits = append(hits, p)
			}
		}
		return nil
	})
	return hits
}

func mirroredURLs(t *testing.T) map[string]string {
	t.Helper()
	p, err := fleet.DBPath()
	if err != nil {
		t.Fatal(err)
	}
	s, err := store.OpenWithContext(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.DB().Query(`SELECT nas, COALESCE(url,'') FROM fleet_sync`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var n, u string
		if err := rows.Scan(&n, &u); err != nil {
			t.Fatal(err)
		}
		out[n] = u
	}
	return out
}

// TestFleetMirrorNeverStoresAddressCredentials is the review's ui2 repro: a
// hand-edited fleet.json URL with user info (http://admin:<password>@nas)
// was refused, but fleet sync copied the raw URL, password included, into
// fleet.db's fleet_sync.url, where it outlived both fixing fleet.json and
// fleet remove. The mirror records MirrorAddress (no user info, query or
// fragment) everywhere; nothing under the data directory ever holds the
// password.
func TestFleetMirrorNeverStoresAddressCredentials(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	srv := fakeABBNAS(t)
	secret := fmt.Sprintf("UI-PASS-%x", time.Now().UnixNano()) // a per-run sentinel, never a literal
	t.Setenv("SYNOLOGY_ACME_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_ACME_PASSWORD", "s3cret")
	regPath, err := fleet.RegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	withPassword := strings.Replace(srv.URL, "://", "://admin:"+secret+"@", 1) + "/?k=" + secret + "#" + secret
	writeRegistry := func(u string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(regPath), 0o700); err != nil {
			t.Fatal(err)
		}
		reg := `{"nas":[{"name":"acme","url":"` + u + `","env_prefix":"SYNOLOGY_ACME"}]}`
		if err := os.WriteFile(regPath, []byte(reg), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	writeRegistry(withPassword)
	if out, errText, code := runCLIErr(t, "fleet", "sync", "--json"); code == 0 || strings.Contains(out+errText, secret) {
		t.Fatalf("fleet sync of a URL with a password: exit %d (want a refusal that never shows it)\n%s%s", code, out, errText)
	}
	if got := mirroredURLs(t)["acme"]; got != dsm.DisplayAddress(withPassword) || strings.Contains(got, secret) {
		t.Fatalf("fleet_sync.url = %q, want %q", got, dsm.DisplayAddress(withPassword))
	}
	if hits := secretInDataDir(t, secret); len(hits) != 0 {
		t.Fatalf("the password from fleet.json was copied into %v", hits)
	}

	// Fixed registry: the same NAS syncs, and the record is the same address.
	writeRegistry(srv.URL)
	if out, errText, code := runCLIErr(t, "fleet", "sync", "--json"); code != 0 {
		t.Fatalf("fleet sync after fixing fleet.json: exit %d\n%s%s", code, out, errText)
	}
	if got := mirroredURLs(t)["acme"]; got != srv.URL {
		t.Fatalf("fleet_sync.url = %q, want %q", got, srv.URL)
	}
	if out, _, _ := runCLIErr(t, "fleet", "status", "--json"); strings.Contains(out, "registered URL changed") || strings.Contains(out, "never synced") {
		t.Fatalf("a clean sync of the same address reads as another URL:\n%s", out)
	}

	// Broken again, synced, removed: the password never reaches the data dir.
	writeRegistry(withPassword)
	_, _, _ = runCLIErr(t, "fleet", "sync", "--json")
	if out, errText, code := runCLIErr(t, "fleet", "remove", "acme", "--json"); code != 0 {
		t.Fatalf("fleet remove: exit %d\n%s%s", code, out, errText)
	}
	if hits := secretInDataDir(t, secret); len(hits) != 0 {
		t.Fatalf("the password from fleet.json was copied into %v", hits)
	}
}
