// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring): the credential classes the print's
// review left open, each closed by routing its source or sink through the kit
// and proven here from the finding's own input, on the real binaries.

package credwire

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"warranty-pp-cli/internal/client"
	"warranty-pp-cli/internal/credkit"
)

func requireNowhere(t *testing.T, s sandbox, res runResult, what, secret string, skipFiles ...string) {
	t.Helper()
	requireAbsent(t, what+" stdout", res.stdout, secret)
	requireAbsent(t, what+" stderr", res.stderr, secret)
	if hits := filesHolding(t, s.home, secret, skipFiles...); len(hits) > 0 {
		t.Errorf("%s wrote the credential to %v", what, hits)
	}
}

// secret-echo: a credential value on the command line, outside auth login. The
// finding's input: `report --client-secret --leakvalue123`, which reached the
// journal as an argv_shape key.
func TestKitArgvCredentialNeverEchoed(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "leakvalue123"
	for _, args := range [][]string{
		{"report", "--client-secret", "--" + secret},
		{"report", "--" + secret},
		{secret},
		{"--" + secret + "=1"},
		{"lookup", "--bogus-flag", secret},
	} {
		s := newSandbox(t)
		res := run(t, cli, s.env("WARRANTY_DELL_CLIENT_SECRET="+secret), args...)
		requireNowhere(t, s, res, fmt.Sprint(args), secret)
		if res.stdout+res.stderr == "" {
			t.Errorf("%v: the run printed nothing at all, so this test proved nothing", args)
		}
	}
}

// secret-echo: TOML multi-line strings, and a config that does not parse. The
// finding's input: access_token="""storedsecret123""" with --config c
// --storedsecret123, where Cobra fails before config.Load.
func TestKitStartupHoldsEveryConfigForm(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "storedsecret123"
	for name, cfg := range map[string]string{
		"basic multi-line":   `access_token="""` + secret + `"""`,
		"literal multi-line": `access_token='''` + secret + `'''`,
		"split multi-line":   "client_secret = \"\"\"\n" + secret + "\"\"\"",
		"escaped":            `client_secret = "storedsecret123"`,
		"headers table":      "[headers]\nX-Custom = \"" + secret + "\"",
		"before a bad line":  `access_token = "` + secret + `"` + "\nnot toml at all [[[",
		"after a bad line":   "not toml at all [[[\n" + `access_token = "` + secret + `"`,
		"in an open array":   "bad = [\n" + `access_token = "` + secret + `"`,
		"dell_client_secret": `dell_client_secret = "` + secret + `"`,
	} {
		s := newSandbox(t)
		c := s.writePrivate(t, "c.toml", cfg+"\n")
		for _, args := range [][]string{{"--config", c, "--" + secret}, {"--config", c, secret}, {"--config=" + c, "doctor", "--json", "--" + secret}} {
			res := run(t, cli, s.env(), args...)
			requireNowhere(t, s, res, name+fmt.Sprint(args), secret)
		}
		// The config the environment names, or the default one, which only
		// Declarations.Files tells the kit about.
		res := run(t, cli, s.env("WARRANTY_CONFIG="+c), "--"+secret)
		requireNowhere(t, s, res, name+" WARRANTY_CONFIG", secret)
		def := filepath.Join(s.home, "config", "config.toml")
		if err := os.MkdirAll(filepath.Dir(def), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(def, []byte(cfg+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		res = run(t, cli, s.env(), "--"+secret)
		requireAbsent(t, name+" default config stdout", res.stdout, secret)
		requireAbsent(t, name+" default config stderr", res.stderr, secret)
		// The MCP server's own flags go through Go's flag package, not Cobra.
		_, mcp := binaries(t)
		res = run(t, mcp, s.env("WARRANTY_CONFIG="+c), "--"+secret)
		requireAbsent(t, name+" mcp stdout", res.stdout, secret)
		requireAbsent(t, name+" mcp stderr", res.stderr, secret)
	}
}

// secret-echo: a minted token not held when saving it fails. The finding's
// input: the token equals the unwritable data directory, and the lock-file
// error names that directory.
func TestKitMintedTokenHeldWhenSavingFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory this user cannot write")
	}
	cli, _ := binaries(t)
	s := newSandbox(t)
	v := newVendors(t, s)
	deny := filepath.Join(s.root, "deny")
	if err := os.Mkdir(deny, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(deny, 0o700) })
	v.dellToken = func() (int, string) { return 200, `{"access_token":"` + deny + `","expires_in":3600}` }
	res := run(t, cli, s.env(append(v.env(), "WARRANTY_DATA_DIR="+deny, "WARRANTY_DELL_CLIENT_ID="+stubDellClient, "WARRANTY_DELL_CLIENT_SECRET="+stubDellSecret)...),
		"lookup", "1ABC234", "--vendor", "dell", "--no-save", "--no-learn", "--json")
	if len(v.requests(hostDell)) == 0 {
		t.Fatalf("the mint never ran (exit %d): %s %s", res.code, res.stdout, res.stderr)
	}
	requireAbsent(t, "stdout", res.stdout, deny)
	requireAbsent(t, "stderr", res.stderr, deny)
}

