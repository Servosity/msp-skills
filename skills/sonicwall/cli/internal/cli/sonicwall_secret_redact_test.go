// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"sonicwall-pp-cli/internal/posture"
)

const redactSentinel = "s3ntinel-SECRET-value"

func TestSonicOSRedactTransport(t *testing.T) {
	users := `{"user":{"local":{"user":[{"name":"alice","password":"` + redactSentinel + `","one_time_password":{"totp":true}}]}}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/firewall/user/local/users":
			_, _ = io.WriteString(w, users)
		case "/firewall/reporting/status/system":
			w.Header().Set("Content-Encoding", "gzip")
			zw := gzip.NewWriter(w)
			_, _ = io.WriteString(zw, `{"assign_token":"`+redactSentinel+`","authentication_code":"`+redactSentinel+`","registration_code":"`+redactSentinel+`","firmware_version":"7.1.1"}`)
			_ = zw.Close()
		case "/api/hgms/get-firewalls":
			_, _ = io.WriteString(w, `{"content":{"rows":[{"serial":"00000000000A","api_key":"`+redactSentinel+`"}]}}`)
		case "/api/product/serviceInfo":
			_, _ = io.WriteString(w, `{"serviceTypeList":[{"serviceList":[{"serviceName":"GAV","tcpassword":"`+redactSentinel+`"}]}]}`)
		case "/api/users/productlist-customer":
			_, _ = io.WriteString(w, `{"content":{"prodList":[{"serialNumber":"00000000000A","authCode":"`+redactSentinel+`"}]}}`)
		case "/api/generate-cscAccessCode":
			_, _ = io.WriteString(w, `{"accessCode":"keep-me","token":"keep-me"}`)
		case "/auth/sso":
			_, _ = io.WriteString(w, `{"status":{"success":true,"info":[{"message":"keep-me"}]},"token":"keep-me"}`)
		case "/devices/00000000000A":
			_, _ = io.WriteString(w, `{"device_info":{"username":"admin","password":"`+redactSentinel+`","authCode":"`+redactSentinel+`"}}`)
		case "/firewall/snmp/base":
			_, _ = io.WriteString(w, `{"snmp":{"get_community_name":"`+redactSentinel+`","trap_community_name":"`+redactSentinel+`"}}`)
		case "/api/user/export-activity-log-details":
			_, _ = io.WriteString(w, `{"actiondetails":[{"activationKey":"`+redactSentinel+`"}]}`)
		case "/firewall/administration/global":
			_, _ = io.WriteString(w, `{"administration":{"password":{"minimum_length":8,"complexity":{"type":"alphanumeric"}},"admin":{"one_time_password":{"totp":true}}},"sonicos_api":{"bearer_token":false}}`)
		case "/tenants/bom":
			_, _ = io.WriteString(w, "\xEF\xBB\xBF"+`{"name":"t1","password":"`+redactSentinel+`"}`)
		case "/tenants/concat":
			_, _ = io.WriteString(w, `{"id":"t1"}`+"\n"+`{"id":"t2","password":"`+redactSentinel+`"}`)
		case "/tenants/html404":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `<html><input name="password"></html>`)
		case "/zeroTouch/devices/status":
			_, _ = io.WriteString(w, `[{"serialNumber":"00000000000A","authCode":"`+redactSentinel+`","regCode":"`+redactSentinel+`"}]`)
		case "/firewall/license/registration-code":
			_, _ = io.WriteString(w, `{"license":{"register_code":"`+redactSentinel+`"},"virtual_assist":{"assistance_code":"`+redactSentinel+`"}}`)
		case "/users/active":
			_, _ = io.WriteString(w, `[{"username":"admin","sessionID":"`+redactSentinel+`","msgCode":"OK","product_code":"TZ370"}]`)
		case "/firewall/odd":
			_, _ = io.WriteString(w, `password: "`+redactSentinel+`" "password"`)
		default:
			_, _ = io.WriteString(w, `{"name":"plain","serialNumber":"00000000000A"}`)
		}
	}))
	defer srv.Close()
	hc := &http.Client{Transport: nsmEnvelopeTransport{base: sonicOSRedactTransport{base: http.DefaultTransport}}}
	get := func(path string, gz bool) (string, error) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("X-DEVICE-ID", "00000000000A")
		if gz {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		resp, err := hc.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b), nil
	}

	body, err := get("/firewall/user/local/users", false)
	if err != nil || strings.Contains(body, redactSentinel) || !strings.Contains(body, `"totp":true`) {
		t.Fatalf("local users: err %v body %q, want password redacted and one_time_password intact", err, body)
	}
	body, err = get("/firewall/reporting/status/system", true)
	if err != nil || strings.Contains(body, redactSentinel) || !strings.Contains(body, "7.1.1") {
		t.Fatalf("gzip system status: err %v body %q, want assign_token and both codes redacted", err, body)
	}
	body, err = get("/firewall/snmp/base", false)
	if err != nil || strings.Contains(body, redactSentinel) {
		t.Fatalf("snmp: err %v body %q, want community names redacted", err, body)
	}
	// Non-secret objects and flags that share a secret name are kept.
	body, err = get("/firewall/administration/global", false)
	if err != nil || !strings.Contains(body, `"minimum_length":8`) || !strings.Contains(body, `"bearer_token":false`) || !strings.Contains(body, `"totp":true`) {
		t.Fatalf("administration: err %v body %q, want the password policy object, bearer_token flag and totp kept", err, body)
	}
	// A success body that is not one JSON value and names a secret is refused
	// with a non-retryable 422, never passed through.
	for _, p := range []string{"/firewall/odd", "/tenants/concat"} {
		resp, err := hc.Get(srv.URL + p)
		if err != nil {
			t.Fatalf("%s: refusal must be an HTTP response, not a transport error: %v", p, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnprocessableEntity || strings.Contains(string(b), redactSentinel) {
			t.Fatalf("%s: status %d body %q, want 422 without the secret", p, resp.StatusCode, b)
		}
	}
	// A BOM is not a guard: the body is still one JSON value, redacted field by field.
	if resp, err := hc.Get(srv.URL + "/tenants/bom"); err != nil {
		t.Fatal(err)
	} else {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || strings.Contains(string(b), redactSentinel) || !strings.Contains(string(b), `"name":"t1"`) {
			t.Fatalf("bom: status %d body %q, want 200 with the password blanked", resp.StatusCode, b)
		}
	}
	// An HTML error page keeps its own status.
	if resp, err := hc.Get(srv.URL + "/tenants/html404"); err != nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("html 404: err %v, want the 404 passed through", err)
	} else {
		resp.Body.Close()
	}

	// MySonicWall reads: api_key, tcpassword and authCode are blanked.
	// Other spellings of the same secrets, and session IDs; error and product codes stay.
	for _, p := range []string{"/zeroTouch/devices/status", "/firewall/license/registration-code", "/users/active"} {
		resp, err := hc.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), redactSentinel) {
			t.Fatalf("%s: body %q, want every code and session ID redacted", p, b)
		}
		if p == "/users/active" && (!strings.Contains(string(b), `"msgCode":"OK"`) || !strings.Contains(string(b), `"product_code":"TZ370"`)) {
			t.Fatalf("%s: body %q, want msgCode and product_code kept", p, b)
		}
	}
	// NSM gateway and MySonicWall reads, with no X-DEVICE-ID header.
	for _, p := range []string{"/api/hgms/get-firewalls", "/api/product/serviceInfo", "/api/users/productlist-customer", "/devices/00000000000A", "/api/user/export-activity-log-details"} {
		resp, err := hc.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), redactSentinel) || !strings.Contains(string(b), "[redacted]") {
			t.Fatalf("%s: body %q, want the secret redacted", p, b)
		}
	}
	// The token exchanges nsm-login depends on are untouched.
	for _, p := range []string{"/api/generate-cscAccessCode", "/auth/sso"} {
		resp, err := hc.Post(srv.URL+p, "application/json", strings.NewReader("{}"))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), "[redacted]") || !strings.Contains(string(b), "keep-me") {
			t.Fatalf("%s: token exchange body changed: %q", p, b)
		}
	}

	// A body with no secret field is passed through byte for byte.
	resp, err := hc.Get(srv.URL + "/tenants")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != `{"name":"plain","serialNumber":"00000000000A"}` {
		t.Fatalf("secret-free body changed: %q", b)
	}
}

// Codex round 14 + adversarial rounds 15-16: the client gets exactly the bytes
// judged here. Guarded JSON is redacted like plain JSON and forwarded without
// its guard; duplicate keys and non-string secrets are blanked; anything that
// is not one JSON document is withheld whole.
func TestRedactJudgesDecodedForm(t *testing.T) {
	// No secret-shaped word in the sentinel itself, or the word check would
	// withhold every body for the wrong reason.
	const neutralSentinel = "XK7Q-ZETA-991"
	cases := []struct {
		name, path, body string
		status           int
	}{
		{"guarded key", "/tenants/g1", `while(1);{"note":"x","password":"` + neutralSentinel + `"}`, http.StatusOK},
		{"guarded escaped key", "/tenants/g2", `while(1);[{"id":"t11","pass\u0077ord":"` + neutralSentinel + `"}]`, http.StatusOK},
		{"guarded separator key", "/tenants/g3", `)]}'` + "\n" + `{"auth-code":"` + neutralSentinel + `"}`, http.StatusOK},
		{"guarded spaced key", "/tenants/g4", `for(;;);{"session id":"` + neutralSentinel + `"}`, http.StatusOK},
		{"guarded passthrough key", "/firewall/g5", `{}&&{"key":"` + neutralSentinel + `"}`, http.StatusOK},
		{"duplicate key", "/firewall/g6", `{"password":"` + neutralSentinel + `","password":{}}`, http.StatusOK},
		{"concatenated escaped key", "/tenants/g7", `{"id":"a"}` + "\n" + `{"id":"b","pass\u0077ord":"` + neutralSentinel + `"}`, http.StatusUnprocessableEntity},
		{"bare string", "/tenants/g8", `"pass\u0077ord: ` + neutralSentinel + `"`, http.StatusUnprocessableEntity},
		{"text escaped word", "/tenants/g9", `error: pass\u0077ord ` + neutralSentinel + ``, http.StatusUnprocessableEntity},
		{"text spaced name", "/tenants/g10", `Session ID: ` + neutralSentinel + ``, http.StatusUnprocessableEntity},
		{"html entity", "/tenants/g11", `<p>&#112;assword: ` + neutralSentinel + `</p>`, http.StatusUnprocessableEntity},
		{"percent-encoded form", "/tenants/g12", `name=alice&pass%77ord=` + neutralSentinel + ``, http.StatusUnprocessableEntity},
		{"text with no secret word", "/tenants/g13", `hello ` + neutralSentinel + ``, http.StatusUnprocessableEntity},
		{"non-string secrets", "/firewall/g14", `{"authentication_code":["` + neutralSentinel + `"],"assign_token":{"value":"` + neutralSentinel + `"},"psk":true}`, http.StatusOK},
		{"wep key leaf", "/firewall/g15", `{"radio":{"wep_key":{"key1":{"type":"hex","value":"` + neutralSentinel + `"}}}}`, http.StatusOK},
		{"guarded, no secret", "/tenants/g16", `while(1);[{"id":"t1","name":"Acme"}]`, http.StatusOK},
		{"plain, no secret", "/tenants/g17", `[{"id":"t1","name":"Acme"}]`, http.StatusOK},
		// Codex round 18: a settings-named key keeps only its schema policy
		// children; a nested string is judged like a bare-string body.
		{"secret array under settings name", "/firewall/g18", `{"user":{"local":{"user":[{"name":"alice","password":["` + neutralSentinel + `"]}]}}}`, http.StatusOK},
		{"cli ftp password default", "/firewall/g19", `{"cli":{"ftp":{"password":{"default":"` + neutralSentinel + `"}}}}`, http.StatusOK},
		{"unknown child under settings name", "/firewall/g20", `{"administration":{"password":{"minimum_length":8,"hint":"` + neutralSentinel + `"}}}`, http.StatusOK},
		{"confirm admin password", "/firewall/g21", `{"administration":{"admin_password":{"new":"` + neutralSentinel + `","confirm_admin_password":"` + neutralSentinel + `"}}}`, http.StatusOK},
		{"passphrase object", "/firewall/g22", `{"passphrase":{"active_time":5,"page":{"custom":"Blocked"},"password":"` + neutralSentinel + `"}}`, http.StatusOK},
		{"shared secret object", "/firewall/g23", `{"shared_secret":{"ike_id":{"local":{"ipv4":"10.0.0.1"}},"shared_secret":"` + neutralSentinel + `"}}`, http.StatusOK},
		{"nested text in array", "/firewall/g24", `["authentication_code: ` + neutralSentinel + `"]`, http.StatusOK},
		{"nested text in message", "/tenants/g25", `{"status":{"success":false,"info":[{"message":"authentication_code: ` + neutralSentinel + `"}]}}`, http.StatusOK},
		{"nested escaped text", "/tenants/g26", `{"note":"pass\u0077ord=` + neutralSentinel + `"}`, http.StatusOK},
		{"nested text under policy key", "/firewall/g27", `{"passphrase":{"page":{"custom":"passphrase: ` + neutralSentinel + `"}}}`, http.StatusOK},
		// Codex round 19: a settings object keeps a child only at a documented
		// path and type; text is decoded escape by escape; keys are judged too.
		{"policy name at an undocumented path", "/firewall/g28", `{"user":{"local":{"user":[{"name":"alice","password":{"type":"` + neutralSentinel + `"}}]}}}`, http.StatusOK},
		{"undocumented value in an enum slot", "/firewall/g29", `{"administration":{"password":{"complexity":{"type":"` + neutralSentinel + `"}}}}`, http.StatusOK},
		{"number where the schema has a string", "/firewall/g30", `{"shared_secret":{"ap_client_id":{"v":"` + neutralSentinel + `"}}}`, http.StatusOK},
		{"valid escape beside a malformed one", "/tenants/g31", `{"note":"pass%77ord: ` + neutralSentinel + `%"}`, http.StatusOK},
		{"mixed encodings", "/tenants/g32", `{"note":"%5Cu0070assword: ` + neutralSentinel + `"}`, http.StatusOK},
		{"key that assigns a credential", "/tenants/g33", `{"authentication_code: ` + neutralSentinel + `":true}`, http.StatusOK},
		{"percent-encoded key", "/tenants/g34", `{"pass%77ord":"` + neutralSentinel + `"}`, http.StatusOK},
		// Codex round 20 + adversarial round 18: an assigning key is dropped
		// with its value, in every container shape.
		{"assigning key inside a settings object", "/firewall/g35", `{"password":{"authentication_code: ` + neutralSentinel + `":true}}`, http.StatusOK},
		{"assigning key inside a secret container", "/firewall/g36", `{"wep_key":{"token: ` + neutralSentinel + `":false}}`, http.StatusOK},
		{"value beside an assigning key", "/tenants/g37", `[{"labels":{"VPN token:":"` + neutralSentinel + `","site":"HQ"}}]`, http.StatusOK},
		// Codex round 21 + adversarial round 19: any non-ASCII punctuation or
		// symbol separates; a settings object drops undocumented children,
		// key included; full-width key spellings match.
		{"unlisted equals lookalike", "/tenants/g38", "{\"note\":\"password\ua78a " + neutralSentinel + "\"}", http.StatusOK},
		{"secret as a key in a settings object", "/firewall/g39", `{"password":{"minimum_length":8,"` + neutralSentinel + `":true}}`, http.StatusOK},
		{"full-width secret key", "/tenants/g40", "{\"\uff50\uff41\uff53\uff53\uff57\uff4f\uff52\uff44\":\"" + neutralSentinel + "\"}", http.StatusOK},
		{"short credential word", "/tenants/g41", `{"comment":"Temp pw: ` + neutralSentinel + `"}`, http.StatusOK},
		// Codex round 22 + adversarial round 20: any whitespace keeps the
		// word; a bare string is judged at least as strictly as a nested one.
		{"short word across a newline", "/tenants/g42", `{"Temp pw\n:":"` + neutralSentinel + `"}`, http.StatusOK},
		{"short word across a no-break space", "/tenants/g43", "{\"note\":\"Code PIN\u00a0: " + neutralSentinel + "\"}", http.StatusOK},
		{"bare free-text word", "/tenants/g44", `"pwd: ` + neutralSentinel + `"`, http.StatusUnprocessableEntity},
		{"bare short word", "/tenants/g45", `"Wi-Fi PIN: ` + neutralSentinel + `"`, http.StatusUnprocessableEntity},
		{"bare community", "/tenants/g46", `"SNMP community: ` + neutralSentinel + `"`, http.StatusUnprocessableEntity},
		// Codex round 23: SonicOS credential names are credentials on every
		// route; a separator newer than this Go build's Unicode tables counts.
		{"SonicOS credential names on an NSM route", "/tenants/g47", `[{"id":"t1","name":"Acme","pop3_password":"` + neutralSentinel + `","shared_secret":"` + neutralSentinel + `"}]`, http.StatusOK},
		{"separator newer than the Unicode tables", "/tenants/g48", "[{\"note\":\"password\u1b4e " + neutralSentinel + "\"}]", http.StatusOK},
	}
	bodies := map[string]string{}
	for _, c := range cases {
		bodies[c.path] = c.body
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, bodies[r.URL.Path])
	}))
	defer srv.Close()
	hc := &http.Client{Transport: sonicOSRedactTransport{base: http.DefaultTransport}}
	for _, c := range cases {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+c.path, nil)
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != c.status || strings.Contains(string(b), neutralSentinel) {
			t.Errorf("%s: status %d body %q, want %d without the secret", c.name, resp.StatusCode, b, c.status)
		}
	}
}

