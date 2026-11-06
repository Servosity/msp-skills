// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated). The fleet tests' fake per-instance child, in Go
// so it runs on every platform: a shell script is not a program on Windows.

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"jamf-pro-pp-cli/internal/fleet"
)

// The fleet runner starts its child as "<exe> fleet exec --raw ...". A test
// binary is never started that way, so these arguments mean this process is the
// fake child, and it answers before any test runs.
func init() {
	if len(os.Args) > 3 && os.Args[1] == "fleet" && os.Args[2] == "exec" && os.Args[3] == "--raw" {
		os.Exit(fakeFleetChild(os.Args[1:]))
	}
}

// fakeFleetChild stands in for the per-instance child process. It fails if the
// parent leaked a base URL, client secret or another instance's secret into the
// child, logs every call, and serves one fixture file per request path.
func fakeFleetChild(args []string) int {
	dir, instance := os.Getenv(fleet.DirEnv), os.Getenv(fleet.SelectEnv)
	fail := func(code int, format string, a ...any) int {
		fmt.Fprintf(os.Stderr, "Error: "+format+"\n", a...)
		return code
	}
	if dir == "" {
		return fail(9, "no fleet dir")
	}
	if f, err := os.OpenFile(filepath.Join(dir, "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintf(f, "%s %s\n", instance, strings.Join(args, " "))
		_ = f.Close()
	}
	if os.Getenv("JAMF_PRO_BASE_URL") != "" || os.Getenv("JAMF_PRO_CLIENT_SECRET") != "" || os.Getenv("JAMF_PRO_SERVER") != "" {
		return fail(9, "isolation env leaked")
	}
	if s := os.Getenv(fleet.EntriesEnv); s != "" && (strings.Contains(s, ",") || !strings.HasPrefix(s, instance+"=")) {
		return fail(9, "another instance's secret leaked")
	}
	if instance == "down" {
		return fail(4, "GET returned HTTP 401: unauthorized")
	}
	name := strings.NewReplacer("/", "_", "(", "_", ")", "_", ".", "_", "&", "_", "%", "_")
	var bodies []string
	for i := 0; i < len(args); i++ {
		if args[i] != "--req" || i+1 >= len(args) {
			continue
		}
		req, _, _ := strings.Cut(args[i+1], "?")
		f := filepath.Join(dir, instance+name.Replace(req)+".json")
		b, err := os.ReadFile(f)
		if err != nil {
			return fail(3, "no fixture %s", f)
		}
		bodies = append(bodies, string(b))
		i++
	}
	fmt.Println("[" + strings.Join(bodies, ",") + "]")
	return 0
}
