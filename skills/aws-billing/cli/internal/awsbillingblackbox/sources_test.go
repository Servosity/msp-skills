// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Every AWS credential source through every sink (the kit wired), and one run
// per sink against answers that repeat every held credential.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// awsCommand reports a command line that talks to AWS through the SDK and so
// takes --profile-aws.
func awsCommand(args []string) bool {
	switch args[0] {
	case "doctor", "sync", "bill", "compare", "consolidated", "forecast", "dimensions", "ask", "waste", "report":
		return true
	case "accounts":
		return len(args) > 1 && args[1] == "list"
	}
	return false
}

// battery is every output path of the CLI, each output kind once: every
// startup source and every SDK credential path. deliverFile and exportFile
// are files the runs write.
func battery(e *bbEnv, deliverFile, exportFile string) [][]string {
	return [][]string{
		{"doctor", "--json"},
		{"sync", "--json"},
		{"bill", "--json", "--deliver", "file:" + deliverFile},
		{"bill", "--data-source", "live", "--json"},
		{"consolidated", "--data-source", "live"},
		{"dimensions", "SERVICE"},
		{"waste", "rank"},
		{"accounts", "list", "--json"},
		{"export", "costs", "--output", exportFile},
		{"report", "--period", "this-month"},
		{"report", "--period", "this-month", "--data-source", "live", "--out", filepath.Join(e.work, "live-report.html")},
		{"teach", "--query", "top services", "--resource", "bill", "--resource-type", "costs", "--notes", "rotated", "--no-validate"},
	}
}

// mcpBattery is the MCP tools over the same paths: mirrored CLI commands (a
// child CLI each), the generated client's endpoint tools and the local store.
func mcpBattery(e *bbEnv) []map[string]any {
	profile := map[string]any{}
	if len(e.awsArgs) > 0 {
		profile["profile-aws"] = e.awsArgs[1]
	}
	with := func(extra map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range profile {
			out[k] = v
		}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}
	return []map[string]any{
		{"name": "sync", "arguments": with(map[string]any{"json": true})},
		{"name": "bill", "arguments": with(map[string]any{"data-source": "live", "json": true})},
		{"name": "accounts_list", "arguments": map[string]any{}},
		{"name": "sql", "arguments": map[string]any{"query": "SELECT data FROM resources"}},
	}
}