// host-pinning: another vendor's credential in an allowed header or query
// string. The findings' inputs: [headers] ClientID = <the Dell secret> with
// `lenovo warranty`, and MCP lenovo_warranty {"Serial": <the Dell secret>}.
func TestKitForeignCredentialNeverReachesAnotherVendor(t *testing.T) {
	cli, mcp := binaries(t)

	t.Run("Dell secret as a configured Lenovo header", func(t *testing.T) {
		s := newSandbox(t)
		v := newVendors(t, s)
		c := s.writePrivate(t, "c.toml", "[headers]\nClientID = \""+stubDellSecret+"\"\n")
		res := run(t, cli, allVendorEnv(t, s, v), "--config", c, "lenovo", "warranty", "--serial", "PF1ABCDE", "--json")
		if v.carried(hostLenovo, stubDellSecret) {
			t.Errorf("the Dell secret reached Lenovo: %+v", v.requests(hostLenovo))
		}
		requireNowhere(t, s, res, "cli", stubDellSecret)
	})
	t.Run("Dell secret as a Lenovo serial", func(t *testing.T) {
		s := newSandbox(t)
		v := newVendors(t, s)
		res := run(t, cli, allVendorEnv(t, s, v), "lenovo", "warranty", "--serial", stubDellSecret, "--json")
		if v.carried(hostLenovo, stubDellSecret) {
			t.Errorf("the Dell secret reached Lenovo: %+v", v.requests(hostLenovo))
		}
		requireNowhere(t, s, res, "cli", stubDellSecret)
	})
	t.Run("Dell secret as a Lenovo serial over MCP", func(t *testing.T) {
		s := newSandbox(t)
		v := newVendors(t, s)
		frames, stderr := mcpCall(t, mcp, allVendorEnv(t, s, v), "lenovo_warranty", map[string]any{"Serial": stubDellSecret})
		if v.carried(hostLenovo, stubDellSecret) {
			t.Errorf("the Dell secret reached Lenovo: %+v", v.requests(hostLenovo))
		}
		requireAbsent(t, "MCP frames", frames, stubDellSecret)
		requireAbsent(t, "MCP stderr", stderr, stubDellSecret)
	})
	t.Run("Lenovo key as a Dell service tag", func(t *testing.T) {
		s := newSandbox(t)
		v := newVendors(t, s)
		res := run(t, cli, allVendorEnv(t, s, v), "dell", "entitlements", "--servicetags", stubLenovoKey, "--json")
		if v.carried(hostDell, stubLenovoKey) {
			t.Errorf("the Lenovo key reached Dell: %+v", v.requests(hostDell))
		}
		requireNowhere(t, s, res, "cli", stubLenovoKey)
	})
	t.Run("Stored Dell token as an Apple device id", func(t *testing.T) {
		s := newSandbox(t)
		v := newVendors(t, s)
		const tok = "stored-dell-token-9K8J"
		c := s.writePrivate(t, "c.toml", "access_token = \""+tok+"\"\n")
		res := run(t, cli, allVendorEnv(t, s, v), "--config", c, "apple", "coverage", tok, "--json")
		if v.carried(hostAppleAPI, tok) {
			t.Errorf("the Dell token reached Apple: %+v", v.requests(hostAppleAPI))
		}
		requireNowhere(t, s, res, "cli", tok, c)
	})
	t.Run("Each vendor's own credential still reaches that vendor", func(t *testing.T) {
		s := newSandbox(t)
		v := newVendors(t, s)
		res := run(t, cli, allVendorEnv(t, s, v), "lookup", "1ABC234", "PF1ABCDE", "C02XG0FDH7JY", "--as-of", "2026-10-28", "--confirm", "--json")
		if res.code != 0 {
			t.Fatalf("lookup failed (exit %d): %s %s", res.code, res.stdout, res.stderr)
		}
		if !v.carried(hostLenovo, stubLenovoKey) {
			t.Errorf("the Lenovo key never reached Lenovo: %+v", v.requests(""))
		}
		if !v.carried(hostDell, "Bearer "+stubDellToken) {
			t.Errorf("the Dell token never reached Dell: %+v", v.requests(hostDell))
		}
		if !v.carried(hostAppleAPI, "Bearer "+stubAppleToken) {
			t.Errorf("the Apple token never reached Apple: %+v", v.requests(""))
		}
		for _, r := range v.requests(hostAppleOAuth) {
			if !strings.Contains(r.body, "client_assertion=") {
				t.Errorf("Apple's token request carried no assertion: %+v", r)
			}
		}
		for _, want := range []string{"1ABC234", "PF1ABCDE", "C02XG0FDH7JY"} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("stdout lacks %s: %s", want, res.stdout)
			}
		}
	})
}

