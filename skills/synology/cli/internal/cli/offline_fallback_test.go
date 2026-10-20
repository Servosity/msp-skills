// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-transport-kind-kept).

package cli

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"synology-pp-cli/internal/dsm/dsmtest"
)

// TestAutoDataSourceFallsBackWhenTheNASIsUnreachable: sanitising transport
// errors (so no server-chosen text reaches the user) used to discard their
// type, so isNetworkError no longer recognised a refused connection and
// --data-source auto stopped falling back to the local store. The sanitised
// error keeps its kind on every path: a request over a cached session, a
// fresh login (the probe fails), and an error whose text was masked (the
// password happens to be a word of the message).
func TestAutoDataSourceFallsBackWhenTheNASIsUnreachable(t *testing.T) {
	for _, tc := range []struct{ name, password string }{
		{"plain", "pw-offline-1"},
		{"masked-text", "reach"}, // "cannot reach the NAS": the message is masked
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := sweepIsolate(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if dsmtest.ServeAPIInfo(w, r) {
					return
				}
				_ = r.ParseForm()
				switch r.Form.Get("api") + "/" + r.Form.Get("method") {
				case "SYNO.API.Auth/login":
					_, _ = w.Write([]byte(`{"success":true,"data":{"sid":"sid-offline","synotoken":"tok-offline"}}`))
				case "SYNO.Core.User/list":
					_, _ = w.Write([]byte(`{"success":true,"data":{"total":1,"offset":0,"users":[{"name":"alice","uid":1026}]}}`))
				default:
					_, _ = w.Write([]byte(`{"success":false,"error":{"code":102}}`))
				}
			}))
			t.Setenv("SYNOLOGY_BASE_URL", srv.URL)
			t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
			t.Setenv("SYNOLOGY_PASSWORD", tc.password)
			if out, errOut, code := sweepExecute(t, "sync", "--resources", "user", "--json"); code != 0 {
				t.Fatalf("sync: exit %d %s %s", code, out, errOut)
			}
			srv.Close() // the NAS is now unreachable (connection refused)

			// 1. Over the cached session: the request itself fails.
			out, errOut, code := sweepExecute(t, "user", "--json")
			if code != 0 || !strings.Contains(out, "alice") || !strings.Contains(out, `"local"`) {
				t.Fatalf("cached session: auto did not fall back to the local store: exit %d out=%s err=%s", code, out, errOut)
			}
			if tc.name != "plain" {
				return
			}
			// 2. No cached session: the login's probe fails.
			removed := 0
			_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.HasPrefix(filepath.Base(p), "session-") {
					if os.Remove(p) == nil {
						removed++
					}
				}
				return nil
			})
			if removed == 0 {
				t.Fatal("no cached session to remove; the login path was not exercised")
			}
			out, errOut, code = sweepExecute(t, "user", "--json")
			if code != 0 || !strings.Contains(out, "alice") || !strings.Contains(out, `"local"`) {
				t.Fatalf("fresh login: auto did not fall back to the local store: exit %d out=%s err=%s", code, out, errOut)
			}
			// The live-only answer still names the kind, never Go's text.
			out, errOut, code = sweepExecute(t, "user", "--json", "--data-source", "live")
			if code == 0 || !strings.Contains(out+errOut, "cannot reach the NAS") || strings.Contains(out+errOut, "connect: connection refused") {
				t.Fatalf("live: exit %d out=%s err=%s", code, out, errOut)
			}
		})
	}
}
