// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package awsbillingblackbox

// The fakes the built binaries talk to:
//
//   - fakeAWS answers the AWS SDK by protocol (the SigV4 credential scope names
//     the service): STS (query, XML: GetCallerIdentity, AssumeRole), Cost
//     Explorer and Organizations (JSON 1.1, X-Amz-Target), EC2 (query, EC2
//     XML), S3 (ListBuckets XML), CloudWatch (RPC v2 CBOR, the protocol of the
//     pinned cloudwatch v1.57.1) and SSO GetRoleCredentials (REST JSON). Every
//     answer repeats every credential the process holds, in every encoding,
//     in the values the CLI prints: Cost Explorer group keys (a service name,
//     an account), account names, dimension values, resource IDs, the caller
//     ARN, error messages; and split across two adjacent group keys, two
//     adjacent accounts and two lines.
//   - fakeAPI is the generated HTTP client's base URL (AWS_BILLING_BASE_URL):
//     the store-backed endpoints, answering with a credential of its own.
//   - tripwire is the HTTPS_PROXY every process gets: a request to any host
//     but the loopback fakes (a real AWS endpoint) would reach it.

import (
	"bytes"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/smithy-go/encoding/cbor"
)

// fakeMode is how the fake AWS answers service calls (the credential services
// always answer).
type fakeMode int

const (
	modeOK   fakeMode = iota // data answers
	modeDeny                 // every service call is refused: AccessDenied, SignatureDoesNotMatch quoting the access key
)

type fakeAWS struct {
	url          string // https://127.0.0.1:port
	caFile       string
	containerURL string // http://127.0.0.1:port: the container credential endpoint

	mu       sync.Mutex
	mode     fakeMode
	echo     []string       // what the process holds: repeated in every answer
	splits   []string       // the source's principal credentials: also repeated split in two
	noSplits bool           // repeat values whole only
	quiet    bool           // repeat nothing (the functional tests)
	cwDeny   bool           // CloudWatch refuses even in modeOK
	hideVol  string         // a volume DescribeVolumes no longer returns
	protos   map[string]int // HTTP protocol versions the requests arrived over
	dimSplit string         // GetDimensionValues answers half of it per call, 12 KB of other values between
	dimCalls int
	mintedBy map[string][]string // a minted access key ID -> the credentials minted with it
	mints    int
	requests []string          // "service Operation", in order
	signedBy map[string]int    // access key IDs in the SigV4 credential scopes received
	sent     map[string]bool   // known secrets the requests carried
	ssoToken map[string]int    // x-amz-sso_bearer_token values received
	redirect map[string]string // operation -> Location (the hop must never be followed)
	hops     int               // requests that arrived at a redirect target path

	// Codex r1 (wrong results): accounts and amounts per signing key, a
	// per-day amount, failing periods, the windows asked for, bucket regions,
	// a copied snapshot. The zero values keep every answer above.
	callerBySigner map[string]string  // GetCallerIdentity Account per signing access key ID (default 111111111111)
	amountBySigner map[string]float64 // the base group's amount per signing key (default 999.99)
	perDay         float64            // when set, the base group's amount is perDay times the bucket's days
	ceFailStart    map[string]bool    // GetCostAndUsage periods (TimePeriod.Start) answered DataUnavailableException
	ceWindows      []string           // "start end granularity dims" of every GetCostAndUsage call
	snapCopy       bool               // DescribeSnapshots also returns a recent copied snapshot
	estimated      bool               // every GetCostAndUsage period is marked Estimated (the current month)

	// Codex r1b (credential-used-but-not-held): AssumeRole is refused with an
	// error whose Message quotes the access key ID that signed it.
	assumeDeny bool
}

// tlsServer starts a TLS fake; caFile is its certificate as a PEM bundle.
func tlsServer(t *testing.T, h http.Handler) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, ca
}

var sigV4Scope = regexp.MustCompile(`Credential=([A-Z0-9]+)/[0-9]{8}/([a-z0-9-]+)/([a-z0-9-]+)/aws4_request`)

