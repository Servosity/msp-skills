// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: cart-items --quantity is read in decimal only.

package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"td-synnex-pp-cli/internal/cliutil/testenv"
)

func TestCartItemsQuantityIsDecimal(t *testing.T) {
	for _, cmdName := range []string{"create", "update"} {
		testenv.Isolate(t)
		var body string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}))
		t.Setenv("TD_SYNNEX_BASE_URL", srv.URL)
		t.Setenv("TD_SYNNEX_ACCESS_TOKEN", "test-token")
		args := []string{"cart-items", cmdName, "108536", "240314"}
		if cmdName == "update" {
			args = append(args, "item1")
		}
		args = append(args, "--plan-name", "accounts/1/products/P/versions/1/plans/X", "--quantity", "012", "--yes", "--json")
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		srv.Close()
		if err != nil || !strings.Contains(body, `"quantity":12`) {
			t.Fatalf("cart-items %s --quantity 012 must send 12: err=%v body=%s out=%s", cmdName, err, body, out.String())
		}
	}
	testenv.Isolate(t)
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"cart-items", "create", "108536", "240314", "--quantity", "0x0C", "--dry-run"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("--quantity 0x0C must be refused")
	}
}
