// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bufio"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"threecx-pp-cli/internal/credkit"
)

// heldBy records what fn holds, and holds nothing in the kit: its held set is
// process-wide with no reset.
func heldBy(t *testing.T, fn func()) []string {
	t.Helper()
	var held []string
	old := Hold
	Hold = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	defer func() { Hold = old }()
	fn()
	return held
}

// Class minted-credential-not-held: every value a minting function answers
// with is held, whatever record shape carries it (Pbx.CrmChoice {Key, Value}
// from GetOAuth, an unnamed {"value"}), and every XAPI function that mints or
// grants access is a minting function.
func TestMintedAnswerHoldsEveryValue(t *testing.T) {
	cases := []struct{ path, body, secret string }{
		{"/xapi/v1/CrmIntegration/Pbx.GetOAuth(variable='OAuth')", `{"@odata.context":"x","Key":"OAuth","Value":"crmTok-77aa31bc"}`, "crmTok-77aa31bc"},
		{"/xapi/v1/CrmIntegration/Pbx.GetOAuth(variable='OAuth')", `{"Key":"OAuth","Value":"{\"access_token\":\"crmNested-5c2e\"}"}`, "crmNested-5c2e"},
		{"/xapi/v1/SystemStatus/Pbx.GetRequestHelpLink(grantPeriodDays=1)", `{"value":"https://help.example/grant/a1b2c3d4e5"}`, "https://help.example/grant/a1b2c3d4e5"},
		{"/xapi/v1/ServicePrincipals(4)/Pbx.GenerateAppToken", `{"value":"appTok-91c3e7a5"}`, "appTok-91c3e7a5"},
		{"/xapi/v1/Users/Pbx.RegeneratePasswords", `{"value":"regenPw-3f9a1c7e"}`, "regenPw-3f9a1c7e"},
	}
	for _, c := range cases {
		if !MintsCredential(c.path) {
			t.Errorf("%s is not treated as a minting function", c.path)
		}
		held := heldBy(t, func() { HoldCredentialFields([]byte(c.body), MintsCredential(c.path)) })
		if !slices.Contains(held, c.secret) {
			t.Errorf("%s: %q from %s not held (held %q)", c.path, c.secret, c.body, held)
		}
		// A minting answer's record name is not the credential.
		if slices.Contains(held, "OAuth") {
			t.Error("the CrmChoice Key (a variable name) was held")
		}
	}
}

// Class credential-field-not-recognized: the OAuth authorization-code flow's
// PKCE verifier and code (Pbx.OauthStateParam.PKCECodeVerifier; the
// McpServers/Pbx.Exchange body {code, verifier, tokenEndpoint, scope}) are
// credentials: held from a tool argument or request, redacted where named.
// An OData error's "code" stays readable.
func TestOAuthCodeAndVerifierAreCredentials(t *testing.T) {
	for _, n := range []string{"PKCECodeVerifier", "verifier", "code_verifier", "CodeVerifier"} {
		if !SecretFieldName(n) {
			t.Errorf("%s is not a credential name", n)
		}
	}
	held := heldBy(t, func() {
		HoldCredentialValues(map[string]any{"state": map[string]any{"Variable": "crm", "PKCECodeVerifier": "pkceVer-0a1b2c3d4e5f"}})
		HoldCredentialValues(map[string]any{"code": "authCode-6a7b8c9d", "verifier": "exchVer-1f2e3d4c", "tokenEndpoint": "https://idp.example/token", "scope": "mcp"})
	})
	for _, s := range []string{"pkceVer-0a1b2c3d4e5f", "authCode-6a7b8c9d", "exchVer-1f2e3d4c"} {
		if !slices.Contains(held, s) {
			t.Errorf("%s not held", s)
		}
	}
	red := RedactSecretFields(map[string]any{"code": "authCode-6a7b8c9d", "verifier": "v", "tokenEndpoint": "https://idp.example/token"}).(map[string]any)
	if red["code"] != RedactedValue || red["verifier"] != RedactedValue {
		t.Errorf("exchange body not redacted: %v", red)
	}
	errBody := RedactSecretFields(map[string]any{"error": map[string]any{"code": "Forbidden", "message": "no"}}).(map[string]any)
	if errBody["error"].(map[string]any)["code"] != "Forbidden" {
		t.Errorf("an OData error code was redacted: %v", errBody)
	}
}

