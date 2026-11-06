// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: tests for ion_redact.go and ion_redact_transport.go.

package cli

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"td-synnex-pp-cli/internal/client"
	"td-synnex-pp-cli/internal/config"
)

const sentinel = "SENTINEL-Hunter2-9f3a-4417"

// secretBodies are the shapes ION documents carrying credentials, plus
// spelling and nesting variants of them.
var secretBodies = map[string]string{
	"customer user":        `{"customers":[{"name":"accounts/1/customers/2","user":{"userName":"u","password":"` + sentinel + `"}}]}`,
	"order attribute":      `{"orders":[{"orderItems":[{"attributes":[{"name":"password","value":"` + sentinel + `"}]}]}]}`,
	"attribute casing":     `{"attributes":[{"Name":"Admin_Password","Value":"` + sentinel + `"}]}`,
	"key/value attribute":  `{"options":[{"key":"tenantAdminPassword","value":"` + sentinel + `"}]}`,
	"provisioning":         `{"userCredentials":{"username":"a","password":"` + sentinel + `","loginUrl":"https://x"}}`,
	"azure info":           `{"azureCustomerInfo":"{\"UserCredentials\":{\"UserName\":\"admin\",\"Password\":\"` + sentinel + `\"}}"}`,
	"nested json string":   `{"note":"{\"inner\":\"{\\\"clientSecret\\\":\\\"` + sentinel + `\\\"}\"}"}`,
	"text assignment":      `{"comment":"login admin Password: ` + sentinel + `"}`,
	"snake and dash keys":  `{"client_secret":"` + sentinel + `","refresh-token":"` + sentinel + `","api.key":"` + sentinel + `"}`,
	"non-string secret":    `{"password":{"value":"` + sentinel + `"},"secret":12345}`,
	"not json":             `Password=` + sentinel,
	"plural key":           `{"passwords":["` + sentinel + `"]}`,
	"word not last":        `{"initialPasswordValue":"` + sentinel + `","secretAccessKey":"` + sentinel + `"}`,
	"free text is":         `{"notes":"Your temporary password is ` + sentinel + `"}`,
	"xml tag":              `{"partnerInfo":"<Password>` + sentinel + `</Password>"}`,
	"attr name/value alt":  `{"a":[{"attributeName":"tenantAdminPassword","attributeValue":"` + sentinel + `"}]}`,
	"attr values array":    `{"a":[{"name":"password","values":["` + sentinel + `"]}]}`,
	"url userinfo":         `{"link":"https://admin:` + sentinel + `@portal.example"}`,
	"query token":          `{"link":"https://portal.example/x?token=` + sentinel + `"}`,
	"bare pass key":        `{"tenantAdmin":{"user":"a","pass":"` + sentinel + `"}}`,
	"bearer text":          `{"h":"Authorization: Bearer ` + sentinel + `"}`,
	"compound short keys":  `{"provisioningInfo":{"adminUser":"a","adminPass":"` + sentinel + `","tempPwd":"` + sentinel + `","initialPin":"` + sentinel + `","admin_pw":"` + sentinel + `"}}`,
	"big attribute":        `{"attributes":[{"name":"tenantAdminPassword","value":"` + sentinel + `","displayName":"Admin password","required":true,"type":"string"}]}`,
	"csv secret column":    `{"results":"user,password\nadmin,` + sentinel + `\nops,` + sentinel + `"}`,
	"tsv secret column":    `{"results":"User\tAdmin Secret\nadmin\t` + sentinel + `"}`,
	"csv after blank line": `{"results":"\nuser,password\nadmin,` + sentinel + `"}`,
	"one-column csv":       `{"results":"password\n` + sentinel + `"}`,
	"xml across lines":     `{"results":"<Password>\n` + sentinel + `\n</Password>"}`,
	"csv cell text":        `{"results":"a,b\nnote,login password is ` + sentinel + `"}`,
	"credentials child":    `{"userCredentials":{"username":"a","pin":"` + sentinel + `","otp":{"code":"` + sentinel + `"}}}`,
	"encoded attr name":    `{"attributes":[{"name":"admin%50assword","value":"` + sentinel + `"}]}`,
	"html attr name":       `{"attributes":[{"name":"admin&#80;assword","value":"` + sentinel + `"}]}`,
	"nbsp attr name":       `{"attributes":[{"name":"API\u00a0key","value":"` + sentinel + `"}]}`,
	"pin first keys":       `{"provisioningInfo":{"pinCode":"` + sentinel + `","otpCode":"` + sentinel + `","pinNumber":"` + sentinel + `"}}`,
	"pin first attr":       `{"attributes":[{"name":"PIN code","value":"` + sentinel + `"},{"name":"PIN (4 digits)","value":"` + sentinel + `"}]}`,
	"encoded role key":     `{"attributes":[{"n%61me":"password","value":"` + sentinel + `"}]}`,
	"pin value attr":       `{"attributes":[{"name":"PIN value","value":"` + sentinel + `"}],"otpValue":"` + sentinel + `"}`,
	// round 3: equal comma counts made prose parse as a table and kept the
	// value on the line after its label.
	"label then value line": `{"actionDetails":"Login: admin@contoso.onmicrosoft.com, Password:\n` + sentinel + `, temporary"}`,
	"csv name/value rows":   `{"results":"name,value\ntenantAdminPassword,` + sentinel + `\nrenewal,auto"}`,
	"csv sep preamble":      `{"results":"sep=,\nuser,password\nadmin,` + sentinel + `"}`,
	"ragged csv pin header": `{"results":"Admin PIN,Customer\n` + sentinel + `,acme\nx,y,z"}`,
	"html-escaped tag":      `{"note":"&lt;Password&gt;` + sentinel + `&lt;/Password&gt;"}`,
	"percent-encoded tag":   `{"note":"%3CPassword%3E` + sentinel + `%3C%2FPassword%3E"}`,
	"capital Is":            `{"notes":"Password Is ` + sentinel + `"}`,
	"text body \\u tag":     `\u003cPassword\u003e` + sentinel,
}

