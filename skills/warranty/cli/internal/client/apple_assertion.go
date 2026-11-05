// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json warranty-apple-auth): Apple Business OAuth client
// assertion, ported from the apple-business connector (internal/client/apple_assertion.go).
//
// Apple's token endpoint does not accept HTTP Basic client auth. It wants a
// short-lived ES256 JWT signed with the API account's private key, sent as
// client_assertion. Wire format per Apple's "Implementing OAuth for the Apple
// School Manager and Apple Business API" and AxMJamfSync ABMService.swift.

package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"warranty-pp-cli/internal/credkit"
)

// AppleOAuthAudience is the aud claim Apple requires; it differs from the
// token URL the request is POSTed to.
const AppleOAuthAudience = "https://account.apple.com/auth/oauth2/v2/token"

// appleAssertionLifetime keeps a leaked assertion useless quickly. Apple
// allows up to 180 days; nothing here needs more than one token request.
const appleAssertionLifetime = 10 * time.Minute

// AddAppleClientAssertion adds client_id, client_assertion_type and
// client_assertion to a client_credentials token form. privateKey is the
// API account's PEM key text or a path to the downloaded .pem file.
func AddAppleClientAssertion(form url.Values, clientID, privateKey, keyID string) error {
	clientID, keyID = strings.TrimSpace(clientID), strings.TrimSpace(keyID)
	if clientID == "" {
		return fmt.Errorf("Apple Business client ID is required (WARRANTY_APPLE_CLIENT_ID)")
	}
	if keyID == "" {
		return fmt.Errorf("Apple Business key ID is required (WARRANTY_APPLE_KEY_ID): the Key ID shown for the API account's private key")
	}
	key, err := loadAppleSigningKey(privateKey)
	if err != nil {
		return err
	}
	assertion, err := signAppleClientAssertion(key, clientID, keyID, time.Now())
	if err != nil {
		return err
	}
	form.Set("client_id", clientID)
	form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	credkit.Hold(sourceAppleAssertion, assertion) // a signed assertion is a credential for its lifetime
	form.Set("client_assertion", assertion)
	return nil
}

// applePEM is the PEM text privateKey names: itself, or the file it is a path of.
func applePEM(privateKey string) (string, error) {
	raw := strings.TrimSpace(privateKey)
	// Values copied from .env files often keep their quotes.
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		raw = strings.TrimSpace(raw[1 : len(raw)-1])
	}
	if raw == "" {
		return "", fmt.Errorf("Apple Business private key is required (WARRANTY_APPLE_PRIVATE_KEY): PEM text or a path to the .pem file")
	}
	if !strings.Contains(raw, "-----BEGIN") {
		path := raw
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[2:])
			}
		}
		data, err := os.ReadFile(filepath.Clean(path)) // the operator's own key file named by WARRANTY_APPLE_PRIVATE_KEY
		if err != nil {
			// Never echo the value: it may be a mistyped key, not a path.
			return "", fmt.Errorf("WARRANTY_APPLE_PRIVATE_KEY is neither PEM text nor a readable file path")
		}
		raw = string(data)
	}
	// .env files and MCP configs often carry the PEM with literal "\n".
	return strings.ReplaceAll(raw, `\n`, "\n"), nil
}

func loadAppleSigningKey(privateKey string) (*ecdsa.PrivateKey, error) {
	raw, err := applePEM(privateKey)
	if err != nil {
		return nil, err
	}
	var block *pem.Block
	for rest := []byte(raw); ; {
		block, rest = pem.Decode(rest)
		if block == nil || block.Type == "PRIVATE KEY" || block.Type == "EC PRIVATE KEY" {
			break
		}
	}
	if block != nil {
		credkit.Hold(sourceAppleKey, appleKeyForms(block.Bytes)...)
	}
	if block == nil {
		return nil, fmt.Errorf("Apple Business private key is not valid PEM (expected a PRIVATE KEY or EC PRIVATE KEY block)")
	}
	var key *ecdsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("Apple Business private key must be an EC (P-256) key")
		}
		key = ec
	} else if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		key = k
	} else {
		return nil, fmt.Errorf("Apple Business private key could not be parsed as PKCS#8 or SEC1 EC")
	}
	if key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("Apple Business private key must use curve P-256 (ES256)")
	}
	return key, nil
}

func signAppleClientAssertion(key *ecdsa.PrivateKey, clientID, keyID string, now time.Time) (string, error) {
	jti := make([]byte, 16)
	if _, err := rand.Read(jti); err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID})
	claims, _ := json.Marshal(map[string]any{
		"sub": clientID,
		"iss": clientID,
		"aud": AppleOAuthAudience,
		"iat": now.Unix(),
		"exp": now.Add(appleAssertionLifetime).Unix(),
		"jti": hex.EncodeToString(jti),
	})
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", fmt.Errorf("signing client assertion: %w", err)
	}
	// JWS ES256 wants raw r||s, each left-padded to 32 bytes (not ASN.1 DER).
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + enc.EncodeToString(sig), nil
}

// appleKeyForms is an Apple key's DER in the text forms the kit holds. Not the
// PEM framing: "PRIVATE KEY" is words any help text may use, and an 8-character
// piece of it would withhold them. Not the raw DER: the program never prints it,
// and with raw bytes held the kit's check of each output unit takes seconds.
// ponytail: base64 and hex are the forms anyone would paste; hold the raw bytes
// too if the kit's check gets cheap for them.
func appleKeyForms(der []byte) []string {
	return []string{base64.StdEncoding.EncodeToString(der), hex.EncodeToString(der)}
}

// appleKeyHeld is what to hold for the value of WARRANTY_APPLE_PRIVATE_KEY: the
// key it names, never a path. A value that is neither PEM nor a readable file
// may be a mistyped key, so it is held as it stands, and likewise a file with no
// PEM block in it.
// Hand-written (handfixes.json warranty-apple-auth).
func appleKeyHeld(privateKey string) []string {
	text, err := applePEM(privateKey)
	if err != nil {
		return []string{strings.TrimSpace(privateKey)}
	}
	var out []string
	for rest := []byte(text); ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		out = append(out, appleKeyForms(block.Bytes)...)
	}
	if out == nil {
		return []string{text}
	}
	return out
}
