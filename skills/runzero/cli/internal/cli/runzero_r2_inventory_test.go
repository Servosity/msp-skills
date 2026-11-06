// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex round 2 of the runzero 4.34.2 reprint review (.review/codex-r2.md):
// the inventory classes as a user meets them, through the CLI's exit codes.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"runzero-pp-cli/internal/cliutil/testenv"
)

// Class historical-inventory-treated-as-current (Codex r2 [P1]): after the
// asset was deleted and `inventory sync --only assets` received [], `affected`
// printed the finding with the deleted asset's name and address and exited 0.
// It prints the finding by asset id alone and exits 6 naming it; certs-expiring
// (which joins assets too) does the same for the asset's certificate.
func TestR2ResultsNameRowsOfAssetsNotInTheSnapshot(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeRunzero(t)
	soon := strconv.FormatInt(time.Now().Add(10*24*time.Hour).Unix(), 10)
	fake.mu.Lock()
	fake.bodies["/api/v1.0/export/org/certificates.json"] = `[{"id":"cert-1","asset_id":"asset-1","subject":"CN=web01","issuer":"CN=R3","validity_end":` + soon + `}]`
	fake.mu.Unlock()
	db := filepath.Join(t.TempDir(), "data.db")
	mustRunRunzero(t, "inventory", "sync", "--db", db, "--json", "--no-learn")
	if _, _, err := runRunzero(t, "affected", "CVE-2024-6387", "--db", db, "--json", "--no-learn"); err != nil {
		t.Fatalf("affected over a consistent store: %v, want exit 0", err)
	}

	fake.mu.Lock()
	fake.bodies["/api/v1.0/export/org/assets.json"] = `[]`
	fake.mu.Unlock()
	mustRunRunzero(t, "inventory", "sync", "--only", "assets", "--db", db, "--json", "--no-learn")

	out, _, err := runRunzero(t, "affected", "CVE-2024-6387", "--db", db, "--json", "--no-learn")
	if exitCodeOf(err) != 6 || err == nil || !strings.Contains(err.Error(), "v-1") {
		t.Errorf("affected over a finding of a deleted asset: exit %d (%v), want 6 naming finding v-1", exitCodeOf(err), err)
	}
	if !strings.Contains(out, "asset-1") || strings.Contains(out, "web01") || strings.Contains(out, "10.0.0.5") {
		t.Errorf("affected printed the deleted asset's name or address as current (or dropped the finding):\n%s", out)
	}
	out, _, err = runRunzero(t, "certs-expiring", "--db", db, "--json", "--no-learn")
	if exitCodeOf(err) != 6 || err == nil || !strings.Contains(err.Error(), "cert-1") {
		t.Errorf("certs-expiring over a certificate of a deleted asset: exit %d (%v), want 6 naming cert-1", exitCodeOf(err), err)
	}
	if !strings.Contains(out, "cert-1") || strings.Contains(out, "10.0.0.5") {
		t.Errorf("certs-expiring printed the deleted asset's address as current (or dropped the certificate):\n%s", out)
	}
	if _, _, err := runRunzero(t, "affected", "CVE-2024-6387", "--db", db, "--json", "--no-learn", "--allow-partial-failure"); err != nil {
		t.Errorf("affected --allow-partial-failure: %v, want exit 0", err)
	}
}

// Class incomplete-fetch-read-as-complete (Codex r2 [P1]): Codex's input run
// as written. After a full sync, `inventory sync --only vulnerabilities` is
// killed (SIGKILL) while /export/org/vulnerabilities.json has not answered;
// `affected` then returned the previous findings with exit 0 and no warning.
// It prints them and exits 6 naming vulnerabilities as not finished, until a
// sync finishes.
func TestR2KilledSyncIsAPartialFailure(t *testing.T) {
	testenv.Isolate(t)
	newFakeRunzero(t)
	db := filepath.Join(t.TempDir(), "data.db")
	mustRunRunzero(t, "inventory", "sync", "--db", db, "--json", "--no-learn")
	if _, _, err := runRunzero(t, "affected", "CVE-2024-6387", "--db", db, "--json", "--no-learn"); err != nil {
		t.Fatalf("affected after a full sync: %v, want exit 0", err)
	}

	asked := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/export/org/vulnerabilities.json") {
			select {
			case asked <- struct{}{}:
			default:
			}
		}
		<-r.Context().Done() // never answers; ends when the client is gone
	}))
	t.Cleanup(slow.Close)
	child := exec.Command(os.Args[0], "-test.run=^TestR2HelperInventorySync$")
	child.Env = append(os.Environ(), "RZ_R2_HELPER_DB="+db, "RUNZERO_BASE_URL="+slow.URL+"/api/v1.0", "TMPDIR="+t.TempDir())
	var childOut bytes.Buffer
	child.Stdout, child.Stderr = &childOut, &childOut
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-asked:
	case <-time.After(90 * time.Second):
		_ = child.Process.Kill()
		_ = child.Wait()
		t.Fatalf("the sync never asked the vulnerabilities export:\n%s", childOut.String())
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()

	out, _, err := runRunzero(t, "affected", "CVE-2024-6387", "--db", db, "--json", "--no-learn")
	if exitCodeOf(err) != 6 || err == nil || !strings.Contains(err.Error(), "sync of vulnerabilities") || !strings.Contains(err.Error(), "did not finish") {
		t.Errorf("affected after a killed vulnerabilities sync: exit %d (%v), want 6 naming vulnerabilities as not finished", exitCodeOf(err), err)
	}
	if !strings.Contains(out, "CVE-2024-6387") {
		t.Errorf("affected withheld the stored findings:\n%s", out)
	}
	mustRunRunzero(t, "inventory", "sync", "--only", "vulnerabilities", "--db", db, "--json", "--no-learn")
	if _, _, err := runRunzero(t, "affected", "CVE-2024-6387", "--db", db, "--json", "--no-learn"); err != nil {
		t.Errorf("affected after a vulnerabilities sync that finished: %v, want exit 0", err)
	}
}

// TestR2HelperInventorySync is the process TestR2KilledSyncIsAPartialFailure
// kills: it runs `inventory sync --only vulnerabilities` and nothing else.
func TestR2HelperInventorySync(t *testing.T) {
	db := os.Getenv("RZ_R2_HELPER_DB")
	if db == "" {
		t.Skip("helper process for TestR2KilledSyncIsAPartialFailure")
	}
	_, _, _ = runRunzero(t, "inventory", "sync", "--only", "vulnerabilities", "--db", db, "--json", "--no-learn")
	os.Exit(0)
}
