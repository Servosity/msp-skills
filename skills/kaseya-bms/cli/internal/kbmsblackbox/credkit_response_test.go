// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

import (
	"bytes"
	"compress/zlib"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// response-credentials-held, through the credential kit (handfixes.json
// credkit-wiring, ResponseFields in internal/credwire): a credential a Kaseya
// BMS response carries under its own name is withheld from the output, at any
// depth and deflate-encoded, and the data beside it is not. The responses are
// the vendor's: an RMM integration setting (Settings.PasswordOrToken, the
// RemoteControl client pair; its Password is a mask) and a webhook
// configuration with its signing Secret. The kit withholds the output unit
// that carries one whole, so each has a control: the same record without its
// credentials prints in full (the data is not held). A masked Password
// ("**********") is held by the kit like any value under a declared name,
// so the control carries none (reported as a kit finding, not patched here).
func TestResponseCredentialFieldsWithheld(t *testing.T) {
	const (
		pwOrToken = "respNestedTokenUnit-A1b2c3d4"
		rcClient  = "respRcClientUnit-5d7f9e1b"
		rcSecret  = "respRcSecretUnit-9a1c3e5f"
		hookSec   = "respWebhookSecretUnit-2c9d4f6a"
		dataName  = "getNameNotSecretK2m4"
		hookName  = "TicketHookName-7k3m"
	)
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/rmm/Clean/status":
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{"Id":4,"Name":%q,"UseAccessToken":true,"Settings":{"Port":443}}}`, dataName)
		case strings.HasPrefix(r.URL.Path, "/v2/rmm/") && strings.HasSuffix(r.URL.Path, "/status"):
			var z bytes.Buffer
			zw := zlib.NewWriter(&z)
			fmt.Fprintf(zw, `{"Success":true,"Error":null,"Result":{"Id":3,"Name":%q,"UseAccessToken":true,`+
				`"Settings":{"PasswordOrToken":%q,"Password":"**********"},"RemoteControl":{"ClientId":%q,"ClientSecret":%q}}}`,
				dataName, pwOrToken, rcClient, rcSecret)
			_ = zw.Close()
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(z.Bytes())
		case r.URL.Path == "/v2/admin/webhooks/6":
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{"Id":6,"Name":%q,"HasSecret":false}}`, hookName)
		case strings.HasPrefix(r.URL.Path, "/v2/admin/webhooks/"):
			fmt.Fprintf(w, `{"Success":true,"Error":null,"Result":{"Id":5,"Name":%q,"HasSecret":true,"Secret":%q}}`, hookName, hookSec)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newEnv(t, &server{url: srv.URL, caFile: ca, headers: map[string]int{}}, credStored)
	for _, c := range []struct {
		args, control []string
		data, secrets []string
	}{
		{[]string{"rmm", "status", "get-rmmintegration", "Datto", "--json"}, []string{"rmm", "status", "get-rmmintegration", "Clean", "--json"}, []string{dataName}, []string{pwOrToken, rcClient, rcSecret}},
		{[]string{"admin", "get-webhook-configuration", "5", "--json"}, []string{"admin", "get-webhook-configuration", "6", "--json"}, []string{hookName}, []string{hookSec}},
	} {
		out := e.run(t, c.args...)
		for _, s := range c.secrets {
			if strings.Contains(out, s) {
				t.Errorf("%v prints the response credential %s:\n%.1500s", c.args, s, out)
			}
		}
		if !strings.Contains(out, "credkit: withheld") {
			t.Errorf("%v: the kit's notice is missing, so the test did not reach the credential:\n%.1500s", c.args, out)
		}
		ctl := e.run(t, c.control...)
		for _, d := range c.data {
			if !strings.Contains(ctl, d) {
				t.Errorf("%v withholds %s, which is data, not a credential:\n%.1500s", c.control, d, ctl)
			}
		}
	}
}
