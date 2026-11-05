// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-novels): end-to-end runs of the
// warranty commands against fake vendors built from the documented response
// shapes, through the real Execute. The credential kit is proven on the built binaries in
// internal/credwire; no test here runs credkit.Start.

package cli

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"warranty-pp-cli/internal/client"
	"warranty-pp-cli/internal/cliutil/testenv"
)

func init() {
	client.OutboundGuard = func(rt http.RoundTripper) http.RoundTripper { return rt }
}

type seenReq struct {
	origin, method, path, query, auth, clientID, body string
}

// fakeVendor answers every vendor endpoint this CLI calls, by origin and path.
type fakeVendor struct {
	mu         sync.Mutex
	seen       []seenReq
	dellToken  func() (int, string)
	dell       func(r *http.Request) (int, string)
	lenovo     func(r *http.Request) (int, string)
	appleToken func() (int, string)
	apple      func(r *http.Request) (int, string)
}

func (f *fakeVendor) RoundTrip(r *http.Request) (*http.Response, error) {
	var body string
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}
	origin := client.Origin(r.URL)
	f.mu.Lock()
	f.seen = append(f.seen, seenReq{origin, r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), r.Header.Get("ClientID"), body})
	f.mu.Unlock()
	code, out := 599, `{"error":"unexpected request"}`
	switch {
	case origin == "https://apigtwb2c.us.dell.com:443" && r.URL.Path == "/auth/oauth/v2/token":
		code, out = f.dellToken()
	case origin == "https://apigtwb2c.us.dell.com:443" && strings.HasPrefix(r.URL.Path, "/PROD/sbil/eapi/v5/"):
		code, out = f.dell(r)
	case origin == client.LenovoOrigin && strings.HasPrefix(r.URL.Path, "/v2.5/"):
		code, out = f.lenovo(r)
	case origin == "https://account.apple.com:443" && r.URL.Path == "/auth/oauth2/token":
		code, out = f.appleToken()
	case origin == client.AppleAPIOrigin && strings.HasPrefix(r.URL.Path, "/v1/"):
		code, out = f.apple(r)
	}
	return &http.Response{StatusCode: code, Status: fmt.Sprintf("%d", code), Body: io.NopCloser(strings.NewReader(out)),
		Header: http.Header{"Content-Type": {"application/json"}}, Request: r, ContentLength: int64(len(out))}, nil
}

func (f *fakeVendor) requests() []seenReq {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]seenReq(nil), f.seen...)
}

const (
	testDellToken  = "dell-access-token-AAAA1111"
	testAppleToken = "apple-access-token-BBBB2222"
)

var dellEnd = "2026-12-01T04:59:59.999Z"

func defaultFakeVendor() *fakeVendor {
	f := &fakeVendor{}
	f.dellToken = func() (int, string) {
		return 200, `{"access_token":"` + testDellToken + `","token_type":"Bearer","expires_in":3600}`
	}
	f.dell = func(r *http.Request) (int, string) {
		tags := strings.Split(r.URL.Query().Get("servicetags"), ",")
		parts := []string{}
		for _, tag := range tags {
			parts = append(parts, `{"id":1,"serviceTag":"`+tag+`","shipDate":"2021-03-14T05:00:00Z","productLineDescription":"LATITUDE 5420","invalid":false,"duplicated":false,
			 "entitlements":[{"itemNumber":"997-8367","startDate":"2021-03-14T05:00:00Z","endDate":"`+dellEnd+`","entitlementType":"INITIAL","serviceLevelCode":"ND","serviceLevelDescription":"ProSupport Plus for PCs","serviceLevelGroup":5}]}`)
		}
		return 200, "[" + strings.Join(parts, ",") + "]"
	}
	f.lenovo = func(r *http.Request) (int, string) {
		parts := []string{}
		for _, s := range r.URL.Query()["Serial"] {
			parts = append(parts, `{"Serial":"`+s+`","Product":"21CBCTO1WW","InWarranty":false,"Purchased":"2020-02-01T00:00:00","Shipped":"2020-01-20T00:00:00",
			 "Warranty":[{"ID":"1EZ","Name":"Base Warranty","Type":"BASE","Start":"2020-01-20T00:00:00","End":"2023-01-19T00:00:00"}],"Contract":[]}`)
		}
		if len(parts) == 1 {
			return 200, parts[0]
		}
		return 200, "[" + strings.Join(parts, ",") + "]"
	}
	f.appleToken = func() (int, string) {
		return 200, `{"access_token":"` + testAppleToken + `","token_type":"Bearer","expires_in":3600}`
	}
	f.apple = func(r *http.Request) (int, string) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/appleCareCoverage"):
			return 200, `{"data":[{"type":"appleCareCoverage","id":"x1","attributes":{"status":"ACTIVE","paymentType":"ABE_SUBSCRIPTION","description":"AppleCare+ for Business","startDateTime":"2025-04-17T00:00:00Z","endDateTime":null,"isRenewable":true,"isCanceled":false}}],"links":{"self":"x"}}`
		case r.URL.Path == "/v1/orgDevices":
			return 200, `{"data":[{"type":"orgDevices","id":"C02XG0FDH7JY","attributes":{"serialNumber":"C02XG0FDH7JY","deviceModel":"MacBook Air","orderDateTime":"2024-05-01T00:00:00Z"}},
			  {"type":"orgDevices","id":"F2LXQ0AAQ6L4","attributes":{"serialNumber":"F2LXQ0AAQ6L4","deviceModel":"iPad Air","orderDateTime":"2025-01-02T00:00:00Z"}}],
			  "links":{"self":"x"},"meta":{"paging":{"limit":1000}}}`
		default:
			return 200, `{"data":{"type":"orgDevices","id":"C02XG0FDH7JY","attributes":{"serialNumber":"C02XG0FDH7JY","deviceModel":"MacBook Air","orderDateTime":"2024-05-01T00:00:00Z"}}}`
		}
	}
	return f
}

