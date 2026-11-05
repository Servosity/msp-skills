package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddAppleClientAssertion(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	keyFile := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(keyFile, []byte(pemText), 0o600); err != nil {
		t.Fatal(err)
	}

	sec1, _ := x509.MarshalECPrivateKey(key)
	withParams := "-----BEGIN EC PARAMETERS-----\nBggqhkjOPQMBBw==\n-----END EC PARAMETERS-----\n" +
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1}))
	escaped := strings.ReplaceAll(pemText, "\n", `\n`)
	quoted := `"` + escaped + `"`
	for _, privateKey := range []string{pemText, keyFile, withParams, escaped, quoted} {
		form := url.Values{"grant_type": {"client_credentials"}}
		if err := AddAppleClientAssertion(form, "BUSINESSAPI.abc", privateKey, "kid-1"); err != nil {
			t.Fatalf("AddAppleClientAssertion: %v", err)
		}
		if form.Get("client_id") != "BUSINESSAPI.abc" || form.Get("client_assertion_type") != "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" {
			t.Fatalf("form fields wrong: %v", form)
		}
		parts := strings.Split(form.Get("client_assertion"), ".")
		if len(parts) != 3 {
			t.Fatalf("assertion is not a compact JWS")
		}
		var hdr map[string]string
		var claims map[string]any
		h, _ := base64.RawURLEncoding.DecodeString(parts[0])
		c, _ := base64.RawURLEncoding.DecodeString(parts[1])
		_ = json.Unmarshal(h, &hdr)
		_ = json.Unmarshal(c, &claims)
		if hdr["alg"] != "ES256" || hdr["kid"] != "kid-1" {
			t.Fatalf("header = %v", hdr)
		}
		if claims["sub"] != "BUSINESSAPI.abc" || claims["iss"] != "BUSINESSAPI.abc" || claims["aud"] != AppleAssertionAudience {
			t.Fatalf("claims = %v", claims)
		}
		if claims["exp"].(float64) <= claims["iat"].(float64) {
			t.Fatalf("exp must be after iat")
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
		if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
			t.Fatalf("signature does not verify")
		}
	}

	if err := AddAppleClientAssertion(url.Values{}, "id", pemText, ""); err == nil {
		t.Fatal("missing key ID must error")
	}
	if err := AddAppleClientAssertion(url.Values{}, "id", "/nonexistent/key.pem", "kid"); err == nil || strings.Contains(err.Error(), "/nonexistent") {
		t.Fatalf("unreadable key must error without echoing the value: %v", err)
	}
}