func TestRedactSecretsJSONRemovesEverySentinel(t *testing.T) {
	for name, body := range secretBodies {
		out := string(redactSecretsJSON([]byte(body)))
		if strings.Contains(out, sentinel) || strings.Contains(out, "12345") {
			t.Errorf("%s: secret survived: %s", name, out)
		}
	}
}

func TestRedactSecretsJSONKeepsOrdinaryData(t *testing.T) {
	body := `{"nextPageToken":"abc","compassId":"c1","productName":"Keeper Password Manager: Business","sku":"1Password: Teams Starter","external_id":9007199254740993,"company":"AT&T <x>","options":[{"key":"autoRenew","value":"true"}],"username":"admin","product":{"name":"1PasswordBusiness-smp","displayName":"1Password Business","msrpValue":7.99,"costValue":6.5,"vendor":"1Password"}}`
	out := string(redactSecretsJSON([]byte(body)))
	for _, want := range []string{`"nextPageToken":"abc"`, `"compassId":"c1"`, `"Keeper Password Manager: Business"`, `"1Password: Teams Starter"`, `9007199254740993`, `"AT&T <x>"`, `"value":"true"`, `"username":"admin"`, `"msrpValue":7.99`, `"costValue":6.5`} {
		if !strings.Contains(out, want) {
			t.Errorf("ordinary data lost: want %s in %s", want, out)
		}
	}
}

func gz(t *testing.T, s string) []byte {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func deflateRaw(t *testing.T, s string) []byte {
	var b bytes.Buffer
	w, _ := flate.NewWriter(&b, flate.DefaultCompression)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func TestRedactingTransportJudgesEveryResponse(t *testing.T) {
	body := secretBodies["azure info"]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			_, _ = io.WriteString(w, `{"access_token":"acc-`+sentinel+`","refresh_token":"rot-1"}`)
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(gz(t, body))
		case "/deflate":
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(deflateRaw(t, body))
		case "/error":
			w.WriteHeader(400)
			_, _ = io.WriteString(w, secretBodies["provisioning"])
		default:
			_, _ = io.WriteString(w, body)
		}
	}))
	defer srv.Close()

	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL) // the host guard: a loopback mock needs the run's endpoint
	c := client.New(&config.Config{}, 0, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	if err := ApplyClientHooks(c); err != nil { // idempotent: no double wrap
		t.Fatal(err)
	}
	if _, ok := c.HTTPClient.Transport.(redactingTransport); !ok {
		t.Fatalf("client hook did not install the redacting transport: %T", c.HTTPClient.Transport)
	}
	if rt := c.HTTPClient.Transport.(redactingTransport); func() bool { _, nested := rt.base.(redactingTransport); return nested }() {
		t.Fatal("redacting transport wrapped twice")
	}
	get := func(path string, hdr map[string]string) string {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	for _, path := range []string{"/accounts/1/customers/2/cloudProviders/30", "/error"} {
		if out := get(path, nil); strings.Contains(out, sentinel) {
			t.Errorf("%s leaked: %s", path, out)
		}
	}
	// A caller-set Accept-Encoding turns off net/http's transparent gzip, so
	// the transport must inflate before judging.
	for _, path := range []string{"/gzip", "/deflate"} {
		out := get(path, map[string]string{"Accept-Encoding": "gzip, deflate"})
		if strings.Contains(out, sentinel) || !strings.Contains(out, "azureCustomerInfo") {
			t.Errorf("%s not judged after decoding: %q", path, out)
		}
	}
	// Only a request postIONForm marked passes unredacted; the same path reached
	// by an ordinary call (for example through a redirect) is judged (Codex
	// round 7, P1).
	if out := get("/oauth/token", nil); strings.Contains(out, sentinel) {
		t.Errorf("unmarked request to the token path passed unredacted: %s", out)
	}
	req, _ := http.NewRequestWithContext(context.WithValue(context.Background(), ionOAuthExchange{}, true), "POST", srv.URL+"/oauth/token", nil)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if b, _ := io.ReadAll(resp.Body); !strings.Contains(string(b), "acc-"+sentinel) {
		t.Errorf("token exchange must pass through untouched: %s", b)
	}
}

// TestRedactSecretsJSONWithholdsCSVWhole: text holding a credential anywhere is
// withheld whole, never cell by cell (Codex/adversarial round 3, P1: prose
// with equal comma counts parsed as a table kept the password on the line
// after its label).
func TestRedactSecretsJSONWithholdsCSVWhole(t *testing.T) {
	for _, body := range []string{
		"customer,sku,seats\nAcme,CFQ7TTC0LDPB:0001,10\nNote,password: " + sentinel + ",0",
		`{"results":"Customer,Product,Seats\nSilverLeaf,M365 Business Standard,10\nNote,password is ` + sentinel + `,0","resultCount":"3"}`,
	} {
		if out := string(redactSecretsJSON([]byte(body))); strings.Contains(out, sentinel) || strings.Contains(out, "Acme") || strings.Contains(out, "SilverLeaf") {
			t.Errorf("text with a secret was not withheld whole: %s", out)
		}
	}
}

// TestRedactorKeepsOrdinaryText is the over-redaction twin of the oracle:
// catalog names, SKUs and reports with no credential must come back unchanged.
func TestRedactorKeepsOrdinaryText(t *testing.T) {
	for _, s := range []string{
		"Microsoft 365 Business Premium", "LastPass Business", "1Password Teams", "Keeper Password Manager",
		"1Password: Teams", "CFQ7TTC0LDPB:0001", "Duo Essentials", "Azure plan", "Secret Renewal",
		"customer,sku,seats\nAcme,CFQ7TTC0LDPB:0001,10\nGlobex,1Password: Teams,5\n",

		"nextPageToken: abc, pageSize: 50",
	} {
		body := `{"productName":` + strconv.Quote(s) + `}`
		if out := string(redactSecretsJSON([]byte(body))); out != body {
			t.Errorf("ordinary text rewritten: %s -> %s", body, out)
		}
	}
}

func TestRedactingTransportRefusesPlainHTTPOffLoopback(t *testing.T) {
	rt := redactingTransport{base: http.DefaultTransport}
	for _, u := range []string{"http://ion.tdsynnex.com/api/v3/accounts/1/customers", "http://10.0.0.5/oauth/token", "http://example.com/x"} {
		req, _ := http.NewRequest("GET", u, nil)
		if _, err := rt.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "refusing plain HTTP") {
			t.Errorf("%s: plain HTTP was not refused: %v", u, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "{}") }))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL) // the host guard: a loopback mock needs the run's endpoint
	req, _ := http.NewRequest("GET", srv.URL+"/x", nil)
	if resp, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("loopback test server refused: %v", err)
	} else {
		resp.Body.Close()
	}
}