// warrantyTestEnv isolates the home, sets every vendor's credentials and
// slots f under the guard on every client the CLI builds.
func warrantyTestEnv(t *testing.T, f *fakeVendor) (home string, secrets map[string]string) {
	t.Helper()
	home = testenv.Isolate(t)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	secrets = map[string]string{
		"WARRANTY_DELL_CLIENT_ID":     "dellclientid-7Q2X9WWW",
		"WARRANTY_DELL_CLIENT_SECRET": "test-secret-dell",
		"WARRANTY_LENOVO_CLIENT_KEY":  "test-key-lenovo",
		"WARRANTY_APPLE_CLIENT_ID":    "BUSINESSAPI.test-0001",
		"WARRANTY_APPLE_KEY_ID":       "kid-test-01",
		"WARRANTY_APPLE_PRIVATE_KEY":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
	}
	for k, v := range secrets {
		t.Setenv(k, v)
	}
	saved := clientHooks
	clientHooks = append([]func(*client.Client) error{func(c *client.Client) error {
		c.HTTPClient.Transport = f
		return nil
	}}, saved...)
	t.Cleanup(func() { clientHooks = saved })
	return home, secrets
}

// execWarranty runs the real Execute with args and stdin, capturing the
// process's stdout and stderr.
func execWarranty(t *testing.T, stdin string, args ...string) (string, string, error) {
	t.Helper()
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	inR, inW, _ := os.Pipe()
	go func() { _, _ = io.WriteString(inW, stdin); _ = inW.Close() }()
	savedOut, savedErr, savedIn, savedArgs := os.Stdout, os.Stderr, os.Stdin, os.Args
	os.Stdout, os.Stderr, os.Stdin = outW, errW, inR
	os.Args = append([]string{"warranty-cli"}, args...)
	var stdout, stderr []byte
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { stdout, _ = io.ReadAll(outR); wg.Done() }()
	go func() { stderr, _ = io.ReadAll(errR); wg.Done() }()
	err := Execute()
	os.Stdout, os.Stderr, os.Stdin, os.Args = savedOut, savedErr, savedIn, savedArgs
	_ = outW.Close()
	_ = errW.Close()
	wg.Wait()
	return string(stdout), string(stderr), err
}

func decodeJSON(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, s)
	}
}

const testImportCSV = "Device Name,Serial Number,Manufacturer,Model\n" +
	"WS-01,1ABC234,Dell Inc.,Latitude 5420\n" +
	"WS-02,PF1ABCDE,LENOVO,ThinkPad T14\n" +
	"MAC-01,C02XG0FDH7JY,Apple Inc.,\n" +
	"WS-03,5CG1234ABC,HP,EliteBook\n"

