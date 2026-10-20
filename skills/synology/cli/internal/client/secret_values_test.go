// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-mask-secret-values).

package client

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/config"
)

// TestCachedAnswersAreMaskedToo: an answer served from the response cache
// goes through the same secret-value mask as a fresh one, so a cache file
// written before a value was known to be secret never prints it.
func TestCachedAnswersAreMaskedToo(t *testing.T) {
	testenv.Isolate(t)
	const pw = "PW-LEAK-cache-4411"
	srv, _, _ := dsmFake(t)
	c := New(&config.Config{BaseURL: srv.URL, SynologyAccount: "admin", SynologyPassword: pw}, 5*time.Second, 0)
	path := "/webapi/entry.cgi?api=SYNO.Core.Package&method=list&version=2"
	c.writeCache(path, nil, json.RawMessage(`{"packages":[{"id":"ActiveBackup","note":"`+pw+`"}]}`))
	data, err := c.Get(context.Background(), path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), pw) || !strings.Contains(string(data), "ActiveBackup") {
		t.Fatalf("cached answer: %s", data)
	}
}
