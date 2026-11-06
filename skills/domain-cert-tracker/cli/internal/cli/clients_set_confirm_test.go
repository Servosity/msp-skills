package cli

import (
	"strings"
	"testing"
)

// clients set changes the inventory every sweep acts on, so like clients
// remove it previews and exits 2 until --yes (CLI) or confirm: true (MCP).
func TestClientsSetNeedsConfirm(t *testing.T) {
	trackerHome(t)
	out, _, err := runRootArgs(t, "--json", "clients", "set", "Acme", "--domain", "example.com")
	if err == nil || !strings.Contains(out, `"confirm_required": true`) {
		t.Fatalf("clients set without --yes/--confirm saved: err=%v out=%s", err, out)
	}
	list, _, err := runRootArgs(t, "--json", "clients", "list")
	if err != nil || strings.Contains(list, "example.com") {
		t.Fatalf("unconfirmed set reached the store: err=%v %s", err, list)
	}
	for _, c := range []string{"--yes", "--confirm"} {
		if _, _, err := runRootArgs(t, "clients", "set", "Acme", "--domain", "example.com", c); err != nil {
			t.Fatalf("clients set %s: %v", c, err)
		}
	}
}
