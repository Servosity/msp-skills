// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"cisco-umbrella-pp-cli/internal/credwire"
)

const (
	otsNoBody     = ""
	otsOwnSecret  = `{"name":"t","authentication":{"parameters":{"idPrefix":"x","secret":"OwnChosenPsk7Qk2"}}}`
	otsOwnPSK     = `{"autoRotate":false,"psk":{"idPrefix":"x","secret":"OwnChosenPsk7Qk2"}}`
	otsAPIKeys    = "https://api.umbrella.com/admin/v2/apiKeys"
	otsRotate     = "https://api.umbrella.com/admin/v2/iam/rotateKey"
	otsTunnels    = "https://api.umbrella.com/deployments/v2/tunnels"
	otsTunnelKeys = "https://api.umbrella.com/deployments/v2/tunnels/7/keys"
)

// refuseOneTimeSecret refuses by route and, for the two tunnel routes, by what the
// body asks Umbrella to generate. Spellings a server may read as the same route are
// the same route, and a body it cannot prove safe is refused.
func TestRefuseOneTimeSecret(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, method, url, body string
		refused                 bool
	}{
		{"create", "POST", otsAPIKeys, otsNoBody, true},
		{"create, lower-case method", "post", otsAPIKeys, otsNoBody, true},
		{"create, trailing slash", "POST", otsAPIKeys + "/", otsNoBody, true},
		{"create, doubled slash", "POST", "https://api.umbrella.com//admin/v2/apiKeys", otsNoBody, true},
		{"create, dot segment", "POST", "https://api.umbrella.com/admin/./v2/x/../apiKeys", otsNoBody, true},
		{"create, other case", "POST", "https://api.umbrella.com/ADMIN/v2/APIKEYS", otsNoBody, true},
		{"create, a query", "POST", otsAPIKeys + "?a=b", otsNoBody, true},
		{"create, a base URL with a path", "POST", "https://proxy.example/umbrella" + "/admin/v2/apiKeys", otsNoBody, true},
		{"refresh", "POST", otsAPIKeys + "/k1/refresh", otsNoBody, true},
		{"refresh, an id that decodes to a slash", "POST", otsAPIKeys + "/a%2Fb/refresh", otsNoBody, true},
		{"rotate", "POST", otsRotate, otsNoBody, true},
		{"add, no body", "POST", otsTunnels, otsNoBody, true},
		{"add, no secret", "POST", otsTunnels, `{"name":"t"}`, true},
		{"add, an id prefix only", "POST", otsTunnels, `{"name":"t","authentication":{"parameters":{"idPrefix":"x"}}}`, true},
		{"add, blank secret", "POST", otsTunnels, `{"name":"t","authentication":{"parameters":{"secret":"  "}}}`, true},
		{"add, null secret", "POST", otsTunnels, `{"name":"t","authentication":{"parameters":{"secret":null}}}`, true},
		{"add, a secret that is not a string", "POST", otsTunnels, `{"name":"t","authentication":{"parameters":{"secret":7}}}`, true},
		{"add, the response's path", "POST", otsTunnels, `{"name":"t","client":{"authentication":{"parameters":{"secret":"S3cret"}}}}`, true},
		{"add, not JSON", "POST", otsTunnels, `name=t`, true},
		{"add, an array", "POST", otsTunnels, `[{"authentication":{"parameters":{"secret":"S3cret"}}}]`, true},
		{"add, a second document", "POST", otsTunnels, otsOwnSecret + ` {"name":"u"}`, true},
		{"add, a duplicate member", "POST", otsTunnels, `{"authentication":{"parameters":{"secret":"S3cret"}},"authentication":{}}`, true},
		{"add, a duplicate nested member", "POST", otsTunnels, `{"authentication":{"parameters":{"secret":"S3cret","secret":""}}}`, true},
		{"keys, no body", "POST", otsTunnelKeys, otsNoBody, true},
		{"keys, auto-rotate", "POST", otsTunnelKeys, `{"autoRotate":true}`, true},
		{"keys, auto-rotate with a secret", "POST", otsTunnelKeys, `{"autoRotate":true,"psk":{"idPrefix":"x","secret":"S3cret"}}`, true},
		{"keys, no autoRotate", "POST", otsTunnelKeys, `{"psk":{"idPrefix":"x","secret":"S3cret"}}`, true},
		{"keys, autoRotate as a string", "POST", otsTunnelKeys, `{"autoRotate":"false","psk":{"secret":"S3cret"}}`, true},
		{"keys, autoRotate null", "POST", otsTunnelKeys, `{"autoRotate":null,"psk":{"secret":"S3cret"}}`, true},
		{"keys, no secret", "POST", otsTunnelKeys, `{"autoRotate":false,"psk":{"idPrefix":"x"}}`, true},
		{"keys, a duplicate autoRotate", "POST", otsTunnelKeys, `{"autoRotate":false,"autoRotate":true,"psk":{"secret":"S3cret"}}`, true},
		{"keys, not JSON", "POST", otsTunnelKeys, `nope`, true},
		{"add, own secret", "POST", otsTunnels, otsOwnSecret, false},
		{"keys, own secret", "POST", otsTunnelKeys, otsOwnPSK, false},
		{"list keys", "GET", otsAPIKeys, otsNoBody, false},
		{"get a key", "GET", otsAPIKeys + "/k1", otsNoBody, false},
		{"update a key", "PATCH", otsAPIKeys + "/k1", otsNoBody, false},
		{"delete a key", "DELETE", otsAPIKeys + "/k1", otsNoBody, false},
		{"list tunnels", "GET", otsTunnels, otsNoBody, false},
		{"update a tunnel", "PUT", otsTunnels + "/7", otsNoBody, false},
		{"delete a tunnel", "DELETE", otsTunnels + "/7", otsNoBody, false},
		{"a name that ends the same", "POST", "https://api.umbrella.com/deployments/v2/mytunnels", otsNoBody, false},
		{"another list", "POST", "https://api.umbrella.com/policies/v2/destinationlists", `{}`, false},
	} {
		err := refuseOneTimeSecret(c.method, c.url, []byte(c.body))
		if got := errors.Is(err, ErrOneTimeSecret); got != c.refused {
			t.Errorf("%s: refused = %v, want %v (%v)", c.name, got, c.refused, err)
		}
	}
}