func TestWarrantyEndToEnd(t *testing.T) {
	if inFreshProcess(t) {
		return
	}
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)

	out, errOut, err := execWarranty(t, testImportCSV, "import", "-", "--customer", "Acme Dental", "--json")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, errOut)
	}
	var imp importResult
	decodeJSON(t, out, &imp)
	if imp.Created != 4 || imp.ByVendor["dell"] != 1 || imp.ByVendor["lenovo"] != 1 || imp.ByVendor["apple"] != 1 || len(imp.NoVendor) != 1 {
		t.Fatalf("import result = %+v", imp)
	}

	out, errOut, err = execWarranty(t, "", "recheck", "--older-than", "30d", "--as-of", "2026-10-28", "--json")
	if err != nil {
		t.Fatalf("recheck: %v\n%s", err, errOut)
	}
	var run lookupRun
	decodeJSON(t, out, &run)
	if run.Calls["dell"] != 1 || run.Calls["lenovo"] != 1 || run.Calls["apple"] != 2 || !run.Saved || len(run.Results) != 3 {
		t.Fatalf("recheck = %+v", run)
	}
	byserial := map[string]lookupOutcome{}
	for _, o := range run.Results {
		byserial[o.Serial] = o
	}
	if o := byserial["1ABC234"]; o.Result != "ok" || o.CoverageEnd != "2026-12-01" || o.Status != "expiring" || o.DaysLeft == nil || *o.DaysLeft != 34 {
		t.Errorf("dell outcome = %+v", o)
	}
	if o := byserial["PF1ABCDE"]; o.Result != "ok" || o.Status != "expired" || o.CoverageEnd != "2023-01-19" {
		t.Errorf("lenovo outcome = %+v", o)
	}
	if o := byserial["C02XG0FDH7JY"]; o.Result != "ok" || !o.CoverageOpen || o.Status != "covered" || o.Model != "MacBook Air" {
		t.Errorf("apple outcome = %+v", o)
	}

	// Each credential went only to its own vendor's origin.
	for _, r := range f.requests() {
		switch r.origin {
		case "https://apigtwb2c.us.dell.com:443":
			if r.clientID != "" {
				t.Errorf("Dell request carried the Lenovo ClientID header: %+v", r)
			}
			if r.path != "/auth/oauth/v2/token" && r.auth != "Bearer "+testDellToken {
				t.Errorf("Dell request auth = %q", r.auth)
			}
		case client.LenovoOrigin:
			if r.clientID != "test-key-lenovo" || r.auth != "" {
				t.Errorf("Lenovo request headers = %+v", r)
			}
			if !strings.Contains(r.query, "Serial=PF1ABCDE") {
				t.Errorf("Lenovo query = %q", r.query)
			}
		case client.AppleAPIOrigin:
			if r.auth != "Bearer "+testAppleToken || r.clientID != "" {
				t.Errorf("Apple request headers = %+v", r)
			}
		case "https://account.apple.com:443":
			if !strings.Contains(r.body, "client_assertion=") || strings.Contains(r.body, "dellsecret") {
				t.Errorf("Apple token request body = %q", r.body)
			}
		default:
			t.Errorf("request to an unexpected origin: %+v", r)
		}
	}

	out, _, err = execWarranty(t, "", "report", "--within", "90d", "--as-of", "2026-10-28", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var rep reportView
	decodeJSON(t, out, &rep)
	if rep.Counts["expiring"] != 1 || rep.Counts["expired"] != 1 || rep.Unverified != 1 || len(rep.Devices) != 2 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Devices[0].Serial != "PF1ABCDE" || rep.Devices[1].Serial != "1ABC234" || rep.Devices[1].Customer != "Acme Dental" {
		t.Errorf("report order/fields = %+v", rep.Devices)
	}

	// The recheck above judged a past --as-of, so it raised no dated alert. A
	// lookup judged today raises them.
	if _, errOut, err := execWarranty(t, "", "lookup", "1ABC234", "PF1ABCDE", "--json"); err != nil {
		t.Fatalf("lookup today: %v\n%s", err, errOut)
	}
	out, _, err = execWarranty(t, "", "events", "list", "--since", "0", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var evs struct {
		Events []map[string]any `json:"events"`
		Cursor string           `json:"cursor"`
	}
	decodeJSON(t, out, &evs)
	names := map[string]int{}
	for _, e := range evs.Events {
		names[fmt.Sprint(e["name"])]++
		if e["client"] != "Acme Dental" {
			t.Errorf("event client = %v", e["client"])
		}
	}
	// Dell ends 2026-12-01 (expiring before it, expired after); Lenovo ended in 2023.
	if names["coverage.expiring"]+names["coverage.expired"] != 2 || names["coverage.expired"] < 1 || names["coverage.changed"] != 0 {
		t.Fatalf("events = %+v", evs.Events)
	}

	out, _, err = execWarranty(t, "", "unresolved", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var un struct {
		Devices []unresolvedDevice `json:"devices"`
	}
	decodeJSON(t, out, &un)
	if len(un.Devices) != 1 || un.Devices[0].Serial != "5CG1234ABC" || un.Devices[0].Reason != "vendor_unknown" {
		t.Fatalf("unresolved = %+v", un.Devices)
	}

	out, _, err = execWarranty(t, "", "lifecycle", "--as-of", "2026-10-28", "--quarters", "2", "--age", "4y", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var lc []map[string]any
	decodeJSON(t, out, &lc)
	if len(lc) != 1 || lc[0]["group"] != "Acme Dental" || lc[0]["devices"] != float64(4) || lc[0]["expired"] != float64(1) ||
		lc[0]["open_ended"] != float64(1) || lc[0]["unverified"] != float64(1) || lc[0]["ends_2026_Q4"] != float64(1) || lc[0]["past_age"] != float64(2) {
		t.Fatalf("lifecycle = %+v", lc)
	}

	// A renewal extends the Dell coverage: the next recheck raises coverage.changed once.
	dellEnd = "2029-12-01T04:59:59.999Z"
	t.Cleanup(func() { dellEnd = "2026-12-01T04:59:59.999Z" })
	for i := 0; i < 2; i++ {
		if _, errOut, err := execWarranty(t, "", "recheck", "--older-than", "0d", "--vendor", "dell", "--as-of", "2026-10-28", "--json"); err != nil {
			t.Fatalf("recheck %d: %v\n%s", i, err, errOut)
		}
	}
	out, _, _ = execWarranty(t, "", "events", "list", "--since", evs.Cursor, "--json")
	decodeJSON(t, out, &evs)
	if len(evs.Events) != 1 || evs.Events[0]["name"] != "coverage.changed" {
		t.Fatalf("after renewal events = %+v", evs.Events)
	}

	// apple-pull: one new Mac/iPad, one already imported.
	out, errOut, err = execWarranty(t, "", "apple-pull", "--customer", "Acme Dental", "--json")
	if err != nil {
		t.Fatalf("apple-pull: %v\n%s", err, errOut)
	}
	var ap applePullView
	decodeJSON(t, out, &ap)
	if !ap.Complete || len(ap.NewInOrg) != 1 || ap.NewInOrg[0] != "F2LXQ0AAQ6L4" || len(ap.NotInImport) != 1 || len(ap.ImportedNotInOrg) != 0 {
		t.Fatalf("apple-pull = %+v", ap)
	}
}

func TestWarrantyLookupVendorRouting(t *testing.T) {
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	t.Setenv("WARRANTY_LENOVO_CLIENT_KEY", "")
	out, errOut, err := execWarranty(t, "", "lookup", "1ABC234", "PF1ABCDE", "ABCDEFGHIJ", "--no-save", "--as-of", "2026-10-28", "--json")
	if err != nil {
		t.Fatalf("lookup: %v\n%s", err, errOut)
	}
	var run lookupRun
	decodeJSON(t, out, &run)
	got := map[string]string{}
	for _, o := range run.Results {
		got[o.Serial] = o.Result
	}
	if got["1ABC234"] != "ok" || got["PF1ABCDE"] != "vendor_unconfigured" || got["ABCDEFGHIJ"] != "vendor_unknown" || run.Saved {
		t.Fatalf("lookup results = %+v", run)
	}
	for _, r := range f.requests() {
		if r.origin == client.LenovoOrigin {
			t.Errorf("an unconfigured vendor was called: %+v", r)
		}
	}
	// --dry-run calls nothing and prints the plan.
	before := len(f.requests())
	out, _, err = execWarranty(t, "", "lookup", "1ABC234", "--dry-run", "--json")
	if err != nil || !strings.Contains(out, "dell 1 serial(s) in 1 call(s)") || len(f.requests()) != before {
		t.Fatalf("dry-run = %q err=%v", out, err)
	}
}

// secretForms is every encoding the leak scan looks for.
func secretForms(s string) []string {
	return []string{s, base64.StdEncoding.EncodeToString([]byte(s)), base64.RawURLEncoding.EncodeToString([]byte(s)),
		hex.EncodeToString([]byte(s)), url.QueryEscape(s), strings.ReplaceAll(s, "-", "\\u002d")}
}

func TestWarrantyRejectedMintDoesNotHang(t *testing.T) {
	f := defaultFakeVendor()
	warrantyTestEnv(t, f)
	f.dellToken = func() (int, string) { return 401, `{"error":"invalid_client"}` }
	done := make(chan error, 1)
	go func() {
		_, _, err := execWarranty(t, "", "lookup", "1ABC234", "--vendor", "dell", "--no-save", "--json")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a rejected token mint must fail the lookup")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("lookup hung after a rejected token mint")
	}
}

func TestWarrantyFileReadsRefusedUnderMCP(t *testing.T) {
	warrantyTestEnv(t, defaultFakeVendor())
	t.Setenv("WARRANTY_MCP_CHILD", "1")
	t.Setenv("WARRANTY_MCP_CHILD", "confirmed") // confirm: true does not unlock a host file read
	for _, args := range [][]string{{"import", "-", "--customer", "X"}, {"lookup", "--file", "/etc/hosts"}} {
		_, errOut, err := execWarranty(t, "1ABC234\n", args...)
		if err == nil || !strings.Contains(errOut+err.Error(), "CLI only") {
			t.Errorf("%v under MCP: err=%v stderr=%s", args, err, errOut)
		}
	}
}