// credentialVocabulary is broader than any rule: every spec name it matches
// must be judged, either a credential (SecretFieldName) or listed below.
var credentialVocabulary = regexp.MustCompile(`(?i)(pass|secret|cred|token|key|pin|verif|auth|cert|code|otp|hash|salt|sign|nonce|cookie|session|private|hmac|bearer|licen|login|digest|sid$|link|ur[il]$)`)

// publicSpecNames are names in spec.yaml that match credentialVocabulary but
// carry no credential (reviewed against 3CX's XAPI swagger, round 1 of the
// 4.34.2 review): routing data, codes of countries, codecs and dial plans,
// booleans that choose what to regenerate, public certificates, links to
// files and recordings that need the bearer token to fetch, schema keywords.
// Adding a name here is a review decision; a credential belongs in the rule.
var publicSpecNames = map[string]string{}

func init() {
	for _, n := range strings.Fields(`
AccountSid AlterDestinationDuringHolidays AlterDestinationDuringOutOfOfficeHours ArchivedUrl AreaCode
AssignClearOperations Assigned AssignedFirmware AssignedUser AuthenticateMode Authentication
AuthenticationMessage AuthId AuthID AuthorizationEndpoint BrandUrl ButtonIconUrl CACertificate Certificate
CertificateExpirationDate CertificateName ClientCertificate ClientUri Code Codec CodeChallenge Codecs
ConfigurationLink ContactUrl CountryCode CountryCodes CultureCode DescriptionLink DestNumberInRequestLineURI
DialCodeBillingCode DialCodeHideCallerID DialCodeHotdesking DialCodeHotelAccess DialCodeIntercom
DialCodeLoggedInQueue DialCodeLoggedOutQueue DialCodeOutOffice DialCodePark DialCodePickup
DialCodeProfileStatus DialCodeUnpark DialCodeVMail DialPlanCode DirectSIPAllowExternal DirectSIPLocalDomain
DownloadLink DownloadUrl E164CountryCode ErpCode ExchangeServerUrl ExitCode ExternalCodecList FileLink FixUrl
FtpValidateCertificate GoogleSignInEnabled HackAuthRequests HotdeskingAssignment InterfaceLink
InternationalCode InvalidKeyForwardDN IsExtensionSignInEnabled JwksUri Keys LanguageCode LastLoginTime License
LicenseActive licenseParams Link LocalCodecList LoginType LogoUri MS365SignInEnabled MulticastCodec
NationalCode NextPageToken NotifyCodes OAuthCimdEnabled OAuthDcrEnabled OAuthOpenIdConnectEnabled openlink
ParentErpCode PassiveServer PassiveServerIsEnabled PhotoUrl PinProtectTimeout playlistKey PolicyUri
PrivateMemory privaterooms ProductCode ProviderUri RecordingUrl RedirectUri RedirectUris RejectedCode
RemoteMyPhoneUriHost RemoveAreaCode RemoveCountryCode ReportLink RequestUrl RpsKey SbcCertificate
SbcCertificateExpirationDate ScriptCode SecureSipEnabled SeparateAuthId Sessionid SftpHostKeyFingerprint
SipAuth StartupLicense TokenEndpoint tokenEndpoint TokenEndpointAuthMethod TosUri URL Url UseSeparateAuthId
ValidateServerCertificate VMDisablePinAuth groupingType Footprint footprint AnalysisCode Stopping
RequestUrlParameters MaxSimCalls IPInRegistrationContact LlDpInfo PublicIPinSIP SessionsList Sid UseIPInContact
YealinkUpdateInterval`) {
		publicSpecNames[n] = "reviewed"
	}
	// "code" is an OData error code or a status code everywhere but the
	// McpServers/Pbx.Exchange body, where the shape rule (OAuthCodeName: a
	// code beside a verifier or token endpoint) makes it a credential.
	publicSpecNames["code"] = "credential only by shape (OAuthCodeName)"
}

