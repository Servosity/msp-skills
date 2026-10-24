// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression test for the large-body scan of the
// success-body funnel on the CLI path, pinned by skills/cipp/handfixes.json
// under "success-body-credential-funnel".

package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLargeBodyCLICleanPassesUnchanged: list-users against a clean 3 MiB
// /ListUsers answer prints every user, and the response cache holds the body
// byte for byte.
func TestLargeBodyCLICleanPassesUnchanged(t *testing.T) {
	var b strings.Builder
	b.WriteString("[")
	for i := 0; b.Len() < 3<<20; i++ {
		fmt.Fprintf(&b, `{"id":"%08d-1111-2222-3333-444455556666","displayName":"User %d","userPrincipalName":"user%d@t.example","mail":"user%d@t.example","accountEnabled":true,"jobTitle":"Engineer","department":"IT","businessPhones":["+1 555 0100"],"assignedLicenses":[{"skuId":"c7df2760-2c81-4ef7-b578-5b5392b571df","disabledPlans":[]}],"createdDateTime":"2024-01-01T00:00:00Z"},`, i, i, i, i)
	}
	b.WriteString(`{"id":"last","userPrincipalName":"last@t.example"}]`)
	body := b.String()
	startFakeCIPP(t, map[string][]string{"/ListUsers": {body}})
	out, err := runCLI(t, "list-users", "--tenant-filter", "t.example", "--json")
	if err != nil {
		t.Fatalf("a clean 3 MiB body was refused: %v", err)
	}
	if !strings.Contains(out, "last@t.example") || !strings.Contains(out, "user0@t.example") {
		t.Fatalf("list-users did not print every user (%d bytes of output)", len(out))
	}
	cached := false
	_ = filepath.WalkDir(os.Getenv("HOME"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if data, rerr := os.ReadFile(p); rerr == nil && bytes.Equal(data, []byte(body)) {
				cached = true
			}
		}
		return nil
	})
	if !cached {
		t.Fatal("the clean 3 MiB body was not cached unchanged under HOME")
	}
}
