// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #270: on press 4.35.1 the sliding-session rotation (every Riverside
// response sets a fresh riverside_auth_access) lands on disk through the
// persistent cookie jar. Under RIVERSIDE_FM_NO_CONFIG_WRITE the CLI and MCP
// clients swap in MemoryOnlyJar (internal/cli/riverside_no_config_write.go):
// the rotated cookie must stay usable in memory while cookies.json stays absent;
// the persistent jar, the control, writes it.

const ncwRotatedAccess = "rot.rot.rot"

func TestNoConfigWriteKeepsTheRotatedCookieInMemoryOnly(t *testing.T) {
	u, _ := url.Parse("https://riverside.com/api/v4/projects")
	rotated := []*http.Cookie{{Name: "riverside_auth_access", Value: ncwRotatedAccess, Path: "/"}}
	inner, _ := cookiejar.New(nil)
	path := filepath.Join(t.TempDir(), "cookies.json")
	persistent := &cookieJar{inner: inner, path: path}

	mem := MemoryOnlyJar(persistent)
	mem.SetCookies(u, rotated)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the memory-only jar wrote %s (stat err %v)", path, err)
	}
	if got := mem.Cookies(u); len(got) != 1 || got[0].Value != ncwRotatedAccess {
		t.Fatalf("rotated cookie not kept in memory: %v", got)
	}

	persistent.SetCookies(u, rotated)
	body, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(body), ncwRotatedAccess) {
		t.Fatalf("control: the persistent jar did not write the rotation, so the memory-only case proves nothing (err %v, body %q)", err, body)
	}
}