type recordingTripper struct{ hits int }

func (r *recordingTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.hits++
	return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingBody) Close() error             { return nil }

// The tripper in the pinned chain sees every hop, a redirect into a refused route
// included, and leaves the body a passed request carries for the next tripper.
func TestOneTimeSecretTripperRefusesEveryHop(t *testing.T) {
	t.Parallel()
	next := &recordingTripper{}
	tr := oneTimeSecretTripper{next: next}
	req, _ := http.NewRequest("POST", otsTunnels, strings.NewReader(`{"name":"t"}`))
	if _, err := tr.RoundTrip(req); !errors.Is(err, ErrOneTimeSecret) || next.hits != 0 {
		t.Errorf("a refused request: %v, %d sent", err, next.hits)
	}
	req, _ = http.NewRequest("POST", otsTunnels, failingBody{})
	if _, err := tr.RoundTrip(req); !errors.Is(err, ErrOneTimeSecret) || next.hits != 0 {
		t.Errorf("an unreadable body: %v, %d sent", err, next.hits)
	}
	var got []byte
	pass := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		got, _ = io.ReadAll(r.Body)
		return &http.Response{StatusCode: 200, Body: http.NoBody}, nil
	})
	req, _ = http.NewRequest("POST", otsTunnels, strings.NewReader(otsOwnSecret))
	if _, err := (oneTimeSecretTripper{next: pass}).RoundTrip(req); err != nil || string(got) != otsOwnSecret {
		t.Errorf("a passed request: %v, the next tripper read %q", err, got)
	}

	// The real chain: a same-origin 307 into a refused route is refused at the hop.
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Path)
		if r.URL.Path != "/admin/v2/apiKeys" {
			w.Header().Set("Location", "/admin/v2/apiKeys")
			w.WriteHeader(http.StatusTemporaryRedirect)
		}
	}))
	t.Cleanup(srv.Close)
	hc := &http.Client{Transport: http.DefaultTransport}
	pinClient(hc, pinnedOrigins(srv.URL, srv.URL+"/auth/v2/token"))
	resp, err := hc.Post(srv.URL+"/policies/v2/destinationlists", "application/json", strings.NewReader(`{}`))
	if err == nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrOneTimeSecret) || len(seen) != 1 {
		t.Errorf("a redirect into a refused route: %v, requests %v", err, seen)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// writeRoutes classifies every non-GET route in the press manifest, so one a reprint
// adds fails until someone reads what it returns. Each entry is "refused" (the guard
// refuses it), "conditional" (the guard reads its body) or the reason its response
// loses nothing. The spec the press read is not in the repository; the manifest and
// internal/types are its copies here, as in TestDeclaredFieldsMatchTheSpec.
var writeRoutes = map[string]string{
	"POST /admin/v2/apiKeys":                    "refused",
	"POST /admin/v2/apiKeys/{apiKeyId}/refresh": "refused",
	"POST /admin/v2/iam/rotateKey":              "refused",
	"POST /deployments/v2/tunnels":              "conditional",
	"POST /deployments/v2/tunnels/{id}/keys":    "conditional",
	"PATCH /admin/v2/apiKeys/{apiKeyId}":        "returns the key's existing clientId (KeyForList's, readable from GET), and no secret",
	"POST /admin/v2/config/logos":               "Logo.token is the logo's random number, also on GET /admin/v2/config/logos; nothing is shown only once",
	"PUT /admin/v2/config/logos/{logoId}":       "Logo.token, as for POST /admin/v2/config/logos",
	"POST /admin/v2/users":                      "UserObject.password is the password the operator just chose; Umbrella generates nothing",
}

// credentialTypes names every struct in internal/types holding a declared credential
// field, with the route that returns it or why it loses nothing. A new credential
// field, in any struct, fails until someone decides.
var credentialTypes = map[string]string{
	"ApiKeysCreateResponse.key":     "POST /admin/v2/apiKeys",
	"ApiKeysRefreshResponse.key":    "POST /admin/v2/apiKeys/{apiKeyId}/refresh",
	"ApiKeysUpdateResponse.key":     "PATCH /admin/v2/apiKeys/{apiKeyId}",
	"ApiKeysGetResponse.key":        "GET: a read",
	"Key.clientId":                  "PATCH and GET: the key's existing clientId",
	"KeyForList.clientId":           "GET: a read",
	"KeyRefreshSecret.clientId":     "POST /admin/v2/apiKeys/{apiKeyId}/refresh",
	"KeyRefreshSecret.clientSecret": "POST /admin/v2/apiKeys/{apiKeyId}/refresh",
	"KeyWithSecret.clientId":        "POST /admin/v2/apiKeys",
	"KeyWithSecret.clientSecret":    "POST /admin/v2/apiKeys",
	"KeyRotation.currentKeyId":      "POST /admin/v2/iam/rotateKey",
	"KeyRotation.oldKeyId":          "POST /admin/v2/iam/rotateKey",
	"KeyRotation.secretAccessKey":   "POST /admin/v2/iam/rotateKey",
	"Logo.token":                    "POST /admin/v2/config/logos",
	"UserObject.password":           "POST /admin/v2/users",
	"parametersWithSecret.secret":   "POST /deployments/v2/tunnels and POST /deployments/v2/tunnels/{id}/keys",
}

// Every non-GET whose documented success response has a declared credential field is
// refused or named with a reason. Bound both ways to the repository's copies of the
// spec, so a reprint or a spec change fails here.
func TestEveryCredentialBearingWriteIsDecided(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "press", "tools-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Tools []struct {
			Name, Method, Path, Description string
		} `json:"tools"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	var writes []string
	seen := map[string]bool{}
	for _, tool := range manifest.Tools {
		if tool.Method == http.MethodGet {
			continue
		}
		route := tool.Method + " " + tool.Path
		seen[route] = true
		writes = append(writes, route)
		verdict, listed := writeRoutes[route]
		if !listed {
			if !noCredentialInResponse[route] {
				t.Errorf("%s is a write the review has not read; decide it in writeRoutes or noCredentialInResponse", route)
			}
			continue
		}
		method, _, _ := strings.Cut(route, " ")
		example := "https://api.umbrella.com" + strings.NewReplacer("{apiKeyId}", "k1", "{id}", "7", "{logoId}", "9").Replace(tool.Path)
		refused := errors.Is(refuseOneTimeSecret(method, example, nil), ErrOneTimeSecret)
		switch verdict {
		case "refused":
			if !refused {
				t.Errorf("%s is listed refused and the guard lets it go", route)
			}
		case "conditional":
			if !refused {
				t.Errorf("%s with no body should be refused", route)
			}
		default:
			if refused {
				t.Errorf("%s is listed %q and the guard refuses it", route, verdict)
			}
		}
	}
	if len(writes) < 50 {
		t.Fatalf("read only %d writes from the manifest; the scan is broken", len(writes))
	}
	for route := range writeRoutes {
		if !seen[route] {
			t.Errorf("writeRoutes names %s, which the manifest no longer has", route)
		}
	}
	for route := range noCredentialInResponse {
		if !seen[route] {
			t.Errorf("noCredentialInResponse names %s, which the manifest no longer has", route)
		}
	}
	src, err := os.ReadFile(filepath.Join("..", "types", "types.go"))
	if err != nil {
		t.Fatal(err)
	}
	declared := append(slices.Clone(credwire.Declarations.ResponseFields), "key")
	found := map[string]bool{}
	current := ""
	for _, line := range strings.Split(string(src), "\n") {
		if m := regexp.MustCompile(`^type (\w+) struct`).FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}
		if m := regexp.MustCompile("^\\s+\\w+\\s+\\S+\\s+`json:\"([^\",]+)").FindStringSubmatch(line); m != nil && slices.Contains(declared, m[1]) {
			key := current + "." + m[1]
			found[key] = true
			if _, ok := credentialTypes[key]; !ok {
				t.Errorf("internal/types: %s is a declared credential field no decision names; add it to credentialTypes, and refuse its route if Umbrella shows it only once", key)
			}
		}
	}
	var missing []string
	for key := range credentialTypes {
		if !found[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	for _, key := range missing {
		t.Errorf("credentialTypes names %s, which internal/types no longer has", key)
	}
}

// noCredentialInResponse are the other writes: checked against spec.yaml's documented
// success responses on 2026-11-03, none holds a declared credential field.
var noCredentialInResponse = map[string]bool{
	"DELETE /admin/v2/apiKeys/{apiKeyId}":                                             true,
	"DELETE /admin/v2/config/cnames/{cnameId}":                                        true,
	"DELETE /admin/v2/config/contacts/{contactId}":                                    true,
	"DELETE /admin/v2/config/logos/{logoId}":                                          true,
	"DELETE /admin/v2/managed/customers/{customerId}":                                 true,
	"DELETE /admin/v2/providers/customers/{customerId}":                               true,
	"DELETE /admin/v2/users/{userId}":                                                 true,
	"DELETE /deployments/v2/internaldomains/{internalDomainId}":                       true,
	"DELETE /deployments/v2/internalnetworks/{internalNetworkId}":                     true,
	"DELETE /deployments/v2/networkdevices/{originId}":                                true,
	"DELETE /deployments/v2/networks/{networkId}":                                     true,
	"DELETE /deployments/v2/policies/{policyId}/identities/{originId}":                true,
	"DELETE /deployments/v2/roamingcomputers/{deviceId}":                              true,
	"DELETE /deployments/v2/sites/{siteId}":                                           true,
	"DELETE /deployments/v2/tags/{tagId}/devices":                                     true,
	"DELETE /deployments/v2/tunnels/{id}":                                             true,
	"DELETE /deployments/v2/virtualappliances/{virtualApplianceId}":                   true,
	"DELETE /policies/v2/applicationLists/{applicationListId}":                        true,
	"DELETE /policies/v2/destinationlists/{destinationListId}":                        true,
	"DELETE /policies/v2/destinationlists/{destinationListId}/destinations/remove":    true,
	"PATCH /deployments/v2/networkdevices/{originId}":                                 true,
	"PATCH /policies/v2/destinationlists/{destinationListId}":                         true,
	"PATCH /reports/v2/appDiscovery/applications":                                     true,
	"PATCH /reports/v2/appDiscovery/applications/{applicationId}":                     true,
	"POST /admin/v2/config/cnames":                                                    true,
	"POST /admin/v2/config/contacts":                                                  true,
	"POST /admin/v2/managed/customers":                                                true,
	"POST /admin/v2/passwordResets/{customerId}":                                      true,
	"POST /admin/v2/providers/customers":                                              true,
	"POST /admin/v2/providers/customers/{customerId}/accessRequests":                  true,
	"POST /admin/v2/providers/customers/{customerId}/trialExtensions":                 true,
	"POST /deployments/v2/deviceSettings/SWGEnabled/list":                             true,
	"POST /deployments/v2/deviceSettings/SWGEnabled/remove":                           true,
	"POST /deployments/v2/deviceSettings/SWGEnabled/set":                              true,
	"POST /deployments/v2/internaldomains":                                            true,
	"POST /deployments/v2/internalnetworks":                                           true,
	"POST /deployments/v2/networkdevices":                                             true,
	"POST /deployments/v2/networks":                                                   true,
	"POST /deployments/v2/sites":                                                      true,
	"POST /deployments/v2/tags":                                                       true,
	"POST /deployments/v2/tags/{tagId}/devices":                                       true,
	"POST /investigate/v2/domains/categorization":                                     true,
	"POST /policies/v2/applicationLists":                                              true,
	"POST /policies/v2/destinationlists":                                              true,
	"POST /policies/v2/destinationlists/{destinationListId}/destinations":             true,
	"POST /reports/v2/identities":                                                     true,
	"POST /reports/v2/providers/customers/{customerId}/securityReportRequests":        true,
	"PUT /admin/v2/config/cnames/{cnameId}":                                           true,
	"PUT /admin/v2/config/contacts/{contactId}":                                       true,
	"PUT /admin/v2/managed/customers/{customerId}":                                    true,
	"PUT /admin/v2/providers/customerDeals/{dealId}":                                  true,
	"PUT /admin/v2/providers/customers/{customerId}":                                  true,
	"PUT /admin/v2/providers/customers/{customerId}/accessRequests/{accessRequestId}": true,
	"PUT /admin/v2/providers/customers/{customerId}/trialconversions":                 true,
	"PUT /deployments/v2/internaldomains/{internalDomainId}":                          true,
	"PUT /deployments/v2/internalnetworks/{internalNetworkId}":                        true,
	"PUT /deployments/v2/networks/{networkId}":                                        true,
	"PUT /deployments/v2/policies/{policyId}/identities/{originId}":                   true,
	"PUT /deployments/v2/roamingcomputers/{deviceId}":                                 true,
	"PUT /deployments/v2/sites/{siteId}":                                              true,
	"PUT /deployments/v2/tunnels/{id}":                                                true,
	"PUT /deployments/v2/virtualappliances/{virtualApplianceId}":                      true,
	"PUT /policies/v2/applicationLists/{applicationListId}":                           true,
}