func TestRedactTextBodies(t *testing.T) {
	for _, body := range []string{"\nuser,password\nadmin," + sentinel, "password\n" + sentinel, "<Password>\n" + sentinel + "\n</Password>", "user;Admin Secret\nx;" + sentinel} {
		if out := string(redactSecretsJSON([]byte(body))); strings.Contains(out, sentinel) {
			t.Errorf("text body leaked: %q -> %q", body, out)
		}
	}
	plain := "customer,sku,seats\nAcme,CFQ7TTC0LDPB:0001,10\n"
	if out := string(redactSecretsJSON([]byte(plain))); out != plain {
		t.Errorf("CSV without secrets was rewritten: %q", out)
	}
}

// TestRedactorOracle crosses secret-name spellings with every container shape
// and encoding the redactor claims to handle; no combination may leak.
func TestRedactorOracle(t *testing.T) {
	keys := []string{"password", "Password", "PASSWORD", "admin_password", "adminPassword", "AdminPass", "temp-pwd",
		"client.secret", "ApiKey", "access_token", "refreshToken", "Passcode", "tenant admin password"}
	enc := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	n := 0
	for _, k := range keys {
		s := fmt.Sprintf("SENT%03dX", n)
		n++
		shapes := []any{
			map[string]any{k: s},
			map[string]any{"wrap": map[string]any{"deeper": []any{map[string]any{k: s}}}},
			[]any{map[string]any{"name": k, "value": s}},
			[]any{map[string]any{"Key": k, "Value": s, "displayName": "x", "required": true, "type": "string"}},
			[]any{map[string]any{"attributeName": k, "attributeValue": s}},
			[]any{map[string]any{"name": k, "values": []any{s}}},
			map[string]any{"s": enc(map[string]any{k: s})},
			map[string]any{"s": enc(map[string]any{"inner": enc(map[string]any{k: s})})},
			map[string]any{"csv": "id," + k + "\n1," + s},
			map[string]any{"csv": "\nid\t" + k + "\n1\t" + s},
			map[string]any{"csv": "id;" + k + "\n1;" + s},
			map[string]any{"csv": k + "\n" + s},
			map[string]any{"t": k + ": " + s},
			map[string]any{"t": k + "=" + s},
			map[string]any{"t": "<" + strings.ReplaceAll(k, " ", "") + ">" + s + "</" + strings.ReplaceAll(k, " ", "") + ">"},
			map[string]any{"t": "<" + strings.ReplaceAll(k, " ", "") + ">\n" + s + "\n</" + strings.ReplaceAll(k, " ", "") + ">"},
			map[string]any{"t": "Login: a@b.example, " + k + ":\n" + s + ", temporary"},
			map[string]any{"t": strings.ToUpper(k[:1]) + k[1:] + " Is " + s},
			map[string]any{"csv": "name,value\n" + k + "," + s + "\nx,y"},
			map[string]any{"csv": "sep=;\nid;" + k + "\n1;" + s},
			map[string]any{"t": "&lt;" + strings.ReplaceAll(k, " ", "") + "&gt;" + s},
			map[string]any{"t": "%3C" + strings.ReplaceAll(k, " ", "") + "%3E" + s},
			map[string]any{"t": "%253C" + strings.ReplaceAll(k, " ", "") + "%253E" + s},
		}
		for i, shape := range shapes {
			body := enc(shape)
			if out := string(redactSecretsJSON([]byte(body))); strings.Contains(out, s) {
				t.Errorf("key %q shape %d leaked: %s -> %s", k, i, body, out)
			}
		}
	}
}

