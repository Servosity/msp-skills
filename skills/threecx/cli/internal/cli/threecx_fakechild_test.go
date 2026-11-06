// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json fleet-tests-portable-child): the per-PBX child the
// fleet tests run, so they need no shell and run on Windows too.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fakeChildEnv turns the test binary into the per-PBX child (TestMain). Its
// name has no THREECX_ prefix, so fleet.Scrub passes it to the child.
const fakeChildEnv = "FAKE_3CX_FLEET_CHILD"

// fakeChildMain stands in for the per-PBX child process. It fails if the parent
// leaked a base URL, the global secret or another PBX's secret into the child,
// logs every call, and serves fixtures from $FIXTURES/<pbx><path>.json.
func fakeChildMain(args []string) int {
	pbx, fixtures := os.Getenv("THREECX_PBX"), os.Getenv("FIXTURES")
	if f, err := os.OpenFile(filepath.Join(fixtures, "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintln(f, strings.Join(append([]string{pbx}, args...), " "))
		_ = f.Close()
	}
	fail := func(code int, msg string) int {
		fmt.Fprintln(os.Stderr, "Error: "+msg)
		return code
	}
	for _, k := range []string{"THREECX_BASE_URL", "THREECX_CLIENT_SECRET", "THREECX_USER_AGENT"} {
		if os.Getenv(k) != "" {
			return fail(9, "isolation env leaked")
		}
	}
	mine := "THREECX_" + strings.ToUpper(pbx) + "_CLIENT_SECRET"
	for _, v := range []string{"THREECX_ACME_CLIENT_SECRET", "THREECX_GLOBEX_CLIENT_SECRET", "THREECX_DOWN_CLIENT_SECRET"} {
		if v != mine && os.Getenv(v) != "" {
			return fail(9, "another PBX's secret leaked")
		}
	}
	if os.Getenv("THREECX_FLEET_DIR") == "" {
		return fail(9, "no fleet dir")
	}
	if pbx == "down" {
		return fail(4, "GET returned HTTP 403: forbidden")
	}
	var bodies []string
	for i := 0; i < len(args); i++ {
		if args[i] != "--req" || i+1 >= len(args) {
			continue
		}
		req, _, _ := strings.Cut(args[i+1], "?")
		name := pbx + strings.NewReplacer("/", "_", "(", "_", ")", "_", ".", "_").Replace(req) + ".json"
		body, err := os.ReadFile(filepath.Join(fixtures, name))
		if err != nil {
			return fail(3, "no fixture "+name)
		}
		bodies = append(bodies, string(body))
		i++
	}
	fmt.Println("[" + strings.Join(bodies, ",") + "]")
	return 0
}
