// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestKCLineNeverPutsTheSecretOnArgv pins the shape of the security(1) call:
// the exec argv is the fixed pair ("/usr/bin/security", "-i") and the secret
// travels inside the stdin command line. kcLine is the only place an operand
// is rendered, so asserting on it covers kcSet, kcGet, and kcDelete.
func TestKCLineNeverPutsTheSecretOnArgv(t *testing.T) {
	line, err := kcLine("add-generic-password", "-U", "-a", kcAcctAccess, "-s", mcpKCService, "-w", `tok"en\with space`)
	if err != nil {
		t.Fatal(err)
	}
	want := `add-generic-password "-U" "-a" "MCP_ACCESS_TOKEN" "-s" "granola-cli-mcp" "-w" "tok\"en\\with space"` + "\n"
	if line != want {
		t.Fatalf("kcLine:\n got %q\nwant %q", line, want)
	}
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("a command line must end in exactly one newline: %q", line)
	}
}

// TestKCQuoteRefusesControlCharacters is the injection guard: a newline in an
// operand would end the command line and run the remainder as a second
// security(1) command. kcQuote must refuse it rather than escape it.
func TestKCQuoteRefusesControlCharacters(t *testing.T) {
	for _, v := range []string{"a\nb", "a\rb", "a\x00b", "a\tb", "a\x7fb"} {
		if _, err := kcQuote(v); err == nil {
			t.Errorf("kcQuote(%q) = nil error, want a refusal", v)
		}
		if _, err := kcLine("add-generic-password", "-w", v); err == nil {
			t.Errorf("kcLine with operand %q = nil error, want a refusal", v)
		}
	}
	// The explicit-keychain operand goes through the same guard.
	prev := kcKeychainPath
	kcKeychainPath = "/tmp/x\ndelete-keychain login.keychain"
	t.Cleanup(func() { kcKeychainPath = prev })
	if _, err := kcLine("find-generic-password", "-a", "x"); err == nil {
		t.Error("kcLine with a newline in the keychain path = nil error, want a refusal")
	}
}

// TestKCRoundTripThroughSecurityStdin drives the real security(1) binary
// against a throwaway keychain. It is opt-in (GRANOLA_KEYCHAIN_IT=1) because
// `security create-keychain` adds the keychain to the user's search list for
// the duration of the test; production code never touches that list.
func TestKCRoundTripThroughSecurityStdin(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("GRANOLA_KEYCHAIN_IT") != "1" {
		t.Skip("darwin-only; set GRANOLA_KEYCHAIN_IT=1 to run against a throwaway keychain")
	}
	kc := filepath.Join(t.TempDir(), "granola-it.keychain-db")
	if out, err := exec.Command("/usr/bin/security", "create-keychain", "-p", "it", kc).CombinedOutput(); err != nil {
		t.Fatalf("create-keychain: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("/usr/bin/security", "delete-keychain", kc).Run() })
	if out, err := exec.Command("/usr/bin/security", "unlock-keychain", "-p", "it", kc).CombinedOutput(); err != nil {
		t.Fatalf("unlock-keychain: %v: %s", err, out)
	}
	prev := kcKeychainPath
	kcKeychainPath = kc
	t.Cleanup(func() { kcKeychainPath = prev })

	const acct = "MCP_IT_ACCOUNT"
	for _, v := range []string{"plain.token-value_~+/=", `has "quotes" and \backslash\`, "-starts-with-a-dash", "has a space"} {
		if err := kcSet(acct, v); err != nil {
			t.Fatalf("kcSet(%q): %v", v, err)
		}
		got, err := kcGet(acct)
		if err != nil {
			t.Fatalf("kcGet after kcSet(%q): %v", v, err)
		}
		if got != strings.TrimSpace(v) {
			t.Fatalf("round trip: set %q, got %q", v, got)
		}
	}
	if err := kcDelete(acct); err != nil {
		t.Fatalf("kcDelete: %v", err)
	}
	if _, err := kcGet(acct); !kcNotFound(err) {
		t.Fatalf("kcGet after delete: err=%v, want errSecItemNotFound (exit 44)", err)
	}
	if err := kcDelete(acct); !kcNotFound(err) {
		t.Fatalf("second kcDelete: err=%v, want errSecItemNotFound (exit 44)", err)
	}
}