// TestRedactorLabelShapes holds the round 4-5 review shapes. Free-text leaks
// (key "notes") must be withheld; free text and name or ID fields with no
// credential must come back unchanged.
func TestRedactorLabelShapes(t *testing.T) {
	leak := []string{"API Key: S1", "Secret Key: S1", "Access Key: S1", "Private Key = S1", "Client secret (value): S1",
		"Temporary password for jdoe@contoso.com: S1", "<b>Password</b>: S1", "**Password**: S1", "Password&nbsp;: S1", "Password\u00a0: S1",
		"https://S1@example.com/", "Password - S1", "Password -> S1", "New Password -- S1", "secret_value -> S1", "Passwort: S1", "Kennwort: S1",
		"Contraseña: S1", "Contrase%C3%B1a: S1", "Mot de passe : S1", "Pass Phrase: S1", "Pass Code: S1", "Pass word: S1", "PIN #: S1", "PIN number: S1",
		"Recovery code: S1", "Verification code: S1", "AccountName=x;AccountKey=S1", "https://b.example/o?X-Amz-Signature=S1",
		"Password (temporary): S1", "Password Is S1", "Your temporary password is S1", "Login: admin@contoso.onmicrosoft.com, Password:\nS1, temporary",
		"name,value\ntenant admin password,S1\nx,y", "Admin PIN,Customer\nS1,acme\nx,y,z",
		// round 5
		"Password changed to: S1", "Password reset to: S1", "Temporary password (valid for 24 hours): S1", "Password (case sensitive): S1",
		"Admin password (do not share): S1", "Global admin password (Contoso): S1", "API key (production): S1", "<b>tenantAdminPassword</b>: S1",
		"`adminPassword`: S1", "tenantAdminPassword&nbsp;: S1", "Password for admin@contosoholdingsgroup.onmicrosoft.com: S1",
		"Password\uff1a S1", "Password\u00a0-\u00a0S1", "Password\u2013S1", "name,value\rpassword,S1", "user,password\rjdoe,S1",
		"admin:S1@mail.contoso.com", "Bearer S1abc12", "Customer,Notes\nAcme,Password changed to: S1\n",
		// round 6
		"Password 2: S1", "Recovery code 1: S1", "PIN 1: S1", "Activation code 3: S1", "Backup code 2 - S1", "Password 1 - S1", "PIN 2 is S1",
		"Password version 2: S1", "Password-S1", "admin:S1@host", "Hi team,\nThe password we set yesterday, after the reset, is S1\nThanks",
		"Hi,\nAdmin password (Contoso), as discussed: S1",
		// round 7
		"API&nbsp;key: S1", "API\u00a0key: S1", "API  key: S1", "API\tkey: S1", "Product&nbsp;key: S1", "Recovery&nbsp;code: S1",
		"The admin password&nbsp;is S1", "Password\u2212 S1", "Password \u2010 S1", "Password \u21d2 S1", "Password\ufe55 S1", "Password\u2236 S1",
		"admin:S1:rest@host", "%2525252570assword=S1", "%25252525252525252570assword=S1",
		"Pass\u200bword: S1", "\uff30\uff41\uff53\uff53\uff57\uff4f\uff52\uff44: S1", "Password\n: S1", "Pass\u00adword: S1",
		// round 8
		"<contrase\u00f1a>S1</contrase\u00f1a>", "login=admin:S1@host.example", "Password \u27a1 S1", "Password \u2b95 S1", "Password \u00bb S1",
		// round 9
		"Password changed -S1", "SIP trunk sip:trunkuser:S1@sip.provider.com", "tel:admin:S1@pbx", "Bearer: S1abc", "Password \U0001F449 S1",
		"Password\u02f8 S1", "Password\ua78a S1", "Password\uff05\uff13\uff21 S1", "User,Password (temp)\r\njdoe@contoso.com,S1\r\n",
		"Login,Password*\r\nadmin,S1\r\n", "Usuario,Contrase\u00f1a\r\nadmin@contoso.com,S1\r\n", "name,value\ncontrase\u00f1a,S1",
		"\ufeff{\"adminPassword\": \"S1\"}",
		// round 10
		"SMTP relay login svc-mail:S1@exch01.contoso.local", "it-contact:S1@fileserver", "scan-to:S1@printer01", "1234:S1@host.example",
		"Customer,Temporary password for tenant admin\nAcme,S1", "New starters\r\nUser,Initial password (change on first login)\r\njdoe@x.com,S1\r\n",
		"UPN\tPassword set by MSP tech\r\njdoe\tS1\r\n",
		// round 11
		"New starters, March 2026\r\nUser,Initial password (change on first login)\r\njdoe@x.com,S1\r\n", "Contoso | New starters\r\nUser|Password set by tech\r\njdoe|S1\r\n",
		"User,Pwd (temp)\r\njdoe,S1\r\n", "User,PW (temp)\r\njdoe,S1\r\n", "User,Pass (temp)\r\njdoe,S1\r\n", "Staff,Door PIN set by tech\r\njdoe,S1\r\n",
		"User,PINs\r\njdoe,S1\r\n", "Usuario,Clave\r\nadmin,S1\r\n", "Front door alarm PIN -4711S1, changed by tech", "Temp password -9xK!S1 (expires Friday)",
		"Password -4711S1", "admin:S1@[2001:db8::1]", "adminpass=S1", "rootpw=S1", "wifipass: S1",
		// round 12
		"\u7528\u6237:S1!@host.example", "Front door alarm PIN -4711, S1"}
	keepText := []string{"CFQ7TTC0LHPW:0001:P1M:M", "CFQ7TTC0LPIN:0001", "CFQ7TTC0L1PW:0001:P1Y:M", "Duo Essentials",
		"Microsoft 365 Business Basic - Monthly", "Postal Code: 12345", "Discount Code: SAVE10", "nextPageToken: abc, pageSize: 50"}
	keepName := []string{"Keeper Password Manager: Business", "Keystone Pass - Holdings", "Snoqualmie Pass - Ski Area", "Duo Hardware Token - D-100",
		"ACRN:Tier9:PW:M", "Secretary: Jane", "Password reset: done", "Token Industries: Annual", "Proton Pass: Business", "NordPass: Teams",
		"LastPass: Business", "Compass Group - Charlotte", "Encompass Health \u2013 Birmingham", "Crumbl Cookie - Dallas", "OTP Bank - US Branch",
		"100 Pin Oak Dr - Suite 4", "MTA6MTA=", "CFQ7TTC0L1PW:0001:P1Y:M", "KeeperPasswordManager-smp:0001", "DelineaSecretServer-smp:pro",
		"https://passwordstate.contoso.com:9119/", "https://secret-server.contoso.com:443/SecretServer", "Order 10:30@Contoso", "Pinnacle Health - North",
		"Token Industries, Inc.", "Secret Garden Florist, LLC", "Azure Key Vault Secrets, Standard", "LastPass Business", "NordPass, Business"}
	for _, s := range leak {
		if out := string(redactSecretsJSON([]byte(`{"notes":` + strconv.Quote(s) + `}`))); strings.Contains(out, "S1") {
			t.Errorf("LEAK %q", s)
		}
	}
	// The free-text rule withholds any mention of a credential word, so these
	// notes and report CSVs are withheld by design (round 12: the separator
	// grammar never converged); as names they survive (keepName below).
	for _, s := range []string{"Token Industries, Inc.", "Keeper Password Manager, Business, 1 year", "Passwordless sign-in, Windows Hello",
		"Meter,Cost\nAzure Key Vault Secrets,0.03\n", "Date,MeterName,Quantity\n2026-10-01,Text Input Tokens,80\n", "LastPass Business",
		"Front door alarm PIN -4711, changed by tech", "tenantAdminPassword-Hunter2!", "Customer,Detail,Setting\nAcme,Region,EU\nAcme,Contrase\u00f1a,Hunter2!"} {
		if out := string(redactSecretsJSON([]byte(`{"notes":` + strconv.Quote(s) + `}`))); out != `{"notes":"[redacted]"}` {
			t.Errorf("free-text mention kept: %q -> %s", s, out)
		}
	}
	// Name fields are judged narrowly but still catch a labelled credential.
	for _, s := range []string{"Password: S1", "Password:S1", "API Key: S1", "tenantAdminPassword=S1", "adminPass=S1", "initialPasswordValue=S1",
		"tempPwd=S1",
		"Temporary password for jdoe@contoso.com: S1", "https://S1@example.com/", "admin:S1@mail.contoso.com", "<Password>S1</Password>"} {
		if out := string(redactSecretsJSON([]byte(`{"displayName":` + strconv.Quote(s) + `}`))); strings.Contains(out, "S1") {
			t.Errorf("LEAK displayName %q", s)
		}
	}
	keepText = append(keepText, "Write to mailto:jdoe@contoso.com for access", "PINNACLE BANK - Microsoft 365 E3 annual",
		"PING IDENTITY CORP: renewal due 2026-11-01", "Renewal for PINE VALLEY SCHOOLS is due in March", "PinPoint Analytics - renewal",
		"OTPBank - US Branch", "<td>Email:jdoe@contoso.com</td>", "Compass Group - renewal in March, 40 seats", "Encompass Health: renewal",
		"Overpass Networks - 10 seats", "Chopin Music School - renewal", "Bypass approved - ticket 4411", "Meeting 10:30@Contoso",
		"Customer,Product,Qty\nPinnock & Sons Ltd,Acronis Cyber Protect Cloud - Per Workload,5\nPinnow Electric,Duo,3\n",
		"Product,SKU,Qty,Credit\nAcronis Cyber Protect,ACR,5, -12.50\n",
		"\"Customer Name\",\"Street\",\"City\",\"Product\"\r\n\"Contoso Ltd\",\"123 PINE ST\",\"Seattle\",\"Acronis Cyber Protect Cloud - Per Workload\"\r\n")
	for key, list := range map[string][]string{"notes": keepText, "displayName": keepName, "customerName": keepName, "mfgPartNumber": keepName,
		"customerOrganization": keepName, "street": keepName} {
		for _, s := range list {
			b := `{"` + key + `":` + strconv.Quote(s) + `}`
			if out := string(redactSecretsJSON([]byte(b))); out != b {
				t.Errorf("OVER %s %q", key, s)
			}
		}
	}
}

