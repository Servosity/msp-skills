// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package runzeroblackbox

// Codex round 1 of the runzero 4.34.2 reprint review, on the built binaries:
// the two credential classes it found, end to end (live output, the local
// store after sync and inventory sync).

import (
	"database/sql"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// Class credential-value-exempt-by-shape: a credential runZero returns under
// a credential name prints nowhere whatever it reads as (Codex's input:
// `account get-key` answering a URL-shaped token). Class
// documented-credential-metadata-redacted: an asset's documented
// `credentials` metadata (credential UUID -> protocol -> matched) prints
// live and from the local store.
func TestR1RunzeroCredentialClassesOnTheBuiltBinary(t *testing.T) {
	t.Parallel() // its own fake runZero and home: runs beside the other long batteries
	const key = "CTr1ReviewKey-5e8a2c71d9f3b04"
	const keyID = "e77602e0-3fb8-4734-aef9-fbc6fdcb0fa8" // gitleaks:allow synthetic test fixture
	const urlToken = "https://vault.example/private-key-material-r1"
	const ipSecret = "10.20.30.41"
	const credID = "5f0d2c61-0d8e-4e64-9a3b-2f4c8f1d7a10"
	const meta = `"credentials":{"` + credID + `":{"snmp":true,"ssh":false}}`
	asset := `{"id":"asset-r1","names":["hq-r1"],"addresses":["10.9.8.7"],` + meta + `}`
	apiKey := fmt.Sprintf(`{"id":%q,"token":%q,"comments":"r1","secret":%q}`, keyID, urlToken, ipSecret)
	bodies := map[string]string{
		apiBase + "/account/keys/" + keyID:  apiKey,
		apiBase + "/account/keys":           `[` + apiKey + `]`,
		apiBase + "/org/assets/asset-r1":    asset,
		apiBase + "/export/org/assets.json": `[` + asset + `]`,
	}
	// TLS: the credential kit sends no held value over plain http.
	origin, ca := tlsFake(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if b, ok := bodies[strings.TrimSuffix(r.URL.Path, "/")]; ok {
			fmt.Fprint(w, b)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	h := t.TempDir()
	env := cleanEnv(h, "RUNZERO_BASE_URL="+origin+apiBase, "RUNZERO_CA_CERT="+ca, "RUNZERO_API_KEY="+key)
	db := filepath.Join(h, "rz.db")

	var leaked strings.Builder
	leaked.WriteString(runWith(t, env, "account", "get-key", keyID, "--json"))
	leaked.WriteString(runWith(t, env, "account", "get-key", keyID))
	leaked.WriteString(runWith(t, env, "account", "get-keys", "--json"))
	leaked.WriteString(runWith(t, env, "sync", "--db", db, "--json"))
	for _, v := range []string{urlToken, "vault.example/private-key-material-r1", ipSecret, key} {
		if strings.Contains(leaked.String(), v) {
			t.Errorf("printed %q:\n%s", v, leaked.String())
		}
	}
	if s, err := sql.Open("sqlite", db); err == nil {
		var n int
		_ = s.QueryRow(`SELECT COUNT(*) FROM resources WHERE data LIKE '%vault.example%' OR data LIKE '%10.20.30.41%'`).Scan(&n)
		_ = s.Close()
		if n != 0 {
			t.Errorf("the local store kept the URL- or IP-shaped credential in %d record(s)", n)
		}
	}

	live := runWith(t, env, "org", "get-asset", "asset-r1", "--json")
	runWith(t, env, "inventory", "sync", "--db", db, "--json")
	local := runWith(t, env, "inventory", "list", "assets", "--db", db, "--json")
	for name, out := range map[string]string{"live org get-asset": live, "inventory list assets": local} {
		if !strings.Contains(out, credID) || !strings.Contains(out, `"snmp": true`) && !strings.Contains(out, `"snmp":true`) {
			t.Errorf("%s does not print the asset's credentials metadata:\n%s", name, out)
		}
	}
}
