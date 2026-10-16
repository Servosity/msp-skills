// Adversarial round 34 receipts: --quiet/--plain print rows, failures by end
// time, token-shaped host values.

package cli

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// --quiet is bare output, one value per line: the rows' identities, never
// the report envelope's first number. --plain prints the rows.
func TestQuietPrintsRowIdentities(t *testing.T) {
	newFixture(t)
	out, _, _ := runCLI(t, "posture", "--only", "red", "--quiet")
	red, _, _ := runCLI(t, "posture", "--only", "red", "--json")
	want := len(decodeJSON[postureReport](t, red).Rows)
	if lines := strings.Fields(out); want == 0 || len(lines) != want || !strings.Contains(out, "default:") {
		t.Errorf("posture --only red --quiet: want %d asset ids, got %q", want, out)
	}
	out, _, _ = runCLI(t, "verify-health", "--quiet")
	if !strings.Contains(out, "main/") {
		t.Errorf("verify-health --quiet: want group identities, got %q", out)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--quiet")
	if strings.TrimSpace(out) == "7" || !strings.Contains(out, "default:") {
		t.Errorf("rpo --quiet: want cluster:vmid lines, got %q", out)
	}
	out, _, _ = runCLI(t, "rpo", "--days", "7", "--plain")
	if !strings.Contains(out, "\t") || strings.Count(out, "\n") < 2 {
		t.Errorf("rpo --plain: want rows, got %q", out)
	}
}

// A failed task that started before --since but ended inside it counts.
func TestFailuresByEndTime(t *testing.T) {
	f := newFixture(t)
	tasks := f.pveRoutes["/nodes/pve1/tasks"].([]map[string]any)
	tasks[0]["starttime"], tasks[0]["endtime"] = ago(f.now, 30), ago(f.now, 2) // a 28h job that failed 2h ago
	// Like PVE: since filters by start time.
	pveVia(t, f, func(w http.ResponseWriter, r *http.Request) bool {
		if !strings.HasSuffix(r.URL.Path, "/nodes/pve1/tasks") {
			return false
		}
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		var out []map[string]any
		for _, tk := range tasks {
			if tk["starttime"].(int64) >= since {
				out = append(out, tk)
			}
		}
		writeExtra(w, out, "total", len(out))
		return true
	})
	out, _, _ := runCLI(t, "failures", "--since", "24h", "--json")
	found := false
	for _, r := range decodeJSON[failuresReport](t, out).Rows {
		found = found || r.VMID == 108
	}
	if !found {
		t.Errorf("the job that failed 2h ago after starting 30h ago is not listed:\n%s", out)
	}
	_ = time.Hour
}

// A token pasted into a host field (clusters.toml or PROXMOX_HOST) is never
// printed and never used as a host.
func TestTokenInHostFieldIsNeverShown(t *testing.T) {
	newFixture(t)
	const secret = "5ec7e7aa-0000-4000-8000-00000000c0de"
	writeClustersFile(t, `
[clusters.acme]
pve_host = "auditor@pve!cli=`+secret+`"
pve_token_env = "ACME_PVE"
`)
	setAcmeTokens(t)
	for _, args := range [][]string{{"clusters", "list", "--json"}, {"clusters", "list"}, {"posture", "--json"}, {"clusters", "check", "--json"}} {
		out, errOut, _ := runCLI(t, args...)
		if strings.Contains(out+errOut, secret) {
			t.Errorf("%v printed the token", args)
		}
	}
	writeClustersFile(t, "")
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "auditor@pve!cli="+secret)
	for _, args := range [][]string{{"doctor", "--json"}, {"posture", "--json"}, {"nodes", "list", "--dry-run"}} {
		out, errOut, _ := runCLI(t, args...)
		if strings.Contains(out+errOut, secret) {
			t.Errorf("env %v printed the token", args)
		}
	}
}