// TestRedactorKeepsLeadingJSONTokens: a string or body that starts with a JSON
// value followed by text is text, not one JSON value; it must come back whole
// (adversarial round 8, P1: a quoted CSV header cut the report to its first cell).
func TestRedactorKeepsLeadingJSONTokens(t *testing.T) {
	for _, v := range []string{"\"Customer Name\",\"SKU\",\"Quantity\"\r\n\"Contoso Ltd\",\"CFQ7TTC0LH18\",5\r\n",
		"\"Ma & Pa\" Hardware LLC", "[1] Renewal pending, call customer", "[2026] Q1 true-up", "\"Best value\" bundle for SMB", "123,Contoso,5"} {
		body := `{"results":` + strconv.Quote(v) + `}`
		if out := string(redactSecretsJSON([]byte(body))); out != body {
			t.Errorf("rewritten: %s -> %s", body, out)
		}
		if out := string(redactSecretsJSON([]byte(v))); out != v {
			t.Errorf("raw body rewritten: %q -> %q", v, out)
		}
	}
}

// TestRedactorRound9Structural: percent-encoded JSON in a name field is judged
// decoded, and PIN/OTP-led metadata keys that are not credentials survive.
func TestRedactorRound9Structural(t *testing.T) {
	enc := `{"displayName":"%7B%22key%22%3A%22clientSecret%22%2C%22data%22%3A%22` + sentinel + `%22%7D"}`
	if out := string(redactSecretsJSON([]byte(enc))); strings.Contains(out, sentinel) {
		t.Errorf("encoded JSON pair leaked: %s", out)
	}
	keep := `{"attributes":[{"name":"pinLength","value":"6"},{"name":"PINNACLE BANK","value":"Contoso"}],"otpEnabled":true,"pinType":"numeric"}`
	if out := string(redactSecretsJSON([]byte(keep))); out != keep {
		t.Errorf("non-credential PIN/OTP keys rewritten: %s", out)
	}
}