// specNames returns every property name (a key whose parent key is
// "properties") and every parameter name ("- name: x" under "parameters") in
// spec.yaml.
func specNames(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("../../spec.yaml")
	if err != nil {
		t.Skipf("spec.yaml not found: %v", err)
	}
	defer f.Close()
	key := regexp.MustCompile(`^( *)(- +)?(['"]?)([^'":#\s][^'":]*)(['"]?)\s*:(\s|$)`)
	type frame struct {
		indent int
		key    string
	}
	var stack []frame
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		m := key.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		indent := len(m[1]) + len(m[2])
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		parent := ""
		if len(stack) > 0 {
			parent = stack[len(stack)-1].key
		}
		switch {
		case parent == "properties":
			seen[m[4]] = true
		case m[2] != "" && m[4] == "name" && parent == "parameters":
			if v := strings.TrimSpace(strings.SplitN(sc.Text(), ":", 2)[1]); v != "" {
				seen[strings.Trim(v, `'"`)] = true
			}
		}
		stack = append(stack, frame{indent, m[4]})
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	if len(out) < 1000 {
		t.Fatalf("read only %d names from spec.yaml; the scan is broken", len(out))
	}
	return out
}

// Class credential-field-not-recognized, closed for the whole vendor schema:
// every property and parameter name in spec.yaml that looks like it could
// carry a credential is either recognized by the one rule or reviewed as
// public. A name a reprint's spec adds fails here until it is judged.
func TestEverySpecNameThatCouldCarryACredentialIsJudged(t *testing.T) {
	for _, n := range specNames(t) {
		if strings.HasPrefix(n, "x-") || strings.HasPrefix(n, "@") || strings.HasPrefix(n, "$") {
			continue
		}
		if !credentialVocabulary.MatchString(n) || SecretFieldName(n) {
			continue
		}
		if _, ok := publicSpecNames[n]; !ok {
			t.Errorf("spec name %q may carry a credential: add it to the rule (threecx_secrets.go) or review it into publicSpecNames", n)
		}
	}
}

// Class credential-misread-as-mask (review round 2): a word is never a mask. A
// password that is literally "hidden" is held; only 3CX's own asterisks, empty
// values and this program's redaction marker are skipped, by shape.
func TestAWordIsNeverAMask(t *testing.T) {
	held := heldBy(t, func() {
		HoldCredentialValues(map[string]any{"AuthPassword": "hidden", "Password": "masked", "FtpPassword": "withheld", "MailPassword": "****", "NsPassword": "[redacted]", "SftpPassword": ""})
	})
	for _, v := range []string{"hidden", "masked", "withheld"} {
		if !slices.Contains(held, v) {
			t.Errorf("the password %q was taken for a mask (held %q)", v, held)
		}
	}
	for _, v := range []string{"****", "[redacted]", ""} {
		if slices.Contains(held, v) {
			t.Errorf("the mask %q was held", v)
		}
	}
}

// Every member of a duplicate key is held, whichever member of a record repeats,
// and so is every document in a body.
func TestEveryDuplicateMemberIsHeld(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{`{"SecretKey":{"Concealed":false,"Value":"dupValue-1a2b"},"SecretKey":null}`, "dupValue-1a2b"},
		{`{"AuthPassword":"dupFirst-3c4d","AuthPassword":"second"}`, "dupFirst-3c4d"},
		{`{"AuthPassword":"first","AuthPassword":"dupLast-5e6f"}`, "dupLast-5e6f"},
		{`{"Concealed":true,"Value":"dupConcealed-7a8b","Value":"x"}`, "dupConcealed-7a8b"},
		{`{"Name":"plain","Name":"CLIENT_SECRET","Value":"dupName-9c0d"}`, "dupName-9c0d"},
		{`{"Name":"CLIENT_SECRET","Name":"plain","Value":"a","Value":"dupPair-1e2f"}`, "dupPair-1e2f"},
		{`{"value":[{"Mystery":{"Concealed":1,"Value":"dupNested-3a4b"},"Mystery":1}]}`, "dupNested-3a4b"},
		{`{"Number":"1001"} {"AuthPassword":"secondDoc-5c6d"}`, "secondDoc-5c6d"},
	} {
		if held := heldBy(t, func() { HoldCredentialFields([]byte(c.body)) }); !slices.Contains(held, c.want) {
			t.Errorf("%s: %q not held, got %v", c.body, c.want, held)
		}
	}
}
