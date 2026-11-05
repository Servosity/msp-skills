// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// Codex r1b class cache-answer-not-the-request, region
// (r1b-inventory-region-resolved), on the built binaries: a live inventory
// read scans the one region the AWS SDK resolves (--region, AWS_REGION,
// AWS_DEFAULT_REGION, the profile's region, else us-east-1). A stored read
// answers for exactly that region, resolved the same way: with the store
// holding a complete us-east-1 scan and the request resolving to us-west-2,
// an automatic read scans us-west-2 live and --data-source local is an error,
// never the us-east-1 rows. The MCP tools run the same CLI.

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestR1bStoredInventoryAnswersOnlyTheResolvedRegion(t *testing.T) {
	t.Parallel()
	for _, how := range []string{"AWS_REGION", "AWS_DEFAULT_REGION", "the profile's region"} {
		t.Run(how, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t, srcEnv)
			e.aws.quiet = true
			if layers := syncLayers(t, e.run(t, "sync", "--skip-cost", "--region", "us-east-1", "--json")); !layers["inventory"] {
				t.Fatalf("the us-east-1 inventory scan failed: %v", layers)
			}
			if east := e.run(t, "waste", "rank", "--region", "us-east-1", "--data-source", "local", "--json"); !r1JSONHas(east, "region", "us-east-1") {
				t.Fatalf("control: the stored us-east-1 scan does not answer a us-east-1 read:\n%.1200s", east)
			}
			west := e.with()
			west.env = withoutEnv(west.env, "AWS_REGION")
			switch how {
			case "AWS_REGION":
				west.env = append(west.env, "AWS_REGION=us-west-2")
			case "AWS_DEFAULT_REGION":
				west.env = append(west.env, "AWS_DEFAULT_REGION=us-west-2")
			default:
				west.input(t, filepath.Join(e.home, ".aws", "config"), "[default]\nregion = us-west-2\n")
			}

			for _, args := range [][]string{{"waste", "rank"}, {"waste", "gp2-gp3"}, {"waste", "snapshots"}} {
				local, code := west.runCode(t, "", bbCLI, append(args, "--data-source", "local", "--json")...)
				if code == 0 || r1JSONHas(local, "region", "us-east-1") || strings.Contains(local, "snap-0hq0001") {
					t.Errorf("%v --data-source local resolving to us-west-2 answered from the us-east-1 scan (exit %d):\n%.1200s", args, code, local)
				}
			}

			e.aws.reset()
			auto := west.run(t, "waste", "rank", "--json")
			if r1JSONHas(auto, "region", "us-east-1") {
				t.Errorf("waste rank resolving to us-west-2 answered with the stored us-east-1 rows:\n%.1200s", auto)
			}
			if e.aws.count("DescribeVolumes") == 0 || !r1JSONHas(auto, "region", "us-west-2") {
				t.Errorf("waste rank resolving to us-west-2 did not scan us-west-2 live (requests %v):\n%.1200s", e.aws.seen(), auto)
			}
			// Both scans are stored now: a local read answers for us-west-2 only.
			if local := west.run(t, "waste", "rank", "--data-source", "local", "--json"); r1JSONHas(local, "region", "us-east-1") || !r1JSONHas(local, "region", "us-west-2") {
				t.Errorf("with both regions stored, a us-west-2 local read did not answer for us-west-2 alone:\n%.1200s", local)
			}
			if how != "AWS_REGION" {
				return
			}
			// The MCP tool (a CLI child with the server's environment).
			fresh := newEnv(t, srcEnv)
			fresh.aws.quiet = true
			fresh.run(t, "sync", "--skip-cost", "--region", "us-east-1", "--json")
			mcp := fresh.with("AWS_REGION=us-west-2").mcpCalls(t, []map[string]any{{"name": "waste_rank", "arguments": map[string]any{"data-source": "local", "json": true}}})
			reply := mcpReplies(mcp)[1]
			if reply == "" || strings.Contains(reply, "snap-0hq0001") || strings.Contains(reply, "vol-0hq0001") || !strings.Contains(reply, `"isError":true`) {
				t.Errorf("MCP waste_rank resolving to us-west-2 answered from the us-east-1 scan:\n%.2000s", mcp)
			}
		})
	}
}