// TestRedactorRound10Keys: OTP/PIN credential keys named first stay secret,
// ordinary words that start with pin do not.
func TestRedactorRound10Keys(t *testing.T) {
	for _, k := range []string{"otpSeed", "otpKey", "otpUri", "pinHash", "pinBlock", "pinCode", "otpValue", "PIN (4 digits)"} {
		if !isSecretKey(k) {
			t.Errorf("%q should be a secret key", k)
		}
	}
	for _, k := range []string{"Pinnock", "Pinnow", "Compass", "Bypass", "pinType", "otpEnabled", "PINNACLE"} {
		if isSecretKey(k) {
			t.Errorf("%q should not be a secret key", k)
		}
	}
}

// TestRedactorAddressLabelsAreLinear: a short note full of address labels must
// be judged quickly (adversarial round 11, P1: a recursive label check made 18
// "Email:" contacts hang the CLI and the MCP server).
func TestRedactorAddressLabelsAreLinear(t *testing.T) {
	note := strings.Repeat("Pat Email:pat@contoso.com\r\n", 200) + strings.Repeat(`<a href="mailto:user@contoso.com">x</a> `, 200)
	start := time.Now()
	if hasSecretText(note, false) {
		t.Fatal("contacts note withheld")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("contacts note took %v", d)
	}
}

// TestRedactorDecodeDepthIsLinear: a small note with a deep percent escape and
// text canonicalText changes must not re-decode per form (adversarial round
// 12, P1: an 8 KB note took 20 s).
func TestRedactorDecodeDepthIsLinear(t *testing.T) {
	note := "%" + strings.Repeat("25", 62) + "41 " + strings.Repeat("hello  world\u00a0", 600)
	start := time.Now()
	_ = redactSecretsJSON([]byte(`{"notes":` + strconv.Quote(note) + `}`))
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("deep escape note took %v", d)
	}
}

// TestRedactingTransportJudgesHeaders: a redirect Location carrying a
// password reaches errors and retry warnings, so headers are judged too
// (Codex round 12, P1).
func TestRedactingTransportJudgesHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "http://127.0.0.1:1/?password="+sentinel)
		w.Header().Set("X-Request-Id", "req-1")
		w.WriteHeader(http.StatusFound)
	}))
	defer srv.Close()
	t.Setenv("TD_SYNNEX_BASE_URL", srv.URL) // the host guard: a loopback mock needs the run's endpoint
	rt := redactingTransport{base: http.DefaultTransport}
	req, _ := http.NewRequest("GET", srv.URL+"/accounts/1/customers", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if loc := resp.Header.Get("Location"); strings.Contains(loc, sentinel) || resp.Header.Get("X-Request-Id") != "req-1" {
		t.Fatalf("headers: Location=%q X-Request-Id=%q", loc, resp.Header.Get("X-Request-Id"))
	}
}

