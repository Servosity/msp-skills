// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sonicwall-pp-cli/internal/cliutil/testenv"
)

// TestNovelBlindSpotsHelpWires smoke-tests that the blind-spots command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelBlindSpotsHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"blind-spots", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("blind-spots --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "blind-spots"} {
		if !strings.Contains(help, want) {
			t.Fatalf("blind-spots --help missing %q in output:\n%s", want, help)
		}
	}
}

// Codex round 17: a registered NSA E-Class firewall ("NSA E7500": no digit
// after NSA) missing from NSM is a blind spot, and an unrecognised model is
// listed as unknown rather than dropped as "other"; only a known non-firewall
// family is dropped by default.
func TestBlindSpotsListsEClassAndUnrecognisedProducts(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SONICWALL_MSW_API_KEY", "test-key")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/users/productlist-customer":
			_, _ = io.WriteString(w, `{"operationStatus":"Success","content":{"totalProducts":"3","filterResultCount":"3","prodList":[`+
				`{"serNo":"0017C5000001","productNameRaw":"NSA E7500"},`+
				`{"serNo":"0017C5000002","productNameRaw":"Frobnicator 9000"},`+
				`{"serNo":"0017C5000003","productName":"SMA 500v"}]}}`)
		case "/devices/tenant":
			_, _ = io.WriteString(w, `[]`)
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	defer srv.Close()
	out, err := runCLI(t, srv, "blind-spots")
	if err != nil {
		t.Fatalf("blind-spots: %v\n%s", err, out)
	}
	var rows []blindSpotRow
	decodeResults(t, out, &rows)
	got := map[string]string{}
	for _, r := range rows {
		got[r.Serial] = r.ProductClass
	}
	want := map[string]string{"0017C5000001": "firewall", "0017C5000002": "unknown"}
	if len(got) != len(want) {
		t.Fatalf("rows = %v, want %v", got, want)
	}
	for serial, class := range want {
		if got[serial] != class {
			t.Errorf("serial %s product_class = %q, want %q", serial, got[serial], class)
		}
	}
	if strings.Contains(out, "Every registered firewall") {
		t.Fatalf("a hidden E-Class firewall must not read as every firewall visible:\n%s", out)
	}
	out, err = runCLI(t, srv, "blind-spots", "--all-products")
	if err != nil {
		t.Fatalf("blind-spots --all-products: %v\n%s", err, out)
	}
	rows = nil
	decodeResults(t, out, &rows)
	if len(rows) != 3 {
		t.Fatalf("--all-products rows = %d, want 3 (the SMA included)", len(rows))
	}
}
