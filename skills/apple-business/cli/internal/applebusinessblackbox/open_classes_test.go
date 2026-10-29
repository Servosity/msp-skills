// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package applebusinessblackbox

// The five credential classes the print's review left open, each from the failing
// input that review named, on the built binary. The credential kit closes them.
// Pointed at binaries built before the kit (APPLE_BUSINESS_BLACKBOX_BIN), each fails:
// .review/open-classes-before.log.

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const binaryCredential = "NewCredential-Ab12Cd34Ef56"

func TestOpenClassesStayClosed(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	keyRef := keyFile(t, f)
	dir := t.TempDir()
	write := func(name, body string, mode os.FileMode) string {
		t.Helper()
		path := filepath.Join(dir, name)
		writePrivate(t, path, body)
		if runtime.GOOS != "windows" {
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
		}
		return path
	}
	clean := func(t *testing.T, out string, secrets ...string) {
		t.Helper()
		for _, s := range secrets {
			if strings.Contains(out, s) {
				t.Errorf("the output repeats %q:\n%s", s, out)
			}
		}
	}

	t.Run("startup-output-bypass", func(t *testing.T) {
		// The loose file's own path is the credential.
		const token = "Cfg-" + "Zx93Km2Pa7Ld8"
		path := write(token+".toml", `access_token = "`+token+`"`+"\n", 0o644)
		out, _ := runCLI(t, bin, home, keyRef, f, "--config", path, "--help")
		clean(t, out, token)
	})
	t.Run("holder-miss-config", func(t *testing.T) {
		// A file that fails to decode, after its token.
		const token = "Parse-" + "Ab12Cd34Ef56"
		path := write("broken.toml", `access_token = "`+token+`"`+"\nbase_url = 42\n", 0o600)
		out, _ := runCLI(t, bin, home, keyRef, f, "--config", path, "devices", "list", "--"+token)
		clean(t, out, token)
	})
	t.Run("holder-miss-headers", func(t *testing.T) {
		// One value inside a Cookie header.
		const token = "Hdr-" + "Vb62Mx9Kc4Td7"
		path := write("cookie.toml", "[headers]\nCookie = \"session="+token+"\"\n", 0o600)
		out, _ := runCLI(t, bin, home, keyRef, f, "--config", path, "devices", "list", "--"+token)
		clean(t, out, token)
	})
	t.Run("holder-strength-exemption", func(t *testing.T) {
		// A short credential, base64 in another field of its own answer.
		out, _ := runCLI(t, bin, home, keyRef, f, "devices", "get", "SHORT", "--json")
		clean(t, out, "Zq7x2", "WnE3eDI=")
		if !strings.Contains(out, "withheld") {
			t.Errorf("the answer must be withheld, whole:\n%s", out)
		}
	})
	t.Run("binary-file-bypass", func(t *testing.T) {
		// A named credential in a binary answer, exported.
		path := filepath.Join(dir, "binary-new.jsonl")
		out, _ := runCLI(t, bin, home, keyRef, f, "export", "devices", "BINARYNEW", "--output", path)
		clean(t, out, binaryCredential)
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		clean(t, string(data), binaryCredential, base64.StdEncoding.EncodeToString([]byte("password="+binaryCredential)))
	})
}

// A credential piped to set-token is held before anything prints, on a run that
// fails on a flag before the command reads its stdin.
func TestPipedTokenIsHeldBeforeFlagParse(t *testing.T) {
	const token = "Piped-" + "Hk47Rt92Ws15"
	bin := binaries(t)
	f := newFake(t)
	cmd := exec.Command(filepath.Join(bin, "apple-business-cli"), "auth", "set-token", "--"+token)
	cmd.Env = env(t.TempDir(), keyFile(t, f), f)
	cmd.Stdin = strings.NewReader(token + "\n")
	out, _ := cmd.CombinedOutput() // the flag error exits non-zero
	if strings.Contains(string(out), token) {
		t.Errorf("the output repeats the piped token:\n%s", out)
	}
}

// A --stdin body is a request: it runs and previews, however it is laid out, and only
// its credential fields are held, before the preview prints.
func TestStdinBodyHoldsOnlyItsCredentialFields(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	home := t.TempDir()
	key := keyFile(t, f)
	preview := func(body string) (string, error) {
		cmd := exec.Command(filepath.Join(bin, "apple-business-cli"), "blueprints", "create", "--stdin", "--dry-run", "--json")
		cmd.Env = env(home, key, f)
		cmd.Stdin = strings.NewReader(body)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := preview("{\n  \"data\": {\n    \"type\": \"blueprints\",\n    \"attributes\": {\n      \"name\": \"Fleet Macs\"\n    }\n  }\n}\n")
	if err != nil || !strings.Contains(out, "Fleet Macs") {
		t.Errorf("a pretty-printed body must preview (%v):\n%s", err, out)
	}
	const secret = "Stdin-" + "Zk38Wq71Lp"
	out, _ = preview(`{"data":{"attributes":{"name":"Fleet Macs","password":"` + secret + `"},"nested":{"token":["` + secret + `2"]}}}`)
	if strings.Contains(out, secret) {
		t.Errorf("the preview repeats a credential field of the body:\n%s", out)
	}
	out, _ = preview("not json " + secret + "\n")
	if strings.Contains(out, secret) {
		t.Errorf("the refusal repeats a body that is not JSON:\n%s", out)
	}
}

// The signed assertion may go to Apple's token endpoint or the configured API origin,
// nowhere else: a token URL on a third origin fails with nothing sent.
func TestTokenURLOnAThirdOriginSendsNothing(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	third := []string{"APPLE_BUSINESS_TOKEN_URL=" + f.evil.URL + "/token"}
	out, code := runCLIEnv(t, bin, t.TempDir(), keyFile(t, f), f, third, "devices", "list", "--json")
	if code == 0 {
		t.Errorf("a token URL on a third origin must fail:\n%s", out)
	}
	if n := f.evilHits.Load(); n != 0 {
		t.Errorf("the third origin got %d request(s)", n)
	}
}

// A mint is followed by ordinary output in the same run: it must not make the kit hold
// the token form's public fields (client.AppleTokenBody), or "business" withholds it.
func TestAMintDoesNotHoldTheFormsPublicFields(t *testing.T) {
	bin := binaries(t)
	f := newFake(t)
	out, code := runCLI(t, bin, t.TempDir(), keyFile(t, f), f, "devices", "get", "PUBLICID", "--json")
	if code != 0 || !strings.Contains(out, clientID) {
		t.Errorf("an answer naming the API account must print after a mint (exit %d):\n%s", code, out)
	}
}