// TestRedactorJudgesPropertyNames: a JSON property name that is not a plain
// field name is text and can hold the credential itself (Codex review round
// 13, P1: {"password: SENTINEL":"invalid"} kept the key).
func TestRedactorJudgesPropertyNames(t *testing.T) {
	const sentinel = "SENTINEL-Hunter2-9f3a-4417"
	for _, in := range []string{
		`{"details":{"password: ` + sentinel + `":"invalid"}}`,
		`{"details":{"Admin Password ` + sentinel + `":"x"}}`,
		`{"details":{"Contraseña: ` + sentinel + `":"x"}}`,
		`{"details":{"password%3A%20` + sentinel + `":"x"}}`,
		`{"credentials":{"username":"ada","password: ` + sentinel + `":"x"}}`,
		`{"note":"{\"password: ` + sentinel + `\":\"x\"}"}`,
	} {
		out := string(redactSecretsJSON([]byte(in)))
		if strings.Contains(out, sentinel) || !strings.Contains(out, "[redacted]") {
			t.Errorf("%s -> %s", in, out)
		}
	}
	// A plain field name is schema: it keeps its name and loses its value.
	out := string(redactSecretsJSON([]byte(`{"azureCustomerInfo":"{\"adminPassword\":\"` + sentinel + `\"}","tenantDomain":"acme.onmicrosoft.com"}`)))
	if strings.Contains(out, sentinel) || !strings.Contains(out, "azureCustomerInfo") || !strings.Contains(out, "acme.onmicrosoft.com") {
		t.Fatalf("plain field names: %s", out)
	}
}

// TestRedactorDecodesSurrogatePairs: strconv.Unquote refused each half of a
// 𝐩 pair (a mathematical bold p, NFKC p), so an encoded label
// passed unjudged (Codex review round 14, P1).
func TestRedactorDecodesSurrogatePairs(t *testing.T) {
	const sentinel = "SENTINEL-Hunter2-9f3a-4417"
	for _, in := range []string{
		`𝐩assword: ` + sentinel,
		`\\uD835\\uDC29assword: ` + sentinel,
		`\uD835assword: ` + sentinel + ` lone high half`,
		`\x70assword: ` + sentinel,
		`password%3A ` + sentinel,
	} {
		if !hasSecretText(in, false) {
			t.Errorf("not judged: %q", in)
		}
		if out := string(redactSecretsJSON([]byte(`{"notes":` + strconv.Quote(in) + `}`))); strings.Contains(out, sentinel) {
			t.Errorf("%q -> %s", in, out)
		}
	}
	// Precision: an escaped non-BMP rune that is not a credential label stays
	// ordinary text (only the pair decoder, not the fail-closed backstop,
	// can tell these apart).
	for _, in := range []string{"Order \U0001D4C5 confirmed for 12 seats", `Great order \uD83D\uDE00 thanks`, `{"note":"Great order \\uD83D\\uDE00 thanks"}`} {
		if hasSecretText(in, false) {
			t.Errorf("ordinary non-BMP text judged secret: %q", in)
		}
	}
}

// TestRedactorBoundsDecodeDepth: every decoded form is judged over the full
// string, so the depth is bounded at textDecodeLimit and a deeper string is
// withheld (adversarial round 13, P1: 8 KB with two 62-level chains took
// 2.2 s, 15.7 KB took 5 s).
func TestRedactorBoundsDecodeDepth(t *testing.T) {
	note := "%" + strings.Repeat("25", 62) + "41 " + "％" + strings.Repeat("２５", 62) + "４１ " + strings.Repeat("a=b ", 3900)
	if len(note) < 15000 {
		t.Fatalf("fixture too small: %d", len(note))
	}
	start := time.Now()
	out := string(redactSecretsJSON([]byte(`{"notes":` + strconv.Quote(note) + `}`)))
	// ponytail: wall-clock ceiling. Bounded runs take 0.5-1.2 s on a laptop and
	// the unbounded decoder 6.5 s, so 3 s still catches the regression without
	// failing on a loaded CI runner (adversarial round 15, P2: 1 s was flaky).
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("deep chains took %v", d)
	}
	if strings.Contains(out, "a=b") {
		t.Fatal("a string nested past textDecodeLimit must be withheld")
	}
	// Realistic depth decodes and is judged, not withheld.
	shallow := "Discount %2525252541 applies; see https://ion.tdsynnex.com/help%3Fq%3D1"
	if out := string(redactSecretsJSON([]byte(`{"notes":` + strconv.Quote(shallow) + `}`))); !strings.Contains(out, "Discount") {
		t.Fatalf("a four-level escape was withheld: %s", out)
	}
	if !hasSecretText("%"+strings.Repeat("25", 9)+"41", false) {
		t.Fatal("nine nested levels must fail closed")
	}
}

