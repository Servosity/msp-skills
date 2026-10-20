// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

func TestEsetLocalPathQueryKeys(t *testing.T) {
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/v1/devices", nil},
		{"https://{region}.policy-management.eset.systems/v2/policy-assignments", nil},
		{"/v1/devices?", nil},
		{"/v1/devices?displayNames=DC01", []string{"displayNames"}},
		{"/v1/devices?displayNames=DC01&displayNames=KIOSK-03", []string{"displayNames"}},
		{"https://{region}.policy-management.eset.systems/v2/policy-assignments?policyUuids=p2&b=1", []string{"b", "policyUuids"}},
		{"/v1/devices:batchGet?devicesUuids=a%2Cb", []string{"devicesUuids"}},
		{"/v1/devices?display%4Eames=x", []string{"displayNames"}},
		{"/v1/devices?%zz=x", []string{"%zz"}},
		{"/v1/devices?=x", []string{"=x"}},
		{"/v1/devices?bare", []string{"bare"}},
		{"/v1/devices?&&a=1&&", []string{"a"}},
	} {
		if got := esetLocalPathQueryKeys(tc.path); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("esetLocalPathQueryKeys(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// localQueryCommand is one generated command that carries a filter in its
// path's query string.
type localQueryCommand struct {
	file, endpoint, key, flag string
}

// localQueryCommands scans the package's generated sources for every command
// that appends a filter to the request path (appendArrayQueryParam), so a
// reprint that adds one is covered without editing this test.
func localQueryCommands(t *testing.T) []localQueryCommand {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	appendRE := regexp.MustCompile(`appendArrayQueryParam\(path, "([^"]+)", (flag[A-Za-z0-9_]+),`)
	endpointRE := regexp.MustCompile(`"pp:endpoint": "([^"]+)"`)
	var out []localQueryCommand
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "helpers.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		if strings.Contains(src, `+ "?`) || strings.Contains(src, `path += "?`) {
			t.Errorf("%s builds a query string into the path some other way; cover it here", f)
		}
		ms := appendRE.FindAllStringSubmatch(src, -1)
		if len(ms) == 0 {
			continue
		}
		ep := endpointRE.FindStringSubmatch(src)
		if ep == nil {
			t.Fatalf("%s: no pp:endpoint annotation", f)
		}
		for _, m := range ms {
			flagRE := regexp.MustCompile(`StringVar\(&` + m[2] + `, "([^"]+)"`)
			fm := flagRE.FindStringSubmatch(src)
			if fm == nil {
				t.Fatalf("%s: no flag bound to %s", f, m[2])
			}
			out = append(out, localQueryCommand{file: f, endpoint: ep[1], key: m[1], flag: fm[1]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].file < out[j].file })
	return out
}

func localQueryFindCommand(root *cobra.Command, endpoint string) *cobra.Command {
	if root.Annotations["pp:endpoint"] == endpoint {
		return root
	}
	for _, sub := range root.Commands() {
		if c := localQueryFindCommand(sub, endpoint); c != nil {
			return c
		}
	}
	return nil
}

// localQuerySeed seeds the default mirror (for the env the caller set) with
// three devices and three policy assignments, and returns the home.
func localQuerySeed(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t)
	pin := RootCmd()
	pin.SetOut(io.Discard)
	pin.SetErr(io.Discard)
	pin.SetArgs([]string{"version", "--home", home})
	if err := pin.Execute(); err != nil {
		t.Fatal(err)
	}
	db, err := store.OpenWithContext(context.Background(), defaultDBPath(esetCLIName))
	if err != nil {
		t.Fatal(err)
	}
	for _, rt := range []string{"devices", "policy-assignments"} {
		rows := make([]json.RawMessage, 0, 3)
		for i, name := range []string{"DC01", "KIOSK-03", "FRONTDESK-01"} {
			b, _ := json.Marshal(map[string]any{"uuid": fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1), "displayName": name, "policyUuid": fmt.Sprintf("p%d", i+1)})
			rows = append(rows, b)
		}
		if _, _, err := db.UpsertBatch(rt, rows); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveSyncState(rt, "", 3); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return home
}

