// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"riverside-fm-pp-cli/internal/client"
	"riverside-fm-pp-cli/internal/cliutil"
	"riverside-fm-pp-cli/internal/cliutil/testenv"
	"riverside-fm-pp-cli/internal/config"
)

func rotateThroughHookedClient(t *testing.T, on bool) bool {
	t.Helper()
	testenv.Isolate(t)
	if on {
		t.Setenv(config.NoConfigWriteEnv, "1")
	} else {
		t.Setenv(config.NoConfigWriteEnv, "")
	}
	c := client.New(&config.Config{BaseURL: "https://riverside.com"}, 10*time.Second, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://riverside.com/api/v4/projects")
	c.HTTPClient.Jar.SetCookies(u, []*http.Cookie{{Name: "riverside_auth_access", Value: "rot.rot.rot", Path: "/"}})
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	_, err = os.Stat(filepath.Join(dir, "cookies.json"))
	return err == nil
}

func TestNoConfigWriteHookKeepsJarOffDisk(t *testing.T) {
	if rotateThroughHookedClient(t, true) {
		t.Fatalf("%s=1: a rotated session cookie reached cookies.json", config.NoConfigWriteEnv)
	}
	if !rotateThroughHookedClient(t, false) {
		t.Fatal("control: without the switch the jar wrote nothing, so the ON case proves nothing")
	}
}
