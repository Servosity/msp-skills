// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package vsphereblackbox

// Codex r1 classes output-before-chokepoint and undecoded-encoding-gap (fix
// helper H1), run against the built binary with Codex's exact inputs.

import (
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Shape 1: a permission-refused credential store whose password the operator
// then types into feedback. Shape 2: a loose config whose base_url userinfo
// holds a raw "?" (and "#", "/") before the last "@". Shape 3: a flag-like
// positional password Cobra rejects as an unknown flag.
func TestR1H1OutputBeforeChokepointShapes(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	if runtime.GOOS != "windows" {
		h := t.TempDir()
		dd := filepath.Join(h, "vc-refused")
		if err := os.MkdirAll(dd, 0o700); err != nil {
			t.Fatal(err)
		}
		store := filepath.Join(dd, "credentials.toml")
		if err := os.WriteFile(store, []byte("username=\"readonly@vsphere.local\"\npassword=\"RefusedPw-Q7n9\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chmod(store, 0o644)
		out.WriteString(runWith(t, cleanEnv(h, "VSPHERE_DATA_DIR="+dd), "feedback", "RefusedPw-Q7n9", "--json"))
		fb, err := os.ReadFile(filepath.Join(dd, "feedback.jsonl"))
		if err != nil {
			t.Fatalf("feedback wrote nothing; the refused-store case did not run: %v\n%s", err, out.String())
		}
		if strings.Contains(string(fb), "RefusedPw-Q7n9") {
			t.Errorf("feedback.jsonl repeats the refused store's password:\n%s", fb)
		}
	}
	for _, u := range []string{
		"https://ro:Only?UrlPw-Q7n9@other.example.com:bad/",
		"https://ro:Hash#UrlPw-H2k8@other.example.com/api",
		"https://ro:Sl/a?sh#UrlPw-S5m1@other.example.com:bad/",
	} {
		h := t.TempDir()
		p := filepath.Join(h, "vc-loose.toml")
		_ = os.WriteFile(p, []byte("base_url=\""+u+"\"\n"), 0o644)
		if runtime.GOOS != "windows" { // no group/other mode bits on Windows
			_ = os.Chmod(p, 0o644)
		}
		got := runWith(t, cleanEnv(h, "VSPHERE_CONFIG="+p, "VSPHERE_PASSWORD=EnvPw-A8s4"), "version")
		if runtime.GOOS != "windows" && !strings.Contains(got, "refused") && !strings.Contains(got, "not applied") && !strings.Contains(got, withheldNotice) {
			t.Fatalf("the loose-config case did not run (no refusal printed):\n%s", got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{"--DarkHorse-Q7n9", "-SingleDash-K8v2", "--DarkEq=Horse-N1c5"} {
		h := t.TempDir()
		got := runWith(t, cleanEnv(h), "auth", "set-credentials", "readonly@vsphere.local", pw)
		if !strings.Contains(got, withheldNotice) {
			t.Fatalf("set-credentials %s printed no notice for its error:\n%s", pw, got)
		}
		out.WriteString(got)
	}
	for _, pw := range []string{"RefusedPw-Q7n9", "UrlPw-Q7n9", "UrlPw-H2k8", "UrlPw-S5m1", "EnvPw-A8s4", "DarkHorse-Q7n9", "SingleDash-K8v2", "DarkEq", "Horse-N1c5"} {
		if strings.Contains(out.String(), pw) {
			t.Errorf("output repeats %q:\n%s", pw, out.String())
		}
	}
}

func z85Encode(b []byte) string {
	const alpha = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ.-:+=^!/*?&<>()[]{}@%$#"
	var sb strings.Builder
	for i := 0; i+4 <= len(b); i += 4 {
		n := uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3])
		var g [5]byte
		for j := 4; j >= 0; j-- {
			g[j] = alpha[n%85]
			n /= 85
		}
		sb.Write(g[:])
	}
	return sb.String()
}

// Codex's input: VSPHERE_PASSWORD='Ab!9xQ', a successful login, and an
// inventory whose VM name is ASCII85 of "aAb!9xQxyz"; plus the same short
// password embedded mid-stream in the other encodings.
func TestR1H1ShortValueEmbeddedInEncodedInventory(t *testing.T) {
	t.Parallel()
	const pw = "Ab!9xQ"
	a85 := func(b []byte) string {
		buf := make([]byte, ascii85.MaxEncodedLen(len(b)))
		return string(buf[:ascii85.Encode(buf, b)])
	}
	names := []string{
		`@6n?G3I"DGH$T`, // Codex's exact name
		"<~" + a85([]byte("xyAb!9xQq")) + "~>",
		z85Encode([]byte("aAb!9xQx")),
		base32.StdEncoding.EncodeToString([]byte("abAb!9xQxy")),
		strings.ToLower(base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("abcAb!9xQ"))),
		base64.RawURLEncoding.EncodeToString([]byte("aAb!9xQ")),
		hex.EncodeToString([]byte("aAb!9xQz")),
	}
	var mu sync.Mutex
	var listed bool
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/session":
			if _, p, ok := r.BasicAuth(); !ok || p != pw {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`"r1h1-session-6d0c2b"`))
		case r.URL.Path == "/api/vcenter/vm":
			mu.Lock()
			listed = true
			mu.Unlock()
			var recs []string
			for i, n := range names {
				v, _ := json.Marshal(n)
				recs = append(recs, fmt.Sprintf(`{"vm":"vm-%d","name":%s,"power_state":"POWERED_OFF"}`, i+1, v))
			}
			_, _ = w.Write([]byte("[" + strings.Join(recs, ",") + "]"))
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	h := t.TempDir()
	got := runWith(t, cleanEnv(h, "VSPHERE_BASE_URL="+srv.URL, "VSPHERE_CA_CERT="+ca, "VSPHERE_USERNAME=ro@vsphere.local", "VSPHERE_PASSWORD="+pw),
		"vm", "list", "--data-source", "live", "--agent")
	mu.Lock()
	ran := listed
	mu.Unlock()
	if !ran || !strings.Contains(got, withheldNotice) {
		t.Fatalf("the inventory was not listed and withheld; the test did not run:\n%s", got)
	}
	for _, n := range names {
		escaped, _ := json.Marshal(n)
		plain := strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&").Replace(string(escaped))
		for _, form := range []string{n, strings.Trim(string(escaped), `"`), strings.Trim(plain, `"`)} {
			if strings.Contains(got, form) {
				t.Errorf("output repeats the encoded password %q as %q:\n%s", n, form, got)
			}
		}
	}
}