func localQueryRun(t *testing.T, home string, args ...string) (string, string, error) {
	t.Helper()
	root := RootCmd()
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append(append([]string(nil), args...), "--json", "--home", home))
	err := root.Execute()
	return out.String(), errOut.String(), err
}

// TestLocalReadsRefusePathQueryFilters (adversarial round 4, P1): a local
// read must never return a row the live filter would exclude. Every
// generated command that carries its filter in the path's query string
// (found by scanning the sources) is run with that filter and
// --data-source local over a mirror that holds matching and non-matching
// rows: each must fail closed naming the filter and print no rows, and
// without the filter the same command still reads the mirror. The auto mode
// fallback (ESET unreachable) refuses the same way.
func TestLocalReadsRefusePathQueryFilters(t *testing.T) {
	cmds := localQueryCommands(t)
	keys := map[string]bool{}
	for _, c := range cmds {
		keys[c.key] = true
	}
	for _, must := range []string{"displayNames", "policyUuids", "devicesUuids"} {
		if !keys[must] {
			t.Fatalf("the source scan missed the %s filter (found %+v)", must, cmds)
		}
	}
	for _, v := range []string{"ESET_PROTECT_TOKEN", "ESET_PROTECT_USERNAME", "ESET_PROTECT_PASSWORD", "ESET_PROTECT_BASE_URL"} {
		t.Setenv(v, "")
	}
	t.Setenv(cliutil.DogfoodEnvVar, "")
	home := localQuerySeed(t)

	run := func(t *testing.T, home string, c localQueryCommand, extra ...string) {
		t.Helper()
		cobraCmd := localQueryFindCommand(RootCmd(), c.endpoint)
		if cobraCmd == nil {
			t.Fatalf("no command annotated %s (%s)", c.endpoint, c.file)
		}
		path := strings.Fields(strings.TrimPrefix(cobraCmd.CommandPath(), cobraCmd.Root().Name()+" "))
		args := append(append(path, "--"+c.flag, "00000000-0000-4000-8000-000000000001,DC01,p1"), extra...)
		out, errOut, err := localQueryRun(t, home, args...)
		want := "local data could not apply " + c.key
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%v: err = %v, want %q\nstdout %s\nstderr %s", args, err, want, out, errOut)
		}
		var unsupported *localQueryUnsupportedError
		if strings.Contains(strings.Join(extra, " "), "local") && !errors.As(err, &unsupported) {
			t.Fatalf("%v: err %T is not the closed local-query refusal", args, err)
		}
		// The filter names DC01; KIOSK-03 and FRONTDESK-01 are rows the live
		// filter excludes (the error text may echo the filter itself).
		if strings.Contains(out, "KIOSK-03") || strings.Contains(out, "FRONTDESK-01") || strings.Contains(out, `"results"`) {
			t.Fatalf("%v printed rows: %s", args, out)
		}
	}

	for _, c := range cmds {
		c := c
		t.Run(c.file+"/local", func(t *testing.T) {
			run(t, home, c, "--data-source", "local")
		})
	}
	// Control: without the filter, the list commands still read the
	// mirror, so the refusal above is the filter's, not an empty mirror.
	for _, c := range cmds {
		cobraCmd := localQueryFindCommand(RootCmd(), c.endpoint)
		if cobraCmd.Name() != "list" {
			continue
		}
		path := strings.Fields(strings.TrimPrefix(cobraCmd.CommandPath(), cobraCmd.Root().Name()+" "))
		out, errOut, err := localQueryRun(t, home, append(path, "--data-source", "local")...)
		if err != nil || !strings.Contains(out, "KIOSK-03") {
			t.Fatalf("%v without the filter: %v\n%s\n%s", path, err, out, errOut)
		}
	}

	// Auto mode with ESET unreachable falls back to the same local read.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + ln.Addr().String()
	_ = ln.Close()
	t.Setenv("ESET_PROTECT_TOKEN", "test-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_BASE_URL", dead)
	t.Setenv(cliutil.DogfoodEnvVar, "1") // no retry backoff against the closed port
	autoHome := localQuerySeed(t)
	for _, c := range cmds {
		c := c
		t.Run(c.file+"/auto-unreachable", func(t *testing.T) {
			run(t, autoHome, c)
		})
	}
}