func newFakeAWS(t *testing.T, echo []string) *fakeAWS {
	t.Helper()
	f := &fakeAWS{echo: append([]string(nil), echo...), signedBy: map[string]int{}, sent: map[string]bool{},
		ssoToken: map[string]int{}, redirect: map[string]string{}, mintedBy: map[string][]string{}, protos: map[string]int{}}
	srv, ca := tlsServer(t, http.HandlerFunc(f.serve))
	f.url, f.caFile = srv.URL, ca
	// The container credential endpoint (ECS, EKS Pod Identity): plain http on
	// loopback, credentials for the authorization token it is sent.
	ecs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, "ecs "+r.Header.Get("Authorization"))
		f.mu.Unlock()
		if r.Header.Get("Authorization") != bbECSAuth {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"code":"Unauthorized","message":"bad token %s"}`, strings.Join(f.echoSet(nil, ""), " "))
			return
		}
		trio := f.mint("ecs")
		w.Header().Set("Content-Type", "application/json")
		v, _ := json.Marshal(map[string]any{"AccessKeyId": trio[0], "SecretAccessKey": trio[1], "Token": trio[2],
			"Expiration": time.Now().Add(time.Hour).UTC().Format(time.RFC3339), "RoleArn": "arn:aws:iam::111111111111:role/hq-task"})
		_, _ = w.Write(v)
	}))
	t.Cleanup(ecs.Close)
	f.containerURL = ecs.URL
	return f
}

// set changes the fake under its lock.
func (f *fakeAWS) set(change func(f *fakeAWS)) {
	f.mu.Lock()
	change(f)
	f.mu.Unlock()
}

func (f *fakeAWS) setMode(m fakeMode) {
	f.mu.Lock()
	f.mode = m
	f.mu.Unlock()
}

// reset forgets the requests seen so far (the echo set stays).
func (f *fakeAWS) reset() {
	f.mu.Lock()
	f.requests, f.signedBy, f.sent, f.ssoToken, f.hops = nil, map[string]int{}, map[string]bool{}, map[string]int{}, 0
	f.mu.Unlock()
}

func (f *fakeAWS) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeAWS) signed(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.signedBy[key]
}

func (f *fakeAWS) sawSSOToken(tok string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ssoToken[tok]
}

func (f *fakeAWS) count(op string) int {
	n := 0
	for _, r := range f.seen() {
		if strings.HasSuffix(r, " "+op) {
			n++
		}
	}
	return n
}

// echoSet is what an answer repeats: what the process holds from its startup
// sources, the credentials minted for the process that signed this request,
// and every known secret this request carried.
func (f *fakeAWS) echoSet(sent []string, signer string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.quiet {
		return nil
	}
	return union(f.echo, f.mintedBy[signer], sent)
}

// mintedSigned is how many requests were signed with minted credentials.
func (f *fakeAWS) mintedSigned() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for key, c := range f.signedBy {
		if f.mintedBy[key] != nil {
			n += c
		}
	}
	return n
}

// mint hands out fresh credentials of kind for one credential answer.
func (f *fakeAWS) mint(kind string) []string {
	f.mu.Lock()
	f.mints++
	n := f.mints
	f.mu.Unlock()
	trio := mint(kind, n)
	f.mu.Lock()
	f.mintedBy[trio[0]] = trio
	f.mu.Unlock()
	return trio
}

func union(sets ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, set := range sets {
		for _, s := range set {
			if s != "" && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}

// sentSecrets is every known secret the request carried (headers, URL, body).
func sentSecrets(r *http.Request, body []byte) []string {
	seen := fmt.Sprint(r.Header) + "\n" + r.URL.String() + "\n" + string(body)
	var out []string
	for _, s := range secrets() {
		if strings.Contains(seen, s) {
			out = append(out, s)
		}
	}
	return out
}

func (f *fakeAWS) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.protos[r.Proto]++
	f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	sent := sentSecrets(r, body)
	service, op, signer := "", "", ""
	if m := sigV4Scope.FindStringSubmatch(r.Header.Get("Authorization")); m != nil {
		service, signer = m[3], m[1]
		f.mu.Lock()
		f.signedBy[m[1]]++
		f.mu.Unlock()
	}
	form, _ := url.ParseQuery(string(body))
	switch {
	case r.URL.Path == "/federation/credentials":
		service, op = "sso", "GetRoleCredentials"
		f.mu.Lock()
		f.ssoToken[r.Header.Get("X-Amz-Sso_bearer_token")]++
		f.mu.Unlock()
	case strings.HasPrefix(r.URL.Path, "/redirected"):
		f.mu.Lock()
		f.hops++
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		return
	case service == "ce" || service == "organizations":
		op = r.Header.Get("X-Amz-Target")
		op = op[strings.LastIndex(op, ".")+1:]
	case service == "monitoring":
		op = r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	case service == "s3":
		op = "ListBuckets"
	default:
		op = form.Get("Action")
	}
	f.mu.Lock()
	f.requests = append(f.requests, service+" "+op)
	for _, s := range sent {
		f.sent[s] = true
	}
	mode := f.mode
	loc := f.redirect[op]
	f.mu.Unlock()
	if loc != "" {
		http.Redirect(w, r, loc, http.StatusTemporaryRedirect)
		return
	}
	echo := f.echoSet(sent, signer)
	switch service {
	case "sts":
		f.sts(w, op, echo, mode, signer)
	case "sso":
		f.sso(w, echo)
	case "ce":
		f.costExplorer(w, op, body, echo, mode, sent, signer)
	case "organizations":
		f.organizations(w, op, echo, mode)
	case "ec2":
		f.ec2(w, op, echo, mode, sent)
	case "s3":
		f.s3(w, echo, mode, sent, r.URL.Query())
	case "monitoring":
		f.cloudWatch(w, echo, mode)
	default:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `{"__type":"UnknownOperationException","message":"no such operation %s"}`, xmlText(strings.Join(echo, " ")))
	}
}

// accessKeyOf is the access key ID a SignatureDoesNotMatch answer quotes: the
// first of sent that looks like one.
func accessKeyOf(sent []string) string {
	for _, s := range sent {
		if strings.HasPrefix(s, "AKIA") || strings.HasPrefix(s, "ASIA") {
			return s
		}
	}
	return "AKIA" + "NOTSENT000000000"
}

const stsNS = "https://sts.amazonaws.com/doc/2011-06-15/"

func (f *fakeAWS) sts(w http.ResponseWriter, op string, echo []string, mode fakeMode, signer string) {
	w.Header().Set("Content-Type", "text/xml")
	switch op {
	case "AssumeRole":
		f.mu.Lock()
		deny := f.assumeDeny
		f.mu.Unlock()
		if deny {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `<ErrorResponse xmlns=%q><Error><Type>Sender</Type><Code>AccessDenied</Code>`+
				`<Message>User: arn:aws:sts::111111111111:assumed-role/bb-source is not authorized to perform: sts:AssumeRole on resource: arn:aws:iam::111111111111:role/bb (AWSAccessKeyId %s)</Message>`+
				`</Error><RequestId>req-5</RequestId></ErrorResponse>`, stsNS, xmlText(signer))
			return
		}
		trio := f.mint("role")
		fmt.Fprintf(w, `<AssumeRoleResponse xmlns=%q><AssumeRoleResult><Credentials><AccessKeyId>%s</AccessKeyId>`+
			`<SecretAccessKey>%s</SecretAccessKey><SessionToken>%s</SessionToken><Expiration>%s</Expiration></Credentials>`+
			`<AssumedRoleUser><Arn>arn:aws:sts::111111111111:assumed-role/bb/HQ-Session %s</Arn><AssumedRoleId>AROABB:HQ</AssumedRoleId></AssumedRoleUser>`+
			`</AssumeRoleResult><ResponseMetadata><RequestId>req-1</RequestId></ResponseMetadata></AssumeRoleResponse>`,
			stsNS, trio[0], xmlText(trio[1]), xmlText(trio[2]), time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
			xmlEcho(union(echo, trio)))
	case "GetCallerIdentity":
		if mode == modeDeny {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintf(w, `<ErrorResponse xmlns=%q><Error><Type>Sender</Type><Code>SignatureDoesNotMatch</Code>`+
				`<Message>The request signature we calculated does not match the signature you provided. Check your AWS Secret Access Key and signing method. AWSAccessKeyId %s %s</Message>`+
				`</Error><RequestId>req-2</RequestId></ErrorResponse>`, stsNS, xmlText(accessKeyOf(echo)), xmlEcho(echo))
			return
		}
		account := f.callerOf(signer)
		fmt.Fprintf(w, `<GetCallerIdentityResponse xmlns=%q><GetCallerIdentityResult><Arn>arn:aws:iam::%s:user/HQ-User %s</Arn>`+
			`<UserId>AIDABBUSER</UserId><Account>%s</Account></GetCallerIdentityResult>`+
			`<ResponseMetadata><RequestId>req-3</RequestId></ResponseMetadata></GetCallerIdentityResponse>`,
			stsNS, account, xmlEcho(echo), account)
	default:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, `<ErrorResponse xmlns=%q><Error><Type>Sender</Type><Code>InvalidAction</Code><Message>%s</Message></Error></ErrorResponse>`, stsNS, xmlText(op))
	}
}

func (f *fakeAWS) sso(w http.ResponseWriter, echo []string) {
	trio := f.mint("sso")
	w.Header().Set("Content-Type", "application/json")
	v, _ := json.Marshal(map[string]any{"roleCredentials": map[string]any{
		"accessKeyId": trio[0], "secretAccessKey": trio[1], "sessionToken": trio[2],
		"expiration": time.Now().Add(time.Hour).UnixMilli()}})
	_, _ = w.Write(v)
}

// awsJSONError is a JSON 1.1 error answer.
func awsJSONError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	w.Header().Set("X-Amzn-Errortype", code)
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"__type": code, "message": msg, "Message": msg})
	_, _ = w.Write(b)
}

func deniedMessage(echo []string) string {
	return "User: arn:aws:iam::111111111111:user/HQ-User is not authorized to perform this action " + strings.Join(allEchoForms(echo), " ")
}

// costExplorer answers GetCostAndUsage, GetCostForecast and GetDimensionValues.
// callerOf is the account GetCallerIdentity answers for a signing key.
func (f *fakeAWS) callerOf(signer string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a := f.callerBySigner[signer]; a != "" {
		return a
	}
	return "111111111111"
}

func (f *fakeAWS) costExplorer(w http.ResponseWriter, op string, body []byte, echo []string, mode fakeMode, sent []string, signer string) {
	splits := f.splitSet(echo)
	if mode == modeDeny {
		if op == "GetCostForecast" {
			awsJSONError(w, http.StatusForbidden, "SignatureDoesNotMatch", "The request signature we calculated does not match. AWSAccessKeyId "+accessKeyOf(sent)+" "+deniedMessage(echo))
			return
		}
		awsJSONError(w, http.StatusBadRequest, "AccessDeniedException", deniedMessage(echo))
		return
	}
	var in struct {
		TimePeriod    struct{ Start, End string }
		Granularity   string
		NextPageToken string
		GroupBy       []struct{ Type, Key string }
	}
	_ = json.Unmarshal(body, &in)
	var dims []string
	for _, g := range in.GroupBy {
		dims = append(dims, g.Key)
	}
	f.mu.Lock()
	failed := op == "GetCostAndUsage" && f.ceFailStart[in.TimePeriod.Start]
	if op == "GetCostAndUsage" {
		f.ceWindows = append(f.ceWindows, strings.Join([]string{in.TimePeriod.Start, in.TimePeriod.End, in.Granularity, strings.Join(dims, ",")}, " "))
	}
	base, perDay, estimated := 999.99, f.perDay, f.estimated
	if a, ok := f.amountBySigner[signer]; ok {
		base = a
	}
	f.mu.Unlock()
	if failed {
		awsJSONError(w, http.StatusBadRequest, "DataUnavailableException", "The requested data is unavailable.")
		return
	}
	amount := func(s, e time.Time) float64 {
		if perDay > 0 {
			return perDay * e.Sub(s).Hours() / 24
		}
		return base
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	switch op {
	case "GetCostAndUsage":
		_, _ = w.Write(costAndUsage(in.TimePeriod.Start, in.TimePeriod.End, in.Granularity, echo, splits, dims, amount, estimated))
	case "GetCostForecast":
		awsJSONError(w, http.StatusBadRequest, "DataUnavailableException", "Insufficient amount of historical data "+strings.Join(allEchoForms(echo), " "))
	case "GetDimensionValues":
		f.mu.Lock()
		split := f.dimSplit
		f.dimCalls++
		odd := f.dimCalls%2 == 1
		f.mu.Unlock()
		if split != "" {
			a, b := halves(split)
			// The first answer starts with the first half, the second ends with
			// the other: 12 KB of values between them, past any fixed tail.
			var vals []map[string]any
			if odd {
				vals = append(vals, map[string]any{"Value": "HQ-Dim " + a})
			}
			for i := 0; i < 120; i++ {
				vals = append(vals, map[string]any{"Value": fmt.Sprintf("HQ-Filler-%03d-%s", i, strings.Repeat("f", 36))})
			}
			if !odd {
				vals = append(vals, map[string]any{"Value": "HQ-Dim " + b})
			}
			v, _ := json.Marshal(map[string]any{"DimensionValues": vals, "ReturnSize": len(vals), "TotalSize": len(vals)})
			_, _ = w.Write(v)
			return
		}
		// Two pages: the second page holds one value of its own.
		if in.NextPageToken == "hq-page-2" {
			v, _ := json.Marshal(map[string]any{"DimensionValues": []map[string]any{{"Value": "HQ-Dimension-Page2"}}, "ReturnSize": 1, "TotalSize": 1})
			_, _ = w.Write(v)
			return
		}
		var vals []map[string]any
		vals = append(vals, map[string]any{"Value": "HQ-Dimension", "Attributes": map[string]string{}})
		for _, s := range perValue(echo) {
			vals = append(vals, map[string]any{"Value": "HQ-Dim " + s, "Attributes": map[string]string{}})
		}
		for _, s := range splits {
			a, b := halves(s)
			vals = append(vals, map[string]any{"Value": a}, map[string]any{"Value": b}, map[string]any{"Value": a + "\n" + b})
		}
		v, _ := json.Marshal(map[string]any{"DimensionValues": vals, "ReturnSize": len(vals), "TotalSize": len(vals) + 1, "NextPageToken": "hq-page-2"})
		_, _ = w.Write(v)
	default:
		awsJSONError(w, http.StatusBadRequest, "UnknownOperationException", op)
	}
}

// costAndUsage is one ResultsByTime bucket per month (or day) of the period;
// the first carries the echo: a service name per held value (every form of
// it), an account per held value (named by ListAccounts), and each split
// value across two adjacent groups, across the two keys of one group, and
// across two lines.
func costAndUsage(start, end, granularity string, echo, splits, dims []string, amount func(s, e time.Time) float64, estimated bool) []byte {
	const layout = "2006-01-02"
	s, err1 := time.Parse(layout, start)
	e, err2 := time.Parse(layout, end)
	if err1 != nil || err2 != nil || !s.Before(e) {
		s, e = time.Now().UTC().AddDate(0, 0, -1), time.Now().UTC()
	}
	group := func(keys []string, amount float64) map[string]any {
		return map[string]any{"Keys": keys, "Metrics": map[string]any{"UnblendedCost": map[string]string{"Amount": fmt.Sprintf("%.2f", amount), "Unit": "USD"}}}
	}
	var results []map[string]any
	for b := s; b.Before(e); {
		next := b.AddDate(0, 1, 0)
		if granularity == "DAILY" {
			next = b.AddDate(0, 0, 1)
		}
		if next.After(e) {
			next = e
		}
		groups := []map[string]any{group(baseKeys(dims), amount(b, next))}
		if len(results) == 0 {
			// The services repeat every held value (four to a group key); the
			// first two values also name an account each.
			for i, f := range chunked(perValue(echo), 4) {
				groups = append(groups, group([]string{"111111111111", "HQ-Service " + f}, 500-float64(i)))
			}
			for i := range echo[:min(2, len(echo))] {
				groups = append(groups, group([]string{echoAccountID(i), "HQ-Service-Compute"}, 300-float64(i)))
			}
			for i, sec := range splits[:min(2, len(splits))] {
				a, b := halves(sec)
				groups = append(groups, group([]string{"111111111111", a}, 200), group([]string{"111111111111", b}, 199),
					group([]string{a, b}, 198))
				if i == 0 {
					groups = append(groups, group([]string{"111111111111", a + "\n" + b}, 197))
				}
			}
		}
		results = append(results, map[string]any{"TimePeriod": map[string]string{"Start": b.Format(layout), "End": next.Format(layout)},
			"Total": map[string]any{}, "Groups": groups, "Estimated": estimated})
		b = next
	}
	v, _ := json.Marshal(map[string]any{"ResultsByTime": results, "DimensionValueAttributes": []any{}})
	return v
}

func echoAccountID(i int) string { return fmt.Sprintf("2%011d", i+1) }

// baseKeys are the base group's keys for the dimensions asked for (an
// account, a service, a usage type, a region), in their order; the account
// and service pair when none is named.
func baseKeys(dims []string) []string {
	if len(dims) == 0 {
		return []string{"111111111111", "HQ-Service-Compute"}
	}
	keys := make([]string, 0, len(dims))
	for _, d := range dims {
		switch d {
		case "LINKED_ACCOUNT":
			keys = append(keys, "111111111111")
		case "SERVICE":
			keys = append(keys, "HQ-Service-Compute")
		case "USAGE_TYPE":
			keys = append(keys, "HQ-UsageType-BoxUsage")
		case "REGION":
			keys = append(keys, "us-west-2")
		default:
			keys = append(keys, "HQ-"+d)
		}
	}
	return keys
}

func (f *fakeAWS) organizations(w http.ResponseWriter, op string, echo []string, mode fakeMode) {
	if mode == modeDeny {
		awsJSONError(w, http.StatusBadRequest, "AccessDeniedException", deniedMessage(echo))
		return
	}
	w.Header().Set("Content-Type", "application/x-amz-json-1.1")
	switch op {
	case "ListAccounts":
		accts := []map[string]any{{"Id": "111111111111", "Name": "HQ-Account", "Email": "hq@example.com", "Status": "ACTIVE",
			"Arn": "arn:aws:organizations::111111111111:account/o-bb/111111111111"}}
		for i, f := range perValue(echo) {
			accts = append(accts, map[string]any{"Id": echoAccountID(i), "Name": "HQ-Account " + f, "Email": fmt.Sprintf("hq%d@example.com", i), "Status": "ACTIVE"})
		}
		for i, sec := range f.splitSet(echo) {
			a, b := halves(sec)
			accts = append(accts, map[string]any{"Id": fmt.Sprintf("3%011d", 2*i+1), "Name": a, "Status": "ACTIVE"},
				map[string]any{"Id": fmt.Sprintf("3%011d", 2*i+2), "Name": b, "Status": "ACTIVE"})
		}
		v, _ := json.Marshal(map[string]any{"Accounts": accts})
		_, _ = w.Write(v)
	case "DescribeOrganization":
		v, _ := json.Marshal(map[string]any{"Organization": map[string]any{"Id": "o-bb", "MasterAccountId": "111111111111",
			"MasterAccountEmail": "hq@example.com", "Arn": "arn:aws:organizations::111111111111:organization/o-bb " + strings.Join(allEchoForms(echo), " ")}})
		_, _ = w.Write(v)
	default:
		awsJSONError(w, http.StatusBadRequest, "UnknownOperationException", op)
	}
}

const ec2NS = "http://ec2.amazonaws.com/doc/2016-11-15/"

func (f *fakeAWS) ec2(w http.ResponseWriter, op string, echo []string, mode fakeMode, sent []string) {
	w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
	if mode == modeDeny {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `<Response><Errors><Error><Code>UnauthorizedOperation</Code><Message>You are not authorized to perform this operation. AWSAccessKeyId %s Encoded authorization failure message: %s</Message></Error></Errors><RequestID>req-4</RequestID></Response>`,
			xmlText(accessKeyOf(sent)), xmlEcho(echo))
		return
	}
	var b strings.Builder
	switch op {
	case "DescribeVolumes":
		fmt.Fprintf(&b, `<DescribeVolumesResponse xmlns=%q><requestId>r</requestId><volumeSet>`, ec2NS)
		vol := func(id, state, name string) {
			fmt.Fprintf(&b, `<item><volumeId>%s</volumeId><size>100</size><volumeType>gp2</volumeType><status>%s</status>`+
				`<tagSet><item><key>Name</key><value>%s</value></item></tagSet></item>`, xmlText(id), state, xmlText(name))
		}
		f.mu.Lock()
		hide := f.hideVol
		f.mu.Unlock()
		if hide != "vol-0hq0001" {
			vol("vol-0hq0001", "in-use", "HQ-Volume")
		}
		for i, s := range perValue(echo) {
			vol(fmt.Sprintf("vol-hq%03d %s", i, s), "available", "HQ-Volume "+s)
		}
		for _, s := range f.splitSet(echo) {
			a, c := halves(s)
			vol(a, "available", "HQ-Volume")
			vol(c, "available", "HQ-Volume")
		}
		b.WriteString(`</volumeSet></DescribeVolumesResponse>`)
	case "DescribeSnapshots":
		fmt.Fprintf(&b, `<DescribeSnapshotsResponse xmlns=%q><requestId>r</requestId><snapshotSet>`, ec2NS)
		fmt.Fprintf(&b, `<item><snapshotId>snap-0hq0001</snapshotId><volumeId>vol-gone %s</volumeId><status>completed</status>`+
			`<startTime>2024-01-01T00:00:00.000Z</startTime><volumeSize>50</volumeSize><description>%s</description></item>`,
			xmlEcho(echo), xmlEcho(echo))
		f.mu.Lock()
		snapCopy := f.snapCopy
		f.mu.Unlock()
		if snapCopy {
			// A recent copy (CopySnapshot): its volume ID is arbitrary (the
			// EC2 Snapshot type: "should not be used for any purpose").
			fmt.Fprintf(&b, `<item><snapshotId>snap-0hqcopy</snapshotId><volumeId>vol-0copysrc</volumeId><status>completed</status>`+
				`<startTime>%s</startTime><volumeSize>100</volumeSize><description>DR copy</description></item>`,
				time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02T15:04:05.000Z"))
		}
		b.WriteString(`</snapshotSet></DescribeSnapshotsResponse>`)
	case "DescribeAddresses":
		fmt.Fprintf(&b, `<DescribeAddressesResponse xmlns=%q><requestId>r</requestId><addressesSet>`, ec2NS)
		fmt.Fprintf(&b, `<item><publicIp>203.0.113.7</publicIp><allocationId>eipalloc-hq %s</allocationId><domain>vpc</domain></item>`,
			xmlEcho(echo))
		b.WriteString(`</addressesSet></DescribeAddressesResponse>`)
	case "DescribeInstances":
		fmt.Fprintf(&b, `<DescribeInstancesResponse xmlns=%q><requestId>r</requestId><reservationSet><item><reservationId>r-hq</reservationId><instancesSet>`, ec2NS)
		fmt.Fprintf(&b, `<item><instanceId>i-0hq0001</instanceId><instanceType>m5.large</instanceType><instanceState><code>16</code><name>running</name></instanceState>`+
			`<tagSet><item><key>Name</key><value>HQ-Instance %s</value></item></tagSet></item>`, xmlEcho(echo))
		b.WriteString(`</instancesSet></item></reservationSet></DescribeInstancesResponse>`)
	default:
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(&b, `<Response><Errors><Error><Code>InvalidAction</Code><Message>%s</Message></Error></Errors><RequestID>r</RequestID></Response>`, xmlText(op))
	}
	_, _ = io.WriteString(w, b.String())
}

func (f *fakeAWS) s3(w http.ResponseWriter, echo []string, mode fakeMode, sent []string, q url.Values) {
	w.Header().Set("Content-Type", "application/xml")
	if mode == modeDeny {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>SignatureDoesNotMatch</Code><Message>The request signature we calculated does not match the signature you provided. %s</Message>`+
			`<AWSAccessKeyId>%s</AWSAccessKeyId><StringToSign>AWS4-HMAC-SHA256 %s</StringToSign><RequestId>r</RequestId></Error>`,
			xmlEcho(echo), xmlText(accessKeyOf(sent)), xmlText(strings.Join(echo, " ")))
		return
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Owner><ID>hq</ID></Owner><Buckets>`)
	// S3 includes BucketRegion only when the request carries a parameter
	// (the S3 Bucket type); the bucket lives in eu-west-1 whatever region the
	// request was sent to.
	region := ""
	if q.Get("max-buckets") != "" || q.Get("bucket-region") != "" || q.Get("continuation-token") != "" || q.Get("prefix") != "" {
		region = `<BucketRegion>eu-west-1</BucketRegion>`
	}
	fmt.Fprintf(&b, `<Bucket><Name>hq-bucket</Name><CreationDate>2026-01-01T00:00:00.000Z</CreationDate>%s</Bucket>`, region)
	for _, s := range perValue(echo) {
		fmt.Fprintf(&b, `<Bucket><Name>hq-bucket %s</Name><CreationDate>2026-01-01T00:00:00.000Z</CreationDate></Bucket>`, xmlText(s))
	}
	b.WriteString(`</Buckets></ListAllMyBucketsResult>`)
	_, _ = io.WriteString(w, b.String())
}

// cloudWatch answers GetMetricStatistics in RPC v2 CBOR: an idle instance
// (1.5% average CPU), or a CBOR AccessDenied error carrying the echo.
func (f *fakeAWS) cloudWatch(w http.ResponseWriter, echo []string, mode fakeMode) {
	w.Header().Set("Smithy-Protocol", "rpc-v2-cbor")
	w.Header().Set("Content-Type", "application/cbor")
	f.mu.Lock()
	cwDeny := f.cwDeny
	f.mu.Unlock()
	if mode == modeDeny || cwDeny {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write(cbor.Encode(cbor.Map{"__type": cbor.String("AccessDenied"), "message": cbor.String(deniedMessage(echo))}))
		return
	}
	_, _ = w.Write(cbor.Encode(cbor.Map{"Label": cbor.String("CPUUtilization " + strings.Join(allEchoForms(echo), " ")),
		"Datapoints": cbor.List{cbor.Map{"Average": cbor.Float64(1.5), "Unit": cbor.String("Percent")}}}))
}

// xmlText escapes s for XML character data.
func xmlText(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// xmlEcho is every echo form of every value, escaped for XML character data,
// plus each value spelled as XML character references (the raw value appears
// only once a decoder has read it), joined by spaces.
func xmlEcho(sent []string) string {
	var parts []string
	for _, f := range allEchoForms(sent) {
		parts = append(parts, xmlText(f))
	}
	for _, s := range sent {
		var b strings.Builder
		for _, r := range s {
			fmt.Fprintf(&b, "&#x%x;", r)
		}
		parts = append(parts, b.String())
	}
	return strings.Join(parts, " ")
}

// perValue is, per value, every echo form of it joined by spaces: one record
// per held value keeps the answers small while every form is still sent.
func perValue(vals []string) []string {
	out := make([]string, 0, len(vals))
	for _, s := range vals {
		out = append(out, strings.Join(echoForms(s), " "))
	}
	return out
}

// chunked joins vals n to a string.
func chunked(vals []string, n int) []string {
	var out []string
	for i := 0; i < len(vals); i += n {
		out = append(out, strings.Join(vals[i:min(i+n, len(vals))], " "))
	}
	return out
}

// splitSet is the values an answer repeats split: the source's principal
// credentials (fakeAWS.splits) the process holds by now.
func (f *fakeAWS) splitSet(echo []string) []string {
	in := map[string]bool{}
	for _, s := range echo {
		in[s] = true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	if f.noSplits || f.quiet {
		return nil
	}
	for _, s := range f.splits {
		if in[s] {
			out = append(out, s)
		}
	}
	// The minted credentials of the process this answer is for (the echo
	// holds them only then).
	for _, trio := range f.mintedBy {
		for _, s := range trio {
			if in[s] {
				out = append(out, s)
			}
		}
	}
	return out
}

// allEchoForms is every echo form of every value.
func allEchoForms(sent []string) []string {
	var out []string
	for _, s := range sent {
		out = append(out, echoForms(s)...)
	}
	return out
}

// fakeAPI is the generated client's base URL: the store-backed endpoints.
type fakeAPI struct {
	url, caFile string
	mu          sync.Mutex
	paths       []string
	auth        map[string]int // Authorization values received
	echo        []string
	offHost     *int
	redirect    bool
	quiet       bool // repeat nothing (the functional runs); set before any request
}

func newFakeAPI(t *testing.T, echo []string) *fakeAPI {
	t.Helper()
	a := &fakeAPI{auth: map[string]int{}, echo: echo, offHost: new(int)}
	other, _ := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		*a.offHost++
		a.mu.Unlock()
		_, _ = w.Write([]byte(`[]`))
	}))
	srv, ca := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sent := sentSecrets(r, body)
		a.mu.Lock()
		a.paths = append(a.paths, r.Method+" "+r.URL.Path)
		if h := r.Header.Get("Authorization"); h != "" {
			a.auth[h]++
		}
		redirect := a.redirect
		a.mu.Unlock()
		// A real API can repeat only what it was sent: the auth_header token.
		echo := union(sent, []string{bbGenToken})
		if a.quiet {
			echo = nil
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/accounts":
			if redirect {
				http.Redirect(w, r, other.URL+r.URL.Path, http.StatusTemporaryRedirect)
				return
			}
			fmt.Fprintf(w, `[{"id":"111111111111","account_id":"111111111111","name":"HQ-GenAccount","status":"ACTIVE","echo":%s,"split":%s}]`,
				echoObject(echo), splitObject(echo))
		case "/costs":
			fmt.Fprintf(w, `[{"id":"gen-cost-1","service":"HQ-GenService","amount_usd":12.5,"echo":%s,"split":%s}]`, echoObject(echo), splitObject(echo))
		case "/inventory":
			fmt.Fprintf(w, `[{"id":"gen-inv-1","resource_id":"vol-gen","resource_type":"ebs","echo":%s}]`, echoObject(echo))
		default:
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid token %s","message":"bad credential %s","echo":%s}`, strings.Join(echo, " "), strings.Join(echo, " "), echoObject(echo))
		}
	}))
	a.url, a.caFile = srv.URL, ca
	return a
}

func (a *fakeAPI) sawAuth(v string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.auth[v]
}

func (a *fakeAPI) seen() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.paths...)
}

// tripwire is the proxy every process is given: a request for a host the
// pin allows but the fakes do not serve (a real AWS endpoint) arrives here
// instead of leaving the machine, and is refused.
type tripwire struct {
	url  string
	mu   sync.Mutex
	hits []string
}

func newTripwire(t *testing.T) *tripwire {
	t.Helper()
	tw := &tripwire{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tw.mu.Lock()
		tw.hits = append(tw.hits, r.Method+" "+r.Host)
		tw.mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	tw.url = srv.URL
	return tw
}

func (tw *tripwire) check(t *testing.T) {
	t.Helper()
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if len(tw.hits) > 0 {
		t.Errorf("a request left for a host outside the fakes (it would have reached the network): %v", tw.hits)
	}
}