// TestRedactorKnowsListedTranslations: governance.md names the translations
// the mention rule covers; each must withhold (adversarial round 13, P1:
// German, French and Spanish labels reached stdout, data.db, the cache and
// MCP results).
func TestRedactorKnowsListedTranslations(t *testing.T) {
	const s = "SENTINEL-Hunter2-9f3a-4417"
	for _, in := range []string{
		"Zugangsdaten: admin / " + s, "Passwörter: " + s, "Kennwörter: " + s, "API-Schlüssel: " + s, "Anmeldedaten: " + s, "Geheimnis: " + s,
		"identifiants : admin / " + s, "mdp : " + s, "clé API : " + s, "jeton: " + s, "code d'accès: " + s,
		"credenciales: admin / " + s, "código de acceso: " + s, "llave API: " + s, "claves: " + s,
		"senhas: " + s, "credenciais: " + s, "chave API: " + s, "segredo: " + s,
		"wachtwoorden: " + s, "credenziali: " + s, "chiave API: " + s, "segreto: " + s,
		"creds: admin / " + s, "p/w: " + s, "pswd: " + s, "passkey: " + s, "Door code: " + s, "access code " + s,
		"Pas͏sword: " + s, "Pas︎sword: " + s,
	} {
		if !hasSecretText(in, false) {
			t.Errorf("not judged: %q", in)
		}
	}
	for _, k := range []string{"Zugangsdaten", "apiSchlüssel", "identifiants", "mdp", "credenciales", "llaveApi", "segredo", "creds"} {
		if !isSecretKey(k) {
			t.Errorf("field name not secret: %q", k)
		}
	}
	for _, in := range []string{"Kunde hat Rabatt im Dezember", "Credenza delivered", "PSK-2200 switch shipped"} {
		if hasSecretText(in, false) {
			t.Errorf("ordinary text judged secret: %q", in)
		}
	}
}

// TestTransportRefusesOffHostWithoutEnv: the access token goes only to
// ion.tdsynnex.com unless TD_SYNNEX_BASE_URL names the endpoint for this
// run; a base_url from a config file is not enough (adversarial round 13,
// P1: governance.md said so, auth mint enforced it, API calls did not).
func TestTransportRefusesOffHostWithoutEnv(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++; _, _ = io.WriteString(w, `{"ok":true}`) }))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: "https://ion.example.invalid/api/v3"}, 0, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TD_SYNNEX_BASE_URL", "")
	// A remote host from a config file is refused before any dial.
	req, _ := http.NewRequest("GET", "https://ion.example.invalid/api/v3/accounts/1/customers", nil)
	if _, err := c.HTTPClient.Do(req); err == nil || !strings.Contains(err.Error(), "TD_SYNNEX_BASE_URL") {
		t.Fatalf("file base_url on a remote host must be refused: err=%v", err)
	}
	// A loopback test server is allowed (doctor and stubs), with or without the env.
	req, _ = http.NewRequest("GET", srv.URL+"/api/v3/accounts/1/customers", nil)
	resp, err := c.HTTPClient.Do(req)
	if err != nil || resp.StatusCode != 200 || hits != 1 {
		t.Fatalf("loopback endpoint must be allowed: err=%v hits=%d", err, hits)
	}
	resp.Body.Close()
	if !ionHostAllowed("ion.tdsynnex.com") || ionHostAllowed("ion.tdsynnex.com.evil.example") || ionHostAllowed("10.0.0.1") {
		t.Fatal("only ion.tdsynnex.com and loopback are allowed without TD_SYNNEX_BASE_URL")
	}
	t.Setenv("TD_SYNNEX_BASE_URL", "https://proxy.example.invalid")
	if !ionHostAllowed("proxy.example.invalid") {
		t.Fatal("TD_SYNNEX_BASE_URL names the endpoint for this run")
	}
}

// TestValidationErrorSiblingsBlanked: a 400 that names a secret field echoes
// the rejected value under description or rejectedValue, so every string
// sibling of a secret-named attribute goes (adversarial round 13, P2).
func TestValidationErrorSiblingsBlanked(t *testing.T) {
	const s = "SENTINEL-Hunter2-9f3a-4417"
	in := `{"error":"bad","details":[{"@type":"BadRequest","fieldViolations":[{"field":"orderItems[0].attributes.tenantAdminPassword","description":"value '` + s + `-FV' does not meet complexity"}]},{"field":"tenantAdminPassword","rejectedValue":"` + s + `-RV","actual":"` + s + `-AC"}]}`
	out := string(redactSecretsJSON([]byte(in)))
	if strings.Contains(out, s) || !strings.Contains(out, "BadRequest") {
		t.Fatalf("%s", out)
	}
}

// TestTransportErrorsNamedByKind: an ordinary API transport error carried
// the server's bytes (net/http quotes a malformed header line), which
// maskError cannot know (Codex round 15, P1).
func TestTransportErrorsNamedByKind(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 4096)
			_, _ = conn.Read(buf)
			_, _ = conn.Write([]byte("HTTP/1.1 400 Bad Request\r\nBad password " + sentinel + "\r\n\r\n"))
			conn.Close()
		}
	}()
	t.Setenv("TD_SYNNEX_BASE_URL", "http://"+ln.Addr().String())
	c := client.New(&config.Config{}, 0, 0)
	if err := ApplyClientHooks(c); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", "http://"+ln.Addr().String()+"/accounts/1/customers", nil)
	_, err = c.HTTPClient.Do(req)
	if err == nil || strings.Contains(err.Error(), sentinel) || !strings.Contains(err.Error(), "transport failure") {
		t.Fatalf("err=%v", err)
	}
	var kind transportKindError
	if !errors.As(err, &kind) || kind.err == nil {
		t.Fatalf("the original error must unwrap for retry checks: %T", err)
	}
}

// TestBareUserinfoWithCombiningMarks (Codex round 15, P1).
func TestBareUserinfoWithCombiningMarks(t *testing.T) {
	for _, in := range []string{"कि:" + sentinel + "@host", "कि:" + sentinel + "@ion.example.com", "admin:" + sentinel + "@host"} {
		if !hasSecretText(in, false) || !hasSecretText(in, true) {
			t.Errorf("not judged: %q", in)
		}
	}
}