// Brute-force oracle for the redaction contract: every secret name the vendor
// schemas document, in every body shape a client could decode (plain, BOM,
// each XSSI/JSONP guard, concatenated, \u-escaped, text with any separator),
// never reaches the caller.
func TestRedactOracleEveryNameEveryShape(t *testing.T) {
	const n = "XK7Q-ZETA-991"
	names := []string{}
	for k := range schemaSecretFields {
		names = append(names, k)
	}
	for k := range nsmCredentialFields {
		if !genericPassthroughNames[k] {
			names = append(names, k)
		}
	}
	esc := func(s string) string { return fmt.Sprintf("%su%04x", `\`, s[0]) + s[1:] }
	fullWidth := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r > 0x20 && r < 0x7F {
				return r + 0xFEE0
			}
			return r
		}, s)
	}
	dotted := func(s string) string { return strings.Join(strings.Split(s, ""), ".") }
	// Codex round 24: every spelling of a name gets the plain name's rule, so the
	// generic passthrough words (key, new, old) are in the key-shaped bodies too
	// (these are /firewall/ reads); free text keeps them, by design.
	keyNames := append([]string{}, names...)
	for k := range genericPassthroughNames {
		keyNames = append(keyNames, k)
	}
	var bodies []string
	for _, name := range keyNames {
		for _, key := range []string{name, esc(name), strings.ReplaceAll(name, "_", "-"), strings.ToUpper(name), fullWidth(name), dotted(name)} {
			obj := `{"id":"t1","` + key + `":"` + n + `"}`
			bodies = append(bodies, obj, "\xEF\xBB\xBF"+obj, `{"id":"a"}`+"\n"+obj)
			for _, g := range []string{")]}'\n", ")]}'", "{}&&", "for(;;);", "while(1);"} {
				bodies = append(bodies, g+obj)
			}
		}
		if genericPassthroughNames[name] {
			continue
		}
		for _, sep := range []string{"_", "-", " ", ""} {
			bodies = append(bodies, strings.ReplaceAll(name, "_", sep)+": "+n)
		}
		bodies = append(bodies, esc(name)+": "+n)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/firewall/"))
		_, _ = io.WriteString(w, bodies[i])
	}))
	defer srv.Close()
	hc := &http.Client{Transport: sonicOSRedactTransport{base: http.DefaultTransport}}
	for i, body := range bodies {
		resp, err := hc.Get(srv.URL + "/firewall/" + strconv.Itoa(i))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), n) {
			t.Errorf("body %q passed through as %d %q", body, resp.StatusCode, b)
		}
	}
}

// Adversarial round 16: a redirect hop the server points at a token-exchange
// path, a GET to one, and a deflate body only the client could decode are all
// judged, never passed through.
func TestRedactNoServerChosenBypass(t *testing.T) {
	const n = "XK7Q-ZETA-991"
	var zlibThenRaw bytes.Buffer
	zlibThenRaw.Write([]byte{0x78, 0x01}) // a valid zlib header ...
	fw, _ := flate.NewWriter(&zlibThenRaw, flate.NoCompression)
	_, _ = io.WriteString(fw, `{"password":"`+n+`"}`) // ... over a raw deflate stream
	_ = fw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/firewall/user/local/users":
			http.Redirect(w, r, "/firewall/user/local/users/auth/sso", http.StatusFound)
		case "/firewall/deflate":
			w.Header().Set("Content-Encoding", "deflate")
			_, _ = w.Write(zlibThenRaw.Bytes())
		default:
			_, _ = io.WriteString(w, `{"password":"`+n+`"}`)
		}
	}))
	defer srv.Close()
	hc := &http.Client{Transport: sonicOSRedactTransport{base: http.DefaultTransport}}
	for _, p := range []string{"/firewall/user/local/users", "/auth/sso", "/firewall/deflate"} {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+p, nil)
		req.Header.Set("Accept-Encoding", "deflate")
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(b), n) {
			t.Errorf("%s: secret passed through: %d %q", p, resp.StatusCode, b)
		}
	}
}

// Codex round 18 + 19 (false-RED side): the schema's policy fields under a
// settings name, enumeration values, and names that merely contain a secret
// word stay readable, because posture and mfa-audit grade and join on them.
func TestRedactKeepsPolicyFieldsNamesAndEnumValues(t *testing.T) {
	body := `{"administration":{"password":{"minimum_length":8,"complexity":{"type":"alpha-and-numeric","digital":1},"aging":{"duration":90},"constraints_apply_to":{"full_admins":true}}},` +
		`"ssl_vpn":{"server":{"auth_type":"password","access":[{"zone":"WAN-secret","enable":true}]}},"ospf":{"auth":"simple-password"},"vpn":{"auth_method":"shared-secret"},` +
		`"shared_secret":{"ike_id":{"peer":{"domain_name":"vpn.example.com"}},"ap_client_id":"ap-1","use_default_key":false},` +
		`"passphrase":{"active_time":5,"page":{"default":true,"custom":"Ask IT for the passphrase"}},"users":[{"name":"alice","auth_type":"passphrase"},{"name":"pskinner"}],` +
		`"groups":[{"name":"Token Users"},{"name":"Secretaries"},{"name":"Password Users"}],` +
		`"status":{"info":[{"message":"invalid password"},{"message":"Password policy updated"},{"message":"Error: invalid token"}]}}`
	var doc any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatal(err)
	}
	if redactSchemaSecrets(doc, true) {
		out, _ := json.Marshal(doc)
		t.Fatalf("a body with only policy fields, names and enum values must not change, got %s", out)
	}
	for _, s := range []string{"password", "shared-secret", "simple-password", "Token Users", "Secretaries", "pskinner",
		"WAN-secret", "Password Users", "invalid password", "Password policy updated", "12:30:00", "https://example.com/a",
		"SonicOS SSLVPN Authentication Bypass: CVE-2024-53704", "NAT hairpin: enabled", "Status: pass", "Spin: 3",
		"Hardware Token \u2013 Pilot", "\u201cToken\u201d Users", "Token\u00ae Group", "Password\u2122 Manager"} {
		if textAssignsSecret(s) {
			t.Errorf("%q assigns nothing to a credential and must be kept", s)
		}
	}
	for _, s := range []string{"authentication_code: X", "password: !@#$%^&*()", "password: \u5bc6\u78bc\uff01", "pass%77ord: X%",
		"%5Cu0070assword: X", "pass&amp;#119;ord=X", "auth-code=X", "Session ID: X", "Passwd: X", "pwd=X",
		"SNMP community: public", `{"password":"X"}`, "https://h/x?token=X", "password\uff1aX", "password\u2236 X",
		"password \u2254 X", "\uff50\uff41\uff53\uff53\uff57\uff4f\uff52\uff44: X", "password\ua78a X", "password\u0589 X",
		"Temp pw: X", "Wi-Fi PIN: X", "VPN pass: X", "pw=X", "pw : X", "Temp pw\n: X", "Code PIN\u00a0: X",
		"Code PIN\u200b: X", "Code PIN\ufeff: X", "Code PIN\u00ad: X", "Code PIN\xa0: X", "Code PIN\ufffd: X",
		"Door pass\u2009= X", "\uff30\uff29\uff2e\u3000\uff1aX", "Wi-Fi PIN&nbsp;: X", "password\ufe30 X"} {
		if !textAssignsSecret(s) {
			t.Errorf("%q assigns a credential and must be blanked", s)
		}
	}
	if s := "%" + strings.Repeat("25", 20) + "41"; !textAssignsSecret(s) {
		t.Errorf("text still changing after %d decode rounds must count as unjudgeable", maxDecodeRounds)
	}
	var nulls any
	_ = json.Unmarshal([]byte(`{"administration":{"password":{"aging":null,"complexity":null,"constraints_apply_to":{}}}}`), &nulls)
	if redactSchemaSecrets(nulls, true) {
		t.Error("a documented null policy object (unconfigured) must be kept")
	}
	if unassigned == nil {
		t.Fatal("the Cn table must exist, or no unassigned code point counts as a separator")
	}
	if redactedText != posture.Redacted {
		t.Fatal("the redaction placeholder and posture's must be one string")
	}
}