// The bindings themselves, for what no binary can be made to send: each held
// value against each origin, in a process that ran credkit.Start with these
// Declarations. TestMain re-enters here when kitMatrixEnv is set.
const kitMatrixEnv = "WARRANTY_TEST_KIT_MATRIX"

type allowAll struct{ sent *bool }

func (a allowAll) RoundTrip(r *http.Request) (*http.Response, error) {
	*a.sent = true
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("{}")), Request: r}, nil
}

func runKitMatrix() {
	credkit.Start(Declarations)
	credkit.Hold(client.SourceDellMint, stubDellToken)
	credkit.Hold("minted:apple-token", stubAppleToken)
	credkit.Hold("minted:apple-assertion", "assertion-value-5T4R3E2W")
	dell, lenovo, appleOAuth, appleAPI := "https://"+hostDell, "https://"+hostLenovo, "https://"+hostAppleOAuth, "https://"+hostAppleAPI
	for _, c := range []struct {
		name, value string
		allowed     []string
	}{
		{"dell client id", stubDellClient, []string{dell}},
		{"dell secret", stubDellSecret, []string{dell}},
		{"dell token", stubDellToken, []string{dell}},
		{"lenovo key", stubLenovoKey, []string{lenovo}},
		{"apple client id", stubAppleUnique, []string{appleOAuth}},
		{"apple key id", stubAppleKeyID, []string{appleOAuth}},
		{"apple assertion", "assertion-value-5T4R3E2W", []string{appleOAuth}},
		{"apple token", stubAppleToken, []string{appleAPI}},
	} {
		for _, origin := range []string{dell, lenovo, appleOAuth, appleAPI, "https://elsewhere.example"} {
			want := false
			for _, a := range c.allowed {
				want = want || a == origin
			}
			for part, build := range map[string]func(*http.Request){
				"query":  func(r *http.Request) { r.URL.RawQuery = "x=" + c.value },
				"header": func(r *http.Request) { r.Header.Set("X-Note", c.value) },
			} {
				sent := false
				req, _ := http.NewRequest(http.MethodGet, origin+"/p", nil)
				build(req)
				resp, err := credkit.Transport(allowAll{&sent}).RoundTrip(req)
				if err == nil {
					_ = resp.Body.Close()
				}
				if sent != want {
					fmt.Printf("FAIL %s in a %s to %s: sent=%v, want %v\n", c.name, part, origin, sent, want)
				}
			}
		}
	}
	fmt.Println("MATRIX DONE")
	credkit.Exit(0)
}

func TestKitOriginMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a child process")
	}
	s := newSandbox(t)
	env := s.env(kitMatrixEnv+"=1", "WARRANTY_DELL_CLIENT_ID="+stubDellClient, "WARRANTY_DELL_CLIENT_SECRET="+stubDellSecret,
		"WARRANTY_LENOVO_CLIENT_KEY="+stubLenovoKey, "WARRANTY_APPLE_CLIENT_ID="+stubAppleClient, "WARRANTY_APPLE_KEY_ID="+stubAppleKeyID)
	res := run(t, os.Args[0], env)
	if strings.Contains(res.stdout, "FAIL ") || !strings.Contains(res.stdout, "MATRIX DONE") {
		t.Fatalf("exit %d\n%s\n%s", res.code, res.stdout, res.stderr)
	}
}

// A vendor echoing a credential back, in plain text or encoded, reaches no
// sink: the moved TestWarrantyNoCredentialLeak.
func TestKitVendorEchoesNeverReachASink(t *testing.T) {
	cli, mcp := binaries(t)
	forms := func(s string) []string {
		return []string{s, base64.StdEncoding.EncodeToString([]byte(s)), hex.EncodeToString([]byte(s))}
	}
	held := map[string]string{"dell secret": stubDellSecret, "lenovo key": stubLenovoKey, "apple token": stubAppleToken, "dell token": stubDellToken, "dell client": stubDellClient}
	setup := func(s sandbox) (*vendors, []string) {
		v := newVendors(t, s)
		v.dell = func(r *http.Request) (int, string) {
			return 400, `{"error":"bad request","echo":"` + base64.StdEncoding.EncodeToString([]byte(stubDellSecret)) + `","hex":"` + hex.EncodeToString([]byte(stubDellSecret)) + `"}`
		}
		v.lenovo = func(r *http.Request) (int, string) {
			return 200, `{"Serial":"PF1ABCDE","Product":"` + stubLenovoKey + `","Warranty":[]}`
		}
		v.apple = func(r *http.Request) (int, string) {
			return 200, `{"data":[{"id":"x","attributes":{"status":"ACTIVE","description":"` + hex.EncodeToString([]byte(stubAppleToken)) + `"}}]}`
		}
		return v, allVendorEnv(t, s, v)
	}
	check := func(s sandbox, what, text string) {
		t.Helper()
		for name, secret := range held {
			for _, f := range forms(secret) {
				if strings.Contains(text, f) {
					t.Errorf("%s repeats the %s (form %q):\n%s", what, name, f, text)
				}
			}
		}
	}
	checkFiles := func(s sandbox, skip ...string) {
		t.Helper()
		for name, secret := range held {
			for _, f := range forms(secret) {
				if hits := synccpuNotStore(filesHolding(t, s.home, f, skip...)); len(hits) > 0 { // synccpu: the local store is not checked
					t.Errorf("the %s is in %v", name, hits)
				}
			}
		}
	}
	s := newSandbox(t)
	v, env := setup(s)
	deliver := filepath.Join(s.root, "delivered.json")
	for _, args := range [][]string{
		{"lookup", "1ABC234", "PF1ABCDE", "C02XG0FDH7JY", "--confirm", "--json"},
		{"lookup", "1ABC234", "--vendor", "dell"},
		{"dell", "entitlements", "--servicetags", "1ABC234", "--json"},
		{"lenovo", "warranty", "--serial", "PF1ABCDE", "--json"},
		{"lenovo", "warranty", "--serial", "PF1ABCDE"},
		{"lenovo", "warranty", "--serial", "PF1ABCDE", "--json", "--deliver", "file:" + deliver},
		{"lenovo", "contract", "4400123456", "--json"},
		{"apple", "list", "--json"},
		{"apple", "coverage", "C02XG0FDH7JY", "--json"},
		{"doctor", "--json"},
		{"auth", "status", "--json"},
		{"report", "--json"},
		{"events", "list", "--json"},
	} {
		res := run(t, cli, env, args...)
		check(s, fmt.Sprint(args)+" output", res.stdout+res.stderr)
	}
	if b, err := os.ReadFile(deliver); err == nil {
		check(s, "the --deliver file", string(b))
	}
	for _, host := range []string{hostDell, hostLenovo, hostAppleAPI} {
		if len(v.requests(host)) == 0 {
			t.Errorf("the echo path for %s was never exercised", host)
		}
	}
	credFile := filepath.Join(s.home, "data", "credentials.toml")
	checkFiles(s, credFile)
	if info, err := os.Stat(credFile); err == nil && runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Errorf("credential store is mode %v", info.Mode().Perm())
	}

	m := newSandbox(t)
	_, menv := setup(m)
	for tool, args := range map[string]map[string]any{
		"lenovo_warranty":   {"Serial": "PF1ABCDE"},
		"dell_entitlements": {"servicetags": "1ABC234"},
		"apple_list":        {},
	} {
		frames, stderr := mcpCall(t, mcp, menv, tool, args)
		check(m, tool+" frames", frames)
		check(m, tool+" stderr", stderr)
	}
	checkFiles(m, filepath.Join(m.home, "data", "credentials.toml"))
}