// Every AWS credential source signs requests that pass the kit's outbound
// guard (a request the guard cannot read fails closed for the whole
// process), and every sink carries the data: the fake repeats nothing here.
func TestEverySourceSignsAndEverySinkWrites(t *testing.T) {
	for _, src := range []source{srcEnv, srcProfileEnv, srcRole, srcSSO} {
		t.Run(src.String(), func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, src)
			e.aws.set(func(f *fakeAWS) { f.quiet = true })
			e.api.quiet = true
			deliverFile := filepath.Join(e.work, "deliver.json")
			exportFile := filepath.Join(e.work, "export.jsonl")
			outs := map[string]string{} // for the diagnostics below
			for _, args := range battery(e, deliverFile, exportFile) {
				if awsCommand(args) {
					args = append(append([]string(nil), args...), e.awsArgs...)
				}
				o, code := e.runCode(t, "", bbCLI, args...)
				outs[strings.Join(args[:min(2, len(args))], " ")] += o
				if code != 0 || strings.Contains(o, "withheld") {
					t.Errorf("%v: exit %d:\n%.1500s", args, code, o)
				}
			}
			calls := mcpBattery(e)
			out := e.mcpCalls(t, calls)
			if len(mcpReplies(out)) < len(calls)+1 || strings.Contains(out, "withheld") || strings.Contains(out, `"isError":true`) {
				t.Errorf("MCP session:\n%.3000s", out)
			}

			if signer := src.signer(); (signer != "" && e.aws.signed(signer) == 0) || (signer == "" && e.aws.mintedSigned() == 0) {
				t.Fatalf("no AWS call was signed with %s's credentials; requests: %v", src, e.aws.seen())
			}
			switch src {
			case srcRole:
				if e.aws.count("AssumeRole") == 0 || e.aws.signed(bbFileKey) == 0 {
					t.Errorf("the role profile never assumed its role with bbfile's keys: %v", e.aws.seen())
				}
			case srcSSO:
				if e.aws.count("GetRoleCredentials") == 0 || e.aws.sawSSOToken(bbSSOToken) == 0 {
					t.Errorf("the SSO profile never sent its cached token: %v", e.aws.seen())
				}
			}
			if e.api.sawAuth("Bearer "+bbGenToken) == 0 {
				t.Errorf("the generated client never sent config.toml's auth_header; requests: %v", e.api.seen())
			}
			for _, op := range []string{"GetCallerIdentity", "GetCostAndUsage", "GetDimensionValues", "ListAccounts", "DescribeOrganization",
				"DescribeVolumes", "DescribeSnapshots", "DescribeAddresses", "DescribeInstances", "GetMetricStatistics", "ListBuckets"} {
				if e.aws.count(op) == 0 {
					t.Errorf("the battery never called %s; requests: %v", op, e.aws.seen())
				}
			}
			files := e.writtenFiles(t)
			for what, ok := range map[string]bool{
				"the local store holds a synced cost line": anyFile(files, func(r string) bool { return strings.HasSuffix(r, ".db") }, "HQ-Service-Compute"),
				"the local store holds a synced account":   anyFile(files, func(r string) bool { return strings.HasSuffix(r, ".db") }, "HQ-Account"),
				"the report HTML is in the working directory": anyFile(files, func(r string) bool {
					return strings.HasPrefix(r, "work/aws-bill-") && strings.HasSuffix(r, ".html")
				}, "HQ-Service-Compute"),
				"the live report HTML was written":            anyFile(files, func(r string) bool { return r == "work/live-report.html" }, "HQ-Service-Compute"),
				"the export file holds the exported record":   anyFile(files, func(r string) bool { return r == "work/export.jsonl" }, "HQ-GenService"),
				"the delivered file holds the bill":           anyFile(files, func(r string) bool { return r == "work/deliver.json" }, "HQ-Service-Compute"),
				"the learn journal holds this run's commands": anyFile(files, func(r string) bool { return strings.Contains(r, "/learn/journal-") }, "dimensions"),
			} {
				if !ok {
					t.Errorf("%s: not so; files written: %v\nreport output:\n%.1500s", what, keys(files), outs["report --period"])
				}
			}
			for rel := range files {
				if runtime.GOOS != "windows" && strings.HasSuffix(rel, ".html") { // Windows reports 0666 for every file
					if info, err := os.Stat(filepath.Join(e.home, rel)); err == nil && info.Mode().Perm() != 0o600 {
						t.Errorf("%s mode %v, want 0600", rel, info.Mode().Perm())
					}
				}
			}
		})
	}
}

// One run per sink against answers that repeat every credential the process
// holds (whole, in every encoding contract 3 names, and split): stdout,
// stderr (a refusing AWS and a flag error), the local store, an export, a
// --deliver file, report HTML, the learn store, and MCP. None of it shows.
func TestNoSinkCarriesAnEchoedCredential(t *testing.T) {
	t.Parallel()
	e := newEnv(t, srcEnv)
	deliverFile := filepath.Join(e.work, "deliver.json")
	for _, args := range [][]string{
		{"sync", "--json"},
		{"bill", "--data-source", "live", "--json"},
		{"bill", "--json", "--deliver", "file:" + deliverFile},
		{"export", "costs", "--output", filepath.Join(e.work, "export.jsonl")},
		{"report", "--period", "this-month", "--data-source", "live", "--out", filepath.Join(e.work, "live-report.html")},
		{"bill", "--no-such-flag=" + e.src.typed()},
		{"teach", "--query", "top services " + e.src.typed(), "--resource", "bill", "--resource-type", "costs",
			"--notes", "rotated " + e.src.typed(), "--no-validate"},
	} {
		assertNoEcho(t, fmt.Sprintf("output of %v", args), e.run(t, args...))
	}
	e.aws.setMode(modeDeny)
	assertNoEcho(t, "output of bill (AWS refusing)", e.run(t, "bill", "--data-source", "live"))
	e.aws.setMode(modeOK)
	out := e.mcpCalls(t, mcpBattery(e)[1:3])
	if len(mcpReplies(out)) < 3 {
		t.Fatalf("MCP session did not answer every call:\n%.3000s", out)
	}
	assertNoEcho(t, "MCP session", out)
	e.assertNoCredentialInFiles(t)
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