// The Apple key: a file's contents are held at startup, and PEM text in the
// environment does not stop ordinary output.
func TestKitAppleKeyHeldBeforeOutput(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	keyPEM, der := appleKeyPEM(t)
	keyFile := s.writePrivate(t, "apple.p8", keyPEM)
	for _, enc := range []string{base64.StdEncoding.EncodeToString(der), hex.EncodeToString(der)} {
		res := run(t, cli, s.env("WARRANTY_APPLE_PRIVATE_KEY="+keyFile), enc)
		requireAbsent(t, "stdout", res.stdout, enc)
		requireAbsent(t, "stderr", res.stderr, enc)
		if res.stderr == "" && res.stdout == "" {
			t.Error("the run printed nothing, so this test proved nothing")
		}
	}
	// A vendor echo of the key's body from a key given inline.
	res := run(t, cli, s.env("WARRANTY_APPLE_PRIVATE_KEY="+keyPEM), "doctor", "--json")
	if !strings.Contains(res.stdout, "lenovo") {
		t.Errorf("PEM text in the environment withheld doctor's report: %q %q", res.stdout, res.stderr)
	}
}

// Ordinary words survive holding every vendor's credentials and minting Apple's
// token: the Apple client ID starts BUSINESSAPI., its token form says
// client_credentials and business.api, and the key file's path shares its
// directories with the mirror's. None of that may withhold a vendor's own words
// or the save.
func TestKitOrdinaryTextSurvivesTheCredentials(t *testing.T) {
	cli, _ := binaries(t)
	s := newSandbox(t)
	v := newVendors(t, s)
	env := allVendorEnv(t, s, v)

	res := run(t, cli, env, "lookup", "C02XG0FDH7JY", "--vendor", "apple", "--confirm", "--json")
	if res.code != 0 || !strings.Contains(res.stdout, "AppleCare+ for Business") {
		t.Errorf("an Apple coverage line was withheld (exit %d): %s %s", res.code, res.stdout, res.stderr)
	}
	res = run(t, cli, env, "lookup", "1ABC234", "--vendor", "dell", "--confirm", "--json")
	if res.code != 0 || !strings.Contains(res.stdout, "Next Business Day") {
		t.Errorf("a Dell coverage line was withheld (exit %d): %s %s", res.code, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, `"saved": true`) {
		t.Errorf("the lookup was not saved: %s %s", res.stdout, res.stderr)
	}

	v.apple = func(*http.Request) (int, string) {
		return 403, `{"errors":[{"status":"403","title":"Forbidden","detail":"invalid credentials"}]}`
	}
	res = run(t, cli, env, "apple", "list", "--json")
	if len(v.requests(hostAppleOAuth)) == 0 {
		t.Fatalf("Apple's token was never minted: %s %s", res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout+res.stderr, "credentials") {
		t.Errorf("after a mint the word credentials is withheld: %q %q", res.stdout, res.stderr)
	}
}

// Contract section 1, brief item 8(c): a saved profile value under a credential flag's
// name or under base-url is a source, every duplicate member included. Input: a store
// a person edited, then profile show. An ordinary value in the same store still prints.
func TestProfileStoreCredentialsAreHeld(t *testing.T) {
	cli, _ := binaries(t)
	const (
		first  = "profile-dup-9z7x5v-Qw3T"
		second = "profile-sec-4k2m8n-Rt6Y"
		inURL  = "profile-url-1p3s5u-Zx9V"
	)
	s := newSandbox(t)
	p := filepath.Join(s.home, "config", "profiles.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	store := `{"profiles":{"p":{"name":"p","description":"nightly","values":{"client-secret":"` + first + `","client-secret":"` + second +
		`","base-url":"https://gateway.example/v5?key=` + inURL + `","limit":"7"}}}}`
	if err := os.WriteFile(p, []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"profile", "show", "p", "--json", "--no-learn"}, {"--no-such-flag", first}, {"--no-such-flag", inURL}} {
		res := run(t, cli, s.env(), args...)
		for _, secret := range []string{first, second, inURL} {
			requireAbsent(t, fmt.Sprintf("%v", args), res.stdout+res.stderr, secret)
		}
	}
	// The store's other text stays usable: a credential value holds nothing else.
	if res := run(t, cli, s.env(), "profile", "show", "nightly", "--no-learn"); !strings.Contains(res.stdout+res.stderr, "nightly") {
		t.Errorf("a word from the store's description was withheld: %q %q", res.stdout, res.stderr)
	}
	if err := os.WriteFile(p, []byte(`{"profiles":{"p":{"name":"p","description":"nightly","values":{"limit":"7"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if res := run(t, cli, s.env(), "profile", "show", "p", "--json", "--no-learn"); !strings.Contains(res.stdout, "limit") {
		t.Errorf("an ordinary saved value was withheld: %q %q", res.stdout, res.stderr)
	}
}

// Contract section 1: no exemption for length or type. Input: a short password of
// punctuation alone inside a base URL, in a saved profile and in the config file,
// and the same in a [headers] value.
func TestShortPunctuationCredentialsAreHeld(t *testing.T) {
	cli, _ := binaries(t)
	const pass = "!~!~!~"
	s := newSandbox(t)
	if err := os.MkdirAll(filepath.Join(s.home, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.home, "config", "profiles.json"),
		[]byte(`{"profiles":{"demo":{"name":"demo","values":{"base-url":"https://user:`+pass+`@example.invalid"}}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	res := run(t, cli, s.env(), "profile", "show", "demo", "--json", "--no-learn")
	requireAbsent(t, "profile show", res.stdout+res.stderr, pass)
	for _, body := range []string{"base_url = \"https://user:" + pass + "@example.invalid\"\n", "[headers]\nX-Key = \"" + pass + "\"\n"} {
		c := s.writePrivate(t, "c.toml", body)
		res = run(t, cli, s.env(), "--config", c, "--no-such-flag", "x"+pass+"x")
		requireAbsent(t, body, res.stdout+res.stderr, pass)
	}
}

// Apple's Get Organization Devices pages with meta.paging.nextCursor. Input: that
// page over MCP apple_list. The result must offer the next page.
func TestAppleListOffersTheNextPage(t *testing.T) {
	_, mcp := binaries(t)
	s := newSandbox(t)
	v := newVendors(t, s)
	v.apple = func(r *http.Request) (int, string) {
		return 200, `{"data":[{"type":"orgDevices","id":"C02XG0FDH7JY","attributes":{"serialNumber":"C02XG0FDH7JY"}}],"links":{"self":"https://api-business.apple.com/v1/orgDevices","next":"https://api-business.apple.com/v1/orgDevices?cursor=pagetwo42"},"meta":{"paging":{"nextCursor":"pagetwo42","limit":1}}}`
	}
	frames, _ := mcpCall(t, mcp, allVendorEnv(t, s, v), "apple_list", map[string]any{"limit": 1})
	if !strings.Contains(frames, "next_cursor") {
		t.Errorf("apple_list dropped the next page: %s", frames)
	}
}
